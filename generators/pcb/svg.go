package pcb

import (
	"container/heap"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/sector-f/hexgrid"
)

const (
	svgUnitsPerInch      = 1000.0
	minComponentDistance = 2
)

var neighborSteps = [...]hexgrid.GridPosition{
	{Q: 1, R: 0}, {Q: 0, R: 1}, {Q: -1, R: 1},
	{Q: -1, R: 0}, {Q: 0, R: -1}, {Q: 1, R: -1},
}

type design struct {
	scene hexgrid.GridScene
	cells map[hexgrid.GridPosition]bool
}

type routeState struct {
	position  hexgrid.GridPosition
	direction int
}

type routeItem struct {
	state    routeState
	cost     float64
	priority float64
	serial   int
}

type routeQueue []routeItem

func (q routeQueue) Len() int { return len(q) }
func (q routeQueue) Less(i, j int) bool {
	if q[i].priority == q[j].priority {
		if q[i].cost == q[j].cost {
			return q[i].serial < q[j].serial
		}
		return q[i].cost < q[j].cost
	}
	return q[i].priority < q[j].priority
}
func (q routeQueue) Swap(i, j int)   { q[i], q[j] = q[j], q[i] }
func (q *routeQueue) Push(value any) { *q = append(*q, value.(routeItem)) }
func (q *routeQueue) Pop() any {
	old := *q
	last := old[len(old)-1]
	*q = old[:len(old)-1]
	return last
}

// Generate creates a randomized PCB-inspired SVG document using options. It
// returns the SVG text, the number of components (including bottom contacts),
// the number of trace bundles, and an error. Set SeedSet to true for reproducible
// output; otherwise every call uses a fresh seed. Generate performs no file I/O.
func Generate(options Options) (string, int, int, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return "", 0, 0, err
	}

	seed := o.Seed
	if !o.SeedSet {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	base, accent, err := generateLayout(o, rng)
	if err != nil {
		return "", 0, 0, err
	}
	svg, err := renderLayers(o, base, accent)
	if err != nil {
		return "", 0, 0, err
	}
	return svg, len(base.scene.Components), len(base.scene.Traces), nil
}

func generateLayout(o Options, rng *rand.Rand) (*design, *design, error) {
	base := newDesign(o, o.BaseColor, o.Background, o.ShowBackgroundGrid)
	accent := newDesign(o, o.AccentColor, "none", false)

	connectors, err := connectorPositions(o)
	if err != nil {
		return nil, nil, err
	}
	for _, position := range connectors {
		base.addComponent(position, hexgrid.ComponentStyleSingle, 0.62)
	}

	// Restrict both components and routing to lattice cells whose equivalent
	// background hexagon (and the largest component/trace envelope) fits fully
	// inside the canvas. This leaves a consistent margin around the board.
	cellClearance := edgeCellClearance(o)
	allCells := make(map[hexgrid.GridPosition]bool)
	componentCells := make([]hexgrid.GridPosition, 0)
	for _, cell := range hexgrid.GridCells(o.Width, o.Height, o.Pitch, "pointy", -cellClearance) {
		position := hexgrid.GridPosition{Q: cell.Q, R: cell.R}
		if !hexagonFullyInsideCanvas(cell.Center, cellClearance, 0, o) {
			continue
		}
		allCells[position] = true
		nearConnector := false
		for _, connector := range connectors {
			if axialDistance(position, connector) < minComponentDistance {
				nearConnector = true
				break
			}
		}
		if nearConnector {
			continue
		}
		componentCells = append(componentCells, position)
	}
	if len(componentCells) < o.ComponentCount+o.ClusterCount {
		return nil, nil, fmt.Errorf("canvas is too small for %d components and %d clusters at pitch %.3f", o.ComponentCount, o.ClusterCount, o.Pitch)
	}

	anchors := chooseClusterAnchors(componentCells, o.ClusterCount, o, rng)
	if len(anchors) != o.ClusterCount {
		return nil, nil, fmt.Errorf("could not place %d component clusters; increase the canvas dimensions or reduce the pitch", o.ClusterCount)
	}
	for _, anchor := range anchors {
		style := randomMajorStyle(rng)
		scale := 0.84 + rng.Float64()*0.15
		base.addComponent(anchor, style, scale)
	}

	// First route the main devices and a handful of power feeds to the connector
	// bank. Passive parts are placed afterward, leaving irregular open trace lanes.
	traceCells := make(map[hexgrid.GridPosition]bool)
	traceEdges := make(map[string]bool)
	connectionCount := addClusterNetwork(base, accent, anchors, connectors, allCells, traceCells, traceEdges, o, rng)
	if connectionCount == 0 {
		return nil, nil, fmt.Errorf("could not route any connections; increase canvas dimensions or reduce component density")
	}

	// The remaining components are intentionally not all connected. Their loose,
	// clustered placement creates the through-hole / unpopulated-pad texture.
	if err := addPassiveComponents(base, anchors, componentCells, traceCells, o, rng); err != nil {
		return nil, nil, err
	}

	// Give a few powered devices and connector contacts orange inner cores even
	// when they are not the endpoints of a highlighted route.
	for _, anchor := range anchors {
		if rng.Float64() < 0.55 {
			accent.addComponent(anchor, hexgrid.ComponentStyleSingle, 0.42)
		}
	}
	for index, connector := range connectors {
		if index%3 == 0 || rng.Float64() < 0.12 {
			accent.addComponent(connector, hexgrid.ComponentStyleSingle, 0.28)
		}
	}

	return base, accent, nil
}

func newDesign(o Options, color, background string, showGrid bool) *design {
	gridStroke := 0.0
	if showGrid {
		gridStroke = o.GridStrokeWidth
	}
	visual := hexgrid.DefaultOptions()
	visual.Width = o.Width
	visual.Height = o.Height
	visual.Orientation = "pointy"
	visual.Color = color
	visual.Background = background
	visual.Grid = hexgrid.Grid{
		Pitch:       o.Pitch,
		Radius:      o.GridRadius,
		StrokeWidth: gridStroke,
		OmitPartial: o.OmitPartialGridHexagons,
	}
	visual.ComponentRadius = o.ComponentRadius
	visual.ComponentStrokeWidth = o.ComponentStrokeWidth
	visual.ComponentMiddleHexagonScale = o.ComponentMiddleHexagonScale
	visual.ComponentInnerHexagonScale = o.ComponentInnerHexagonScale
	visual.TraceStrokeWidth = o.TraceStrokeWidth
	visual.TraceSpacing = o.TraceSpacing
	visual.NodeRadius = o.NodeRadius
	return &design{
		scene: hexgrid.GridScene{Options: visual},
		cells: make(map[hexgrid.GridPosition]bool),
	}
}

func (d *design) addComponent(position hexgrid.GridPosition, style hexgrid.ComponentStyle, radiusScale float64) {
	if d.cells[position] {
		return
	}
	d.cells[position] = true
	d.scene.Components = append(d.scene.Components, hexgrid.GridComponent{
		Position:    position,
		Style:       style,
		RadiusScale: radiusScale,
	})
}

func (d *design) addTrace(start hexgrid.GridPosition, path []hexgrid.GridPosition, lines int, hideTerminal bool) {
	if len(path) == 0 {
		return
	}
	d.scene.Traces = append(d.scene.Traces, hexgrid.GridTrace{
		Start:        start,
		Path:         append([]hexgrid.GridPosition(nil), path...),
		Lines:        lines,
		HideTerminal: hideTerminal,
	})
}

func chooseClusterAnchors(candidates []hexgrid.GridPosition, count int, o Options, rng *rand.Rand) []hexgrid.GridPosition {
	if len(candidates) == 0 || count < 1 {
		return nil
	}
	// Stratified random sampling gives the board broad spatial coverage without
	// fixing component coordinates. Each seed is random within its own sector.
	columns := int(math.Ceil(math.Sqrt(float64(count) * o.Width / o.Height)))
	if columns < 1 {
		columns = 1
	}
	rows := int(math.Ceil(float64(count) / float64(columns)))
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, candidate := range candidates {
		y := point(candidate, o).Y
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	buckets := make([][]hexgrid.GridPosition, columns*rows)
	for _, candidate := range candidates {
		p := point(candidate, o)
		column := int(math.Floor((p.X / o.Width) * float64(columns)))
		row := int(math.Floor(((p.Y - minY) / math.Max(maxY-minY, 1e-9)) * float64(rows)))
		if column >= columns {
			column = columns - 1
		}
		if row >= rows {
			row = rows - 1
		}
		buckets[row*columns+column] = append(buckets[row*columns+column], candidate)
	}
	sectors := make([]int, len(buckets))
	for i := range sectors {
		sectors[i] = i
	}
	rng.Shuffle(len(sectors), func(i, j int) { sectors[i], sectors[j] = sectors[j], sectors[i] })
	anchors := make([]hexgrid.GridPosition, 0, count)
	for _, sector := range sectors {
		bucket := buckets[sector]
		if len(bucket) == 0 {
			continue
		}
		rng.Shuffle(len(bucket), func(i, j int) { bucket[i], bucket[j] = bucket[j], bucket[i] })
		for _, candidate := range bucket {
			separated := true
			for _, anchor := range anchors {
				if axialDistance(candidate, anchor) < 3 {
					separated = false
					break
				}
			}
			if separated {
				anchors = append(anchors, candidate)
				break
			}
		}
		if len(anchors) == count {
			return anchors
		}
	}
	// If unusual dimensions leave sectors empty, fall back to shuffled random
	// sampling while retaining a minimum distance between cluster centers.
	pool := append([]hexgrid.GridPosition(nil), candidates...)
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for _, minimumDistance := range []int{3, 2} {
		for _, candidate := range pool {
			if containsPosition(anchors, candidate) {
				continue
			}
			separated := true
			for _, anchor := range anchors {
				if axialDistance(candidate, anchor) < minimumDistance {
					separated = false
					break
				}
			}
			if separated {
				anchors = append(anchors, candidate)
			}
			if len(anchors) == count {
				return anchors
			}
		}
	}
	return nil
}

func containsPosition(positions []hexgrid.GridPosition, target hexgrid.GridPosition) bool {
	for _, position := range positions {
		if position == target {
			return true
		}
	}
	return false
}

func addClusterNetwork(base, accent *design, anchors, connectors []hexgrid.GridPosition,
	allCells, traceCells map[hexgrid.GridPosition]bool, traceEdges map[string]bool, o Options, rng *rand.Rand) int {
	blocked := make(map[hexgrid.GridPosition]bool, len(base.cells))
	for position := range base.cells {
		blocked[position] = true
	}
	pairs := make([][2]hexgrid.GridPosition, 0, o.Connections+len(connectors))

	// A randomized spanning tree gives every major cluster at least one path into
	// the rest of the board; extra edges create loops and secondary branches.
	for index := 1; index < len(anchors); index++ {
		target := rng.Intn(index)
		pairs = append(pairs, [2]hexgrid.GridPosition{anchors[index], anchors[target]})
	}
	used := make(map[string]bool)
	markPair := func(a, b hexgrid.GridPosition) {
		used[pairKey(a, b)] = true
	}
	for _, pair := range pairs {
		markPair(pair[0], pair[1])
	}

	feedCount := minInt(3, maxInt(2, o.Connections/6))
	if feedCount > len(connectors) {
		feedCount = len(connectors)
	}
	for i := 0; i < feedCount; i++ {
		source := anchors[rng.Intn(len(anchors))]
		destination := connectors[(i*len(connectors))/feedCount]
		pairs = append(pairs, [2]hexgrid.GridPosition{destination, source})
		markPair(destination, source)
	}

	// Add random cross-links and additional connector feeds until the requested
	// number of trace bundles is reached or the candidate graph is exhausted.
	for attempts := 0; len(pairs) < o.Connections && attempts < o.Connections*30; attempts++ {
		var a, b hexgrid.GridPosition
		if rng.Intn(4) == 0 {
			a = connectors[rng.Intn(len(connectors))]
			b = anchors[rng.Intn(len(anchors))]
		} else {
			a = anchors[rng.Intn(len(anchors))]
			b = anchors[rng.Intn(len(anchors))]
		}
		if a == b || used[pairKey(a, b)] {
			continue
		}
		markPair(a, b)
		pairs = append(pairs, [2]hexgrid.GridPosition{a, b})
	}
	if len(pairs) > o.Connections {
		pairs = pairs[:o.Connections]
	}

	// Route connector feeds before the other links. This reserves their required
	// northeast/northwest exits before unrelated routes can occupy those cells.
	orderedPairs := make([][2]hexgrid.GridPosition, 0, len(pairs))
	for _, pair := range pairs {
		if isPowerFeed(pair[0], connectors) || isPowerFeed(pair[1], connectors) {
			orderedPairs = append(orderedPairs, pair)
		}
	}
	for _, pair := range pairs {
		if !isPowerFeed(pair[0], connectors) && !isPowerFeed(pair[1], connectors) {
			orderedPairs = append(orderedPairs, pair)
		}
	}

	// Keep the search graph bounded to the drawable canvas. A* uses a modest bend
	// penalty, but previously used cells and edges are hard obstacles: routes may
	// share a component endpoint, but they cannot reuse a segment or cross at an
	// intermediate lattice cell.
	for _, pair := range orderedPairs {
		start, end := pair[0], pair[1]
		// Traces attached to the bottom connector bank are represented as
		// originating at the connector, so their first segment can be constrained
		// to leave toward the board interior (northeast or northwest).
		if isPowerFeed(end, connectors) && !isPowerFeed(start, connectors) {
			start, end = end, start
		}
		connectorExit := isPowerFeed(start, connectors)
		path := findRoute(start, end, allCells, blocked, traceCells, traceEdges, rng, connectorExit)
		if len(path) == 0 {
			continue
		}
		lines := randomBundleSize(o.MaxParallel, rng)
		base.addTrace(start, path, lines, true)
		previous := start
		for _, position := range path {
			traceCells[position] = true
			traceEdges[pairKey(previous, position)] = true
			previous = position
		}
		if connectorExit || rng.Float64() < 0.28 {
			addAccentTrace(accent, start, path)
		}
	}
	return len(base.scene.Traces)
}

func addAccentTrace(accent *design, start hexgrid.GridPosition, path []hexgrid.GridPosition) {
	end := path[len(path)-1]
	accent.addComponent(start, hexgrid.ComponentStyleSingle, 0.42)
	accent.addComponent(end, hexgrid.ComponentStyleSingle, 0.28)
	accent.addTrace(start, path, 1, false)
}

func isPowerFeed(position hexgrid.GridPosition, connectors []hexgrid.GridPosition) bool {
	for _, connector := range connectors {
		if position == connector {
			return true
		}
	}
	return false
}

func findRoute(start, end hexgrid.GridPosition,
	allowed, blocked, used map[hexgrid.GridPosition]bool,
	usedEdges map[string]bool,
	rng *rand.Rand,
	connectorExit bool,
) []hexgrid.GridPosition {
	if start == end || !allowed[start] || !allowed[end] {
		return nil
	}
	// Each search receives a fresh randomized direction order. The A* heuristic is
	// axial distance; turns and already-routed cells slightly affect the path score.
	directions := append([]hexgrid.GridPosition(nil), neighborSteps[:]...)
	rng.Shuffle(len(directions), func(i, j int) { directions[i], directions[j] = directions[j], directions[i] })
	startState := routeState{position: start, direction: -1}
	open := &routeQueue{{state: startState, priority: float64(axialDistance(start, end))}}
	heap.Init(open)
	costs := map[routeState]float64{startState: 0}
	parents := make(map[routeState]routeState)
	serial := 0
	var goal routeState
	found := false

	for open.Len() > 0 {
		current := heap.Pop(open).(routeItem)
		known, exists := costs[current.state]
		if !exists || current.cost > known+1e-9 {
			continue
		}
		if current.state.position == end {
			goal = current.state
			found = true
			break
		}
		for directionIndex, direction := range directions {
			if connectorExit && current.state.position == start &&
				!isConnectorExitDirection(direction) {
				continue
			}
			next := add(current.state.position, direction)
			edge := pairKey(current.state.position, next)
			if !allowed[next] || (blocked[next] && next != end) {
				continue
			}
			// Reaching an already-used component is allowed when it is this route's
			// destination (a legitimate junction). Reusing an intermediate cell or
			// any existing edge is not: on the triangular axial lattice, distinct
			// edges can only cross by sharing a vertex or overlapping an edge.
			if (used[next] && next != end) || usedEdges[edge] {
				continue
			}
			stepCost := 1.0
			if current.state.direction >= 0 && current.state.direction != directionIndex {
				stepCost += 0.18
			}
			nextState := routeState{position: next, direction: directionIndex}
			nextCost := current.cost + stepCost
			if previous, ok := costs[nextState]; ok && previous <= nextCost+1e-9 {
				continue
			}
			costs[nextState] = nextCost
			parents[nextState] = current.state
			serial++
			priority := nextCost + float64(axialDistance(next, end))
			heap.Push(open, routeItem{state: nextState, cost: nextCost, priority: priority, serial: serial})
		}
	}
	if !found {
		return nil
	}
	path := make([]hexgrid.GridPosition, 0, axialDistance(start, end)+4)
	for state := goal; state != startState; state = parents[state] {
		path = append(path, state.position)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func addPassiveComponents(base *design, anchors, candidates []hexgrid.GridPosition,
	traceCells map[hexgrid.GridPosition]bool, o Options, rng *rand.Rand) error {
	remaining := o.ComponentCount - len(anchors)
	if remaining <= 0 {
		return nil
	}
	valid := make(map[hexgrid.GridPosition]bool, len(candidates))
	for _, position := range candidates {
		valid[position] = true
	}

	// Build one nearby candidate pool per cluster, then fill the clusters in
	// rounds. This produces recognizable local groups instead of sprinkling all
	// small devices uniformly across the entire board.
	pools := make([][]hexgrid.GridPosition, len(anchors))
	for index, anchor := range anchors {
		for radius := 1; radius <= 4; radius++ {
			for _, offset := range ringPositions(radius) {
				candidate := add(anchor, offset)
				if valid[candidate] && !base.cells[candidate] && !traceCells[candidate] {
					pools[index] = append(pools[index], candidate)
				}
			}
		}
		rng.Shuffle(len(pools[index]), func(i, j int) {
			pools[index][i], pools[index][j] = pools[index][j], pools[index][i]
		})
	}

	addCandidate := func(candidate hexgrid.GridPosition) bool {
		if remaining == 0 || base.cells[candidate] || traceCells[candidate] {
			return false
		}
		for occupied := range base.cells {
			if axialDistance(candidate, occupied) < minComponentDistance {
				return false
			}
		}
		style := randomPassiveStyle(rng)
		scale := 0.40 + rng.Float64()*0.30
		base.addComponent(candidate, style, scale)
		remaining--
		return true
	}

	indices := make([]int, len(pools))
	for remaining > 0 {
		progress := false
		for poolIndex, pool := range pools {
			for indices[poolIndex] < len(pool) {
				candidate := pool[indices[poolIndex]]
				indices[poolIndex]++
				if addCandidate(candidate) {
					progress = true
					break
				}
			}
			if remaining == 0 {
				break
			}
		}
		if !progress {
			break
		}
	}

	if remaining > 0 {
		fallback := append([]hexgrid.GridPosition(nil), candidates...)
		rng.Shuffle(len(fallback), func(i, j int) { fallback[i], fallback[j] = fallback[j], fallback[i] })
		for _, candidate := range fallback {
			addCandidate(candidate)
			if remaining == 0 {
				break
			}
		}
	}
	if remaining > 0 {
		return fmt.Errorf("placed %d of %d requested non-connector components; reduce ComponentCount or increase canvas dimensions", o.ComponentCount-remaining, o.ComponentCount)
	}
	return nil
}

func ringPositions(radius int) []hexgrid.GridPosition {
	if radius < 1 {
		return nil
	}
	current := hexgrid.GridPosition{Q: 0, R: -radius}
	positions := make([]hexgrid.GridPosition, 0, 6*radius)
	for _, direction := range neighborSteps {
		for step := 0; step < radius; step++ {
			positions = append(positions, current)
			current = add(current, direction)
		}
	}
	return positions
}

func randomMajorStyle(rng *rand.Rand) hexgrid.ComponentStyle {
	value := rng.Intn(100)
	if value < 18 {
		return hexgrid.ComponentStyleSingle
	}
	if value < 74 {
		return hexgrid.ComponentStyleDouble
	}
	return hexgrid.ComponentStyleTriple
}

func randomPassiveStyle(rng *rand.Rand) hexgrid.ComponentStyle {
	value := rng.Intn(100)
	if value < 78 {
		return hexgrid.ComponentStyleSingle
	}
	if value < 97 {
		return hexgrid.ComponentStyleDouble
	}
	return hexgrid.ComponentStyleTriple
}

func randomBundleSize(maximum int, rng *rand.Rand) int {
	if maximum == 1 {
		return 1
	}
	if maximum == 2 {
		if rng.Float64() < 0.38 {
			return 1
		}
		return 2
	}
	value := rng.Float64()
	if value < 0.18 {
		return 1
	}
	if value < 0.57 {
		return 2
	}
	return 3
}

func connectorPositions(o Options) ([]hexgrid.GridPosition, error) {
	// Pointy-top axial coordinates have a horizontal row at constant R. Start
	// with the nearest row to the requested bottom inset and move inward only if
	// the connector outlines would cross the canvas edge.
	verticalStep := math.Sqrt(3) * o.Pitch / 2
	connectorRadius := o.ComponentRadius * 0.62
	connectorInset := math.Max(o.ConnectorInset, edgeCellClearance(o))
	desiredRow := int(math.Round((o.Height/2 - connectorInset) / verticalStep))
	for row := desiredRow; row >= 0; row-- {
		firstQ := int(math.Round(-float64(row)/2 - float64(o.ConnectorCount-1)/2))
		positions := make([]hexgrid.GridPosition, o.ConnectorCount)
		fits := true
		for index := range positions {
			position := hexgrid.GridPosition{Q: firstQ + index, R: row}
			center := point(position, o)
			if !hexagonFullyInsideCanvas(center, edgeCellClearance(o), 0, o) ||
				center.X < connectorRadius+o.ComponentStrokeWidth/2 ||
				center.X > o.Width-connectorRadius-o.ComponentStrokeWidth/2 ||
				center.Y < connectorRadius+o.ComponentStrokeWidth/2 ||
				center.Y > o.Height-connectorRadius-o.ComponentStrokeWidth/2 {
				fits = false
				break
			}
			positions[index] = position
		}
		if fits {
			return positions, nil
		}
	}
	return nil, fmt.Errorf("bottom connector row does not fit the canvas; reduce ConnectorCount or increase canvas dimensions")
}

func renderLayers(o Options, layers ...*design) (string, error) {
	if len(layers) == 0 {
		return "", fmt.Errorf("no circuit-board layers supplied")
	}
	var output strings.Builder
	for index, layer := range layers {
		scene, err := hexgrid.BuildScene(layer.scene)
		if err != nil {
			return "", fmt.Errorf("build circuit-board layer %d: %w", index+1, err)
		}
		if index > 0 || !o.ShowBackgroundGrid {
			scene.Grid.Pitch = 0
		}
		rendered := hexgrid.Render(scene)
		rootStart := strings.Index(rendered, "<svg")
		if rootStart < 0 {
			return "", fmt.Errorf("hexgrid returned SVG without a root element in layer %d", index+1)
		}
		openEndRelative := strings.IndexByte(rendered[rootStart:], '>')
		closeStart := strings.LastIndex(rendered, "</svg>")
		if openEndRelative < 0 || closeStart < rootStart+openEndRelative {
			return "", fmt.Errorf("hexgrid returned malformed SVG in layer %d", index+1)
		}
		openEnd := rootStart + openEndRelative
		if index == 0 {
			output.WriteString(rendered[:openEnd+1])
		}
		body := rendered[openEnd+1 : closeStart]
		if index > 0 {
			for _, id := range []string{
				"background-fill", "background-grid", "traces", "components",
				"component-labels", "terminal-pads",
			} {
				body = strings.ReplaceAll(body, `id="`+id+`"`, fmt.Sprintf(`id="%s-layer-%d"`, id, index+1))
			}
		}
		output.WriteString(body)
	}
	output.WriteString("</svg>\n")
	return output.String(), nil
}

// edgeCellClearance is the circumradius of the largest hypothetical hexagon
// that must fit around every usable lattice center. It covers background-grid
// cells, full-size components, and the outer envelope of the largest trace bundle.
func edgeCellClearance(o Options) float64 {
	gridEnvelope := o.GridRadius + o.GridStrokeWidth/2
	componentEnvelope := o.ComponentRadius + o.ComponentStrokeWidth/2
	traceEnvelope := o.NodeRadius + o.TraceStrokeWidth/2 + float64(o.MaxParallel-1)*o.TraceSpacing/2
	return math.Max(gridEnvelope, math.Max(componentEnvelope, traceEnvelope))
}

// hexagonFullyInsideCanvas mirrors hexgrid's partial-grid-cell test. Using the
// exported HexVertices geometry keeps placement policy aligned with rendering.
func hexagonFullyInsideCanvas(center hexgrid.Point, radius, strokeWidth float64, o Options) bool {
	margin := strokeWidth / 2
	for _, vertex := range hexgrid.HexVertices(center.X, center.Y, radius, "pointy") {
		if vertex.X < margin || vertex.X > o.Width-margin ||
			vertex.Y < margin || vertex.Y > o.Height-margin {
			return false
		}
	}
	return true
}

// isConnectorExitDirection accepts only the two northward axial moves from a
// pointy-top connector row: northeast (1,-1) and northwest (0,-1).
func isConnectorExitDirection(direction hexgrid.GridPosition) bool {
	return direction == (hexgrid.GridPosition{Q: 1, R: -1}) ||
		direction == (hexgrid.GridPosition{Q: 0, R: -1})
}

func point(position hexgrid.GridPosition, o Options) hexgrid.Point {
	return hexgrid.GridPositionToPoint(position, o.Width, o.Height, o.Pitch, "pointy")
}

func add(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q + b.Q, R: a.R + b.R}
}

func axialDistance(a, b hexgrid.GridPosition) int {
	dq, dr := a.Q-b.Q, a.R-b.R
	return maxInt(absInt(dq), absInt(dr), absInt(dq+dr))
}

func pairKey(a, b hexgrid.GridPosition) string {
	if a.Q > b.Q || (a.Q == b.Q && a.R > b.R) {
		a, b = b, a
	}
	return fmt.Sprintf("%d,%d:%d,%d", a.Q, a.R, b.Q, b.R)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(values ...int) int {
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
