package context

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

// maxYAMLChars limits architecture YAML embedded in the LLM system context.
// Keep this conservative; very large YAML payloads can trigger upstream model timeouts.
const maxYAMLChars = 4000

func BuildCompactContext(
	specSummary map[string]any,
	diagramJSON map[string]any,
	yamlContent string,
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

	yamlContent = strings.TrimSpace(yamlContent)
	if yamlContent != "" {
		yl := yamlContent
		truncated := false
		if len(yl) > maxYAMLChars {
			yl = yl[:maxYAMLChars] + "\n... [yaml truncated for context size]"
			truncated = true
		}
		blocks = append(blocks, compactYAMLContextBlock(yl))
		usedParts = append(usedParts, "yaml_content")
		signals["yaml_chars"] = len(yamlContent)
		if truncated {
			signals["yaml_truncated"] = true
		}
	}

	if depNote, depSig := dependencyConsistencyNote(diagramJSON, yamlContent); depNote != "" {
		blocks = append(blocks, depNote)
		for k, v := range depSig {
			signals[k] = v
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

	if note := connectivityAuthoritativeNote(signals, yamlContent); note != "" {
		blocks = append(blocks, note)
		signals["connectivity_all_sources_empty"] = true
	}

	return strings.Join(blocks, "\n\n"), strings.Join(usedParts, "+"), signals
}

func compactYAMLContextBlock(yaml string) string {
	depPairs := parseYAMLDependencyPairs(yaml)
	if len(depPairs) == 0 {
		if yamlShowsDependenciesAsEmptyList(yaml) {
			return "ARCHITECTURE YAML:\nDependencies: [] (explicitly empty in YAML)."
		}
		return "ARCHITECTURE YAML:\nYAML is present for this version, but no explicit dependency pairs were extracted."
	}

	deps := make([]string, 0, len(depPairs))
	for dep := range depPairs {
		deps = append(deps, dep)
	}
	sort.Strings(deps)

	const maxLines = 60
	var b strings.Builder
	b.WriteString("ARCHITECTURE YAML (dependency pairs extracted):\n")
	for i, dep := range deps {
		if i >= maxLines {
			b.WriteString("- ... additional dependencies omitted\n")
			break
		}
		b.WriteString("- ")
		b.WriteString(dep)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// yamlShowsDependenciesAsEmptyList is true when YAML clearly has dependencies: [] (possibly indented).
func yamlShowsDependenciesAsEmptyList(y string) bool {
	y = strings.TrimSpace(y)
	if y == "" {
		return true
	}
	re := regexp.MustCompile(`(?m)^\s*dependencies:\s*\[\s*\]\s*$`)
	if re.MatchString(y) {
		return true
	}
	// Inline / compact forms often used by generators
	return regexp.MustCompile(`(?m)\bdependencies:\s*\[\s*\]`).MatchString(y)
}

// yamlShowsDependencyObjects returns true when YAML lists at least one dependency object under dependencies.
func yamlShowsDependencyObjects(y string) bool {
	y = strings.TrimSpace(y)
	if y == "" {
		return false
	}
	i := strings.Index(y, "dependencies:")
	if i < 0 {
		return false
	}
	rest := y[i+len("dependencies:"):]
	// next top-level key at column 0 ends the section
	end := len(rest)
	for j := 1; j < len(rest); j++ {
		if rest[j] == '\n' && j+1 < len(rest) {
			c := rest[j+1]
			if c != ' ' && c != '\t' && c != '\r' && c != '\n' && c != '#' {
				// start of a new top-level line (letter or quote)
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					end = j + 1
					break
				}
			}
		}
	}
	section := rest[:end]
	return regexp.MustCompile(`(?m)^\s*-\s+from:`).MatchString(section)
}

func connectivityAuthoritativeNote(signals map[string]any, yaml string) string {
	diagDeps := intFromSignals(signals, "dependencies_count")
	specDeps := intFromSignals(signals, "spec_dependencies_count")
	if diagDeps > 0 || specDeps > 0 {
		return ""
	}
	comp := intFromSignals(signals, "services_count")
	if comp == 0 {
		comp = intFromSignals(signals, "spec_services_count")
	}
	comp += intFromSignals(signals, "spec_datastores_count")
	if comp < 1 {
		return ""
	}
	// YAML with real dependency objects overrides "empty" heuristics
	if yamlShowsDependencyObjects(yaml) {
		return ""
	}
	if !yamlShowsDependenciesAsEmptyList(yaml) {
		// YAML present but we cannot confirm an empty dependency list — skip the strong banner
		if strings.TrimSpace(yaml) != "" {
			return ""
		}
	}

	var b strings.Builder
	b.WriteString("CONNECTIVITY (authoritative for this turn — read before chat history):\n")
	b.WriteString("- diagram_json, spec_summary, and (where present) architecture YAML all show **no** dependencies/edges between components.\n")
	b.WriteString("- Do **not** state that nodes are connected, fully meshed, or wired together.\n")
	b.WriteString("- If a prior assistant message in history claimed connectivity, treat that message as **wrong** and correct it using this context only.\n")
	b.WriteString("- Diagram-visible gap: components are listed without any specified call/data/control flow between them (orphan / unspecified topology).\n")
	return b.String()
}

func intFromSignals(signals map[string]any, key string) int {
	v, ok := signals[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
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
	} else if len(services) > 0 || len(datastores) > 0 {
		b.WriteString("Dependencies:\n(none listed — do not assume services or datastores are connected.)\n")
		sig["spec_dependencies_missing"] = true
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

	// services[] / datastores[] / topics[] carry kinds used by dependencies (from/to are names, not canvas ids).
	mergeDiagramIdentityMaps(m, idToLabel, idToType)

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
					// Exact names only in context text (kinds are still in mergeDiagramIdentityMaps for analysis).
					services = append(services, n)
				}
			}
		}
	}
	var datastores []string
	if dv, ok := m["datastores"].([]any); ok {
		for _, v := range dv {
			dm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(dm["name"]))
			if name == "" || name == "<nil>" {
				continue
			}
			datastores = append(datastores, name)
		}
	}
	var topics []string
	if tv, ok := m["topics"].([]any); ok {
		for _, v := range tv {
			dm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(dm["name"]))
			if name == "" || name == "<nil>" {
				continue
			}
			topics = append(topics, name)
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
					if lbl == "" || lbl == "<nil>" {
						continue
					}
					// Label only; node types are available to risk hints via idToType.
					services = append(services, lbl)
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
	sig["datastores_count"] = len(datastores)
	sig["topics_count"] = len(topics)

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
		b.WriteString("Nodes (service and other components; names are exact strings from diagram JSON):\n")
		for _, s := range services {
			b.WriteString("- " + s + "\n")
		}
	}
	if len(datastores) > 0 {
		b.WriteString("Datastores (exact names from diagram JSON):\n")
		for _, d := range datastores {
			b.WriteString("- " + d + "\n")
		}
	}
	if len(topics) > 0 {
		b.WriteString("Topics (exact names from diagram JSON):\n")
		for _, tn := range topics {
			b.WriteString("- " + tn + "\n")
		}
	}
	if len(deps) > 0 {
		b.WriteString("Edges:\n")
		for _, d := range deps {
			b.WriteString("- " + d + "\n")
		}
	} else if len(services) > 0 {
		b.WriteString("Edges:\n(none listed in diagram JSON — do not infer connections between nodes.)\n")
		sig["diagram_edges_missing"] = true
	}

	// Highlight outbound edges from user-facing / entry nodes (client, user, user_actor, external)
	entryOutbound := diagramEntryOutboundLines(m, idToLabel, idToType)
	if len(entryOutbound) > 0 {
		b.WriteString("Entry / user-facing connectivity (outbound from client, user, user_actor, or external nodes):\n")
		for _, line := range entryOutbound {
			b.WriteString("- " + line + "\n")
		}
		sig["diagram_entry_edges_count"] = len(entryOutbound)
	}

	// Precomputed structural hints reduce reasoning misses for smaller LLMs.
	if hintsText, hintSignals := diagramRiskHints(m, idToLabel, idToType); hintsText != "" {
		b.WriteString("Structural risk hints (precomputed from topology):\n")
		b.WriteString(hintsText)
		if !strings.HasSuffix(hintsText, "\n") {
			b.WriteString("\n")
		}
		for k, v := range hintSignals {
			sig[k] = v
		}
	}

	if b.Len() == 0 {
		b.WriteString("Diagram JSON provided but no known keys found (expected services/dependencies or nodes/edges).")
	}
	return strings.TrimSpace(b.String()), sig
}

// mergeDiagramIdentityMaps fills idToLabel/idToType from services[], datastores[], and topics[]
// when the payload uses the spec-style shape (names in dependency from/to). Canvas nodes[] wins
// for ids that already have a type.
func mergeDiagramIdentityMaps(m map[string]any, idToLabel, idToType map[string]string) {
	if m == nil {
		return
	}
	if sv, ok := m["services"].([]any); ok {
		for _, v := range sv {
			switch t := v.(type) {
			case string:
				name := strings.TrimSpace(t)
				if name == "" {
					continue
				}
				if _, ok := idToLabel[name]; !ok {
					idToLabel[name] = name
				}
				if _, exists := idToType[name]; !exists {
					idToType[name] = "service"
				}
			case map[string]any:
				name := strings.TrimSpace(fmt.Sprint(t["name"]))
				if name == "" || name == "<nil>" {
					continue
				}
				if _, ok := idToLabel[name]; !ok {
					idToLabel[name] = name
				}
				kind := strings.ToLower(strings.TrimSpace(fmt.Sprint(t["kind"])))
				if kind != "" && kind != "<nil>" {
					if _, exists := idToType[name]; !exists {
						idToType[name] = kind
					}
				} else if _, exists := idToType[name]; !exists {
					idToType[name] = "service"
				}
			}
		}
	}
	if dv, ok := m["datastores"].([]any); ok {
		for _, v := range dv {
			dm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(dm["name"]))
			if name == "" || name == "<nil>" {
				continue
			}
			if _, ok := idToLabel[name]; !ok {
				idToLabel[name] = name
			}
			if _, exists := idToType[name]; !exists {
				idToType[name] = "db"
			}
		}
	}
	if tv, ok := m["topics"].([]any); ok {
		for _, v := range tv {
			dm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(dm["name"]))
			if name == "" || name == "<nil>" {
				continue
			}
			if _, ok := idToLabel[name]; !ok {
				idToLabel[name] = name
			}
			if _, exists := idToType[name]; !exists {
				idToType[name] = "topic"
			}
		}
	}
}

type topoEdge struct {
	FromID string
	ToID   string
	Proto  string
}

func diagramRiskHints(m map[string]any, idToLabel map[string]string, idToType map[string]string) (string, map[string]any) {
	sig := map[string]any{}
	edges := parseTopoEdges(m)
	if len(idToType) == 0 && len(edges) > 0 {
		for _, e := range edges {
			for _, id := range []string{e.FromID, e.ToID} {
				id = strings.TrimSpace(id)
				if id == "" || id == "<nil>" {
					continue
				}
				if _, ok := idToLabel[id]; !ok {
					idToLabel[id] = id
				}
				if _, ok := idToType[id]; !ok {
					idToType[id] = "unknown"
				}
			}
		}
	}
	if len(idToType) == 0 {
		return "", sig
	}

	hasGateway := false
	for _, t := range idToType {
		if t == "gateway" {
			hasGateway = true
			break
		}
	}

	incident := make(map[string]int, len(idToType))
	inbound := make(map[string][]topoEdge, len(idToType))
	outbound := make(map[string][]topoEdge, len(idToType))
	for _, e := range edges {
		incident[e.FromID]++
		incident[e.ToID]++
		outbound[e.FromID] = append(outbound[e.FromID], e)
		inbound[e.ToID] = append(inbound[e.ToID], e)
	}

	entryTypes := map[string]bool{"client": true, "user": true, "user_actor": true, "external": true}
	internalTypes := map[string]bool{"service": true, "gateway": true, "db": true, "database": true, "datastore": true, "topic": true, "queue": true}
	dbTypes := map[string]bool{"db": true, "database": true, "datastore": true}

	var orphan []string
	for id, typ := range idToType {
		if incident[id] != 0 {
			continue
		}
		_ = typ
		orphan = append(orphan, nodeLabel(id, idToLabel))
	}
	if len(orphan) > 0 {
		sig["orphan_components_count"] = len(orphan)
	}

	var gatewayBypass []string
	if hasGateway {
		for _, e := range edges {
			ft := idToType[e.FromID]
			tt := idToType[e.ToID]
			if !entryTypes[ft] {
				continue
			}
			if tt == "gateway" {
				continue
			}
			if internalTypes[tt] {
				gatewayBypass = append(gatewayBypass, fmt.Sprintf("%s -> %s", nodeLabel(e.FromID, idToLabel), nodeLabel(e.ToID, idToLabel)))
			}
		}
		if len(gatewayBypass) > 0 {
			sig["gateway_bypass_count"] = len(gatewayBypass)
		}
	}

	gatewayKinds := map[string]bool{"gateway": true, "bff": true, "edge": true, "apigateway": true}
	var serviceToGateway []string
	for _, e := range edges {
		ft := idToType[e.FromID]
		tt := idToType[e.ToID]
		if ft == "service" && gatewayKinds[tt] {
			serviceToGateway = append(serviceToGateway, fmt.Sprintf("%s -> %s", nodeLabel(e.FromID, idToLabel), nodeLabel(e.ToID, idToLabel)))
		}
	}
	if len(serviceToGateway) > 0 {
		sig["service_to_gateway_edges_count"] = len(serviceToGateway)
	}

	// Shared DB fan-in: more than one service writing/calling the same DB.
	dbCallers := map[string]map[string]bool{}
	for _, e := range edges {
		ft := idToType[e.FromID]
		tt := idToType[e.ToID]
		if ft != "service" || !dbTypes[tt] {
			continue
		}
		if dbCallers[e.ToID] == nil {
			dbCallers[e.ToID] = map[string]bool{}
		}
		dbCallers[e.ToID][e.FromID] = true
	}
	var sharedDB []string
	for dbID, callers := range dbCallers {
		if len(callers) > 1 {
			services := make([]string, 0, len(callers))
			for sid := range callers {
				services = append(services, nodeLabel(sid, idToLabel))
			}
			sharedDB = append(sharedDB, fmt.Sprintf("%s <= %s", nodeLabel(dbID, idToLabel), strings.Join(services, ", ")))
		}
	}
	if len(sharedDB) > 0 {
		sig["shared_db_fanin_count"] = len(sharedDB)
	}

	var dbOutbound []string
	var externalDB []string
	for _, e := range edges {
		ft := idToType[e.FromID]
		tt := idToType[e.ToID]
		if dbTypes[ft] {
			dbOutbound = append(dbOutbound, fmt.Sprintf("%s -> %s", nodeLabel(e.FromID, idToLabel), nodeLabel(e.ToID, idToLabel)))
			if entryTypes[tt] {
				externalDB = append(externalDB, fmt.Sprintf("%s -> %s", nodeLabel(e.FromID, idToLabel), nodeLabel(e.ToID, idToLabel)))
			}
		}
		if dbTypes[tt] && entryTypes[ft] {
			externalDB = append(externalDB, fmt.Sprintf("%s -> %s", nodeLabel(e.FromID, idToLabel), nodeLabel(e.ToID, idToLabel)))
		}
	}
	if len(dbOutbound) > 0 {
		sig["db_outbound_edges_count"] = len(dbOutbound)
	}
	if len(externalDB) > 0 {
		sig["external_db_direct_edges_count"] = len(externalDB)
	}

	cycleCount := countCycles(edges)
	if cycleCount > 0 {
		sig["cycle_count"] = cycleCount
	}

	var missingProtocol []string
	for _, e := range edges {
		if strings.TrimSpace(e.Proto) != "" {
			continue
		}
		missingProtocol = append(missingProtocol, fmt.Sprintf("%s -> %s", nodeLabel(e.FromID, idToLabel), nodeLabel(e.ToID, idToLabel)))
	}
	if len(missingProtocol) > 0 {
		sig["missing_protocol_edges_count"] = len(missingProtocol)
	}

	var lines []string
	if len(orphan) > 0 {
		lines = append(lines, "- graph-isolated components (listed in diagram but zero dependency edges as from/to): "+strings.Join(orphan, ", "))
	}
	if len(gatewayBypass) > 0 {
		lines = append(lines, "- gateway bypass edges: "+strings.Join(gatewayBypass, "; "))
	}
	if len(serviceToGateway) > 0 {
		lines = append(lines, "- service → gateway edges (ingress is usually gateway → service; verify intent): "+strings.Join(serviceToGateway, "; "))
	}
	if len(sharedDB) > 0 {
		lines = append(lines, "- shared database fan-in: "+strings.Join(sharedDB, "; "))
	}
	if len(dbOutbound) > 0 {
		lines = append(lines, "- database outbound edges (suspicious): "+strings.Join(dbOutbound, "; "))
	}
	if len(externalDB) > 0 {
		lines = append(lines, "- direct external<->database access: "+strings.Join(externalDB, "; "))
	}
	if cycleCount > 0 {
		lines = append(lines, fmt.Sprintf("- dependency cycles detected: %d", cycleCount))
	}
	if len(missingProtocol) > 0 {
		lines = append(lines, "- edges with missing protocol values: "+strings.Join(missingProtocol, "; "))
	}
	if len(lines) == 0 {
		return "", sig
	}
	return strings.Join(lines, "\n") + "\n", sig
}

func dependencyConsistencyNote(diagramJSON map[string]any, yamlContent string) (string, map[string]any) {
	sig := map[string]any{}
	yamlContent = strings.TrimSpace(yamlContent)
	if len(diagramJSON) == 0 || yamlContent == "" {
		return "", sig
	}

	yamlDeps := parseYAMLDependencyPairs(yamlContent)
	if len(yamlDeps) == 0 {
		return "", sig
	}
	diagramDeps := parseDiagramDependencyPairs(diagramJSON)
	if len(diagramDeps) == 0 {
		// If diagram has no edges but YAML has deps, it's a strong inconsistency.
		sig["yaml_diagram_dependency_mismatch_count"] = len(yamlDeps)
		return "DEPENDENCY CONSISTENCY HINT:\n- YAML lists dependencies but diagram_json has no listed edges/dependencies. Treat this as a structural inconsistency.\n", sig
	}

	var yamlOnly []string
	var diagramOnly []string
	for dep := range yamlDeps {
		if !diagramDeps[dep] {
			yamlOnly = append(yamlOnly, dep)
		}
	}
	for dep := range diagramDeps {
		if !yamlDeps[dep] {
			diagramOnly = append(diagramOnly, dep)
		}
	}
	if len(yamlOnly) == 0 && len(diagramOnly) == 0 {
		return "", sig
	}

	sig["yaml_diagram_dependency_mismatch_count"] = len(yamlOnly) + len(diagramOnly)
	var b strings.Builder
	b.WriteString("DEPENDENCY CONSISTENCY HINT:\n")
	if len(yamlOnly) > 0 {
		b.WriteString("- present in YAML only: " + strings.Join(yamlOnly, "; ") + "\n")
	}
	if len(diagramOnly) > 0 {
		b.WriteString("- present in diagram only: " + strings.Join(diagramOnly, "; ") + "\n")
	}
	b.WriteString("- Treat this mismatch as a diagram-visible structural inconsistency.\n")
	return b.String(), sig
}

func parseYAMLDependencyPairs(y string) map[string]bool {
	out := map[string]bool{}
	if strings.TrimSpace(y) == "" {
		return out
	}
	lines := strings.Split(y, "\n")
	inDeps := false
	currentFrom := ""
	currentTo := ""
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !inDeps {
			if strings.HasPrefix(trimmed, "dependencies:") {
				inDeps = true
			}
			continue
		}
		// stop section on next top-level key
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			break
		}
		if strings.HasPrefix(trimmed, "- from:") {
			currentFrom = strings.TrimSpace(strings.TrimPrefix(trimmed, "- from:"))
			currentFrom = strings.Trim(currentFrom, `"'`)
			currentTo = ""
			continue
		}
		if strings.HasPrefix(trimmed, "to:") {
			currentTo = strings.TrimSpace(strings.TrimPrefix(trimmed, "to:"))
			currentTo = strings.Trim(currentTo, `"'`)
			if currentFrom != "" && currentTo != "" {
				out[strings.ToLower(currentFrom)+"->"+strings.ToLower(currentTo)] = true
			}
		}
	}
	return out
}

func parseDiagramDependencyPairs(m map[string]any) map[string]bool {
	out := map[string]bool{}
	idToLabel := map[string]string{}
	if nv, ok := m["nodes"].([]any); ok {
		for _, v := range nv {
			nm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			id := strings.TrimSpace(fmt.Sprint(nm["id"]))
			lbl := strings.TrimSpace(fmt.Sprint(nm["label"]))
			if id == "" || id == "<nil>" || lbl == "" || lbl == "<nil>" {
				continue
			}
			idToLabel[id] = lbl
		}
	}
	for _, e := range parseTopoEdges(m) {
		from := nodeLabel(e.FromID, idToLabel)
		to := nodeLabel(e.ToID, idToLabel)
		from = strings.TrimSpace(strings.ToLower(from))
		to = strings.TrimSpace(strings.ToLower(to))
		if from == "" || to == "" || from == "<nil>" || to == "<nil>" {
			continue
		}
		out[from+"->"+to] = true
	}
	return out
}

func parseTopoEdges(m map[string]any) []topoEdge {
	var out []topoEdge
	if ev, ok := m["edges"].([]any); ok {
		for _, v := range ev {
			em, ok := v.(map[string]any)
			if !ok {
				continue
			}
			from := strings.TrimSpace(fmt.Sprint(em["from"]))
			to := strings.TrimSpace(fmt.Sprint(em["to"]))
			if from == "" || from == "<nil>" || to == "" || to == "<nil>" {
				continue
			}
			proto := strings.TrimSpace(fmt.Sprint(em["protocol"]))
			if proto == "<nil>" {
				proto = ""
			}
			out = append(out, topoEdge{FromID: from, ToID: to, Proto: proto})
		}
		return out
	}
	if dv, ok := m["dependencies"].([]any); ok {
		for _, v := range dv {
			dm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			from := strings.TrimSpace(fmt.Sprint(dm["from"]))
			to := strings.TrimSpace(fmt.Sprint(dm["to"]))
			if from == "" || from == "<nil>" || to == "" || to == "<nil>" {
				continue
			}
			proto := strings.TrimSpace(fmt.Sprint(dm["kind"]))
			if proto == "<nil>" {
				proto = ""
			}
			out = append(out, topoEdge{FromID: from, ToID: to, Proto: proto})
		}
	}
	return out
}

func nodeLabel(id string, idToLabel map[string]string) string {
	if l, ok := idToLabel[id]; ok && l != "" {
		return l
	}
	return id
}

func countCycles(edges []topoEdge) int {
	adj := map[string][]string{}
	nodes := map[string]bool{}
	for _, e := range edges {
		adj[e.FromID] = append(adj[e.FromID], e.ToID)
		nodes[e.FromID] = true
		nodes[e.ToID] = true
	}
	seen := map[string]bool{}
	stack := map[string]bool{}
	cycles := 0

	var dfs func(string)
	dfs = func(n string) {
		seen[n] = true
		stack[n] = true
		for _, nxt := range adj[n] {
			if !seen[nxt] {
				dfs(nxt)
				continue
			}
			if stack[nxt] {
				cycles++
			}
		}
		stack[n] = false
	}

	for n := range nodes {
		if !seen[n] {
			dfs(n)
		}
	}
	return cycles
}

// diagramEntryOutboundLines lists edges whose source node type is a flow actor (client, user, user_actor, external).
func diagramEntryOutboundLines(
	m map[string]any,
	idToLabel map[string]string,
	idToType map[string]string,
) []string {
	entryKind := map[string]bool{
		"client": true, "user": true, "user_actor": true, "external": true,
	}
	var lines []string
	for _, e := range parseTopoEdges(m) {
		fromID := strings.TrimSpace(e.FromID)
		if fromID == "" || fromID == "<nil>" {
			continue
		}
		if !entryKind[idToType[fromID]] {
			continue
		}
		toID := strings.TrimSpace(e.ToID)
		fromLbl := nodeLabel(fromID, idToLabel)
		toLbl := nodeLabel(toID, idToLabel)
		proto := strings.TrimSpace(e.Proto)
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
