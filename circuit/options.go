package circuit

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// FallbackEdgeOrder controls how fallback component pairs are ordered after preferred edges.
type FallbackEdgeOrder string

const (
	// FallbackEdgeOrderRandomized tries fallback pairs in randomized order.
	// This preserves the generator's existing effective ordering behavior.
	FallbackEdgeOrderRandomized FallbackEdgeOrder = "randomized"
	// FallbackEdgeOrderRanked prioritizes same-cluster fallback pairs, then shorter axial distances.
	FallbackEdgeOrderRanked FallbackEdgeOrder = "ranked"
)

// Options contains the configuration needed to generate the SVG. All dimensions are in inches.
type Options struct {
	// Width is the physical width of the SVG canvas, in inches.
	Width float64
	// Height is the physical height of the SVG canvas, in inches.
	Height float64
	// Color is the stroke color used for the grid, components, traces, and terminal pads.
	Color string
	// Background is the canvas fill color; "none" or "transparent" leaves it transparent.
	Background string
	// Orientation sets the hexagon layout: "pointy" has a vertex at the top,
	// while "flat" has a vertex at the right.
	Orientation string

	// ComponentCount is the target number of circuit-component hexagons to place.
	ComponentCount int
	// ComponentRadius is the maximum circumradius of a component hexagon;
	// individual components may be smaller.
	ComponentRadius float64
	// RoutingPitch is the center-to-center spacing of neighboring positions
	// in the hexagonal lattice.
	RoutingPitch float64
	// AutoRoutingPitch allows the generator to increase RoutingPitch when it
	// is too small to fit the grid and its gap.
	AutoRoutingPitch bool
	// GridRadius is the circumradius of each background hexagon, in inches.
	GridRadius float64
	// GridGap is the intended visible gap between neighboring background hexagon outlines, in inches.
	GridGap float64
	// GridWidth is the stroke thickness of the background hexagon outlines, in inches.
	GridWidth float64
	// OmitPartialGridHexagons skips background hexagons whose stroked outlines
	// would extend beyond the canvas.
	OmitPartialGridHexagons bool

	// Connections is the target number of component-to-component trace bundles.
	// Fewer may be accepted if routing constraints prevent additional connections.
	Connections int
	// MaxParallel is the maximum number of parallel traces that a single bundle may contain.
	MaxParallel int
	// TrackCountWeights gives the percentage weight for each bundle size from
	// 1 through MaxParallel; the values must sum to 100.
	TrackCountWeights []float64
	// ParallelSpacing is the perpendicular centerline distance between neighboring tracks in a bundle, in inches.
	ParallelSpacing float64
	// StrokeWidth is the line thickness used to outline component hexagons and their inner details, in inches.
	StrokeWidth float64
	// TraceWidth is the line thickness of circuit traces and terminal-circle outlines, in inches.
	TraceWidth float64
	// NodeRadius is the radius of each hollow circular trace terminal, in inches.
	NodeRadius float64
	// TraceClearance is the minimum extra separation requested between traces
	// and other routes or unrelated components, in inches.
	TraceClearance float64
	// DetailProb is the base probability (0 to 1) that a sufficiently large
	// component receives inner hexagon details.
	DetailProb float64
	// ShowLabels enables automatically placed component labels.
	ShowLabels bool
	// LabelFont is a system font family name resolved by the SVG viewer; it is not embedded.
	LabelFont string
	// LabelFontSize is the label font size in points.
	LabelFontSize float64
	// LabelProbability is the probability that each component receives a label.
	// FontColor is the explicit component-label text color.
	FontColor string
	// FontColorSpecified indicates whether FontColor was explicitly provided.
	// When false, component labels use the existing Options.Color.
	FontColorSpecified bool
	LabelProbability   float64
	// LabelDistance is the minimum gap outside a component, or minimum inset when a label is inside.
	LabelDistance float64
	// LabelInsideComponents permits a label inside its component when no external position fits.
	LabelInsideComponents bool
	// UseDeterministicRouteCost selects the route-cost model. When true, route costs
	// depend only on move count and turns. When false, the legacy model adds random
	// jitter to each move's accumulated cost.
	UseDeterministicRouteCost bool
	// FallbackEdgeOrder controls how fallback component pairs are tried. Preferred
	// placement edges remain first in either mode. The zero value defaults to randomized.
	FallbackEdgeOrder FallbackEdgeOrder
	// MaxRouteSteps limits how many lattice steps the search may take before
	// abandoning a candidate route.
	MaxRouteSteps int
	// RouteAttempts is the number of alternative route attempts made for each
	// direction of a candidate component pair.
	RouteAttempts int

	// Seed is the integer seed used to make layout and routing choices
	// reproducible when SeedSet is true.
	Seed int64
	// SeedSet indicates whether Seed was explicitly supplied; false makes the generator choose a seed for this run.
	SeedSet bool
}

// DefaultOptions returns the default settings for SVG generation.
func DefaultOptions() Options {
	return Options{
		Width:                   10.0,
		Height:                  8.0,
		Color:                   "#C7834A",
		Background:              "none",
		Orientation:             "pointy",
		ComponentCount:          18,
		ComponentRadius:         0.40,
		RoutingPitch:            0.62,
		AutoRoutingPitch:        true,
		GridRadius:              0.33255,
		GridGap:                 0.04,
		GridWidth:               0.004,
		OmitPartialGridHexagons: false,
		Connections:             18,
		MaxParallel:             3,
		// These percentages preserve the former 1:4:4 weighting for 1, 2, and 3 tracks.
		TrackCountWeights: []float64{11.1111, 44.44445, 44.44445},
		ParallelSpacing:   0.035,
		StrokeWidth:       0.012,
		TraceWidth:        0.008,
		NodeRadius:        0.014,
		TraceClearance:    0.025,
		DetailProb:        0.48,

		ShowLabels:            false,
		LabelFont:             "monospace",
		FontColor:             "",
		FontColorSpecified:    false,
		LabelFontSize:         80.0,
		LabelProbability:      0.35,
		LabelDistance:         0.025,
		LabelInsideComponents: false,

		UseDeterministicRouteCost: false,                       // Preserve the legacy route-cost model by default.
		FallbackEdgeOrder:         FallbackEdgeOrderRandomized, // Preserve the current effective fallback ordering.
		MaxRouteSteps:             24,
		RouteAttempts:             12,
	}
}

// normalizeOptions validates all generation options and, when enabled, raises the
// routing pitch enough to fit the background grid.
func normalizeOptions(o Options) (Options, error) {
	// An empty value is the zero value for callers constructing Options directly;
	// default it to the pre-existing randomized fallback order for compatibility.
	if o.FallbackEdgeOrder == "" {
		o.FallbackEdgeOrder = FallbackEdgeOrderRandomized
	}
	if o.FallbackEdgeOrder != FallbackEdgeOrderRandomized && o.FallbackEdgeOrder != FallbackEdgeOrderRanked {
		return o, fmt.Errorf("fallback edge order must be %q or %q", FallbackEdgeOrderRandomized, FallbackEdgeOrderRanked)
	}

	// These measurements must be finite and positive; zero-width strokes/radii would
	// produce degenerate geometry and negative dimensions are never meaningful here.
	positive := []struct {
		name  string
		value float64
	}{
		{"width", o.Width},
		{"height", o.Height},
		{"component radius", o.ComponentRadius},
		{"routing pitch", o.RoutingPitch},
		{"grid radius", o.GridRadius},
		{"grid width", o.GridWidth},
		{"parallel spacing", o.ParallelSpacing},
		{"component stroke width", o.StrokeWidth},
		{"trace width", o.TraceWidth},
		{"node radius", o.NodeRadius},
	}
	for _, item := range positive {
		if !isFinite(item.value) || item.value <= 0 {
			return o, fmt.Errorf("%s must be finite and greater than zero", item.name)
		}
	}
	if !isFinite(o.GridGap) || o.GridGap < 0 {
		return o, errors.New("grid gap must be finite and greater than or equal to zero")
	}
	if !isFinite(o.TraceClearance) || o.TraceClearance < 0 {
		return o, errors.New("trace clearance must be finite and greater than or equal to zero")
	}
	if !isFinite(o.DetailProb) || o.DetailProb < 0 || o.DetailProb > 1 {
		return o, errors.New("detail probability must be between 0 and 1")
	}

	if !isFinite(o.LabelFontSize) || o.LabelFontSize <= 0 {
		return o, errors.New("label font size must be finite and greater than zero")
	}
	if !isFinite(o.LabelProbability) || o.LabelProbability < 0 || o.LabelProbability > 1 {
		return o, errors.New("label probability must be between 0 and 1")
	}
	if o.FontColorSpecified && strings.TrimSpace(o.FontColor) == "" {
		return o, errors.New("font color cannot be empty when explicitly specified")
	}
	o.FontColor = strings.TrimSpace(o.FontColor)
	if !isFinite(o.LabelDistance) || o.LabelDistance < 0 {
		return o, errors.New("label distance must be finite and greater than or equal to zero")
	}
	if o.ShowLabels && strings.TrimSpace(o.LabelFont) == "" {
		return o, errors.New("label font cannot be empty when labels are enabled")
	}
	o.LabelFont = strings.TrimSpace(o.LabelFont)

	if o.Connections < 1 {
		return o, errors.New("connection count must be at least 1")
	}
	if o.MaxParallel < 1 {
		return o, errors.New("maximum parallel track count must be at least 1")
	}
	if len(o.TrackCountWeights) != o.MaxParallel {
		return o, fmt.Errorf("track-count weights must contain exactly %d percentages, one for each possible track count", o.MaxParallel)
	}
	weightTotal := 0.0
	for i, weight := range o.TrackCountWeights {
		if !isFinite(weight) || weight < 0 || weight > 100 {
			return o, fmt.Errorf("track-count weight %d must be a finite percentage between 0 and 100", i+1)
		}
		weightTotal += weight
	}
	if math.Abs(weightTotal-100.0) > 1e-5 {
		return o, fmt.Errorf("track-count weight percentages must add up to 100 (current sum: %.8g)", weightTotal)
	}
	if o.MaxRouteSteps < minLatticeMoves {
		return o, fmt.Errorf("maximum route steps must be at least %d to allow a valid route with a bend", minLatticeMoves)
	}
	if o.RouteAttempts < 1 {
		return o, errors.New("route attempts must be at least 1")
	}
	if strings.TrimSpace(o.Color) == "" {
		return o, errors.New("stroke color cannot be empty")
	}
	if o.Orientation != "pointy" && o.Orientation != "flat" {
		return o, errors.New("orientation must be either pointy or flat")
	}
	// Neighboring lattice centers must be far enough apart to fit the grid hexagons,
	// their visible gap, and their stroke. Auto-adjust only when AutoRoutingPitch is true.
	minimumPitch := sqrt3*o.GridRadius + o.GridGap + o.GridWidth
	if o.RoutingPitch+epsilon < minimumPitch {
		if !o.AutoRoutingPitch {
			return o, fmt.Errorf("routing pitch %.6g in is too small for grid radius %.6g in with the requested grid gap/width; it must be at least %.4f inches", o.RoutingPitch, o.GridRadius, minimumPitch)
		}
		o.RoutingPitch = minimumPitch
	}
	if o.ComponentRadius <= o.NodeRadius+o.TraceWidth {
		return o, errors.New("component radius must be larger than node radius plus trace width")
	}
	if o.RoutingPitch <= 1.4*o.ComponentRadius {
		return o, errors.New("routing pitch must exceed 1.4 times the component radius to leave room for routed traces")
	}
	if o.GridGap+o.GridWidth >= o.RoutingPitch {
		return o, errors.New("grid gap plus grid width must be smaller than routing pitch")
	}
	return o, nil
}
