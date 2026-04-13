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

	var blocks []string
	var usedParts []string

	// Prefer diagram_json first (more ground truth), but include spec_summary too if provided.
	if diagramJSON != nil && len(diagramJSON) > 0 {
		t, sig := compactFromDiagram(diagramJSON)
		for k, v := range sig {
			signals[k] = v
		}
		if strings.TrimSpace(t) != "" {
			blocks = append(blocks, "DIAGRAM CONTEXT:\n"+t)
			usedParts = append(usedParts, "diagram_json")
		}
	}

	if specSummary != nil && len(specSummary) > 0 {
		t, sig := compactFromSpecSummary(specSummary)
		for k, v := range sig {
			signals[k] = v
		}
		if strings.TrimSpace(t) != "" {
			blocks = append(blocks, "SPEC SUMMARY:\n"+t)
			usedParts = append(usedParts, "spec_summary")
		}
	}

	if len(atts) > 0 {
		var lines []string
		for _, a := range atts {
			lines = append(lines, fmt.Sprintf("- %s (%s)", a.Name, a.ContentType))
		}
		signals["attachments_detected"] = len(atts)
		blocks = append(blocks, "ATTACHMENTS:\n"+strings.Join(lines, "\n"))
		usedParts = append(usedParts, "attachments")
	}

	if len(blocks) == 0 {
		return "", "none", signals
	}
	return strings.Join(blocks, "\n\n"), strings.Join(usedParts, "+"), signals
}

// Instead of “keys only”, show useful content (services/dependencies/datastores if present).
func compactFromSpecSummary(m map[string]any) (string, map[string]any) {
	sig := map[string]any{}

	services := readStringList(m["services"])
	deps := readStringList(m["dependencies"])
	datastores := readStringList(m["datastores"])

	sig["spec_services_count"] = len(services)
	sig["spec_dependencies_count"] = len(deps)
	sig["spec_datastores_count"] = len(datastores)

	serviceTypes := readStringStringMap(m["service_types"])
	if len(serviceTypes) > 0 {
		sig["spec_service_types_count"] = len(serviceTypes)
	}

	var b strings.Builder
	if len(services) > 0 {
		b.WriteString("Components (non-datastore):\n")
		for _, s := range services {
			if typ, ok := serviceTypes[s]; ok && typ != "" {
				b.WriteString("- " + s + " (" + typ + ")\n")
			} else {
				b.WriteString("- " + s + "\n")
			}
		}
	}
	if len(deps) > 0 {
		b.WriteString("Dependencies:\n")
		for _, d := range deps {
			b.WriteString("- " + d + "\n")
		}
	}
	if len(datastores) > 0 {
		b.WriteString("Datastores:\n")
		for _, d := range datastores {
			b.WriteString("- " + d + "\n")
		}
	}

	// fallback if user sent unexpected schema
	if b.Len() == 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		b.WriteString(fmt.Sprintf("spec_summary provided (keys: %v).", keys))
	}

	return strings.TrimSpace(b.String()), sig
}

func compactFromDiagram(m map[string]any) (string, map[string]any) {
	sig := map[string]any{}

	// Build id->label and id->type so edges and entry hints use human-readable names
	idToLabel := map[string]string{}
	idToType := map[string]string{}
	if nv, ok := m["nodes"].([]any); ok {
		for _, v := range nv {
			if nm, ok := v.(map[string]any); ok {
				id := fmt.Sprint(nm["id"])
				lbl := fmt.Sprint(nm["label"])
				if id != "" && id != "<nil>" && lbl != "" && lbl != "<nil>" {
					idToLabel[id] = lbl
				}
				typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(nm["type"])))
				if id != "" && id != "<nil>" && typ != "" && typ != "<nil>" {
					idToType[id] = typ
				}
			}
		}
	}

	var services []string
	var deps []string

	// support both “services/dependencies” and “nodes/edges”
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
					lbl := fmt.Sprint(nm["label"])
					typ := fmt.Sprint(nm["type"])
					if lbl == "" || lbl == "<nil>" {
						continue
					}
					if typ != "" && typ != "<nil>" {
						services = append(services, fmt.Sprintf("%s (%s)", lbl, typ))
					} else {
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

					if lbl, ok := idToLabel[from]; ok {
						from = lbl
					}
					if lbl, ok := idToLabel[to]; ok {
						to = lbl
					}
					deps = append(deps, fmt.Sprintf("%s -> %s (%s)", from, to, proto))
				}
			}
		}
	}

	sig["services_count"] = len(services)
	sig["dependencies_count"] = len(deps)

	// include diagram_version_id if present
	if md, ok := m["metadata"].(map[string]any); ok {
		dv := fmt.Sprint(md["diagram_version_id"])
		if dv != "" && dv != "<nil>" {
			sig["diagram_version_id"] = dv
		}
	}

	var b strings.Builder
	if dv, ok := sig["diagram_version_id"].(string); ok && dv != "" {
		b.WriteString("Diagram version: " + dv + "\n")
	}
	if len(services) > 0 {
		b.WriteString("Nodes:\n")
		for _, s := range services {
			b.WriteString("- " + s + "\n")
		}
	}
	if len(deps) > 0 {
		b.WriteString("Edges:\n")
		for _, d := range deps {
			b.WriteString("- " + d + "\n")
		}
	}

	// Highlight outbound edges from user-facing / entry nodes (client, user, external)
	entryOutbound := diagramEntryOutboundLines(m, idToLabel, idToType)
	if len(entryOutbound) > 0 {
		b.WriteString("Entry / user-facing connectivity (outbound from client, user, or external nodes):\n")
		for _, line := range entryOutbound {
			b.WriteString("- " + line + "\n")
		}
		sig["diagram_entry_edges_count"] = len(entryOutbound)
	}

	if b.Len() == 0 {
		b.WriteString("Diagram JSON provided but no known keys found (expected services/dependencies or nodes/edges).")
	}
	return strings.TrimSpace(b.String()), sig
}

// diagramEntryOutboundLines lists edges whose source node type is client, user, or external.
func diagramEntryOutboundLines(
	m map[string]any,
	idToLabel map[string]string,
	idToType map[string]string,
) []string {
	ev, ok := m["edges"].([]any)
	if !ok || len(ev) == 0 {
		return nil
	}
	entryKind := map[string]bool{
		"client": true, "user": true, "external": true,
	}
	var lines []string
	for _, v := range ev {
		em, ok := v.(map[string]any)
		if !ok {
			continue
		}
		fromID := strings.TrimSpace(fmt.Sprint(em["from"]))
		if fromID == "" || fromID == "<nil>" {
			continue
		}
		if !entryKind[idToType[fromID]] {
			continue
		}
		toID := strings.TrimSpace(fmt.Sprint(em["to"]))
		fromLbl := fromID
		if lb, ok := idToLabel[fromID]; ok && lb != "" {
			fromLbl = lb
		}
		toLbl := toID
		if lb, ok := idToLabel[toID]; ok && lb != "" {
			toLbl = lb
		}
		proto := strings.TrimSpace(fmt.Sprint(em["protocol"]))
		if proto == "" || proto == "<nil>" {
			proto = "?"
		}
		lines = append(lines, fmt.Sprintf("%s → %s (%s)", fromLbl, toLbl, proto))
	}
	return lines
}

func readStringList(v any) []string {
	var out []string
	switch t := v.(type) {
	case []string:
		return append(out, t...)
	case []any:
		for _, x := range t {
			s := strings.TrimSpace(fmt.Sprint(x))
			if s != "" && s != "<nil>" {
				out = append(out, s)
			}
		}
	}
	return out
}

// readStringStringMap reads JSON objects like service_types into a string map.
func readStringStringMap(v any) map[string]string {
	out := make(map[string]string)
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			s := strings.TrimSpace(fmt.Sprint(val))
			if s != "" && s != "<nil>" {
				out[k] = strings.ToLower(s)
			}
		}
	case map[string]string:
		for k, val := range t {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			s := strings.TrimSpace(val)
			if s != "" {
				out[k] = strings.ToLower(s)
			}
		}
	}
	return out
}
