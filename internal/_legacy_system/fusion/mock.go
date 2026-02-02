package fusion

import "github.com/MalithGihan/uigp-service/pkg/types"

func MockFromIntermediate(ig types.IntermediateGraph) map[string]any {
	spec := map[string]any{
		"services":     []map[string]any{},
		"dependencies": []map[string]any{},
		"datastores":   []map[string]any{},
		"topics":       []map[string]any{},
		"configs":      map[string]any{},
		"gaps":         []string{},
		"conflicts":    []string{},
		"trace":        []any{},
		"metadata":     map[string]any{"generator": "mock", "schemaVersion": "0.1.0"},
	}

	for _, n := range ig.Nodes {
		switch n.Type {
		case "db":
			spec["datastores"] = append(spec["datastores"].([]map[string]any),
				map[string]any{"name": n.Label, "engine": "unknown", "ownerService": nil})
		case "queue":
			spec["topics"] = append(spec["topics"].([]map[string]any),
				map[string]any{"name": n.Label, "semantics": "at-least-once"})
		default:
			spec["services"] = append(spec["services"].([]map[string]any),
				map[string]any{"name": n.Label, "type": n.Type})
		}
	}

	for _, e := range ig.Edges {
		kind := "rest"
		switch e.Protocol {
		case "gRPC":
			kind = "grpc"
		case "PUB", "SUB":
			kind = "event"
		}
		spec["dependencies"] = append(spec["dependencies"].([]map[string]any),
			map[string]any{"from": e.From, "to": e.To, "kind": kind,
				"sync": e.Protocol == "REST" || e.Protocol == "gRPC"})
	}

	return spec
}
