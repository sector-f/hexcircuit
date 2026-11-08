// Package circuitboard generates a hand-composed, hexagonal PCB illustration.
package circuitboard

import (
	"fmt"
	"math"
	"strings"
)

const (
	defaultWidth  = 13.5
	defaultHeight = 19.0

	panelClearance = 0.10
)

// Options contains the settings used to generate the circuit-board SVG.
// All dimensions are expressed in inches. The board uses a pointy-top axial
// lattice and a tapered panel outline.
type Options struct {
	// Width and Height are the physical dimensions of the SVG canvas.
	Width  float64
	Height float64
	// TopWidth and BottomWidth define the centered, tapered board outline.
	TopWidth    float64
	BottomWidth float64

	// BaseColor is used for the main components, traces, and optional hex field.
	BaseColor string
	// AccentColor is used for power-feed traces and selected component cores.
	AccentColor string
	// Background is the panel fill; "none" or "transparent" leaves it transparent.
	Background string

	// Pitch is the center-to-center spacing of adjacent cells in the layout lattice.
	Pitch float64
	// ComponentRadius is the maximum circumradius of a component hexagon.
	ComponentRadius float64
	// ComponentStrokeWidth is the outline width of components, in inches.
	ComponentStrokeWidth float64
	// ComponentMiddleHexagonScale and ComponentInnerHexagonScale control the
	// relative sizes of the inner component outlines.
	ComponentMiddleHexagonScale float64
	ComponentInnerHexagonScale  float64
	// TraceStrokeWidth is the width of each circuit trace, in inches.
	TraceStrokeWidth float64
	// TraceSpacing is the perpendicular spacing between parallel traces, in inches.
	TraceSpacing float64
	// NodeRadius is the radius of the circular trace terminals, in inches.
	NodeRadius float64

	// ShowBackgroundGrid enables the low-contrast hexagonal field. It is off by default.
	ShowBackgroundGrid bool
	// GridStrokeWidth is the outline width of background hexagons, in inches.
	GridStrokeWidth float64

	// ConnectorCount is the number of contacts in the fixed bottom-edge connector row.
	ConnectorCount int
	// ConnectorInset is the distance from the bottom edge to the connector centers.
	ConnectorInset float64
}

// DefaultOptions returns the default settings for the circuit-board illustration.
func DefaultOptions() Options {
	return Options{
		Width:                       defaultWidth,
		Height:                      defaultHeight,
		TopWidth:                    13.5,
		BottomWidth:                 8.5,
		BaseColor:                   "#396CF8",
		AccentColor:                 "#FF6A00",
		Background:                  "none",
		Pitch:                       0.58,
		ComponentRadius:             0.28,
		ComponentStrokeWidth:        0.014,
		ComponentMiddleHexagonScale: 0.60,
		ComponentInnerHexagonScale:  0.34,
		TraceStrokeWidth:            0.010,
		TraceSpacing:                0.035,
		NodeRadius:                  0.018,
		ShowBackgroundGrid:          false,
		GridStrokeWidth:             0.0035,
		ConnectorCount:              12,
		ConnectorInset:              2.0,
	}
}

func normalizeOptions(o Options) (Options, error) {
	if strings.TrimSpace(o.BaseColor) == "" {
		return o, fmt.Errorf("base color cannot be empty")
	}
	if strings.TrimSpace(o.AccentColor) == "" {
		return o, fmt.Errorf("accent color cannot be empty")
	}
	o.BaseColor = strings.TrimSpace(o.BaseColor)
	o.AccentColor = strings.TrimSpace(o.AccentColor)
	if strings.TrimSpace(o.Background) == "" {
		o.Background = "none"
	}

	positive := []struct {
		name  string
		value float64
	}{
		{"width", o.Width},
		{"height", o.Height},
		{"top width", o.TopWidth},
		{"bottom width", o.BottomWidth},
		{"pitch", o.Pitch},
		{"component radius", o.ComponentRadius},
		{"component stroke width", o.ComponentStrokeWidth},
		{"trace stroke width", o.TraceStrokeWidth},
		{"node radius", o.NodeRadius},
		{"connector inset", o.ConnectorInset},
	}
	for _, item := range positive {
		if !isFinite(item.value) || item.value <= 0 {
			return o, fmt.Errorf("%s must be finite and greater than zero", item.name)
		}
	}
	if o.TopWidth > o.Width || o.BottomWidth > o.Width {
		return o, fmt.Errorf("top and bottom widths must not exceed the canvas width")
	}
	if o.BottomWidth > o.TopWidth {
		return o, fmt.Errorf("bottom width must not exceed top width")
	}
	if !isFinite(o.GridStrokeWidth) || o.GridStrokeWidth < 0 {
		return o, fmt.Errorf("grid stroke width must be finite and greater than or equal to zero")
	}
	if !isFinite(o.TraceSpacing) || o.TraceSpacing < 0 {
		return o, fmt.Errorf("trace spacing must be finite and greater than or equal to zero")
	}
	if o.TraceSpacing == 0 {
		return o, fmt.Errorf("trace spacing must be greater than zero")
	}
	if !isFinite(o.ComponentMiddleHexagonScale) || !isFinite(o.ComponentInnerHexagonScale) ||
		o.ComponentMiddleHexagonScale <= 0 || o.ComponentMiddleHexagonScale >= 1 ||
		o.ComponentInnerHexagonScale <= 0 || o.ComponentInnerHexagonScale >= o.ComponentMiddleHexagonScale {
		return o, fmt.Errorf("component inner scales must satisfy 0 < inner < middle < 1")
	}
	if o.ConnectorCount < 1 {
		return o, fmt.Errorf("connector count must be at least 1")
	}
	if o.ConnectorInset >= o.Height/2 {
		return o, fmt.Errorf("connector inset must be less than half the canvas height")
	}

	// Reject dimensions that would force fixed board components or connector
	// contacts outside the tapered panel instead of silently dropping them.
	for _, position := range fixedComponentPositions() {
		if !withinPanel(o, position, o.ComponentRadius, panelClearance) {
			return o, fmt.Errorf("fixed board layout does not fit the configured panel at grid position (%d, %d)", position.Q, position.R)
		}
	}
	for _, position := range connectorPositions(o) {
		if !withinPanel(o, position, o.ComponentRadius*0.62, panelClearance) {
			return o, fmt.Errorf("bottom connector row does not fit the configured panel; reduce connector count or increase panel width")
		}
	}

	return o, nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
