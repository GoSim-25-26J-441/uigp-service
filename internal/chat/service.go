package chat

import (
	stdctx "context"
	"strings"
	"time"

	archctx "github.com/MalithGihan/uigp-service/internal/context"
	"github.com/MalithGihan/uigp-service/internal/llm"
)

type LLMProfile struct {
	Temperature float64
	NumCtx      int
	NumPredict  int
}

type ServiceDeps struct {
	LLM             llm.Client
	MaxHistoryItems int
	MaxHistoryChars int
	LLMConcurrency  int

	DomainStrict   bool
	DomainKeywords []string

	BaseProfile LLMProfile

	ModeDefault     string
	InstantProfile  LLMProfile
	ThinkingProfile LLMProfile
}

type Service struct {
	llm             llm.Client
	maxHistoryItems int
	maxHistoryChars int
	sem             chan struct{}

	domainStrict   bool
	domainKeywords []string

	modeDefault     string
	baseProfile     LLMProfile
	instantProfile  LLMProfile
	thinkingProfile LLMProfile
}

func NewService(d ServiceDeps) *Service {
	c := d.LLMConcurrency
	if c <= 0 {
		c = 1
	}

	base := d.BaseProfile
	if base.Temperature == 0 {
		base.Temperature = 0.2
	}
	if base.NumCtx <= 0 {
		base.NumCtx = 1024
	}
	if base.NumPredict <= 0 {
		base.NumPredict = 256
	}

	instant := d.InstantProfile
	if instant.Temperature == 0 {
		instant.Temperature = base.Temperature
	}
	if instant.NumCtx <= 0 {
		instant.NumCtx = base.NumCtx
	}
	if instant.NumPredict <= 0 {
		instant.NumPredict = base.NumPredict
	}

	thinking := d.ThinkingProfile
	if thinking.Temperature == 0 {
		thinking.Temperature = base.Temperature
	}
	if thinking.NumCtx <= 0 {
		thinking.NumCtx = base.NumCtx
	}
	if thinking.NumPredict <= 0 {
		thinking.NumPredict = base.NumPredict
	}

	md := strings.ToLower(strings.TrimSpace(d.ModeDefault))
	if md == "" {
		md = "auto"
	}

	return &Service{
		llm:             d.LLM,
		maxHistoryItems: d.MaxHistoryItems,
		maxHistoryChars: d.MaxHistoryChars,
		sem:             make(chan struct{}, c),

		domainStrict:   d.DomainStrict,
		domainKeywords: d.DomainKeywords,

		modeDefault:     md,
		baseProfile:     base,
		instantProfile:  instant,
		thinkingProfile: thinking,
	}
}

func (s *Service) pickProfile(req ChatRequest, ctxUsed string, historyUsed int) (LLMProfile, string, bool) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = s.modeDefault
	}

	switch mode {
	case "instant":
		return s.instantProfile, "instant", false
	case "thinking":
		return s.thinkingProfile, "thinking", false
	case "auto":
		detail := strings.ToLower(strings.TrimSpace(req.Detail))
		if ctxUsed != "none" || detail == "high" || detail == "detailed" || historyUsed >= 6 || len(req.Message) > 180 {
			return s.thinkingProfile, "thinking", false
		}
		return s.instantProfile, "instant", false
	default:
		return s.baseProfile, "base", true
	}
}

func (s *Service) Handle(ctx stdctx.Context, req ChatRequest) ChatResponse {
	start := time.Now()

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return ChatResponse{
			OK:     false,
			Source: SourceInfo{Provider: s.llm.Provider(), Model: s.llm.Model()},
			Refs:   []any{}, Signals: map[string]any{},
			Error: &struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{Code: "bad_request", Message: "message is required"},
		}
	}

	ctxText, ctxUsed, ctxSignals := archctx.BuildCompactContext(req.SpecSummary, req.DiagramJSON, req.Attachments)

	ctxSignals = mergeSignals(ctxSignals, map[string]any{
		"domain_strict": s.domainStrict,
	})

	if s.domainStrict && isOutOfScope(req, msg, s.domainKeywords) {
		refusal := "I can only help with microservices architecture and performance. Ask about services, dependencies, APIs, data stores, scaling, latency, throughput, deployments, or share a diagram/spec."
		return ChatResponse{
			OK:     true,
			Answer: refusal,
			Source: SourceInfo{Provider: s.llm.Provider(), Model: s.llm.Model()},
			Refs:   []any{},
			Signals: mergeSignals(ctxSignals, map[string]any{
				"out_of_scope": true,
			}),
			Meta: map[string]any{
				"blocked":      true,
				"latency_ms":   time.Since(start).Milliseconds(),
				"context_used": ctxUsed,
			},
		}
	}

	h := normalizeHistory(req.History)
	h = budgetHistory(h, s.maxHistoryItems, s.maxHistoryChars)

	system := systemPrompt()

	llmMsgs := []llm.Message{
		{Role: "system", Content: system},
	}
	if ctxText != "" {
		llmMsgs = append(llmMsgs, llm.Message{
			Role:    "system",
			Content: "Architecture context (treat as factual input):\n" + ctxText,
		})
	}
	for _, it := range h {
		llmMsgs = append(llmMsgs, llm.Message{Role: it.Role, Content: it.Content})
	}
	llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: msg})

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return ChatResponse{
			OK:     false,
			Source: SourceInfo{Provider: s.llm.Provider(), Model: s.llm.Model()},
			Refs:   []any{}, Signals: map[string]any{},
			Error: &struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{Code: "timeout", Message: "request cancelled"},
		}
	}

	profile, modeUsed, modeInvalid := s.pickProfile(req, ctxUsed, len(h))

	opts := map[string]any{
		"temperature": profile.Temperature,
		"num_ctx":     profile.NumCtx,
		"num_predict": profile.NumPredict,
	}

	answer, err := s.llm.Chat(ctx, llm.ChatRequest{
		Model:    s.llm.Model(),
		Messages: llmMsgs,
		Stream:   false,
		Options:  opts,
	})
	if err != nil {
		return ChatResponse{
			OK:      false,
			Source:  SourceInfo{Provider: s.llm.Provider(), Model: s.llm.Model()},
			Refs:    []any{},
			Signals: mergeSignals(ctxSignals, map[string]any{"llm_error": err.Error()}),
			Meta: map[string]any{
				"latency_ms":   time.Since(start).Milliseconds(),
				"context_used": ctxUsed,
			},
			Error: &struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{Code: "llm_failed", Message: "LLM request failed"},
		}
	}

	return ChatResponse{
		OK:      true,
		Answer:  answer,
		Source:  SourceInfo{Provider: s.llm.Provider(), Model: s.llm.Model()},
		Refs:    []any{},
		Signals: ctxSignals,
		Meta: map[string]any{
			"latency_ms":   time.Since(start).Milliseconds(),
			"context_used": ctxUsed,
			"history_used": len(h),
			"mode_used":    modeUsed,
			"mode_invalid": modeInvalid,
		},
	}
}

func systemPrompt() string {
	return `You are UIGP, a stateless microservices assistant.
Answer the user's question clearly and directly.
If crucial info is missing, ask only the minimum necessary clarifying question(s).
Do not hallucinate architecture facts. Use the provided context if present.
Keep the answer practical and implementation-oriented when relevant.
Return concise answers by default (<= 120 words).
Only expand if the user asks for details.`
}

func normalizeHistory(in []HistoryItem) []HistoryItem {
	out := make([]HistoryItem, 0, len(in))
	for _, it := range in {
		role := strings.ToLower(strings.TrimSpace(it.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		c := strings.TrimSpace(it.Content)
		if c == "" {
			continue
		}
		out = append(out, HistoryItem{Role: role, Content: c})
	}
	return out
}

func budgetHistory(in []HistoryItem, maxItems, maxChars int) []HistoryItem {
	if maxItems <= 0 && maxChars <= 0 {
		return in
	}
	// take last maxItems
	if maxItems > 0 && len(in) > maxItems {
		in = in[len(in)-maxItems:]
	}
	if maxChars <= 0 {
		return in
	}
	total := 0
	for _, it := range in {
		total += len(it.Content)
	}
	for total > maxChars && len(in) > 0 {
		total -= len(in[0].Content)
		in = in[1:]
	}
	return in
}

func mergeSignals(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func isOutOfScope(req ChatRequest, msg string, keywords []string) bool {
	if len(req.SpecSummary) > 0 || len(req.DiagramJSON) > 0 || len(req.Attachments) > 0 {
		return false
	}
	m := strings.ToLower(msg)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(m, strings.ToLower(strings.TrimSpace(kw))) {
			return false
		}
	}
	return true
}
