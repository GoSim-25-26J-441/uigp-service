package context

import (
	"encoding/json"
	"strings"
	"testing"
)

// Regression: services[]+dependencies[] payloads used to skip risk hints because idToType
// was only filled from nodes[]. The model then misread "disconnected" (e.g. notification-service
// with db-1→notification) and missed true orphans (service-6).
func TestBuildCompactContext_servicesDependenciesRiskHints(t *testing.T) {
	const payload = `{
  "services": [
    {"name": "web-site", "kind": "client"},
    {"name": "auth-gateway", "kind": "gateway"},
    {"name": "user-service", "kind": "service"},
    {"name": "order-service", "kind": "service"},
    {"name": "resturant-service", "kind": "service"},
    {"name": "notification-service", "kind": "service"},
    {"name": "payment-service", "kind": "service"},
    {"name": "service-6", "kind": "service"}
  ],
  "datastores": [{"name": "db-1"}],
  "topics": [],
  "dependencies": [
    {"from": "web-site", "to": "auth-gateway", "kind": "rest", "sync": true},
    {"from": "auth-gateway", "to": "resturant-service", "kind": "rest", "sync": true},
    {"from": "user-service", "to": "resturant-service", "kind": "rest", "sync": true},
    {"from": "user-service", "to": "order-service", "kind": "rest", "sync": true},
    {"from": "resturant-service", "to": "order-service", "kind": "rest", "sync": true},
    {"from": "order-service", "to": "resturant-service", "kind": "rest", "sync": true},
    {"from": "user-service", "to": "payment-service", "kind": "rest", "sync": true},
    {"from": "payment-service", "to": "resturant-service", "kind": "rest", "sync": true},
    {"from": "user-service", "to": "auth-gateway", "kind": "rest", "sync": true},
    {"from": "order-service", "to": "db-1", "kind": "rest", "sync": true},
    {"from": "resturant-service", "to": "db-1", "kind": "rest", "sync": true},
    {"from": "db-1", "to": "notification-service", "kind": "rest", "sync": true}
  ]
}`

	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatal(err)
	}

	text, used, sig := BuildCompactContext(nil, m, "", nil)
	if !strings.Contains(used, "diagram_json") {
		t.Fatalf("expected diagram_json in used, got %q", used)
	}
	if !strings.Contains(text, "Structural risk hints") {
		t.Fatalf("expected structural risk hints block:\n%s", text)
	}
	if !strings.Contains(text, "service-6") {
		t.Fatalf("expected graph-isolated service-6 in hints:\n%s", text)
	}
	if !strings.Contains(text, "db-1 -> notification-service") && !strings.Contains(text, "database outbound") {
		t.Logf("context text:\n%s", text)
		t.Fatalf("expected db outbound / notification edge in hints")
	}
	if !strings.Contains(text, "user-service -> auth-gateway") && !strings.Contains(text, "service → gateway") {
		t.Fatalf("expected service→gateway hint:\n%s", text)
	}
	if c, ok := sig["orphan_components_count"].(int); !ok || c < 1 {
		t.Fatalf("expected orphan_components_count >= 1, sig=%v", sig)
	}
	if c, ok := sig["shared_db_fanin_count"].(int); !ok || c < 1 {
		t.Fatalf("expected shared_db_fanin_count >= 1, sig=%v", sig)
	}
	if c, ok := sig["service_to_gateway_edges_count"].(int); !ok || c < 1 {
		t.Fatalf("expected service_to_gateway_edges_count >= 1, sig=%v", sig)
	}
	if !strings.Contains(text, "web-site") || !strings.Contains(text, "Entry / user-facing connectivity") {
		t.Fatalf("expected entry connectivity for client outbound:\n%s", text)
	}
	if !strings.Contains(text, "Datastores (exact names") || !strings.Contains(text, "- db-1") {
		t.Fatalf("expected Datastores section with db-1:\n%s", text)
	}
}
