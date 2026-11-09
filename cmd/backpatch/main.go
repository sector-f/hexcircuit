package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/sector-f/hexgrid"
)

const (
	panelWidth       = 13.5
	panelHeight      = 19.0
	panelTopWidth    = 13.5
	panelBottomWidth = 8.5

	blue   = "#396CF8"
	orange = "#FF6A00"
)

type design struct {
	scene hexgrid.GridScene
	cells map[hexgrid.GridPosition]bool
}

type spineSegment struct {
	start hexgrid.GridPosition
	end   hexgrid.GridPosition
	path  []hexgrid.GridPosition
}

func newDesign(color string, pitch, componentRadius float64) *design {
	o := hexgrid.DefaultOptions()
	o.Width = panelWidth
	o.Height = panelHeight
	o.Orientation = "pointy"
	o.Color = color
	o.Background = "none"
	o.Grid = hexgrid.Grid{
		Pitch:       pitch,
		Radius:      pitch * 0.52,
		StrokeWidth: 0,
	}
	o.ComponentRadius = componentRadius
	o.ComponentStrokeWidth = 0.017
	o.ComponentMiddleHexagonScale = 0.64
	o.ComponentInnerHexagonScale = 0.36
	o.TraceStrokeWidth = 0.012
	o.TraceSpacing = 0.035
	o.NodeRadius = 0.022

	return &design{
		scene: hexgrid.GridScene{Options: o},
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

func (d *design) addTrace(start hexgrid.GridPosition, path []hexgrid.GridPosition, lines int) {
	if len(path) == 0 {
		return
	}
	d.scene.Traces = append(d.scene.Traces, hexgrid.GridTrace{
		Start: start,
		Path:  append([]hexgrid.GridPosition(nil), path...),
		Lines: lines,
	})
}

func add(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q + b.Q, R: a.R + b.R}
}

func subtract(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q - b.Q, R: a.R - b.R}
}

var axialDirections = []hexgrid.GridPosition{
	{Q: 0, R: -1},
	{Q: 1, R: -1},
	{Q: 1, R: 0},
	{Q: 0, R: 1},
	{Q: -1, R: 1},
	{Q: -1, R: 0},
}

// axialLine returns every cell after start through end when the two positions
// lie on one of the six straight lattice directions.
func axialLine(start, end hexgrid.GridPosition) ([]hexgrid.GridPosition, bool) {
	delta := subtract(end, start)
	for _, direction := range axialDirections {
		steps := 0
		switch {
		case direction.Q == 0:
			if delta.Q != 0 || delta.R*direction.R < 0 {
				continue
			}
			steps = delta.R / direction.R
		case direction.R == 0:
			if delta.R != 0 || delta.Q*direction.Q < 0 {
				continue
			}
			steps = delta.Q / direction.Q
		default:
			if delta.Q%direction.Q != 0 || delta.R%direction.R != 0 {
				continue
			}
			steps = delta.Q / direction.Q
			if steps != delta.R/direction.R {
				continue
			}
		}
		if steps < 1 {
			continue
		}

		path := make([]hexgrid.GridPosition, 0, steps)
		current := start
		for i := 0; i < steps; i++ {
			current = add(current, direction)
			path = append(path, current)
		}
		return path, true
	}
	return nil, false
}

// ringPositions returns the cells around a hexagonal ring in cyclic order.
// Every adjacent pair in the returned slice is a neighboring axial cell.
func ringPositions(radius int) []hexgrid.GridPosition {
	if radius < 1 {
		return nil
	}
	directions := []hexgrid.GridPosition{
		{Q: 1, R: 0},
		{Q: 0, R: 1},
		{Q: -1, R: 1},
		{Q: -1, R: 0},
		{Q: 0, R: -1},
		{Q: 1, R: -1},
	}

	current := hexgrid.GridPosition{Q: 0, R: -radius}
	positions := make([]hexgrid.GridPosition, 0, 6*radius)
	for _, direction := range directions {
		for step := 0; step < radius; step++ {
			positions = append(positions, current)
			current = add(current, direction)
		}
	}
	return positions
}

func addRing(d *design, radius, stride int, style hexgrid.ComponentStyle, radiusScale float64) []hexgrid.GridPosition {
	ring := ringPositions(radius)
	anchors := make([]hexgrid.GridPosition, 0, len(ring)/stride)
	for index := 0; index < len(ring); index += stride {
		position := ring[index]
		anchors = append(anchors, position)
		d.addComponent(position, style, radiusScale)
	}
	return anchors
}

func addRingTraces(d *design, radius, stride int, lines int) {
	ring := ringPositions(radius)
	for index := 0; index < len(ring); index += stride {
		path := make([]hexgrid.GridPosition, 0, stride)
		for offset := 1; offset <= stride; offset++ {
			path = append(path, ring[(index+offset)%len(ring)])
		}
		d.addTrace(ring[index], path, lines)
	}
}

func withinPanel(position hexgrid.GridPosition, pitch, componentRadius, extraMargin float64) bool {
	point := hexgrid.GridPositionToPoint(position, panelWidth, panelHeight, pitch, "pointy")
	if point.Y < componentRadius+extraMargin || point.Y > panelHeight-componentRadius-extraMargin {
		return false
	}
	fraction := point.Y / panelHeight
	widthAtY := panelTopWidth + (panelBottomWidth-panelTopWidth)*fraction
	halfWidth := widthAtY / 2
	xOffset := math.Abs(point.X - panelWidth/2)
	return xOffset+componentRadius+extraMargin <= halfWidth
}

func buildVerticalSpine(d *design, upward bool) []spineSegment {
	current := hexgrid.GridPosition{}
	source := current
	pending := make([]hexgrid.GridPosition, 0, 4)
	segments := make([]spineSegment, 0, 3)

	first := hexgrid.GridPosition{Q: 0, R: 1}
	second := hexgrid.GridPosition{Q: -1, R: 1}
	if upward {
		first = hexgrid.GridPosition{Q: 0, R: -1}
		second = hexgrid.GridPosition{Q: 1, R: -1}
	}

	for cycle := 1; cycle <= 6; cycle++ {
		current = add(current, first)
		pending = append(pending, current)
		current = add(current, second)
		pending = append(pending, current)

		if cycle%2 != 0 {
			continue
		}
		style := hexgrid.ComponentStyleSingle
		if cycle == 4 {
			style = hexgrid.ComponentStyleDouble
		} else if cycle == 6 {
			style = hexgrid.ComponentStyleTriple
		}
		d.addComponent(current, style, 0.82)
		d.addTrace(source, pending, 1)
		segments = append(segments, spineSegment{
			start: source,
			end:   current,
			path:  append([]hexgrid.GridPosition(nil), pending...),
		})
		source = current
		pending = pending[:0]
	}
	return segments
}

// writeLayeredSVG renders each layer through hexgrid.BuildScene and hexgrid.Render,
// then combines the generated SVG groups into one document. Layer dimensions must match.
func writeLayeredSVG(filename string, layers ...*design) error {
	if len(layers) == 0 {
		return fmt.Errorf("no SVG layers supplied for %s", filename)
	}

	var output strings.Builder
	for index, layer := range layers {
		// BuildScene consumes the GridScene lattice pitch to place every component
		// and trace. The lower-level Scene uses Grid.Pitch only for the optional
		// background lattice; disable that lattice here so the artwork follows the
		// jacket's tapered outline instead of filling a rectangular background.
		scene, err := hexgrid.BuildScene(layer.scene)
		if err != nil {
			return fmt.Errorf("build %s layer %d: %w", filename, index+1, err)
		}
		// Only render the background grid on the base (blue) layer.
		if index > 0 {
			scene.Grid.Pitch = 0
		}
		rendered := hexgrid.Render(scene)
		// Render includes an XML declaration and a generated comment before
		// the root element. Find the actual <svg> start tag rather than the
		// first '>' in the document (which belongs to <?xml ...?>).
		rootStart := strings.Index(rendered, "<svg")
		if rootStart < 0 {
			return fmt.Errorf("hexgrid returned SVG without a root element for %s layer %d", filename, index+1)
		}
		openEndRelative := strings.IndexByte(rendered[rootStart:], '>')
		closeStart := strings.LastIndex(rendered, "</svg>")
		if openEndRelative < 0 || closeStart < rootStart+openEndRelative {
			return fmt.Errorf("hexgrid returned malformed SVG for %s layer %d", filename, index+1)
		}
		openEnd := rootStart + openEndRelative

		if index == 0 {
			// Preserve the first document's XML declaration, comment and root
			// opening tag. Additional layers contribute only their child nodes.
			output.WriteString(rendered[:openEnd+1])
		}
		body := rendered[openEnd+1 : closeStart]
		if index > 0 {
			for _, id := range []string{"background-fill", "background-grid", "traces", "components", "component-labels", "terminal-pads"} {
				body = strings.ReplaceAll(body, `id="`+id+`"`, fmt.Sprintf(`id="%s-layer-%d"`, id, index+1))
			}
		}
		output.WriteString(body)
	}
	output.WriteString("</svg>\n")

	if err := os.WriteFile(filename, []byte(output.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return nil
}

// Concept 1: a compact, concentric circuit insignia with six radial spokes.
func generateHexagonalCircuitSigil() error {
	const pitch = 0.72
	base := newDesign(blue, pitch, 0.30)
	accent := newDesign(orange, pitch, 0.30)
	origin := hexgrid.GridPosition{}

	base.addComponent(origin, hexgrid.ComponentStyleTriple, 1)
	inner := addRing(base, 3, 3, hexgrid.ComponentStyleDouble, 1)
	outer := addRing(base, 7, 7, hexgrid.ComponentStyleSingle, 1)
	addRingTraces(base, 3, 3, 2)
	addRingTraces(base, 7, 7, 1)

	for index, direction := range axialDirections {
		innerPosition := hexgrid.GridPosition{Q: 3 * direction.Q, R: 3 * direction.R}
		outerPosition := hexgrid.GridPosition{Q: 7 * direction.Q, R: 7 * direction.R}
		if path, ok := axialLine(origin, innerPosition); ok {
			base.addTrace(origin, path, 1)
		}
		if path, ok := axialLine(innerPosition, outerPosition); ok {
			base.addTrace(innerPosition, path, 1+index%2)
		}

		// Three alternating orange spokes and their inner component rings.
		if index%2 == 0 {
			accent.addComponent(inner[index], hexgrid.ComponentStyleSingle, 0.58)
			accent.addComponent(outer[index], hexgrid.ComponentStyleSingle, 0.58)
			if path, ok := axialLine(innerPosition, outerPosition); ok {
				accent.addTrace(innerPosition, path, 1)
			}
		}
	}
	accent.addComponent(origin, hexgrid.ComponentStyleSingle, 0.52)

	return writeLayeredSVG("01-hexagonal-circuit-sigil.svg", base, accent)
}

func mod(value, modulus int) int {
	result := value % modulus
	if result < 0 {
		result += modulus
	}
	return result
}

func fieldHash(q, r int) int {
	return mod(q*37+r*19+q*r*7+q*q*3+r*r*5, 101)
}

// Concept 2: an irregular field whose density follows a broad diagonal sweep.
func generateAsymmetricHexField() error {
	const (
		pitch           = 0.61
		componentRadius = 0.265
	)
	base := newDesign(blue, pitch, componentRadius)
	accent := newDesign(orange, pitch, componentRadius)

	for q := -28; q <= 28; q++ {
		for r := -28; r <= 28; r++ {
			position := hexgrid.GridPosition{Q: q, R: r}
			if !withinPanel(position, pitch, componentRadius, 0.10) {
				continue
			}
			point := hexgrid.GridPositionToPoint(position, panelWidth, panelHeight, pitch, "pointy")
			t := point.Y / panelHeight
			x := point.X - panelWidth/2
			sweep := -2.1 + 4.2*t + 0.62*math.Sin(t*2*math.Pi)
			distance := math.Abs(x - sweep)
			hash := fieldHash(q, r)

			// A broken diagonal band, with occasional isolated cells outside it.
			keep := (distance < 1.95 && hash%13 < 3) ||
				(distance < 3.35 && hash%23 == 0) ||
				(distance > 3.4 && math.Abs(x) < 3.0 && hash%47 == 0)
			if !keep {
				continue
			}

			style := hexgrid.ComponentStyleSingle
			if hash%29 == 0 {
				style = hexgrid.ComponentStyleTriple
			} else if hash%7 == 0 {
				style = hexgrid.ComponentStyleDouble
			}
			base.addComponent(position, style, 1)
			if hash%17 == 0 {
				accent.addComponent(position, hexgrid.ComponentStyleSingle, 0.48)
			}
		}
	}

	// Connect selected neighboring cells. Only three of the six directions are
	// considered so every undirected edge is emitted at most once.
	fieldDirections := []hexgrid.GridPosition{
		{Q: 1, R: 0},
		{Q: 0, R: 1},
		{Q: -1, R: 1},
	}
	for _, component := range base.scene.Components {
		position := component.Position
		hash := fieldHash(position.Q, position.R)
		for directionIndex, direction := range fieldDirections {
			neighbor := add(position, direction)
			if !base.cells[neighbor] || mod(hash+directionIndex*11, 5) != 0 {
				continue
			}
			base.addTrace(position, []hexgrid.GridPosition{neighbor}, 1+hash%2)
		}
	}
	for _, component := range accent.scene.Components {
		position := component.Position
		for _, direction := range fieldDirections {
			neighbor := add(position, direction)
			if accent.cells[neighbor] {
				accent.addTrace(position, []hexgrid.GridPosition{neighbor}, 1)
			}
		}
	}

	return writeLayeredSVG("02-asymmetric-hex-field.svg", base, accent)
}

// Concept 3: a concentric transmission array with long vertical feed lines.
func generateRadialTransmissionArray() error {
	const pitch = 0.72
	base := newDesign(blue, pitch, 0.30)
	accent := newDesign(orange, pitch, 0.30)
	origin := hexgrid.GridPosition{}

	base.addComponent(origin, hexgrid.ComponentStyleTriple, 1)
	addRing(base, 3, 3, hexgrid.ComponentStyleDouble, 1)
	addRing(base, 6, 6, hexgrid.ComponentStyleSingle, 1)
	addRingTraces(base, 3, 3, 2)
	addRingTraces(base, 6, 6, 1)

	for index, direction := range axialDirections {
		inner := hexgrid.GridPosition{Q: 3 * direction.Q, R: 3 * direction.R}
		outer := hexgrid.GridPosition{Q: 6 * direction.Q, R: 6 * direction.R}
		if path, ok := axialLine(origin, inner); ok {
			base.addTrace(origin, path, 1)
		}
		if path, ok := axialLine(inner, outer); ok {
			base.addTrace(inner, path, 1+index%2)
		}
	}

	upward := buildVerticalSpine(base, true)
	downward := buildVerticalSpine(base, false)

	// Orange inner outlines emphasize the source and alternating ring nodes.
	accent.addComponent(origin, hexgrid.ComponentStyleSingle, 0.54)
	innerRing := ringPositions(3)
	outerRing := ringPositions(6)
	for index := 0; index < 6; index += 2 {
		accent.addComponent(innerRing[index*3], hexgrid.ComponentStyleSingle, 0.56)
		accent.addComponent(outerRing[index*6], hexgrid.ComponentStyleSingle, 0.56)
	}

	// Highlight the outer two sections of each vertical feed without turning the
	// long feed lines into a continuous orange stripe.
	for _, segments := range [][]spineSegment{upward, downward} {
		for _, segment := range segments[1:] {
			accent.addComponent(segment.start, hexgrid.ComponentStyleSingle, 0.50)
			accent.addComponent(segment.end, hexgrid.ComponentStyleSingle, 0.50)
			accent.addTrace(segment.start, segment.path, 1)
		}
	}

	return writeLayeredSVG("03-radial-transmission-array.svg", base, accent)
}

// Concept 4: a denser hexagonal PCB with clustered ICs, branching buses,
// parallel trace bundles, passive-component pods, and an edge-connector bank.
func generateHexagonalCircuitBoard() error {
	const (
		pitch           = 0.58
		componentRadius = 0.28
	)
	base := newDesign(blue, pitch, componentRadius)
	accent := newDesign(orange, pitch, componentRadius)

	base.scene.Options.Grid.StrokeWidth = 0.004

	for _, layer := range []*design{base, accent} {
		layer.scene.Options.ComponentStrokeWidth = 0.014
		layer.scene.Options.ComponentMiddleHexagonScale = 0.60
		layer.scene.Options.ComponentInnerHexagonScale = 0.34
		layer.scene.Options.TraceStrokeWidth = 0.010
		layer.scene.Options.TraceSpacing = 0.035
		layer.scene.Options.NodeRadius = 0.018
	}

	// Large devices form several irregular clusters rather than a single focal
	// emblem. The positions are axial coordinates in the pointy-top grid.
	topController := hexgrid.GridPosition{Q: 5, R: -13}
	northAux := hexgrid.GridPosition{Q: 5, R: -10}
	memoryLeft := hexgrid.GridPosition{Q: 0, R: -9}
	memoryRight := hexgrid.GridPosition{Q: 11, R: -8}
	leftHub := hexgrid.GridPosition{Q: -7, R: -3}
	mainCPU := hexgrid.GridPosition{Q: 1, R: 0}
	rightIO := hexgrid.GridPosition{Q: 9, R: -1}
	rightBranch := hexgrid.GridPosition{Q: 5, R: 4}
	dataBranch := hexgrid.GridPosition{Q: -5, R: 6}
	powerController := hexgrid.GridPosition{Q: -5, R: 8}
	busJunction := hexgrid.GridPosition{Q: -1, R: 10}
	lowIO := hexgrid.GridPosition{Q: -1, R: 12}
	lowerRight := hexgrid.GridPosition{Q: 2, R: 10}
	leftPort := hexgrid.GridPosition{Q: -9, R: 1}
	leftTest := hexgrid.GridPosition{Q: -6, R: 5}
	southBus := hexgrid.GridPosition{Q: -6, R: 12}
	debugPort := hexgrid.GridPosition{Q: 7, R: 2}

	base.addComponent(topController, hexgrid.ComponentStyleTriple, 0.98)
	base.addComponent(northAux, hexgrid.ComponentStyleSingle, 0.72)
	base.addComponent(memoryLeft, hexgrid.ComponentStyleDouble, 0.90)
	base.addComponent(memoryRight, hexgrid.ComponentStyleDouble, 0.90)
	base.addComponent(leftHub, hexgrid.ComponentStyleDouble, 0.90)
	base.addComponent(mainCPU, hexgrid.ComponentStyleTriple, 1.00)
	base.addComponent(rightIO, hexgrid.ComponentStyleDouble, 0.92)
	base.addComponent(rightBranch, hexgrid.ComponentStyleDouble, 0.82)
	base.addComponent(dataBranch, hexgrid.ComponentStyleTriple, 0.90)
	base.addComponent(powerController, hexgrid.ComponentStyleDouble, 0.90)
	base.addComponent(busJunction, hexgrid.ComponentStyleDouble, 0.82)
	base.addComponent(lowIO, hexgrid.ComponentStyleDouble, 0.86)
	base.addComponent(lowerRight, hexgrid.ComponentStyleDouble, 0.82)
	base.addComponent(leftPort, hexgrid.ComponentStyleSingle, 0.72)
	base.addComponent(leftTest, hexgrid.ComponentStyleSingle, 0.70)
	base.addComponent(southBus, hexgrid.ComponentStyleDouble, 0.82)
	base.addComponent(debugPort, hexgrid.ComponentStyleSingle, 0.66)

	// A bank of individual edge contacts, like a board-to-board connector.
	connectorPositions := make([]hexgrid.GridPosition, 0, 12)
	for q := -13; q <= -2; q++ {
		position := hexgrid.GridPosition{Q: q, R: 15}
		if !withinPanel(position, pitch, componentRadius, 0.10) {
			continue
		}
		connectorPositions = append(connectorPositions, position)
		base.addComponent(position, hexgrid.ComponentStyleSingle, 0.62)
	}

	type routeStep struct {
		direction hexgrid.GridPosition
		count     int
	}
	makePath := func(start hexgrid.GridPosition, steps ...routeStep) []hexgrid.GridPosition {
		current := start
		path := make([]hexgrid.GridPosition, 0, 24)
		for _, step := range steps {
			for i := 0; i < step.count; i++ {
				current = add(current, step.direction)
				path = append(path, current)
			}
		}
		return path
	}

	// Keep passive components out of the actual trace corridors. The traces
	// themselves are explicit 60-degree lattice routes; bundle widths follow
	// the circuit generator's familiar one-, two-, and three-track vocabulary.
	routeCells := make(map[hexgrid.GridPosition]bool)
	connect := func(start hexgrid.GridPosition, lines int, highlight bool, steps ...routeStep) {
		path := makePath(start, steps...)
		if len(path) == 0 {
			return
		}
		end := path[len(path)-1]
		base.addComponent(start, hexgrid.ComponentStyleSingle, 0.70)
		base.addComponent(end, hexgrid.ComponentStyleSingle, 0.70)
		base.addTrace(start, path, lines)
		routeCells[start] = true
		for _, position := range path {
			routeCells[position] = true
		}
		if highlight {
			accent.addComponent(start, hexgrid.ComponentStyleSingle, 0.52)
			accent.addComponent(end, hexgrid.ComponentStyleSingle, 0.52)
			accent.addTrace(start, path, 1)
		}
	}

	// Upper data network. The central controller fans out to two memory clusters
	// and a smaller auxiliary device. Each route turns only by 60 degrees, as in
	// the circuit package's router, and each has at least one bend.
	connect(topController, 3, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 4},
		routeStep{hexgrid.GridPosition{Q: -1, R: 0}, 1},
	)
	connect(topController, 2, true,
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 6},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 5},
	)
	connect(topController, 2, false,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 1},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 1},
	)
	connect(memoryLeft, 2, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 6},
		routeStep{hexgrid.GridPosition{Q: -1, R: 0}, 1},
	)
	connect(memoryRight, 2, true,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 5},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 2},
	)

	// Two approaches to the main processor create a broad, asymmetric backbone.
	connect(leftHub, 3, false,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 3},
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 8},
	)
	connect(rightIO, 3, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 0}, 7},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 1},
	)
	connect(mainCPU, 3, true,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 5},
		routeStep{hexgrid.GridPosition{Q: -1, R: 0}, 1},
	)

	// The left side of the processor distributes data to the power controller
	// and to a separate lower-board bus junction.
	connect(dataBranch, 2, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 1},
	)
	connect(dataBranch, 2, false,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 4},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 3},
	)
	connect(busJunction, 2, true,
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 1},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 1},
	)
	connect(powerController, 2, false,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 3},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 1},
	)

	// Dedicated power and I/O feeds terminate at separate contacts along the
	// bottom edge of the board.
	connect(southBus, 2, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 2},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
	)
	connect(powerController, 3, true,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 5},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 2},
	)
	connect(lowerRight, 2, true,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 4},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
	)
	connect(lowIO, 2, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 0}, 1},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 2},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
	)

	// A second, shorter I/O branch on the right and a small debug/test port.
	connect(rightIO, 2, true,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 4},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
	)
	connect(rightBranch, 2, false,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 3},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 3},
	)
	connect(rightBranch, 1, false,
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 1},
		routeStep{hexgrid.GridPosition{Q: 1, R: -1}, 1},
		routeStep{hexgrid.GridPosition{Q: 0, R: -1}, 1},
	)

	// The left-side expansion provides a separate chain of smaller devices
	// feeding into the lower data bus.
	connect(leftHub, 2, false,
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 2},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 2},
	)
	connect(leftPort, 2, false,
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 4},
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 3},
	)
	connect(leftTest, 1, false,
		routeStep{hexgrid.GridPosition{Q: 1, R: -1}, 1},
		routeStep{hexgrid.GridPosition{Q: 1, R: 0}, 1},
		routeStep{hexgrid.GridPosition{Q: 0, R: 1}, 1},
		routeStep{hexgrid.GridPosition{Q: -1, R: 1}, 1},
	)

	// Add deterministic passive-component pods around the major clusters. The
	// two-cell separation mirrors the circuit generator's clustered placement
	// policy; route corridors and the tapered panel boundary remain unobstructed.
	axialDistance := func(a, b hexgrid.GridPosition) int {
		dq := a.Q - b.Q
		dr := a.R - b.R
		abs := func(value int) int {
			if value < 0 {
				return -value
			}
			return value
		}
		maximum := abs(dq)
		if value := abs(dr); value > maximum {
			maximum = value
		}
		if value := abs(dq + dr); value > maximum {
			maximum = value
		}
		return maximum
	}
	clusterCenters := []hexgrid.GridPosition{
		topController, northAux, memoryLeft, memoryRight,
		leftHub, mainCPU, rightIO, rightBranch,
		dataBranch, powerController, lowIO, lowerRight,
		leftPort, leftTest, southBus, busJunction,
	}
	for clusterIndex, center := range clusterCenters {
		added := 0
		for radius := 2; radius <= 4 && added < 2; radius++ {
			for _, offset := range ringPositions(radius) {
				if added >= 2 {
					break
				}
				position := add(center, offset)
				score := mod(fieldHash(
					position.Q+clusterIndex*7,
					position.R-clusterIndex*11,
				)+clusterIndex*13, 101)
				if score%4 > 1 || routeCells[position] || base.cells[position] {
					continue
				}
				if !withinPanel(position, pitch, componentRadius, 0.10) {
					continue
				}

				tooClose := false
				for occupied := range base.cells {
					if axialDistance(position, occupied) < 2 {
						tooClose = true
						break
					}
				}
				if tooClose {
					continue
				}

				style := hexgrid.ComponentStyleSingle
				if score%13 == 0 {
					style = hexgrid.ComponentStyleTriple
				} else if score%3 == 0 {
					style = hexgrid.ComponentStyleDouble
				}
				scales := [...]float64{0.46, 0.52, 0.58, 0.64}
				radiusScale := scales[(score/4)%len(scales)]
				base.addComponent(position, style, radiusScale)
				if score%17 == 0 {
					accent.addComponent(position, hexgrid.ComponentStyleSingle, 0.32)
				}
				added++
			}
		}
	}

	// Small orange cores punctuate the main devices. A few orange connector
	// contacts reinforce the power/data-feed hierarchy without coloring the
	// entire connector row.
	for _, position := range []hexgrid.GridPosition{
		topController, mainCPU, dataBranch, powerController, rightBranch,
	} {
		accent.addComponent(position, hexgrid.ComponentStyleSingle, 0.40)
	}
	for index, position := range connectorPositions {
		if index%3 == 0 {
			accent.addComponent(position, hexgrid.ComponentStyleSingle, 0.36)
		}
	}

	return writeLayeredSVG("04-hexagonal-circuit-board.svg", base, accent)
}

func main() {
	generators := []func() error{
		generateHexagonalCircuitSigil,
		generateAsymmetricHexField,
		generateRadialTransmissionArray,
		generateHexagonalCircuitBoard,
	}
	for _, generate := range generators {
		if err := generate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("Generated %d back-patch SVG concepts.\n", len(generators))
}
