package llm

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Format   any            `json:"format,omitempty"` // keep flexible
	Options  map[string]any `json:"options,omitempty"`
}

type Client interface {
	Provider() string
	Model() string
	Ping(ctx context.Context) error
	Chat(ctx context.Context, req ChatRequest) (string, error)
}
