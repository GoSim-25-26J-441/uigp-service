package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

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
		status := mapErrorToStatus(resp)
		normalizeErrorMessage(&resp)
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func mapErrorToStatus(resp chat.ChatResponse) int {
	if resp.Error == nil {
		return http.StatusBadRequest
	}
	switch resp.Error.Code {
	case "bad_request":
		return http.StatusBadRequest
	case "timeout":
		return http.StatusGatewayTimeout
	case "llm_failed":
		if isUpstreamTimeout(resp.Signals) {
			return http.StatusGatewayTimeout
		}
		return http.StatusBadGateway
	default:
		return http.StatusBadRequest
	}
}

func normalizeErrorMessage(resp *chat.ChatResponse) {
	if resp == nil || resp.Error == nil {
		return
	}
	switch resp.Error.Code {
	case "timeout":
		resp.Error.Message = "Request timed out while processing. Please retry."
	case "llm_failed":
		if isUpstreamTimeout(resp.Signals) {
			resp.Error.Message = "Upstream LLM timed out. Please retry or reduce request complexity."
		} else if strings.TrimSpace(resp.Error.Message) == "" || resp.Error.Message == "LLM request failed" {
			resp.Error.Message = "Upstream LLM request failed."
		}
	}
}

func isUpstreamTimeout(signals map[string]any) bool {
	if len(signals) == 0 {
		return false
	}
	v, ok := signals["llm_error"]
	if !ok || v == nil {
		return false
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	errText := strings.ToLower(strings.TrimSpace(s))
	if errText == "" {
		return false
	}
	return strings.Contains(errText, "context deadline exceeded") ||
		strings.Contains(errText, "client.timeout exceeded") ||
		strings.Contains(errText, "timeout")
}
