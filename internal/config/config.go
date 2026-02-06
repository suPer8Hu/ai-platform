package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DBDSN         string
	JWTSecret     string
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	SMTPHost              string
	SMTPPort              int
	SMTPUser              string
	SMTPPass              string
	SMTPFrom              string
	ChatContextWindowSize int

	// AI provider
	AIProvider         string
	OllamaBaseURL      string
	OllamaModel        string
	OpenRouterBaseURL  string
	OpenRouterAPIKey   string
	OpenRouterModel    string
	OpenRouterSiteURL  string
	OpenRouterAppName  string

	// rabbitMQ
	RabbitURL   string
	RabbitQueue string
}

func Load() Config {
	// DSN demo：
	// app:apppass@tcp(127.0.0.1:3306)/ai_platform?charset=utf8mb4&parseTime=true&loc=Local
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
			"app", "apppass", "127.0.0.1", "3306", "ai_platform",
		)
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}

	redisDB := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			redisDB = n
		}
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := 587
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			smtpPort = n
		}
	}
	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		smtpFrom = os.Getenv("SMTP_USER")
	}

	windowSize := 20
	if v := os.Getenv("CHAT_CONTEXT_WINDOW_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			windowSize = n
		}
	}

	// AI provider config
	aiProvider := os.Getenv("AI_PROVIDER")
	if aiProvider == "" {
		aiProvider = "ollama"
	}

	ollamaBaseURL := os.Getenv("OLLAMA_BASE_URL")
	if ollamaBaseURL == "" {
		ollamaBaseURL = "http://localhost:11434"
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama3:latest"
	}

	openRouterBaseURL := os.Getenv("OPENROUTER_BASE_URL")
	if openRouterBaseURL == "" {
		openRouterBaseURL = "https://openrouter.ai/api/v1"
	}
	openRouterModel := os.Getenv("OPENROUTER_MODEL")
	if openRouterModel == "" {
		openRouterModel = "openrouter/auto"
	}

	// rabbitMQ config
	rabbitURL := os.Getenv("RABBIT_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}
	rabbitQueue := os.Getenv("RABBIT_QUEUE")
	if rabbitQueue == "" {
		rabbitQueue = "chat_jobs"
	}

	return Config{
		DBDSN:     dsn,
		JWTSecret: secret,

		RedisAddr:     redisAddr,
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,

		SMTPHost:              smtpHost,
		SMTPPort:              smtpPort,
		SMTPUser:              os.Getenv("SMTP_USER"),
		SMTPPass:              os.Getenv("SMTP_PASS"),
		SMTPFrom:              smtpFrom,
		ChatContextWindowSize: windowSize,

		AIProvider:        aiProvider,
		OllamaBaseURL:     ollamaBaseURL,
		OllamaModel:       ollamaModel,
		OpenRouterBaseURL: openRouterBaseURL,
		OpenRouterAPIKey:  os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:   openRouterModel,
		OpenRouterSiteURL: os.Getenv("OPENROUTER_SITE_URL"),
		OpenRouterAppName: os.Getenv("OPENROUTER_APP_NAME"),

		RabbitURL:   rabbitURL,
		RabbitQueue: rabbitQueue,
	}
}
