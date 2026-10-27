package circuit

import (
	"encoding/xml"
	"io"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// closeEnough compares floating-point geometry using an absolute tolerance.
// The tests use simple coordinates, so a small absolute tolerance is sufficient.
func closeEnough(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}

func TestAxialDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b Coord
		want int
	}{
		{name: "same cell", a: Coord{0, 0}, b: Coord{0, 0}, want: 0},
		{name: "one step along q", a: Coord{0, 0}, b: Coord{1, 0}, want: 1},
		{name: "one step along r", a: Coord{0, 0}, b: Coord{0, 1}, want: 1},
		{name: "one diagonal step", a: Coord{0, 0}, b: Coord{-1, 1}, want: 1},
		{name: "multiple steps", a: Coord{0, 0}, b: Coord{3, -2}, want: 3},
		{name: "distance is translation invariant", a: Coord{-4, 3}, b: Coord{-2, 1}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := axialDistance(tt.a, tt.b); got != tt.want {
				t.Errorf("axialDistance(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsAxisAligned(t *testing.T) {
	origin := Coord{0, 0}

	// Every legal lattice step follows one of the six principal directions.
	for direction, step := range axialSteps {
		if !isAxisAligned(origin, step) {
			t.Errorf("direction %d (%v) should be axis-aligned", direction, step)
		}
	}

	if isAxisAligned(origin, Coord{Q: 2, R: 1}) {
		t.Error("coordinate (2, 1) should not be axis-aligned with the origin")
	}
}

func TestCoordToXY(t *testing.T) {
	const (
		width  = 12.0
		height = 8.0
		pitch  = 0.7
	)
	baseAngle := degreesToRadians(-60)

	origin := coordToXY(Coord{}, width, height, pitch, baseAngle)
	if !closeEnough(origin.X, width/2, 1e-9) || !closeEnough(origin.Y, height/2, 1e-9) {
		t.Fatalf("origin maps to (%f, %f), want canvas center (%f, %f)", origin.X, origin.Y, width/2, height/2)
	}

	// Each of the six neighboring lattice cells must be exactly one pitch away.
	for direction, step := range axialSteps {
		neighbor := coordToXY(step, width, height, pitch, baseAngle)
		if got := distance(origin, neighbor); !closeEnough(got, pitch, 1e-9) {
			t.Errorf("neighbor in direction %d is %.12f inches away, want %.12f", direction, got, pitch)
		}
	}
}

func TestGetHexVertices(t *testing.T) {
	const radius = 2.0
	tests := []struct {
		name        string
		orientation string
		firstWant   Point
	}{
		{name: "pointy has a top vertex", orientation: "pointy", firstWant: Point{X: 0, Y: -radius}},
		{name: "flat has a right vertex", orientation: "flat", firstWant: Point{X: radius, Y: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vertices := getHexVertices(0, 0, radius, tt.orientation)
			if len(vertices) != 6 {
				t.Fatalf("got %d vertices, want 6", len(vertices))
			}

			if distance(vertices[0], tt.firstWant) > 1e-9 {
				t.Errorf("first vertex is %v, want %v", vertices[0], tt.firstWant)
			}

			// All corners of a regular hexagon are one circumradius from its center.
			for i, vertex := range vertices {
				if got := distance(vertex, Point{}); !closeEnough(got, radius, 1e-9) {
					t.Errorf("vertex %d is %.12f from the center, want radius %.12f", i, got, radius)
				}
			}

			// All six sides of a regular hexagon have equal length.
			for i := range vertices {
				next := vertices[(i+1)%len(vertices)]
				if got := distance(vertices[i], next); !closeEnough(got, radius, 1e-9) {
					t.Errorf("side %d has length %.12f, want %.12f", i, got, radius)
				}
			}
		})
	}
}

func TestPointSegmentDistance(t *testing.T) {
	a := Point{X: 0, Y: 0}
	b := Point{X: 2, Y: 0}

	tests := []struct {
		name string
		p    Point
		a, b Point
		want float64
	}{
		{name: "projection lands inside segment", p: Point{X: 1, Y: 1}, a: a, b: b, want: 1},
		{name: "projection before start clamps to start", p: Point{X: -1, Y: 0}, a: a, b: b, want: 1},
		{name: "projection after end clamps to end", p: Point{X: 3, Y: 0}, a: a, b: b, want: 1},
		{name: "zero-length segment", p: Point{X: 3, Y: 4}, a: Point{X: 0, Y: 0}, b: Point{X: 0, Y: 0}, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pointSegmentDistance(tt.p, tt.a, tt.b); !closeEnough(got, tt.want, 1e-9) {
				t.Errorf("pointSegmentDistance() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestSegmentsIntersect(t *testing.T) {
	tests := []struct {
		name string
		a, b Point
		c, d Point
		want bool
	}{
		{
			name: "crossing segments",
			a:    Point{X: 0, Y: 0}, b: Point{X: 2, Y: 0},
			c: Point{X: 1, Y: -1}, d: Point{X: 1, Y: 1}, want: true,
		},
		{
			name: "segments touch at an endpoint",
			a:    Point{X: 0, Y: 0}, b: Point{X: 1, Y: 0},
			c: Point{X: 1, Y: 0}, d: Point{X: 1, Y: 1}, want: true,
		},
		{
			name: "collinear segments overlap",
			a:    Point{X: 0, Y: 0}, b: Point{X: 3, Y: 0},
			c: Point{X: 2, Y: 0}, d: Point{X: 4, Y: 0}, want: true,
		},
		{
			name: "parallel segments do not touch",
			a:    Point{X: 0, Y: 0}, b: Point{X: 2, Y: 0},
			c: Point{X: 0, Y: 1}, d: Point{X: 2, Y: 1}, want: false,
		},
		{
			name: "separate collinear segments",
			a:    Point{X: 0, Y: 0}, b: Point{X: 1, Y: 0},
			c: Point{X: 2, Y: 0}, d: Point{X: 3, Y: 0}, want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := segmentsIntersect(tt.a, tt.b, tt.c, tt.d); got != tt.want {
				t.Errorf("segmentsIntersect() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSegmentSegmentDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b Point
		c, d Point
		want float64
	}{
		{
			name: "intersecting segments have zero distance",
			a:    Point{X: 0, Y: 0}, b: Point{X: 2, Y: 0},
			c: Point{X: 1, Y: -1}, d: Point{X: 1, Y: 1}, want: 0,
		},
		{
			name: "parallel segments have perpendicular separation",
			a:    Point{X: 0, Y: 0}, b: Point{X: 2, Y: 0},
			c: Point{X: 0, Y: 3}, d: Point{X: 2, Y: 3}, want: 3,
		},
		{
			name: "separated endpoints determine shortest distance",
			a:    Point{X: 0, Y: 0}, b: Point{X: 1, Y: 0},
			c: Point{X: 2, Y: 2}, d: Point{X: 2, Y: 3}, want: math.Sqrt(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := segmentSegmentDistance(tt.a, tt.b, tt.c, tt.d); !closeEnough(got, tt.want, 1e-9) {
				t.Errorf("segmentSegmentDistance() = %.12f, want %.12f", got, tt.want)
			}
		})
	}
}

func TestOffsetPolylineMiterAtRightAngle(t *testing.T) {
	// A left-hand offset of this L-shaped path should keep a square miter at the bend.
	path := []Point{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
	}

	got := offsetPolyline(path, 0.1)
	want := []Point{
		{X: 0, Y: 0.1},
		{X: 0.9, Y: 0.1},
		{X: 0.9, Y: 1},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d points, want %d", len(got), len(want))
	}
	for i := range want {
		if distance(got[i], want[i]) > 1e-9 {
			t.Errorf("point %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRouteHitsComponent(t *testing.T) {
	component := Component{HexComponent: HexComponent{Index: 0, X: 0, Y: 0, Radius: 1}}

	tests := []struct {
		name  string
		track []Point
		want  bool
	}{
		{
			name:  "line through component is rejected",
			track: []Point{{X: -2, Y: 0}, {X: 2, Y: 0}},
			want:  true,
		},
		{
			name:  "line with sufficient clearance is allowed",
			track: []Point{{X: -2, Y: 1.5}, {X: 2, Y: 1.5}},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeHitsComponent(tt.track, component, "pointy", 0.1); got != tt.want {
				t.Errorf("routeHitsComponent() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestTracksCollide(t *testing.T) {
	horizontal := []Point{{X: 0, Y: 0}, {X: 2, Y: 0}}
	crossing := []Point{{X: 1, Y: -1}, {X: 1, Y: 1}}
	separated := []Point{{X: 0, Y: 1}, {X: 2, Y: 1}}
	nearbyParallel := []Point{{X: 0, Y: 0.05}, {X: 2, Y: 0.05}}

	tests := []struct {
		name            string
		candidateTracks [][]Point
		existingTracks  [][]Point
		clearance       float64
		want            bool
	}{
		{
			name:            "candidate crossing an accepted track collides",
			candidateTracks: [][]Point{crossing},
			existingTracks:  [][]Point{horizontal},
			clearance:       0.1,
			want:            true,
		},
		{
			name:            "well-separated tracks do not collide",
			candidateTracks: [][]Point{separated},
			existingTracks:  [][]Point{horizontal},
			clearance:       0.1,
			want:            false,
		},
		{
			name:            "tracks in the same bundle must have spacing",
			candidateTracks: [][]Point{horizontal, nearbyParallel},
			clearance:       0.1,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tracksCollide(tt.candidateTracks, tt.existingTracks, tt.clearance); got != tt.want {
				t.Errorf("tracksCollide() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestFindRouteForSide(t *testing.T) {
	const (
		wantMinimumLatticeMoves = 3
		width                   = 20.0
		height                  = 20.0
		pitch                   = 1.0
	)
	if minLatticeMoves != wantMinimumLatticeMoves {
		t.Fatalf("minLatticeMoves = %d, want %d", minLatticeMoves, wantMinimumLatticeMoves)
	}
	baseAngle := degreesToRadians(-60)

	// This destination requires at least one turn after leaving the source in direction 0.
	src := Component{HexComponent: HexComponent{Index: 0}, Q: 0, R: 0}
	dst := Component{HexComponent: HexComponent{Index: 1}, Q: 2, R: 1}
	components := []Component{src, dst}

	opts := DefaultOptions()
	opts.Width, opts.Height, opts.RoutingPitch = width, height, pitch
	opts.MaxRouteSteps = 24
	opts.UseDeterministicRouteCost = false
	rt := newRouter(opts, components, nil, baseAngle, rand.New(rand.NewSource(42)))
	path := rt.findRouteForSide(src, dst, nil, 0)
	if path == nil {
		t.Fatal("findRouteForSide() returned no route for a reachable destination")
	}
	if len(path) < minLatticeMoves+1 {
		t.Fatalf("route has %d coordinates, want at least %d for %d lattice moves", len(path), minLatticeMoves+1, minLatticeMoves)
	}
	if path[0] != src.Coord() {
		t.Errorf("route starts at %v, want source %v", path[0], src.Coord())
	}
	if path[1] != (Coord{Q: 1, R: 0}) {
		t.Errorf("first move is to %v, want the neighbor in direction 0", path[1])
	}
	if path[len(path)-1] != dst.Coord() {
		t.Errorf("route ends at %v, want destination %v", path[len(path)-1], dst.Coord())
	}

	hasBend := false
	lastDirection := -1
	for i := 0; i+1 < len(path); i++ {
		delta := Coord{Q: path[i+1].Q - path[i].Q, R: path[i+1].R - path[i].R}
		direction := -1
		for j, step := range axialSteps {
			if delta == step {
				direction = j
				break
			}
		}
		if direction == -1 {
			t.Fatalf("route step %v -> %v is not a legal neighboring-cell move", path[i], path[i+1])
		}
		if lastDirection >= 0 {
			deltaDirection := (direction - lastDirection + 6) % 6
			if deltaDirection != 0 && deltaDirection != 1 && deltaDirection != 5 {
				t.Fatalf("route turns from direction %d to %d, but only straight or 60-degree turns are allowed", lastDirection, direction)
			}
			if direction != lastDirection {
				hasBend = true
			}
		}
		lastDirection = direction
	}
	if !hasBend {
		t.Error("route contains no bend, but routed connections require at least one")
	}
}

func TestFindRouteForSideWithDeterministicCosts(t *testing.T) {
	const (
		width  = 20.0
		height = 20.0
		pitch  = 1.0
	)
	baseAngle := degreesToRadians(-60)
	src := Component{HexComponent: HexComponent{Index: 0}, Q: 0, R: 0}
	dst := Component{HexComponent: HexComponent{Index: 1}, Q: 2, R: 1}
	components := []Component{src, dst}

	opts := DefaultOptions()
	opts.Width, opts.Height, opts.RoutingPitch = width, height, pitch
	opts.MaxRouteSteps = 24
	opts.UseDeterministicRouteCost = true
	rt := newRouter(opts, components, nil, baseAngle, rand.New(rand.NewSource(42)))
	path := rt.findRouteForSide(src, dst, nil, 0)
	if path == nil {
		t.Fatal("findRouteForSide() returned no route with deterministic costs")
	}
	if len(path) < minLatticeMoves+1 {
		t.Fatalf("route has %d coordinates, want at least %d", len(path), minLatticeMoves+1)
	}
	if path[0] != src.Coord() || path[len(path)-1] != dst.Coord() {
		t.Fatalf("route endpoints are %v and %v, want %v and %v", path[0], path[len(path)-1], src.Coord(), dst.Coord())
	}
}

func TestRouteCostModels(t *testing.T) {
	const currentCost = 3.0

	// Both models charge one unit per move and a fixed penalty for a turn.
	wantBaseCost := currentCost + routeStepCost + routeTurnPenalty
	if got := nextDeterministicRouteCost(currentCost, true); got != wantBaseCost {
		t.Fatalf("nextDeterministicRouteCost() = %v, want %v", got, wantBaseCost)
	}

	// The legacy model adds one random jitter value to that same base cost.
	opts := DefaultOptions()
	rt := newRouter(opts, nil, nil, 0, rand.New(rand.NewSource(42)))
	wantJitter := rand.New(rand.NewSource(42)).Float64() * routeCostJitter
	if got := rt.nextLegacyRouteCost(currentCost, true); got != wantBaseCost+wantJitter {
		t.Fatalf("router.nextLegacyRouteCost() = %v, want base cost %v plus jitter %v", got, wantBaseCost, wantJitter)
	}
}

func TestDefaultOptionsPreserveLegacyRouteCosts(t *testing.T) {
	if DefaultOptions().UseDeterministicRouteCost {
		t.Fatal("DefaultOptions().UseDeterministicRouteCost = true, want false for backward compatibility")
	}
}

func TestWeightedChoiceReturnsSelectedIndex(t *testing.T) {
	// Only index 1 has a non-zero weight, so every random seed must select it.
	got := weightedChoice(rand.New(rand.NewSource(42)), []float64{0, 10, 0})
	if got != 1 {
		t.Fatalf("weightedChoice() = %d, want index 1", got)
	}
}

func TestTrimEndpointDoesNotMutateInput(t *testing.T) {
	track := []Point{{X: 0, Y: 0}, {X: 2, Y: 0}}
	original := append([]Point(nil), track...)

	trimmed := trimEndpoint(track, 0.5)
	wantTrimmed := []Point{{X: 0, Y: 0}, {X: 1.5, Y: 0}}
	if !reflect.DeepEqual(trimmed, wantTrimmed) {
		t.Fatalf("trimEndpoint() = %v, want %v", trimmed, wantTrimmed)
	}
	if !reflect.DeepEqual(track, original) {
		t.Fatalf("trimEndpoint() changed the input: got %v, want original %v", track, original)
	}

	// The result must not share its backing array with the input either.
	trimmed[0].X = 99
	if !reflect.DeepEqual(track, original) {
		t.Fatalf("modifying the result changed the input: got %v, want original %v", track, original)
	}
}

func TestChooseComponentsPlacesUniqueComponents(t *testing.T) {
	opts := DefaultOptions()
	baseAngle := degreesToRadians(-60)

	components, edges, err := chooseComponents(opts, rand.New(rand.NewSource(1234)), baseAngle)
	if err != nil {
		t.Fatalf("chooseComponents() returned an error: %v", err)
	}
	if len(components) != opts.ComponentCount {
		t.Fatalf("placed %d components, want %d", len(components), opts.ComponentCount)
	}

	seenCoords := make(map[Coord]bool, len(components))
	margin := opts.ComponentRadius + opts.TraceWidth + opts.NodeRadius
	for i, component := range components {
		if component.Index != i {
			t.Errorf("component at position %d has index %d", i, component.Index)
		}
		if seenCoords[component.Coord()] {
			t.Errorf("multiple components were placed at coordinate %v", component.Coord())
		}
		seenCoords[component.Coord()] = true

		if component.X < margin || component.X > opts.Width-margin ||
			component.Y < margin || component.Y > opts.Height-margin {
			t.Errorf("component %d center (%f, %f) is inside the canvas but too close to an edge", component.Index, component.X, component.Y)
		}
	}

	for _, edge := range edges {
		if edge[0] < 0 || edge[0] >= len(components) || edge[1] < 0 || edge[1] >= len(components) {
			t.Errorf("base edge %v refers to a nonexistent component", edge)
		}
		if edge[0] == edge[1] {
			t.Errorf("base edge %v connects a component to itself", edge)
		}
	}
}

func TestChooseComponentsIsRepeatableForFixedSeed(t *testing.T) {
	opts := DefaultOptions()
	baseAngle := degreesToRadians(-60)

	firstComponents, firstEdges, err := chooseComponents(opts, rand.New(rand.NewSource(9876)), baseAngle)
	if err != nil {
		t.Fatalf("first chooseComponents() call returned an error: %v", err)
	}
	secondComponents, secondEdges, err := chooseComponents(opts, rand.New(rand.NewSource(9876)), baseAngle)
	if err != nil {
		t.Fatalf("second chooseComponents() call returned an error: %v", err)
	}

	if !reflect.DeepEqual(firstComponents, secondComponents) {
		t.Error("component placement changed between calls with the same seed")
	}
	if !reflect.DeepEqual(firstEdges, secondEdges) {
		t.Error("preferred connections changed between calls with the same seed")
	}
}

func TestGenerateIsRepeatableWithDeterministicRouteCosts(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 20261025
	opts.SeedSet = true
	opts.UseDeterministicRouteCost = true

	firstSVG, firstAccepted, firstAttempts, err := Generate(opts)
	if err != nil {
		t.Fatalf("first Generate() call returned an error: %v", err)
	}
	secondSVG, secondAccepted, secondAttempts, err := Generate(opts)
	if err != nil {
		t.Fatalf("second Generate() call returned an error: %v", err)
	}

	if firstSVG != secondSVG {
		t.Error("deterministic-cost SVG output changed between calls with the same seed and options")
	}
	if firstAccepted != secondAccepted {
		t.Errorf("accepted connection count changed: first %d, second %d", firstAccepted, secondAccepted)
	}
	if firstAttempts != secondAttempts {
		t.Errorf("route-attempt count changed: first %d, second %d", firstAttempts, secondAttempts)
	}
}

func TestGenerateIsRepeatableForFixedSeed(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 20261025
	opts.SeedSet = true

	firstSVG, firstAccepted, firstAttempts, err := Generate(opts)
	if err != nil {
		t.Fatalf("first Generate() call returned an error: %v", err)
	}
	secondSVG, secondAccepted, secondAttempts, err := Generate(opts)
	if err != nil {
		t.Fatalf("second Generate() call returned an error: %v", err)
	}

	if firstSVG != secondSVG {
		t.Error("SVG output changed between calls with the same seed and options")
	}
	if firstAccepted != secondAccepted {
		t.Errorf("accepted connection count changed: first %d, second %d", firstAccepted, secondAccepted)
	}
	if firstAttempts != secondAttempts {
		t.Errorf("route-attempt count changed: first %d, second %d", firstAttempts, secondAttempts)
	}
	if firstAccepted < 0 || firstAccepted > opts.Connections {
		t.Errorf("accepted %d connections, outside the allowed range 0..%d", firstAccepted, opts.Connections)
	}
	if firstAttempts < firstAccepted {
		t.Errorf("made %d attempts but accepted %d connections", firstAttempts, firstAccepted)
	}
	if firstSVG == "" {
		t.Fatal("Generate() returned an empty SVG document")
	}

	// Parse the output as XML so malformed SVG cannot pass as a successful render.
	decoder := xml.NewDecoder(strings.NewReader(firstSVG))
	foundSVGRoot := false
	foundTraceLayer := false
	foundComponentLayer := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("generated SVG is not well-formed XML: %v", err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "svg" {
			foundSVGRoot = true
		}
		if start.Name.Local == "g" {
			for _, attr := range start.Attr {
				if attr.Name.Local != "id" {
					continue
				}
				switch attr.Value {
				case "traces":
					foundTraceLayer = true
				case "components":
					foundComponentLayer = true
				}
			}
		}
	}
	if !foundSVGRoot {
		t.Error("generated document has no <svg> root element")
	}
	if !foundTraceLayer {
		t.Error("generated document has no traces layer")
	}
	if !foundComponentLayer {
		t.Error("generated document has no components layer")
	}
}
