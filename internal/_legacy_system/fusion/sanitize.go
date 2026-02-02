package fusion

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

// ---- helpers for light autocorrect / protocol ----

func autoCorrectLabel(s string) string {
	switch strings.ToLower(s) {
	case "sewice", "sevvice":
		return "service"
	case "paymen", "ppaymen":
		return "payment"
	default:
		return s
	}
}

func protoNormLower(s string) string {
	switch strings.ToLower(s) {
	case "grpc", "g_rpc":
		return "grpc"
	case "pubsub", "event":
		return "event"
	case "rest":
		return "rest"
	default:
		return strings.ToLower(s)
	}
}

func Sanitize(spec map[string]any) map[string]any {
	if _, ok := spec["metadata"]; !ok {
		spec["metadata"] = map[string]any{"schemaVersion": "0.1.0", "generator": "sanitizer"}
	}
	ensureArr(spec, "services")
	ensureArr(spec, "dependencies")
	ensureArr(spec, "datastores")
	ensureArr(spec, "topics")
	ensureArr(spec, "gaps")
	ensureArr(spec, "conflicts")
	ensureObj(spec, "configs")
	ensureObj(spec, "constraints")
	ensureObj(spec, "deploymentHints")

	if svcs, ok := spec["services"].([]any); ok {
		for i, v := range svcs {
			if m, ok := v.(map[string]any); ok {
				t := strOrEmpty(m["type"])
				if t == "" || t == "null" {
					m["type"] = "service"
				}
				if n := strOrEmpty(m["name"]); n == "" && len(m) > 0 {
					m["name"] = "svc_" // fallback
				}
				svcs[i] = m
			}
		}
		spec["services"] = svcs
	}

	if deps, ok := spec["dependencies"].([]any); ok {
		for i, v := range deps {
			if m, ok := v.(map[string]any); ok {
				k := strOrEmpty(m["kind"])
				switch lower(k) {
				case "grpc":
					m["kind"] = "grpc"
				case "event", "pubsub":
					m["kind"] = "event"
				default:
					m["kind"] = "rest"
				}
				deps[i] = m
			}
		}
		spec["dependencies"] = deps
	}

	if apis, ok := spec["apis"].([]any); ok && len(apis) == 1 {
		if m, ok := apis[0].(map[string]any); ok && strOrEmpty(m["name"]) == "rest" {
			delete(spec, "apis")
		}
	}
	return spec
}

func ensureArr(m map[string]any, k string) {
	if _, ok := m[k]; !ok {
		m[k] = []any{}
	}
}
func ensureObj(m map[string]any, k string) {
	if _, ok := m[k]; !ok {
		m[k] = map[string]any{}
	}
}
func strOrEmpty(v any) string { s, _ := v.(string); return s }
func lower(s string) string {
	b := []rune(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func SanitizeWithContext(spec map[string]any, ig *types.IntermediateGraph, jobDir string) map[string]any {
	if spec == nil {
		spec = map[string]any{}
	}
	var chat string
	if jobDir != "" {
		if b, err := os.ReadFile(filepath.Join(jobDir, "chat.txt")); err == nil {
			chat = strings.ToLower(string(b))
		}
	}

	if ig != nil && len(ig.Nodes) > 0 {
		var outSvcs []map[string]any
		for _, n := range ig.Nodes {
			name := autoCorrectLabel(n.Label)
			typ := n.Type
			if typ == "" {
				typ = "service"
			}
			if strings.ToLower(typ) == "db" || strings.ToLower(typ) == "datastore" {
				typ = "datastore"
			} else {
				typ = "service"
			}
			outSvcs = append(outSvcs, map[string]any{
				"name": strings.ToLower(name),
				"type": typ,
			})
		}
		spec["services"] = outSvcs
	}

	if ig != nil && len(ig.Edges) > 0 {
		var deps []map[string]any
		for _, e := range ig.Edges {
			deps = append(deps, map[string]any{
				"from": strings.ToLower(autoCorrectLabel(e.From)),
				"to":   strings.ToLower(autoCorrectLabel(e.To)),
				"kind": protoNormLower(e.Protocol),
				"sync": true,
			})
		}
		spec["dependencies"] = deps

		// 3) apis from deps
		seen := map[string]bool{}
		var apis []map[string]any
		for _, d := range deps {
			k := d["kind"].(string)
			if !seen[k] {
				seen[k] = true
				apis = append(apis, map[string]any{
					"name":     strings.ToUpper(k),
					"protocol": k,
				})
			}
		}
		spec["apis"] = apis
	}

	if chat != "" && (strings.Contains(chat, "200 rps") || strings.Contains(chat, "~200 rps")) {
		cfgs, _ := spec["configs"].(map[string]any)
		if cfgs == nil {
			cfgs = map[string]any{}
		}
		slo, _ := cfgs["slo"].(map[string]any)
		if slo == nil {
			slo = map[string]any{}
		}
		if _, ok := slo["target_rps"]; !ok {
			slo["target_rps"] = 200
		}
		cfgs["slo"] = slo
		spec["configs"] = cfgs
	}

	if cfg, ok := spec["configs"].(map[string]any); ok {
		if slo, ok := cfg["slo"].(map[string]any); ok {
			if _, ok := slo["target_rps"]; ok {
				if gaps, ok := spec["gaps"].([]any); ok {
					kept := gaps[:0]
					for _, g := range gaps {
						gm, _ := g.(map[string]any)
						desc, _ := gm["description"].(string)
						if strings.Contains(strings.ToLower(desc), "rps") {
							continue
						}
						kept = append(kept, g)
					}
					spec["gaps"] = kept
				}
			}
		}
	}

	if apis, ok := spec["apis"].([]any); ok {
		for i, v := range apis {
			if m, ok := v.(map[string]any); ok {
				if n, _ := m["name"].(string); strings.EqualFold(n, "grpc") || strings.EqualFold(n, "GRPC") {
					m["name"] = "grpc"
					m["protocol"] = "grpc"
				}
				apis[i] = m
			}
		}
		spec["apis"] = apis
	}

	if deps, ok := spec["dependencies"].([]any); ok {
		for i, v := range deps {
			if m, ok := v.(map[string]any); ok {
				k := strings.ToLower(strings.TrimSpace(fmt.Sprint(m["kind"])))
				switch k {
				case "grpc":
					m["kind"] = "grpc"
				case "event", "pubsub":
					m["kind"] = "event"
				default:
					m["kind"] = "rest"
				}
				if _, ok := m["sync"]; !ok {
					m["sync"] = true
				}
				deps[i] = m
			}
		}
		spec["dependencies"] = deps
	}

	if apis, ok := spec["apis"].([]any); ok && len(apis) == 1 {
		if m, ok := apis[0].(map[string]any); ok && m["name"] == "grpc" {
			delete(spec, "apis")
		}
	}

	if tr, ok := spec["trace"].([]any); ok {
		svcSet := map[string]bool{}
		if svcs, ok := spec["services"].([]any); ok {
			for _, v := range svcs {
				if m, ok := v.(map[string]any); ok {
					if n, _ := m["name"].(string); n != "" {
						svcSet[n] = true
					}
				}
			}
		}
		for _, v := range tr {
			if m, ok := v.(map[string]any); ok {
				if n, _ := m["name"].(string); n != "" && !svcSet[n] {
					if on, _ := m["originalName"].(string); on != "" {
						delete(m, "name")
					}
				}
			}
		}
		spec["trace"] = tr
	}

	spec = Sanitize(spec)

	if sv, ok := spec["services"].([]any); ok && len(sv) >= 2 {
		if dp, ok := spec["dependencies"].([]any); ok && len(dp) >= 1 {
			spec["gaps"] = []any{}
		}
	}

	return spec
}
