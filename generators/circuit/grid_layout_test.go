package circuit

import (
	"fmt"
	"math"
	"strings"
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

	svg, err := hexgrid.Render(layout)
	if err != nil {
		t.Fatalf("Render() returned an error for converted circuit scene: %v", err)
	}
	componentLayer := svgLayerContents(t, svg, "components")
	traceLayer := svgLayerContents(t, svg, "traces")
	terminalLayer := svgLayerContents(t, svg, "terminal-pads")
	if got, want := strings.Count(componentLayer, "<polygon"), 2; got != want {
		t.Errorf("rendered component count = %d, want %d", got, want)
	}
	if got, want := strings.Count(traceLayer, "<polyline"), 2; got != want {
		t.Errorf("rendered trace count = %d, want %d", got, want)
	}
	if got, want := strings.Count(terminalLayer, "<circle"), 2; got != want {
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
	scene := hexgrid.Scene{
		Options: hexgrid.Options{ComponentRadius: 0.4},
		Components: []hexgrid.GridComponent{{
			Position: hexgrid.GridPosition{},
			Style: hexgrid.ComponentStyleSingle,
			RadiusScale: 1,
		}},
	}

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
	for line := range legacyTracks {
		offset := (float64(line) - 0.5) * options.ParallelSpacing
		legacyTracks[line] = offsetPolyline(routePoints, offset)
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
	svg, err := hexgrid.Render(layout)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	traceLayer := svgLayerContents(t, svg, "traces")
	for line, track := range legacyTracks {
		rendered := trimTestTraceEndpoint(track, options.NodeRadius)
		expected := expectedSVGPolyline(rendered)
		if !strings.Contains(traceLayer, expected) {
			t.Errorf("rendered line %d does not contain router-equivalent points %q", line, expected)
		}
	}
}

func svgLayerContents(t *testing.T, svg, id string) string {
	t.Helper()
	startTag := `<g id="` + id + `"`
	start := strings.Index(svg, startTag)
	if start < 0 {
		t.Fatalf("SVG is missing layer %q", id)
	}
	end := strings.Index(svg[start:], "</g>")
	if end < 0 {
		t.Fatalf("SVG layer %q is not closed", id)
	}
	return svg[start : start+end]
}

func trimTestTraceEndpoint(points []Point, radius float64) []Point {
	trimmed := append([]Point(nil), points...)
	if len(trimmed) < 2 {
		return trimmed
	}
	end, previous := trimmed[len(trimmed)-1], trimmed[len(trimmed)-2]
	dx, dy := end.X-previous.X, end.Y-previous.Y
	length := math.Hypot(dx, dy)
	if length <= radius {
		return trimmed[:len(trimmed)-1]
	}
	trimmed[len(trimmed)-1] = Point{
		X: end.X - dx/length*radius,
		Y: end.Y - dy/length*radius,
	}
	return trimmed
}

func expectedSVGPolyline(points []Point) string {
	coordinates := make([]string, len(points))
	for i, point := range points {
		coordinates[i] = fmt.Sprintf("%d,%d", int(math.Round(point.X*1000)), int(math.Round(point.Y*1000)))
	}
	return `points="` + strings.Join(coordinates, " ") + `"`
}
