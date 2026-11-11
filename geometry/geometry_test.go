package geometry

import (
	"math"
	"reflect"
	"testing"
)

func TestHexVertices(t *testing.T) {
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
			vertices := HexVertices(0, 0, radius, tt.orientation)
			if len(vertices) != 6 {
				t.Fatalf("got %d vertices, want 6", len(vertices))
			}
			if math.Hypot(vertices[0].X-tt.firstWant.X, vertices[0].Y-tt.firstWant.Y) > 1e-9 {
				t.Errorf("first vertex is %v, want %v", vertices[0], tt.firstWant)
			}
			for i, vertex := range vertices {
				if got := math.Hypot(vertex.X, vertex.Y); math.Abs(got-radius) > 1e-9 {
					t.Errorf("vertex %d is %.12f from the center, want radius %.12f", i, got, radius)
				}
				next := vertices[(i+1)%len(vertices)]
				if got := math.Hypot(vertex.X-next.X, vertex.Y-next.Y); math.Abs(got-radius) > 1e-9 {
					t.Errorf("side %d has length %.12f, want %.12f", i, got, radius)
				}
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
			if got := SegmentsIntersect(tt.a, tt.b, tt.c, tt.d); got != tt.want {
				t.Errorf("SegmentsIntersect() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestTrimEndpointDoesNotMutateInput(t *testing.T) {
	track := []Point{{X: 0, Y: 0}, {X: 2, Y: 0}}
	original := append([]Point(nil), track...)

	trimmed := TrimEndpoint(track, 0.5)
	wantTrimmed := []Point{{X: 0, Y: 0}, {X: 1.5, Y: 0}}
	if !reflect.DeepEqual(trimmed, wantTrimmed) {
		t.Fatalf("TrimEndpoint() = %v, want %v", trimmed, wantTrimmed)
	}
	if !reflect.DeepEqual(track, original) {
		t.Fatalf("TrimEndpoint() changed the input: got %v, want original %v", track, original)
	}

	trimmed[0].X = 99
	if !reflect.DeepEqual(track, original) {
		t.Fatalf("modifying the result changed the input: got %v, want original %v", track, original)
	}
}

func TestExternalLabelCandidatesAnchorFacingCornerToSide(t *testing.T) {
	component := Component{
		Index:  7,
		Radius: 0.4,
		X:      5,
		Y:      4,
	}
	const (
		width   = 3.5
		height  = 0.8
		gap     = 0.025
		padding = 0.005
	)
	baseAngle := -math.Pi / 3
	positions := ExternalLabelCandidates(component, "Example 1", TextMetrics{}, width, height, gap, baseAngle, padding)
	if len(positions) != 6 {
		t.Fatalf("got %d candidates, want 6", len(positions))
	}

	apothem := component.Radius * math.Sqrt(3) / 2
	for side, candidate := range positions {
		theta := baseAngle + float64(side)*math.Pi/3
		normal := Point{X: math.Cos(theta), Y: math.Sin(theta)}
		facePoint := Point{
			X: component.X + apothem*normal.X,
			Y: component.Y + apothem*normal.Y,
		}
		facingCorner := Point{
			X: midpointFacingAxis(candidate.Bounds.MinX, candidate.Bounds.MaxX, normal.X),
			Y: midpointFacingAxis(candidate.Bounds.MinY, candidate.Bounds.MaxY, normal.Y),
		}
		want := Point{X: facePoint.X + normal.X*gap, Y: facePoint.Y + normal.Y*gap}
		if got := math.Hypot(facingCorner.X-want.X, facingCorner.Y-want.Y); got > 1e-9 {
			t.Errorf("side %d: facing corner is %.9f inches from target, want <= 1e-9", side, got)
		}
	}
}

func midpointFacingAxis(min, max, normal float64) float64 {
	switch AxisSign(normal) {
	case 1:
		return min
	case -1:
		return max
	default:
		return (min + max) / 2
	}
}
