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

type powerRouteAssignment struct {
	clusterIndex int
	sourceIndex int
	source powerSource
	target hexgrid.GridPosition
}

// powerSourcePositions returns centered top and bottom rows of potential power
// contacts. A zero count disables that row.
func powerSourcePositions(o Options) (top, bottom []hexgrid.GridPosition, err error) {
	if o.TopPowerSourceCount == 0 && o.BottomPowerSourceCount == 0 {
		return nil, nil, nil
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
	top, err = selectPowerSourceRow(rows, o.TopPowerSourceCount, true, o.PowerSourceSpacing)
	if err != nil {
		return nil, nil, err
	}
	bottom, err = selectPowerSourceRow(rows, o.BottomPowerSourceCount, false, o.PowerSourceSpacing)
	if err != nil {
		return nil, nil, err
	}
	if len(top) > 0 && len(bottom) > 0 && top[0].R == bottom[0].R {
		return nil, nil, fmt.Errorf("canvas is too short to fit distinct top and bottom power-source rows")
	}
	return top, bottom, nil
}

func selectPowerSourceRow(rows map[int][]hexgrid.Cell, count int, top bool, spacing PowerSourceSpacing) ([]hexgrid.GridPosition, error) {
	if count == 0 {
		return nil, nil
	}
	step := 1
	if spacing == PowerSourceSpacingSeparated {
		step = 2
	}
	requiredSpan := 1 + (count-1)*step
	rowName := "bottom"
	if top {
		rowName = "top"
	}

	type rowCandidate struct {
		r     int
		cells []hexgrid.Cell
		y     float64
	}
	candidates := make([]rowCandidate, 0, len(rows))
	widestRow := 0
	for r, row := range rows {
		if len(row) > widestRow {
			widestRow = len(row)
		}
		if len(row) == 0 {
			continue
		}
		candidates = append(candidates, rowCandidate{
			r:     r,
			cells: append([]hexgrid.Cell(nil), row...),
			y:     row[0].Center.Y,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].y != candidates[j].y {
			if top {
				return candidates[i].y < candidates[j].y
			}
			return candidates[i].y > candidates[j].y
		}
		if top {
			return candidates[i].r < candidates[j].r
		}
		return candidates[i].r > candidates[j].r
	})

	// Keep the existing capacity rule: start at the outermost row that can
	// accommodate the requested contacts and their configured spacing.
	outermost := -1
	for i, candidate := range candidates {
		if len(candidate.cells) >= requiredSpan {
			outermost = i
			break
		}
	}
	if outermost < 0 {
		return nil, fmt.Errorf("%s power-source row does not fit: need %d contacts spanning %d grid cells at %q spacing, but the widest available grid row has %d cells",
			rowName, count, requiredSpan, spacing, widestRow)
	}

	selected := candidates[outermost]
	if !powerSourceRowIsSymmetric(len(selected.cells), requiredSpan) {
		// An even-length row has its center between cells. If the centered
		// fixed-spacing selection is asymmetric, move exactly one row inward.
		inward := outermost + 1
		if inward >= len(candidates) {
			return nil, fmt.Errorf("%s power-source row cannot be made horizontally symmetrical: no row one step inward", rowName)
		}
		selected = candidates[inward]
		if len(selected.cells) < requiredSpan {
			return nil, fmt.Errorf("%s power-source row cannot be made horizontally symmetrical: the next inward row has %d cells, need %d",
				rowName, len(selected.cells), requiredSpan)
		}
		if !powerSourceRowIsSymmetric(len(selected.cells), requiredSpan) {
			return nil, fmt.Errorf("%s power-source row cannot be made horizontally symmetrical: the next inward row has %d cells, which cannot center %d contacts at %q spacing",
				rowName, len(selected.cells), count, spacing)
		}
	}

	sort.Slice(selected.cells, func(i, j int) bool {
		return selected.cells[i].Center.X < selected.cells[j].Center.X
	})
	start := (len(selected.cells) - requiredSpan) / 2
	positions := make([]hexgrid.GridPosition, count)
	for i := range positions {
		cell := selected.cells[start+i*step]
		positions[i] = hexgrid.GridPosition{Q: cell.Q, R: cell.R}
	}
	return positions, nil
}

// powerSourceRowIsSymmetric reports whether a centered, fixed-span selection can
// be reflected exactly across the horizontal centerline of a row.
func powerSourceRowIsSymmetric(rowLength, requiredSpan int) bool {
	return rowLength >= requiredSpan && (rowLength-requiredSpan)%2 == 0
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

	// Source rows are reserved exclusively for power contacts. The vertical
	// clearance keeps components away from the row; this explicit check prevents
	// regressions if the clearance changes later.
	sourceRows := powerSourceRowIDs(topSources, bottomSources)
	var trapezoid *powerSourceTrapezoid
	if o.ConstrainLayoutToPowerSourceTrapezoid {
		bounds, err := newPowerSourceTrapezoid(topSources, bottomSources, o)
		if err != nil {
			return nil, nil, err
		}
		trapezoid = &bounds
	}
	candidates := make([]hexgrid.GridPosition, 0, len(allCells))
	for _, cell := range allCells {
		if sourceRows[cell.R] {
			continue
		}
		if len(topSources) > 0 && cell.Center.Y < topY+1.6*o.Pitch {
			continue
		}
		if len(bottomSources) > 0 && cell.Center.Y > bottomY-1.6*o.Pitch {
			continue
		}
		position := hexgrid.GridPosition{Q: cell.Q, R: cell.R}
		if trapezoid != nil && !trapezoid.contains(position, o) {
			continue
		}
		candidates = append(candidates, position)
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
	// Ordinary traces may never enter or leave the vertical corridor bounded by
	// the source rows. Power feeds get a copy with their source contact re-enabled
	// as the start, then leave that row on their first step.
	baseAllowedRoutes := routingCellsWithinPowerSourceCorridor(
		routingCellsWithoutPowerSourceRows(o, topSources, bottomSources), topSources, bottomSources, o,
	)
	constrainedCandidatesFailed := false

	// Retry different source, center, and placement combinations when a set of
	// non-crossing feeds or compact clusters cannot be constructed.
	for attempt := 0; attempt < 192; attempt++ {
		sourceCounts := allocatePowerSourceCounts(
			o.ClusterCount, o.MinPowerSourcesPerCluster, o.MaxPowerSourcesPerCluster,
			len(topSources)+len(bottomSources), rng,
		)
		totalSources := 0
		for _, count := range sourceCounts {
			totalSources += count
		}
		sources, err := choosePowerSourceAssignments(topSources, bottomSources, totalSources, rng)
		if err != nil {
			return nil, nil, err
		}

		spanSources := powerSourceSpanAssignments(sources, toPowerSources(topSources, powerSideTop), toPowerSources(bottomSources, powerSideBottom))
		attemptCandidates := candidates
		attemptAllowedRoutes := baseAllowedRoutes
		if o.ConstrainComponentsToPowerSourceSpan {
			attemptCandidates = candidatesWithinPowerSourceSpan(attemptCandidates, spanSources, o)
			attemptAllowedRoutes = routingCellsWithinPowerSourceSpan(attemptAllowedRoutes, spanSources, o)
			if len(attemptCandidates) < o.ComponentCount {
				constrainedCandidatesFailed = true
				continue
			}
		}
		if trapezoid != nil {
			attemptCandidates = candidatesWithinPowerSourceTrapezoid(attemptCandidates, *trapezoid, o)
			attemptAllowedRoutes = routingCellsWithinPowerSourceTrapezoid(attemptAllowedRoutes, *trapezoid, o)
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

		// Assign the selected contacts to clusters in left-to-right order. Each
		// cluster receives a contiguous group of zero or more source contacts.
		sort.Slice(sources, func(i, j int) bool {
			a := hexgrid.GridPositionToPoint(sources[i].Position, o.Width, o.Height, o.Pitch, "pointy")
			b := hexgrid.GridPositionToPoint(sources[j].Position, o.Width, o.Height, o.Pitch, "pointy")
			if a.X != b.X {
				return a.X < b.X
			}
			return sources[i].Side < sources[j].Side
		})
		sourceOffset := 0
		for i := range centerClusters {
			end := sourceOffset + sourceCounts[i]
			centerClusters[i].PowerSources = append([]powerSource(nil), sources[sourceOffset:end]...)
			centerClusters[i].PowerPaths = make([][]hexgrid.GridPosition, sourceCounts[i])
			sourceOffset = end
		}

		// Place complete clusters before routing power feeds. This makes every
		// component in a cluster available as a possible power-feed destination.
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
			cluster, err := growCluster(
				clusterID, center, sizes[clusterID], eligible, occupied,
				map[hexgrid.GridPosition]bool{}, rng,
			)
			if err != nil {
				failed = true
				break
			}
			cluster.PowerSources = append([]powerSource(nil), centerClusters[clusterID].PowerSources...)
			cluster.PowerTargets = choosePowerTargets(cluster.Components, len(cluster.PowerSources), rng)
			cluster.PowerPaths = make([][]hexgrid.GridPosition, len(cluster.PowerSources))
			clusters[clusterID] = cluster
			placements = append(placements, cluster.Components...)
			for _, component := range cluster.Components {
				occupied[component.Position] = clusterID
			}
		}
		if failed || len(placements) != o.ComponentCount {
			continue
		}

		// Power routes may pass through empty cells, but never through another
		// component or source contact. Their destination component is the only
		// blocked cell they are allowed to enter.
		blockedForPower := make(map[hexgrid.GridPosition]bool, len(topSources)+len(bottomSources)+len(placements))
		for _, source := range topSources {
			blockedForPower[source] = true
		}
		for _, source := range bottomSources {
			blockedForPower[source] = true
		}
		for _, cluster := range clusters {
			for _, component := range cluster.Components {
				blockedForPower[component.Position] = true
			}
		}

		// Route the longest feeds first. Feeds to different components in one
		// cluster share neither route cells nor endpoints unless the feed count
		// exceeds the number of components in that cluster.
		var routeOrder []powerRouteAssignment
		for clusterIndex, cluster := range clusters {
			for sourceIndex, source := range cluster.PowerSources {
				routeOrder = append(routeOrder, powerRouteAssignment{
					clusterIndex: clusterIndex,
					sourceIndex:  sourceIndex,
					source:       source,
					target:       cluster.PowerTargets[sourceIndex],
				})
			}
		}
		sort.SliceStable(routeOrder, func(i, j int) bool {
			a := routeOrder[i]
			b := routeOrder[j]
			return hexDistance(a.source.Position, a.target) > hexDistance(b.source.Position, b.target)
		})
		powerAllowedRoutes := copyAllowedRoutes(attemptAllowedRoutes)
		for _, source := range sources {
			powerAllowedRoutes[source.Position] = true
		}
		if !routePowerFeeds(o.RouteAttempts, rng, routeOrder, clusters, powerAllowedRoutes, blockedForPower) {
			continue
		}

		for clusterID := range clusters {
			cluster := &clusters[clusterID]
			if len(cluster.PowerSources) > 0 {
				cluster.PowerSource = cluster.PowerSources[0].Position
				cluster.PowerSide = cluster.PowerSources[0].Side
				cluster.PowerPath = append([]hexgrid.GridPosition(nil), cluster.PowerPaths[0]...)
			}
		}
		return placements, clusters, nil
	}
	if constrainedCandidatesFailed {
		return nil, nil, &retryableGenerationError{err: fmt.Errorf("could not place %d components within the horizontal span of the active power sources; increase the source-row width or disable ConstrainComponentsToPowerSourceSpan", o.ComponentCount)}
	}
	return nil, nil, &retryableGenerationError{err: fmt.Errorf("could not place %d components in %d separated clusters with non-crossing power feeds; reduce component density or cluster size, or increase the canvas dimensions", o.ComponentCount, o.ClusterCount)}
}

// routePowerFeeds retries the complete feed set with fresh reservations. Changing
// an earlier feed's path can leave a valid corridor for a later feed.
func routePowerFeeds(attempts int, rng *rand.Rand, order []powerRouteAssignment, clusters []clusterPlan, allowed, blocked map[hexgrid.GridPosition]bool) bool {
	for attempt := 0; attempt < attempts; attempt++ {
		reserved := make(map[hexgrid.GridPosition]bool)
		paths := make([][]hexgrid.GridPosition, len(order))
		failed := false
		for i, assignment := range order {
			path, ok := findRoute(assignment.source.Position, assignment.target, allowed, blocked, reserved, assignment.source.Side, rng)
			if !ok || len(path) < 2 { failed = true; break }
			paths[i] = path
			for _, position := range path { reserved[position] = true }
		}
		if failed { continue }
		for i, assignment := range order { clusters[assignment.clusterIndex].PowerPaths[assignment.sourceIndex] = paths[i] }
		return true
	}
	return false
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


type powerSourceTrapezoid struct {
	topY, bottomY           float64
	topLeft, topRight       float64
	bottomLeft, bottomRight float64
}

func newPowerSourceTrapezoid(topSources, bottomSources []hexgrid.GridPosition, o Options) (powerSourceTrapezoid, error) {
	if len(topSources) == 0 || len(bottomSources) == 0 {
		return powerSourceTrapezoid{}, fmt.Errorf("power-source trapezoid requires both top and bottom source rows")
	}
	topLeft := hexgrid.GridPositionToPoint(topSources[0], o.Width, o.Height, o.Pitch, "pointy")
	topRight := hexgrid.GridPositionToPoint(topSources[len(topSources)-1], o.Width, o.Height, o.Pitch, "pointy")
	bottomLeft := hexgrid.GridPositionToPoint(bottomSources[0], o.Width, o.Height, o.Pitch, "pointy")
	bottomRight := hexgrid.GridPositionToPoint(bottomSources[len(bottomSources)-1], o.Width, o.Height, o.Pitch, "pointy")
	if bottomLeft.Y <= topLeft.Y {
		return powerSourceTrapezoid{}, fmt.Errorf("cannot construct power-source trapezoid: bottom row must be below the top row")
	}
	return powerSourceTrapezoid{
		topY: topLeft.Y, bottomY: bottomLeft.Y,
		topLeft: topLeft.X, topRight: topRight.X,
		bottomLeft: bottomLeft.X, bottomRight: bottomRight.X,
	}, nil
}

func (b powerSourceTrapezoid) contains(position hexgrid.GridPosition, o Options) bool {
	point := hexgrid.GridPositionToPoint(position, o.Width, o.Height, o.Pitch, "pointy")
	const epsilon = 1e-9
	if point.Y < b.topY-epsilon || point.Y > b.bottomY+epsilon {
		return false
	}
	t := (point.Y - b.topY) / (b.bottomY - b.topY)
	left := b.topLeft + (b.bottomLeft-b.topLeft)*t
	right := b.topRight + (b.bottomRight-b.topRight)*t
	return point.X >= left-epsilon && point.X <= right+epsilon
}

func candidatesWithinPowerSourceTrapezoid(
	candidates []hexgrid.GridPosition,
	bounds powerSourceTrapezoid,
	o Options,
) []hexgrid.GridPosition {
	filtered := make([]hexgrid.GridPosition, 0, len(candidates))
	for _, candidate := range candidates {
		if bounds.contains(candidate, o) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func choosePowerSourceAssignments(
	topSources, bottomSources []hexgrid.GridPosition,
	sourceCount int,
	rng *rand.Rand,
) ([]powerSource, error) {
	topCapacity := len(topSources)
	bottomCapacity := len(bottomSources)
	minimumTop := max(0, sourceCount-bottomCapacity)
	maximumTop := min(sourceCount, topCapacity)
	// Prefer to use both edges when multiple active contacts are selected.
	if sourceCount > 1 && len(topSources) > 0 && len(bottomSources) > 0 {
		minimumTop = max(minimumTop, 1)
		maximumTop = min(maximumTop, sourceCount-1)
	}
	if minimumTop > maximumTop {
		return nil, fmt.Errorf("cannot assign %d active power sources to the available top and bottom contacts", sourceCount)
	}
	topCount := minimumTop
	if maximumTop > minimumTop {
		topCount += rng.Intn(maximumTop - minimumTop + 1)
	}
	bottomCount := sourceCount - topCount

	topIndices := choosePowerSourceIndices(len(topSources), topCount, PowerSourceSpacingAdjacent, rng)
	bottomIndices := choosePowerSourceIndices(len(bottomSources), bottomCount, PowerSourceSpacingAdjacent, rng)
	if len(topIndices) != topCount || len(bottomIndices) != bottomCount {
		return nil, fmt.Errorf("could not choose %d top and %d bottom power contacts", topCount, bottomCount)
	}

	selected := make([]powerSource, 0, sourceCount)
	for _, index := range topIndices {
		selected = append(selected, powerSource{Position: topSources[index], Side: powerSideTop})
	}
	for _, index := range bottomIndices {
		selected = append(selected, powerSource{Position: bottomSources[index], Side: powerSideBottom})
	}
	return selected, nil
}

// allocatePowerSourceCounts chooses a per-cluster feed count within the configured
// range, reducing randomly selected extras when the physical contact pool is smaller
// than the initial choices. The minimum is preserved for every cluster.
func allocatePowerSourceCounts(clusterCount, minimum, maximum, available int, rng *rand.Rand) []int {
	counts := make([]int, clusterCount)
	total := 0
	for i := range counts {
		counts[i] = minimum
		if maximum > minimum {
			counts[i] += rng.Intn(maximum-minimum+1)
		}
		total += counts[i]
	}
	for total > available {
		var eligible []int
		for i, count := range counts {
			if count > minimum {
				eligible = append(eligible, i)
			}
		}
		if len(eligible) == 0 {
			break
		}
		index := eligible[rng.Intn(len(eligible))]
		counts[index]--
		total--
	}
	return counts
}

// powerSourceSpanAssignments falls back to the configured physical row contacts
// when no contacts are active, so the optional horizontal constraint remains useful
// for layouts in which every cluster is intentionally unpowered.
func powerSourceSpanAssignments(active, top, bottom []powerSource) []powerSource {
	if len(active) > 0 {
		return active
	}
	return append(append([]powerSource(nil), top...), bottom...)
}

func toPowerSources(positions []hexgrid.GridPosition, side powerSide) []powerSource {
	sources := make([]powerSource, len(positions))
	for i, position := range positions {
		sources[i] = powerSource{Position: position, Side: side}
	}
	return sources
}

// choosePowerTargets assigns each feed to a component in its cluster. It uses
// every component once before reusing a destination when there are more feeds
// than components, then repeats that order as necessary.
func choosePowerTargets(components []placedComponent, count int, rng *rand.Rand) []hexgrid.GridPosition {
	if count <= 0 || len(components) == 0 {
		return nil
	}
	positions := make([]hexgrid.GridPosition, len(components))
	for i, component := range components {
		positions[i] = component.Position
	}
	rng.Shuffle(len(positions), func(i, j int) {
		positions[i], positions[j] = positions[j], positions[i]
	})
	targets := make([]hexgrid.GridPosition, count)
	for i := range targets {
		targets[i] = positions[i%len(positions)]
	}
	return targets
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
