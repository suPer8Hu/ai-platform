package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/suPer8Hu/ai-platform/internal/common"
	"github.com/suPer8Hu/ai-platform/internal/config"
	"github.com/suPer8Hu/ai-platform/internal/httpapi/handlers"
	"github.com/suPer8Hu/ai-platform/internal/httpapi/middleware"
	"github.com/suPer8Hu/ai-platform/internal/store/redisstore"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, cfg config.Config, rds *redisstore.Store) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(gin.Logger())
	// r.Use(gin.Recovery())
	r.Use(middleware.Recovery())

	r.NoRoute(func(c *gin.Context) {
		common.Fail(c, http.StatusNotFound, 40400, "route not found")
	})
	r.NoMethod(func(c *gin.Context) {
		common.Fail(c, http.StatusMethodNotAllowed, 40500, "method not allowed")
	})

	r.Use(middleware.RequestID())

	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:3001",
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "Idempotency-Key"},
		ExposeHeaders: []string{
			"X-Request-Id",
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	h := handlers.NewHandler(db, cfg, rds)

	r.GET("/ping", h.Ping)

	// captcha
	r.POST("/captcha", h.SendCaptcha)

	// CRUD users register
	r.POST("/users", h.CreateUser)
	r.GET("/users/:id", h.GetUserByID)

	// auth
	r.POST("/login", h.Login)
	authGroup := r.Group("/")
	authGroup.Use(middleware.AuthRequired(cfg.JWTSecret))
	authGroup.GET("/me", h.Me)
	// Chat (JWT required)
	authGroup.POST("/chat/sessions", h.CreateChatSession)
	authGroup.GET("/chat/sessions", h.ListChatSessions)
	authGroup.POST("/chat/messages", h.SendChatMessage)
	authGroup.POST("/chat/messages/stream", h.SendChatMessageStream)
	authGroup.POST("/chat/messages/async", h.SendChatMessageAsync)
	authGroup.GET("/chat/sessions/:session_id/messages", h.ListChatMessages)
	authGroup.GET("/chat/jobs/:job_id", h.GetChatJob)

	return r
}
