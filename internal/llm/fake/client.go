package fake

import (
	"context"
	"strings"

	"github.com/MalithGihan/uigp-service/internal/llm"
)

type Client struct {
	model    string
	provider string
}

func New() *Client {
	return &Client{model: "fake", provider: "fake"}
}

func (c *Client) Provider() string               { return c.provider }
func (c *Client) Model() string                  { return c.model }
func (c *Client) Ping(ctx context.Context) error { return nil }

func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (string, error) {
	last := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(req.Messages[i].Content) != "" {
			last = req.Messages[i].Content
			break
		}
	}
	if last == "" {
		return "fake: empty", nil
	}
	return "fake: " + last, nil
}
