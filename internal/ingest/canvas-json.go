package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

type canvasDoc struct {
	Services []struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"services"`

	Datastores []struct {
		Name string `json:"name"`
	} `json:"datastores"`

	Topics []struct {
		Name string `json:"name"`
	} `json:"topics"`

	Dependencies []struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Kind  string `json:"kind"`
		Sync  bool   `json:"sync"`
		Label string `json:"label"`
	} `json:"dependencies"`
}

// parse one canvas JSON file into IntermediateGraph
func fromCanvasJSON(path string) (*types.IntermediateGraph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var doc canvasDoc
	if err := json.NewDecoder(f).Decode(&doc); err != nil {
		return nil, err
	}

	g := &types.IntermediateGraph{
		// BBox fields left as zero values
	}

	addNode := func(id, kind string) {
		if id == "" {
			return
		}
		t := "service"
		k := strings.ToLower(kind)

		switch k {
		case "database", "db", "datastore":
			t = "database"
		case "gateway", "api gateway":
			t = "gateway"
		case "client", "user", "actor":
			t = "actor"
		case "topic":
			t = "topic"
		}

		g.Nodes = append(g.Nodes, types.Node{
			ID:     id,
			Label:  id,
			Type:   t,
			Source: "canvas-json",
		})
	}

	// services, datastores, topics → nodes
	for _, s := range doc.Services {
		addNode(s.Name, s.Kind)
	}
	for _, d := range doc.Datastores {
		addNode(d.Name, "database")
	}
	for _, t := range doc.Topics {
		addNode(t.Name, "topic")
	}

	// dependencies → edges
	for _, dep := range doc.Dependencies {
		proto := strings.ToUpper(dep.Kind)
		if proto == "" {
			proto = "REST"
		}

		g.Edges = append(g.Edges, types.Edge{
			From:     dep.From,
			To:       dep.To,
			Protocol: proto,
		})
	}

	return g, nil
}

func ParseCanvasJSON(path string) (ParsedFile, error) {
	g, err := fromCanvasJSON(path) // your existing helper returning *types.IntermediateGraph
	if err != nil {
		return ParsedFile{}, err
	}

	return ParsedFile{
		Name:  filepath.Base(path),
		Nodes: g.Nodes, // []types.Node
		Edges: g.Edges, // []types.Edge
		Notes: g.Notes,
	}, nil
}
