package hexgrid

import (
	"fmt"
	"math"
)

// GridPosition identifies a cell in the hexagonal lattice using axial coordinates.
// For pointy-top layouts, positive Q points east and positive R points southeast.
// The origin (0, 0) is at the center of the canvas.
type GridPosition struct {
	Q, R int
}

// ComponentStyle selects how many concentric hexagons make up a component.
// The outer hexagon always uses Options.ComponentRadius; inner hexagons use the
// configurable scale factors in Options.
type ComponentStyle uint8

const (
	ComponentStyleSingle ComponentStyle = iota + 1
	ComponentStyleDouble
	ComponentStyleTriple
)

// HexagonCount reports the number of outlines rendered by the style. It returns
// zero for an invalid style.
func (style ComponentStyle) HexagonCount() int {
	switch style {
	case ComponentStyleSingle:
		return 1
	case ComponentStyleDouble:
		return 2
	case ComponentStyleTriple:
		return 3
	default:
		return 0
	}
}

// GridComponent places a component on one lattice cell. RadiusScale multiplies
// Options.ComponentRadius; a zero value means 1 (the configured base radius).
type GridComponent struct {
	Position    GridPosition
	Style       ComponentStyle
	RadiusScale float64
}

// GridTrace describes a bundle that starts at a component and follows a sequence
// of straight grid-axis segments. Path contains one or more waypoints after Start;
// intermediate cells may be omitted when a segment continues along a single grid
// axis. The final position may identify a component or a free terminal location.
// HideTerminal requires the final position to contain a component and clips each
// track at that component's outer boundary instead of drawing a terminal circle.
type GridTrace struct {
	Start        GridPosition
	Path         []GridPosition
	Lines        int
	HideTerminal bool
}

// Options contains the physical dimensions and styling used to turn a grid layout
// into SVG geometry. All distances are in inches, except the scale factors, which
// are unitless. ComponentRadius is the outer radius for a component before applying
// a GridComponent's optional RadiusScale.
type Options struct {
	Width, Height float64
	Orientation   string
	Color         string
	Background    string

	Grid Grid

	ComponentRadius             float64
	ComponentStrokeWidth        float64
	ComponentMiddleHexagonScale float64
	ComponentInnerHexagonScale  float64
	TraceStrokeWidth            float64
	// TraceSpacing is the perpendicular centerline distance between bundle lines.
	TraceSpacing float64
	// NodeRadius is the radius of each terminal pad at a trace's destination.
	NodeRadius float64
}

// DefaultOptions returns a useful starting configuration for a manually-authored
// grid layout. Callers can change any dimension without changing layout coordinates.
func DefaultOptions() Options {
	return Options{
		Width:                       10,
		Height:                      8,
		Orientation:                 "pointy",
		Color:                       "#C7834A",
		Background:                  "none",
		Grid:                        Grid{Pitch: 0.62, Radius: 0.33255, StrokeWidth: 0.004},
		ComponentRadius:             0.40,
		ComponentStrokeWidth:        0.012,
		ComponentMiddleHexagonScale: largeInnerDetailScale,
		ComponentInnerHexagonScale:  smallInnerDetailScale,
		TraceStrokeWidth:            0.008,
		TraceSpacing:                0.035,
		NodeRadius:                  0.014,
	}
}

// GridScene is the high-level, manually-authored representation of a hexagonal
// illustration. Components and traces are positioned in grid coordinates; Options
// supplies all physical dimensions and visual settings.
type GridScene struct {
	Options    Options
	Components []GridComponent
	Traces     []GridTrace
}

// BuildScene converts a grid-based layout into the renderer's inch-based Scene.
// It validates grid positions, component styles, trace paths, and bundle dimensions.
// The returned Scene can also be passed to the existing Render and label-placement APIs.
func BuildScene(layout GridScene) (Scene, error) {
	o := layout.Options
	if err := validateOptions(o); err != nil {
		return Scene{}, err
	}

	baseAngle := orientationBaseAngle(o.Orientation)
	visual := Scene{
		Width:                       o.Width,
		Height:                      o.Height,
		Orientation:                 o.Orientation,
		Color:                       o.Color,
		Background:                  o.Background,
		Grid:                        o.Grid,
		ComponentStrokeWidth:        o.ComponentStrokeWidth,
		ComponentMiddleHexagonScale: o.ComponentMiddleHexagonScale,
		ComponentInnerHexagonScale:  o.ComponentInnerHexagonScale,
		TraceStrokeWidth:            o.TraceStrokeWidth,
		NodeRadius:                  o.NodeRadius,
	}

	componentsByPosition := make(map[GridPosition]Component, len(layout.Components))
	for i, component := range layout.Components {
		if component.Style.HexagonCount() == 0 {
			return Scene{}, fmt.Errorf("component %d has invalid style %d", i, component.Style)
		}
		radiusScale := component.RadiusScale
		if radiusScale == 0 {
			radiusScale = 1
		}
		if !isFinite(radiusScale) || radiusScale <= 0 {
			return Scene{}, fmt.Errorf("component %d radius scale must be finite and greater than zero", i)
		}
		if _, exists := componentsByPosition[component.Position]; exists {
			return Scene{}, fmt.Errorf("multiple components occupy grid position (%d, %d)", component.Position.Q, component.Position.R)
		}

		center := GridPositionToPoint(component.Position, o.Width, o.Height, o.Grid.Pitch, o.Orientation)
		rendered := Component{
			Index:  i,
			X:      center.X,
			Y:      center.Y,
			Radius: o.ComponentRadius * radiusScale,
			Style:  component.Style,
		}
		componentsByPosition[component.Position] = rendered
		visual.Components = append(visual.Components, rendered)
	}

	for traceIndex, trace := range layout.Traces {
		if trace.Lines < 1 {
			return Scene{}, fmt.Errorf("trace %d must contain at least one line", traceIndex)
		}
		if trace.Lines > 1 && o.TraceSpacing == 0 {
			return Scene{}, fmt.Errorf("trace %d has multiple lines but trace spacing is zero", traceIndex)
		}
		if len(trace.Path) == 0 {
			return Scene{}, fmt.Errorf("trace %d must contain at least one path position", traceIndex)
		}
		source, found := componentsByPosition[trace.Start]
		if !found {
			return Scene{}, fmt.Errorf("trace %d starts at grid position (%d, %d), which has no component", traceIndex, trace.Start.Q, trace.Start.R)
		}
		destinationPosition := trace.Path[len(trace.Path)-1]
		destination, destinationFound := componentsByPosition[destinationPosition]
		if trace.HideTerminal && !destinationFound {
			return Scene{}, fmt.Errorf("trace %d hides its terminal but ends at grid position (%d, %d), which has no component", traceIndex, destinationPosition.Q, destinationPosition.R)
		}

		positions := make([]GridPosition, 1, len(trace.Path)+1)
		positions[0] = trace.Start
		positions = append(positions, trace.Path...)
		points := make([]Point, len(positions))
		for i, position := range positions {
			if i > 0 && !areOnSameGridAxis(positions[i-1], position) {
				return Scene{}, fmt.Errorf("trace %d path positions %d and %d do not lie on the same straight grid axis", traceIndex, i-1, i)
			}
			points[i] = GridPositionToPoint(position, o.Width, o.Height, o.Grid.Pitch, o.Orientation)
		}
		if len(points) < 2 {
			return Scene{}, fmt.Errorf("trace %d must connect at least two grid positions", traceIndex)
		}

		maxOffset := float64(trace.Lines-1) * o.TraceSpacing / 2
		if maxOffset+o.TraceStrokeWidth/2 >= source.Radius*sqrt3/2 {
			return Scene{}, fmt.Errorf("trace %d bundle does not fit inside its source component", traceIndex)
		}
		if destinationFound {
			destinationMargin := o.NodeRadius
			if trace.HideTerminal {
				destinationMargin = 0
			}
			if maxOffset+destinationMargin+o.TraceStrokeWidth/2 >= destination.Radius*sqrt3/2 {
				return Scene{}, fmt.Errorf("trace %d bundle does not fit inside its destination component", traceIndex)
			}
		}

		for line := 0; line < trace.Lines; line++ {
			offset := (float64(line) - float64(trace.Lines-1)/2) * o.TraceSpacing
			track := offsetPolyline(points, offset)
			// Clip the source end at the source boundary. The SVG stroke extends
			// across that boundary by half its width, keeping the line connected.
			track, err := clipTrackStartToComponent(track, source, baseAngle, 0)
			if err != nil {
				return Scene{}, fmt.Errorf("trace %d line %d: %w", traceIndex, line+1, err)
			}

			if trace.HideTerminal {
				track, err = clipTrackEndToComponent(track, destination, baseAngle, 0)
				if err != nil {
					return Scene{}, fmt.Errorf("trace %d line %d: %w", traceIndex, line+1, err)
				}
			} else if destinationFound {
				endpoint := track[len(track)-1]
				if !PointInsideHexWithMargin(endpoint, Point{X: destination.X, Y: destination.Y}, destination.Radius, baseAngle, o.NodeRadius+o.TraceStrokeWidth/2) {
					return Scene{}, fmt.Errorf("trace %d line %d terminal does not fit inside its destination component", traceIndex, line+1)
				}
			}

			endpoint := track[len(track)-1]
			visual.Traces = append(visual.Traces, Trace{Points: track, HideTerminal: trace.HideTerminal})
			if !trace.HideTerminal {
				visual.Terminals = append(visual.Terminals, Terminal{
					X:      endpoint.X,
					Y:      endpoint.Y,
					Radius: o.NodeRadius,
				})
			}
		}
	}

	return visual, nil
}

// RenderGrid builds and renders a high-level grid layout as SVG.
func RenderGrid(layout GridScene) (string, error) {
	scene, err := BuildScene(layout)
	if err != nil {
		return "", err
	}
	return Render(scene), nil
}

// GridPositionToPoint converts an axial coordinate to a model-space point in inches.
// For pointy-top layouts, positive Q points east and positive R points southeast.
// The origin is the canvas center.
func GridPositionToPoint(position GridPosition, width, height, pitch float64, orientation string) Point {
	baseAngle := orientationBaseAngle(orientation)
	q, r := float64(position.Q), float64(position.R)
	if orientation != "flat" {
		// Convert conventional pointy-top axial coordinates back to the previous
		// basis before evaluating the equivalent geometry. Besides making the
		// coordinate convention explicit at the API, this retains the exact
		// floating-point operation order used by existing SVG golden files.
		q, r = float64(-position.R), float64(position.Q+position.R)
	}
	theta1 := baseAngle + math.Pi/3
	return Point{
		X: width/2 + pitch*(q*math.Cos(baseAngle)+r*math.Cos(theta1)),
		Y: height/2 + pitch*(q*math.Sin(baseAngle)+r*math.Sin(theta1)),
	}
}

func validateOptions(o Options) error {
	positive := []struct {
		name  string
		value float64
	}{
		{"canvas width", o.Width},
		{"canvas height", o.Height},
		{"grid pitch", o.Grid.Pitch},
		{"grid radius", o.Grid.Radius},
		{"component radius", o.ComponentRadius},
		{"component stroke width", o.ComponentStrokeWidth},
		{"trace stroke width", o.TraceStrokeWidth},
		{"terminal radius", o.NodeRadius},
	}
	for _, item := range positive {
		if !isFinite(item.value) || item.value <= 0 {
			return fmt.Errorf("%s must be finite and greater than zero", item.name)
		}
	}
	if !isFinite(o.Grid.StrokeWidth) || o.Grid.StrokeWidth < 0 {
		return fmt.Errorf("grid stroke width must be finite and greater than or equal to zero")
	}
	if o.Orientation != "pointy" && o.Orientation != "flat" {
		return fmt.Errorf("orientation must be either %q or %q", "pointy", "flat")
	}
	if o.TraceSpacing < 0 || !isFinite(o.TraceSpacing) {
		return fmt.Errorf("trace spacing must be finite and greater than or equal to zero")
	}
	if !isFinite(o.ComponentMiddleHexagonScale) || !isFinite(o.ComponentInnerHexagonScale) ||
		o.ComponentMiddleHexagonScale <= 0 || o.ComponentMiddleHexagonScale >= 1 ||
		o.ComponentInnerHexagonScale <= 0 || o.ComponentInnerHexagonScale >= o.ComponentMiddleHexagonScale {
		return fmt.Errorf("component inner scales must satisfy 0 < inner-inner < inner < 1")
	}
	if o.Color == "" {
		return fmt.Errorf("stroke color cannot be empty")
	}
	return nil
}

// areOnSameGridAxis reports whether two distinct axial coordinates are collinear
// along one of the six legal hex-grid directions. The distance may span multiple cells.
func areOnSameGridAxis(a, b GridPosition) bool {
	dq, dr := b.Q-a.Q, b.R-a.R
	if dq == 0 && dr == 0 {
		return false
	}
	return dq == 0 || dr == 0 || dq+dr == 0
}

func clipTrackStartToComponent(track []Point, component Component, baseAngle, margin float64) ([]Point, error) {
	if len(track) < 2 {
		return nil, fmt.Errorf("trace path must contain at least two points")
	}
	center := Point{X: component.X, Y: component.Y}
	inside := func(point Point) bool {
		return PointInsideHexWithMargin(point, center, component.Radius, baseAngle, margin)
	}
	if !inside(track[0]) {
		return nil, fmt.Errorf("starting line does not fit inside its source component")
	}
	for i := 1; i < len(track); i++ {
		if inside(track[i]) {
			continue
		}
		low, high := 0.0, 1.0
		start, delta := track[i-1], sub(track[i], track[i-1])
		for iteration := 0; iteration < 60; iteration++ {
			middle := (low + high) / 2
			candidate := Point{X: start.X + delta.X*middle, Y: start.Y + delta.Y*middle}
			if inside(candidate) {
				low = middle
			} else {
				high = middle
			}
		}
		boundary := Point{X: start.X + delta.X*high, Y: start.Y + delta.Y*high}
		result := make([]Point, 0, len(track)-i+1)
		result = append(result, boundary)
		result = append(result, track[i:]...)
		return result, nil
	}
	return nil, fmt.Errorf("trace path never exits its source component")
}

// clipTrackEndToComponent trims a track at the first point where it enters the
// destination component. Unlike a terminal-terminated trace, the returned endpoint
// lies on the component's outer hexagonal boundary.
func clipTrackEndToComponent(track []Point, component Component, baseAngle, margin float64) ([]Point, error) {
	if len(track) < 2 {
		return nil, fmt.Errorf("trace path must contain at least two points")
	}
	center := Point{X: component.X, Y: component.Y}
	inside := func(point Point) bool {
		return PointInsideHexWithMargin(point, center, component.Radius, baseAngle, margin)
	}
	if !inside(track[len(track)-1]) {
		return nil, fmt.Errorf("ending line does not fit inside its destination component")
	}

	for i := len(track) - 2; i >= 0; i-- {
		if inside(track[i]) {
			continue
		}
		outside, delta := track[i], sub(track[i+1], track[i])
		low, high := 0.0, 1.0 // low is outside; high is inside.
		for iteration := 0; iteration < 60; iteration++ {
			middle := (low + high) / 2
			candidate := Point{X: outside.X + delta.X*middle, Y: outside.Y + delta.Y*middle}
			if inside(candidate) {
				high = middle
			} else {
				low = middle
			}
		}
		boundary := Point{X: outside.X + delta.X*high, Y: outside.Y + delta.Y*high}
		result := append([]Point(nil), track[:i+1]...)
		result = append(result, boundary)
		return result, nil
	}
	return nil, fmt.Errorf("trace path never enters its destination component from outside")
}

func offsetPolyline(points []Point, offset float64) []Point {
	if len(points) < 2 || math.Abs(offset) < vectorEpsilon {
		return append([]Point(nil), points...)
	}
	directions := make([]Point, len(points)-1)
	normals := make([]Point, len(points)-1)
	for i := 0; i < len(points)-1; i++ {
		delta := sub(points[i+1], points[i])
		length := math.Hypot(delta.X, delta.Y)
		if length < vectorEpsilon {
			return append([]Point(nil), points...)
		}
		directions[i] = Point{X: delta.X / length, Y: delta.Y / length}
		normals[i] = Point{X: -directions[i].Y, Y: directions[i].X}
	}
	result := make([]Point, 0, len(points))
	result = append(result, addPoint(points[0], scalePoint(normals[0], offset)))
	for i := 1; i < len(points)-1; i++ {
		p := points[i]
		line1 := addPoint(p, scalePoint(normals[i-1], offset))
		line2 := addPoint(p, scalePoint(normals[i], offset))
		denominator := cross(directions[i-1], directions[i])
		if math.Abs(denominator) < epsilon {
			result = append(result, line1)
			continue
		}
		t := cross(sub(line2, line1), directions[i]) / denominator
		result = append(result, addPoint(line1, scalePoint(directions[i-1], t)))
	}
	last := len(points) - 1
	result = append(result, addPoint(points[last], scalePoint(normals[last-1], offset)))
	return result
}

func addPoint(a, b Point) Point { return Point{X: a.X + b.X, Y: a.Y + b.Y} }
func scalePoint(point Point, scale float64) Point {
	return Point{X: point.X * scale, Y: point.Y * scale}
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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
