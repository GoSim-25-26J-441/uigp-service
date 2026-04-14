package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port string

	APIKey       string
	MaxBodyBytes int64

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	MaxHistoryItems int
	MaxHistoryChars int
	LLMConcurrency  int

	LLMProvider   string
	OllamaURL     string
	OllamaModel   string
	OllamaTimeout time.Duration

	DomainStrict   bool
	DomainKeywords []string

	OllamaNumCtx      int
	OllamaNumPredict  int
	OllamaTemperature float64

	ChatModeDefault string

	ChatInstantNumCtx      int
	ChatInstantNumPredict  int
	ChatInstantTemperature float64

	ChatThinkingNumCtx      int
	ChatThinkingNumPredict  int
	ChatThinkingTemperature float64
}

func Load() Config {
	cfg := Config{
		Port: getenv("PORT", "8081"),

		APIKey:       os.Getenv("UIGP_API_KEY"),
		MaxBodyBytes: getenvInt64("MAX_BODY_BYTES", 8<<20),

		ReadTimeout: getenvDuration("READ_TIMEOUT", 10*time.Second),
		// Must exceed OLLAMA_TIMEOUT so the handler can finish after the LLM returns.
		WriteTimeout: getenvDuration("WRITE_TIMEOUT", 240*time.Second),
		IdleTimeout:  getenvDuration("IDLE_TIMEOUT", 120*time.Second),

		MaxHistoryItems: getenvInt("MAX_HISTORY_ITEMS", 20),
		MaxHistoryChars: getenvInt("MAX_HISTORY_CHARS", 12000),
		LLMConcurrency:  getenvInt("LLM_CONCURRENCY", 2),

		LLMProvider: getenv("LLM_PROVIDER", "ollama"),
		OllamaURL:   getenv("OLLAMA_URL", "http://localhost:11434"),
		OllamaModel: getenv("OLLAMA_MODEL", "llama3:instruct"),
		// Local inference (thinking mode, long prompts) often exceeds 60s.
		OllamaTimeout: getenvDuration("OLLAMA_TIMEOUT", 180*time.Second),

		// Default to strict domain filtering so general chit-chat/out-of-scope questions
		DomainStrict: getenvBool("DOMAIN_STRICT", true),

		DomainKeywords: getenvCSV("DOMAIN_KEYWORDS"),

		OllamaNumCtx:      getenvInt("OLLAMA_NUM_CTX", 2048),
		OllamaNumPredict:  getenvInt("OLLAMA_NUM_PREDICT", 512),
		OllamaTemperature: getenvFloat("OLLAMA_TEMPERATURE", 0.2),

		ChatModeDefault: getenv("CHAT_MODE_DEFAULT", "auto"),

		ChatInstantNumCtx:      getenvInt("CHAT_INSTANT_NUM_CTX", 1024),
		ChatInstantNumPredict:  getenvInt("CHAT_INSTANT_NUM_PREDICT", 128),
		ChatInstantTemperature: getenvFloat("CHAT_INSTANT_TEMPERATURE", 0.2),

		ChatThinkingNumCtx:      getenvInt("CHAT_THINKING_NUM_CTX", 2048),
		ChatThinkingNumPredict:  getenvInt("CHAT_THINKING_NUM_PREDICT", 384),
		ChatThinkingTemperature: getenvFloat("CHAT_THINKING_TEMPERATURE", 0.2),
	}

	if cfg.ChatInstantNumCtx == 0 {
		cfg.ChatInstantNumCtx = cfg.OllamaNumCtx
	}
	if cfg.ChatInstantNumPredict == 0 {
		cfg.ChatInstantNumPredict = cfg.OllamaNumPredict
	}
	if cfg.ChatInstantTemperature == 0 {
		cfg.ChatInstantTemperature = cfg.OllamaTemperature
	}

	return cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
func getenvInt64(k string, def int64) int64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}
func getenvFloat(k string, def float64) float64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
func getenvDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getenvCSV(k string) []string {
	v := os.Getenv(k)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getenvBool(k string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}
