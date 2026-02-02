package chat

import "github.com/MalithGihan/uigp-service/pkg/types"

type HistoryItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	SpecSummary map[string]any     `json:"spec_summary"`
	DiagramJSON map[string]any     `json:"diagram_json"`
	Attachments []types.Attachment `json:"attachments"`
	History     []HistoryItem      `json:"history"`
	Message     string             `json:"message"`
	Mode        string             `json:"mode,omitempty"`
	Detail      string             `json:"detail,omitempty"`
}

type SourceInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ChatResponse struct {
	OK      bool           `json:"ok"`
	Answer  string         `json:"answer,omitempty"`
	Source  SourceInfo     `json:"source"`
	Refs    []any          `json:"refs"`
	Signals map[string]any `json:"signals"`
	Meta    map[string]any `json:"meta,omitempty"`

	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
