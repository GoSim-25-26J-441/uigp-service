package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MalithGihan/uigp-service/internal/llm"
)

type Config struct {
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
}

type Client struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewClient(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	to := cfg.Timeout
	if to <= 0 {
		to = 180 * time.Second
	}
	model := cfg.DefaultModel
	if model == "" {
		model = "llama3:instruct"
	}
	return &Client{
		baseURL: base,
		model:   model,
		http:    &http.Client{Timeout: to},
	}
}

func (c *Client) Provider() string { return "ollama" }
func (c *Client) Model() string    { return c.model }

func (c *Client) Ping(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("ollama unhealthy: status=%d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (string, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	payload := map[string]any{
		"model":    model,
		"messages": req.Messages,
		"stream":   false,
	}
	if req.Options != nil {
		payload["options"] = req.Options
	}
	if req.Format != nil {
		payload["format"] = req.Format
	}

	b, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama chat error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var raw struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done bool `json:"done"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}

	return raw.Message.Content, nil
}
