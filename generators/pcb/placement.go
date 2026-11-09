package pcb

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/sector-f/hexgrid"
)

var axialDirections = [...]hexgrid.GridPosition{
	{Q: 1, R: 0},
	{Q: 0, R: 1},
	{Q: -1, R: 1},
	{Q: -1, R: 0},
	{Q: 0, R: -1},
	{Q: 1, R: -1},
}

func connectorPositions(o Options) ([]hexgrid.GridPosition, error) {
	margin := math.Max(o.ComponentRadius+o.ComponentStrokeWidth/2, o.ConnectorInset)
	cells := hexgrid.GridCells(o.Width, o.Height, o.Pitch, "pointy", margin)
	rows := make(map[int][]hexgrid.Cell)
	for _, cell := range cells {
		rows[cell.R] = append(rows[cell.R], cell)
	}

	var best []hexgrid.Cell
	bestY := math.Inf(-1)
	for _, row := range rows {
		if len(row) < o.ConnectorCount {
			continue
		}
		y := row[0].Center.Y
		if y > bestY {
			bestY = y
			best = row
		}
	}
	if len(best) < o.ConnectorCount {
		return nil, fmt.Errorf("bottom connector row does not fit: need %d contacts, but the available grid row has %d", o.ConnectorCount, len(best))
	}
	sort.Slice(best, func(i, j int) bool { return best[i].Center.X < best[j].Center.X })
	start := (len(best) - o.ConnectorCount) / 2
	positions := make([]hexgrid.GridPosition, o.ConnectorCount)
	for i := range positions {
		positions[i] = hexgrid.GridPosition{Q: best[start+i].Q, R: best[start+i].R}
	}
	return positions, nil
}

func placeClusters(
	o Options,
	rng *rand.Rand,
	connectors []hexgrid.GridPosition,
) ([]placedComponent, []clusterPlan, map[int]hexgrid.GridPosition, map[int][]hexgrid.GridPosition, error) {
	allCells := hexgrid.GridCells(
		o.Width,
		o.Height,
		o.Pitch,
		"pointy",
		o.ComponentRadius+o.ComponentStrokeWidth/2,
	)
	connectorY := hexgrid.GridPositionToPoint(connectors[0], o.Width, o.Height, o.Pitch, "pointy").Y
	maxComponentY := connectorY - 1.6*o.Pitch

	eligible := make(map[hexgrid.GridPosition]bool, len(allCells))
	candidates := make([]hexgrid.GridPosition, 0, len(allCells))
	for _, cell := range allCells {
		if cell.Center.Y > maxComponentY {
			continue
		}
		position := hexgrid.GridPosition{Q: cell.Q, R: cell.R}
		eligible[position] = true
		candidates = append(candidates, position)
	}
	if len(candidates) < o.ComponentCount {
		return nil, nil, nil, nil, fmt.Errorf("not enough component cells above the power corridor: need %d, have %d", o.ComponentCount, len(candidates))
	}

	sizes := allocateClusterSizes(o.ComponentCount, o.ClusterCount, o.MinComponentsPerCluster, rng)
	order := make([]int, len(sizes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return sizes[order[i]] > sizes[order[j]] })
	allowedRoutes := routingCells(o)

	// Select centers, plan power feeds, then grow components around the reserved
	// corridors. This ordering guarantees that cluster population cannot occupy
	// the only usable route between a bottom contact and its destination hub.
	for attempt := 0; attempt < 128; attempt++ {
		centers := selectClusterCenters(candidates, o.ClusterCount, rng)
		if len(centers) != o.ClusterCount {
			continue
		}

		centerClusters := make([]clusterPlan, len(centers))
		for i, center := range centers {
			centerClusters[i] = clusterPlan{ID: i, Hub: center}
		}
		sortClustersByX(centerClusters, o)
		sourceIndices := choosePowerSourceIndices(len(connectors), len(centers), rng)
		if len(sourceIndices) != len(centers) {
			continue
		}

		blockedForPower := make(map[hexgrid.GridPosition]bool, len(connectors)+len(centers))
		for _, connector := range connectors {
			blockedForPower[connector] = true
		}
		for _, center := range centers {
			blockedForPower[center] = true
		}
		powerSources := make(map[int]hexgrid.GridPosition, len(centers))
		powerPaths := make(map[int][]hexgrid.GridPosition, len(centers))
		powerCorridors := make(map[hexgrid.GridPosition]bool)
		failed := false
		for i, cluster := range centerClusters {
			source := connectors[sourceIndices[i]]
			path, ok := findRoute(source, cluster.Hub, allowedRoutes, blockedForPower, powerCorridors, true, false, rng)
			if !ok {
				path, ok = findRoute(source, cluster.Hub, allowedRoutes, blockedForPower, powerCorridors, true, true, rng)
			}
			if !ok || len(path) < 2 {
				failed = true
				break
			}
			powerSources[cluster.ID] = source
			powerPaths[cluster.ID] = path
			for _, position := range path[1 : len(path)-1] {
				powerCorridors[position] = true
			}
		}
		if failed {
			continue
		}

		reservedCells := make(map[hexgrid.GridPosition]bool, len(powerCorridors))
		for position := range powerCorridors {
			reservedCells[position] = true
		}

		occupied := make(map[hexgrid.GridPosition]int, o.ComponentCount)
		clusters := make([]clusterPlan, o.ClusterCount)
		placements := make([]placedComponent, 0, o.ComponentCount)
		failed = false
		for _, clusterID := range order {
			center := centers[clusterID]
			if _, exists := occupied[center]; exists || adjacentToOtherCluster(center, occupied, clusterID) {
				failed = true
				break
			}
			cluster, err := growCluster(clusterID, center, sizes[clusterID], eligible, occupied, reservedCells, rng)
			if err != nil {
				failed = true
				break
			}
			clusters[clusterID] = cluster
			placements = append(placements, cluster.Components...)
			for _, component := range cluster.Components {
				occupied[component.Position] = clusterID
			}
		}
		if !failed && len(placements) == o.ComponentCount {
			return placements, clusters, powerSources, powerPaths, nil
		}
	}
	return nil, nil, nil, nil, fmt.Errorf("could not place %d components in %d separated clusters with clear power corridors; reduce component density or cluster size", o.ComponentCount, o.ClusterCount)
}

func allocateClusterSizes(componentCount, clusterCount, minimum int, rng *rand.Rand) []int {
	sizes := make([]int, clusterCount)
	for i := range sizes {
		sizes[i] = minimum
	}
	remaining := componentCount - clusterCount*minimum
	for i := 0; i < remaining; i++ {
		sizes[rng.Intn(clusterCount)]++
	}
	return sizes
}

func selectClusterCenters(candidates []hexgrid.GridPosition, count int, rng *rand.Rand) []hexgrid.GridPosition {
	if count <= 0 || len(candidates) == 0 {
		return nil
	}
	first := rng.Intn(len(candidates))
	centers := []hexgrid.GridPosition{candidates[first]}
	selected := map[hexgrid.GridPosition]bool{candidates[first]: true}
	for len(centers) < count {
		bestDistance := -1
		var best []hexgrid.GridPosition
		for _, candidate := range candidates {
			if selected[candidate] {
				continue
			}
			minimumDistance := int(^uint(0) >> 1)
			for _, center := range centers {
				distance := hexDistance(candidate, center)
				if distance < minimumDistance {
					minimumDistance = distance
				}
			}
			if minimumDistance > bestDistance {
				bestDistance = minimumDistance
				best = []hexgrid.GridPosition{candidate}
			} else if minimumDistance == bestDistance {
				best = append(best, candidate)
			}
		}
		if len(best) == 0 {
			break
		}
		choice := best[rng.Intn(len(best))]
		centers = append(centers, choice)
		selected[choice] = true
	}
	return centers
}

func growCluster(
	clusterID int,
	hub hexgrid.GridPosition,
	target int,
	eligible map[hexgrid.GridPosition]bool,
	occupied map[hexgrid.GridPosition]int,
	reservedCells map[hexgrid.GridPosition]bool,
	rng *rand.Rand,
) (clusterPlan, error) {
	cluster := clusterPlan{ID: clusterID, Hub: hub}
	cluster.Components = append(cluster.Components, placedComponent{
		Position:    hub,
		Style:       hexgrid.ComponentStyleTriple,
		RadiusScale: 1,
		ClusterID:   clusterID,
	})
	local := map[hexgrid.GridPosition]bool{hub: true}
	// A square-root bound keeps ordinary clusters compact while allowing room
	// for the one-cell separation between clusters and the reserved power feed.
	maxRadius := int(math.Ceil(math.Sqrt(float64(target))))
	if maxRadius < 2 {
		maxRadius = 2
	}

	for len(cluster.Components) < target {
		frontierSet := make(map[hexgrid.GridPosition]bool)
		for _, component := range cluster.Components {
			for _, direction := range axialDirections {
				candidate := addPosition(component.Position, direction)
				_, alreadyOccupied := occupied[candidate]
				if !eligible[candidate] || local[candidate] || alreadyOccupied || reservedCells[candidate] ||
					hexDistance(candidate, hub) > maxRadius {
					continue
				}
				if adjacentToOtherCluster(candidate, occupied, clusterID) {
					continue
				}
				frontierSet[candidate] = true
			}
		}
		frontier := make([]hexgrid.GridPosition, 0, len(frontierSet))
		for position := range frontierSet {
			frontier = append(frontier, position)
		}
		// Sort before shuffling so a fixed seed produces the same placement across
		// runs. Go deliberately randomizes map iteration order.
		sort.Slice(frontier, func(i, j int) bool {
			if frontier[i].Q != frontier[j].Q {
				return frontier[i].Q < frontier[j].Q
			}
			return frontier[i].R < frontier[j].R
		})
		if len(frontier) == 0 {
			return clusterPlan{}, fmt.Errorf("cluster %d ran out of adjacent cells", clusterID+1)
		}
		rng.Shuffle(len(frontier), func(i, j int) { frontier[i], frontier[j] = frontier[j], frontier[i] })
		candidate := frontier[0]

		parents := make([]hexgrid.GridPosition, 0, 3)
		for _, component := range cluster.Components {
			if hexDistance(component.Position, candidate) == 1 {
				parents = append(parents, component.Position)
			}
		}
		if len(parents) == 0 {
			return clusterPlan{}, fmt.Errorf("cluster %d selected a cell without a local parent", clusterID+1)
		}
		parent := parents[rng.Intn(len(parents))]
		style := randomComponentStyle(rng)
		radiusScale := 0.72 + rng.Float64()*0.22
		cluster.Components = append(cluster.Components, placedComponent{
			Position:    candidate,
			Style:       style,
			RadiusScale: radiusScale,
			Parent:      &parent,
			ClusterID:   clusterID,
		})
		local[candidate] = true
	}
	return cluster, nil
}

func adjacentToOtherCluster(candidate hexgrid.GridPosition, occupied map[hexgrid.GridPosition]int, clusterID int) bool {
	for _, direction := range axialDirections {
		neighbor := addPosition(candidate, direction)
		if owner, exists := occupied[neighbor]; exists && owner != clusterID {
			return true
		}
	}
	return false
}

func randomComponentStyle(rng *rand.Rand) hexgrid.ComponentStyle {
	roll := rng.Intn(100)
	switch {
	case roll < 46:
		return hexgrid.ComponentStyleSingle
	case roll < 82:
		return hexgrid.ComponentStyleDouble
	default:
		return hexgrid.ComponentStyleTriple
	}
}

// choosePowerSourceIndices returns sorted, distinct contact indices. When possible,
// it enforces at least one unused contact between selected power sources.
func choosePowerSourceIndices(connectorCount, sourceCount int, rng *rand.Rand) []int {
	if sourceCount <= 0 || connectorCount < sourceCount {
		return nil
	}
	minimumGap := 2
	if sourceCount > (connectorCount+1)/2 {
		minimumGap = 1
	}
	chosen := make([]int, 0, sourceCount)
	var search func(start, remaining int) bool
	search = func(start, remaining int) bool {
		if remaining == 0 {
			return true
		}
		lastPossible := connectorCount - 1 - (remaining-1)*minimumGap
		if start > lastPossible {
			return false
		}
		candidates := make([]int, 0, lastPossible-start+1)
		for index := start; index <= lastPossible; index++ {
			candidates = append(candidates, index)
		}
		rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		for _, index := range candidates {
			chosen = append(chosen, index)
			if search(index+minimumGap, remaining-1) {
				return true
			}
			chosen = chosen[:len(chosen)-1]
		}
		return false
	}
	if !search(0, sourceCount) {
		return nil
	}
	sort.Ints(chosen)
	return chosen
}

func addPosition(a, b hexgrid.GridPosition) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: a.Q + b.Q, R: a.R + b.R}
}

func hexDistance(a, b hexgrid.GridPosition) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	return max(absInt(dq), absInt(dr), absInt(dq+dr))
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
