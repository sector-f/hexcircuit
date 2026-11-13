package pcb

import (
	"math/rand"
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

func TestDecorativeTraceCanParallelAnEarlierDecorativeRoute(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 4
	o.DecorativeTraceMaxBends = 3

	firstDecorative := []hexgrid.GridPosition{
		{Q: 0, R: 0}, {Q: 0, R: 1}, {Q: 1, R: 1}, {Q: 2, R: 1},
		{Q: 3, R: 1}, {Q: 4, R: 1}, {Q: 5, R: 1},
	}
	start := hexgrid.GridPosition{Q: 0, R: 2}
	candidates := decorativeParallelCandidates(start, [][]hexgrid.GridPosition{firstDecorative}, o)
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
