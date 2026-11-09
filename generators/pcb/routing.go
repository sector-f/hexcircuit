package pcb

import (
	"container/heap"
	"math/rand"

	"github.com/sector-f/hexgrid"
)

type routeQueueItem struct {
	position hexgrid.GridPosition
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
	cells := hexgrid.GridCells(
		o.Width,
		o.Height,
		o.Pitch,
		"pointy",
		o.ComponentRadius+o.ComponentStrokeWidth/2,
	)
	allowed := make(map[hexgrid.GridPosition]bool, len(cells))
	for _, cell := range cells {
		allowed[hexgrid.GridPosition{Q: cell.Q, R: cell.R}] = true
	}
	return allowed
}

// findRoute performs A* on the axial lattice. Power feeds are constrained to leave
// the bottom connector through one of the two north-facing hex directions.
func findRoute(
	start, goal hexgrid.GridPosition,
	allowed, blocked, reserved map[hexgrid.GridPosition]bool,
	leaveNorth, allowReserved bool,
	rng *rand.Rand,
) ([]hexgrid.GridPosition, bool) {
	if start == goal || !allowed[start] || !allowed[goal] {
		return nil, false
	}
	if blocked[start] == false || blocked[goal] == false {
		// Endpoints must be actual component cells. A missing endpoint is almost
		// always a placement/routing integration bug, so reject it explicitly.
		return nil, false
	}

	gScore := map[hexgrid.GridPosition]float64{start: 0}
	cameFrom := make(map[hexgrid.GridPosition]hexgrid.GridPosition)
	closed := make(map[hexgrid.GridPosition]bool)
	frontier := &routePriorityQueue{}
	heap.Init(frontier)
	heap.Push(frontier, &routeQueueItem{position: start, priority: float64(hexDistance(start, goal))})

	for frontier.Len() > 0 {
		current := heap.Pop(frontier).(*routeQueueItem).position
		if closed[current] {
			continue
		}
		if current == goal {
			return reconstructPath(start, goal, cameFrom), true
		}
		closed[current] = true

		directions := append([]hexgrid.GridPosition(nil), axialDirections[:]...)
		rng.Shuffle(len(directions), func(i, j int) { directions[i], directions[j] = directions[j], directions[i] })
		for _, direction := range directions {
			if leaveNorth && current == start && !(direction.R == -1 && (direction.Q == 0 || direction.Q == 1)) {
				continue
			}
			neighbor := addPosition(current, direction)
			if leaveNorth && neighbor == goal {
				// A power feed must enter the cluster hub from one of its two
				// south-facing neighbors, preserving a clear point of entry.
				fromSouthwest := addPosition(goal, hexgrid.GridPosition{Q: 0, R: 1})
				fromSoutheast := addPosition(goal, hexgrid.GridPosition{Q: -1, R: 1})
				if current != fromSouthwest && current != fromSoutheast {
					continue
				}
			}
			if !allowed[neighbor] || closed[neighbor] {
				continue
			}
			if neighbor != goal && blocked[neighbor] {
				continue
			}
			if neighbor != goal && neighbor != start && reserved[neighbor] && !allowReserved {
				continue
			}

			stepCost := 1.0 + rng.Float64()*0.035
			if reserved[neighbor] {
				stepCost += 2.5
			}
			// A small clearance penalty nudges routes away from existing traces without
			// making a short detour preferable to an unnecessarily long path.
			for _, nearby := range axialDirections {
				if reserved[addPosition(neighbor, nearby)] {
					stepCost += 0.08
				}
			}
			candidateScore := gScore[current] + stepCost
			previous, exists := gScore[neighbor]
			if exists && candidateScore >= previous {
				continue
			}
			cameFrom[neighbor] = current
			gScore[neighbor] = candidateScore
			priority := candidateScore + float64(hexDistance(neighbor, goal))
			heap.Push(frontier, &routeQueueItem{position: neighbor, priority: priority})
		}
	}
	return nil, false
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
		path, ok := findRoute(candidate.a, candidate.b, allowed, blocked, reserved, false, false, rng)
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
