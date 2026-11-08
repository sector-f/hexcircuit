package hexgrid

import (
	"math"
	"strings"
	"testing"
)

func TestGridSceneBuildsComponentsAndParallelTrace(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
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

	scene, err := BuildScene(layout)
	if err != nil {
		t.Fatalf("BuildScene() error = %v", err)
	}
	if len(scene.Components) != 2 {
		t.Fatalf("got %d components, want 2", len(scene.Components))
	}
	if scene.Components[0].Style != ComponentStyleSingle || scene.Components[1].Style != ComponentStyleTriple {
		t.Fatalf("component styles = %v, %v", scene.Components[0].Style, scene.Components[1].Style)
	}
	if scene.Components[1].Radius != opts.ComponentRadius*0.9 {
		t.Errorf("destination radius = %v, want %v", scene.Components[1].Radius, opts.ComponentRadius*0.9)
	}
	if len(scene.Traces) != 3 || len(scene.Terminals) != 3 {
		t.Fatalf("got %d traces and %d terminals, want 3 each", len(scene.Traces), len(scene.Terminals))
	}

	source := GridPositionToPoint(layout.Components[0].Position, opts.Width, opts.Height, opts.Grid.Pitch, opts.Orientation)
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
	scene := Scene{
		Width:            2,
		Height:           2,
		Color:            "#000000",
		TraceStrokeWidth: 0.01,
		NodeRadius:       0.2,
		Traces: []Trace{{
			Points:       []Point{{X: 0.5, Y: 0.5}, {X: 1.5, Y: 0.5}},
			HideTerminal: true,
		}},
	}
	rendered := Render(scene)
	if !strings.Contains(rendered, "1500,500") {
		t.Fatalf("Render() shortened a HideTerminal trace instead of preserving its final point: %s", rendered)
	}
}

func TestGridPositionToPointUsesCanvasCenterAsOrigin(t *testing.T) {
	center := GridPositionToPoint(GridPosition{}, 10, 8, 0.5, "pointy")
	if center != (Point{X: 5, Y: 4}) {
		t.Fatalf("origin = %+v, want center of canvas (5, 4)", center)
	}
	west := GridPositionToPoint(GridPosition{Q: -1}, 10, 8, 0.5, "pointy")
	if west != (Point{X: 4.5, Y: 4}) {
		t.Fatalf("Q=-1, R=0 maps to %+v, want one pitch west of center", west)
	}
	southeast := GridPositionToPoint(GridPosition{R: 1}, 10, 8, 0.5, "pointy")
	wantSoutheast := Point{X: 5.25, Y: 4 + 0.5*math.Sqrt(3)/2}
	if distance(southeast, wantSoutheast) > 1e-9 {
		t.Fatalf("Q=0, R=1 maps to %+v, want southeast neighbor %+v", southeast, wantSoutheast)
	}
}

func TestGridPositionToPointUsesFlatTopAxialBasis(t *testing.T) {
	const pitch = 0.5
	center := GridPositionToPoint(GridPosition{}, 10, 8, pitch, "flat")
	q := GridPositionToPoint(GridPosition{Q: 1}, 10, 8, pitch, "flat")
	r := GridPositionToPoint(GridPosition{R: 1}, 10, 8, pitch, "flat")
	wantQ := Point{X: center.X + pitch*math.Sqrt(3)/2, Y: center.Y + pitch/2}
	wantR := Point{X: center.X, Y: center.Y + pitch}
	if distance(q, wantQ) > 1e-9 {
		t.Errorf("flat-top Q neighbor = %+v, want %+v", q, wantQ)
	}
	if distance(r, wantR) > 1e-9 {
		t.Errorf("flat-top R neighbor = %+v, want %+v", r, wantR)
	}
}

func TestGridCellsRetainPhysicalCentersWithConventionalCoordinates(t *testing.T) {
	const width, height, pitch = 6.0, 4.0, 0.5
	cells := GridCells(width, height, pitch, "pointy", 0)
	if len(cells) == 0 {
		t.Fatal("GridCells() returned no cells")
	}
	for _, cell := range cells {
		center := GridPositionToPoint(GridPosition{Q: cell.Q, R: cell.R}, width, height, pitch, "pointy")
		if distance(center, cell.Center) > 1e-9 {
			t.Errorf("cell (%d, %d) stores center %+v, but its axial coordinate maps to %+v", cell.Q, cell.R, cell.Center, center)
		}
	}
}

func TestComponentStyleHexagonCounts(t *testing.T) {
	for _, test := range []struct {
		style ComponentStyle
		want  int
	}{
		{ComponentStyleSingle, 1},
		{ComponentStyleDouble, 2},
		{ComponentStyleTriple, 3},
		{ComponentStyle(255), 0},
	} {
		if got := test.style.HexagonCount(); got != test.want {
			t.Errorf("style %d HexagonCount() = %d, want %d", test.style, got, test.want)
		}
	}
}

func TestGridSceneRejectsPathSegmentThatIsNotOnAGridAxis(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
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
	if _, err := BuildScene(layout); err == nil {
		t.Fatal("BuildScene() accepted a path segment that is not aligned with a hex-grid axis")
	}
}

func TestGridSceneAcceptsStraightPathSegmentThatSkipsCells(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
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
	if _, err := BuildScene(layout); err != nil {
		t.Fatalf("BuildScene() rejected a straight segment spanning two grid steps: %v", err)
	}
}

func TestGridSceneAcceptsStraightRunsAndTurns(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
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
	if _, err := BuildScene(layout); err != nil {
		t.Fatalf("BuildScene() rejected straight runs separated by a turn: %v", err)
	}
}

func TestGridSceneAllowsTerminalAtCoordinateWithoutComponent(t *testing.T) {
	opts := DefaultOptions()
	endpoint := GridPosition{Q: 0, R: 2}
	layout := GridScene{
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
	scene, err := BuildScene(layout)
	if err != nil {
		t.Fatalf("BuildScene() rejected a free terminal coordinate: %v", err)
	}
	if len(scene.Terminals) != 1 {
		t.Fatalf("got %d terminal pads, want 1", len(scene.Terminals))
	}
	want := GridPositionToPoint(endpoint, opts.Width, opts.Height, opts.Grid.Pitch, opts.Orientation)
	got := scene.Terminals[0]
	if distance(Point{X: got.X, Y: got.Y}, want) > 1e-9 {
		t.Errorf("terminal center = (%v, %v), want endpoint (%v, %v)", got.X, got.Y, want.X, want.Y)
	}
}

func TestGridSceneHideTerminalRequiresDestinationComponent(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
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
	if _, err := BuildScene(layout); err == nil {
		t.Fatal("BuildScene() accepted HideTerminal without a destination component")
	}
}

func TestGridSceneHideTerminalClipsBundleToDestinationBoundary(t *testing.T) {
	opts := DefaultOptions()
	destinationPosition := GridPosition{Q: 0, R: 2}
	layout := GridScene{
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
	scene, err := BuildScene(layout)
	if err != nil {
		t.Fatalf("BuildScene() error = %v", err)
	}
	if len(scene.Traces) != 3 {
		t.Fatalf("got %d tracks, want 3", len(scene.Traces))
	}
	if len(scene.Terminals) != 0 {
		t.Fatalf("got %d terminal pads, want none", len(scene.Terminals))
	}
	destination := scene.Components[1]
	center := Point{X: destination.X, Y: destination.Y}
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
			normal := Point{X: math.Cos(theta), Y: math.Sin(theta)}
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

func distance(a, b Point) float64 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return math.Hypot(dx, dy)
}

func TestComponentStyleSelectsConfiguredNumberOfHexagons(t *testing.T) {
	scene := Scene{ComponentMiddleHexagonScale: 0.6, ComponentInnerHexagonScale: 0.3}
	for _, test := range []struct {
		style ComponentStyle
		want  int
	}{
		{ComponentStyleSingle, 0},
		{ComponentStyleDouble, 1},
		{ComponentStyleTriple, 2},
	} {
		got := len(componentInnerScales(scene, Component{Style: test.style}))
		if got != test.want {
			t.Errorf("style %d produced %d inner hexagons, want %d", test.style, got, test.want)
		}
	}
}

func TestLegacyComponentDetailKeepsOriginalInnerHexagons(t *testing.T) {
	got := componentInnerScales(Scene{}, Component{Detail: ComponentDetailNestedHexagons})
	want := []float64{largeInnerDetailScale, smallInnerDetailScale}
	if len(got) != len(want) {
		t.Fatalf("got %d legacy inner hexagon scales, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("legacy inner scale %d = %v, want %v", i, got[i], want[i])
		}
	}
}
