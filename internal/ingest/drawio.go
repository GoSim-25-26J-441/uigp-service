package ingest

import (
	"encoding/xml"
	"os"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

type mxfile struct {
	Diagram []diagram `xml:"diagram"`
}
type diagram struct {
	MxGraphModel mxGraphModel `xml:"mxGraphModel"`
}
type mxGraphModel struct {
	Root root `xml:"root"`
}
type root struct {
	Cells []mxCell `xml:"mxCell"`
}

type mxCell struct {
	ID     string `xml:"id,attr"`
	Value  string `xml:"value,attr"`
	Vertex string `xml:"vertex,attr"` // "1" if node
	Edge   string `xml:"edge,attr"`   // "1" if edge
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
	Parent string `xml:"parent,attr"`
}

func ParseDrawIO(path string) (ParsedFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{Name: path}, err
	}

	var doc mxfile
	if err := xml.Unmarshal(b, &doc); err != nil {
		return ParsedFile{Name: path, Notes: []string{"xml unmarshal failed"}}, nil // soft fail: just a note
	}

	var nodes []types.Node
	var edges []types.Edge

	for _, d := range doc.Diagram {
		for _, c := range d.MxGraphModel.Root.Cells {
			if c.Vertex == "1" {
				label := htmlUnescape(stripHTML(c.Value))
				if label == "" {
					label = "node-" + c.ID
				}
				nodes = append(nodes, types.Node{
					ID: c.ID, Type: guessTypeFromLabel(label),
					Label: label, Source: "drawio",
				})
			} else if c.Edge == "1" {
				edges = append(edges, types.Edge{
					From: c.Source, To: c.Target,
					Protocol: guessProtocolFromValue(c.Value),
				})
			}
		}
	}
	return ParsedFile{Name: path, Nodes: nodes, Edges: edges}, nil
}

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	// draw.io often wraps labels like <div>Service</div>
	s = strings.ReplaceAll(s, "<div>", "")
	s = strings.ReplaceAll(s, "</div>", "")
	return s
}
func htmlUnescape(s string) string {
	repl := map[string]string{"&amp;": "&", "&lt;": "<", "&gt;": ">", "&#xa;": " "}
	for k, v := range repl {
		s = strings.ReplaceAll(s, k, v)
	}
	return strings.TrimSpace(s)
}
func guessTypeFromLabel(label string) string {
	l := strings.ToLower(label)
	switch {
	case strings.HasPrefix(l, "db:"):
		return "db"
	case strings.HasPrefix(l, "q:"), strings.Contains(l, "queue"):
		return "queue"
	case strings.Contains(l, "gateway") || strings.HasPrefix(l, "gw:"):
		return "gateway"
	default:
		return "service"
	}
}
func guessProtocolFromValue(v string) string {
	l := strings.ToLower(v)
	switch {
	case strings.Contains(l, "grpc"):
		return "gRPC"
	case strings.Contains(l, "rest"):
		return "REST"
	case strings.Contains(l, "pub"):
		return "PUB"
	case strings.Contains(l, "sub"):
		return "SUB"
	default:
		return ""
	}
}
