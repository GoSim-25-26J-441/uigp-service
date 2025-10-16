package ingest

import (
	"os"
	"regexp"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

var (
	reComp = regexp.MustCompile(`(?i)^(component|rectangle|node)\s+"?([^"]+)"?\s*(as\s+([A-Za-z0-9_]+))?`)
	reLink = regexp.MustCompile(`(?i)^([A-Za-z0-9_"]+)\s*[-\.]*>{1,2}\s*([A-Za-z0-9_"]+)\s*(?::\s*(.+))?$`)
)

func ParsePUML(path string) (ParsedFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{Name: path}, err
	}
	lines := strings.Split(string(b), "\n")

	idByLabel := map[string]string{}
	var nodes []types.Node
	var edges []types.Edge

	for _, ln := range lines {
		l := strings.TrimSpace(ln)
		if l == "" || strings.HasPrefix(l, "'") {
			continue
		}

		if m := reComp.FindStringSubmatch(l); m != nil {
			label := strings.TrimSpace(m[2])
			id := m[4]
			if id == "" {
				id = sanitizeID(label)
			}
			idByLabel[label] = id
			nodes = append(nodes, types.Node{
				ID: id, Label: label, Type: "service", Source: "puml",
			})
			continue
		}
		if m := reLink.FindStringSubmatch(l); m != nil {
			from := cleanRef(m[1], idByLabel)
			to := cleanRef(m[2], idByLabel)
			proto := guessProtocolFromValue(m[3])
			edges = append(edges, types.Edge{From: from, To: to, Protocol: proto})
		}
	}
	return ParsedFile{Name: path, Nodes: nodes, Edges: edges}, nil
}

func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
func cleanRef(s string, ids map[string]string) string {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if id, ok := ids[s]; ok {
		return id
	}
	return sanitizeID(s)
}
