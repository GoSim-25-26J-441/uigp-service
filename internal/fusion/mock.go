package fusion

import "github.com/MalithGihan/uigp-service/pkg/types"

type ArchitectureSpec struct {
	Services     []map[string]any `json:"services"`
	Dependencies []map[string]any `json:"dependencies"`
	Datastores   []map[string]any `json:"datastores"`
	Topics       []map[string]any `json:"topics"`
	Configs      map[string]any   `json:"configs"`
	Gaps         []string         `json:"gaps"`
	Conflicts    []string         `json:"conflicts"`
	Trace        []any            `json:"trace"`
	Metadata     map[string]any   `json:"metadata"`
}

// Very simple mapping: nodes -> services/db/topics; edges -> dependencies
func MockFromIntermediate(ig types.IntermediateGraph) ArchitectureSpec {
	spec := ArchitectureSpec{
		Configs:  map[string]any{},
		Metadata: map[string]any{"generator": "mock", "schemaVersion": "0.1.0"},
	}
	for _, n := range ig.Nodes {
		switch n.Type {
		case "db":
			spec.Datastores = append(spec.Datastores, map[string]any{
				"name": n.Label, "engine": "unknown", "ownerService": nil,
			})
		case "queue":
			spec.Topics = append(spec.Topics, map[string]any{
				"name": n.Label, "semantics": "at-least-once",
			})
		default: // service/gateway/ext
			spec.Services = append(spec.Services, map[string]any{
				"name": n.Label, "type": n.Type,
			})
		}
	}
	for _, e := range ig.Edges {
		spec.Dependencies = append(spec.Dependencies, map[string]any{
			"from": e.From, "to": e.To, "kind": protoToKind(e.Protocol),
			"sync": e.Protocol == "REST" || e.Protocol == "gRPC",
		})
	}
	return spec
}

func protoToKind(p string) string {
	switch p {
	case "gRPC":
		return "grpc"
	case "REST":
		return "rest"
	case "PUB", "SUB":
		return "event"
	default:
		return "rest"
	}
}
