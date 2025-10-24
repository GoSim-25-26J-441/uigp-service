package fusion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func RepairWithOllama(ctx context.Context, ollamaURL, model string, bad map[string]any, validationErr string) (map[string]any, error) {
	if model == "" {
		model = "llama3:instruct"
	}
	req := map[string]any{
		"model": model,
		"system": `You returned JSON that failed schema validation.
Repair it to satisfy exactly these keys:
{ services:[], apis:[], datastores:[], topics:[], dependencies:[], configs:{}, constraints:{}, deploymentHints:{}, gaps:[], conflicts:[], trace:[], metadata:{schemaVersion:string} }
Keep facts; fix only structure/types. Return ONLY valid JSON.`,
		"prompt":  map[string]any{"spec": bad, "errors": validationErr},
		"format":  "json",
		"stream":  false,
		"options": map[string]any{"temperature": 0.2, "num_ctx": 1024, "num_predict": 512},
	}
	b, _ := json.Marshal(req)
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	httpClient := &http.Client{Timeout: 90 * time.Second}

	r, err := http.NewRequestWithContext(ctx, "POST", ollamaURL+"/api/generate", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(raw.Response), &out); err != nil {
		return nil, err
	}
	return out, nil
}
