// Package hexgrid provides the grid-coordinate scene API for hexagonal artwork.
// It owns axial positions, grid composition types, and the high-level renderer.
// Inch-based geometry primitives and the arbitrary-geometry SVG renderer live in
// github.com/sector-f/hexgrid/geometry.
package hexgrid

import (
	"math"

	"github.com/sector-f/hexgrid/geometry"
)

// GridPosition identifies a cell in the hexagonal lattice using axial coordinates.
// For pointy-top layouts, positive Q points east and positive R points southeast.
// The origin (0, 0) is at the center of the canvas.
type GridPosition struct {
	Q, R int
}

// Cell is one axial grid cell and its position in model-space inches.
type Cell struct {
	Q, R   int
	Center Point
}

// ComponentStyle selects how many concentric hexagons make up a component.
type ComponentStyle = geometry.ComponentStyle

// Keep the style constants available from the high-level package as well. Their
// values and type are exactly the corresponding geometry package constants.
const (
	ComponentStyleSingle ComponentStyle = geometry.ComponentStyleSingle
	ComponentStyleDouble ComponentStyle = geometry.ComponentStyleDouble
	ComponentStyleTriple ComponentStyle = geometry.ComponentStyleTriple
)

// Grid describes the visible hexagonal lattice. Dimensions are in inches.
type Grid = geometry.Grid

// Point, Rect, and label types are shared with the lower-level geometry package.
type Point = geometry.Point
type Rect = geometry.Rect
type TextMetrics = geometry.TextMetrics
type Label = geometry.Label
type LabelRequest = geometry.LabelRequest

// GridCells returns lattice cells whose centers fall within the canvas rectangle inset
// by margin. A negative margin deliberately includes off-canvas cells so the grid can
// continue seamlessly to the image edges. Cells are returned in a stable scan order.
func GridCells(width, height, pitch float64, orientation string, margin float64) []Cell {
	if pitch <= 0 || math.IsNaN(pitch) || math.IsInf(pitch, 0) ||
		width < 0 || height < 0 || math.IsNaN(width) || math.IsNaN(height) ||
		math.IsInf(width, 0) || math.IsInf(height, 0) {
		return nil
	}
	limit := int(math.Ceil((width+height)/pitch)) + 3
	result := make([]Cell, 0)
	for q := -limit; q <= limit; q++ {
		for r := -limit; r <= limit; r++ {
			position := GridPosition{Q: q, R: r}
			if orientation != "flat" {
				// Keep the established scan order (and therefore SVG element order)
				// while returning coordinates in the conventional pointy-top basis.
				position = GridPosition{Q: q + r, R: -q}
			}
			center := GridPositionToPoint(position, width, height, pitch, orientation)
			if margin <= center.X && center.X <= width-margin && margin <= center.Y && center.Y <= height-margin {
				result = append(result, Cell{Q: position.Q, R: position.R, Center: center})
			}
		}
	}
	return result
}

func dot(a, b Point) float64 {
	return a.X*b.X + a.Y*b.Y
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
