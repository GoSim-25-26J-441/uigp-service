package context

import (
	"fmt"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

func BuildCompactContext(
	specSummary map[string]any,
	diagramJSON map[string]any,
	atts []types.Attachment,
) (text string, used string, signals map[string]any) {

	signals = map[string]any{}

	if specSummary != nil && len(specSummary) > 0 {
		return compactFromMap("spec_summary", specSummary), "spec_summary", signals
	}
	if diagramJSON != nil && len(diagramJSON) > 0 {
		t, sig := compactFromDiagram(diagramJSON)
		for k, v := range sig {
			signals[k] = v
		}
		return t, "diagram_json", signals
	}
	if len(atts) > 0 {
		var lines []string
		for _, a := range atts {
			lines = append(lines, fmt.Sprintf("- %s (%s)", a.Name, a.ContentType))
		}
		signals["attachments_detected"] = len(atts)
		return "Attachments provided:\n" + strings.Join(lines, "\n"), "attachments", signals
	}

	return "", "none", signals
}

func compactFromMap(label string, m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return fmt.Sprintf("%s keys: %v\n(Provide more structure in spec_summary for best results.)", label, keys)
}

func compactFromDiagram(m map[string]any) (string, map[string]any) {
	sig := map[string]any{}
	var services []string
	var deps []string

	if sv, ok := m["services"].([]any); ok {
		for _, v := range sv {
			switch t := v.(type) {
			case string:
				services = append(services, t)
			case map[string]any:
				if n, ok := t["name"].(string); ok {
					services = append(services, n)
				}
			}
		}
	}
	if dv, ok := m["dependencies"].([]any); ok {
		for _, v := range dv {
			if dm, ok := v.(map[string]any); ok {
				f := fmt.Sprint(dm["from"])
				to := fmt.Sprint(dm["to"])
				kind := fmt.Sprint(dm["kind"])
				deps = append(deps, fmt.Sprintf("%s -> %s (%s)", f, to, kind))
			}
		}
	}

	if len(services) == 0 {
		if nv, ok := m["nodes"].([]any); ok {
			for _, v := range nv {
				if nm, ok := v.(map[string]any); ok {
					if lbl, ok := nm["label"].(string); ok {
						services = append(services, lbl)
					}
				}
			}
		}
	}
	if len(deps) == 0 {
		if ev, ok := m["edges"].([]any); ok {
			for _, v := range ev {
				if em, ok := v.(map[string]any); ok {
					from := fmt.Sprint(em["from"])
					to := fmt.Sprint(em["to"])
					proto := fmt.Sprint(em["protocol"])
					deps = append(deps, fmt.Sprintf("%s -> %s (%s)", from, to, proto))
				}
			}
		}
	}

	sig["services_count"] = len(services)
	sig["dependencies_count"] = len(deps)

	var b strings.Builder
	if len(services) > 0 {
		b.WriteString("Services:\n")
		for _, s := range services {
			b.WriteString("- " + s + "\n")
		}
	}
	if len(deps) > 0 {
		b.WriteString("Dependencies:\n")
		for _, d := range deps {
			b.WriteString("- " + d + "\n")
		}
	}
	if b.Len() == 0 {
		b.WriteString("Diagram JSON provided but no known keys found (expected services/dependencies or nodes/edges).")
	}
	return b.String(), sig
}
