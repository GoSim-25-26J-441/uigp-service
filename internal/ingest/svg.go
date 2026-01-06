package ingest

import (
	"encoding/xml"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

type svgDoc struct {
	Texts []svgText `xml:"text"`
	Lines []svgLine `xml:"line"`
}
type svgText struct {
	X string `xml:"x,attr"`
	Y string `xml:"y,attr"`
	T string `xml:",chardata"`
}
type svgLine struct {
	X1        string `xml:"x1,attr"`
	Y1        string `xml:"y1,attr"`
	X2        string `xml:"x2,attr"`
	Y2        string `xml:"y2,attr"`
	Class     string `xml:"class,attr"`
	MarkerEnd string `xml:"marker-end,attr"`
}

func ParseSVG(fp string) (ParsedFile, error) {
	b, err := os.ReadFile(fp)
	if err != nil {
		return ParsedFile{}, err
	}

	var doc svgDoc
	if err := xml.Unmarshal(b, &doc); err != nil {
		return ParsedFile{
			Name:  filepath.Base(fp),
			Notes: []string{"svg: unmarshal failed (unsupported structure)"},
		}, nil
	}

	var nodes []types.Node
	type pt struct{ x, y float64 }
	textPos := map[string]pt{}
	seen := map[string]int{}

	for _, t := range doc.Texts {
		label := strings.TrimSpace(t.T)
		if label == "" {
			continue
		}
		id := slugify(label)
		seen[id]++
		if seen[id] > 1 {
			id = id + "_" + itoa(seen[id])
		}

		xf, _ := strconv.ParseFloat(strings.TrimSpace(t.X), 64)
		yf, _ := strconv.ParseFloat(strings.TrimSpace(t.Y), 64)
		textPos[id] = pt{xf, yf}

		nodes = append(nodes, types.Node{
			ID:     id,
			Type:   guessTypeFromLabel(label),
			Label:  label,
			Source: "svg",
		})
	}

	var edges []types.Edge
	if len(nodes) > 1 && len(doc.Lines) > 0 {
		type npos struct {
			id string
			p  pt
		}
		var lst []npos
		for id, p := range textPos {
			lst = append(lst, npos{id, p})
		}

		nearest := func(x, y float64) string {
			best := ""
			bestd := math.MaxFloat64
			for _, np := range lst {
				dx := x - np.p.x
				dy := y - np.p.y
				d := dx*dx + dy*dy
				if d < bestd {
					bestd = d
					best = np.id
				}
			}
			return best
		}

		for _, ln := range doc.Lines {
			if !(strings.Contains(strings.ToLower(ln.Class), "arrow") || ln.MarkerEnd != "") {
				continue
			}
			x1, _ := strconv.ParseFloat(strings.TrimSpace(ln.X1), 64)
			y1, _ := strconv.ParseFloat(strings.TrimSpace(ln.Y1), 64)
			x2, _ := strconv.ParseFloat(strings.TrimSpace(ln.X2), 64)
			y2, _ := strconv.ParseFloat(strings.TrimSpace(ln.Y2), 64)

			from := nearest(x1, y1)
			to := nearest(x2, y2)
			if from == "" || to == "" || from == to {
				continue
			}

			proto := "REST"
			cl := strings.ToLower(ln.Class)
			switch {
			case strings.Contains(cl, "grpc"):
				proto = "gRPC"
			case strings.Contains(cl, "pub") || strings.Contains(cl, "sub"):
				proto = "PUBSUB"
			}

			edges = append(edges, types.Edge{
				From:     from,
				To:       to,
				Protocol: proto,
			})
		}
	}

	notes := []string{}
	if len(edges) == 0 {
		notes = append(notes, `svg: text parsed, no arrows detected (add class="arrow" or marker-end for lines)`)
	}

	return ParsedFile{
		Name:  filepath.Base(fp),
		Nodes: nodes,
		Edges: edges,
		Notes: notes,
	}, nil
}
