package factory

import (
	"fmt"

	"github.com/MalithGihan/uigp-service/internal/config"
	"github.com/MalithGihan/uigp-service/internal/llm"
	"github.com/MalithGihan/uigp-service/internal/llm/ollama"
)

func NewClientFromConfig(cfg config.Config) (llm.Client, error) {
	switch cfg.LLMProvider {
	case "", "ollama":
		return ollama.NewClient(ollama.Config{
			BaseURL:      cfg.OllamaURL,
			DefaultModel: cfg.OllamaModel,
			Timeout:      cfg.OllamaTimeout,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER: %s", cfg.LLMProvider)
	}
}
