// Package hexgrid provides the grid-coordinate scene API for hexagonal artwork.
package hexgrid

import "math"

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
