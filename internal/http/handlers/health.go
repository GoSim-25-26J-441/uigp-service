package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/MalithGihan/uigp-service/internal/llm"
)

type Health struct {
	llm llm.Client
}

func NewHealth(c llm.Client) *Health { return &Health{llm: c} }

func (h *Health) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"service": "uigp-service",
	})
}

func (h *Health) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err := h.llm.Ping(ctx)
	out := map[string]any{
		"ok":       err == nil,
		"provider": h.llm.Provider(),
		"model":    h.llm.Model(),
	}
	if err != nil {
		out["error"] = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
