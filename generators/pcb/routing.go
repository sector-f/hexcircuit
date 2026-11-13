package pcb

import (
	"container/heap"
	"math"
	"math/rand"

	"github.com/sector-f/hexgrid"
	"github.com/sector-f/hexgrid/internal/gridgeom"
)

type routeSearchState struct {
	position          hexgrid.GridPosition
	incomingDirection int // -1 for the initial state; otherwise an axialDirections index.
}

type routeQueueItem struct {
	state    routeSearchState
	priority float64
	index    int
}

type routePriorityQueue []*routeQueueItem

func (q routePriorityQueue) Len() int           { return len(q) }
func (q routePriorityQueue) Less(i, j int) bool { return q[i].priority < q[j].priority }
func (q routePriorityQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *routePriorityQueue) Push(value any) {
	item := value.(*routeQueueItem)
	item.index = len(*q)
	*q = append(*q, item)
}
func (q *routePriorityQueue) Pop() any {
	old := *q
	last := old[len(old)-1]
	old[len(old)-1] = nil
	*q = old[:len(old)-1]
	return last
}

func routingCells(o Options) map[hexgrid.GridPosition]bool {
	margin := math.Max(o.ComponentRadius+o.ComponentStrokeWidth/2, o.ConnectorInset)
	cells := gridgeom.Cells(o.Width, o.Height, o.Pitch, "pointy", margin)
	allowed := make(map[hexgrid.GridPosition]bool, len(cells))
	for _, cell := range cells {
		allowed[hexgrid.GridPosition{Q: cell.Q, R: cell.R}] = true
	}
	return allowed
}

func powerSourceRowIDs(topSources, bottomSources []hexgrid.GridPosition) map[int]bool {
	rows := make(map[int]bool, 2)
	if len(topSources) > 0 {
		rows[topSources[0].R] = true
	}
	if len(bottomSources) > 0 {
		rows[bottomSources[0].R] = true
	}
	return rows
}

func routingCellsWithoutPowerSourceRows(o Options, topSources, bottomSources []hexgrid.GridPosition) map[hexgrid.GridPosition]bool {
	allowed := routingCells(o)
	rows := powerSourceRowIDs(topSources, bottomSources)
	for position := range allowed {
		if rows[position.R] {
			delete(allowed, position)
		}
	}
	return allowed
}

func routingCellsWithinPowerSourceCorridor(
	allowed map[hexgrid.GridPosition]bool,
	topSources, bottomSources []hexgrid.GridPosition,
	o Options,
) map[hexgrid.GridPosition]bool {
	topY, bottomY := math.Inf(-1), math.Inf(1)
	if len(topSources) > 0 {
		topY = gridgeom.PositionToPoint(topSources[0].Q, topSources[0].R, o.Width, o.Height, o.Pitch, "pointy").Y
	}
	if len(bottomSources) > 0 {
		bottomY = gridgeom.PositionToPoint(bottomSources[0].Q, bottomSources[0].R, o.Width, o.Height, o.Pitch, "pointy").Y
	}
	filtered := make(map[hexgrid.GridPosition]bool, len(allowed))
	const epsilon = 1e-9
	for position := range allowed {
		y := gridgeom.PositionToPoint(position.Q, position.R, o.Width, o.Height, o.Pitch, "pointy").Y
		if y > topY+epsilon && y < bottomY-epsilon {
			filtered[position] = true
		}
	}
	return filtered
}

func copyAllowedRoutes(allowed map[hexgrid.GridPosition]bool) map[hexgrid.GridPosition]bool {
	copied := make(map[hexgrid.GridPosition]bool, len(allowed))
	for position := range allowed {
		copied[position] = true
	}
	return copied
}

func routingCellsWithinPowerSourceSpan(
	allowed map[hexgrid.GridPosition]bool,
	sources []powerSource,
	o Options,
) map[hexgrid.GridPosition]bool {
	minimumX, maximumX := math.Inf(1), math.Inf(-1)
	for _, source := range sources {
		x := gridgeom.PositionToPoint(source.Position.Q, source.Position.R, o.Width, o.Height, o.Pitch, "pointy").X
		minimumX = math.Min(minimumX, x)
		maximumX = math.Max(maximumX, x)
	}
	filtered := make(map[hexgrid.GridPosition]bool, len(allowed))
	if math.IsInf(minimumX, 0) || math.IsInf(maximumX, 0) {
		return filtered
	}
	const epsilon = 1e-9
	for position := range allowed {
		x := gridgeom.PositionToPoint(position.Q, position.R, o.Width, o.Height, o.Pitch, "pointy").X
		if x >= minimumX-epsilon && x <= maximumX+epsilon {
			filtered[position] = true
		}
	}
	return filtered
}

func routingCellsWithinPowerSourceTrapezoid(
	allowed map[hexgrid.GridPosition]bool,
	bounds powerSourceTrapezoid,
	o Options,
) map[hexgrid.GridPosition]bool {
	filtered := make(map[hexgrid.GridPosition]bool, len(allowed))
	for position := range allowed {
		if bounds.contains(position, o) {
			filtered[position] = true
		}
	}
	return filtered
}

// findRoute performs turn-aware A* on the axial lattice. A route reserves
// every lattice vertex it visits, so subsequent routes cannot share a vertex
// or cross an existing route. Components are preferred as obstacles, but may
// be used as concealed waypoints with a routing penalty. In empty space,
// direction changes are limited to one adjacent hex-grid axis (a 60-degree
// heading change). The sharp 60-degree inside corner created by a 120-degree
// heading change is allowed only at a component. The sourceSide argument
// restricts the first step so a top source exits downward and a bottom source
// exits upward.
func findRoute(
	start, goal hexgrid.GridPosition,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
	sourceSide powerSide,
	rng *rand.Rand,
) ([]hexgrid.GridPosition, bool) {
	if start == goal || !allowed[start] || !allowed[goal] {
		return nil, false
	}
	if !blocked[start] || !blocked[goal] {
		return nil, false
	}

	startState := routeSearchState{position: start, incomingDirection: -1}
	gScore := map[routeSearchState]float64{startState: 0}
	cameFrom := make(map[routeSearchState]routeSearchState)
	closed := make(map[routeSearchState]bool)
	frontier := &routePriorityQueue{}
	heap.Init(frontier)
	heap.Push(frontier, &routeQueueItem{
		state:    startState,
		priority: float64(hexDistance(start, goal)),
	})

	for frontier.Len() > 0 {
		currentState := heap.Pop(frontier).(*routeQueueItem).state
		if closed[currentState] {
			continue
		}
		current := currentState.position
		if current == goal {
			path := reconstructRoutePath(startState, currentState, cameFrom)
			return path, len(path) >= 2 && hasNoSharpTraceTurns(path, blocked)
		}
		closed[currentState] = true

		directionOrder := []int{0, 1, 2, 3, 4, 5}
		rng.Shuffle(len(directionOrder), func(i, j int) { directionOrder[i], directionOrder[j] = directionOrder[j], directionOrder[i] })
		for _, directionIndex := range directionOrder {
			direction := axialDirections[directionIndex]
			if current == start && !allowedPowerDeparture(direction, sourceSide) {
				continue
			}
			if currentState.incomingDirection >= 0 &&
				!traceTurnAllowed(currentState.incomingDirection, directionIndex, blocked[current]) {
				continue
			}
			neighbor := addPosition(current, direction)
			if !allowed[neighbor] {
				continue
			}
			if neighbor != goal && neighbor != start && reserved[neighbor] {
				continue
			}
			if routePathContainsPosition(currentState, neighbor, cameFrom) {
				continue
			}

			nextState := routeSearchState{position: neighbor, incomingDirection: directionIndex}
			if closed[nextState] {
				continue
			}

			stepCost := 1.0 + rng.Float64()*0.035
			if blocked[neighbor] && neighbor != goal {
				// Prefer routing around components, but permit a path to pass
				// through one when that is the only clean way to continue.
				stepCost += 1.5
			}
			candidateScore := gScore[currentState] + stepCost
			previous, exists := gScore[nextState]
			if exists && candidateScore >= previous {
				continue
			}
			cameFrom[nextState] = currentState
			gScore[nextState] = candidateScore
			priority := candidateScore + float64(hexDistance(neighbor, goal))
			heap.Push(frontier, &routeQueueItem{state: nextState, priority: priority})
		}
	}
	return nil, false
}

// traceTurnAllowed prevents acute free-space corners. Consecutive axial
// directions may stay the same or move to either adjacent direction in the
// six-direction cycle. A component cell is exempt because the trace is
// visually entering, crossing, or leaving that component.
func traceTurnAllowed(previousDirection, nextDirection int, throughComponent bool) bool {
	if throughComponent {
		return true
	}
	if previousDirection < 0 || previousDirection >= len(axialDirections) ||
		nextDirection < 0 || nextDirection >= len(axialDirections) {
		return false
	}
	difference := absInt(previousDirection - nextDirection)
	if difference > len(axialDirections)/2 {
		difference = len(axialDirections) - difference
	}
	return difference <= 1
}

// hasNoSharpTraceTurns validates every free-space bend in a complete lattice path.
// The components map marks positions where a turn is visually absorbed by a component.
func hasNoSharpTraceTurns(path []hexgrid.GridPosition, components map[hexgrid.GridPosition]bool) bool {
	for i := 1; i < len(path)-1; i++ {
		if components[path[i]] {
			continue
		}
		incoming := directionIndex(subtractPosition(path[i], path[i-1]))
		outgoing := directionIndex(subtractPosition(path[i+1], path[i]))
		if !traceTurnAllowed(incoming, outgoing, false) {
			return false
		}
	}
	return true
}

func directionIndex(direction hexgrid.GridPosition) int {
	for i, candidate := range axialDirections {
		if candidate == direction {
			return i
		}
	}
	return -1
}

func routePathContainsPosition(
	current routeSearchState,
	position hexgrid.GridPosition,
	cameFrom map[routeSearchState]routeSearchState,
) bool {
	for {
		if current.position == position {
			return true
		}
		previous, ok := cameFrom[current]
		if !ok {
			return false
		}
		current = previous
	}
}

func reconstructRoutePath(
	start, goal routeSearchState,
	cameFrom map[routeSearchState]routeSearchState,
) []hexgrid.GridPosition {
	states := []routeSearchState{goal}
	current := goal
	for current != start {
		previous, ok := cameFrom[current]
		if !ok {
			return nil
		}
		current = previous
		states = append(states, current)
	}
	path := make([]hexgrid.GridPosition, len(states))
	for i := range states {
		path[len(states)-1-i] = states[i].position
	}
	return path
}

func allowedPowerDeparture(direction hexgrid.GridPosition, side powerSide) bool {
	switch side {
	case powerSideTop:
		// Pointy-top axial coordinates: these are the two southward directions.
		return direction.R == 1 && (direction.Q == 0 || direction.Q == -1)
	case powerSideBottom:
		// These are the two northward directions.
		return direction.R == -1 && (direction.Q == 0 || direction.Q == 1)
	default:
		return true
	}
}

func reconstructPath(start, goal hexgrid.GridPosition, cameFrom map[hexgrid.GridPosition]hexgrid.GridPosition) []hexgrid.GridPosition {
	path := []hexgrid.GridPosition{goal}
	current := goal
	for current != start {
		previous, ok := cameFrom[current]
		if !ok {
			return nil
		}
		current = previous
		path = append(path, current)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

// compressPath retains only direction changes, producing the axis-aligned waypoints
// expected by hexgrid.GridTrace while leaving the full cell path available for routing.
func compressPath(path []hexgrid.GridPosition) []hexgrid.GridPosition {
	if len(path) < 2 {
		return nil
	}
	result := make([]hexgrid.GridPosition, 0, len(path)-1)
	previousDirection := hexgrid.GridPosition{
		Q: path[1].Q - path[0].Q,
		R: path[1].R - path[0].R,
	}
	for i := 2; i < len(path); i++ {
		direction := hexgrid.GridPosition{
			Q: path[i].Q - path[i-1].Q,
			R: path[i].R - path[i-1].R,
		}
		if direction != previousDirection {
			result = append(result, path[i-1])
			previousDirection = direction
		}
	}
	result = append(result, path[len(path)-1])
	return result
}

func addExtraSignalConnections(
	o Options,
	rng *rand.Rand,
	clusters []clusterPlan,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) ([]hexgrid.GridTrace, error) {
	type pair struct {
		a, b hexgrid.GridPosition
	}
	candidates := make([]pair, 0)
	for _, cluster := range clusters {
		for i := 0; i < len(cluster.Components); i++ {
			for j := i + 1; j < len(cluster.Components); j++ {
				a, b := cluster.Components[i].Position, cluster.Components[j].Position
				if hasTreeEdge(cluster, a, b) {
					continue
				}
				candidates = append(candidates, pair{a: a, b: b})
			}
		}
	}
	rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	traces := make([]hexgrid.GridTrace, 0, o.Connections)
	for _, candidate := range candidates {
		if len(traces) >= o.Connections {
			break
		}
		path, ok := findRoute(candidate.a, candidate.b, allowed, blocked, reserved, powerSideAny, rng)
		if !ok || len(path) < 2 {
			continue
		}
		traces = append(traces, hexgrid.GridTrace{
			Start:        candidate.a,
			Path:         compressPath(path),
			Lines:        randomLines(rng, o.MaxParallel),
			HideTerminal: true,
		})
		for _, position := range path[1 : len(path)-1] {
			reserved[position] = true
		}
	}
	return traces, nil
}

// addInterClusterConnections opportunistically adds accent-colored routes
// between distinct clusters. It chooses a random number of cluster pairs from
// zero through clusterCount-1, shuffles pair and endpoint order, and skips pairs
// for which no non-crossing route can be found.
func addInterClusterConnections(
	o Options,
	rng *rand.Rand,
	clusters []clusterPlan,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
) []hexgrid.GridTrace {
	if len(clusters) < 2 {
		return nil
	}

	type clusterPair struct {
		a, b int
	}
	pairs := make([]clusterPair, 0, len(clusters)*(len(clusters)-1)/2)
	for i := 0; i < len(clusters); i++ {
		for j := i + 1; j < len(clusters); j++ {
			pairs = append(pairs, clusterPair{a: i, b: j})
		}
	}
	rng.Shuffle(len(pairs), func(i, j int) { pairs[i], pairs[j] = pairs[j], pairs[i] })

	// A sparse random set of links creates occasional cross-cluster structure
	// without making the board's clusters look like a fully connected mesh.
	targetCount := rng.Intn(len(clusters))
	traces := make([]hexgrid.GridTrace, 0, targetCount)
	for _, pair := range pairs {
		if len(traces) >= targetCount {
			break
		}
		endpoints := make([][2]hexgrid.GridPosition, 0,
			len(clusters[pair.a].Components)*len(clusters[pair.b].Components))
		for _, a := range clusters[pair.a].Components {
			for _, b := range clusters[pair.b].Components {
				endpoints = append(endpoints, [2]hexgrid.GridPosition{a.Position, b.Position})
			}
		}
		rng.Shuffle(len(endpoints), func(i, j int) { endpoints[i], endpoints[j] = endpoints[j], endpoints[i] })
		for _, endpoint := range endpoints {
			path, ok := findRoute(endpoint[0], endpoint[1], allowed, blocked, reserved, powerSideAny, rng)
			if !ok || len(path) < 2 {
				continue
			}
			traces = append(traces, hexgrid.GridTrace{
				Start:        endpoint[0],
				Path:         compressPath(path),
				Lines:        randomLines(rng, o.MaxParallel),
				HideTerminal: true,
			})
			for _, position := range path[1 : len(path)-1] {
				reserved[position] = true
			}
			break
		}
	}
	return traces
}

func hasTreeEdge(cluster clusterPlan, a, b hexgrid.GridPosition) bool {
	for _, component := range cluster.Components {
		if component.Parent == nil {
			continue
		}
		parent := *component.Parent
		if parent == a && component.Position == b || parent == b && component.Position == a {
			return true
		}
	}
	return false
}
