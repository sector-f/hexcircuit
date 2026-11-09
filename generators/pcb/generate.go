package pcb

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/sector-f/hexgrid"
)

type clusterPlan struct {
	ID          int
	Hub         hexgrid.GridPosition
	Components  []placedComponent
	PowerSource hexgrid.GridPosition
	PowerSide   powerSide
	PowerPath   []hexgrid.GridPosition
}

type placedComponent struct {
	Position    hexgrid.GridPosition
	Style       hexgrid.ComponentStyle
	RadiusScale float64
	Parent      *hexgrid.GridPosition
	ClusterID   int
}

type boardPlan struct {
	Components   []hexgrid.GridComponent
	SignalTraces []hexgrid.GridTrace
	PowerCores   []hexgrid.GridComponent
	PowerTraces  []hexgrid.GridTrace
	PowerRoutes  [][]hexgrid.GridPosition // Full cell paths, retained for no-crossing tests.
	Clusters     []clusterPlan
}

// Generate constructs and renders a PCB-inspired illustration.
// It returns the SVG, the number of rendered base-layer components (cluster
// components plus every configured top- and bottom-row source contact), the
// number of rendered trace bundles (signal plus power), and any generation error.
func Generate(options Options) (string, int, int, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return "", 0, 0, err
	}

	seed := o.Seed
	if !o.SeedSet {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	topSources, bottomSources, err := powerSourcePositions(o)
	if err != nil {
		return "", 0, 0, err
	}
	plan, err := buildBoard(o, rng, topSources, bottomSources)
	if err != nil {
		return "", 0, 0, err
	}
	svg, err := renderBoard(o, plan)
	if err != nil {
		return "", 0, 0, fmt.Errorf("render PCB: %w", err)
	}
	return svg, len(plan.Components), len(plan.SignalTraces) + len(plan.PowerTraces), nil
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

	// Every configured contact is a physical component, regardless of whether
	// it is selected to power a cluster. Active contacts are distinguished by
	// their accent-colored core and attached power route in the overlay layer.
	sourceStyle := powerSourceComponentStyle(o.PowerSourceStyle)
	for _, source := range topSources {
		allPositions[source] = true
		plan.Components = append(plan.Components, hexgrid.GridComponent{
			Position: source,
			Style:    sourceStyle,
		})
	}
	for _, source := range bottomSources {
		allPositions[source] = true
		plan.Components = append(plan.Components, hexgrid.GridComponent{
			Position: source,
			Style:    sourceStyle,
		})
	}

	reserved := make(map[hexgrid.GridPosition]bool)
	for _, cluster := range clusters {
		path := cluster.PowerPath
		if len(path) < 2 {
			return boardPlan{}, fmt.Errorf("cluster %d has no complete power route", cluster.ID+1)
		}
		plan.PowerRoutes = append(plan.PowerRoutes, append([]hexgrid.GridPosition(nil), path...))
		plan.PowerTraces = append(plan.PowerTraces, hexgrid.GridTrace{
			Start:        cluster.PowerSource,
			Path:         compressPath(path),
			Lines:        randomLines(rng, o.MaxParallel),
			HideTerminal: true,
		})
		for _, position := range path {
			reserved[position] = true
		}
		plan.PowerCores = append(plan.PowerCores,
			hexgrid.GridComponent{Position: cluster.PowerSource, Style: hexgrid.ComponentStyleSingle, RadiusScale: powerCoreRadiusScale},
			hexgrid.GridComponent{Position: cluster.Hub, Style: hexgrid.ComponentStyleSingle, RadiusScale: powerCoreRadiusScale},
		)
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

	if o.Connections > 0 {
		extra, err := addExtraSignalConnections(o, rng, clusters, routingCells(o), blocked, reserved)
		if err != nil {
			return boardPlan{}, err
		}
		plan.SignalTraces = append(plan.SignalTraces, extra...)
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
	base.Background = o.Background
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

	baseSVG, err := hexgrid.RenderGrid(hexgrid.GridScene{
		Options:    base,
		Components: plan.Components,
		Traces:     plan.SignalTraces,
	})
	if err != nil {
		return "", fmt.Errorf("render base layer: %w", err)
	}
	if len(plan.PowerTraces) == 0 {
		return baseSVG, nil
	}

	accent := base
	accent.Color = o.AccentColor
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
	combined, err := appendSVGLayer(baseSVG, accentSVG)
	if err != nil {
		return "", err
	}
	return combined, nil
}

func appendSVGLayer(baseSVG, overlaySVG string) (string, error) {
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
		overlayBody = strings.ReplaceAll(overlayBody, `id="`+id+`"`, `id="power-`+id+`"`)
	}
	return baseSVG[:baseInsert] + overlayBody + baseSVG[baseInsert:], nil
}
