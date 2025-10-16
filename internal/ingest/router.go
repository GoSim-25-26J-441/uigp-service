package ingest

import (
	"path/filepath"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

func DetectType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".drawio":
		return "drawio"
	case ".puml", ".plantuml":
		return "puml"
	case ".svg":
		return "svg"
	case ".pdf":
		return "pdf"
	case ".png", ".jpg", ".jpeg":
		return "raster"
	default:
		return "unknown"
	}
}

// Merge multiple files into one IntermediateGraph (append nodes/edges).
func BuildIntermediate(files []ParsedFile) types.IntermediateGraph {
	ig := types.IntermediateGraph{}
	for _, f := range files {
		ig.Nodes = append(ig.Nodes, f.Nodes...)
		ig.Edges = append(ig.Edges, f.Edges...)
		ig.Notes = append(ig.Notes, f.Notes...)
	}
	return ig
}

type ParsedFile struct {
	Name  string
	Nodes []types.Node
	Edges []types.Edge
	Notes []string
}
