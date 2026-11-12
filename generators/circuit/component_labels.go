package circuit

import (
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/sector-f/hexgrid"
)

var defaultLabels = []string{
	"VCA", "VCO", "BGA", "PGA", "LGA", "CPU", "CAPACITOR",
	"EG", "ADSR", "RECTIFIER", "LPF", "BPF", "HPF", "CLOCK",
}

const (
	componentLabelPadding          = 0.005
	componentLabelObjectGap        = 0.006
)

// componentLabelFontColor returns the explicitly configured label color when
// supplied; otherwise it preserves the generator's existing foreground color.
func componentLabelFontColor(o Options) string {
	if o.FontColorSpecified {
		return o.FontColor
	}
	return o.Color
}

// Label diagnostics are opt-in so normal generation remains quiet. Set
// HEXAGONS_DEBUG_LABELS=1 when investigating placements.
func labelDebugEnabled() bool {
	value := strings.TrimSpace(os.Getenv("HEXAGONS_DEBUG_LABELS"))
	return value != "" && value != "0" && !strings.EqualFold(value, "false")
}

func labelDebugf(format string, args ...any) {
	if labelDebugEnabled() {
		fmt.Fprintf(os.Stderr, "[component-labels] "+format+"\n", args...)
	}
}

type labelComponentGeometry struct {
	Index  int
	X, Y   float64
	Radius float64
}

func componentGeometryForLabel(scene hexgrid.Scene, index int) labelComponentGeometry {
	component := scene.Components[index]
	radiusScale := component.RadiusScale
	if radiusScale == 0 {
		radiusScale = 1
	}
	center := coordToXY(Coord{Q: component.Position.Q, R: component.Position.R},
		scene.Options.Width, scene.Options.Height, scene.Options.Grid.Pitch, scene.Options.Orientation)
	return labelComponentGeometry{
		Index: index,
		X: center.X,
		Y: center.Y,
		Radius: scene.Options.ComponentRadius * radiusScale,
	}
}

// componentLabelRequests chooses text candidates and side orders without doing any
// physical placement. hexgrid.Render delegates measurement and collision resolution
// to the geometry package after converting the grid scene.
func componentLabelRequests(o Options, scene hexgrid.Scene, seed int64, wordList []string) []hexgrid.GridLabelRequest {
	if !o.ShowLabels || o.LabelProbability <= 0 || len(scene.Components) == 0 || len(wordList) == 0 {
		return nil
	}
	labelRNG := rand.New(rand.NewSource(seed ^ int64(0x4c4142454c)))
	requests := make([]hexgrid.GridLabelRequest, 0, len(scene.Components))

	for componentIndex, component := range scene.Components {
		componentGeometry := componentGeometryForLabel(scene, componentIndex)
		request := hexgrid.GridLabelRequest{
			Component: component.Position,
			Numbered: true,
			InsideFallback: o.LabelInsideComponents,
			Gap: o.LabelDistance,
			Padding: componentLabelPadding,
			ObjectGap: componentLabelObjectGap,
		}
		// Defer random selection until Render processes this component. Side-order
		// shuffles are also lazy, preserving the historical RNG consumption order:
		// later components are not randomized until earlier labels finish placement.
		request.CandidateProvider = func() []hexgrid.GridLabelCandidate {
			if labelRNG.Float64() >= o.LabelProbability {
				labelDebugf("skip component index=%d center=(%.6f, %.6f) in: probability check did not select it",
					componentGeometry.Index, componentGeometry.X, componentGeometry.Y)
				return nil
			}
			wordOrder := labelRNG.Perm(len(wordList))
			candidates := make([]hexgrid.GridLabelCandidate, 0, len(wordOrder))
			for _, wordIndex := range wordOrder {
				word := wordList[wordIndex]
				candidates = append(candidates, hexgrid.GridLabelCandidate{
					Text: word,
					SideOrderProvider: func() []int {
						sideOrder := []int{0, 1, 2, 3, 4, 5}
						labelRNG.Shuffle(len(sideOrder), func(i, j int) {
							sideOrder[i], sideOrder[j] = sideOrder[j], sideOrder[i]
						})
						return sideOrder
					},
				})
			}
			labelDebugf("requested component index=%d center=(%.6f, %.6f) in radius=%.6f in candidates=%d inside_fallback=%t",
				componentGeometry.Index, componentGeometry.X, componentGeometry.Y, componentGeometry.Radius, len(candidates), o.LabelInsideComponents)
			return candidates
		}
		requests = append(requests, request)
	}
	labelDebugf("prepared %d label request(s)", len(requests))
	return requests
}
