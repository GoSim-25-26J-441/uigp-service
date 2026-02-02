//go:build integration
// +build integration

package tests_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

type ChatResponse struct {
	OK      bool           `json:"ok"`
	Answer  string         `json:"answer"`
	Signals map[string]any `json:"signals"`
	Meta    map[string]any `json:"meta"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func doJSON(t *testing.T, method, url string, headers map[string]string, body any, out any) int {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}

	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	c := &http.Client{Timeout: 45 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestHealthz(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")

	var out map[string]any
	status := doJSON(t, "GET", base+"/healthz", nil, nil, &out)
	if status != 200 {
		t.Fatalf("expected 200, got %d, body=%v", status, out)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out["ok"])
	}
}

func TestReadyz_WithKey(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")
	key := getenv("UIGP_API_KEY", "dev-key")

	h := map[string]string{"X-API-Key": key}

	var out map[string]any
	status := doJSON(t, "GET", base+"/readyz", h, nil, &out)
	if status != 200 {
		t.Fatalf("expected 200, got %d, body=%v", status, out)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok=true, got %v", out["ok"])
	}
}

func TestChat_RequiresKey(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")

	req := map[string]any{"message": "hello", "history": []any{}}

	var out ChatResponse
	status := doJSON(t, "POST", base+"/api/v1/chat", nil, req, &out)

	if status != 401 && status != 403 {
		t.Fatalf("expected 401/403, got %d, resp=%+v", status, out)
	}
}

func TestChat_MessageRequired(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")
	key := getenv("UIGP_API_KEY", "dev-key")

	h := map[string]string{"X-API-Key": key}
	req := map[string]any{"message": "", "history": []any{}}

	var out ChatResponse
	status := doJSON(t, "POST", base+"/api/v1/chat", h, req, &out)
	if status != 400 {
		t.Fatalf("expected 400, got %d, resp=%+v", status, out)
	}
	if out.Error == nil || out.Error.Code != "bad_request" {
		t.Fatalf("expected bad_request, got %+v", out.Error)
	}
}

func TestChat_WithHistory(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")
	key := getenv("UIGP_API_KEY", "dev-key")

	h := map[string]string{"X-API-Key": key}
	req := map[string]any{
		"message": "What should I improve?",
		"history": []any{
			map[string]any{"role": "user", "content": "We have 12 services, REST calls, Postgres, and Kafka."},
			map[string]any{"role": "assistant", "content": "Got it. Any latency or traffic targets?"},
			map[string]any{"role": "user", "content": "Peak is ~200 rps, p95 < 300ms."},
		},
	}

	var out ChatResponse
	status := doJSON(t, "POST", base+"/api/v1/chat", h, req, &out)
	if status != 200 {
		t.Fatalf("expected 200, got %d, resp=%+v", status, out)
	}
	if !out.OK || out.Answer == "" {
		t.Fatalf("expected ok=true + answer, resp=%+v", out)
	}
}

func TestChat_WithDiagramJSON(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")
	key := getenv("UIGP_API_KEY", "dev-key")

	h := map[string]string{"X-API-Key": key}
	req := map[string]any{
		"message": "Spot missing dependencies or gaps.",
		"history": []any{},
		"diagram_json": map[string]any{
			"metadata": map[string]any{"diagram_version_id": "dv_test"},
			"nodes": []any{
				map[string]any{"id": "api", "label": "api-gateway"},
				map[string]any{"id": "u", "label": "user-service"},
				map[string]any{"id": "o", "label": "order-service"},
				map[string]any{"id": "db", "label": "postgres"},
			},
			"edges": []any{
				map[string]any{"from": "api", "to": "u", "protocol": "REST"},
				map[string]any{"from": "api", "to": "o", "protocol": "REST"},
				map[string]any{"from": "u", "to": "db", "protocol": "SQL"},
			},
		},
	}

	var out ChatResponse
	status := doJSON(t, "POST", base+"/api/v1/chat", h, req, &out)
	if status != 200 {
		t.Fatalf("expected 200, got %d, resp=%+v", status, out)
	}
	if out.Meta != nil {
		if v, ok := out.Meta["context_used"]; ok && v != "diagram_json" {
			t.Fatalf("expected context_used=diagram_json, got %v", v)
		}
	}
}

func TestChat_ModeInstantThinking(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")
	key := getenv("UIGP_API_KEY", "dev-key")

	h := map[string]string{
		"X-API-Key": key,
	}

	httpClient := &http.Client{Timeout: 120 * time.Second}

	for _, mode := range []string{"instant", "thinking"} {
		req := map[string]any{
			"message": "Give 3 quick tips for p95 latency",
			"history": []any{},
			"mode":    mode,
		}

		var out ChatResponse
		status := doJSONWithClient(t, httpClient, "POST", base+"/api/v1/chat", h, req, &out)
		if status != 200 {
			t.Fatalf("mode=%s expected 200 got %d resp=%+v", mode, status, out)
		}

		if out.OK != true {
			t.Fatalf("mode=%s expected ok=true got resp=%+v", mode, out)
		}
		if out.Meta != nil {
			if got, _ := out.Meta["mode_used"].(string); got != "" && got != mode {
				t.Fatalf("mode=%s expected meta.mode_used=%s got %v", mode, mode, out.Meta["mode_used"])
			}
		}
	}
}

func doJSONWithClient(
	t *testing.T,
	c *http.Client,
	method, url string,
	headers map[string]string,
	reqBody any,
	out any,
) int {
	t.Helper()

	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			t.Fatalf("marshal req: %v", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if out != nil {
		decErr := json.NewDecoder(resp.Body).Decode(out)
		if decErr != nil && decErr != io.EOF {
			t.Fatalf("decode resp: %v", decErr)
		}
	}

	return resp.StatusCode
}

func TestChat_DomainStrictBlocksOutOfScopeIfEnabled(t *testing.T) {
	base := getenv("UIGP_BASE_URL", "http://localhost:8081")
	key := getenv("UIGP_API_KEY", "dev-key")
	h := map[string]string{"X-API-Key": key}

	req := map[string]any{
		"message": "Write me a love poem",
		"history": []any{},
	}
	var out ChatResponse
	status := doJSON(t, "POST", base+"/api/v1/chat", h, req, &out)
	if status != 200 {
		t.Fatalf("expected 200, got %d, resp=%+v", status, out)
	}

	if out.Signals != nil && out.Signals["domain_strict"] == true {
		if out.Signals["out_of_scope"] != true {
			t.Fatalf("expected out_of_scope=true, signals=%v", out.Signals)
		}
		if out.Meta == nil || out.Meta["blocked"] != true {
			t.Fatalf("expected meta.blocked=true, meta=%v", out.Meta)
		}
	}
}
