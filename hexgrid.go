// Package hexgrid provides a grid-coordinate scene API for hexagonal artwork.
// It owns axial positions, grid composition types, and the high-level renderer.
// Physical geometry and SVG primitives are implemented by the geometry subpackage.
package hexgrid

// GridPosition identifies a cell in the hexagonal lattice using axial coordinates.
// For pointy-top layouts, positive Q points east and positive R points southeast.
// The origin (0, 0) is at the center of the canvas.
type GridPosition struct {
	Q, R int
}

// hexSideCount is the number of sides on each cell in the grid.
const hexSideCount = 6

// ComponentStyle selects how many concentric hexagon outlines make up a component.
type ComponentStyle uint8

const (
	ComponentStyleSingle ComponentStyle = iota + 1
	ComponentStyleDouble
	ComponentStyleTriple
)
