package pcb

import (
	"math/rand"
	"sort"

	"github.com/sector-f/hexgrid"
)

const decorativeTraceMaxSourceDistance = 2

type decorativeTraceCandidate struct {
	path           []hexgrid.GridPosition // Full cell path, including the source component.
	sourcePath     []hexgrid.GridPosition // Existing path this candidate intentionally parallels.
	sourceDistance int
}

// addDecorativeTraces creates free-ended base-color traces from cluster leaf
// components. Each leaf looks for a nearby segment of an existing power,
// inter-cluster, or decorative trace, steps onto a one-cell-offset copy, and
// follows that route's bends. Placement is best-effort.
func addDecorativeTraces(
	o Options,
	rng *rand.Rand,
	clusters []clusterPlan,
	powerRoutes [][]hexgrid.GridPosition,
	interClusterTraces []hexgrid.GridTrace,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if o.DecorativeTraceCount <= 0 || len(clusters) == 0 {
		return nil
	}
	if reserved == nil {
		reserved = make(map[hexgrid.GridPosition]bool)
	}

	sources := make([][]hexgrid.GridPosition, 0, len(powerRoutes)+len(interClusterTraces)+o.DecorativeTraceCount)
	for _, path := range powerRoutes {
		if len(path) >= 2 {
			sources = append(sources, append([]hexgrid.GridPosition(nil), path...))
		}
	}
	for _, trace := range interClusterTraces {
		if path := expandCompressedTrace(trace); len(path) >= 2 {
			sources = append(sources, path)
		}
	}

	byCluster := make([][]hexgrid.GridPosition, len(clusters))
	for clusterIndex, cluster := range clusters {
		children := make(map[hexgrid.GridPosition]int, len(cluster.Components))
		for _, component := range cluster.Components {
			if component.Parent != nil {
				children[*component.Parent]++
			}
		}
		for _, component := range cluster.Components {
			if children[component.Position] == 0 {
				byCluster[clusterIndex] = append(byCluster[clusterIndex], component.Position)
			}
		}
		rng.Shuffle(len(byCluster[clusterIndex]), func(i, j int) {
			byCluster[clusterIndex][i], byCluster[clusterIndex][j] = byCluster[clusterIndex][j], byCluster[clusterIndex][i]
		})
	}

	clusterOrder := make([]int, len(clusters))
	for i := range clusterOrder {
		clusterOrder[i] = i
	}
	rng.Shuffle(len(clusterOrder), func(i, j int) {
		clusterOrder[i], clusterOrder[j] = clusterOrder[j], clusterOrder[i]
	})

	usedStarts := make(map[hexgrid.GridPosition]bool)
	traces := make([]hexgrid.GridTrace, 0, o.DecorativeTraceCount)
	for len(traces) < o.DecorativeTraceCount {
		progress := false
		for _, clusterIndex := range clusterOrder {
			addedThisCluster := false
			for _, start := range byCluster[clusterIndex] {
				if usedStarts[start] {
					continue
				}
				candidates := decorativeParallelCandidates(start, sources, o)
				shuffleDecorativeCandidates(candidates, rng)
				for _, candidate := range candidates {
					if !decorativeTracePathIsClear(candidate.path, allowed, blocked, reserved, candidate.sourcePath) {
						continue
					}
					traces = append(traces, hexgrid.GridTrace{
						Start:        start,
						Path:         compressPath(candidate.path),
						Lines:        randomLines(rng, o.MaxParallel),
						HideTerminal: false,
					})
					usedStarts[start] = true
					for _, position := range candidate.path[1:] {
						reserved[position] = true
					}
					// Accepted decorations become sources for later leaf components.
					sources = append(sources, append([]hexgrid.GridPosition(nil), candidate.path...))
					progress = true
					addedThisCluster = true
					break
				}
				if addedThisCluster || len(traces) >= o.DecorativeTraceCount {
					break
				}
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

// decorativeRouteReservations marks every cell used by power or inter-cluster
// routes. Decorative paths are added to this map incrementally.
func decorativeRouteReservations(
	powerRoutes [][]hexgrid.GridPosition,
	interClusterTraces []hexgrid.GridTrace,
	signalTraceGroups ...[]hexgrid.GridTrace,
) map[hexgrid.GridPosition]bool {
	reserved := make(map[hexgrid.GridPosition]bool)
	for _, path := range powerRoutes {
		for _, position := range path {
			reserved[position] = true
		}
	}
	for _, trace := range interClusterTraces {
		for _, position := range expandCompressedTrace(trace) {
			reserved[position] = true
		}
	}
	for _, traces := range signalTraceGroups {
		for _, trace := range traces {
			for _, position := range expandCompressedTrace(trace) {
				reserved[position] = true
			}
		}
	}
	return reserved
}

// decorativeParallelCandidates finds nearby source-route segments and creates
// offset copies that can be reached from start in one lattice step. Closer source
// segments are tried before more distant ones.
func decorativeParallelCandidates(
	start hexgrid.GridPosition,
	sourcePaths [][]hexgrid.GridPosition,
	o Options,
) []decorativeTraceCandidate {
	candidates := make([]decorativeTraceCandidate, 0)
	for _, source := range sourcePaths {
		if len(source) < o.DecorativeTraceMinLength {
			continue
		}
		for _, path := range [][]hexgrid.GridPosition{source, reversedCopy(source)} {
			for sourceIndex := 0; sourceIndex < len(path)-1; sourceIndex++ {
				distance := hexDistance(start, path[sourceIndex])
				if distance > decorativeTraceMaxSourceDistance {
					continue
				}
				tangent := subtractPosition(path[sourceIndex+1], path[sourceIndex])
				if !isAxialDirection(tangent) {
					continue
				}

				maxLength := o.DecorativeTraceMaxLength
				if remaining := len(path) - sourceIndex; maxLength > remaining {
					maxLength = remaining
				}
				for _, offset := range axialDirections {
					if offset == tangent || offset == negatePosition(tangent) {
						continue
					}
					firstParallelCell := addPosition(path[sourceIndex], offset)
					if hexDistance(start, firstParallelCell) != 1 {
						continue
					}

					for runLength := o.DecorativeTraceMinLength; runLength <= maxLength; runLength++ {
						candidatePath := make([]hexgrid.GridPosition, 0, runLength+1)
						candidatePath = append(candidatePath, start, firstParallelCell)
						for i := 1; i < runLength; i++ {
							candidatePath = append(candidatePath, addPosition(path[sourceIndex+i], offset))
						}
						if decorativeTraceBendCount(candidatePath) > o.DecorativeTraceMaxBends {
							continue
						}
						candidates = append(candidates, decorativeTraceCandidate{
							path:           candidatePath,
							sourcePath:     path,
							sourceDistance: distance,
						})
					}
				}
			}
		}
	}
	return candidates
}

func shuffleDecorativeCandidates(candidates []decorativeTraceCandidate, rng *rand.Rand) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].sourceDistance < candidates[j].sourceDistance
	})
	for start := 0; start < len(candidates); {
		end := start + 1
		for end < len(candidates) && candidates[end].sourceDistance == candidates[start].sourceDistance {
			end++
		}
		rng.Shuffle(end-start, func(i, j int) {
			candidates[start+i], candidates[start+j] = candidates[start+j], candidates[start+i]
		})
		start = end
	}
}

// expandCompressedTrace reconstructs every lattice cell along a GridTrace's
// compressed axis-aligned waypoints.
func expandCompressedTrace(trace hexgrid.GridTrace) []hexgrid.GridPosition {
	if len(trace.Path) == 0 {
		return nil
	}
	full := []hexgrid.GridPosition{trace.Start}
	current := trace.Start
	for _, waypoint := range trace.Path {
		distance := hexDistance(current, waypoint)
		if distance < 1 {
			return nil
		}
		direction := hexgrid.GridPosition{
			Q: (waypoint.Q - current.Q) / distance,
			R: (waypoint.R - current.R) / distance,
		}
		if !isAxialDirection(direction) ||
			current.Q+direction.Q*distance != waypoint.Q ||
			current.R+direction.R*distance != waypoint.R {
			return nil
		}
		for step := 0; step < distance; step++ {
			current = addPosition(current, direction)
			full = append(full, current)
		}
	}
	return full
}

func decorativeTraceBendCount(path []hexgrid.GridPosition) int {
	bends := 0
	var previous hexgrid.GridPosition
	for i := 1; i < len(path); i++ {
		direction := subtractPosition(path[i], path[i-1])
		if i > 1 && direction != previous {
			bends++
		}
		previous = direction
	}
	return bends
}

func decorativeTracePathIsClear(
	path []hexgrid.GridPosition,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
	sourcePath []hexgrid.GridPosition,
) bool {
	if len(path) < 4 || !blocked[path[0]] {
		return false
	}
	visited := make(map[hexgrid.GridPosition]bool, len(path))
	visited[path[0]] = true
	for i, position := range path[1:] {
		previous := path[i]
		if hexDistance(previous, position) != 1 ||
			!allowed[position] || blocked[position] || reserved[position] || visited[position] {
			return false
		}
		visited[position] = true
	}

	// Keep the hollow terminal pad clear of components and unrelated routes.
	// The source route itself is allowed to remain adjacent because the new
	// trace is deliberately running parallel to it.
	parallelSource := make(map[hexgrid.GridPosition]bool, len(sourcePath))
	for _, position := range sourcePath {
		parallelSource[position] = true
	}
	end := path[len(path)-1]
	for _, direction := range axialDirections {
		neighbor := addPosition(end, direction)
		if blocked[neighbor] || reserved[neighbor] && !parallelSource[neighbor] {
			return false
		}
	}
	return true
}

func isAxialDirection(direction hexgrid.GridPosition) bool {
	for _, candidate := range axialDirections {
		if candidate == direction {
			return true
		}
	}
	return false
}

func negatePosition(position hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: -position.Q, R: -position.R}
}

func reversedCopy(path []hexgrid.GridPosition) []hexgrid.GridPosition {
	result := append([]hexgrid.GridPosition(nil), path...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func subtractPosition(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q - b.Q, R: a.R - b.R}
}
