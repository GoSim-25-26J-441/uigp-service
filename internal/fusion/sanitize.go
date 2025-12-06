package fusion

func Sanitize(spec map[string]any) map[string]any {
	// ensure metadata
	if _, ok := spec["metadata"]; !ok {
		spec["metadata"] = map[string]any{"schemaVersion": "0.1.0", "generator": "sanitizer"}
	}
	// default empty arrays/objects
	ensureArr(spec, "services")
	ensureArr(spec, "dependencies")
	ensureArr(spec, "datastores")
	ensureArr(spec, "topics")
	ensureArr(spec, "gaps")
	ensureArr(spec, "conflicts")
	ensureObj(spec, "configs")
	ensureObj(spec, "constraints")
	ensureObj(spec, "deploymentHints")

	// fix services[].type
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

	// normalize dependencies[].kind
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

	// drop generic apis block if useless
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
