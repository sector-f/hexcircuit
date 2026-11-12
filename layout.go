package hexgrid

import (
	"fmt"
	"math"

	"github.com/sector-f/hexgrid/geometry"
	"github.com/sector-f/hexgrid/internal/gridgeom"
)

const (
	sqrt3         = 1.7320508075688772935
	epsilon       = 1e-9
	vectorEpsilon = 1e-12
)

func orientationBaseAngle(orientation string) float64 {
	if orientation == "flat" {
		return 30 * math.Pi / 180
	}
	return -60 * math.Pi / 180
}

func cross(a, b geometry.Point) float64 { return a.X*b.Y - a.Y*b.X }
func dot(a, b geometry.Point) float64 { return a.X*b.X + a.Y*b.Y }
func sub(a, b geometry.Point) geometry.Point { return geometry.Point{X: a.X - b.X, Y: a.Y - b.Y} }
func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// GridLabelCandidate is one alternative text value for a label. Candidates are
// tried in order until one can be placed without colliding with the scene. Hand-authored
// scenes can provide SideOrder directly; SideOrderProvider is an optional lazy form
// for callers whose side preference is randomized or otherwise computed on demand.
type GridLabelCandidate struct {
	Text              string
	SideOrder         []int
	SideOrderProvider func() []int
}

// GridLabelRequest asks the renderer to place a label relative to a component.
// Candidates are tried in order; CandidateProvider is an optional lazy alternative
// used instead of Candidates. Side order is specified by each candidate. Numbered
// appends a per-text 1-based count, incremented only after successful placement.
// InsideFallback permits a centered label after all external candidates fail.
// Distances are measured in inches.
type GridLabelRequest struct {
	Component         GridPosition
	Candidates        []GridLabelCandidate
	CandidateProvider func() []GridLabelCandidate
	Numbered          bool
	InsideFallback bool
	Gap            float64
	Padding        float64
	ObjectGap      float64
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
	GridColor     string
	Background    string

	// GlowEnabled adds a soft neon bloom to foreground traces, components, and terminal pads.
	GlowEnabled bool
	// GlowBlurRadius is the Gaussian blur standard deviation, in inches.
	GlowBlurRadius float64
	// GlowOpacity controls the opacity of the blurred glow (0-1).
	GlowOpacity float64

	Grid geometry.Grid

	ComponentRadius             float64
	ComponentStrokeWidth        float64
	ComponentMiddleHexagonScale float64
	ComponentInnerHexagonScale  float64
	TraceStrokeWidth            float64
	// TraceSpacing is the perpendicular centerline distance between bundle lines.
	TraceSpacing float64
	// NodeRadius is the radius of each terminal pad at a trace's destination.
	NodeRadius float64

	LabelColor      string
	LabelFontFamily string
	LabelFontSize   float64
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
		GlowBlurRadius:              0.05,
		GlowOpacity:                 0.55,
		Grid:                        geometry.Grid{Pitch: 0.62, Radius: 0.33255, StrokeWidth: 0.004},
		ComponentRadius:             0.40,
		ComponentStrokeWidth:        0.012,
		ComponentMiddleHexagonScale: 0.60,
		ComponentInnerHexagonScale:  0.34,
		TraceStrokeWidth:            0.008,
		TraceSpacing:                0.035,
		NodeRadius:                  0.014,
	}
}

// Scene is the high-level, manually-authored representation of a hexagonal
// illustration. Components, traces, and labels belong to the grid-oriented API.
type Scene struct {
	Options    Options
	Components []GridComponent
	Traces     []GridTrace
	Labels     []GridLabelRequest
}

// buildGeometryScene converts a grid-oriented scene into its private low-level representation.
// It validates grid positions, component styles, trace paths, and bundle dimensions.
func buildGeometryScene(layout Scene) (geometry.Scene, error) {
	o := layout.Options
	if err := validateOptions(o); err != nil {
		return geometry.Scene{}, err
	}

	baseAngle := orientationBaseAngle(o.Orientation)
	visual := geometry.Scene{
		Width:                       o.Width,
		Height:                      o.Height,
		Orientation:                 o.Orientation,
		Color:                       o.Color,
		GridColor:                   o.GridColor,
		Background:                  o.Background,
		GlowEnabled:                 o.GlowEnabled,
		GlowBlurRadius:              o.GlowBlurRadius,
		GlowOpacity:                 o.GlowOpacity,
		Grid: o.Grid,
		ComponentStrokeWidth:        o.ComponentStrokeWidth,
		ComponentMiddleHexagonScale: o.ComponentMiddleHexagonScale,
		ComponentInnerHexagonScale:  o.ComponentInnerHexagonScale,
		TraceStrokeWidth:            o.TraceStrokeWidth,
		NodeRadius:                  o.NodeRadius,
		LabelColor:                  o.LabelColor,
		LabelFontFamily:             o.LabelFontFamily,
		LabelFontSize:               o.LabelFontSize,
		
	}

	componentsByPosition := make(map[GridPosition]geometry.Component, len(layout.Components))
	for i, component := range layout.Components {
		geometryStyle, validStyle := geometryComponentStyle(component.Style)
		if !validStyle {
			return geometry.Scene{}, fmt.Errorf("component %d has invalid style %d", i, component.Style)
		}
		radiusScale := component.RadiusScale
		if radiusScale == 0 {
			radiusScale = 1
		}
		if !isFinite(radiusScale) || radiusScale <= 0 {
			return geometry.Scene{}, fmt.Errorf("component %d radius scale must be finite and greater than zero", i)
		}
		if _, exists := componentsByPosition[component.Position]; exists {
			return geometry.Scene{}, fmt.Errorf("multiple components occupy grid position (%d, %d)", component.Position.Q, component.Position.R)
		}

		center := gridgeom.PositionToPoint(component.Position.Q, component.Position.R, o.Width, o.Height, o.Grid.Pitch, o.Orientation)
		rendered := geometry.Component{
			Index:  i,
			X:      center.X,
			Y:      center.Y,
			Radius: o.ComponentRadius * radiusScale,
			Style:  geometryStyle,
		}
		componentsByPosition[component.Position] = rendered
		visual.Components = append(visual.Components, rendered)
	}

	for traceIndex, trace := range layout.Traces {
		if trace.Lines < 1 {
			return geometry.Scene{}, fmt.Errorf("trace %d must contain at least one line", traceIndex)
		}
		if trace.Lines > 1 && o.TraceSpacing == 0 {
			return geometry.Scene{}, fmt.Errorf("trace %d has multiple lines but trace spacing is zero", traceIndex)
		}
		if len(trace.Path) == 0 {
			return geometry.Scene{}, fmt.Errorf("trace %d must contain at least one path position", traceIndex)
		}
		source, found := componentsByPosition[trace.Start]
		if !found {
			return geometry.Scene{}, fmt.Errorf("trace %d starts at grid position (%d, %d), which has no component", traceIndex, trace.Start.Q, trace.Start.R)
		}
		destinationPosition := trace.Path[len(trace.Path)-1]
		destination, destinationFound := componentsByPosition[destinationPosition]
		if trace.HideTerminal && !destinationFound {
			return geometry.Scene{}, fmt.Errorf("trace %d hides its terminal but ends at grid position (%d, %d), which has no component", traceIndex, destinationPosition.Q, destinationPosition.R)
		}

		positions := make([]GridPosition, 1, len(trace.Path)+1)
		positions[0] = trace.Start
		positions = append(positions, trace.Path...)
		points := make([]geometry.Point, len(positions))
		for i, position := range positions {
			if i > 0 && !areOnSameGridAxis(positions[i-1], position) {
				return geometry.Scene{}, fmt.Errorf("trace %d path positions %d and %d do not lie on the same straight grid axis", traceIndex, i-1, i)
			}
			points[i] = gridgeom.PositionToPoint(position.Q, position.R, o.Width, o.Height, o.Grid.Pitch, o.Orientation)
		}
		if len(points) < 2 {
			return geometry.Scene{}, fmt.Errorf("trace %d must connect at least two grid positions", traceIndex)
		}

		maxOffset := float64(trace.Lines-1) * o.TraceSpacing / 2
		if maxOffset+o.TraceStrokeWidth/2 >= source.Radius*sqrt3/2 {
			return geometry.Scene{}, fmt.Errorf("trace %d bundle does not fit inside its source component", traceIndex)
		}
		if destinationFound {
			destinationMargin := o.NodeRadius
			if trace.HideTerminal {
				destinationMargin = 0
			}
			if maxOffset+destinationMargin+o.TraceStrokeWidth/2 >= destination.Radius*sqrt3/2 {
				return geometry.Scene{}, fmt.Errorf("trace %d bundle does not fit inside its destination component", traceIndex)
			}
		}

		for line := 0; line < trace.Lines; line++ {
			offset := (float64(line) - float64(trace.Lines-1)/2) * o.TraceSpacing
			track := offsetPolyline(points, offset)
			// Clip the source end at the source boundary. The SVG stroke extends
			// across that boundary by half its width, keeping the line connected.
			track, err := clipTrackStartToComponent(track, source, baseAngle, 0)
			if err != nil {
				return geometry.Scene{}, fmt.Errorf("trace %d line %d: %w", traceIndex, line+1, err)
			}

			if trace.HideTerminal {
				track, err = clipTrackEndToComponent(track, destination, baseAngle, 0)
				if err != nil {
					return geometry.Scene{}, fmt.Errorf("trace %d line %d: %w", traceIndex, line+1, err)
				}
			} else if destinationFound {
				endpoint := track[len(track)-1]
				if !geometry.PointInsideHexWithMargin(endpoint, geometry.Point{X: destination.X, Y: destination.Y}, destination.Radius, baseAngle, o.NodeRadius+o.TraceStrokeWidth/2) {
					return geometry.Scene{}, fmt.Errorf("trace %d line %d terminal does not fit inside its destination component", traceIndex, line+1)
				}
			}

			endpoint := track[len(track)-1]
			visual.Traces = append(visual.Traces, geometry.Trace{Points: track, HideTerminal: trace.HideTerminal})
			if !trace.HideTerminal {
				visual.Terminals = append(visual.Terminals, geometry.Terminal{
					X:      endpoint.X,
					Y:      endpoint.Y,
					Radius: o.NodeRadius,
				})
			}
		}
	}

	return visual, nil
}

// Render validates, converts, places grid-oriented labels, and renders a scene as SVG.
func Render(scene Scene) (string, error) {
	visual, err := buildGeometryScene(scene)
	if err != nil {
		return "", err
	}

	componentIndices := make(map[GridPosition]int, len(scene.Components))
	for index, component := range scene.Components {
		componentIndices[component.Position] = index
	}
	groups := make([]geometry.LabelGroupRequest, 0, len(scene.Labels))
	for labelIndex, request := range scene.Labels {
		if len(request.Candidates) == 0 && request.CandidateProvider == nil {
			continue
		}
		componentIndex, exists := componentIndices[request.Component]
		if !exists {
			return "", fmt.Errorf("label %d refers to grid position (%d, %d), which has no component", labelIndex, request.Component.Q, request.Component.R)
		}
		convertCandidates := func(gridCandidates []GridLabelCandidate) []geometry.LabelCandidate {
			candidates := make([]geometry.LabelCandidate, len(gridCandidates))
			for i, candidate := range gridCandidates {
				candidates[i] = geometry.LabelCandidate{
					Text: candidate.Text,
					SideOrder: append([]int(nil), candidate.SideOrder...),
					SideOrderProvider: candidate.SideOrderProvider,
				}
			}
			return candidates
		}
		group := geometry.LabelGroupRequest{
			ComponentIndex: componentIndex,
			Numbered: request.Numbered,
			InsideFallback: request.InsideFallback,
			Gap: request.Gap,
			Padding: request.Padding,
			ObjectGap: request.ObjectGap,
		}
		if request.CandidateProvider != nil {
			provider := request.CandidateProvider
			group.CandidateProvider = func() []geometry.LabelCandidate {
				return convertCandidates(provider())
			}
		} else {
			group.Candidates = convertCandidates(request.Candidates)
		}
		groups = append(groups, group)
	}

	placedLabels, err := geometry.PlaceLabelGroups(&visual, groups)
	if err != nil {
		return "", err
	}
	// An internal label occupies the component's center. Suppress nested outlines
	// beneath that label without requiring callers to know its eventual placement.
	for _, label := range placedLabels {
		if label.Inside && label.ComponentIndex >= 0 && label.ComponentIndex < len(visual.Components) {
			visual.Components[label.ComponentIndex].Style = geometry.ComponentStyleSingle
			visual.Components[label.ComponentIndex].Detail = geometry.ComponentDetailNone
		}
	}
	return geometry.Render(visual), nil
}

func geometryComponentStyle(style ComponentStyle) (geometry.ComponentStyle, bool) {
	switch style {
	case ComponentStyleSingle:
		return geometry.ComponentStyleSingle, true
	case ComponentStyleDouble:
		return geometry.ComponentStyleDouble, true
	case ComponentStyleTriple:
		return geometry.ComponentStyleTriple, true
	default:
		return 0, false
	}
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

func clipTrackStartToComponent(track []geometry.Point, component geometry.Component, baseAngle, margin float64) ([]geometry.Point, error) {
	if len(track) < 2 {
		return nil, fmt.Errorf("trace path must contain at least two points")
	}
	center := geometry.Point{X: component.X, Y: component.Y}
	inside := func(point geometry.Point) bool {
		return geometry.PointInsideHexWithMargin(point, center, component.Radius, baseAngle, margin)
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
			candidate := geometry.Point{X: start.X + delta.X*middle, Y: start.Y + delta.Y*middle}
			if inside(candidate) {
				low = middle
			} else {
				high = middle
			}
		}
		boundary := geometry.Point{X: start.X + delta.X*high, Y: start.Y + delta.Y*high}
		result := make([]geometry.Point, 0, len(track)-i+1)
		result = append(result, boundary)
		result = append(result, track[i:]...)
		return result, nil
	}
	return nil, fmt.Errorf("trace path never exits its source component")
}

// clipTrackEndToComponent trims a track at the first point where it enters the
// destination component. Unlike a terminal-terminated trace, the returned endpoint
// lies on the component's outer hexagonal boundary.
func clipTrackEndToComponent(track []geometry.Point, component geometry.Component, baseAngle, margin float64) ([]geometry.Point, error) {
	if len(track) < 2 {
		return nil, fmt.Errorf("trace path must contain at least two points")
	}
	center := geometry.Point{X: component.X, Y: component.Y}
	inside := func(point geometry.Point) bool {
		return geometry.PointInsideHexWithMargin(point, center, component.Radius, baseAngle, margin)
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
			candidate := geometry.Point{X: outside.X + delta.X*middle, Y: outside.Y + delta.Y*middle}
			if inside(candidate) {
				high = middle
			} else {
				low = middle
			}
		}
		boundary := geometry.Point{X: outside.X + delta.X*high, Y: outside.Y + delta.Y*high}
		result := append([]geometry.Point(nil), track[:i+1]...)
		result = append(result, boundary)
		return result, nil
	}
	return nil, fmt.Errorf("trace path never enters its destination component from outside")
}

func offsetPolyline(points []geometry.Point, offset float64) []geometry.Point {
	if len(points) < 2 || math.Abs(offset) < vectorEpsilon {
		return append([]geometry.Point(nil), points...)
	}
	directions := make([]geometry.Point, len(points)-1)
	normals := make([]geometry.Point, len(points)-1)
	for i := 0; i < len(points)-1; i++ {
		delta := sub(points[i+1], points[i])
		length := math.Hypot(delta.X, delta.Y)
		if length < vectorEpsilon {
			return append([]geometry.Point(nil), points...)
		}
		directions[i] = geometry.Point{X: delta.X / length, Y: delta.Y / length}
		normals[i] = geometry.Point{X: -directions[i].Y, Y: directions[i].X}
	}
	result := make([]geometry.Point, 0, len(points))
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

func addPoint(a, b geometry.Point) geometry.Point { return geometry.Point{X: a.X + b.X, Y: a.Y + b.Y} }
func scalePoint(point geometry.Point, scale float64) geometry.Point {
	return geometry.Point{X: point.X * scale, Y: point.Y * scale}
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
