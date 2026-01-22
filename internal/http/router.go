package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/MalithGihan/uigp-service/internal/chat"
	"github.com/MalithGihan/uigp-service/internal/config"
	"github.com/MalithGihan/uigp-service/internal/http/handlers"
	"github.com/MalithGihan/uigp-service/internal/http/middleware"
	"github.com/MalithGihan/uigp-service/internal/llm"
)

func NewRouter(cfg config.Config, llmClient llm.Client, chatSvc *chat.Service) http.Handler {
	r := chi.NewRouter()

	// Baseline middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.BodyLimit(cfg.MaxBodyBytes))

	// Health
	hh := handlers.NewHealth(llmClient)
	r.Get("/healthz", hh.Healthz)
	r.Get("/readyz", hh.Readyz)

	// Versioned API
	ch := handlers.NewChat(chatSvc)
	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(middleware.APIKey(cfg.APIKey))
		v1.Post("/chat", ch.Chat)
	})

	r.Post("/chat", ch.Chat)

	return r
}
