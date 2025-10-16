package ingest

import (
	"encoding/xml"
	"os"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

type svgDoc struct {
	Texts []svgText `xml:"text"`
}
type svgText struct {
	X string `xml:"x,attr"`
	Y string `xml:"y,attr"`
	T string `xml:",chardata"`
}

func ParseSVG(path string) (ParsedFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{Name: path}, err
	}

	var doc svgDoc
	_ = xml.Unmarshal(b, &doc) // best-effort; if it fails, we just return notes

	var nodes []types.Node
	for i, t := range doc.Texts {
		label := strings.TrimSpace(t.T)
		if label == "" {
			continue
		}
		nodes = append(nodes, types.Node{
			ID:     "svg_" + strings.TrimSpace(t.X) + "_" + strings.TrimSpace(t.Y) + "_" + string(rune(i)),
			Type:   guessTypeFromLabel(label),
			Label:  label,
			Source: "svg",
		})
	}
	notes := []string{"svg: best-effort text extraction; edges not parsed yet"}
	return ParsedFile{Name: path, Nodes: nodes, Notes: notes}, nil
}
