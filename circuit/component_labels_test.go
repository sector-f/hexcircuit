package circuit

import (
	"math"
	"testing"
)

func TestExternalLabelCandidatesAnchorFacingCornerToSide(t *testing.T) {
	component := Component{
		HexComponent: HexComponent{
			Index:  7,
			Radius: 0.4,
			X:      5,
			Y:      4,
		},
	}
	const (
		width  = 3.5
		height = 0.8
		gap    = 0.025
	)
	baseAngle := degreesToRadians(-60)
	positions := externalLabelCandidates(component, "Example 1", labelMetrics{}, width, height, gap, baseAngle)
	if len(positions) != 6 {
		t.Fatalf("got %d candidates, want 6", len(positions))
	}

	apothem := component.Radius * sqrt3 / 2
	for side, candidate := range positions {
		theta := directionAngle(baseAngle, side)
		normal := Point{X: math.Cos(theta), Y: math.Sin(theta)}
		facePoint := Point{
			X: component.X + apothem*normal.X,
			Y: component.Y + apothem*normal.Y,
		}
		facingCorner := Point{
			X: midpointFacingAxis(candidate.Bounds.MinX, candidate.Bounds.MaxX, normal.X),
			Y: midpointFacingAxis(candidate.Bounds.MinY, candidate.Bounds.MaxY, normal.Y),
		}
		want := add(facePoint, mul(normal, gap))
		if got := distance(facingCorner, want); got > 1e-9 {
			t.Errorf("side %d: facing corner is %.9f inches from target, want <= 1e-9", side, got)
		}
	}
}

func midpointFacingAxis(min, max, normal float64) float64 {
	switch axisSign(normal) {
	case 1:
		return min
	case -1:
		return max
	default:
		return (min + max) / 2
	}
}
