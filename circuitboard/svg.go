package circuitboard

import (
	"fmt"
	"math"
	"strings"

	hexgrid "github.com/sector-f/hexcircuit/hexgrid"
)

const svgUnitsPerInch = 1000.0

type design struct {
	scene hexgrid.GridScene
	cells map[hexgrid.GridPosition]bool
}

type routeStep struct {
	direction hexgrid.GridPosition
	count     int
}

// Generate creates an SVG document using options. It performs no file I/O and
// returns the number of base-layer components and trace bundles. The board layout
// is deterministic: the same options always produce the same SVG.
func Generate(options Options) (string, int, int, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return "", 0, 0, err
	}

	base, accent := buildBoard(o)
	svgText, err := renderLayers(o, base, accent)
	if err != nil {
		return "", 0, 0, err
	}
	return svgText, len(base.scene.Components), len(base.scene.Traces), nil
}

func buildBoard(o Options) (*design, *design) {
	base := newDesign(o, o.BaseColor, o.Background, o.ShowBackgroundGrid)
	accent := newDesign(o, o.AccentColor, "none", false)

	// These component and trace proportions preserve the appearance of the
	// original generateHexagonalCircuitBoard composition.
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

	// The connector bank is a required part of the board, not a probabilistic
	// decoration. Its axial row is calculated from the panel's bottom inset and
	// remains centered as the canvas dimensions or pitch are changed.
	connectorRow := connectorPositions(o)
	for _, position := range connectorRow {
		base.addComponent(position, hexgrid.ComponentStyleSingle, 0.62)
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

	// Route definitions are deliberately explicit. This is a composed PCB-like
	// network, rather than the circuit package's general placement and router.
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
		for _, position := range path {
			routeCells[position] = true
		}
		routeCells[start] = true
		if highlight {
			accent.addComponent(start, hexgrid.ComponentStyleSingle, 0.52)
			accent.addComponent(end, hexgrid.ComponentStyleSingle, 0.52)
			accent.addTrace(start, path, 1)
		}
	}

	// Upper data network: the controller fans out to two memory clusters and
	// a smaller auxiliary device.
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

	// The left side distributes data to the power controller and lower bus.
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

	// Dedicated power and I/O feeds terminate at separate bottom contacts.
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

	// Right-side I/O and debug branches.
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

	// The left expansion feeds the lower data bus.
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

	// Deterministic passive-component pods fill gaps around the larger devices,
	// while respecting trace corridors, the panel taper, and component spacing.
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
				if !withinPanel(o, position, o.ComponentRadius, panelClearance) {
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

	// Orange cores identify major powered devices. Selected edge contacts are
	// highlighted as power pins without turning the entire connector bank orange.
	for _, position := range []hexgrid.GridPosition{
		topController, mainCPU, dataBranch, powerController, rightBranch,
	} {
		accent.addComponent(position, hexgrid.ComponentStyleSingle, 0.40)
	}
	for index, position := range connectorRow {
		if index%3 == 0 {
			accent.addComponent(position, hexgrid.ComponentStyleSingle, 0.36)
		}
	}

	return base, accent
}

func newDesign(o Options, color, background string, showGrid bool) *design {
	gridStrokeWidth := 0.0
	if showGrid {
		gridStrokeWidth = o.GridStrokeWidth
	}
	visual := hexgrid.DefaultOptions()
	visual.Width = o.Width
	visual.Height = o.Height
	visual.Orientation = "pointy"
	visual.Color = color
	visual.Background = background
	visual.Grid = hexgrid.Grid{
		Pitch:       o.Pitch,
		Radius:      o.Pitch * 0.52,
		StrokeWidth: gridStrokeWidth,
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

func connectorPositions(o Options) []hexgrid.GridPosition {
	// For pointy-top axial coordinates, a constant R gives a horizontal row.
	// Q moves the contacts left/right along that row.
	row := int(math.Round((o.Height/2 - o.ConnectorInset) / (o.Pitch * math.Sqrt(3) / 2)))
	firstQ := int(math.Round(-float64(row)/2 - float64(o.ConnectorCount-1)/2))
	positions := make([]hexgrid.GridPosition, o.ConnectorCount)
	for index := range positions {
		positions[index] = hexgrid.GridPosition{Q: firstQ + index, R: row}
	}
	return positions
}

func fixedComponentPositions() []hexgrid.GridPosition {
	return []hexgrid.GridPosition{
		{Q: 5, R: -13}, {Q: 5, R: -10}, {Q: 0, R: -9}, {Q: 11, R: -8},
		{Q: -7, R: -3}, {Q: 1, R: 0}, {Q: 9, R: -1}, {Q: 5, R: 4},
		{Q: -5, R: 6}, {Q: -5, R: 8}, {Q: -1, R: 10}, {Q: -1, R: 12},
		{Q: 2, R: 10}, {Q: -9, R: 1}, {Q: -6, R: 5}, {Q: -6, R: 12},
		{Q: 7, R: 2},
	}
}

func withinPanel(o Options, position hexgrid.GridPosition, componentRadius, extraMargin float64) bool {
	point := hexgrid.GridPositionToPoint(position, o.Width, o.Height, o.Pitch, "pointy")
	if point.Y < componentRadius+extraMargin || point.Y > o.Height-componentRadius-extraMargin {
		return false
	}
	fraction := point.Y / o.Height
	widthAtY := o.TopWidth + (o.BottomWidth-o.TopWidth)*fraction
	halfWidth := widthAtY / 2
	xOffset := math.Abs(point.X - o.Width/2)
	return xOffset+componentRadius+extraMargin <= halfWidth
}

func axialDistance(a, b hexgrid.GridPosition) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	return maxInt(absInt(dq), absInt(dr), absInt(dq+dr))
}

func ringPositions(radius int) []hexgrid.GridPosition {
	if radius < 1 {
		return nil
	}
	directions := [...]hexgrid.GridPosition{
		{Q: 1, R: 0}, {Q: 0, R: 1}, {Q: -1, R: 1},
		{Q: -1, R: 0}, {Q: 0, R: -1}, {Q: 1, R: -1},
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

func add(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q + b.Q, R: a.R + b.R}
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
		// BuildScene needs the lattice pitch to convert axial positions. Render
		// uses that same field only for the optional background lattice, so zero
		// it on every layer except the base layer when the grid is enabled.
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
			// Preserve the XML declaration, generated comment, and root SVG tag.
			output.WriteString(rendered[:openEnd+1])
			output.WriteString(panelClipPath(o))
			output.WriteString(`<g clip-path="url(#board-panel-clip)">`)
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
	output.WriteString("</g></svg>\n")
	return output.String(), nil
}

func panelClipPath(o Options) string {
	leftTop := (o.Width - o.TopWidth) / 2 * svgUnitsPerInch
	rightTop := (o.Width + o.TopWidth) / 2 * svgUnitsPerInch
	leftBottom := (o.Width - o.BottomWidth) / 2 * svgUnitsPerInch
	rightBottom := (o.Width + o.BottomWidth) / 2 * svgUnitsPerInch
	bottom := o.Height * svgUnitsPerInch
	return fmt.Sprintf(
		`<defs><clipPath id="board-panel-clip" clipPathUnits="userSpaceOnUse"><path d="M %.3f 0 H %.3f L %.3f %.3f H %.3f Z"/></clipPath></defs>`,
		leftTop, rightTop, rightBottom, bottom, leftBottom,
	)
}
