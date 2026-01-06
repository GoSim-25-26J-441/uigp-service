// TODO(christy): OCR-based raster parsing is experimental.
// Replace with ML-based diagram recognizer in v2.

package ingest

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/MalithGihan/uigp-service/pkg/types"
)

type ocrLine struct {
	Text      string
	Left, Top int
	Width, Ht int
}

var relPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b([a-z0-9_\-]+)\s*(?:[-–—]{1,2}\s*>\s*|→)\s*([a-z0-9_\-]+)\s*(?:\(\s*([a-z\s\-]+)\s*\))?\b`),
	regexp.MustCompile(`(?i)\b([a-z0-9_\-]+)\s+to\s+([a-z0-9_\-]+)\s+([a-z\s\-]+)\b`),
	regexp.MustCompile(`(?i)\b([a-z0-9_\-]+)\s+to\s+([a-z0-9_\-]+)\b.*\b(grpc|rest|pub\s*sub)\b`),
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type nodePos struct {
	id   string
	x, y int
}

var rxNameProto = regexp.MustCompile(`(?i)^\s*([a-z0-9_\-]+)\s*\(\s*([a-z\s\-]+)\s*\)\s*$`)

func normalizeWord(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "grp", "grcp", "gprc", "qrpc", "grpe", "grpo", "gpc", "gkpc",
		"grfc", "grefc", "rpc":
		return "grpc"
	case "rest", "reist", "re5t", "rex":
		return "rest"
	case "pub", "sub", "pubsub":
		return "pubsub"

	case "servlce", "sevvice", "servicc", "servicee",
		"sewice", "sevice", "srvice", "servce":
		return "service"

	case "payrnent", "paymen", "paymant", "paymens":
		return "payment"

	case "order", "orders", "arders", "aorders", "aerders", "aers",
		"onsers", "ners":
		return "orders"

	case "ication", "notication", "notfeston", "notfeation",
		"notfication", "nofication":
		return "notification"
	}

	return s
}

func protocolNorm(s string) string {
	s = normalizeProtoToken(s)

	switch s {
	case "grpc":
		return "gRPC"
	case "pubsub":
		return "PUBSUB"
	case "rest", "http":
		return "REST"
	default:
		return ""
	}
}

func centerOf(l ocrLine) (cx, cy int) { return l.Left + l.Width/2, l.Top + l.Ht/2 }

func ocrWithTesseract(fp string) (string, error) {
	try := func(psm string) (string, error) {
		cmd := exec.Command(
			"tesseract", fp, "stdout",
			"--oem", "1",
			"--psm", psm,
			"-l", "eng",
			"--dpi", "300",
			"-c", "tessedit_char_whitelist=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789->() ",
			"-c", "preserve_interword_spaces=1",
		)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return out.String(), nil
	}
	if s, err := try("11"); err == nil && strings.TrimSpace(s) != "" {
		return s, nil
	}
	if s, err := try("6"); err == nil && strings.TrimSpace(s) != "" {
		return s, nil
	}
	if s, err := try("12"); err == nil && strings.TrimSpace(s) != "" {
		return s, nil
	}
	return "", nil
}

func ocrTSVLinesWithBoxes(fp string) ([]ocrLine, error) {
	cmd := exec.Command("tesseract", fp, "stdout", "-l", "eng", "--oem", "1", "--psm", "6", "tsv")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var lines []ocrLine
	curLine := -1
	var words []string
	l, t, w, h := 0, 0, 0, 0

	sc := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 12 {
			continue
		}

		lineNumStr := fields[4]
		leftStr, topStr, widthStr, heightStr := fields[6], fields[7], fields[8], fields[9]
		confStr, text := fields[10], strings.TrimSpace(fields[11])

		lineNum, _ := strconv.Atoi(lineNumStr)
		left, _ := strconv.Atoi(leftStr)
		top, _ := strconv.Atoi(topStr)
		width, _ := strconv.Atoi(widthStr)
		height, _ := strconv.Atoi(heightStr)
		conf, _ := strconv.Atoi(confStr)

		if text == "" || conf < 40 {
			continue
		}
		text = normalizeWord(text)

		if curLine == -1 {
			curLine = lineNum
			l, t, w, h = left, top, width, height
		}
		if lineNum != curLine {
			if len(words) > 0 {
				lines = append(lines, ocrLine{
					Text: strings.Join(words, " "), Left: l, Top: t, Width: w, Ht: h,
				})
			}
			words = nil
			curLine = lineNum
			l, t, w, h = left, top, width, height
		} else {
			// expand current bbox
			if left < l {
				l = left
			}
			if top < t {
				t = top
			}
			if left+width > l+w {
				w = left + width - l
			}
			if top+height > t+h {
				h = top + height - t
			}
		}
		words = append(words, text)
	}
	if len(words) > 0 {
		lines = append(lines, ocrLine{Text: strings.Join(words, " "), Left: l, Top: t, Width: w, Ht: h})
	}
	return lines, nil
}

func pickLabels(s string) []string {
	s = strings.ReplaceAll(s, "0", "o")
	s = strings.ReplaceAll(s, "1", "l")
	s = strings.ReplaceAll(s, "rn", "m")
	s = strings.ReplaceAll(s, "vv", "w")

	split := func(r rune) bool { return !(unicode.IsLetter(r) || unicode.IsNumber(r)) }
	var out []string
	for _, tok := range strings.FieldsFunc(s, split) {
		tok = normalizeWord(tok)
		tok = normToken(tok)

		if tok == "" {
			continue
		}
		// skip arrow chars & pure protocols
		if tok == "-" || tok == ">" || tok == "grpc" || tok == "rest" || tok == "pubsub" {
			continue
		}
		if !isPlausibleLabel(tok) {
			continue
		}
		out = append(out, tok)
	}
	return dedupLower(out)
}

func normToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "vv", "w")
	s = strings.ReplaceAll(s, "rn", "m")
	var b strings.Builder
	prev := rune(0)
	dup := 0
	for _, r := range s {
		if r == prev {
			dup++
		} else {
			dup = 0
		}
		if dup < 1 {
			b.WriteRune(r)
		} // at most double
		prev = r
	}
	return b.String()
}

func isPlausibleLabel(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}

	if s == "db" || s == "api" {
		return true
	}

	if len(s) < 3 || len(s) > 30 {
		return false
	}

	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return false
		}
	}

	vowels := 0
	for _, r := range s {
		switch r {
		case 'a', 'e', 'i', 'o', 'u':
			vowels++
		}
	}
	if vowels == 0 || vowels == len(s) {
		return false
	}

	prev := rune(0)
	run := 1
	for _, r := range s {
		if r == prev {
			run++
		} else {
			run = 1
		}
		if run > 3 {
			return false
		}
		prev = r
	}
	return true
}

func dedupLower(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		k := strings.ToLower(s)
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

func buildNodesFromLabels(labels []string) []types.Node {
	var nodes []types.Node
	seen := map[string]int{}
	for _, lbl := range labels {
		id := slugify(lbl)
		seen[id]++
		if seen[id] > 1 {
			id = id + "_" + itoa(seen[id])
		}
		nodes = append(nodes, types.Node{
			ID: id, Type: guessTypeFromLabel(lbl), Label: lbl, Source: "raster",
		})
	}
	return nodes
}

func edgesFromText(ocr string, nodes []types.Node) []types.Edge {
	text := strings.ToLower(ocr)
	var edges []types.Edge

	for _, rx := range relPatterns {
		ms := rx.FindAllStringSubmatch(text, -1)
		for _, m := range ms {
			fromName, toName := normalizeWord(m[1]), normalizeWord(m[2])

			proto := ""
			if len(m) >= 4 && strings.TrimSpace(m[3]) != "" {
				proto = protocolNorm(m[3])
			}
			if proto == "" {
				proto = detectProtoFromText(text)
			}

			fromID, ok1 := closestNodeID(fromName, nodes)
			toID, ok2 := closestNodeID(toName, nodes)
			if ok1 && ok2 && fromID != toID {
				edges = append(edges, types.Edge{
					From:     fromID,
					To:       toID,
					Protocol: proto,
				})
			}
		}
	}

	if len(nodes) == 2 && (len(edges) == 0 || len(edges) == 1) {
		n0Server := isServerish(nodes[0].Label)
		n1Server := isServerish(nodes[1].Label)

		from := nodes[0].ID
		to := nodes[1].ID

		if n0Server && !n1Server {
			from, to = nodes[1].ID, nodes[0].ID
		} else if n1Server && !n0Server {
			from, to = nodes[0].ID, nodes[1].ID
		} else {
			l0 := strings.ToLower(nodes[0].Label)
			l1 := strings.ToLower(nodes[1].Label)
			i0 := strings.Index(text, l0)
			i1 := strings.Index(text, l1)
			if i0 >= 0 && i1 >= 0 && i1 < i0 {
				from, to = nodes[1].ID, nodes[0].ID
			}
		}

		proto := detectProtoFromText(text)

		if len(edges) == 0 {
			edges = append(edges, types.Edge{From: from, To: to, Protocol: proto})
		} else {
			edges[0].From = from
			edges[0].To = to
			edges[0].Protocol = proto
		}
	}

	return edges
}

func editDist(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	m, n := len(ra), len(rb)
	dp := make([]int, n+1)
	for j := 0; j <= n; j++ {
		dp[j] = j
	}
	for i := 1; i <= m; i++ {
		prev, cur := i-1, i
		dp[0] = i
		for j := 1; j <= n; j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			v := min3(dp[j]+1, cur+1, prev+cost)
			prev, cur, dp[j] = dp[j], v, v
		}
	}
	return dp[n]
}
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func closestNodeID(name string, nodes []types.Node) (string, bool) {
	name = normToken(name)
	bestID, best := "", 999
	for _, n := range nodes {
		cand := normToken(n.Label)
		d := editDist(name, cand)
		if d < best {
			best, bestID = d, n.ID
		}
	}
	return bestID, best <= 2
}

func ParseRaster(fp string) (ParsedFile, error) {

	tsv, _ := ocrTSVLinesWithBoxes(fp)

	var norm []string
	if len(tsv) > 0 {
		norm = make([]string, len(tsv))
		for i, ln := range tsv {
			parts := strings.Fields(ln.Text)
			for j, p := range parts {
				parts[j] = normalizeWord(p)
			}
			norm[i] = strings.Join(parts, " ")
			tsv[i].Text = norm[i]

			fmt.Printf("[tsv %d] %q at (%d,%d) %dx%d\n",
				i, tsv[i].Text, ln.Left, ln.Top, ln.Width, ln.Ht)
		}
	}

	plain, _ := ocrWithTesseract(fp)

	var rawParts []string
	if len(norm) > 0 {
		rawParts = append(rawParts, strings.Join(norm, "\n"))
	}
	if strings.TrimSpace(plain) != "" {
		rawParts = append(rawParts, plain)
	}
	raw := strings.Join(rawParts, "\n")

	rawSnippet := raw
	if len(rawSnippet) > 200 {
		rawSnippet = rawSnippet[:200] + "..."
	}

	labels := pickLabels(raw)
	nodes := buildNodesFromLabels(labels)

	var edges []types.Edge
	edges = append(edges, edgesFromText(raw, nodes)...)
	for _, ln := range norm {
		edges = append(edges, edgesFromText(ln, nodes)...)
	}
	if len(tsv) > 0 {
		edges = append(edges, inferEdgesByProximity(tsv, nodes)...)
	}

	notes := []string{
		fmt.Sprintf("raster: OCR extracted %d labels; relationships inferred", len(nodes)),
		fmt.Sprintf("raster: raw OCR = %q", rawSnippet),
	}

	return ParsedFile{
		Name:  filepath.Base(fp),
		Nodes: nodes,
		Edges: edges,
		Notes: notes,
	}, nil

}

func isServerish(label string) bool {
	l := strings.ToLower(strings.TrimSpace(label))
	return strings.Contains(l, "service") ||
		strings.Contains(l, "api") ||
		strings.Contains(l, "gateway") ||
		strings.Contains(l, "db") ||
		strings.Contains(l, "store") ||
		strings.Contains(l, "queue") ||
		strings.Contains(l, "cache")
}

func normalizeProtoToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, "()[]{}.,")
	s = strings.ReplaceAll(s, " ", "")
	s = normalizeWord(s)
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func detectProtoFromText(text string) string {
	lower := strings.ToLower(text)

	// quick substring checks
	if strings.Contains(lower, "grpc") {
		return "gRPC"
	}
	if strings.Contains(lower, "g rpc") {
		return "gRPC"
	}
	if strings.Contains(lower, "pubsub") || strings.Contains(lower, "pub sub") {
		return "PUBSUB"
	}
	if strings.Contains(lower, "rest") {
		return "REST"
	}

	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r))
	})

	bestProto := ""
	bestDist := 99

	targets := []struct {
		word string
		prot string
	}{
		{"grpc", "gRPC"},
		{"rest", "REST"},
		{"pubsub", "PUBSUB"},
	}

	for _, p := range parts {
		tok := normalizeProtoToken(p)
		if tok == "" {
			continue
		}
		for _, t := range targets {
			d := editDist(tok, t.word)
			if d < bestDist {
				bestDist = d
				bestProto = t.prot
			}
		}
	}

	if bestDist <= 1 && bestProto != "" {
		return bestProto
	}

	return "REST"
}

// Build approximate centers for each node using OCR lines.
func buildNodeCenters(lines []ocrLine, nodes []types.Node) map[string]nodePos {
	out := make(map[string]nodePos)

	for _, n := range nodes {
		label := strings.ToLower(n.Label)
		bestIdx := -1
		bestLen := 1 << 30

		for i, ln := range lines {
			if strings.Contains(strings.ToLower(ln.Text), label) {
				// prefer the shortest matching text (usually just the label itself)
				if len(ln.Text) < bestLen {
					bestLen = len(ln.Text)
					bestIdx = i
				}
			}
		}

		if bestIdx >= 0 {
			cx, cy := centerOf(lines[bestIdx])
			out[n.ID] = nodePos{id: n.ID, x: cx, y: cy}
		}
	}

	return out
}

// Very loose proto detection on a single line of text *without* defaulting to REST.
func protoFromTextLoose(s string) string {
	l := strings.ToLower(s)

	// because TSV words already went through normalizeWord, "grfc" etc. are now "grpc"
	if strings.Contains(l, "grpc") {
		return "gRPC"
	}
	if strings.Contains(l, "pubsub") || strings.Contains(l, "pub sub") {
		return "PUBSUB"
	}
	if strings.Contains(l, "rest") {
		return "REST"
	}
	return ""
}

// Attach protocol hints to the nearest node (Orders ⇐ (gRPC), Payment ⇐ (REST), etc.)
func assignProtoHints(lines []ocrLine, centers map[string]nodePos) map[string]string {
	out := make(map[string]string)
	if len(centers) == 0 {
		return out
	}

	// flatten centers to slice for distance calc
	var ns []nodePos
	for _, c := range centers {
		ns = append(ns, c)
	}

	for _, ln := range lines {
		proto := protoFromTextLoose(ln.Text)
		if proto == "" {
			continue
		}

		cx, cy := centerOf(ln)
		bestID := ""
		bestD := 1 << 60

		for _, np := range ns {
			dx, dy := np.x-cx, np.y-cy
			d := dx*dx + dy*dy
			if d < bestD {
				bestD = d
				bestID = np.id
			}
		}

		if bestID != "" {
			// keep first assignment; usually fine for these small diagrams
			if _, exists := out[bestID]; !exists {
				out[bestID] = proto
			}
		}
	}

	return out
}

// Build edges from layout: vertical columns (bottom→top) + horizontal rows (left→right).
func buildEdgesFromLayout(centers map[string]nodePos, protoByNode map[string]string) []types.Edge {
	if len(centers) < 2 {
		return nil
	}

	// collect into slice
	ns := make([]nodePos, 0, len(centers))
	for _, c := range centers {
		ns = append(ns, c)
	}

	const xEPS = 40 // "same column" tolerance
	const yEPS = 40 // "same row" tolerance

	edges := []types.Edge{}
	seen := map[string]bool{}

	// ---------- horizontal groups: same y → left→right ----------
	sort.Slice(ns, func(i, j int) bool { return ns[i].y < ns[j].y })

	var horizGroups [][]nodePos
	if len(ns) > 0 {
		group := []nodePos{ns[0]}
		for i := 1; i < len(ns); i++ {
			if iabs(ns[i].y-ns[i-1].y) <= yEPS {
				group = append(group, ns[i])
			} else {
				if len(group) >= 2 {
					horizGroups = append(horizGroups, group)
				}
				group = []nodePos{ns[i]}
			}
		}
		if len(group) >= 2 {
			horizGroups = append(horizGroups, group)
		}
	}

	for _, g := range horizGroups {
		// left→right
		sort.Slice(g, func(i, j int) bool { return g[i].x < g[j].x })
		for i := 0; i < len(g)-1; i++ {
			from := g[i].id
			to := g[i+1].id
			key := from + "->" + to
			if seen[key] {
				continue
			}
			proto := protoByNode[from]
			if proto == "" {
				proto = protoByNode[to]
			}
			if proto == "" {
				proto = "REST"
			}
			edges = append(edges, types.Edge{From: from, To: to, Protocol: proto})
			seen[key] = true
		}
	}

	// ---------- vertical groups: same x → bottom→top ----------
	sort.Slice(ns, func(i, j int) bool { return ns[i].x < ns[j].x })

	var vertGroups [][]nodePos
	if len(ns) > 0 {
		group := []nodePos{ns[0]}
		for i := 1; i < len(ns); i++ {
			if iabs(ns[i].x-ns[i-1].x) <= xEPS {
				group = append(group, ns[i])
			} else {
				if len(group) >= 2 {
					vertGroups = append(vertGroups, group)
				}
				group = []nodePos{ns[i]}
			}
		}
		if len(group) >= 2 {
			vertGroups = append(vertGroups, group)
		}
	}

	for _, g := range vertGroups {
		// sort by y (top→bottom); we want bottom→top edges
		sort.Slice(g, func(i, j int) bool { return g[i].y < g[j].y })
		for i := 0; i < len(g)-1; i++ {
			from := g[i+1].id // lower (bigger y)
			to := g[i].id     // upper (smaller y)
			key := from + "->" + to
			if seen[key] {
				continue
			}
			proto := protoByNode[from]
			if proto == "" {
				proto = protoByNode[to]
			}
			if proto == "" {
				proto = "REST"
			}
			edges = append(edges, types.Edge{From: from, To: to, Protocol: proto})
			seen[key] = true
		}
	}

	return edges
}

// Main proximity-based inference entrypoint used by ParseRaster.
func inferEdgesByProximity(lines []ocrLine, nodes []types.Node) []types.Edge {
	if len(nodes) < 2 || len(lines) == 0 {
		return nil
	}

	centers := buildNodeCenters(lines, nodes)
	if len(centers) < 2 {
		return nil
	}

	protoByNode := assignProtoHints(lines, centers)
	return buildEdgesFromLayout(centers, protoByNode)
}
