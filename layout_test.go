package hexgrid

import (
	"math"
	"strings"
	"testing"

	"github.com/sector-f/hexgrid/geometry"
)

func TestSceneBuildsComponentsAndParallelTrace(t *testing.T) {
	opts := DefaultOptions()
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
			{Position: GridPosition{Q: 2, R: 1}, Style: ComponentStyleTriple, RadiusScale: 0.9},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path: []GridPosition{
				{Q: 1, R: 0},
				{Q: 1, R: 1},
				{Q: 2, R: 1},
			},
			Lines: 3,
		}},
	}

	scene, err := buildGeometryScene(layout)
	if err != nil {
		t.Fatalf("buildGeometryScene() error = %v", err)
	}
	if len(scene.Components) != 2 {
		t.Fatalf("got %d components, want 2", len(scene.Components))
	}
	if scene.Components[0].Style != geometry.ComponentStyleSingle || scene.Components[1].Style != geometry.ComponentStyleTriple {
		t.Fatalf("component styles = %v, %v", scene.Components[0].Style, scene.Components[1].Style)
	}
	if scene.Components[1].Radius != opts.ComponentRadius*0.9 {
		t.Errorf("destination radius = %v, want %v", scene.Components[1].Radius, opts.ComponentRadius*0.9)
	}
	if len(scene.Traces) != 3 || len(scene.Terminals) != 3 {
		t.Fatalf("got %d traces and %d terminals, want 3 each", len(scene.Traces), len(scene.Terminals))
	}

	source := gridgeom.PositionToPoint(layout.Components[0].Position.Q, layout.Components[0].Position.R, opts.Width, opts.Height, opts.Grid.Pitch, opts.Orientation)
	for i, trace := range scene.Traces {
		if len(trace.Points) < 2 {
			t.Errorf("trace %d has %d points, want at least 2", i, len(trace.Points))
			continue
		}
		if distance(trace.Points[0], source) < scene.Components[0].Radius*0.8 {
			t.Errorf("trace %d starts inside its source component: point=%+v center=%+v", i, trace.Points[0], source)
		}
	}

}

func TestRenderDoesNotTrimHideTerminalTrace(t *testing.T) {
	scene := geometry.Scene{
		Width:            2,
		Height:           2,
		Color:            "#000000",
		TraceStrokeWidth: 0.01,
		NodeRadius:       0.2,
		Traces: []geometry.Trace{{
			Points:       []geometry.Point{{X: 0.5, Y: 0.5}, {X: 1.5, Y: 0.5}},
			HideTerminal: true,
		}},
	}
	rendered := geometry.Render(scene)
	if !strings.Contains(rendered, "1500,500") {
		t.Fatalf("Render() shortened a HideTerminal trace instead of preserving its final point: %s", rendered)
	}
}

func TestGridPositionToPointUsesCanvasCenterAsOrigin(t *testing.T) {
	center := gridgeom.PositionToPoint(0, 0, 10, 8, 0.5, "pointy")
	if center != (geometry.Point{X: 5, Y: 4}) {
		t.Fatalf("origin = %+v, want center of canvas (5, 4)", center)
	}
	west := gridgeom.PositionToPoint(-1, 0, 10, 8, 0.5, "pointy")
	if west != (geometry.Point{X: 4.5, Y: 4}) {
		t.Fatalf("Q=-1, R=0 maps to %+v, want one pitch west of center", west)
	}
	southeast := gridgeom.PositionToPoint(0, 1, 10, 8, 0.5, "pointy")
	wantSoutheast := geometry.Point{X: 5.25, Y: 4 + 0.5*math.Sqrt(3)/2}
	if distance(southeast, wantSoutheast) > 1e-9 {
		t.Fatalf("Q=0, R=1 maps to %+v, want southeast neighbor %+v", southeast, wantSoutheast)
	}
}
func TestGridPositionToPointUsesFlatTopAxialBasis(t *testing.T) {
	const pitch = 0.5
	center := gridgeom.PositionToPoint(0, 0, 10, 8, pitch, "flat")
	q := gridgeom.PositionToPoint(1, 0, 10, 8, pitch, "flat")
	r := gridgeom.PositionToPoint(0, 1, 10, 8, pitch, "flat")
	wantQ := geometry.Point{X: center.X + pitch*math.Sqrt(3)/2, Y: center.Y + pitch/2}
	wantR := geometry.Point{X: center.X, Y: center.Y + pitch}
	if distance(q, wantQ) > 1e-9 {
		t.Errorf("flat-top Q neighbor = %+v, want %+v", q, wantQ)
	}
	if distance(r, wantR) > 1e-9 {
		t.Errorf("flat-top R neighbor = %+v, want %+v", r, wantR)
	}
}
func TestGridCellsRetainPhysicalCentersWithConventionalCoordinates(t *testing.T) {
	const width, height, pitch = 6.0, 4.0, 0.5
	cells := gridgeom.Cells(width, height, pitch, "pointy", 0)
	if len(cells) == 0 {
		t.Fatal("gridgeom.Cells() returned no cells")
	}
	for _, cell := range cells {
		center := gridgeom.PositionToPoint(cell.Q, cell.R, width, height, pitch, "pointy")
		if distance(center, cell.Center) > 1e-9 {
			t.Errorf("cell (%d, %d) stores center %+v, but its axial coordinate maps to %+v", cell.Q, cell.R, cell.Center, center)
		}
	}
}
func TestSceneRejectsPathSegmentThatIsNotOnAGridAxis(t *testing.T) {
	opts := DefaultOptions()
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
			{Position: GridPosition{Q: 2, R: 1}, Style: ComponentStyleSingle},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path:  []GridPosition{{Q: 2, R: 1}},
			Lines: 1,
		}},
	}
	if _, err := buildGeometryScene(layout); err == nil {
		t.Fatal("buildGeometryScene() accepted a path segment that is not aligned with a hex-grid axis")
	}
}

func TestSceneAcceptsStraightPathSegmentThatSkipsCells(t *testing.T) {
	opts := DefaultOptions()
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
			{Position: GridPosition{Q: 0, R: -2}, Style: ComponentStyleSingle},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path:  []GridPosition{{Q: 0, R: -2}},
			Lines: 1,
		}},
	}
	if _, err := buildGeometryScene(layout); err != nil {
		t.Fatalf("buildGeometryScene() rejected a straight segment spanning two grid steps: %v", err)
	}
}

func TestSceneAcceptsStraightRunsAndTurns(t *testing.T) {
	opts := DefaultOptions()
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
			{Position: GridPosition{Q: 2, R: -2}, Style: ComponentStyleDouble},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path: []GridPosition{
				{Q: 0, R: -2},
				{Q: 2, R: -2},
			},
			Lines: 1,
		}},
	}
	if _, err := buildGeometryScene(layout); err != nil {
		t.Fatalf("buildGeometryScene() rejected straight runs separated by a turn: %v", err)
	}
}

func TestSceneAllowsTerminalAtCoordinateWithoutComponent(t *testing.T) {
	opts := DefaultOptions()
	endpoint := GridPosition{Q: 0, R: 2}
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path:  []GridPosition{endpoint},
			Lines: 1,
		}},
	}
	scene, err := buildGeometryScene(layout)
	if err != nil {
		t.Fatalf("buildGeometryScene() rejected a free terminal coordinate: %v", err)
	}
	if len(scene.Terminals) != 1 {
		t.Fatalf("got %d terminal pads, want 1", len(scene.Terminals))
	}
	want := gridgeom.PositionToPoint(endpoint.Q, endpoint.R, opts.Width, opts.Height, opts.Grid.Pitch, opts.Orientation)
	got := scene.Terminals[0]
	if distance(geometry.Point{X: got.X, Y: got.Y}, want) > 1e-9 {
		t.Errorf("terminal center = (%v, %v), want endpoint (%v, %v)", got.X, got.Y, want.X, want.Y)
	}
}

func TestSceneHideTerminalRequiresDestinationComponent(t *testing.T) {
	opts := DefaultOptions()
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
		},
		Traces: []GridTrace{{
			Start:        GridPosition{Q: 0, R: 0},
			Path:         []GridPosition{{Q: 0, R: 2}},
			Lines:        1,
			HideTerminal: true,
		}},
	}
	if _, err := buildGeometryScene(layout); err == nil {
		t.Fatal("buildGeometryScene() accepted HideTerminal without a destination component")
	}
}

func TestSceneHideTerminalClipsBundleToDestinationBoundary(t *testing.T) {
	opts := DefaultOptions()
	destinationPosition := GridPosition{Q: 0, R: 2}
	layout := Scene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
			{Position: destinationPosition, Style: ComponentStyleTriple, RadiusScale: 0.9},
		},
		Traces: []GridTrace{{
			Start:        GridPosition{Q: 0, R: 0},
			Path:         []GridPosition{destinationPosition},
			Lines:        3,
			HideTerminal: true,
		}},
	}
	scene, err := buildGeometryScene(layout)
	if err != nil {
		t.Fatalf("buildGeometryScene() error = %v", err)
	}
	if len(scene.Traces) != 3 {
		t.Fatalf("got %d tracks, want 3", len(scene.Traces))
	}
	if len(scene.Terminals) != 0 {
		t.Fatalf("got %d terminal pads, want none", len(scene.Terminals))
	}
	destination := scene.Components[1]
	center := geometry.Point{X: destination.X, Y: destination.Y}
	baseAngle := orientationBaseAngle(opts.Orientation)
	apothem := destination.Radius * sqrt3 / 2
	for i, trace := range scene.Traces {
		if !trace.HideTerminal {
			t.Errorf("track %d did not preserve HideTerminal in the rendered scene", i)
		}
		endpoint := trace.Points[len(trace.Points)-1]
		maxProjection := math.Inf(-1)
		for side := 0; side < hexSideCount; side++ {
			theta := baseAngle + float64(side)*math.Pi/3
			normal := geometry.Point{X: math.Cos(theta), Y: math.Sin(theta)}
			delta := sub(endpoint, center)
			projection := dot(delta, normal)
			if projection > maxProjection {
				maxProjection = projection
			}
		}
		if math.Abs(maxProjection-apothem) > 1e-8 {
			t.Errorf("track %d ends %.9f inches from the supporting boundary, want apothem %.9f", i, maxProjection, apothem)
		}
	}
}


func TestRenderPlacesGridLabelRequests(t *testing.T) {
	options := DefaultOptions()
	options.LabelFontFamily = "DejaVu Sans Mono"
	options.LabelFontSize = 36
	scene := Scene{
		Options: options,
		Components: []GridComponent{{
			Position: GridPosition{},
			Style: ComponentStyleSingle,
		}},
		Labels: []GridLabelRequest{{
			Component: GridPosition{},
			Candidates: []GridLabelCandidate{{
				Text: "VCO",
				SideOrder: []int{0, 1, 2, 3, 4, 5},
			}},
			Gap: 0.05,
			Padding: 0.005,
			ObjectGap: 0.006,
		}},
	}
	svg, err := Render(scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(svg, "VCO") {
		t.Fatalf("rendered SVG does not contain the requested label: %s", svg)
	}
}

func distance(a, b geometry.Point) float64 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return math.Hypot(dx, dy)
}

func TestGridColorOverridesOnlyBackgroundGrid(t *testing.T) {
	options := DefaultOptions()
	options.Color = "#123456"
	options.GridColor = "#abcdef"

	scene := Scene{Options: options}
	svg, err := Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	if scene.Options.GridColor != options.GridColor {
		t.Fatalf("scene grid color = %q, want %q", scene.Options.GridColor, options.GridColor)
	}
	gridStart := strings.Index(svg, `id="background-grid"`)
	componentsStart := strings.Index(svg, `id="components"`)
	if gridStart < 0 || componentsStart <= gridStart {
		t.Fatalf("could not locate grid and component layers in SVG: %s", svg)
	}
	if !strings.Contains(svg[gridStart:componentsStart], `stroke="#abcdef"`) {
		t.Fatal("background grid does not use GridColor")
	}
	if !strings.Contains(svg[componentsStart:], `stroke="#123456"`) {
		t.Fatal("components do not retain Color")
	}
}

func TestGridColorFallsBackToSceneColor(t *testing.T) {
	options := DefaultOptions()
	options.Width = 2
	options.Height = 2
	options.Color = "#123456"
	options.Grid = geometry.Grid{Pitch: 0.5, Radius: 0.2, StrokeWidth: 0.01}
	svg, err := Render(Scene{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	gridStart := strings.Index(svg, `id="background-grid"`)
	tracesStart := strings.Index(svg, `id="traces"`)
	if gridStart < 0 || tracesStart <= gridStart {
		t.Fatalf("could not locate background grid layer in SVG")
	}
	if !strings.Contains(svg[gridStart:tracesStart], `stroke="#123456"`) {
		t.Fatal("background grid did not fall back to Scene.Color")
	}
}

func TestGlowSettingsAreDisabledByDefault(t *testing.T) {
	options := DefaultOptions()
	if options.GlowEnabled {
		t.Fatal("glow should be disabled by default")
	}
	if options.GlowBlurRadius <= 0 || options.GlowOpacity <= 0 {
		t.Fatalf("default glow settings must be usable when enabled: radius=%v opacity=%v", options.GlowBlurRadius, options.GlowOpacity)
	}
	svg, err := Render(Scene{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(svg, "hexgrid-neon-glow") || strings.Contains(svg, "<filter") {
		t.Fatal("disabled glow unexpectedly emitted an SVG filter")
	}
}

func TestGlowRendersOnlyOnForegroundLayers(t *testing.T) {
	options := DefaultOptions()
	options.GlowEnabled = true
	options.GlowBlurRadius = 0.06
	options.GlowOpacity = 0.4

	svg, err := Render(Scene{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svg, `id="hexgrid-neon-glow"`) {
		t.Fatal("enabled glow did not emit its SVG filter")
	}
	groupTag := func(group string) string {
		start := strings.Index(svg, `<g id="`+group+`"`)
		if start < 0 {
			return ""
		}
		end := strings.Index(svg[start:], ">")
		if end < 0 {
			return ""
		}
		return svg[start : start+end+1]
	}
	for _, group := range []string{"traces", "components", "terminal-pads"} {
		if tag := groupTag(group); !strings.Contains(tag, `filter="url(#hexgrid-neon-glow)"`) {
			t.Errorf("%s group is missing its glow filter: %s", group, tag)
		}
	}
	for _, group := range []string{"background-grid", "component-labels"} {
		if tag := groupTag(group); strings.Contains(tag, `filter="url(#hexgrid-neon-glow)"`) {
			t.Errorf("%s should remain crisp and should not use the glow filter", group)
		}
	}
	if !strings.Contains(svg, `stdDeviation="60.000"`) || !strings.Contains(svg, `slope="0.400"`) {
		t.Fatal("glow filter does not contain the configured blur radius and opacity")
	}
}

func TestRenderRejectsInvalidGlowSettings(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Options)
	}{
		{"negative blur radius", func(o *Options) { o.GlowBlurRadius = -0.01 }},
		{"NaN blur radius", func(o *Options) { o.GlowBlurRadius = math.NaN() }},
		{"opacity below zero", func(o *Options) { o.GlowOpacity = -0.1 }},
		{"opacity above one", func(o *Options) { o.GlowOpacity = 1.1 }},
		{"enabled glow with zero blur", func(o *Options) { o.GlowEnabled = true; o.GlowBlurRadius = 0 }},
		{"enabled glow with zero opacity", func(o *Options) { o.GlowEnabled = true; o.GlowOpacity = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := DefaultOptions()
			test.edit(&options)
			if _, err := buildGeometryScene(Scene{Options: options}); err == nil {
				t.Fatal("expected invalid glow settings to be rejected")
			}
		})
	}
}

