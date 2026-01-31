package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type OllamaProvider struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

type ollamaStreamResp struct {
	Message ollamaMsg `json:"message"`
	Done    bool      `json:"done"`
	Error   string    `json:"error,omitempty"`
}

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3:latest"
	}
	return &OllamaProvider{
		BaseURL: baseURL,
		Model:   model,
		Client:  &http.Client{Timeout: 90 * time.Second},
	}
}

type ollamaChatReq struct {
	Model    string      `json:"model"`
	Messages []ollamaMsg `json:"messages"`
	Stream   bool        `json:"stream"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResp struct {
	Message ollamaMsg `json:"message"`
	Error   string    `json:"error,omitempty"`
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	if p.Client == nil {
		return "", errors.New("ollama: http client is nil")
	}

	reqBody := ollamaChatReq{
		Model:  p.Model,
		Stream: false,
		Messages: func() []ollamaMsg {
			out := make([]ollamaMsg, 0, len(messages))
			for _, m := range messages {
				out = append(out, ollamaMsg{Role: m.Role, Content: m.Content})
			}
			return out
		}(),
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/chat", p.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama: status %d", resp.StatusCode)
	}

	var decoded ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Error != "" {
		return "", errors.New(decoded.Error)
	}
	return decoded.Message.Content, nil
}

// StreamChat streams assistant content chunks.
// It returns immediately with two channels; both will be closed when streaming ends.
func (p *OllamaProvider) StreamChat(ctx context.Context, messages []Message) (<-chan string, <-chan error) {
	chunks := make(chan string, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		if p.Client == nil {
			errs <- errors.New("ollama: http client is nil")
			return
		}

		reqBody := ollamaChatReq{
			Model:  p.Model,
			Stream: true,
			Messages: func() []ollamaMsg {
				out := make([]ollamaMsg, 0, len(messages))
				for _, m := range messages {
					out = append(out, ollamaMsg{Role: m.Role, Content: m.Content})
				}
				return out
			}(),
		}

		b, err := json.Marshal(reqBody)
		if err != nil {
			errs <- err
			return
		}

		url := fmt.Sprintf("%s/api/chat", p.BaseURL)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			errs <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// Ensure a reasonable timeout; streaming can be longer.
		if p.Client.Timeout < 30*time.Second {
			p.Client.Timeout = 0 // no global timeout; ctx controls it
		}

		resp, err := p.Client.Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errs <- fmt.Errorf("ollama: status %d", resp.StatusCode)
			return
		}

		sc := bufio.NewScanner(resp.Body)
		// Increase scanner buffer for long JSON lines.
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 2*1024*1024)

		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}

			var decoded ollamaStreamResp
			if err := json.Unmarshal(line, &decoded); err != nil {
				errs <- err
				return
			}
			if decoded.Error != "" {
				errs <- errors.New(decoded.Error)
				return
			}

			if decoded.Message.Content != "" {
				chunks <- decoded.Message.Content
			}

			if decoded.Done {
				return
			}
		}

		if err := sc.Err(); err != nil {
			errs <- err
			return
		}
	}()

	return chunks, errs
}
