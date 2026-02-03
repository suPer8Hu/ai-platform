package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/suPer8Hu/ai-platform/internal/chat"
	"github.com/suPer8Hu/ai-platform/internal/common"
	"github.com/suPer8Hu/ai-platform/internal/httpapi/middleware"
	"gorm.io/gorm"
)

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}

func fail(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, gin.H{
		"code":    code,
		"message": msg,
		"data":    nil,
	})
}

func userIDFromContext(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint64)
	return id, ok
}

type createSessionReq struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (h *Handler) CreateChatSession(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	var req createSessionReq
	_ = c.ShouldBindJSON(&req) // allow empty {}

	sess, err := h.ChatSvc.CreateSession(c.Request.Context(), uid, req.Provider, req.Model)
	if err != nil {
		fail(c, http.StatusInternalServerError, 50001, "failed to create session")
		return
	}

	ok(c, gin.H{"session_id": sess.SessionID})
}

type sendMessageReq struct {
	SessionID string `json:"session_id" binding:"required"`
	Message   string `json:"message" binding:"required"`
}

func (h *Handler) SendChatMessage(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	var req sendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 10001, "invalid json")
		return
	}

	reply, msgID, err := h.ChatSvc.SendMessage(c.Request.Context(), uid, req.SessionID, req.Message)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40004, "session not found")
			return
		}
		fail(c, http.StatusBadRequest, 40001, "failed to send message")
		return
	}

	ok(c, gin.H{
		"session_id": req.SessionID,
		"reply":      reply,
		"message_id": msgID,
	})
}

func (h *Handler) ListChatMessages(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	sessionID := c.Param("session_id")

	limit, _ := strconv.Atoi(c.Query("limit"))
	beforeIDStr := c.Query("before_id")
	var beforeID uint64
	if beforeIDStr != "" {
		if n, err := strconv.ParseUint(beforeIDStr, 10, 64); err == nil {
			beforeID = n
		}
	}

	msgs, err := h.ChatSvc.ListMessages(c.Request.Context(), uid, sessionID, limit, beforeID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40004, "session not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50002, "failed to list messages")
		return
	}

	var nextBeforeID uint64
	if len(msgs) > 0 {
		nextBeforeID = msgs[len(msgs)-1].ID
	}

	ok(c, gin.H{
		"messages":       msgs,
		"next_before_id": nextBeforeID,
	})
}

func (h *Handler) SendChatMessageStream(c *gin.Context) {
	type reqBody struct {
		SessionID string `json:"session_id" binding:"required"`
		Message   string `json:"message" binding:"required"`
	}

	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	var req reqBody
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 10001, "invalid json")
		return
	}

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // helpful if behind nginx

	// avoid gin writing a JSON response later
	c.Status(http.StatusOK)

	ctx := c.Request.Context()
	chunks, done, msgIDCh, errs := h.ChatSvc.SendMessageStream(ctx, uid, req.SessionID, req.Message)

	// heartbeat ticker (keeps connections alive)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		// can't stream
		fmt.Fprintf(c.Writer, "event: error\ndata: flusher not supported\n\n")
		return
	}

	writeJSON := func(event string, payload any) {
		b, err := json.Marshal(payload)
		if err != nil {
			// last-resort: send a simple error that won't break SSE framing
			fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":\"json marshal failed\"}\n\n")
			flusher.Flush()
			return
		}
		if event != "" {
			fmt.Fprintf(c.Writer, "event: %s\n", event)
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(b))
		flusher.Flush()
	}

	for {
		select {
		case ch, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			writeJSON("chunk", gin.H{
				"type":  "chunk",
				"delta": ch,
			})

		case <-ticker.C:
			writeJSON("ping", gin.H{
				"type": "ping",
				"ts":   time.Now().Unix(),
			})

		case err := <-errs:
			if err == nil {
				continue
			}
			if err == gorm.ErrRecordNotFound {
				writeJSON("error", gin.H{
					"type":    "error",
					"message": "session not found",
				})
				return
			}
			writeJSON("error", gin.H{
				"type":    "error",
				"message": err.Error(),
			})
			return

		case <-done:
			var mid uint64
			select {
			case mid = <-msgIDCh:
			default:
			}
			writeJSON("done", gin.H{
				"type":       "done",
				"message_id": mid,
			})
			return

		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) SendChatMessageAsync(c *gin.Context) {
	type reqBody struct {
		SessionID string `json:"session_id" binding:"required"`
		Message   string `json:"message" binding:"required"`
	}
	var req reqBody

	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 10001, "invalid json")
		return
	}

	// read idempotency key
	idempoKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idempoKey) > 128 {
		fail(c, http.StatusBadRequest, 10003, "idempotency key too long")
		return
	}

	var idempoKeyPtr *string
	if idempoKey != "" {
		idempoKeyPtr = &idempoKey
	}

	// Validate session belongs to user
	if err := h.ChatSvc.ValidateSessionOwner(c.Request.Context(), uid, req.SessionID); err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40401, "session not found")
			return
		}
		log.Printf("[SendChatMessageAsync] ValidateSessionOwner failed uid=%d session_id=%s err=%v", uid, req.SessionID, err)
		fail(c, http.StatusInternalServerError, 50001, "internal error")
		return
	}

	// Insert user message immediately (A-mode)
	// NOTE: still not idempotent; can be addressed later with message-level idempotency.
	if err := h.ChatSvc.InsertUserMessage(c.Request.Context(), uid, req.SessionID, req.Message); err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40401, "session not found")
			return
		}
		log.Printf("[SendChatMessageAsync] InsertUserMessage failed uid=%d session_id=%s err=%v", uid, req.SessionID, err)
		fail(c, http.StatusInternalServerError, 50001, "internal error")
		return
	}

	jobID, err := common.NewULID()
	if err != nil {
		log.Printf("[SendChatMessageAsync] NewULID failed uid=%d session_id=%s err=%v", uid, req.SessionID, err)
		fail(c, http.StatusInternalServerError, 50001, "internal error")
		return
	}

	// Create job row (idempotent if key is provided)
	j := &chat.Job{
		ID:             jobID,
		UserID:         uid,
		SessionID:      req.SessionID,
		Prompt:         req.Message,
		IdempotencyKey: idempoKeyPtr,
		Status:         chat.JobQueued,
	}

	created := true
	if idempoKeyPtr == nil {
		// backward-compatible: always new job
		if err := h.ChatSvc.CreateJob(c.Request.Context(), j); err != nil {
			log.Printf("[SendChatMessageAsync] CreateJob failed uid=%d session_id=%s job_id=%s err=%v", uid, req.SessionID, jobID, err)
			fail(c, http.StatusInternalServerError, 50001, "internal error")
			return
		}
	} else {
		var job *chat.Job
		job, created, err = h.ChatSvc.CreateJobOrGetExisting(c.Request.Context(), j)
		if err != nil {
			log.Printf("[SendChatMessageAsync] CreateJobOrGetExisting failed uid=%d session_id=%s job_id=%s key=%s err=%v", uid, req.SessionID, jobID, idempoKey, err)
			fail(c, http.StatusInternalServerError, 50001, "internal error")
			return
		}
		// If existing, use its ID for response/publish decision
		j = job
	}

	// Enqueue only when a new job was created
	if created {
		if err := h.Rabbit.PublishJob(c.Request.Context(), j.ID); err != nil {
			log.Printf("[SendChatMessageAsync] PublishJob failed uid=%d session_id=%s job_id=%s err=%v", uid, req.SessionID, j.ID, err)
			fail(c, http.StatusInternalServerError, 50002, "enqueue failed")
			return
		}
	}

	ok(c, gin.H{"job_id": j.ID})
}

func (h *Handler) GetChatJob(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}
	jobID := c.Param("job_id")
	if jobID == "" {
		fail(c, http.StatusBadRequest, 10002, "job_id required")
		return
	}

	j, err := h.ChatSvc.GetJob(c.Request.Context(), jobID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40402, "job not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50001, "internal error")
		return
	}
	if j.UserID != uid {
		// hide existence
		fail(c, http.StatusNotFound, 40402, "job not found")
		return
	}

	ok(c, gin.H{
		"job": gin.H{
			"id":                j.ID,
			"session_id":        j.SessionID,
			"status":            j.Status,
			"result_message_id": j.ResultMessageID,
			"error":             j.Error,
			"created_at":        j.CreatedAt,
			"updated_at":        j.UpdatedAt,
		},
	})
}
