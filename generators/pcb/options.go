// Package pcb generates randomized PCB-inspired artwork on a hexagonal lattice.
package pcb

import (
	"fmt"
	"math"
	"strings"
)

const (
	defaultWidth         = 10.0
	defaultHeight        = 8.0
	powerCoreRadiusScale = 0.36
)

// Options contains the settings used to generate a PCB-inspired illustration.
// All dimensions are expressed in inches. ComponentCount excludes the mandatory
// connector row; the component count returned by Generate includes that row.
type Options struct {
	// Width and Height are the physical dimensions of the SVG canvas.
	Width  float64
	Height float64

	// BaseColor is used for ordinary components, signal traces, and the background grid.
	// AccentColor is reserved exclusively for power feeds and their connected terminals.
	BaseColor   string
	AccentColor string

	// Background is the canvas fill; "none" or "transparent" leaves it transparent.
	Background string

	// ComponentCount is the number of components placed in clusters, excluding connectors.
	ComponentCount int
	// ClusterCount is the number of independent component clusters.
	ClusterCount int
	// MinComponentsPerCluster is the minimum number of components assigned to each cluster.
	// A value of 1 permits single-component clusters.
	MinComponentsPerCluster int
	// Connections is the target number of additional signal connections beyond the
	// spanning-tree connections required to connect every multi-component cluster.
	Connections int
	// MaxParallel is the maximum number of parallel tracks in a trace bundle (1-3).
	MaxParallel int

	// Pitch is the center-to-center spacing of neighboring axial grid positions.
	Pitch float64
	// ComponentRadius is the circumradius of a full-size component hexagon.
	ComponentRadius float64
	// ComponentStrokeWidth is the outline width of component hexagons, in inches.
	ComponentStrokeWidth float64
	// ComponentMiddleHexagonScale and ComponentInnerHexagonScale set the sizes
	// of the inner outlines relative to each component's outer radius.
	ComponentMiddleHexagonScale float64
	ComponentInnerHexagonScale  float64
	// TraceStrokeWidth is the width of each trace, in inches.
	TraceStrokeWidth float64
	// TraceSpacing is the perpendicular centerline spacing of parallel traces.
	TraceSpacing float64
	// NodeRadius is the radius of each hollow terminal pad, in inches.
	NodeRadius float64

	// ShowBackgroundGrid enables the low-contrast hexagonal field. It is off by default.
	ShowBackgroundGrid bool
	// OmitPartialGridHexagons omits background hexagons that extend beyond the canvas.
	OmitPartialGridHexagons bool
	// GridRadius is the circumradius of background hexagons, in inches.
	GridRadius float64
	// GridStrokeWidth is the outline width of background hexagons, in inches.
	GridStrokeWidth float64

	// ConnectorCount is the number of contacts in the mandatory bottom row.
	ConnectorCount int
	// ConnectorInset is the requested gap between connector centers and the bottom edge.
	// Lattice snapping and component clearance may leave a larger gap.
	ConnectorInset float64

	// Seed makes generation reproducible when SeedSet is true.
	Seed int64
	// SeedSet distinguishes an explicitly selected seed (including zero) from the
	// default behavior, which selects a fresh seed for each generation.
	SeedSet bool
}

// DefaultOptions returns the default settings for SVG generation.
func DefaultOptions() Options {
	return Options{
		Width:                       defaultWidth,
		Height:                      defaultHeight,
		BaseColor:                   "#396CF8",
		AccentColor:                 "#FF6A00",
		Background:                  "none",
		ComponentCount:              38,
		ClusterCount:                6,
		MinComponentsPerCluster:     1,
		Connections:                 18,
		MaxParallel:                 3,
		Pitch:                       0.50,
		ComponentRadius:             0.26,
		ComponentStrokeWidth:        0.012,
		ComponentMiddleHexagonScale: 0.60,
		ComponentInnerHexagonScale:  0.34,
		TraceStrokeWidth:            0.009,
		TraceSpacing:                0.032,
		NodeRadius:                  0.014,
		ShowBackgroundGrid:          false,
		OmitPartialGridHexagons:     false,
		GridRadius:                  0.26,
		GridStrokeWidth:             0.0035,
		ConnectorCount:              13,
		ConnectorInset:              0.25,
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
		{"pitch", o.Pitch},
		{"component radius", o.ComponentRadius},
		{"component stroke width", o.ComponentStrokeWidth},
		{"trace stroke width", o.TraceStrokeWidth},
		{"node radius", o.NodeRadius},
		{"grid radius", o.GridRadius},
		{"connector inset", o.ConnectorInset},
	}
	for _, item := range positive {
		if !isFinite(item.value) || item.value <= 0 {
			return o, fmt.Errorf("%s must be finite and greater than zero", item.name)
		}
	}
	if !isFinite(o.GridStrokeWidth) || o.GridStrokeWidth < 0 {
		return o, fmt.Errorf("grid stroke width must be finite and greater than or equal to zero")
	}
	if !isFinite(o.TraceSpacing) || o.TraceSpacing <= 0 {
		return o, fmt.Errorf("trace spacing must be finite and greater than zero")
	}
	if !validScale(o.ComponentMiddleHexagonScale) || !validScale(o.ComponentInnerHexagonScale) ||
		o.ComponentInnerHexagonScale >= o.ComponentMiddleHexagonScale {
		return o, fmt.Errorf("component inner scales must satisfy 0 < inner < middle < 1")
	}
	if o.ComponentCount < 1 {
		return o, fmt.Errorf("component count must be at least 1")
	}
	if o.ClusterCount < 1 {
		return o, fmt.Errorf("cluster count must be at least 1")
	}
	if o.MinComponentsPerCluster < 1 {
		return o, fmt.Errorf("minimum components per cluster must be at least 1")
	}
	if o.ClusterCount > o.ComponentCount/o.MinComponentsPerCluster {
		return o, fmt.Errorf("component count (%d) cannot satisfy %d clusters with a minimum of %d components each", o.ComponentCount, o.ClusterCount, o.MinComponentsPerCluster)
	}
	if o.Connections < 0 {
		return o, fmt.Errorf("additional connection count cannot be negative")
	}
	if o.MaxParallel < 1 || o.MaxParallel > 3 {
		return o, fmt.Errorf("maximum parallel track count must be between 1 and 3")
	}
	if o.ConnectorCount < 3 {
		return o, fmt.Errorf("connector count must be at least 3")
	}
	if o.ConnectorCount < o.ClusterCount {
		return o, fmt.Errorf("connector count must be at least the cluster count")
	}
	if o.ConnectorInset >= o.Height/2 {
		return o, fmt.Errorf("connector inset must be less than half the canvas height")
	}
	if o.Pitch <= o.ComponentRadius*1.4 {
		return o, fmt.Errorf("pitch must be greater than 1.4 times the component radius")
	}
	maximumBundleReach := float64(o.MaxParallel-1)*o.TraceSpacing/2 + o.NodeRadius + o.TraceStrokeWidth/2
	powerCoreApothem := o.ComponentRadius * powerCoreRadiusScale * math.Sqrt(3) / 2
	if maximumBundleReach >= powerCoreApothem {
		return o, fmt.Errorf("trace bundle does not fit inside power-entry cores; reduce trace spacing, node radius, or maximum parallel tracks")
	}
	if _, err := connectorPositions(o); err != nil {
		return o, err
	}
	return o, nil
}

func validScale(value float64) bool {
	return isFinite(value) && value > 0 && value < 1
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
