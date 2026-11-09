package circuit

import (
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestGridSceneForRenderUsesAxialComponentsAndGridTraces(t *testing.T) {
	options := DefaultOptions()
	options.Width = 8
	options.Height = 6
	options.ComponentRadius = 0.4
	options.RoutingPitch = 0.62
	options.ParallelSpacing = 0.035

	components := []Component{
		{
			HexComponent: HexComponent{Index: 0, Radius: 0.3},
			Q:            0, R: 0, RadiusScale: 0.75,
		},
		{
			HexComponent: HexComponent{Index: 1, Radius: 0.4},
			Q:            2, R: 2, RadiusScale: 1,
		},
	}
	connections := []AcceptedConnection{
		{
			Source: 0,
			Target: 1,
			Path: []Coord{
				{Q: 0, R: 0},
				{Q: 1, R: 0},
				{Q: 2, R: 0},
				{Q: 2, R: 1},
				{Q: 2, R: 2},
			},
			Lines: 2,
		},
	}

	layout, err := gridSceneForRender(options, components, connections)
	if err != nil {
		t.Fatalf("gridSceneForRender() error = %v", err)
	}
	if len(layout.Components) != 2 {
		t.Fatalf("got %d grid components, want 2", len(layout.Components))
	}
	if got, want := layout.Components[0].Position, (hexgrid.GridPosition{Q: 0, R: 0}); got != want {
		t.Errorf("source component position = %+v, want %+v", got, want)
	}
	if got, want := layout.Components[0].Style, hexgrid.ComponentStyleSingle; got != want {
		t.Errorf("initial component style = %v, want %v", got, want)
	}
	if got, want := layout.Components[0].RadiusScale, 0.75; got != want {
		t.Errorf("component radius scale = %v, want %v", got, want)
	}
	if len(layout.Traces) != 1 {
		t.Fatalf("got %d grid traces, want 1", len(layout.Traces))
	}
	trace := layout.Traces[0]
	if got, want := trace.Start, (hexgrid.GridPosition{Q: 0, R: 0}); got != want {
		t.Errorf("trace start = %+v, want %+v", got, want)
	}
	if got, want := trace.Lines, 2; got != want {
		t.Errorf("trace line count = %d, want %d", got, want)
	}
	if got, want := len(trace.Path), 4; got != want {
		t.Fatalf("trace path length = %d, want %d", got, want)
	}
	if got, want := trace.Path[len(trace.Path)-1], (hexgrid.GridPosition{Q: 2, R: 2}); got != want {
		t.Errorf("trace destination = %+v, want %+v", got, want)
	}

	scene, err := hexgrid.BuildScene(layout)
	if err != nil {
		t.Fatalf("BuildScene returned an error for converted circuit layout: %v", err)
	}
	if got, want := len(scene.Components), 2; got != want {
		t.Errorf("rendered component count = %d, want %d", got, want)
	}
	if got, want := len(scene.Traces), 2; got != want {
		t.Errorf("rendered trace count = %d, want %d", got, want)
	}
	if got, want := len(scene.Terminals), 2; got != want {
		t.Errorf("rendered terminal count = %d, want %d", got, want)
	}
}

func TestAssignComponentDetailsUsesTripleGridStyle(t *testing.T) {
	options := DefaultOptions()
	options.ComponentRadius = 0.4
	options.DetailProb = 1

	components := []Component{{
		HexComponent: HexComponent{Index: 0, Radius: 0.4},
		Hub:          true,
	}}
	scene := hexgrid.Scene{Components: []hexgrid.Component{{
		Index:  0,
		Radius: 0.4,
		Style:  hexgrid.ComponentStyleSingle,
	}}}

	assignComponentDetails(options, components, &scene, 42)
	if got, want := scene.Components[0].Style, hexgrid.ComponentStyleTriple; got != want {
		t.Errorf("component style after detail assignment = %v, want %v", got, want)
	}
}

func TestGridTraceGeometryMatchesRouterGeometry(t *testing.T) {
	options := DefaultOptions()
	options.Width = 8
	options.Height = 6

	path := []Coord{
		{Q: 0, R: 0},
		{Q: 1, R: 0},
		{Q: 2, R: 0},
		{Q: 2, R: 1},
		{Q: 2, R: 2},
	}
	baseAngle := degreesToRadians(-60)
	components := []Component{
		{
			HexComponent: HexComponent{Index: 0, X: options.Width / 2, Y: options.Height / 2, Radius: options.ComponentRadius},
			Q:            0, R: 0, RadiusScale: 1,
		},
		{
			HexComponent: HexComponent{
				Index:  1,
				X:      coordToXY(path[len(path)-1], options.Width, options.Height, options.RoutingPitch, options.Orientation).X,
				Y:      coordToXY(path[len(path)-1], options.Width, options.Height, options.RoutingPitch, options.Orientation).Y,
				Radius: options.ComponentRadius,
			},
			Q: 2, R: 2, RadiusScale: 1,
		},
	}

	// Reconstruct the router's established centerline and parallel tracks.
	routePoints := []Point{faceMidpoint(components[0], 1, baseAngle)}
	for _, coord := range path[1:] {
		routePoints = append(routePoints, coordToXY(coord, options.Width, options.Height, options.RoutingPitch, options.Orientation))
	}
	legacyTracks := make([][]Point, 2)
	legacyEndpoints := make([]Point, 2)
	for line := range legacyTracks {
		offset := (float64(line) - 0.5) * options.ParallelSpacing
		legacyTracks[line] = offsetPolyline(routePoints, offset)
		legacyEndpoints[line] = legacyTracks[line][len(legacyTracks[line])-1]
	}

	connection := AcceptedConnection{
		Source: 0, Target: 1,
		Path: path, Lines: len(legacyTracks),
		Tracks: legacyTracks, Endpoints: legacyEndpoints,
	}
	layout, err := gridSceneForRender(options, components, []AcceptedConnection{connection})
	if err != nil {
		t.Fatalf("gridSceneForRender() error = %v", err)
	}
	scene, err := hexgrid.BuildScene(layout)
	if err != nil {
		t.Fatalf("BuildScene() error = %v", err)
	}
	if len(scene.Traces) != len(legacyTracks) {
		t.Fatalf("got %d built traces, want %d", len(scene.Traces), len(legacyTracks))
	}
	for line := range legacyTracks {
		got, want := scene.Traces[line].Points, legacyTracks[line]
		if len(got) != len(want) {
			t.Fatalf("line %d has %d points after grid conversion, want %d", line, len(got), len(want))
		}
		for pointIndex := range want {
			if distance(got[pointIndex], want[pointIndex]) > 2e-9 {
				t.Errorf("line %d point %d changed during grid conversion: got %+v, want %+v", line, pointIndex, got[pointIndex], want[pointIndex])
			}
		}
	}
	for line, want := range legacyEndpoints {
		got := scene.Terminals[line]
		if distance(Point{X: got.X, Y: got.Y}, want) > 2e-9 {
			t.Errorf("terminal %d changed during grid conversion: got (%f, %f), want %+v", line, got.X, got.Y, want)
		}
	}
}
