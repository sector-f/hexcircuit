package hexgrid

import (
	"math"
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

func TestGridPositionToPointUsesCanvasCenterAsOrigin(t *testing.T) {
	center := GridPositionToPoint(GridPosition{}, 10, 8, 0.5, "pointy")
	if center != (Point{X: 5, Y: 4}) {
		t.Fatalf("origin = %+v, want center of canvas (5, 4)", center)
	}
	q := GridPositionToPoint(GridPosition{Q: 1}, 10, 8, 0.5, "pointy")
	if q.X == center.X || q.Y == center.Y {
		t.Fatalf("q-axis did not follow pointy orientation: center=%+v q=%+v", center, q)
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

func TestGridSceneRejectsInvalidTracePath(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
			{Position: GridPosition{Q: 2, R: 0}, Style: ComponentStyleSingle},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path:  []GridPosition{{Q: 2, R: 0}},
			Lines: 1,
		}},
	}
	if _, err := BuildScene(layout); err == nil {
		t.Fatal("BuildScene() accepted a path that skips a neighboring cell")
	}
}

func TestGridSceneRejectsMissingEndpointComponent(t *testing.T) {
	opts := DefaultOptions()
	layout := GridScene{
		Options: opts,
		Components: []GridComponent{
			{Position: GridPosition{Q: 0, R: 0}, Style: ComponentStyleSingle},
		},
		Traces: []GridTrace{{
			Start: GridPosition{Q: 0, R: 0},
			Path:  []GridPosition{{Q: 1, R: 0}},
			Lines: 1,
		}},
	}
	if _, err := BuildScene(layout); err == nil {
		t.Fatal("BuildScene() accepted a trace whose endpoint has no component")
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
