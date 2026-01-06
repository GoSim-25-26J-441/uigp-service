package ingest

import (
	"os"
	"regexp"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

var reComp = regexp.MustCompile(`(?i)^(component|rectangle|node)\s+"?([^"]+)"?\s*(?:as\s+([A-Za-z0-9_]+))?`)

var reLink = regexp.MustCompile(`(?i)^("?[\w\s\-]+"?|[A-Za-z0-9_]+)\s*[-\.~]*[o]?>{1,2}\s*(?:left|right|up|down)?\s*("?[\w\s\-]+"?|[A-Za-z0-9_]+)\s*(?::\s*(.+))?$`)

var reProto = regexp.MustCompile(`(?i)(?:<<\s*(grpc|rest|pub|sub)\s*>>|\[(grpc|rest|pub|sub)\])`)

func ParsePUML(path string) (ParsedFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{Name: path}, err
	}
	lines := strings.Split(string(b), "\n")

	idByLabel := map[string]string{}
	var nodes []types.Node
	var edges []types.Edge
	var notes []string

	seen := func(id string) bool {
		for _, n := range nodes {
			if n.ID == id {
				return true
			}
		}
		return false
	}

	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "'") {
			continue
		}
		if m := reComp.FindStringSubmatch(l); m != nil {
			label := strings.TrimSpace(m[2])
			alias := strings.TrimSpace(m[3])
			id := alias
			if id == "" {
				id = sanitizeID(label)
			}
			idByLabel[label] = id
			if !seen(id) {
				nodes = append(nodes, types.Node{
					ID:     id,
					Type:   guessTypeFromLabel(label),
					Label:  label,
					Source: "puml",
				})
			}
			continue
		}

		if m := reLink.FindStringSubmatch(l); m != nil {
			from := cleanRef(m[1], idByLabel)
			to := cleanRef(m[2], idByLabel)
			ann := strings.TrimSpace(m[3])
			proto := guessProtocolFromValue(ann)

			if proto == "" {
				if pm := reProto.FindStringSubmatch(ann); pm != nil {
					if pm[1] != "" {
						proto = strings.ToUpper(pm[1])
					} else {
						proto = strings.ToUpper(pm[2])
					}
				}
			}
			edges = append(edges, types.Edge{From: from, To: to, Protocol: proto})
			continue
		}

	}

	for lbl, id := range idByLabel {
		if !seen(id) {
			nodes = append(nodes, types.Node{ID: id, Type: guessTypeFromLabel(lbl), Label: lbl, Source: "puml"})
		}
	}

	if len(nodes) == 0 && len(edges) == 0 {
		notes = append(notes, "puml: no components/links recognized (check syntax)")
	}
	return ParsedFile{Name: path, Nodes: nodes, Edges: edges, Notes: notes}, nil
}

func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.Trim(s, `"`)))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func cleanRef(s string, ids map[string]string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	if id, ok := ids[s]; ok {
		return id
	}

	return sanitizeID(s)
}
