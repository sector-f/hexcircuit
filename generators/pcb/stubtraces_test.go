package pcb

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestAddDecorativeTracesParallelToNearestPowerAndInterclusterRoutes(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 4
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 6
	o.DecorativeTraceMaxBends = 3
	o.MaxParallel = 3

	powerPath := make([]hexgrid.GridPosition, 11)
	for i := range powerPath {
		powerPath[i] = hexgrid.GridPosition{Q: i, R: 2}
	}
	interclusterPath := make([]hexgrid.GridPosition, 11)
	for i := range interclusterPath {
		interclusterPath[i] = hexgrid.GridPosition{Q: 20, R: i}
	}
	interclusterTrace := hexgrid.GridTrace{
		Start: interclusterPath[0],
		Path:  []hexgrid.GridPosition{interclusterPath[len(interclusterPath)-1]},
		Lines: 1,
	}
	clusters := []clusterPlan{
		{ID: 0, Components: []placedComponent{{Position: hexgrid.GridPosition{Q: 0, R: 0}}}},
		{ID: 1, Components: []placedComponent{{Position: hexgrid.GridPosition{Q: 18, R: 0}}}},
	}

	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -5; q <= 30; q++ {
		for r := -5; r <= 20; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		{Q: 0, R: 0}: true, {Q: 18, R: 0}: true,
		powerPath[0]: true, powerPath[len(powerPath)-1]: true,
		interclusterPath[0]: true, interclusterPath[len(interclusterPath)-1]: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{powerPath}, []hexgrid.GridTrace{interclusterTrace})

	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(23)), clusters,
		[][]hexgrid.GridPosition{powerPath}, []hexgrid.GridTrace{interclusterTrace},
		allowed, blocked, reserved,
	)
	if len(traces) != 2 {
		t.Fatalf("got %d decorative traces, want one from each nearby component", len(traces))
	}

	starts := map[hexgrid.GridPosition]bool{}
	for i, trace := range traces {
		starts[trace.Start] = true
		if trace.HideTerminal {
			t.Errorf("decorative trace %d should end at a hollow terminal pad", i)
		}
		if trace.Lines < 1 || trace.Lines > o.MaxParallel {
			t.Errorf("decorative trace %d line count %d outside [1, %d]", i, trace.Lines, o.MaxParallel)
		}
		full := expandCompressedTrace(trace)
		if len(full) < o.DecorativeTraceMinLength+1 || len(full) > o.DecorativeTraceMaxLength+1 {
			t.Errorf("decorative trace %d has %d steps, want [%d, %d]", i, len(full)-1, o.DecorativeTraceMinLength, o.DecorativeTraceMaxLength)
			continue
		}
		if decorativeTraceBendCount(full) > o.DecorativeTraceMaxBends {
			t.Errorf("decorative trace %d has %d bends, maximum %d", i, decorativeTraceBendCount(full), o.DecorativeTraceMaxBends)
		}
		if !decorativeTraceFollowsSource(full, [][]hexgrid.GridPosition{powerPath, interclusterPath}) {
			t.Errorf("decorative trace %d does not follow a nearby primary route at a constant offset", i)
		}
	}
	if !starts[clusters[0].Components[0].Position] || !starts[clusters[1].Components[0].Position] {
		t.Fatalf("decorative traces did not use both leaf components: %v", starts)
	}
}

func TestDecorativeConnectorAllowsClearTwoStepPaths(t *testing.T) {
	start := hexgrid.GridPosition{Q: 0, R: 0}
	target := hexgrid.GridPosition{Q: 1, R: 1}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -2; q <= 3; q++ {
		for r := -2; r <= 3; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	paths := decorativeConnectorPaths(start, target, allowed, map[hexgrid.GridPosition]bool{start: true}, nil)
	if len(paths) == 0 {
		t.Fatal("no clear two-step connector found")
	}
	foundTwoStep := false
	for _, path := range paths {
		if len(path) < 3 || len(path) > decorativeTraceMaxConnectorLength+1 ||
			path[0] != start || path[len(path)-1] != target {
			t.Fatalf("invalid bounded connector: %v", path)
		}
		if len(path) == 3 {
			foundTwoStep = true
		}
		if !hasNoSharpTraceTurns(path, nil) {
			t.Fatalf("connector contains a prohibited sharp turn: %v", path)
		}
	}
	if !foundTwoStep {
		t.Fatal("no two-step connector was returned")
	}
}

func TestDecorativeConnectorDetoursAroundBlockedShortestPath(t *testing.T) {
	start := hexgrid.GridPosition{Q: 0, R: 0}
	target := hexgrid.GridPosition{Q: 2, R: 0}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -2; q <= 4; q++ {
		for r := -2; r <= 4; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		start: true,
		{Q: 1, R: 0}: true,
	}
	paths := decorativeConnectorPaths(start, target, allowed, blocked, nil)
	if len(paths) == 0 {
		t.Fatal("no connector detoured around the blocked shortest-path cell")
	}
	for _, path := range paths {
		if len(path) != 4 {
			t.Fatalf("detour has %d steps, want exactly 3: %v", len(path)-1, path)
		}
		if path[1] == (hexgrid.GridPosition{Q: 1, R: 0}) ||
			path[2] == (hexgrid.GridPosition{Q: 1, R: 0}) {
			t.Fatalf("detour crossed the blocked shortest-path cell: %v", path)
		}
		if !hasNoSharpTraceTurns(path, nil) {
			t.Fatalf("detour contains an acute turn: %v", path)
		}
	}
}

func TestDecorativeTraceCanReachOffsetLaneWhenSourceSegmentIsFourCellsAway(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceMinLength = 5
	o.DecorativeTraceMaxLength = 5
	o.DecorativeTraceMaxBends = 3

	source := make([]hexgrid.GridPosition, 6)
	for i := range source {
		source[i] = hexgrid.GridPosition{Q: i, R: 4}
	}
	start := hexgrid.GridPosition{Q: 0, R: 0}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 8; q++ {
		for r := -2; r <= 8; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		start: true, source[0]: true, source[len(source)-1]: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{source}, nil)

	traces := addDecorativeTraceGroups(
		o, rand.New(rand.NewSource(6)),
		[][]hexgrid.GridPosition{{start}}, [][]hexgrid.GridPosition{source},
		1, allowed, blocked, reserved,
	)
	if len(traces) != 1 {
		t.Fatalf("got %d decorative traces, want one parallel trace from the four-cell-away source segment", len(traces))
	}
	path := expandCompressedTrace(traces[0])
	if len(path)-1 != o.DecorativeTraceMinLength {
		t.Fatalf("decorative trace has %d steps, want %d: %v", len(path)-1, o.DecorativeTraceMinLength, path)
	}
	if !decorativeTraceFollowsSource(path, [][]hexgrid.GridPosition{source}) {
		t.Fatalf("decorative trace does not parallel the nearby source route: %v", path)
	}
}

func TestDecorativeTraceTargetsCanExceedLeafCount(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 3
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 6
	o.DecorativeTraceMaxBends = 3

	sourceA := make([]hexgrid.GridPosition, 12)
	for i := range sourceA {
		sourceA[i] = hexgrid.GridPosition{Q: i, R: 3}
	}
	sourceB := make([]hexgrid.GridPosition, 12)
	for i := range sourceB {
		sourceB[i] = hexgrid.GridPosition{Q: -i, R: -2}
	}
	root := hexgrid.GridPosition{Q: 0, R: 0}
	leaf := hexgrid.GridPosition{Q: 0, R: 1}
	clusters := []clusterPlan{{
		ID: 0,
		Components: []placedComponent{
			{Position: root, Parent: nil},
			{Position: leaf, Parent: &root},
		},
	}}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 16; q++ {
		for r := -6; r <= 6; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		root: true, leaf: true, sourceA[0]: true, sourceA[len(sourceA)-1]: true,
		sourceB[0]: true, sourceB[len(sourceB)-1]: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{sourceA, sourceB}, nil)
	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(22)), clusters,
		[][]hexgrid.GridPosition{sourceA, sourceB}, nil,
		allowed, blocked, reserved,
	)
	if len(traces) < 2 {
		t.Fatalf("got %d decorative traces, want at least two despite only one leaf", len(traces))
	}
}

func TestDecorativeTraceCanProduceMultipleBranchesFromOneSourceComponent(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 2
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 4
	o.DecorativeTraceMaxBends = 3

	upper := make([]hexgrid.GridPosition, 9)
	lower := make([]hexgrid.GridPosition, 9)
	for i := range upper {
		upper[i] = hexgrid.GridPosition{Q: i, R: 2}
		lower[i] = hexgrid.GridPosition{Q: -i, R: -2}
	}
	start := hexgrid.GridPosition{Q: 0, R: 0}
	clusters := []clusterPlan{{
		ID: 0,
		Components: []placedComponent{{Position: start}},
	}}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 12; q++ {
		for r := -5; r <= 5; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		start: true,
		upper[0]: true, upper[len(upper)-1]: true,
		lower[0]: true, lower[len(lower)-1]: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{upper, lower}, nil)

	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(91)), clusters,
		[][]hexgrid.GridPosition{upper, lower}, nil,
		allowed, blocked, reserved,
	)
	if len(traces) != 2 {
		t.Fatalf("got %d decorative traces from one source component, want 2", len(traces))
	}
	for i, trace := range traces {
		if trace.Start != start {
			t.Errorf("trace %d starts at %v, want shared source component %v", i, trace.Start, start)
		}
	}
	if reflect.DeepEqual(expandCompressedTrace(traces[0]), expandCompressedTrace(traces[1])) {
		t.Fatal("multiple branches from one component produced duplicate routes")
	}
}

func TestDecorativeTraceCanParallelAnEarlierDecorativeRoute(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 4
	o.DecorativeTraceMaxBends = 3

	firstDecorative := []hexgrid.GridPosition{
		{Q: 0, R: 1}, {Q: 1, R: 1}, {Q: 2, R: 1}, {Q: 3, R: 1},
		{Q: 4, R: 1}, {Q: 5, R: 1}, {Q: 6, R: 1},
	}
	start := hexgrid.GridPosition{Q: 0, R: 0}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 8; q++ {
		for r := -4; r <= 8; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{start: true, firstDecorative[0]: true, firstDecorative[len(firstDecorative)-1]: true}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{firstDecorative}, nil)
	candidates := decorativeParallelCandidates(start, [][]hexgrid.GridPosition{firstDecorative}, o, allowed, blocked, reserved)
	foundParallel := false
	for _, candidate := range candidates {
		if decorativeTraceFollowsSource(candidate.path, [][]hexgrid.GridPosition{firstDecorative}) {
			foundParallel = true
			break
		}
	}
	if !foundParallel {
		t.Fatal("no parallel candidate was found alongside the earlier decorative route")
	}
}

func TestDecorativeRouteReservationsIncludeSignalTraces(t *testing.T) {
	signalTrace := hexgrid.GridTrace{
		Start: hexgrid.GridPosition{Q: 0, R: 0},
		Path:  []hexgrid.GridPosition{{Q: 0, R: 3}},
		Lines: 1,
	}
	reserved := decorativeRouteReservations(nil, nil, []hexgrid.GridTrace{signalTrace})
	for _, position := range []hexgrid.GridPosition{
		{Q: 0, R: 0}, {Q: 0, R: 1}, {Q: 0, R: 2}, {Q: 0, R: 3},
	} {
		if !reserved[position] {
			t.Errorf("signal trace cell %v was not reserved", position)
		}
	}
}

func TestDecorativeTerminalMayAdjoinOnlyItsParallelSourceRoute(t *testing.T) {
	path := []hexgrid.GridPosition{
		{Q: 0, R: 0}, {Q: 0, R: 1}, {Q: 1, R: 1}, {Q: 2, R: 1},
	}
	source := []hexgrid.GridPosition{
		{Q: 0, R: 2}, {Q: 1, R: 2}, {Q: 2, R: 2},
	}
	allowed := map[hexgrid.GridPosition]bool{
		{Q: 0, R: 0}: true, {Q: 0, R: 1}: true,
		{Q: 1, R: 1}: true, {Q: 2, R: 1}: true,
	}
	blocked := map[hexgrid.GridPosition]bool{{Q: 0, R: 0}: true}
	reserved := map[hexgrid.GridPosition]bool{
		{Q: 0, R: 2}: true, {Q: 1, R: 2}: true, {Q: 2, R: 2}: true,
	}

	if !decorativeTracePathIsClear(path, allowed, blocked, reserved, source) {
		t.Fatal("terminal clearance rejected adjacency to the route being paralleled")
	}

	reserved[hexgrid.GridPosition{Q: 3, R: 1}] = true
	if decorativeTracePathIsClear(path, allowed, blocked, reserved, source) {
		t.Fatal("terminal clearance accepted adjacency to an unrelated route")
	}
	delete(reserved, hexgrid.GridPosition{Q: 3, R: 1})

	blocked[hexgrid.GridPosition{Q: 3, R: 1}] = true
	if decorativeTracePathIsClear(path, allowed, blocked, reserved, source) {
		t.Fatal("terminal clearance accepted adjacency to another component")
	}
}

func TestUnusedPowerSourcePositionsExcludeAssignedFeeds(t *testing.T) {
	top := []hexgrid.GridPosition{{Q: 0, R: -8}, {Q: 2, R: -8}, {Q: 4, R: -8}}
	bottom := []hexgrid.GridPosition{{Q: 0, R: 8}, {Q: 2, R: 8}}
	clusters := []clusterPlan{
		{PowerSources: []powerSource{{Position: top[1]}}},
		{PowerSources: []powerSource{{Position: bottom[0]}}},
	}
	got := unusedPowerSourcePositions(top, bottom, clusters)
	want := []hexgrid.GridPosition{top[0], top[2], bottom[1]}
	if len(got) != len(want) {
		t.Fatalf("got %d unused contacts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unused contact %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAddDecorativePowerTracesStartAtUnusedPowerContacts(t *testing.T) {
	o := DefaultOptions()
	o.DecorativePowerTraceCount = 1
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 4
	o.DecorativeTraceMaxBends = 3

	powerPath := make([]hexgrid.GridPosition, 11)
	for i := range powerPath {
		powerPath[i] = hexgrid.GridPosition{Q: i, R: 2}
	}
	unused := hexgrid.GridPosition{Q: 0, R: 0}
	activeSource := powerPath[0]
	activeTarget := powerPath[len(powerPath)-1]
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -3; q <= 14; q++ {
		for r := -3; r <= 6; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		unused: true, activeSource: true, activeTarget: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{powerPath}, nil)

	traces := addDecorativePowerTraces(
		o, rand.New(rand.NewSource(12)), []hexgrid.GridPosition{unused},
		[][]hexgrid.GridPosition{powerPath}, nil, nil,
		allowed, blocked, reserved,
	)
	if len(traces) != 1 {
		t.Fatalf("got %d decorative power traces, want 1", len(traces))
	}
	trace := traces[0]
	if trace.Start != unused {
		t.Fatalf("decorative power trace starts at %v, want unused contact %v", trace.Start, unused)
	}
	if trace.HideTerminal {
		t.Fatal("decorative power trace should have a hollow terminal pad")
	}
	full := expandCompressedTrace(trace)
	if len(full) != o.DecorativeTraceMinLength+1 {
		t.Fatalf("decorative power trace has %d steps, want %d", len(full)-1, o.DecorativeTraceMinLength)
	}
	if !decorativeTraceFollowsSource(full, [][]hexgrid.GridPosition{powerPath}) {
		t.Fatalf("decorative power trace does not parallel the existing power route: %v", full)
	}
}

func TestDecorativePowerTraceCanProduceMultipleBranchesFromOneUnusedContact(t *testing.T) {
	o := DefaultOptions()
	o.DecorativePowerTraceCount = 2
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 4
	o.DecorativeTraceMaxBends = 3

	upper := make([]hexgrid.GridPosition, 9)
	lower := make([]hexgrid.GridPosition, 9)
	for i := range upper {
		upper[i] = hexgrid.GridPosition{Q: i, R: 2}
		lower[i] = hexgrid.GridPosition{Q: -i, R: -2}
	}
	unused := hexgrid.GridPosition{Q: 0, R: 0}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 12; q++ {
		for r := -5; r <= 5; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		unused: true,
		upper[0]: true, upper[len(upper)-1]: true,
		lower[0]: true, lower[len(lower)-1]: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{upper, lower}, nil)

	traces := addDecorativePowerTraces(
		o, rand.New(rand.NewSource(91)), []hexgrid.GridPosition{unused},
		[][]hexgrid.GridPosition{upper, lower}, nil, nil,
		allowed, blocked, reserved,
	)
	if len(traces) != 2 {
		t.Fatalf("got %d decorative power traces from one unused contact, want 2", len(traces))
	}
	for i, trace := range traces {
		if trace.Start != unused {
			t.Errorf("trace %d starts at %v, want unused contact %v", i, trace.Start, unused)
		}
	}
	if reflect.DeepEqual(expandCompressedTrace(traces[0]), expandCompressedTrace(traces[1])) {
		t.Fatal("multiple power decorations from one contact produced duplicate routes")
	}
}

func TestAddDecorativePowerTracesAreBestEffortAndDisabledByDefault(t *testing.T) {
	o := DefaultOptions()
	start := hexgrid.GridPosition{Q: 0, R: 0}
	path := []hexgrid.GridPosition{{Q: 0, R: 5}, {Q: 1, R: 5}, {Q: 2, R: 5}, {Q: 3, R: 5}}
	allowed := map[hexgrid.GridPosition]bool{start: true}
	blocked := map[hexgrid.GridPosition]bool{start: true}
	if got := addDecorativePowerTraces(o, rand.New(rand.NewSource(1)), []hexgrid.GridPosition{start}, [][]hexgrid.GridPosition{path}, nil, nil, allowed, blocked, nil); len(got) != 0 {
		t.Fatalf("default-disabled decorative power traces produced %d routes", len(got))
	}
	o.DecorativePowerTraceCount = 2
	got := addDecorativePowerTraces(o, rand.New(rand.NewSource(1)), []hexgrid.GridPosition{start}, [][]hexgrid.GridPosition{path}, nil, nil, allowed, blocked, nil)
	if len(got) != 0 {
		t.Fatalf("unroutable unused contact produced %d traces, want best-effort skip", len(got))
	}
	if got := addDecorativePowerTraces(o, rand.New(rand.NewSource(1)), nil, [][]hexgrid.GridPosition{path}, nil, nil, allowed, blocked, nil); len(got) != 0 {
		t.Fatalf("no unused sources produced %d decorative power traces", len(got))
	}
}

func TestDecorativeTraceMayMakeSharpTurnOnlyAtAComponent(t *testing.T) {
	path := []hexgrid.GridPosition{
		{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 0, R: 1}, {Q: -1, R: 2},
	}
	allowed := make(map[hexgrid.GridPosition]bool)
	for _, position := range path {
		allowed[position] = true
	}
	blocked := map[hexgrid.GridPosition]bool{
		path[0]: true, path[1]: true,
	}
	if !decorativeTracePathIsClear(path, allowed, blocked, nil, nil) {
		t.Fatal("decorative trace with a component transition was rejected")
	}

	delete(blocked, path[1])
	if decorativeTracePathIsClear(path, allowed, blocked, nil, nil) {
		t.Fatal("decorative trace with a sharp free-space turn was accepted")
	}
}

func TestAddDecorativeTracesSkipsWhenNoNearbyRouteExists(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 3
	path := []hexgrid.GridPosition{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}, {Q: 3, R: 0}}
	clusters := []clusterPlan{{ID: 0, Components: []placedComponent{{Position: hexgrid.GridPosition{Q: 0, R: 8}}}}}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -5; q <= 8; q++ {
		for r := -5; r <= 12; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		{Q: 0, R: 8}: true, path[0]: true, path[len(path)-1]: true,
	}
	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(1)), clusters, [][]hexgrid.GridPosition{path}, nil,
		allowed, blocked, decorativeRouteReservations([][]hexgrid.GridPosition{path}, nil),
	)
	if len(traces) != 0 {
		t.Fatalf("got %d traces from a source %d cells away, want none", len(traces), hexDistance(clusters[0].Components[0].Position, path[0]))
	}
}

func TestAddDecorativeTracesDisabled(t *testing.T) {
	o := DefaultOptions()
	clusters := []clusterPlan{{ID: 0, Components: []placedComponent{{Position: hexgrid.GridPosition{}}}}}
	if got := addDecorativeTraces(
		o, rand.New(rand.NewSource(1)), clusters, nil, nil,
		map[hexgrid.GridPosition]bool{}, map[hexgrid.GridPosition]bool{}, nil,
	); len(got) != 0 {
		t.Fatalf("disabled decorative traces produced %d routes, want none", len(got))
	}
}

func TestDecorativeTraceExpandsCompressedInterclusterRoute(t *testing.T) {
	trace := hexgrid.GridTrace{
		Start: hexgrid.GridPosition{Q: 0, R: 0},
		Path: []hexgrid.GridPosition{
			{Q: 3, R: 0},
			{Q: 3, R: 4},
			{Q: 6, R: 4},
		},
	}
	got := expandCompressedTrace(trace)
	want := []hexgrid.GridPosition{
		{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}, {Q: 3, R: 0},
		{Q: 3, R: 1}, {Q: 3, R: 2}, {Q: 3, R: 3}, {Q: 3, R: 4},
		{Q: 4, R: 4}, {Q: 5, R: 4}, {Q: 6, R: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("expanded route has %d cells, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expanded cell %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func decorativeTraceFollowsSource(
	decorative []hexgrid.GridPosition,
	sources [][]hexgrid.GridPosition,
) bool {
	if len(decorative) < 3 {
		return false
	}
	for _, source := range sources {
		for _, path := range [][]hexgrid.GridPosition{source, reversedCopy(source)} {
			for index := 0; index < len(path)-1; index++ {
				offset := subtractPosition(decorative[1], path[index])
				if hexDistance(offset, hexgrid.GridPosition{}) != 1 ||
					decorative[2] != addPosition(path[index+1], offset) {
					continue
				}
				if index+len(decorative)-1 > len(path) {
					continue
				}
				matches := true
				for i := 3; i < len(decorative); i++ {
					if decorative[i] != addPosition(path[index+i-1], offset) {
						matches = false
						break
					}
				}
				if matches {
					return true
				}
			}
		}
	}
	return false
}
