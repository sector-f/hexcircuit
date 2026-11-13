package pcb

import (
	"math/rand"
	"sort"

	"github.com/sector-f/hexgrid"
)

const (
	decorativeTraceMaxSourceDistance = 2
	decorativeTraceMaxConnectorLength = 3
)

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

	startGroups := make([][]hexgrid.GridPosition, len(clusters))
	for clusterIndex, cluster := range clusters {
		children := make(map[hexgrid.GridPosition]int, len(cluster.Components))
		for _, component := range cluster.Components {
			if component.Parent != nil {
				children[*component.Parent]++
			}
		}
		// Leaf components are visually preferable origins, so try them first.
		// Internal components remain eligible when the requested count exceeds
		// what can be placed from leaves alone.
	leafStarts := make([]hexgrid.GridPosition, 0, len(cluster.Components))
		internalStarts := make([]hexgrid.GridPosition, 0, len(cluster.Components))
		for _, component := range cluster.Components {
			if children[component.Position] == 0 {
				leafStarts = append(leafStarts, component.Position)
			} else {
				internalStarts = append(internalStarts, component.Position)
			}
		}
		rng.Shuffle(len(leafStarts), func(i, j int) {
			leafStarts[i], leafStarts[j] = leafStarts[j], leafStarts[i]
		})
		rng.Shuffle(len(internalStarts), func(i, j int) {
			internalStarts[i], internalStarts[j] = internalStarts[j], internalStarts[i]
		})
		startGroups[clusterIndex] = append(leafStarts, internalStarts...)
	}
	sources := decorativeTraceSourcePaths(powerRoutes, interClusterTraces, nil)
	return addDecorativeTraceGroups(
		o, rng, startGroups, sources, o.DecorativeTraceCount,
		allowed, blocked, reserved,
	)
}

// addDecorativePowerTraces adds base-color decorations from configured physical
// source contacts that were not assigned to active power feeds. Existing normal
// decorative traces are eligible as parallel source routes.
func addDecorativePowerTraces(
	o Options,
	rng *rand.Rand,
	unusedPowerSources []hexgrid.GridPosition,
	powerRoutes [][]hexgrid.GridPosition,
	interClusterTraces []hexgrid.GridTrace,
	priorDecorativeTraces []hexgrid.GridTrace,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if o.DecorativePowerTraceCount <= 0 || len(unusedPowerSources) == 0 {
		return nil
	}
	sources := decorativeTraceSourcePaths(powerRoutes, interClusterTraces, priorDecorativeTraces)
	starts := append([]hexgrid.GridPosition(nil), unusedPowerSources...)
	rng.Shuffle(len(starts), func(i, j int) {
		starts[i], starts[j] = starts[j], starts[i]
	})
	return addDecorativeTraceGroups(
		o, rng, [][]hexgrid.GridPosition{starts},
		sources, o.DecorativePowerTraceCount,
		allowed, blocked, reserved,
	)
}

// unusedPowerSourcePositions returns all physical contacts that were not assigned
// to a cluster's active power-feed list.
func unusedPowerSourcePositions(
	topSources, bottomSources []hexgrid.GridPosition,
	clusters []clusterPlan,
) []hexgrid.GridPosition {
	active := make(map[hexgrid.GridPosition]bool)
	for _, cluster := range clusters {
		for _, source := range cluster.PowerSources {
			active[source.Position] = true
		}
	}

	unused := make([]hexgrid.GridPosition, 0, len(topSources)+len(bottomSources)-len(active))
	for _, source := range append(append([]hexgrid.GridPosition(nil), topSources...), bottomSources...) {
		if !active[source] {
			unused = append(unused, source)
		}
	}
	return unused
}

func decorativeTraceSourcePaths(
	powerRoutes [][]hexgrid.GridPosition,
	interClusterTraces []hexgrid.GridTrace,
	decorativeTraces []hexgrid.GridTrace,
) [][]hexgrid.GridPosition {
	sources := make([][]hexgrid.GridPosition, 0, len(powerRoutes)+len(interClusterTraces)+len(decorativeTraces))
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
	for _, trace := range decorativeTraces {
		if path := expandCompressedTrace(trace); len(path) >= 2 {
			sources = append(sources, path)
		}
	}
	return sources
}

func addDecorativeTraceGroups(
	o Options,
	rng *rand.Rand,
	startGroups [][]hexgrid.GridPosition,
	sources [][]hexgrid.GridPosition,
	count int,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if count <= 0 || len(startGroups) == 0 || len(sources) == 0 {
		return nil
	}
	if reserved == nil {
		reserved = make(map[hexgrid.GridPosition]bool)
	}

	groupOrder := make([]int, len(startGroups))
	for i := range groupOrder {
		groupOrder[i] = i
	}
	rng.Shuffle(len(groupOrder), func(i, j int) {
		groupOrder[i], groupOrder[j] = groupOrder[j], groupOrder[i]
	})

	traces := make([]hexgrid.GridTrace, 0, count)
	round := 0
	for len(traces) < count {
		progress := false
		for _, groupIndex := range groupOrder {
			starts := startGroups[groupIndex]
			if len(starts) == 0 {
				continue
			}
			addedThisGroup := false
			startOffset := round % len(starts)
			for startIndex := 0; startIndex < len(starts); startIndex++ {
				start := starts[(startOffset+startIndex)%len(starts)]
				candidates := decorativeParallelCandidates(start, sources, o, allowed, blocked, reserved)
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
					for _, position := range candidate.path[1:] {
						reserved[position] = true
					}
					// Each accepted decoration becomes a source for later traces.
					sources = append(sources, append([]hexgrid.GridPosition(nil), candidate.path...))
					progress = true
					addedThisGroup = true
					break
				}
				if addedThisGroup || len(traces) >= count {
					break
				}
			}
			if len(traces) >= count {
				break
			}
		}
		if !progress {
			break
		}
		round++
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
// decorativeParallelCandidates finds nearby source-route segments and creates
// offset copies that can be reached from start with up to two clear steps.
// Closer source segments are tried first.
func decorativeParallelCandidates(
	start hexgrid.GridPosition,
	sourcePaths [][]hexgrid.GridPosition,
	o Options,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []decorativeTraceCandidate {
	candidates := make([]decorativeTraceCandidate, 0)
	for _, source := range sourcePaths {
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

				for _, offset := range axialDirections {
					if offset == tangent || offset == negatePosition(tangent) {
						continue
					}
					firstParallelCell := addPosition(path[sourceIndex], offset)
					connectors := decorativeConnectorPaths(start, firstParallelCell, allowed, blocked, reserved)
					for _, connector := range connectors {
						connectorSteps := len(connector) - 1
						minFollowSteps := o.DecorativeTraceMinLength - connectorSteps
						if minFollowSteps < 1 {
							minFollowSteps = 1
						}
						maxFollowSteps := o.DecorativeTraceMaxLength - connectorSteps
						if available := len(path) - sourceIndex - 1; maxFollowSteps > available {
							maxFollowSteps = available
						}
						for followSteps := minFollowSteps; followSteps <= maxFollowSteps; followSteps++ {
							candidatePath := append([]hexgrid.GridPosition(nil), connector...)
							for i := 1; i <= followSteps; i++ {
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
	}
	return candidates
}

// decorativeConnectorPaths returns all clear shortest paths of at most two
// steps from a component to the first cell of a parallel run. Intermediate
// cells may not cross components or any existing route.
// decorativeConnectorPaths enumerates simple, turn-safe paths of at most
// decorativeTraceMaxConnectorLength steps from a component to the first cell
// of a parallel run. This includes short detours when the shortest connector
// intersects a component or an existing route.
func decorativeConnectorPaths(
	start, target hexgrid.GridPosition,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) [][]hexgrid.GridPosition {
	distance := hexDistance(start, target)
	if distance < 1 || distance > decorativeTraceMaxConnectorLength ||
		!allowed[target] || blocked[target] || reserved[target] {
		return nil
	}

	connectors := make([][]hexgrid.GridPosition, 0, 4)
	queue := [][]hexgrid.GridPosition{{start}}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if len(path)-1 >= decorativeTraceMaxConnectorLength {
			continue
		}
		current := path[len(path)-1]
		previousDirection := -1
		if len(path) > 1 {
			previousDirection = directionIndex(subtractPosition(current, path[len(path)-2]))
		}

		for nextDirection, direction := range axialDirections {
			next := addPosition(current, direction)
			if previousDirection >= 0 && !traceTurnAllowed(previousDirection, nextDirection, false) {
				continue
			}
			if next == target {
				connector := append(append([]hexgrid.GridPosition(nil), path...), target)
				connectors = append(connectors, connector)
				continue
			}
			if !allowed[next] || blocked[next] || reserved[next] ||
				containsGridPosition(path, next) {
				continue
			}
			queue = append(queue, append(append([]hexgrid.GridPosition(nil), path...), next))
		}
	}
	return connectors
}

func containsGridPosition(path []hexgrid.GridPosition, position hexgrid.GridPosition) bool {
	for _, existing := range path {
		if existing == position {
			return true
		}
	}
	return false
}

func shuffleDecorativeCandidates(candidates []decorativeTraceCandidate, rng *rand.Rand) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].sourceDistance != candidates[j].sourceDistance {
			return candidates[i].sourceDistance < candidates[j].sourceDistance
		}
		if len(candidates[i].path) != len(candidates[j].path) {
			return len(candidates[i].path) < len(candidates[j].path)
		}
		return decorativeTraceBendCount(candidates[i].path) < decorativeTraceBendCount(candidates[j].path)
	})
	for start := 0; start < len(candidates); {
		end := start + 1
		for end < len(candidates) &&
			candidates[end].sourceDistance == candidates[start].sourceDistance &&
			len(candidates[end].path) == len(candidates[start].path) &&
			decorativeTraceBendCount(candidates[end].path) == decorativeTraceBendCount(candidates[start].path) {
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
	if len(path) < 4 || !blocked[path[0]] || !hasNoSharpTraceTurns(path, blocked) {
		return false
	}
	visited := make(map[hexgrid.GridPosition]bool, len(path))
	visited[path[0]] = true
	for i, position := range path[1:] {
		previous := path[i]
		pathIndex := i + 1
		if hexDistance(previous, position) != 1 ||
			!allowed[position] || reserved[position] || visited[position] {
			return false
		}
		if blocked[position] && !hasSharpTurnAt(path, pathIndex) {
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

func hasSharpTurnAt(path []hexgrid.GridPosition, index int) bool {
	if index <= 0 || index >= len(path)-1 {
		return false
	}
	incoming := directionIndex(subtractPosition(path[index], path[index-1]))
	outgoing := directionIndex(subtractPosition(path[index+1], path[index]))
	if incoming < 0 || outgoing < 0 {
		return false
	}
	difference := absInt(incoming - outgoing)
	if difference > len(axialDirections)/2 {
		difference = len(axialDirections) - difference
	}
	return difference == 2
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
