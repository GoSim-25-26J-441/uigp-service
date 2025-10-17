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
	sys := `You returned JSON that failed schema validation. 
Repair it so it satisfies the schema keys exactly:
{ services:[], apis:[], datastores:[], topics:[], dependencies:[], configs:{}, constraints:{}, deploymentHints:{}, gaps:[], conflicts:[], trace:[], metadata:{schemaVersion:string} }
- Keep all facts you already produced.
- Fix only structure/types to satisfy the schema.
- Return ONLY valid JSON.`

	req := map[string]any{
		"model":  model,
		"system": sys,
		"prompt": map[string]any{
			"spec":   bad,
			"errors": validationErr,
		},
		"format": "json",
		"options": map[string]any{
			"temperature": 0.2,
			"num_ctx":     1024,
		},
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
