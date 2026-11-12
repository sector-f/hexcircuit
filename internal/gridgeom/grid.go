// Package gridgeom contains repository-internal conversions between axial grid
// positions and physical drawing coordinates. It is shared by hexgrid and generators;
// it is not part of the public grid API.
package gridgeom

import (
	"math"

	"github.com/sector-f/hexgrid/geometry"
)

type Cell struct {
	Q, R   int
	Center geometry.Point
}

func PositionToPoint(qPosition, rPosition int, width, height, pitch float64, orientation string) geometry.Point {
	baseAngle := orientationBaseAngle(orientation)
	q, r := float64(qPosition), float64(rPosition)
	if orientation != "flat" {
		q, r = float64(-rPosition), float64(qPosition+rPosition)
	}
	theta1 := baseAngle + math.Pi/3
	return geometry.Point{
		X: width/2 + pitch*(q*math.Cos(baseAngle)+r*math.Cos(theta1)),
		Y: height/2 + pitch*(q*math.Sin(baseAngle)+r*math.Sin(theta1)),
	}
}

func Cells(width, height, pitch float64, orientation string, margin float64) []Cell {
	if pitch <= 0 || math.IsNaN(pitch) || math.IsInf(pitch, 0) ||
		width < 0 || height < 0 || math.IsNaN(width) || math.IsNaN(height) ||
		math.IsInf(width, 0) || math.IsInf(height, 0) {
		return nil
	}
	limit := int(math.Ceil((width+height)/pitch)) + 3
	result := make([]Cell, 0)
	for q := -limit; q <= limit; q++ {
		for r := -limit; r <= limit; r++ {
			positionQ, positionR := q, r
			if orientation != "flat" {
				positionQ, positionR = q+r, -q
			}
			center := PositionToPoint(positionQ, positionR, width, height, pitch, orientation)
			if margin <= center.X && center.X <= width-margin && margin <= center.Y && center.Y <= height-margin {
				result = append(result, Cell{Q: positionQ, R: positionR, Center: center})
			}
		}
	}
	return result
}

func orientationBaseAngle(orientation string) float64 {
	if orientation == "flat" {
		return 30 * math.Pi / 180
	}
	return -60 * math.Pi / 180
}
