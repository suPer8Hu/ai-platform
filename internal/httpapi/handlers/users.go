package handlers

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/suPer8Hu/ai-platform/internal/auth"
	"github.com/suPer8Hu/ai-platform/internal/common"
	"github.com/suPer8Hu/ai-platform/internal/email"
	"github.com/suPer8Hu/ai-platform/internal/models"
	"gorm.io/gorm"
)

type createUserReq struct {
	Email    string `json:"email"`
	Captcha  string `json:"captcha"`
	Password string `json:"password"`
}

// generate a 11 digit random username
func randomUsername11() (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, 11)
	for i := 0; i < 11; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		out[i] = letters[n.Int64()]
	}
	return string(out), nil
}

func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Fail(c, http.StatusBadRequest, 10001, "invalid json")
		return
	}
	if req.Email == "" || req.Password == "" || req.Captcha == "" {
		common.Fail(c, http.StatusBadRequest, 10002, "email, captcha and password required")
		return
	}

	// redis verification
	code, err := h.Redis.GetCaptcha(c.Request.Context(), req.Email)
	if err != nil {
		if err == redis.Nil {
			common.Fail(c, http.StatusBadRequest, 10020, "captcha expired or not found")
			return
		}
		common.Fail(c, http.StatusInternalServerError, 20001, "redis error")
		return
	}
	if code != req.Captcha {
		common.Fail(c, http.StatusBadRequest, 10021, "invalid captcha")
		return
	}
	_ = h.Redis.DeleteCaptcha(c.Request.Context(), req.Email)

	// TODO: change to bcrypt hash
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		common.Fail(c, http.StatusInternalServerError, 20002, "failed to hash password")
		return
	}

	// generate username to avoid conflict
	var username string
	for i := 0; i < 5; i++ {
		u, err := randomUsername11()
		if err != nil {
			common.Fail(c, http.StatusInternalServerError, 20004, "failed to generate username")
			return
		}

		var cnt int64
		if err := h.DB.Model(&models.User{}).Where("username = ?", u).Count(&cnt).Error; err != nil {
			common.Fail(c, http.StatusInternalServerError, 20005, "failed to check username")
			return
		}
		if cnt == 0 {
			username = u
			break
		}
	}
	if username == "" {
		common.Fail(c, http.StatusInternalServerError, 20006, "failed to allocate username")
		return
	}

	// create user
	user := models.User{
		Email:        req.Email,
		Username:     username,
		PasswordHash: hash,
	}
	if err := h.DB.Create(&user).Error; err != nil {
		common.Fail(c, http.StatusBadRequest, 10003, "failed to create user (maybe email already exists)")
		return
	}

	// sign token
	token, err := auth.SignJWT(user.ID, h.Cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		common.Fail(c, http.StatusInternalServerError, 20003, "failed to sign token")
		return
	}

	// send welcome email
	go func(to, uname string) {
		subject := "Welcome to GopherChat — Your account is ready"
		body := "Hello,\n\n" +
			"Welcome to GopherChat. Your account has been successfully created.\n\n" +
			"Username: " + uname + "\n\n" +
			"If you did not request this account, please contact our support immediately.\n\n" +
			"Best regards,\n" +
			"GopherChat\n"
		_ = email.SendText(h.SMTPSetting, to, subject, body)
	}(user.Email, user.Username)

	common.OK(c, gin.H{
		"id":       user.ID,
		"email":    user.Email,
		"username": user.Username,
		"token":    token,
	})
}

func (h *Handler) GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		common.Fail(c, http.StatusBadRequest, 10004, "invalid user id")
		return
	}

	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.Fail(c, http.StatusNotFound, 40401, "user not found")
			return
		}
		common.Fail(c, http.StatusInternalServerError, 20001, "db error")
		return
	}

	common.OK(c, gin.H{
		"id":         user.ID,
		"email":      user.Email,
		"created_at": user.CreatedAt,
	})
}
