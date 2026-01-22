package config

import (
	"os"
	"strconv"
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

	LLMProvider   string // "ollama" now, later "openai", etc.
	OllamaURL     string
	OllamaModel   string
	OllamaTimeout time.Duration

	DomainStrict   bool
	DomainKeywords []string

	OllamaNumCtx      int
	OllamaNumPredict  int
	OllamaTemperature float64
}

func Load() Config {
	return Config{
		Port: getenv("PORT", "8081"),

		APIKey:       os.Getenv("UIGP_API_KEY"),
		MaxBodyBytes: getenvInt64("MAX_BODY_BYTES", 8<<20), // 8MB

		ReadTimeout:  getenvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout: getenvDuration("WRITE_TIMEOUT", 120*time.Second),
		IdleTimeout:  getenvDuration("IDLE_TIMEOUT", 60*time.Second),

		MaxHistoryItems: getenvInt("MAX_HISTORY_ITEMS", 20),
		MaxHistoryChars: getenvInt("MAX_HISTORY_CHARS", 12000),
		LLMConcurrency:  getenvInt("LLM_CONCURRENCY", 2),

		LLMProvider:   getenv("LLM_PROVIDER", "ollama"),
		OllamaURL:     getenv("OLLAMA_URL", "http://localhost:11434"),
		OllamaModel:   getenv("OLLAMA_MODEL", "llama3:instruct"),
		OllamaTimeout: getenvDuration("OLLAMA_TIMEOUT", 60*time.Second),

		OllamaNumCtx:      getenvInt("OLLAMA_NUM_CTX", 2048),
		OllamaNumPredict:  getenvInt("OLLAMA_NUM_PREDICT", 512),
		OllamaTemperature: getenvFloat("OLLAMA_TEMPERATURE", 0.2),
	}
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
