package types

type Node struct {
	ID     string
	Type   string // service|db|queue|gateway|ext
	Label  string
	Source string
	BBox   [4]int
}
type Edge struct {
	From, To, Protocol string // REST|gRPC|PUB|SUB
	BBox               [4]int
}
type IntermediateGraph struct {
	Nodes []Node
	Edges []Edge
	Notes []string
	Trace []any
}
