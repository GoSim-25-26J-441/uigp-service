package fusion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

type spec = map[string]any

func FuseWithOllama(ctx context.Context, ollamaURL, model string, ig types.IntermediateGraph, chat string) (spec, error) {

	if model == "" {
		model = "llama3:instruct"
	}

	sys := `You are a microservice architecture fusion engine.
Return ONLY valid JSON matching this schema keys:
{ services:[], apis:[], datastores:[], topics:[], dependencies:[], configs:{}, constraints:{}, deploymentHints:{}, gaps:[], conflicts:[], trace:[] }
Rules:
- Do NOT invent info. If missing/uncertain, add an item in "gaps".
- Prefer diagram facts (nodes/edges/protocols) over chat when conflicting; record conflicts.
- Normalize service names to kebab-case; keep original in trace.`

	features := lite(ig, chat)

	reqBody := map[string]any{
		"model":  model,
		"system": sys,
		"prompt": mustJSON(features),
		"format": "json",
		"stream": false,
		"options": map[string]any{
			"temperature": 0.2,
			"num_ctx":     1024,
			"num_predict": 512,
		},
	}
	b, _ := json.Marshal(reqBody)

	httpClient := &http.Client{Timeout: 90 * time.Second}
	u := ollamaURL
	if u == "" {
		u = "http://localhost:11434"
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", u+"/api/generate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
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

	var out spec
	if err := json.Unmarshal([]byte(raw.Response), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func LoadChat(jobDir string) string {
	b, _ := os.ReadFile(jobDir + "/chat.txt")
	return string(b)
}

func lite(ig types.IntermediateGraph, chat string) map[string]any {
	labels := make([]string, 0, len(ig.Nodes))
	for _, n := range ig.Nodes {
		labels = append(labels, n.Label)
	}
	edges := make([][3]string, 0, len(ig.Edges))
	for _, e := range ig.Edges {
		edges = append(edges, [3]string{e.From, e.To, e.Protocol})
	}
	if len(chat) > 400 {
		chat = chat[:400]
	} // trim long chat
	return map[string]any{"services": labels, "links": edges, "chat": chat}
}
