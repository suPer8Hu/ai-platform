package handlers

import (
	"context"
	"strings"

	"github.com/suPer8Hu/ai-platform/internal/ai"
	"github.com/suPer8Hu/ai-platform/internal/chat"
	"github.com/suPer8Hu/ai-platform/internal/config"
	"github.com/suPer8Hu/ai-platform/internal/email"
	"github.com/suPer8Hu/ai-platform/internal/store/rabbitmq"
	"github.com/suPer8Hu/ai-platform/internal/store/redisstore"
	"gorm.io/gorm"
)

type Handler struct {
	DB          *gorm.DB
	Cfg         config.Config
	Redis       *redisstore.Store
	SMTPSetting email.SMTPConfig
	ChatSvc     *chat.Service
	Rabbit      *rabbitmq.Publisher
}

func NewHandler(db *gorm.DB, cfg config.Config, r *redisstore.Store) *Handler {
	repo := chat.NewRepo(db)
	// real provider
	// provider := ai.NewOllamaProvider("http://localhost:11434", "llama3:latest")
	// var provider ai.Provider
	// switch strings.ToLower(cfg.AIProvider) {
	// case "", "ollama":
	// 	provider = ai.NewOllamaProvider(cfg.OllamaBaseURL, cfg.OllamaModel)
	// default:
	// 	panic(fmt.Sprintf("unsupported AI_PROVIDER=%q", cfg.AIProvider))
	// }

	// Provider registry (route by session.Provider + session.Model)
	reg := ai.NewRegistry()

	// Register Ollama (default)
	reg.Register("ollama", func(ctx context.Context, model string) (ai.Provider, error) {
		m := strings.TrimSpace(model)
		if m == "" {
			m = cfg.OllamaModel
		}
		return ai.NewOllamaProvider(cfg.OllamaBaseURL, m), nil
	})

	// Register OpenRouter (OpenAI-compatible)
	reg.Register("openrouter", func(ctx context.Context, model string) (ai.Provider, error) {
		_ = ctx
		m := strings.TrimSpace(model)
		if m == "" {
			m = cfg.OpenRouterModel
		}
		return ai.NewOpenRouterProvider(
			cfg.OpenRouterBaseURL,
			cfg.OpenRouterAPIKey,
			m,
			cfg.OpenRouterSiteURL,
			cfg.OpenRouterAppName,
		), nil
	})

	chatSvc := chat.NewService(repo, reg, cfg.ChatContextWindowSize)

	// rabbitmq
	pub, err := rabbitmq.NewPublisher(cfg.RabbitURL, cfg.RabbitQueue)
	if err != nil {
		panic(err)
	}
	return &Handler{DB: db, Cfg: cfg, Redis: r, SMTPSetting: email.SMTPConfig{Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom},
		ChatSvc: chatSvc,
		Rabbit:  pub,
	}
}
