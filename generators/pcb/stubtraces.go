package pcb

import (
	"math/rand"
	"sort"

	"github.com/sector-f/hexgrid"
)

type decorativeTraceCandidate struct {
	start hexgrid.GridPosition
	path  []hexgrid.GridPosition // Full cell path, including the component at index zero.
}

// addDecorativeTraces creates free-ended, base-color trace bundles from leaf
// components. Each route has at least one bend and may use multiple parallel
// lines. Placement is best-effort and round-robins across clusters.
func addDecorativeTraces(
	o Options,
	rng *rand.Rand,
	clusters []clusterPlan,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if o.DecorativeTraceCount <= 0 || len(clusters) == 0 {
		return nil
	}

	byCluster := make([][]decorativeTraceCandidate, len(clusters))
	for clusterIndex, cluster := range clusters {
		children := make(map[hexgrid.GridPosition]int, len(cluster.Components))
		for _, component := range cluster.Components {
			if component.Parent != nil {
				children[*component.Parent]++
			}
		}

		for _, component := range cluster.Components {
			if children[component.Position] != 0 {
				continue
			}

			directions := append([]hexgrid.GridPosition(nil), axialDirections[:]...)
			if component.Parent != nil {
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
					directions[i+1], directions[j+1] = directions[j+1], directions[i+1]
				})
			} else {
				rng.Shuffle(len(directions), func(i, j int) {
					directions[i], directions[j] = directions[j], directions[i]
				})
			}

			// Keep a small set of alternative exits per leaf. The first
			// valid route in each direction is enough for candidate diversity.
			for _, direction := range directions {
				for attempt := 0; attempt < 4; attempt++ {
					path := randomDecorativeTracePath(
						component.Position, direction, o, allowed, blocked, reserved, rng,
					)
					if len(path) == 0 {
						continue
					}
					byCluster[clusterIndex] = append(byCluster[clusterIndex], decorativeTraceCandidate{
						start: component.Position,
						path:  path,
					})
					break
				}
			}
		}
		rng.Shuffle(len(byCluster[clusterIndex]), func(i, j int) {
			byCluster[clusterIndex][i], byCluster[clusterIndex][j] = byCluster[clusterIndex][j], byCluster[clusterIndex][i]
		})
	}

	traces := make([]hexgrid.GridTrace, 0, o.DecorativeTraceCount)
	usedStarts := make(map[hexgrid.GridPosition]bool)
	clusterOrder := make([]int, len(clusters))
	for i := range clusterOrder {
		clusterOrder[i] = i
	}
	rng.Shuffle(len(clusterOrder), func(i, j int) {
		clusterOrder[i], clusterOrder[j] = clusterOrder[j], clusterOrder[i]
	})

	for len(traces) < o.DecorativeTraceCount {
		progress := false
		for _, clusterIndex := range clusterOrder {
			for len(byCluster[clusterIndex]) > 0 {
				candidate := byCluster[clusterIndex][0]
				byCluster[clusterIndex] = byCluster[clusterIndex][1:]
				if usedStarts[candidate.start] {
					continue
				}
				if !decorativeTracePathIsClear(candidate.path, allowed, blocked, reserved) {
					continue
				}

				traces = append(traces, hexgrid.GridTrace{
					Start:        candidate.start,
					Path:         compressPath(candidate.path),
					Lines:        randomLines(rng, o.MaxParallel),
					HideTerminal: false,
				})
				usedStarts[candidate.start] = true
				for _, position := range candidate.path[1:] {
					reserved[position] = true
				}
				progress = true
				break
			}
			if len(traces) >= o.DecorativeTraceCount {
				break
			}
		}
		if !progress {
			break
		}
	}
	return traces
}

// randomDecorativeTracePath generates a self-avoiding path with a randomized
// length and at least one 60-degree bend. Turns are placed between straight
// segments; only cells in the allowed routing region may be traversed.
func randomDecorativeTracePath(
	start, firstDirection hexgrid.GridPosition,
	o Options,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
	rng *rand.Rand,
) []hexgrid.GridPosition {
	directionIndex := -1
	for i, direction := range axialDirections {
		if direction == firstDirection {
			directionIndex = i
			break
		}
	}
	if directionIndex < 0 {
		return nil
	}

	length := o.DecorativeTraceMinLength + rng.Intn(o.DecorativeTraceMaxLength-o.DecorativeTraceMinLength+1)
	maxBends := o.DecorativeTraceMaxBends
	if maxBends > length-1 {
		maxBends = length - 1
	}
	bendCount := 1 + rng.Intn(maxBends)

	possibleTurns := make([]int, length-1)
	for i := range possibleTurns {
		possibleTurns[i] = i + 1 // Turn after this many steps.
	}
	rng.Shuffle(len(possibleTurns), func(i, j int) {
		possibleTurns[i], possibleTurns[j] = possibleTurns[j], possibleTurns[i]
	})
	turns := append([]int(nil), possibleTurns[:bendCount]...)
	sort.Ints(turns)
	turnAt := make(map[int]bool, bendCount)
	for _, turn := range turns {
		turnAt[turn] = true
	}

	path := make([]hexgrid.GridPosition, 1, length+1)
	path[0] = start
	visited := map[hexgrid.GridPosition]bool{start: true}
	for step := 1; step <= length; step++ {
		if turnAt[step-1] {
			if rng.Intn(2) == 0 {
				directionIndex = (directionIndex + 1) % len(axialDirections)
			} else {
				directionIndex = (directionIndex + len(axialDirections) - 1) % len(axialDirections)
			}
		}
		next := addPosition(path[len(path)-1], axialDirections[directionIndex])
		if !allowed[next] || blocked[next] || reserved[next] || visited[next] {
			return nil
		}
		path = append(path, next)
		visited[next] = true
	}

	end := path[len(path)-1]
	for _, direction := range axialDirections {
		neighbor := addPosition(end, direction)
		if blocked[neighbor] || reserved[neighbor] {
			return nil
		}
	}
	return path
}

func decorativeTracePathIsClear(
	path []hexgrid.GridPosition,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) bool {
	if len(path) < 4 {
		return false
	}
	for _, position := range path[1:] {
		if !allowed[position] || blocked[position] || reserved[position] {
			return false
		}
	}
	end := path[len(path)-1]
	for _, direction := range axialDirections {
		neighbor := addPosition(end, direction)
		if blocked[neighbor] || reserved[neighbor] {
			return false
		}
	}
	return true
}
