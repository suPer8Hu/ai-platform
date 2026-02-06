package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

	provider := strings.TrimSpace(req.Provider)
	model := strings.TrimSpace(req.Model)
	if provider == "" {
		provider = h.Cfg.AIProvider
	}
	if model == "" {
		switch strings.ToLower(provider) {
		case "openrouter":
			model = h.Cfg.OpenRouterModel
		case "ollama", "":
			model = h.Cfg.OllamaModel
		}
	}

	sess, err := h.ChatSvc.CreateSession(c.Request.Context(), uid, provider, model)
	if err != nil {
		fail(c, http.StatusInternalServerError, 50001, "failed to create session")
		return
	}

	ok(c, gin.H{"session_id": sess.SessionID})
}

func (h *Handler) ListChatSessions(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	var beforeID uint64
	if s := c.Query("before_id"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 64); err == nil {
			beforeID = n
		}
	}

	sess, err := h.ChatSvc.ListSessions(c.Request.Context(), uid, limit, beforeID)
	if err != nil {
		fail(c, http.StatusInternalServerError, 50003, "failed to list sessions")
		return
	}

	var nextBeforeID *uint64
	if len(sess) > 0 {
		v := sess[len(sess)-1].ID
		nextBeforeID = &v
	}

	ok(c, gin.H{
		"sessions":       sess,
		"next_before_id": nextBeforeID,
	})
}

type updateSessionTitleReq struct {
	Title string `json:"title"`
}

func (h *Handler) UpdateChatSessionTitle(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	sessionID := c.Param("session_id")
	if sessionID == "" {
		fail(c, http.StatusBadRequest, 10002, "session_id required")
		return
	}

	var req updateSessionTitleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 10001, "invalid json")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		fail(c, http.StatusBadRequest, 10002, "title required")
		return
	}
	if utf8.RuneCountInString(title) > 128 {
		fail(c, http.StatusBadRequest, 10002, "title too long")
		return
	}

	if err := h.ChatSvc.UpdateSessionTitle(c.Request.Context(), uid, sessionID, title); err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40401, "session not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50004, "failed to update session title")
		return
	}

	ok(c, gin.H{"session_id": sessionID, "title": title})
}

func (h *Handler) DeleteChatSession(c *gin.Context) {
	uid, okk := userIDFromContext(c)
	if !okk {
		fail(c, http.StatusUnauthorized, 40101, "unauthorized")
		return
	}

	sessionID := c.Param("session_id")
	if sessionID == "" {
		fail(c, http.StatusBadRequest, 10002, "session_id required")
		return
	}

	if err := h.ChatSvc.DeleteSession(c.Request.Context(), uid, sessionID); err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(c, http.StatusNotFound, 40401, "session not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50005, "failed to delete session")
		return
	}

	ok(c, gin.H{"session_id": sessionID, "deleted": true})
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

	// idempotency key (optional)
	idempoKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idempoKey) > 128 {
		fail(c, http.StatusBadRequest, 10003, "idempotency key too long")
		return
	}
	var idempoKeyPtr *string
	if idempoKey != "" {
		idempoKeyPtr = &idempoKey
	}

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // helpful if behind nginx

	// avoid gin writing a JSON response later
	c.Status(http.StatusOK)

	ctx := c.Request.Context()
	chunks, done, msgIDCh, errs := h.ChatSvc.SendMessageStream(ctx, uid, req.SessionID, req.Message, idempoKeyPtr)

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

	// Build job (ID only matters if we end up creating a new row)
	jobID, err := common.NewULID()
	if err != nil {
		log.Printf("[SendChatMessageAsync] NewULID failed uid=%d session_id=%s err=%v", uid, req.SessionID, err)
		fail(c, http.StatusInternalServerError, 50001, "internal error")
		return
	}

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
		// no idempotency -> always new job
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
		j = job
	}

	// Only the creator should insert the user message and enqueue the job.
	if created {
		// Insert user message (idempotent when key is present)
		if idempoKeyPtr == nil {
			if err := h.ChatSvc.InsertUserMessage(c.Request.Context(), uid, req.SessionID, req.Message); err != nil {
				if err == gorm.ErrRecordNotFound {
					fail(c, http.StatusNotFound, 40401, "session not found")
					return
				}
				log.Printf("[SendChatMessageAsync] InsertUserMessage failed uid=%d session_id=%s err=%v", uid, req.SessionID, err)
				fail(c, http.StatusInternalServerError, 50001, "internal error")
				return
			}
		} else {
			if _, _, err := h.ChatSvc.InsertUserMessageOrGetExisting(c.Request.Context(), uid, req.SessionID, req.Message, idempoKeyPtr); err != nil {
				log.Printf("[SendChatMessageAsync] InsertUserMessageOrGetExisting failed uid=%d session_id=%s key=%s err=%v", uid, req.SessionID, idempoKey, err)
				fail(c, http.StatusInternalServerError, 50001, "internal error")
				return
			}
		}

		// Enqueue
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
