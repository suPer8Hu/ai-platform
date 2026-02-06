package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenRouterProvider struct {
	BaseURL string
	APIKey  string
	Model   string
	SiteURL string
	AppName string
	Client  *http.Client
}

type openRouterMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterChatReq struct {
	Model    string         `json:"model"`
	Messages []openRouterMsg `json:"messages"`
	Stream   bool           `json:"stream"`
}

type openRouterChatResp struct {
	Choices []struct {
		Message openRouterMsg `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type openRouterStreamResp struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func NewOpenRouterProvider(baseURL, apiKey, model, siteURL, appName string) *OpenRouterProvider {
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		SiteURL: siteURL,
		AppName: appName,
		Client:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (p *OpenRouterProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	if p.Client == nil {
		return "", errors.New("openrouter: http client is nil")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return "", errors.New("openrouter: api key is required")
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return "", errors.New("openrouter: model is required")
	}

	reqBody := openRouterChatReq{
		Model:  model,
		Stream: false,
		Messages: func() []openRouterMsg {
			out := make([]openRouterMsg, 0, len(messages))
			for _, m := range messages {
				out = append(out, openRouterMsg{Role: m.Role, Content: m.Content})
			}
			return out
		}(),
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(p.BaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	if p.SiteURL != "" {
		req.Header.Set("HTTP-Referer", p.SiteURL)
	}
	if p.AppName != "" {
		req.Header.Set("X-Title", p.AppName)
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("openrouter: %s", msg)
	}

	var decoded openRouterChatResp
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", errors.New(decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("openrouter: empty response")
	}
	return decoded.Choices[0].Message.Content, nil
}

// StreamChat streams assistant content chunks via SSE.
func (p *OpenRouterProvider) StreamChat(ctx context.Context, messages []Message) (<-chan string, <-chan error) {
	chunks := make(chan string, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		if p.Client == nil {
			errs <- errors.New("openrouter: http client is nil")
			return
		}
		if strings.TrimSpace(p.APIKey) == "" {
			errs <- errors.New("openrouter: api key is required")
			return
		}
		model := strings.TrimSpace(p.Model)
		if model == "" {
			errs <- errors.New("openrouter: model is required")
			return
		}

		reqBody := openRouterChatReq{
			Model:  model,
			Stream: true,
			Messages: func() []openRouterMsg {
				out := make([]openRouterMsg, 0, len(messages))
				for _, m := range messages {
					out = append(out, openRouterMsg{Role: m.Role, Content: m.Content})
				}
				return out
			}(),
		}

		b, err := json.Marshal(reqBody)
		if err != nil {
			errs <- err
			return
		}

		url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(p.BaseURL, "/"))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			errs <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
		if p.SiteURL != "" {
			req.Header.Set("HTTP-Referer", p.SiteURL)
		}
		if p.AppName != "" {
			req.Header.Set("X-Title", p.AppName)
		}

		if p.Client.Timeout < 30*time.Second {
			p.Client.Timeout = 0
		}

		resp, err := p.Client.Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
			msg := strings.TrimSpace(string(body))
			if msg == "" {
				msg = fmt.Sprintf("status %d", resp.StatusCode)
			}
			errs <- fmt.Errorf("openrouter: %s", msg)
			return
		}

		sc := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 2*1024*1024)

		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}
			var decoded openRouterStreamResp
			if err := json.Unmarshal([]byte(data), &decoded); err != nil {
				errs <- err
				return
			}
			if decoded.Error != nil && decoded.Error.Message != "" {
				errs <- errors.New(decoded.Error.Message)
				return
			}
			if len(decoded.Choices) == 0 {
				continue
			}
			delta := decoded.Choices[0].Delta.Content
			if delta != "" {
				chunks <- delta
			}
		}

		if err := sc.Err(); err != nil {
			errs <- err
			return
		}
	}()

	return chunks, errs
}
