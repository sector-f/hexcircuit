package pcb

import (
	"math/rand"

	"github.com/sector-f/hexgrid"
)

type decorativeTraceCandidate struct {
	path []hexgrid.GridPosition // Full cell path, including the source component.
}

type decorativeTraceSource struct {
	path         []hexgrid.GridPosition
	allowReverse bool
}

// addDecorativeTraces adds free-ended base-color traces alongside existing power,
// inter-cluster, and previously accepted decorative routes. Each new trace starts
// at an existing route endpoint component, steps sideways, then follows a translated
// section of the source route. Placement is best-effort.
func addDecorativeTraces(
	o Options,
	rng *rand.Rand,
	powerRoutes [][]hexgrid.GridPosition,
	interClusterTraces []hexgrid.GridTrace,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if o.DecorativeTraceCount <= 0 {
		return nil
	}
	if reserved == nil {
		reserved = make(map[hexgrid.GridPosition]bool)
	}

	sources := make([]decorativeTraceSource, 0, len(powerRoutes)+len(interClusterTraces))
	for _, path := range powerRoutes {
		if len(path) >= 2 {
			sources = append(sources, decorativeTraceSource{path: append([]hexgrid.GridPosition(nil), path...), allowReverse: true})
		}
	}
	for _, trace := range interClusterTraces {
		path := expandCompressedTrace(trace)
		if len(path) >= 2 {
			sources = append(sources, decorativeTraceSource{path: path, allowReverse: true})
		}
	}

	candidates := make([]decorativeTraceCandidate, 0)
	for _, source := range sources {
		candidates = append(candidates, decorativeParallelCandidates(source.path, source.allowReverse, o)...)
	}
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	traces := make([]hexgrid.GridTrace, 0, o.DecorativeTraceCount)
	for next := 0; next < len(candidates) && len(traces) < o.DecorativeTraceCount; next++ {
		candidate := candidates[next]
		if !decorativeTracePathIsClear(candidate.path, allowed, blocked, reserved) {
			continue
		}

		traces = append(traces, hexgrid.GridTrace{
			Start:        candidate.path[0],
			Path:         compressPath(candidate.path),
			Lines:        randomLines(rng, o.MaxParallel),
			HideTerminal: false,
		})
		for _, position := range candidate.path[1:] {
			reserved[position] = true
		}

		// A newly placed decorative trace becomes a source for later traces.
		// Only its component-backed start can anchor another route; its terminal
		// pad is intentionally not treated as a component.
		additional := decorativeParallelCandidates(candidate.path, false, o)
		rng.Shuffle(len(additional), func(i, j int) {
			additional[i], additional[j] = additional[j], additional[i]
		})
		candidates = append(candidates, additional...)
	}
	return traces
}

// decorativeRouteReservations returns the occupied cells belonging to power and
// inter-cluster routes. Local intra-cluster signal edges are deliberately excluded;
// decorative traces are designed to echo these two longer route families.
func decorativeRouteReservations(
	powerRoutes [][]hexgrid.GridPosition,
	interClusterTraces []hexgrid.GridTrace,
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
	return reserved
}

// decorativeParallelCandidates creates one-cell-offset copies of a source route.
// The first step leaves the existing component sideways; the remaining steps follow
// the source path with an identical translation, preserving every bend.
func decorativeParallelCandidates(
	sourcePath []hexgrid.GridPosition,
	allowReverse bool,
	o Options,
) []decorativeTraceCandidate {
	if len(sourcePath) < o.DecorativeTraceMinLength {
		return nil
	}

	orientations := [][]hexgrid.GridPosition{
		append([]hexgrid.GridPosition(nil), sourcePath...),
	}
	if allowReverse {
		reversed := append([]hexgrid.GridPosition(nil), sourcePath...)
		reverseGridPath(reversed)
		orientations = append(orientations, reversed)
	}

	candidates := make([]decorativeTraceCandidate, 0)
	for _, path := range orientations {
		firstDirection := subtractPosition(path[1], path[0])
		directionIndex := -1
		for i, direction := range axialDirections {
			if direction == firstDirection {
				directionIndex = i
				break
			}
		}
		if directionIndex < 0 {
			continue
		}

		// Exclude offsets parallel to the source segment. The other four adjacent
		// lattice directions place the decorative run on either side of the trace.
		offsets := make([]hexgrid.GridPosition, 0, 4)
		for i, direction := range axialDirections {
			if i == directionIndex || i == (directionIndex+3)%len(axialDirections) {
				continue
			}
			offsets = append(offsets, direction)
		}

		maxLength := o.DecorativeTraceMaxLength
		if maxLength > len(path) {
			maxLength = len(path)
		}
		for length := o.DecorativeTraceMinLength; length <= maxLength; length++ {
			for _, offset := range offsets {
				candidate := make([]hexgrid.GridPosition, 0, length+1)
				candidate = append(candidate, path[0], addPosition(path[0], offset))
				for i := 1; i < length; i++ {
					candidate = append(candidate, addPosition(path[i], offset))
				}
				if decorativeTraceBendCount(candidate) > o.DecorativeTraceMaxBends {
					continue
				}
				candidates = append(candidates, decorativeTraceCandidate{path: candidate})
			}
		}
	}
	return candidates
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
		validDirection := false
		for _, candidate := range axialDirections {
			if candidate == direction {
				validDirection = true
				break
			}
		}
		if !validDirection ||
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
	return true
}

func reverseGridPath(path []hexgrid.GridPosition) {
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
}

func subtractPosition(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q - b.Q, R: a.R - b.R}
}
