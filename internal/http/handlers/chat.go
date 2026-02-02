package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/MalithGihan/uigp-service/internal/chat"
)

type Chat struct {
	svc *chat.Service
}

func NewChat(svc *chat.Service) *Chat { return &Chat{svc: svc} }

func (h *Chat) Chat(w http.ResponseWriter, r *http.Request) {
	var req chat.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	resp := h.svc.Handle(r.Context(), req)

	w.Header().Set("Content-Type", "application/json")
	if !resp.OK {
		// still return JSON body
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = json.NewEncoder(w).Encode(resp)
}
