package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"

	"github.com/MalithGihan/uigp-service/internal/chat"
	"github.com/MalithGihan/uigp-service/internal/config"
	httpapi "github.com/MalithGihan/uigp-service/internal/http"
	llmfactory "github.com/MalithGihan/uigp-service/internal/llm/factory"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	llmClient, err := llmfactory.NewClientFromConfig(cfg)

	if err != nil {
		log.Fatalf("llm init error: %v", err)
	}

	chatSvc := chat.NewService(chat.ServiceDeps{
		LLM:             llmClient,
		MaxHistoryItems: cfg.MaxHistoryItems,
		MaxHistoryChars: cfg.MaxHistoryChars,
		LLMConcurrency:  cfg.LLMConcurrency,

		DomainStrict:   cfg.DomainStrict,
		DomainKeywords: cfg.DomainKeywords,

		BaseProfile: chat.LLMProfile{
			Temperature: cfg.OllamaTemperature,
			NumCtx:      cfg.OllamaNumCtx,
			NumPredict:  cfg.OllamaNumPredict,
		},

		ModeDefault: cfg.ChatModeDefault,

		InstantProfile: chat.LLMProfile{
			Temperature: cfg.ChatInstantTemperature,
			NumCtx:      cfg.ChatInstantNumCtx,
			NumPredict:  cfg.ChatInstantNumPredict,
		},

		ThinkingProfile: chat.LLMProfile{
			Temperature: cfg.ChatThinkingTemperature,
			NumCtx:      cfg.ChatThinkingNumCtx,
			NumPredict:  cfg.ChatThinkingNumPredict,
		},
	})

	handler := httpapi.NewRouter(cfg, llmClient, chatSvc)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	log.Printf("uigp-service listening on :%s (provider=%s model=%s)", cfg.Port, llmClient.Provider(), llmClient.Model())
	log.Fatal(srv.ListenAndServe())
}
