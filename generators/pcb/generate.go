package pcb

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/sector-f/hexgrid"
)

type clusterPlan struct {
	ID           int
	Hub          hexgrid.GridPosition
	Components   []placedComponent
	PowerSource  hexgrid.GridPosition // First-feed alias retained for test/helper compatibility.
	PowerSide    powerSide
	PowerPath    []hexgrid.GridPosition // First-feed alias retained for test/helper compatibility.
	PowerSources []powerSource
	PowerTargets []hexgrid.GridPosition
	PowerPaths   [][]hexgrid.GridPosition
}

type placedComponent struct {
	Position    hexgrid.GridPosition
	Style       hexgrid.ComponentStyle
	RadiusScale float64
	Parent      *hexgrid.GridPosition
	ClusterID   int
}

type boardPlan struct {
	Components             []hexgrid.GridComponent
	SignalTraces           []hexgrid.GridTrace
	PowerCores             []hexgrid.GridComponent
	PowerTraces            []hexgrid.GridTrace
	// InterclusterComponents contains the endpoint cores rendered with inter-cluster traces.
	InterclusterComponents []hexgrid.GridComponent
	// InterClusterTraces remains separate from the power layer so its style can evolve independently.
	InterClusterTraces     []hexgrid.GridTrace
	// DecorativeTraces are bent, base-color traces that end at hollow terminal pads.
	DecorativeTraces        []hexgrid.GridTrace
	PowerRoutes            [][]hexgrid.GridPosition // Full cell paths, retained for no-crossing tests.
	Clusters               []clusterPlan
}

// Generate constructs and renders a PCB-inspired illustration.
// It returns the SVG, the number of rendered base-layer components (cluster
// components plus every configured top- and bottom-row source contact), the
// number of rendered trace bundles (signal, power, inter-cluster, and decorative), and any generation error.
func Generate(options Options) (string, int, int, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return "", 0, 0, err
	}

	seed := o.Seed
	if !o.SeedSet {
		seed = time.Now().UnixNano()
	}
	topSources, bottomSources, err := powerSourcePositions(o)
	if err != nil {
		return "", 0, 0, err
	}
	plan, err := buildBoardWithSeedAttempts(o.GenerationAttempts, seed, func(attemptSeed int64) (boardPlan, error) {
		rng := rand.New(rand.NewSource(attemptSeed))
		return buildBoard(o, rng, topSources, bottomSources)
	})
	if err != nil {
		return "", 0, 0, err
	}
	svg, err := renderBoard(o, plan)
	if err != nil {
		return "", 0, 0, fmt.Errorf("render PCB: %w", err)
	}
	return svg, len(plan.Components), len(plan.SignalTraces) + len(plan.PowerTraces) + len(plan.InterClusterTraces) + len(plan.DecorativeTraces), nil
}


// buildBoardWithSeedAttempts retries only failures caused by exhausted randomized
// placement and mandatory power routing. The first attempt always uses masterSeed;
// fallback seeds are derived deterministically so explicit-seed runs stay reproducible.
func buildBoardWithSeedAttempts(
	attempts int,
	masterSeed int64,
	build func(seed int64) (boardPlan, error),
) (boardPlan, error) {
	if attempts < 1 {
		return boardPlan{}, fmt.Errorf("generation attempts must be at least 1")
	}

	var lastErr error
	var lastSeed int64
	for attempt := 0; attempt < attempts; attempt++ {
		seed := generationAttemptSeed(masterSeed, attempt)
		plan, err := build(seed)
		if err == nil {
			return plan, nil
		}
		var retryable *retryableGenerationError
		if !errors.As(err, &retryable) {
			return boardPlan{}, err
		}
		lastErr = err
		lastSeed = seed
	}

	return boardPlan{}, fmt.Errorf(
		"generation failed after %d seed attempts (last seed %d): %w",
		attempts, lastSeed, lastErr,
	)
}

// generationAttemptSeed preserves the master seed for attempt zero and uses a
// SplitMix64 step to derive well-distributed, reproducible fallback seeds.
func generationAttemptSeed(masterSeed int64, attempt int) int64 {
	if attempt <= 0 {
		return masterSeed
	}
	value := uint64(masterSeed) + uint64(attempt)*0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	value ^= value >> 31
	return int64(value)
}

type retryableGenerationError struct {
	err error
}

func (e *retryableGenerationError) Error() string {
	return e.err.Error()
}

func (e *retryableGenerationError) Unwrap() error {
	return e.err
}

func buildBoard(
	o Options,
	rng *rand.Rand,
	topSources, bottomSources []hexgrid.GridPosition,
) (boardPlan, error) {
	placements, clusters, err := placeClusters(o, rng, topSources, bottomSources)
	if err != nil {
		return boardPlan{}, err
	}

	plan := boardPlan{Clusters: clusters}
	allPositions := make(map[hexgrid.GridPosition]bool, len(placements)+len(topSources)+len(bottomSources))
	for _, p := range placements {
		allPositions[p.Position] = true
		plan.Components = append(plan.Components, hexgrid.GridComponent{
			Position:    p.Position,
			Style:       p.Style,
			RadiusScale: p.RadiusScale,
		})
	}

	// Every configured physical contact is rendered. In separated mode, the
	// unused lattice cell between neighboring contacts is not a contact.
	sourceStyle := powerSourceComponentStyle(o.PowerSourceStyle)
	for _, source := range topSources {
		allPositions[source] = true
		plan.Components = append(plan.Components, hexgrid.GridComponent{Position: source, Style: sourceStyle})
	}
	for _, source := range bottomSources {
		allPositions[source] = true
		plan.Components = append(plan.Components, hexgrid.GridComponent{Position: source, Style: sourceStyle})
	}

	reserved := make(map[hexgrid.GridPosition]bool)
	for _, cluster := range clusters {
		endpointCores := make(map[hexgrid.GridPosition]bool)
		for i, source := range cluster.PowerSources {
			path := cluster.PowerPaths[i]
			if len(path) < 2 {
				return boardPlan{}, fmt.Errorf("cluster %d power feed %d has no complete route", cluster.ID+1, i+1)
			}
			target := cluster.PowerTargets[i]
			if path[0] != source.Position || path[len(path)-1] != target {
				return boardPlan{}, fmt.Errorf("cluster %d power feed %d route does not connect its source to its assigned component", cluster.ID+1, i+1)
			}
			plan.PowerRoutes = append(plan.PowerRoutes, append([]hexgrid.GridPosition(nil), path...))
			plan.PowerTraces = append(plan.PowerTraces, hexgrid.GridTrace{
				Start:        source.Position,
				Path:         compressPath(path),
				Lines:        randomLines(rng, o.MaxParallel),
				HideTerminal: true,
			})
			for _, position := range path {
				reserved[position] = true
			}
			plan.PowerCores = append(plan.PowerCores,
				hexgrid.GridComponent{Position: source.Position, Style: hexgrid.ComponentStyleSingle, RadiusScale: powerCoreRadiusScale},
			)
			if !endpointCores[target] {
				plan.PowerCores = append(plan.PowerCores,
					hexgrid.GridComponent{Position: target, Style: hexgrid.ComponentStyleSingle, RadiusScale: powerCoreRadiusScale},
				)
				endpointCores[target] = true
			}
		}
	}

	blocked := allPositions
	// Every cluster already has a connected placement tree. Convert its parent
	// relationships into mandatory local signal traces before adding optional links.
	for _, cluster := range clusters {
		for _, component := range cluster.Components {
			if component.Parent == nil {
				continue
			}
			plan.SignalTraces = append(plan.SignalTraces, hexgrid.GridTrace{
				Start:        *component.Parent,
				Path:         []hexgrid.GridPosition{component.Position},
				Lines:        randomLines(rng, o.MaxParallel),
				HideTerminal: true,
			})
		}
	}

	if o.Connections > 0 || o.InterClusterTraces || o.DecorativeTraceCount > 0 {
		signalRoutes := routingCellsWithinPowerSourceCorridor(
			routingCellsWithoutPowerSourceRows(o, topSources, bottomSources), topSources, bottomSources, o,
		)
		activeSources := make([]powerSource, 0)
		for _, cluster := range clusters {
			activeSources = append(activeSources, cluster.PowerSources...)
		}
		if o.ConstrainComponentsToPowerSourceSpan {
			spanSources := powerSourceSpanAssignments(activeSources, toPowerSources(topSources, powerSideTop), toPowerSources(bottomSources, powerSideBottom))
			signalRoutes = routingCellsWithinPowerSourceSpan(signalRoutes, spanSources, o)
		}
		if o.ConstrainLayoutToPowerSourceTrapezoid {
			bounds, err := newPowerSourceTrapezoid(topSources, bottomSources, o)
			if err != nil {
				return boardPlan{}, err
			}
			signalRoutes = routingCellsWithinPowerSourceTrapezoid(signalRoutes, bounds, o)
		}
		if o.Connections > 0 {
			extra, err := addExtraSignalConnections(o, rng, clusters, signalRoutes, blocked, reserved)
			if err != nil {
				return boardPlan{}, err
			}
			plan.SignalTraces = append(plan.SignalTraces, extra...)
		}
		if o.InterClusterTraces {
			plan.InterClusterTraces = addInterClusterConnections(o, rng, clusters, signalRoutes, blocked, reserved)
			seenEndpoints := make(map[hexgrid.GridPosition]bool)
			for _, trace := range plan.InterClusterTraces {
				if len(trace.Path) == 0 {
					continue
				}
				for _, position := range []hexgrid.GridPosition{
					trace.Start,
					trace.Path[len(trace.Path)-1],
				} {
					if seenEndpoints[position] {
						continue
					}
					plan.InterclusterComponents = append(plan.InterclusterComponents, hexgrid.GridComponent{
						Position:    position,
						Style:       hexgrid.ComponentStyleSingle,
						RadiusScale: powerCoreRadiusScale,
					})
					seenEndpoints[position] = true
				}
			}
		}
		if o.DecorativeTraceCount > 0 {
			plan.DecorativeTraces = addDecorativeTraces(o, rng, clusters, signalRoutes, blocked, reserved)
		}
	}

	return plan, nil
}

func sortClustersByX(clusters []clusterPlan, o Options) {
	// The list is small; insertion sort avoids another package dependency and keeps
	// this helper deterministic for equal horizontal coordinates.
	for i := 1; i < len(clusters); i++ {
		for j := i; j > 0; j-- {
			a := hexgrid.GridPositionToPoint(clusters[j-1].Hub, o.Width, o.Height, o.Pitch, "pointy")
			b := hexgrid.GridPositionToPoint(clusters[j].Hub, o.Width, o.Height, o.Pitch, "pointy")
			if a.X <= b.X {
				break
			}
			clusters[j-1], clusters[j] = clusters[j], clusters[j-1]
		}
	}
}

func randomLines(rng *rand.Rand, maximum int) int {
	if maximum <= 1 {
		return 1
	}
	// Favor single and double tracks while retaining occasional dense bundles.
	roll := rng.Intn(100)
	if maximum >= 3 && roll >= 88 {
		return 3
	}
	if maximum >= 2 && roll >= 48 {
		return 2
	}
	return 1
}

func renderBoard(o Options, plan boardPlan) (string, error) {
	base := hexgrid.DefaultOptions()
	base.Width = o.Width
	base.Height = o.Height
	base.Orientation = "pointy"
	base.Color = o.BaseColor
	base.GridColor = o.GridColor
	base.Background = o.Background
	base.GlowEnabled = o.BaseGlow
	base.GlowBlurRadius = o.BaseGlowBlurRadius
	base.GlowOpacity = o.BaseGlowOpacity
	base.Grid = hexgrid.Grid{
		Pitch:       o.Pitch,
		Radius:      o.GridRadius,
		StrokeWidth: 0,
		OmitPartial: o.OmitPartialGridHexagons,
	}
	if o.ShowBackgroundGrid {
		base.Grid.StrokeWidth = o.GridStrokeWidth
	}
	base.ComponentRadius = o.ComponentRadius
	base.ComponentStrokeWidth = o.ComponentStrokeWidth
	base.ComponentMiddleHexagonScale = o.ComponentMiddleHexagonScale
	base.ComponentInnerHexagonScale = o.ComponentInnerHexagonScale
	base.TraceStrokeWidth = o.TraceStrokeWidth
	base.TraceSpacing = o.TraceSpacing
	base.NodeRadius = o.NodeRadius

	baseTraces := append(append([]hexgrid.GridTrace(nil), plan.SignalTraces...), plan.DecorativeTraces...)
	baseSVG, err := hexgrid.RenderGrid(hexgrid.GridScene{
		Options:    base,
		Components: plan.Components,
		Traces:     baseTraces,
	})
	if err != nil {
		return "", fmt.Errorf("render base layer: %w", err)
	}
	combined := baseSVG
	if len(plan.PowerTraces) > 0 {
		accent := base
		accent.Color = o.AccentColor
		accent.GlowEnabled = o.AccentGlow
		accent.GlowBlurRadius = o.AccentGlowBlurRadius
		accent.GlowOpacity = o.AccentGlowOpacity
		accent.Background = "none"
		accent.Grid.StrokeWidth = 0
		accentSVG, err := hexgrid.RenderGrid(hexgrid.GridScene{
			Options:    accent,
			Components: plan.PowerCores,
			Traces:     plan.PowerTraces,
		})
		if err != nil {
			return "", fmt.Errorf("render power layer: %w", err)
		}
		combined, err = appendSVGLayerNamed(combined, accentSVG, "power")
		if err != nil {
			return "", err
		}
	}
	if len(plan.InterClusterTraces) > 0 {
		interCluster := base
		interCluster.Color = o.AccentColor
		interCluster.GlowEnabled = o.AccentGlow
		interCluster.GlowBlurRadius = o.AccentGlowBlurRadius
		interCluster.GlowOpacity = o.AccentGlowOpacity
		interCluster.Background = "none"
		interCluster.Grid.StrokeWidth = 0
		interClusterSVG, err := hexgrid.RenderGrid(hexgrid.GridScene{
			Options:    interCluster,
			Components: plan.InterclusterComponents,
			Traces:     plan.InterClusterTraces,
		})
		if err != nil {
			return "", fmt.Errorf("render inter-cluster trace layer: %w", err)
		}
		combined, err = appendSVGLayerNamed(combined, interClusterSVG, "inter-cluster")
		if err != nil {
			return "", err
		}
	}

	return combined, nil
}

func appendSVGLayer(baseSVG, overlaySVG string) (string, error) {
	return appendSVGLayerNamed(baseSVG, overlaySVG, "power")
}

func appendSVGLayerNamed(baseSVG, overlaySVG, prefix string) (string, error) {
	baseOpen := strings.Index(baseSVG, "<svg")
	baseClose := strings.LastIndex(baseSVG, "</svg>")
	overlayOpen := strings.Index(overlaySVG, "<svg")
	overlayClose := strings.LastIndex(overlaySVG, "</svg>")
	if baseOpen < 0 || baseClose < 0 || overlayOpen < 0 || overlayClose < 0 {
		return "", fmt.Errorf("could not combine rendered SVG layers: missing SVG root element")
	}
	baseTagEnd := strings.Index(baseSVG[baseOpen:], ">")
	overlayTagEnd := strings.Index(overlaySVG[overlayOpen:], ">")
	if baseTagEnd < 0 || overlayTagEnd < 0 {
		return "", fmt.Errorf("could not combine rendered SVG layers: malformed SVG root element")
	}
	baseInsert := baseClose
	overlayBodyStart := overlayOpen + overlayTagEnd + 1
	overlayBody := overlaySVG[overlayBodyStart:overlayClose]
	// RenderGrid gives each layer generic group IDs. Namespace the overlay IDs so
	// combining the two documents does not leave duplicate IDs in the final SVG.
	for _, id := range []string{
		"background-fill",
		"background-grid",
		"traces",
		"components",
		"component-labels",
		"terminal-pads",
	} {
		overlayBody = strings.ReplaceAll(overlayBody, `id="`+id+`"`, `id="`+prefix+`-`+id+`"`)
	}
	overlayBody = strings.ReplaceAll(overlayBody, `id="hexgrid-neon-glow"`, `id="`+prefix+`-hexgrid-neon-glow"`)
	overlayBody = strings.ReplaceAll(overlayBody, "url(#hexgrid-neon-glow)", "url(#"+prefix+"-hexgrid-neon-glow)")
	return baseSVG[:baseInsert] + overlayBody + baseSVG[baseInsert:], nil
}
