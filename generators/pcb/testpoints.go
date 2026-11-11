package pcb

import (
	"math/rand"

	"github.com/sector-f/hexgrid"
)

type testPointCandidate struct {
	clusterID int
	start     hexgrid.GridPosition
	path      []hexgrid.GridPosition
}

// addDecorativeTestPoints places short straight stubs from leaf components to
// empty grid cells. Candidates are distributed one per cluster per pass, then
// additional stubs are added only after every cluster has had an opportunity.
// The target is best-effort: unavailable corridors are skipped.
func addDecorativeTestPoints(
	o Options,
	rng *rand.Rand,
	clusters []clusterPlan,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if o.TestPointCount <= 0 || len(clusters) == 0 {
		return nil
	}

	byCluster := make([][]testPointCandidate, len(clusters))
	for clusterIndex, cluster := range clusters {
		children := make(map[hexgrid.GridPosition]int, len(cluster.Components))
		for _, component := range cluster.Components {
			if component.Parent != nil {
				children[*component.Parent]++
			}
		}

		for _, component := range cluster.Components {
			// Components with no children are the leaves of the cluster's
			// placement tree. This excludes internal nodes and power contacts.
			if children[component.Position] != 0 {
				continue
			}

			directions := append([]hexgrid.GridPosition(nil), axialDirections[:]...)
			if component.Parent != nil {
				// Prefer continuing outward from the cluster along the edge
				// that led to this leaf, but retain the other five choices.
				outward := hexgrid.GridPosition{
					Q: component.Position.Q - component.Parent.Q,
					R: component.Position.R - component.Parent.R,
				}
				for i, direction := range directions {
					if direction == outward {
						directions[0], directions[i] = directions[i], directions[0]
						break
					}
				}
				rng.Shuffle(len(directions)-1, func(i, j int) {
					i++
					j++
					directions[i], directions[j] = directions[j], directions[i]
				})
			} else {
				rng.Shuffle(len(directions), func(i, j int) {
					directions[i], directions[j] = directions[j], directions[i]
				})
			}

			for _, direction := range directions {
				for length := o.TestPointMinLength; length <= o.TestPointMaxLength; length++ {
					path := make([]hexgrid.GridPosition, 0, length)
					position := component.Position
					valid := true
					for step := 0; step < length; step++ {
						position = addPosition(position, direction)
						if !allowed[position] || blocked[position] || reserved[position] {
							valid = false
							break
						}
						path = append(path, position)
					}
					if valid {
						byCluster[clusterIndex] = append(byCluster[clusterIndex], testPointCandidate{
							clusterID: clusterIndex,
							start: component.Position,
							path: path,
						})
					}
				}
			}
		}
		rng.Shuffle(len(byCluster[clusterIndex]), func(i, j int) {
			byCluster[clusterIndex][i], byCluster[clusterIndex][j] = byCluster[clusterIndex][j], byCluster[clusterIndex][i]
		})
	}

	traces := make([]hexgrid.GridTrace, 0, o.TestPointCount)
	usedStarts := make(map[hexgrid.GridPosition]bool)
	clusterOrder := make([]int, len(clusters))
	for i := range clusterOrder {
		clusterOrder[i] = i
	}
	rng.Shuffle(len(clusterOrder), func(i, j int) {
		clusterOrder[i], clusterOrder[j] = clusterOrder[j], clusterOrder[i]
	})

	// Round-robin across clusters so a dense cluster cannot consume the whole
	// target before smaller clusters get a chance to contribute.
	for len(traces) < o.TestPointCount {
		progress := false
		for _, clusterIndex := range clusterOrder {
			for len(byCluster[clusterIndex]) > 0 {
				candidate := byCluster[clusterIndex][0]
				byCluster[clusterIndex] = byCluster[clusterIndex][1:]
				if usedStarts[candidate.start] {
					continue
				}
				valid := true
				for _, position := range candidate.path {
					if !allowed[position] || blocked[position] || reserved[position] {
						valid = false
						break
					}
				}
				if !valid {
					continue
				}

				traces = append(traces, hexgrid.GridTrace{
					Start: candidate.start,
					Path: []hexgrid.GridPosition{candidate.path[len(candidate.path)-1]},
					Lines: 1,
					HideTerminal: false,
				})
				usedStarts[candidate.start] = true
				for _, position := range candidate.path {
					reserved[position] = true
				}
				progress = true
				break
			}
			if len(traces) >= o.TestPointCount {
				break
			}
		}
		if !progress {
			break
		}
	}
	return traces
}
