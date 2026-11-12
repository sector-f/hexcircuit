package circuit

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/sector-f/hexgrid"
)

const (
	sqrt3               = 1.7320508075688772935
	epsilon             = 1e-9
	vectorEpsilon       = 1e-12
	candidatePitchFloor = 1e-9
	hexDirectionCount   = 6
	degreesPerHexStep   = 60.0
	radiansPerHexStep   = math.Pi / 3.0

	// The deterministic route-cost model charges for moves and turns only. The
	// legacy model adds random jitter to each move; both models randomize queue ties.
	routeStepCost           = 1.0
	routeTurnPenalty        = 0.18
	routeCostJitter         = 0.025 // Legacy mode adds this random range to each explored move's cost.
	routeQueueTieBreakRange = 0.001
	minLatticeMoves         = 3 // Three lattice moves are required for a valid connection.

	// legacyEarlyGoalMoves preserves the old search's stopping behavior for
	// compatibility: it stopped at a turned, two-move path, which router.buildConnection
	// then rejected. Legacy mode now returns no route in that case, with the same
	// effect on subsequent attempts and random-number consumption.
	legacyEarlyGoalMoves = 2

	// These margins keep route geometry away from the canvas boundary and other
	// traces by enough room for the rendered stroke width.
	routeCanvasInset             = 0.02
	routeCanvasSafetyMargin      = 0.002
	parallelTrackCollisionFactor = 0.8

	// Component placement and randomized detail selection are generation policy.
	componentsPerClusterAnchor  = 5
	maximumClusterCount         = 4
	minimumComponentSeparation  = 2
	minimumLocalParentDistance  = 2
	maximumLocalParentDistance  = 5
	minimumInterclusterDistance = 2
	maximumInterclusterDistance = 10
	minimumFallbackEdgeDistance = 2
	maximumFallbackEdgeDistance = 6

	// Component-detail selection is generation policy, not renderer behavior.
	componentDetailScale = 0.65
	hubDetailScale       = 1.5
	minimumDetailRadius  = 0.72

	hubRadiusMinimumScale = 0.88
	hubRadiusScaleRange   = 0.12
	hubRadiusThreshold    = 0.86
	singleTrackWeight     = 1.0
	doubleTrackWeight     = 4.0
	tripleTrackWeight     = 4.0

	smallComponentRadiusWeight       = 18.0
	mediumSmallComponentRadiusWeight = 28.0
	mediumComponentRadiusWeight      = 28.0
	mediumLargeComponentRadiusWeight = 18.0
	fullComponentRadiusWeight        = 8.0

	smallComponentRadiusScale       = 0.42
	mediumSmallComponentRadiusScale = 0.55
	mediumComponentRadiusScale      = 0.70
	mediumLargeComponentRadiusScale = 0.86
	fullComponentRadiusScale        = 1.0
)

// axialSteps lists the six legal moves for pointy-top axial coordinates. The order
// starts at -60 degrees and proceeds clockwise in 60-degree increments. Flat-top
// layouts use flatAxialSteps to keep those same direction indices aligned with the
// corresponding physical hexagon sides.
var axialSteps = [...]Coord{
	{Q: 1, R: -1}, {Q: 1, R: 0}, {Q: 0, R: 1},
	{Q: -1, R: 1}, {Q: -1, R: 0}, {Q: 0, R: -1},
}

var flatAxialSteps = [...]Coord{
	{Q: 1, R: 0}, {Q: 0, R: 1}, {Q: -1, R: 1},
	{Q: -1, R: 0}, {Q: 0, R: -1}, {Q: 1, R: -1},
}

// Point is a position in the SVG's internal coordinate system, measured in inches
// while geometry is generated and converted to SVG user units only when rendered.
type Point struct { X, Y float64 }

// Coord identifies a cell in the hexagonal lattice using axial (Q, R) coordinates.
// For pointy-top layouts, positive Q points east and positive R points southeast.
// Each coordinate pair maps to one point in the 2D drawing through coordToXY.
type Coord struct{ Q, R int }

func axialStepsForOrientation(orientation string) [hexDirectionCount]Coord {
	if orientation == "flat" {
		return flatAxialSteps
	}
	return axialSteps
}

// conventionalCoord converts the generator's former pointy-top basis to the
// conventional axial basis. The transform preserves the represented lattice cell.
// Flat-top coordinates already use the conventional basis and are unchanged.
func conventionalCoord(coord Coord, orientation string) Coord {
	if orientation == "flat" {
		return coord
	}
	return Coord{Q: coord.Q + coord.R, R: -coord.Q}
}

// HexComponent is the circuit router's local physical geometry for a component.
// It is deliberately independent of the low-level hexgrid/geometry package.
type HexComponent struct {
	Index  int
	X, Y   float64
	Radius float64
}

// Component is a hexagonal circuit component placed at one lattice coordinate.
// The embedded geometry is used by the router for collision checks; final rendering
// uses Q/R and RadiusScale to construct a hexgrid.GridComponent.
type Component struct {
	HexComponent
	Q, R        int
	Cluster     int
	Hub         bool
	RadiusScale float64
}

// Coord returns the component's lattice coordinate, independent of its pixel position.
func (c Component) Coord() Coord { return Coord{Q: c.Q, R: c.R} }

// AcceptedConnection stores one successful component-to-component route. Path and
// Lines are the high-level grid representation; Tracks and Endpoints are retained as
// router geometry for collision checks while later connections are being accepted.
type AcceptedConnection struct {
	Source, Target         int
	SourceSide, TargetSide int
	// Path contains the axial route from source component through destination.
	// Rendering converts it into a hexgrid.GridTrace; Tracks and Endpoints are
	// retained for collision checks while the router is accepting connections.
	Path      []Coord
	Lines     int
	Tracks    [][]Point
	Endpoints []Point
}

// selectedComponent is the placement-time form of a component. Parent records the
// earlier component this one was added near, which becomes a preferred network edge.
type selectedComponent struct {
	coord     Coord
	cluster   int
	parent    Coord
	hasParent bool
}

// gridCandidate pairs an axial cell with its drawing position. Keeping both values
// together avoids repeatedly converting coordinates during placement and rendering.
type gridCandidate struct {
	Coord Coord
	Point Point
}

// edgeCandidate ranks a possible pair of components to connect. Lower priority and
// shorter distance are considered first, subject to randomized tie ordering.
type edgeCandidate struct {
	a, b     int
	priority int
	distance int
}

// routeState includes heading and whether the path has turned, because both affect
// which continuations are valid and whether the destination is acceptable.
type routeState struct {
	Q, R    int
	LastDir int
	Turned  bool
}

// routeItem is a queued route state, ordered by estimated route cost and tie-breakers.
type routeItem struct {
	state    routeState
	cost     float64 // Accumulated path cost; legacy mode includes random jitter.
	priority float64 // Cost so far plus the axial-distance heuristic.
	tie      float64 // Randomized tie-breaker; it does not affect path cost.
	serial   int
}

// routeQueue implements heap.Interface so the route search can explore the most
// promising path state first.
type routeQueue []routeItem

func (q routeQueue) Len() int { return len(q) }

// Less orders states by estimated cost, then randomized tie-break, then insertion order.
func (q routeQueue) Less(i, j int) bool {
	if q[i].priority == q[j].priority {
		if q[i].tie == q[j].tie {
			return q[i].serial < q[j].serial
		}
		return q[i].tie < q[j].tie
	}
	return q[i].priority < q[j].priority
}

func (q routeQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *routeQueue) Push(x any) { *q = append(*q, x.(routeItem)) }

func (q *routeQueue) Pop() any {
	old := *q
	n := len(old)
	x := old[n-1]
	*q = old[:n-1]
	return x
}

// Generate creates an SVG document using opts. It returns the SVG text, the
// number of accepted trace bundles, the number of candidate connections attempted,
// and an error. It does not write files or print output, making it suitable for a
// future library package.
func Generate(options Options) (string, int, int, error) {
	// Validate the caller's configuration first. The returned copy may contain an
	// automatically increased RoutingPitch if the caller did not explicitly set it.
	opts, err := normalizeOptions(options)
	if err != nil {
		return "", 0, 0, err
	}

	// A fixed seed makes the same options reproducible. Otherwise choose a fresh master
	// seed once, then derive every fallback seed from it.
	masterSeed := opts.Seed
	if !opts.SeedSet {
		masterSeed = time.Now().UnixNano()
	}

	// Work out the angle conventions for the chosen orientation. All routing directions
	// then use the same six 60-degree increments as the hexagonal lattice.
	vertexStart := -90.0
	if opts.Orientation == "flat" {
		vertexStart = 0
	}
	baseAngle := degreesToRadians(vertexStart + 30.0)

	return generateWithSeedAttempts(opts.GenerationAttempts, masterSeed, func(seed int64) (string, int, int, error) {
		return generateAttempt(opts, seed, baseAngle)
	})
}

// generateWithSeedAttempts retries only failures explicitly marked as dependent
// on randomized placement. Attempt zero uses the master seed unchanged.
func generateWithSeedAttempts(
	attempts int,
	masterSeed int64,
	generate func(seed int64) (string, int, int, error),
) (string, int, int, error) {
	if attempts < 1 {
		return "", 0, 0, fmt.Errorf("generation attempts must be at least 1")
	}

	var lastErr error
	var lastSeed int64
	for attempt := 0; attempt < attempts; attempt++ {
		seed := generationAttemptSeed(masterSeed, attempt)
		svg, accepted, candidateAttempts, err := generate(seed)
		if err == nil {
			return svg, accepted, candidateAttempts, nil
		}
		var retryable *retryableGenerationError
		if !errors.As(err, &retryable) {
			return "", 0, 0, err
		}
		lastErr = err
		lastSeed = seed
	}

	return "", 0, 0, fmt.Errorf(
		"generation failed after %d seed attempts (last seed %d): %w",
		attempts, lastSeed, lastErr,
	)
}

// generationAttemptSeed preserves the master seed for attempt zero and uses a
// SplitMix64 step to derive well-distributed, reproducible fallback seeds.
func generationAttemptSeed(masterSeed int64, attempt int) int64 {
	if attempt <= 0 {
		return masterSeed
	}
	value := uint64(masterSeed) + uint64(attempt)*0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	value ^= value >> 31
	return int64(value)
}

type retryableGenerationError struct {
	err error
}

func (e *retryableGenerationError) Error() string { return e.err.Error() }
func (e *retryableGenerationError) Unwrap() error { return e.err }

// generateAttempt builds and renders one complete layout from a single RNG seed.
func generateAttempt(opts Options, seed int64, baseAngle float64) (string, int, int, error) {
	rng := rand.New(rand.NewSource(seed))
	components, baseEdges, err := chooseComponents(opts, rng)
	if err != nil {
		return "", 0, 0, err
	}

	rt := newRouter(opts, components, baseEdges, baseAngle, rng)
	accepted, attempts := rt.route()

	svgText, err := renderSVG(opts, components, accepted, seed)
	if err != nil {
		return "", 0, 0, err
	}
	return svgText, len(accepted), attempts, nil
}

// faceKey tracks which side of each component is already used by a connection.
type faceKey struct{ Component, Side int }

// router owns the mutable state needed to route connections without colliding with
// connections accepted earlier in the same generation run.
type router struct {
	options        Options
	components     []Component
	preferredEdges [][2]int
	baseAngle      float64
	axialSteps     [hexDirectionCount]Coord
	rng            *rand.Rand

	acceptedTracks [][]Point
	occupiedFaces  map[faceKey]bool
	endpointPoints []Point
}

func newRouter(options Options, components []Component, preferredEdges [][2]int, baseAngle float64, rng *rand.Rand) *router {
	return &router{
		options:        options,
		components:     components,
		preferredEdges: preferredEdges,
		baseAngle:      baseAngle,
		axialSteps:     axialStepsForOrientation(options.Orientation),
		rng:            rng,
		acceptedTracks: make([][]Point, 0),
		occupiedFaces:  make(map[faceKey]bool),
		endpointPoints: make([]Point, 0),
	}
}

// route generates candidate pairs and tries them in order. Attempts counts failed and
// successful pairs, stopping when the requested number of connections is accepted or candidates run out.
func (rt *router) route() ([]AcceptedConnection, int) {
	// Candidate generation uses the same RNG as placement and route search. Keep it
	// first so seeded output consumes random values in the same order as before.
	candidateEdges := rt.edgeCandidates()
	limit := rt.options.Connections
	accepted := make([]AcceptedConnection, 0, limit)
	attempts := 0
	for _, edge := range candidateEdges {
		if len(accepted) >= limit {
			break
		}
		attempts++
		connection := rt.tryEdge(edge[0], edge[1])
		if connection == nil {
			continue
		}
		accepted = append(accepted, *connection)
		rt.accept(connection)
	}
	return accepted, attempts
}

func (rt *router) accept(connection *AcceptedConnection) {
	rt.acceptedTracks = append(rt.acceptedTracks, connection.Tracks...)
	rt.endpointPoints = append(rt.endpointPoints, connection.Endpoints...)
	rt.occupiedFaces[faceKey{Component: connection.Source, Side: connection.SourceSide}] = true
	rt.occupiedFaces[faceKey{Component: connection.Target, Side: connection.TargetSide}] = true
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func degreesToRadians(degrees float64) float64 { return degrees * math.Pi / 180.0 }

// axialDistance is the axial hex-grid metric: max(|dq|, |dr|, |dq+dr|).
func axialDistance(a, b Coord) int {
	dq, dr := a.Q-b.Q, a.R-b.R
	return maxInt(absInt(dq), absInt(dr), absInt(dq+dr))
}

// isAxisAligned reports whether the cells lie on one of the six principal lattice axes.
func isAxisAligned(a, b Coord) bool {
	dq, dr := b.Q-a.Q, b.R-a.R
	return dq == 0 || dr == 0 || dq+dr == 0
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(values ...int) int {
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func add(a, b Point) Point { return Point{X: a.X + b.X, Y: a.Y + b.Y} }

func sub(a, b Point) Point { return Point{X: a.X - b.X, Y: a.Y - b.Y} }

func mul(a Point, scalar float64) Point { return Point{X: a.X * scalar, Y: a.Y * scalar} }

func dot(a, b Point) float64 { return a.X*b.X + a.Y*b.Y }

func cross(a, b Point) float64 { return a.X*b.Y - a.Y*b.X }

func length(v Point) float64 { return math.Hypot(v.X, v.Y) }

func distance(a, b Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

// unit returns a unit-length vector, or the zero vector for a degenerate input.
func unit(v Point) Point {
	norm := length(v)
	if norm < vectorEpsilon {
		return Point{}
	}
	return Point{X: v.X / norm, Y: v.Y / norm}
}

// directionAngle maps a lattice direction index to its angle in drawing coordinates.
func directionAngle(baseAngle float64, index int) float64 {
	return baseAngle + degreesToRadians(degreesPerHexStep*float64(index))
}

// faceMidpoint offsets the center along a side's outward normal by the apothem,
// radius*sqrt(3)/2 for a regular hexagon.
func faceMidpoint(component Component, side int, baseAngle float64) Point {
	apothem := component.Radius * sqrt3 / 2.0
	theta := directionAngle(baseAngle, side)
	return Point{X: component.X + apothem*math.Cos(theta), Y: component.Y + apothem*math.Sin(theta)}
}

// coordToXY maps conventional axial coordinates to inches. For pointy-top layouts,
// positive Q points east and positive R points southeast. The origin is the canvas
// center. The inverse pointy-top basis conversion preserves the previous floating-point
// operation order, so the geometry continues to match existing SVG golden files.
func coordToXY(coord Coord, width, height, pitch float64, orientation string) Point {
	baseAngle := degreesToRadians(30)
	q, r := float64(coord.Q), float64(coord.R)
	if orientation != "flat" {
		baseAngle = degreesToRadians(-60)
		q, r = float64(-coord.R), float64(coord.Q+coord.R)
	}
	theta0, theta1 := baseAngle, baseAngle+radiansPerHexStep
	return Point{
		X: width/2.0 + pitch*(q*math.Cos(theta0)+r*math.Cos(theta1)),
		Y: height/2.0 + pitch*(q*math.Sin(theta0)+r*math.Sin(theta1)),
	}
}

// candidateCoords returns lattice cells whose centers lie within the margin-adjusted
// canvas. A negative margin intentionally includes off-canvas centers for the grid layer.
func candidateCoords(width, height, pitch, margin float64, orientation string) []gridCandidate {
	// The search range must cover the canvas in either axial direction. The pitch
	// floor prevents division by zero if this helper is called with invalid input.
	limit := int(math.Ceil((width+height)/math.Max(pitch, candidatePitchFloor))) + 3
	result := make([]gridCandidate, 0)
	for q := -limit; q <= limit; q++ {
		for r := -limit; r <= limit; r++ {
			coord := conventionalCoord(Coord{Q: q, R: r}, orientation)
			point := coordToXY(coord, width, height, pitch, orientation)
			if margin <= point.X && point.X <= width-margin && margin <= point.Y && point.Y <= height-margin {
				result = append(result, gridCandidate{Coord: coord, Point: point})
			}
		}
	}
	return result
}

// chooseComponents places the circuit's hexagonal components and returns preferred
// connections between them. Placement is split into anchor selection, cluster growth,
// and component/edge construction so each stage can be understood independently.
func chooseComponents(o Options, rng *rand.Rand) ([]Component, [][2]int, error) {
	margin := o.ComponentRadius + o.TraceWidth + o.NodeRadius
	candidates := candidateCoords(o.Width, o.Height, o.RoutingPitch, margin, o.Orientation)
	if len(candidates) < o.ComponentCount {
		return nil, nil, fmt.Errorf("only %d candidate positions fit within the canvas for %d requested components; increase the canvas dimensions, reduce the component count, or reduce the routing pitch", len(candidates), o.ComponentCount)
	}

	candidateMap := make(map[Coord]Point, len(candidates))
	coords := make([]Coord, 0, len(candidates))
	for _, candidate := range candidates {
		candidateMap[candidate.Coord] = candidate.Point
		coords = append(coords, candidate.Coord)
	}
	// Keep candidate ordering deterministic for a fixed seed. Later stages use this
	// shuffled order when choosing among equally suitable positions.
	rng.Shuffle(len(coords), func(i, j int) { coords[i], coords[j] = coords[j], coords[i] })

	clusterCount := minInt(maximumClusterCount, maxInt(1, int(math.Ceil(float64(o.ComponentCount)/float64(componentsPerClusterAnchor)))))
	anchors := chooseClusterAnchors(coords, candidateMap, clusterCount)
	selected, err := growClusters(coords, anchors, o.ComponentCount, rng)
	if err != nil {
		return nil, nil, err
	}
	components, preferredEdges := buildComponentsAndEdges(selected, candidateMap, o, clusterCount, rng)
	return components, preferredEdges, nil
}

// chooseClusterAnchors selects cluster centers using farthest-point sampling. Starting
// with the first shuffled candidate makes the result seeded while spreading later
// anchors as far apart as possible from the anchors already chosen.
func chooseClusterAnchors(coords []Coord, candidateMap map[Coord]Point, clusterCount int) []Coord {
	anchors := []Coord{coords[0]}
	for len(anchors) < clusterCount {
		best := Coord{}
		bestScore := -1.0
		for _, candidate := range coords {
			isAnchor := false
			for _, anchor := range anchors {
				if candidate == anchor {
					isAnchor = true
					break
				}
			}
			if isAnchor {
				continue
			}
			minDistance := math.Inf(1)
			for _, anchor := range anchors {
				d := distance(candidateMap[candidate], candidateMap[anchor])
				if d < minDistance {
					minDistance = d
				}
			}
			if minDistance > bestScore {
				bestScore, best = minDistance, candidate
			}
		}
		anchors = append(anchors, best)
	}
	return anchors
}

// growClusters places non-anchor components near members of the least-populated
// clusters. It records each component's parent coordinate so construction can turn
// those relationships into preferred routing edges later.
func growClusters(coords, anchors []Coord, componentCount int, rng *rand.Rand) ([]selectedComponent, error) {
	clusterCount := len(anchors)
	selected := make([]selectedComponent, 0, componentCount)
	for clusterID, coord := range anchors {
		selected = append(selected, selectedComponent{coord: coord, cluster: clusterID})
	}
	clusterSizes := make([]int, clusterCount)
	for i := range clusterSizes {
		clusterSizes[i] = 1
	}

	for len(selected) < componentCount {
		selectedCoords := make([]Coord, len(selected))
		for i, item := range selected {
			selectedCoords[i] = item.coord
		}

		// Only consider unused cells at least minimumComponentSeparation lattice
		// moves away from every component already placed.
		eligible := make([]Coord, 0)
		for _, candidate := range coords {
			used := false
			for _, current := range selectedCoords {
				if current == candidate {
					used = true
					break
				}
			}
			if used {
				continue
			}
			farEnough := true
			for _, current := range selectedCoords {
				if axialDistance(candidate, current) < minimumComponentSeparation {
					farEnough = false
					break
				}
			}
			if farEnough {
				eligible = append(eligible, candidate)
			}
		}
		if len(eligible) == 0 {
			return nil, &retryableGenerationError{err: fmt.Errorf("could not place %d components within the canvas while maintaining the minimum separation; increase the canvas dimensions or reduce the component count", componentCount)}
		}

		clusterOrder := make([]int, clusterCount)
		for i := range clusterOrder {
			clusterOrder[i] = i
		}
		sort.SliceStable(clusterOrder, func(i, j int) bool { return clusterSizes[clusterOrder[i]] < clusterSizes[clusterOrder[j]] })

		var chosen selectedComponent
		found := false
		for _, clusterID := range clusterOrder {
			clusterNodes := make([]Coord, 0)
			for _, item := range selected {
				if item.cluster == clusterID {
					clusterNodes = append(clusterNodes, item.coord)
				}
			}
			type localCandidate struct {
				coord, parent Coord
				d             int
			}
			local := make([]localCandidate, 0)
			for _, candidate := range eligible {
				var bestParent Coord
				bestD := math.MaxInt
				for _, node := range clusterNodes {
					d := axialDistance(candidate, node)
					if d >= minimumLocalParentDistance && d <= maximumLocalParentDistance && !isAxisAligned(candidate, node) && d < bestD {
						bestD, bestParent = d, node
					}
				}
				if bestD != math.MaxInt {
					local = append(local, localCandidate{coord: candidate, parent: bestParent, d: bestD})
				}
			}
			if len(local) > 0 {
				pick := local[rng.Intn(len(local))]
				chosen = selectedComponent{coord: pick.coord, cluster: clusterID, parent: pick.parent, hasParent: true}
				found = true
				break
			}
		}
		if !found {
			coord := eligible[rng.Intn(len(eligible))]
			parentItem := selected[0]
			bestD := axialDistance(coord, parentItem.coord)
			for _, item := range selected[1:] {
				d := axialDistance(coord, item.coord)
				if d < bestD {
					parentItem, bestD = item, d
				}
			}
			chosen = selectedComponent{coord: coord, cluster: parentItem.cluster, parent: parentItem.coord, hasParent: true}
		}

		selected = append(selected, chosen)
		clusterSizes[chosen.cluster]++
	}
	return selected, nil
}

// buildComponentsAndEdges assigns component sizes and roles, then converts the parent
// links recorded during cluster growth into preferred edges. Finally it adds a short
// chain of inter-cluster edges to help the generated network read as one composition.
func buildComponentsAndEdges(
	selected []selectedComponent,
	candidateMap map[Coord]Point,
	o Options,
	clusterCount int,
	rng *rand.Rand,
) ([]Component, [][2]int) {
	radiusWeights := [...]float64{
		smallComponentRadiusWeight,
		mediumSmallComponentRadiusWeight,
		mediumComponentRadiusWeight,
		mediumLargeComponentRadiusWeight,
		fullComponentRadiusWeight,
	}
	radiusScales := [...]float64{
		smallComponentRadiusScale,
		mediumSmallComponentRadiusScale,
		mediumComponentRadiusScale,
		mediumLargeComponentRadiusScale,
		fullComponentRadiusScale,
	}
	components := make([]Component, 0, len(selected))
	preferredEdges := make([][2]int, 0, len(selected))
	for index, item := range selected {
		// Cluster anchors become larger hubs. Other components use a weighted size
		// distribution, so the image has a mix of small details and medium-sized nodes.
		var radius float64
		var radiusScale float64
		var hub bool
		if index < clusterCount {
			radiusScale = hubRadiusMinimumScale + hubRadiusScaleRange*rng.Float64()
			radius = o.ComponentRadius * radiusScale
			hub = true
		} else {
			// Radius scales are fractions of the configured maximum. The 0.55 and 0.70
			// scales are most common, while full-size components are deliberately rare.
			radiusScale = radiusScales[weightedChoice(rng, radiusWeights[:])]
			radius = o.ComponentRadius * radiusScale
			hub = radiusScale >= hubRadiusThreshold
		}
		p := candidateMap[item.coord]
		components = append(
			components,
			Component{
				HexComponent: HexComponent{
					Index:  index,
					X:      p.X,
					Y:      p.Y,
					Radius: radius,
				},
				Q:           item.coord.Q,
				R:           item.coord.R,
				Cluster:     item.cluster,
				Hub:         hub,
				RadiusScale: radiusScale,
			},
		)
		if item.hasParent {
			parentIndex := -1
			for previous := 0; previous < index; previous++ {
				if selected[previous].coord == item.parent {
					parentIndex = previous
					break
				}
			}
			if parentIndex >= 0 {
				preferredEdges = append(preferredEdges, [2]int{index, parentIndex})
			}
		}
	}

	for clusterID := 0; clusterID < clusterCount-1; clusterID++ {
		bestA, bestB, bestDistance := -1, -1, math.MaxInt
		for _, a := range components {
			if a.Cluster != clusterID {
				continue
			}
			for _, b := range components {
				if b.Cluster != clusterID+1 {
					continue
				}
				d := axialDistance(a.Coord(), b.Coord())
				if d >= minimumInterclusterDistance && d <= maximumInterclusterDistance && !isAxisAligned(a.Coord(), b.Coord()) && d < bestDistance {
					bestA, bestB, bestDistance = a.Index, b.Index, d
				}
			}
		}
		if bestA >= 0 {
			preferredEdges = append(preferredEdges, [2]int{bestA, bestB})
		}
	}
	return components, preferredEdges
}

// weightedChoice returns an index selected with probability proportional to its
// weight. The weights must be non-empty, non-negative, and have a positive sum.
func weightedChoice(rng *rand.Rand, weights []float64) int {
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	pick := rng.Float64() * total
	for i, weight := range weights {
		pick -= weight
		if pick < 0 {
			return i
		}
	}
	return len(weights) - 1
}

// offsetPolyline shifts each segment along its perpendicular normal. At a bend, it
// intersects the neighboring shifted lines to form a miter; translating every point
// by one vector would not preserve perpendicular spacing along a bent path.
func offsetPolyline(points []Point, offset float64) []Point {
	if len(points) < 2 {
		return append([]Point(nil), points...)
	}
	if math.Abs(offset) < vectorEpsilon {
		return append([]Point(nil), points...)
	}
	// Compute the direction and left-hand normal for every segment. Offsetting a line
	// means moving it along its normal, not along the direction in which it travels.
	dirs := make([]Point, len(points)-1)
	normals := make([]Point, len(points)-1)
	for i := 0; i < len(points)-1; i++ {
		dirs[i] = unit(sub(points[i+1], points[i]))
		normals[i] = Point{X: -dirs[i].Y, Y: dirs[i].X}
	}
	result := make([]Point, 0, len(points))
	result = append(result, add(points[0], mul(normals[0], offset)))
	// At each bend, intersect the two shifted segment lines to make a miter corner.
	// Parallel neighboring segments have no unique intersection, so use one normal.
	for i := 1; i < len(points)-1; i++ {
		p, d1, d2 := points[i], dirs[i-1], dirs[i]
		n1, n2 := normals[i-1], normals[i]
		line1, line2 := add(p, mul(n1, offset)), add(p, mul(n2, offset))
		denominator := cross(d1, d2)
		if math.Abs(denominator) < epsilon {
			result = append(result, add(p, mul(n1, offset)))
		} else {
			t := cross(sub(line2, line1), d2) / denominator
			result = append(result, add(line1, mul(d1, t)))
		}
	}
	result = append(result, add(points[len(points)-1], mul(normals[len(normals)-1], offset)))
	return result
}

// pointSegmentDistance returns the shortest distance from point p to the finite segment
// from a to b, clamping the projection so it cannot land beyond either endpoint.
func pointSegmentDistance(p, a, b Point) float64 {
	ab := sub(b, a)
	denominator := dot(ab, ab)
	if denominator < vectorEpsilon {
		return distance(p, a)
	}
	t := math.Max(0, math.Min(1, dot(sub(p, a), ab)/denominator))
	return distance(p, add(a, mul(ab, t)))
}

// segmentSegmentDistance returns zero for intersecting segments; otherwise it returns
// the minimum distance between an endpoint of either segment and the other segment.
func segmentSegmentDistance(a, b, c, d Point) float64 {
	if segmentsIntersect(a, b, c, d) {
		return 0
	}
	return math.Min(math.Min(pointSegmentDistance(a, c, d), pointSegmentDistance(b, c, d)), math.Min(pointSegmentDistance(c, a, b), pointSegmentDistance(d, a, b)))
}

// hexagonVertices returns the six corners of a regular hexagon. Pointy orientation
// starts at the top; flat orientation starts at the right.
func hexagonVertices(cx, cy, radius float64, orientation string) []Point {
	start := -90.0
	if orientation == "flat" {
		start = 0
	}
	vertices := make([]Point, hexDirectionCount)
	for i := range vertices {
		angle := (start + degreesPerHexStep*float64(i)) * math.Pi / 180
		vertices[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}
	return vertices
}

// pointInPolygon reports whether point lies inside a simple polygon using ray casting.
func pointInPolygon(point Point, polygon []Point) bool {
	x, y := point.X, point.Y
	inside := false
	j := len(polygon) - 1
	for i := 0; i < len(polygon); i++ {
		xi, yi := polygon[i].X, polygon[i].Y
		xj, yj := polygon[j].X, polygon[j].Y
		if (yi > y) != (yj > y) {
			xIntersect := (xj-xi)*(y-yi)/(yj-yi) + xi
			if x < xIntersect {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// segmentsIntersect treats touching endpoints and collinear overlap as intersections.
func segmentsIntersect(a, b, c, d Point) bool {
	ab, cd, ac := sub(b, a), sub(d, c), sub(c, a)
	denominator := cross(ab, cd)
	if math.Abs(denominator) < epsilon {
		if math.Abs(cross(ac, ab)) > epsilon {
			return false
		}
		axis := 0
		if math.Abs(ab.Y) > math.Abs(ab.X) {
			axis = 1
		}
		var aa, bb, cc, dd float64
		if axis == 0 {
			aa, bb, cc, dd = a.X, b.X, c.X, d.X
		} else {
			aa, bb, cc, dd = a.Y, b.Y, c.Y, d.Y
		}
		if aa > bb {
			aa, bb = bb, aa
		}
		if cc > dd {
			cc, dd = dd, cc
		}
		return math.Max(aa, cc) <= math.Min(bb, dd)+epsilon
	}
	t := cross(ac, cd) / denominator
	u := cross(ac, ab) / denominator
	return t >= -epsilon && t <= 1+epsilon && u >= -epsilon && u <= 1+epsilon
}

// pointInsideHexWithMargin tests a regular hexagon's six half-planes after insetting
// every side by margin.
func pointInsideHexWithMargin(point, center Point, radius, baseAngle, margin float64) bool {
	allowed := radius*sqrt3/2 - margin
	if allowed <= 0 {
		return false
	}
	delta := sub(point, center)
	for side := 0; side < hexDirectionCount; side++ {
		theta := baseAngle + degreesPerHexStep*float64(side)*math.Pi/180
		normal := Point{X: math.Cos(theta), Y: math.Sin(theta)}
		if dot(delta, normal) > allowed+epsilon {
			return false
		}
	}
	return true
}

// routeHitsComponent rejects a track that enters a component polygon or comes within
// clearance of any of its edges.
func routeHitsComponent(track []Point, component Component, orientation string, clearance float64) bool {
	polygon := hexagonVertices(component.X, component.Y, component.Radius, orientation)
	for _, pair := range segmentPairs(track) {
		a, b := pair[0], pair[1]
		if pointInPolygon(a, polygon) || pointInPolygon(b, polygon) {
			return true
		}
		for i := 0; i < len(polygon); i++ {
			c, d := polygon[i], polygon[(i+1)%len(polygon)]
			if segmentSegmentDistance(a, b, c, d) < clearance {
				return true
			}
		}
	}
	return false
}

func segmentPairs(points []Point) [][2]Point {
	pairs := make([][2]Point, 0, maxInt(0, len(points)-1))
	for i := 0; i+1 < len(points); i++ {
		pairs = append(pairs, [2]Point{points[i], points[i+1]})
	}
	return pairs
}

// tracksCollide rejects intersections and close passes. New tracks use the full
// clearance against accepted tracks; tracks within one bundle use a reduced threshold
// because they are deliberately parallel and closely spaced.
func tracksCollide(candidateTracks, existingTracks [][]Point, clearance float64) bool {
	// First compare every new route against all routes accepted on earlier iterations.
	for _, candidate := range candidateTracks {
		for _, existing := range existingTracks {
			for _, a := range segmentPairs(candidate) {
				for _, b := range segmentPairs(existing) {
					if segmentSegmentDistance(a[0], a[1], b[0], b[1]) < clearance {
						return true
					}
				}
			}
		}
	}
	// Then check the new bundle internally, allowing a slightly smaller threshold so
	// a valid closely packed parallel bundle is not rejected unnecessarily.
	for i, first := range candidateTracks {
		for _, second := range candidateTracks[i+1:] {
			for _, a := range segmentPairs(first) {
				for _, b := range segmentPairs(second) {
					if segmentSegmentDistance(a[0], a[1], b[0], b[1]) < clearance*parallelTrackCollisionFactor {
						return true
					}
				}
			}
		}
	}
	return false
}

// trackInsideCanvas requires every polyline vertex to remain within the inset canvas.
func trackInsideCanvas(track []Point, width, height, margin float64) bool {
	for _, point := range track {
		if point.X < margin || point.X > width-margin || point.Y < margin || point.Y > height-margin {
			return false
		}
	}
	return true
}

// buildConnection turns a lattice route into a visible bundle of parallel SVG tracks.
// It validates route direction and bends, chooses a feasible bundle size, offsets the
// tracks, places endpoints inside the destination hexagon, and rejects routes that
// collide with other components, terminal pads, or previously accepted tracks.
func (rt *router) buildConnection(src, dst Component, path []Coord, side int) *AcceptedConnection {
	components := rt.components
	o := rt.options
	baseAngle := rt.baseAngle
	rng := rt.rng
	occupiedFaces := rt.occupiedFaces
	endpointPoints := rt.endpointPoints
	existingTracks := rt.acceptedTracks

	if occupiedFaces[faceKey{Component: src.Index, Side: side}] {
		return nil
	}
	// Require at least three lattice moves and an exact destination match.
	if len(path) < minLatticeMoves+1 || path[len(path)-1] != dst.Coord() {
		return nil
	}
	// Directions let validation ensure the path starts through the requested face and
	// each turn is exactly 60 degrees (a 120-degree interior angle).
	directions := make([]int, 0, len(path)-1)
	for i := 0; i+1 < len(path); i++ {
		delta := Coord{Q: path[i+1].Q - path[i].Q, R: path[i+1].R - path[i].R}
		direction := -1
		for j, step := range rt.axialSteps {
			if step == delta {
				direction = j
				break
			}
		}
		if direction < 0 {
			return nil
		}
		directions = append(directions, direction)
	}
	if directions[0] != side {
		return nil
	}
	hasBend := false
	for i := 0; i+1 < len(directions); i++ {
		if directions[i] != directions[i+1] {
			hasBend = true
			break
		}
	}
	if !hasBend {
		return nil
	}
	for i := 0; i+1 < len(directions); i++ {
		delta := (directions[i+1] - directions[i] + hexDirectionCount) % hexDirectionCount
		if delta != 0 && delta != 1 && delta != hexDirectionCount-1 {
			return nil
		}
	}
	targetSide := (directions[len(directions)-1] + hexDirectionCount/2) % hexDirectionCount
	if occupiedFaces[faceKey{Component: dst.Index, Side: targetSide}] {
		return nil
	}

	// Reject bundle sizes whose outer tracks and terminal pads cannot fit inside both
	// endpoint hexagons. Among feasible sizes, 2- and 3-track bundles are favored.
	feasibleCounts := make([]int, 0)
	for count := 1; count <= o.MaxParallel; count++ {
		maxOffset := float64(count-1) * o.ParallelSpacing / 2.0
		sourceRoom := maxOffset+o.TraceWidth/2.0 <= src.Radius/2.0
		targetRoom := maxOffset+o.NodeRadius+o.TraceWidth/2.0 <= dst.Radius*sqrt3/2.0
		if sourceRoom && targetRoom {
			feasibleCounts = append(feasibleCounts, count)
		}
	}
	if len(feasibleCounts) == 0 {
		feasibleCounts = []int{1}
	}
	weights := []float64{singleTrackWeight, doubleTrackWeight, tripleTrackWeight}
	feasibleWeights := make([]float64, len(feasibleCounts))
	for i, count := range feasibleCounts {
		if count <= len(weights) {
			feasibleWeights[i] = weights[count-1]
		} else {
			feasibleWeights[i] = 1
		}
	}
	trackCount := feasibleCounts[weightedChoice(rng, feasibleWeights)]

	// Start at the source face midpoint, then follow cell centers. Parallel tracks are
	// offsets of this shared centerline.
	routePoints := []Point{faceMidpoint(src, side, baseAngle)}
	for _, coord := range path[1:] {
		routePoints = append(routePoints, coordToXY(coord, o.Width, o.Height, o.RoutingPitch, o.Orientation))
	}
	offsets := make([]float64, trackCount)
	candidateTracks := make([][]Point, trackCount)
	for i := 0; i < trackCount; i++ {
		offsets[i] = (float64(i) - float64(trackCount-1)/2.0) * o.ParallelSpacing
		candidateTracks[i] = offsetPolyline(routePoints, offsets[i])
	}
	canvasMargin := o.TraceWidth/2.0 + routeCanvasSafetyMargin
	for _, track := range candidateTracks {
		if !trackInsideCanvas(track, o.Width, o.Height, canvasMargin) {
			return nil
		}
	}

	// Every offset track has a different endpoint. Each pad must fit inside the target
	// hexagon with room for the circle radius and trace stroke.
	endpoints := make([]Point, 0, len(candidateTracks))
	for _, track := range candidateTracks {
		endpoint := track[len(track)-1]
		if !pointInsideHexWithMargin(
			endpoint,
			Point{X: dst.X, Y: dst.Y},
			dst.Radius,
			baseAngle,
			o.NodeRadius+o.TraceWidth/2.0,
		) {
			return nil
		}
		endpoints = append(endpoints, endpoint)
	}

	// The endpoint components are intentional contacts; all other components, existing
	// terminal pads, and accepted tracks remain obstacles.
	ignored := map[int]bool{src.Index: true, dst.Index: true}
	for _, track := range candidateTracks {
		for _, component := range components {
			if !ignored[component.Index] && routeHitsComponent(track, component, o.Orientation, o.TraceClearance+o.TraceWidth/2.0) {
				return nil
			}
		}
		for _, endpoint := range endpointPoints {
			for _, segment := range segmentPairs(track) {
				if pointSegmentDistance(endpoint, segment[0], segment[1]) < o.NodeRadius+o.TraceClearance {
					return nil
				}
			}
		}
	}
	if tracksCollide(candidateTracks, existingTracks, o.TraceClearance+o.TraceWidth) {
		return nil
	}
	return &AcceptedConnection{
		Source: src.Index, Target: dst.Index,
		SourceSide: side, TargetSide: targetSide,
		Path: append([]Coord(nil), path...), Lines: trackCount,
		Tracks: candidateTracks, Endpoints: endpoints,
	}
}

// tryEdge tries both source/target directions and starting sides in proximity order.
// It returns the first route that passes path validation and all collision checks.
func (rt *router) tryEdge(aIndex, bIndex int) *AcceptedConnection {
	directions := [][2]int{{aIndex, bIndex}, {bIndex, aIndex}}
	rt.rng.Shuffle(len(directions), func(i, j int) { directions[i], directions[j] = directions[j], directions[i] })
	for _, pair := range directions {
		src, dst := rt.components[pair[0]], rt.components[pair[1]]
		targetAngle := math.Atan2(dst.Y-src.Y, dst.X-src.X)
		approxSide := int(math.Round((targetAngle-rt.baseAngle)/radiansPerHexStep)) % hexDirectionCount
		if approxSide < 0 {
			approxSide += hexDirectionCount
		}
		sides := []int{
			approxSide,
			(approxSide + 1) % hexDirectionCount,
			(approxSide + hexDirectionCount - 1) % hexDirectionCount,
			(approxSide + 2) % hexDirectionCount,
			(approxSide + hexDirectionCount - 2) % hexDirectionCount,
			(approxSide + hexDirectionCount/2) % hexDirectionCount,
		}
		for attempt := 0; attempt < rt.options.RouteAttempts; attempt++ {
			side := sides[attempt%len(sides)]
			if rt.occupiedFaces[faceKey{Component: src.Index, Side: side}] {
				continue
			}
			path := rt.findRouteForSide(src, dst, nil, side)
			if path == nil {
				continue
			}
			connection := rt.buildConnection(src, dst, path, side)
			if connection != nil {
				return connection
			}
		}
	}
	return nil
}

// nextDeterministicRouteCost scores a move using only its geometric properties.
// A straight move costs routeStepCost; changing direction also incurs routeTurnPenalty.
func nextDeterministicRouteCost(currentCost float64, turned bool) float64 {
	cost := currentCost + routeStepCost
	if turned {
		cost += routeTurnPenalty
	}
	return cost
}

// nextLegacyRouteCost preserves the earlier cost model by adding random jitter to
// every explored move. It is a router method because consuming the router's RNG is
// part of the legacy search behavior.
func (rt *router) nextLegacyRouteCost(currentCost float64, turned bool) float64 {
	return nextDeterministicRouteCost(currentCost, turned) + rt.rng.Float64()*routeCostJitter
}

// findRouteForSide searches for a route whose first step leaves src through
// firstDirection. In deterministic-cost mode, each move has a fixed cost and turns
// have a fixed penalty; legacy mode additionally adds random jitter to each move's
// cost. Both modes retain randomized exploration order and queue tie-breaking.
// Other components and points too near the canvas edge are blocked, and a valid route
// must turn at least once and contain minLatticeMoves moves. It returns nil on failure.
func (rt *router) findRouteForSide(src, dst Component, reservedCoords map[Coord]bool, firstDirection int) []Coord {
	components := rt.components
	width, height := rt.options.Width, rt.options.Height
	pitch := rt.options.RoutingPitch
	maxSteps, rng := rt.options.MaxRouteSteps, rt.rng
	useDeterministicRouteCost := rt.options.UseDeterministicRouteCost

	// Components other than this route's endpoints are impassable lattice cells.
	blocked := make(map[Coord]bool)
	for _, component := range components {
		if component.Index != src.Index && component.Index != dst.Index {
			blocked[component.Coord()] = true
		}
	}
	for coord := range reservedCoords {
		blocked[coord] = true
	}
	step := rt.axialSteps[firstDirection]
	firstCoord := Coord{Q: src.Q + step.Q, R: src.R + step.R}
	withinCanvas := func(coord Coord) bool {
		p := coordToXY(coord, width, height, pitch, rt.options.Orientation)
		return routeCanvasInset <= p.X && p.X <= width-routeCanvasInset &&
			routeCanvasInset <= p.Y && p.Y <= height-routeCanvasInset
	}
	if blocked[firstCoord] || !withinCanvas(firstCoord) {
		return nil
	}
	// The first move is fixed by the source side. Search begins in the adjacent cell;
	// Turned tracks whether the route has made the required at-least-one bend so far.
	start := routeState{Q: firstCoord.Q, R: firstCoord.R, LastDir: firstDirection, Turned: false}
	bestCost := map[routeState]float64{start: routeStepCost}
	latticeMoves := map[routeState]int{start: 1}
	cameFrom := make(map[routeState]routeState)
	queue := &routeQueue{}
	heap.Init(queue)
	serial := 0
	heap.Push(queue, routeItem{state: start, cost: routeStepCost, priority: routeStepCost + float64(axialDistance(firstCoord, dst.Coord())), tie: rng.Float64() * routeQueueTieBreakRange, serial: serial})
	var found routeState
	foundOK := false
	for queue.Len() > 0 {
		item := heap.Pop(queue).(routeItem)
		state := item.state
		currentCost, known := bestCost[state]
		if useDeterministicRouteCost && (!known || item.cost != currentCost) {
			// In the deterministic search, discard entries superseded by a cheaper
			// route. Legacy mode deliberately retains the old queue behavior so fixed
			// seeds continue to reproduce the pre-refactor route network.
			continue
		}
		coord := Coord{Q: state.Q, R: state.R}
		if coord == dst.Coord() {
			if state.Turned && latticeMoves[state] >= minLatticeMoves {
				found, foundOK = state, true
				break
			}
			if !useDeterministicRouteCost && state.Turned && latticeMoves[state] >= legacyEarlyGoalMoves {
				// Historically, the search stopped at the first turned destination,
				// even when it was too short for buildConnection. Returning nil here
				// preserves that attempt's behavior without exposing an invalid path.
				return nil
			}
			continue
		}
		if latticeMoves[state] >= maxSteps {
			continue
		}
		// Only straight-ahead or +/-60-degree turns are allowed. A 60-degree change in
		// heading gives the desired 120-degree interior angle at a polyline corner.
		choices := []int{
			(state.LastDir + hexDirectionCount - 1) % hexDirectionCount,
			state.LastDir,
			(state.LastDir + 1) % hexDirectionCount,
		}
		rng.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
		for _, nextDirection := range choices {
			delta := rt.axialSteps[nextDirection]
			next := Coord{Q: coord.Q + delta.Q, R: coord.R + delta.R}
			if next != dst.Coord() && blocked[next] {
				continue
			}
			if next == src.Coord() || !withinCanvas(next) {
				continue
			}
			turned := nextDirection != state.LastDir
			nextTurned := state.Turned || turned
			nextState := routeState{Q: next.Q, R: next.R, LastDir: nextDirection, Turned: nextTurned}
			var tentative float64
			if useDeterministicRouteCost {
				tentative = nextDeterministicRouteCost(currentCost, turned)
			} else {
				tentative = rt.nextLegacyRouteCost(currentCost, turned)
			}
			old, exists := bestCost[nextState]
			if exists && tentative >= old {
				continue
			}
			bestCost[nextState] = tentative
			latticeMoves[nextState] = latticeMoves[state] + 1
			cameFrom[nextState] = state
			serial++
			heap.Push(queue, routeItem{state: nextState, cost: tentative, priority: tentative + float64(axialDistance(next, dst.Coord())), tie: rng.Float64() * routeQueueTieBreakRange, serial: serial})
		}
	}
	if !foundOK {
		return nil
	}

	// Follow parent links backward from the goal, then reverse to return source-to-goal order.
	tail := make([]Coord, 0)
	state := found
	for {
		tail = append(tail, Coord{Q: state.Q, R: state.R})
		parent, ok := cameFrom[state]
		if !ok {
			break
		}
		state = parent
	}
	for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
		tail[i], tail[j] = tail[j], tail[i]
	}
	return append([]Coord{src.Coord()}, tail...)
}

// edgeCandidates builds the order in which component pairs should be connected. The
// placement algorithm's parent/cluster links are always preferred. Fallback pairs are
// either randomized or ranked by same-cluster priority and axial distance.
func (rt *router) edgeCandidates() [][2]int {
	components := rt.components
	baseEdges := rt.preferredEdges
	rng := rt.rng

	// Keep parent/cluster links and deduplicate pairs before generating fallbacks.
	seen := make(map[[2]int]bool)
	result := make([][2]int, 0)
	for _, edge := range baseEdges {
		key := orderedPair(edge[0], edge[1])
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}

	// Build fallback links between nearby, non-axis-aligned cells. Shuffle before
	// sorting so pairs with equal priority and distance retain a randomized tie order.
	allPairs := make([]edgeCandidate, 0)
	for a := 0; a < len(components); a++ {
		for b := a + 1; b < len(components); b++ {
			key := [2]int{a, b}
			if seen[key] {
				continue
			}
			d := axialDistance(components[a].Coord(), components[b].Coord())
			if d < minimumFallbackEdgeDistance || d > maximumFallbackEdgeDistance || isAxisAligned(components[a].Coord(), components[b].Coord()) {
				continue
			}
			priority := 1
			if components[a].Cluster == components[b].Cluster {
				priority = 0
			}
			allPairs = append(allPairs, edgeCandidate{a: a, b: b, priority: priority, distance: d})
		}
	}
	rng.Shuffle(len(allPairs), func(i, j int) { allPairs[i], allPairs[j] = allPairs[j], allPairs[i] })
	sort.SliceStable(allPairs, func(i, j int) bool {
		if allPairs[i].priority != allPairs[j].priority {
			return allPairs[i].priority < allPairs[j].priority
		}
		return allPairs[i].distance < allPairs[j].distance
	})

	// Keep a copy of the ranked fallback sequence for ranked mode. result also gets
	// these pairs because the existing full-list shuffle below is retained in both
	// modes, keeping random-number consumption unchanged for a given seed.
	fallbacks := make([][2]int, 0, len(allPairs))
	for _, edge := range allPairs {
		pair := [2]int{edge.a, edge.b}
		fallbacks = append(fallbacks, pair)
		result = append(result, pair)
	}

	// The randomized mode keeps the historical sequence exactly: shuffle the complete
	// list, then stably move preferred edges ahead of fallback edges. In ranked mode,
	// the same shuffle randomizes preferred-edge order, but fallback ranking is retained.
	rng.Shuffle(len(result), func(i, j int) { result[i], result[j] = result[j], result[i] })
	baseSet := make(map[[2]int]bool, len(baseEdges))
	for _, edge := range baseEdges {
		baseSet[orderedPair(edge[0], edge[1])] = true
	}

	if rt.options.FallbackEdgeOrder == FallbackEdgeOrderRanked {
		preferred := make([][2]int, 0, len(baseEdges))
		for _, edge := range result {
			if baseSet[edge] {
				preferred = append(preferred, edge)
			}
		}
		return append(preferred, fallbacks...)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return baseSet[result[i]] && !baseSet[result[j]]
	})
	return result
}
func orderedPair(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

// renderSVG serializes a generated grid layout into SVG. The circuit package owns
// placement policy, route finding, and collision avoidance; hexgrid owns conversion
// from axial components/traces to drawable geometry and SVG rendering.
func renderSVG(
	o Options,
	components []Component,
	connections []AcceptedConnection,
	seed int64,
) (string, error) {
	scene, err := gridSceneForRender(o, components, connections)
	if err != nil {
		return "", fmt.Errorf("convert circuit layout to hexgrid: %w", err)
	}

	var wordList []string = defaultLabels
	if o.LabelWordListSet {
		wordList = o.LabelWordList
	}
	if o.ShowLabels && o.LabelProbability > 0 {
		scene.Labels = componentLabelRequests(o, scene, seed, wordList)
	}
	scene.Options.LabelFontFamily = o.LabelFont
	scene.Options.LabelColor = componentLabelFontColor(o)
	scene.Options.LabelFontSize = o.LabelFontSize
	assignComponentDetails(o, components, &scene, seed)
	return hexgrid.Render(scene)
}

// assignComponentDetails makes randomized visual decisions in the generator. The
// grid renderer receives an explicit ComponentStyle; it does not make random choices.
// A separate RNG stream keeps detail selection from perturbing placement/routing.
func assignComponentDetails(o Options, components []Component, scene *hexgrid.Scene, seed int64) {
	componentByIndex := make(map[int]Component, len(components))
	for _, component := range components {
		componentByIndex[component.Index] = component
	}

	detailRNG := rand.New(rand.NewSource(seed))
	for i := range scene.Components {
		visualComponent := &scene.Components[i]
		component, ok := componentByIndex[i]
		if !ok {
			continue
		}

		radiusScale := visualComponent.RadiusScale
		if radiusScale == 0 {
			radiusScale = 1
		}
		radius := o.ComponentRadius * radiusScale
		detailChance := o.DetailProb * componentDetailScale
		if component.Hub {
			detailChance = math.Min(1.0, o.DetailProb*hubDetailScale)
		}
		wantsDetails := radius >= o.ComponentRadius*minimumDetailRadius && detailRNG.Float64() < detailChance
		if wantsDetails {
			// The former nested-detail motif contains an outer, middle, and inner
			// hexagon, so it maps directly to the new triple component style.
			visualComponent.Style = hexgrid.ComponentStyleTriple
		}
	}
}

// gridSceneForRender converts generation policy structures into the high-level
// hexgrid API. Components retain their axial positions and relative radii; each
// accepted route retains its axial path and selected parallel-line count.
func gridSceneForRender(
	o Options,
	components []Component,
	connections []AcceptedConnection,
) (hexgrid.Scene, error) {
	gridOptions := hexgrid.DefaultOptions()
	gridOptions.Width = o.Width
	gridOptions.Height = o.Height
	gridOptions.Orientation = o.Orientation
	gridOptions.Color = o.Color
	gridOptions.GridColor = o.GridColor
	gridOptions.Background = o.Background
	gridOptions.Grid.Pitch = o.RoutingPitch
	gridOptions.Grid.Radius = o.GridRadius
	gridOptions.Grid.StrokeWidth = o.GridWidth
	gridOptions.Grid.OmitPartial = o.OmitPartialGridHexagons
	gridOptions.ComponentRadius = o.ComponentRadius
	gridOptions.ComponentStrokeWidth = o.StrokeWidth
	gridOptions.TraceStrokeWidth = o.TraceWidth
	gridOptions.TraceSpacing = o.ParallelSpacing
	gridOptions.NodeRadius = o.NodeRadius

	layout := hexgrid.Scene{Options: gridOptions}
	layout.Components = make([]hexgrid.GridComponent, 0, len(components))
	for _, component := range components {
		radiusScale := component.RadiusScale
		if radiusScale == 0 {
			// Retain compatibility for callers constructing Component values directly.
			radiusScale = component.Radius / o.ComponentRadius
		}
		layout.Components = append(layout.Components, hexgrid.GridComponent{
			Position:    gridPosition(component.Coord()),
			Style:       hexgrid.ComponentStyleSingle,
			RadiusScale: radiusScale,
		})
	}

	layout.Traces = make([]hexgrid.GridTrace, 0, len(connections))
	for i, connection := range connections {
		if len(connection.Path) < 2 {
			return hexgrid.Scene{}, fmt.Errorf("connection %d must contain a source and destination grid position", i)
		}
		if connection.Lines < 1 {
			return hexgrid.Scene{}, fmt.Errorf("connection %d must contain at least one parallel line", i)
		}
		trace := hexgrid.GridTrace{
			Start: gridPosition(connection.Path[0]),
			Lines: connection.Lines,
			Path:  make([]hexgrid.GridPosition, 0, len(connection.Path)-1),
		}
		for _, position := range connection.Path[1:] {
			trace.Path = append(trace.Path, gridPosition(position))
		}
		layout.Traces = append(layout.Traces, trace)
	}
	return layout, nil
}

func gridPosition(coord Coord) hexgrid.GridPosition {
	return hexgrid.GridPosition{Q: coord.Q, R: coord.R}
}
