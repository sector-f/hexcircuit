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

type powerSide uint8

const (
	powerSideAny powerSide = iota
	powerSideTop
	powerSideBottom
)

type powerSource struct {
	Position hexgrid.GridPosition
	Side     powerSide
}

// powerSourcePositions returns centered top and bottom rows of potential power
// contacts. A zero count disables that row.
func powerSourcePositions(o Options) (top, bottom []hexgrid.GridPosition, err error) {
	if o.TopPowerSourceCount == 0 && o.BottomPowerSourceCount == 0 {
		return nil, nil, fmt.Errorf("at least one top or bottom power-source contact is required")
	}
	margin := math.Max(o.ComponentRadius+o.ComponentStrokeWidth/2, o.ConnectorInset)
	cells := hexgrid.GridCells(o.Width, o.Height, o.Pitch, "pointy", margin)
	if len(cells) == 0 {
		return nil, nil, fmt.Errorf("canvas does not contain any power-source grid positions")
	}

	rows := make(map[int][]hexgrid.Cell)
	for _, cell := range cells {
		rows[cell.R] = append(rows[cell.R], cell)
	}
	top, err = selectPowerSourceRow(rows, o.TopPowerSourceCount, true)
	if err != nil {
		return nil, nil, err
	}
	bottom, err = selectPowerSourceRow(rows, o.BottomPowerSourceCount, false)
	if err != nil {
		return nil, nil, err
	}
	if len(top) > 0 && len(bottom) > 0 && top[0].R == bottom[0].R {
		return nil, nil, fmt.Errorf("canvas is too short to fit distinct top and bottom power-source rows")
	}
	return top, bottom, nil
}

func selectPowerSourceRow(rows map[int][]hexgrid.Cell, count int, top bool) ([]hexgrid.GridPosition, error) {
	if count == 0 {
		return nil, nil
	}
	var best []hexgrid.Cell
	bestY := math.Inf(1)
	if !top {
		bestY = math.Inf(-1)
	}
	for _, row := range rows {
		if len(row) < count {
			continue
		}
		y := row[0].Center.Y
		if (top && y < bestY) || (!top && y > bestY) {
			bestY = y
			best = row
		}
	}
	rowName := "bottom"
	if top {
		rowName = "top"
	}
	if len(best) < count {
		return nil, fmt.Errorf("%s power-source row does not fit: need %d contacts, but the available grid row has %d", rowName, count, len(best))
	}
	sort.Slice(best, func(i, j int) bool { return best[i].Center.X < best[j].Center.X })
	start := (len(best) - count) / 2
	positions := make([]hexgrid.GridPosition, count)
	for i := range positions {
		positions[i] = hexgrid.GridPosition{Q: best[start+i].Q, R: best[start+i].R}
	}
	return positions, nil
}

func placeClusters(
	o Options,
	rng *rand.Rand,
	topSources, bottomSources []hexgrid.GridPosition,
) ([]placedComponent, []clusterPlan, error) {
	allCells := hexgrid.GridCells(
		o.Width,
		o.Height,
		o.Pitch,
		"pointy",
		o.ComponentRadius+o.ComponentStrokeWidth/2,
	)
	var topY, bottomY float64
	if len(topSources) > 0 {
		topY = hexgrid.GridPositionToPoint(topSources[0], o.Width, o.Height, o.Pitch, "pointy").Y
	}
	if len(bottomSources) > 0 {
		bottomY = hexgrid.GridPositionToPoint(bottomSources[0], o.Width, o.Height, o.Pitch, "pointy").Y
	}

	// These candidates are bounded vertically by the power rows. When the
	// optional horizontal constraint is enabled, a per-attempt filter narrows
	// them to the span of the active (selected) power sources.
	candidates := make([]hexgrid.GridPosition, 0, len(allCells))
	for _, cell := range allCells {
		if len(topSources) > 0 && cell.Center.Y < topY+1.6*o.Pitch {
			continue
		}
		if len(bottomSources) > 0 && cell.Center.Y > bottomY-1.6*o.Pitch {
			continue
		}
		candidates = append(candidates, hexgrid.GridPosition{Q: cell.Q, R: cell.R})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Q != candidates[j].Q {
			return candidates[i].Q < candidates[j].Q
		}
		return candidates[i].R < candidates[j].R
	})
	if len(candidates) < o.ComponentCount {
		return nil, nil, fmt.Errorf("not enough component cells between the top and bottom power corridors: need %d, have %d", o.ComponentCount, len(candidates))
	}

	sizes := allocateClusterSizes(o.ComponentCount, o.ClusterCount, o.MinComponentsPerCluster, rng)
	order := make([]int, len(sizes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return sizes[order[i]] > sizes[order[j]] })
	allowedRoutes := routingCells(o)
	constrainedCandidatesFailed := false

	// Retry different source, center, and placement combinations when a set of
	// non-crossing feeds or compact clusters cannot be constructed.
	for attempt := 0; attempt < 192; attempt++ {
		sources, err := choosePowerSourceAssignments(topSources, bottomSources, o.ClusterCount, o.PowerSourceSpacing, rng)
		if err != nil {
			return nil, nil, err
		}

		attemptCandidates := candidates
		if o.ConstrainComponentsToPowerSourceSpan {
			attemptCandidates = candidatesWithinPowerSourceSpan(candidates, sources, o)
			if len(attemptCandidates) < o.ComponentCount {
				constrainedCandidatesFailed = true
				continue
			}
		}
		eligible := make(map[hexgrid.GridPosition]bool, len(attemptCandidates))
		for _, position := range attemptCandidates {
			eligible[position] = true
		}

		centers := selectClusterCenters(attemptCandidates, o.ClusterCount, rng)
		if len(centers) != o.ClusterCount {
			continue
		}
		centerClusters := make([]clusterPlan, len(centers))
		for i, center := range centers {
			centerClusters[i] = clusterPlan{ID: i, Hub: center}
		}
		sortClustersByX(centerClusters, o)

		// Match selected contacts to cluster hubs from left to right. The active
		// contacts themselves are still randomized, including their choice of row.
		sort.Slice(sources, func(i, j int) bool {
			a := hexgrid.GridPositionToPoint(sources[i].Position, o.Width, o.Height, o.Pitch, "pointy")
			b := hexgrid.GridPositionToPoint(sources[j].Position, o.Width, o.Height, o.Pitch, "pointy")
			if a.X != b.X {
				return a.X < b.X
			}
			return sources[i].Side < sources[j].Side
		})
		for i := range centerClusters {
			centerClusters[i].PowerSource = sources[i].Position
			centerClusters[i].PowerSide = sources[i].Side
		}

		blockedForPower := make(map[hexgrid.GridPosition]bool, len(topSources)+len(bottomSources)+len(centers))
		for _, source := range topSources {
			blockedForPower[source] = true
		}
		for _, source := range bottomSources {
			blockedForPower[source] = true
		}
		for _, cluster := range centerClusters {
			blockedForPower[cluster.Hub] = true
		}

		// Route the longest feeds first. On an axial hex lattice, two grid-axis
		// segments can intersect only at a lattice vertex; reserving every path
		// vertex prevents power-feed crossings as well as shared segments.
		routeOrder := make([]int, len(centerClusters))
		for i := range routeOrder {
			routeOrder[i] = i
		}
		sort.SliceStable(routeOrder, func(i, j int) bool {
			a := centerClusters[routeOrder[i]]
			b := centerClusters[routeOrder[j]]
			return hexDistance(a.PowerSource, a.Hub) > hexDistance(b.PowerSource, b.Hub)
		})
		powerCorridors := make(map[hexgrid.GridPosition]bool)
		routed := true
		for _, clusterID := range routeOrder {
			cluster := &centerClusters[clusterID]
			path, ok := findRoute(cluster.PowerSource, cluster.Hub, allowedRoutes, blockedForPower, powerCorridors, cluster.PowerSide, rng)
			if !ok || len(path) < 2 {
				routed = false
				break
			}
			cluster.PowerPath = path
			for _, position := range path {
				powerCorridors[position] = true
			}
		}
		if !routed {
			continue
		}

		reservedCells := make(map[hexgrid.GridPosition]bool, len(powerCorridors))
		for position := range powerCorridors {
			reservedCells[position] = true
		}

		occupied := make(map[hexgrid.GridPosition]int, o.ComponentCount)
		clusters := make([]clusterPlan, o.ClusterCount)
		placements := make([]placedComponent, 0, o.ComponentCount)
		failed := false
		for _, clusterID := range order {
			center := centerClusters[clusterID].Hub
			if _, exists := occupied[center]; exists || adjacentToOtherCluster(center, occupied, clusterID) {
				failed = true
				break
			}
			cluster, err := growCluster(clusterID, center, sizes[clusterID], eligible, occupied, reservedCells, rng)
			if err != nil {
				failed = true
				break
			}
			cluster.PowerSource = centerClusters[clusterID].PowerSource
			cluster.PowerSide = centerClusters[clusterID].PowerSide
			cluster.PowerPath = append([]hexgrid.GridPosition(nil), centerClusters[clusterID].PowerPath...)
			clusters[clusterID] = cluster
			placements = append(placements, cluster.Components...)
			for _, component := range cluster.Components {
				occupied[component.Position] = clusterID
			}
		}
		if !failed && len(placements) == o.ComponentCount {
			return placements, clusters, nil
		}
	}
	if constrainedCandidatesFailed {
		return nil, nil, fmt.Errorf("could not place %d components within the horizontal span of the active power sources; increase the source-row width or disable ConstrainComponentsToPowerSourceSpan", o.ComponentCount)
	}
	return nil, nil, fmt.Errorf("could not place %d components in %d separated clusters with non-crossing power feeds; reduce component density or cluster size, or increase the canvas dimensions", o.ComponentCount, o.ClusterCount)
}

// candidatesWithinPowerSourceSpan keeps ordinary component positions between
// the leftmost and rightmost selected power-source centers. Inactive potential
// contacts do not affect the span because they are not rendered as components.
func candidatesWithinPowerSourceSpan(
	candidates []hexgrid.GridPosition,
	sources []powerSource,
	o Options,
) []hexgrid.GridPosition {
	minimumX, maximumX := math.Inf(1), math.Inf(-1)
	for _, source := range sources {
		x := hexgrid.GridPositionToPoint(source.Position, o.Width, o.Height, o.Pitch, "pointy").X
		minimumX = math.Min(minimumX, x)
		maximumX = math.Max(maximumX, x)
	}
	if math.IsInf(minimumX, 0) || math.IsInf(maximumX, 0) {
		return nil
	}

	const epsilon = 1e-9
	filtered := make([]hexgrid.GridPosition, 0, len(candidates))
	for _, candidate := range candidates {
		x := hexgrid.GridPositionToPoint(candidate, o.Width, o.Height, o.Pitch, "pointy").X
		if x >= minimumX-epsilon && x <= maximumX+epsilon {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func choosePowerSourceAssignments(
	topSources, bottomSources []hexgrid.GridPosition,
	clusterCount int,
	spacing PowerSourceSpacing,
	rng *rand.Rand,
) ([]powerSource, error) {
	topCapacity := len(topSources)
	bottomCapacity := len(bottomSources)
	if spacing == PowerSourceSpacingSeparated {
		topCapacity = maximumSeparatedSources(topCapacity)
		bottomCapacity = maximumSeparatedSources(bottomCapacity)
	}
	minimumTop := max(0, clusterCount-bottomCapacity)
	maximumTop := min(clusterCount, topCapacity)
	// Prefer to use both edges when there are enough clusters and both rows exist.
	if clusterCount > 1 && len(topSources) > 0 && len(bottomSources) > 0 {
		minimumTop = max(minimumTop, 1)
		maximumTop = min(maximumTop, clusterCount-1)
	}
	if minimumTop > maximumTop {
		return nil, fmt.Errorf("cannot assign %d clusters to the available top and bottom power sources using %q spacing", clusterCount, spacing)
	}
	topCount := minimumTop
	if maximumTop > minimumTop {
		topCount += rng.Intn(maximumTop - minimumTop + 1)
	}
	bottomCount := clusterCount - topCount

	topIndices := choosePowerSourceIndices(len(topSources), topCount, spacing, rng)
	bottomIndices := choosePowerSourceIndices(len(bottomSources), bottomCount, spacing, rng)
	if len(topIndices) != topCount || len(bottomIndices) != bottomCount {
		return nil, fmt.Errorf("could not choose %d top and %d bottom power contacts with %q spacing", topCount, bottomCount, spacing)
	}

	selected := make([]powerSource, 0, clusterCount)
	for _, index := range topIndices {
		selected = append(selected, powerSource{Position: topSources[index], Side: powerSideTop})
	}
	for _, index := range bottomIndices {
		selected = append(selected, powerSource{Position: bottomSources[index], Side: powerSideBottom})
	}
	return selected, nil
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
	// for the one-cell separation between clusters and reserved power feeds.
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
		cluster.Components = append(cluster.Components, placedComponent{
			Position:    candidate,
			Style:       randomComponentStyle(rng),
			RadiusScale: 0.72 + rng.Float64()*0.22,
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

func powerSourceComponentStyle(style PowerSourceStyle) hexgrid.ComponentStyle {
	if style == PowerSourceStyleDouble {
		return hexgrid.ComponentStyleDouble
	}
	return hexgrid.ComponentStyleSingle
}

// choosePowerSourceIndices selects sorted, distinct contacts. Separated mode
// leaves at least one unused contact between selected positions; adjacent mode
// permits consecutive contacts.
func choosePowerSourceIndices(contactCount, sourceCount int, spacing PowerSourceSpacing, rng *rand.Rand) []int {
	if sourceCount < 0 || sourceCount == 0 || contactCount < sourceCount {
		if sourceCount == 0 {
			return []int{}
		}
		return nil
	}
	minimumGap := 1
	if spacing == PowerSourceSpacingSeparated {
		minimumGap = 2
	}
	chosen := make([]int, 0, sourceCount)
	var search func(start, remaining int) bool
	search = func(start, remaining int) bool {
		if remaining == 0 {
			return true
		}
		lastPossible := contactCount - 1 - (remaining-1)*minimumGap
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
