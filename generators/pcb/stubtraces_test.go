package pcb

import (
	"math/rand"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestAddDecorativeTracesRunParallelToPowerAndInterclusterRoutes(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 4
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 6
	o.DecorativeTraceMaxBends = 3
	o.MaxParallel = 3

	powerPath := make([]hexgrid.GridPosition, 11)
	for i := range powerPath {
		powerPath[i] = hexgrid.GridPosition{Q: i, R: 0}
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

	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -5; q <= 30; q++ {
		for r := -5; r <= 20; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		powerPath[0]: true, powerPath[len(powerPath)-1]: true,
		interclusterPath[0]: true, interclusterPath[len(interclusterPath)-1]: true,
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{powerPath}, []hexgrid.GridTrace{interclusterTrace})

	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(23)),
		[][]hexgrid.GridPosition{powerPath}, []hexgrid.GridTrace{interclusterTrace},
		allowed, blocked, reserved,
	)
	if len(traces) != o.DecorativeTraceCount {
		t.Fatalf("got %d decorative traces, want %d", len(traces), o.DecorativeTraceCount)
	}

	for i, trace := range traces {
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
		if !blocked[trace.Start] {
			t.Errorf("decorative trace %d does not start at an existing component", i)
		}
		if decorativeTraceBendCount(full) < 1 || decorativeTraceBendCount(full) > o.DecorativeTraceMaxBends {
			t.Errorf("decorative trace %d has %d bends, maximum %d", i, decorativeTraceBendCount(full), o.DecorativeTraceMaxBends)
		}
		if !decorativeTracePathIsParallelToAny(full, [][]hexgrid.GridPosition{powerPath, interclusterPath}) {
			t.Errorf("decorative trace %d does not follow an existing route at a constant one-cell offset", i)
		}
	}
}

func TestDecorativeTracesCanFollowPreviouslyPlacedDecorativeTraces(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 8
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 4
	o.DecorativeTraceMaxBends = 3

	source := make([]hexgrid.GridPosition, 12)
	for i := range source {
		source[i] = hexgrid.GridPosition{Q: i, R: 0}
	}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -5; q <= 20; q++ {
		for r := -8; r <= 8; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{source[0]: true, source[len(source)-1]: true}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{source}, nil)

	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(7)), [][]hexgrid.GridPosition{source}, nil,
		allowed, blocked, reserved,
	)
	if len(traces) < 2 {
		t.Fatalf("got %d decorative traces, want at least two parallel layers", len(traces))
	}
	for i, trace := range traces {
		if !blocked[trace.Start] {
			t.Errorf("trace %d starts at %v, not at an existing component", i, trace.Start)
		}
		for _, position := range expandCompressedTrace(trace)[1:] {
			if !reserved[position] {
				t.Errorf("trace %d path cell %v was not reserved for subsequent decorations", i, position)
			}
		}
	}
}

func TestAddDecorativeTracesRequiresAnExistingLongRoute(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 3
	path := []hexgrid.GridPosition{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}}
	blocked := map[hexgrid.GridPosition]bool{path[0]: true, path[len(path)-1]: true}
	allowed := map[hexgrid.GridPosition]bool{{Q: 0, R: 0}: true, {Q: 1, R: 0}: true, {Q: 2, R: 0}: true}
	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(1)), [][]hexgrid.GridPosition{path}, nil,
		allowed, blocked, decorativeRouteReservations([][]hexgrid.GridPosition{path}, nil),
	)
	if len(traces) != 0 {
		t.Fatalf("short source route produced %d decorative traces, want none", len(traces))
	}
}

func TestAddDecorativeTracesSkipsBlockedOffsetsAndHonorsBestEffortTarget(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 5
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 6
	path := make([]hexgrid.GridPosition, 9)
	for i := range path {
		path[i] = hexgrid.GridPosition{Q: i, R: 0}
	}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 12; q++ {
		for r := -4; r <= 4; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{path[0]: true, path[len(path)-1]: true}
	for q := -4; q <= 12; q++ {
		for r := -4; r <= 4; r++ {
			if r != 0 {
				blocked[hexgrid.GridPosition{Q: q, R: r}] = true
			}
		}
	}
	reserved := decorativeRouteReservations([][]hexgrid.GridPosition{path}, nil)

	traces := addDecorativeTraces(
		o, rand.New(rand.NewSource(9)), [][]hexgrid.GridPosition{path}, nil,
		allowed, blocked, reserved,
	)
	if len(traces) != 0 {
		t.Fatalf("fully blocked parallel offsets produced %d routes, want none", len(traces))
	}
}

func TestAddDecorativeTracesDisabled(t *testing.T) {
	o := DefaultOptions()
	path := []hexgrid.GridPosition{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}, {Q: 3, R: 0}}
	if got := addDecorativeTraces(
		o, rand.New(rand.NewSource(1)), [][]hexgrid.GridPosition{path}, nil,
		map[hexgrid.GridPosition]bool{}, map[hexgrid.GridPosition]bool{}, nil,
	); len(got) != 0 {
		t.Fatalf("disabled decorative traces produced %d routes", len(got))
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

func decorativeTracePathIsParallelToAny(
	decorative []hexgrid.GridPosition,
	sources [][]hexgrid.GridPosition,
) bool {
	if len(decorative) < 4 {
		return false
	}
	for _, source := range sources {
		for _, oriented := range [][]hexgrid.GridPosition{source, reversedCopy(source)} {
			if len(oriented) < 2 || decorative[0] != oriented[0] {
				continue
			}
			offset := subtractPosition(decorative[2], oriented[1])
			if hexDistance(offset, hexgrid.GridPosition{}) != 1 {
				continue
			}
			if decorative[1] != addPosition(oriented[0], offset) {
				continue
			}
			for i := 2; i < len(decorative); i++ {
				if i-1 >= len(oriented) || decorative[i] != addPosition(oriented[i-1], offset) {
					break
				}
				if i == len(decorative)-1 {
					return true
				}
			}
		}
	}
	return false
}

func reversedCopy(path []hexgrid.GridPosition) []hexgrid.GridPosition {
	result := append([]hexgrid.GridPosition(nil), path...)
	reverseGridPath(result)
	return result
}
