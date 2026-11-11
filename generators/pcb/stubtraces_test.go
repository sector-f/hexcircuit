package pcb

import (
	"math/rand"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestAddDecorativeTracesUsesBendsParallelLinesAndClusterDistribution(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 6
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 10
	o.DecorativeTraceMaxBends = 3
	o.MaxParallel = 3

	clusters := make([]clusterPlan, 6)
	blocked := make(map[hexgrid.GridPosition]bool)
	allowed := make(map[hexgrid.GridPosition]bool)
	for i := range clusters {
		root := hexgrid.GridPosition{Q: i * 12, R: 0}
		leaf := hexgrid.GridPosition{Q: i*12 + 1, R: 0}
		clusters[i] = clusterPlan{ID: i, Components: []placedComponent{
			{Position: root},
			{Position: leaf, Parent: &root},
		}}
		blocked[root], blocked[leaf] = true, true
	}
	for q := -10; q < 90; q++ {
		for r := -20; r <= 20; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	reserved := make(map[hexgrid.GridPosition]bool)

	traces := addDecorativeTraces(o, rand.New(rand.NewSource(23)), clusters, allowed, blocked, reserved)
	if len(traces) != o.DecorativeTraceCount {
		t.Fatalf("got %d decorative traces, want %d", len(traces), o.DecorativeTraceCount)
	}
	starts := make(map[hexgrid.GridPosition]bool)
	multipleLines := false
	for _, trace := range traces {
		starts[trace.Start] = true
		if len(trace.Path) < 2 {
			t.Errorf("trace has %d waypoints, want at least one bend and an endpoint", len(trace.Path))
		}
		if len(trace.Path)-1 > o.DecorativeTraceMaxBends {
			t.Errorf("trace has %d bends, maximum is %d", len(trace.Path)-1, o.DecorativeTraceMaxBends)
		}
		if trace.Lines < 1 || trace.Lines > o.MaxParallel {
			t.Errorf("trace line count %d outside [1, %d]", trace.Lines, o.MaxParallel)
		}
		multipleLines = multipleLines || trace.Lines > 1
		if trace.HideTerminal {
			t.Error("free-ended trace should render its hollow terminal pad")
		}
		totalLength := 0
		previous := trace.Start
		for _, waypoint := range trace.Path {
			totalLength += hexDistance(previous, waypoint)
			previous = waypoint
		}
		if totalLength < o.DecorativeTraceMinLength || totalLength > o.DecorativeTraceMaxLength {
			t.Errorf("trace length %d outside [%d, %d]", totalLength, o.DecorativeTraceMinLength, o.DecorativeTraceMaxLength)
		}
		if !reserved[trace.Path[len(trace.Path)-1]] {
			t.Errorf("trace endpoint %v was not reserved", trace.Path[len(trace.Path)-1])
		}
	}
	if len(starts) != len(clusters) {
		t.Fatalf("traces used %d distinct leaf components across %d clusters", len(starts), len(clusters))
	}
	if !multipleLines {
		t.Error("no decorative trace used multiple parallel lines")
	}
}

func TestAddDecorativeTracesSkipsBlockedPathsAndHonorsBestEffortTarget(t *testing.T) {
	o := DefaultOptions()
	o.DecorativeTraceCount = 5
	o.DecorativeTraceMinLength = 4
	o.DecorativeTraceMaxLength = 10
	start := hexgrid.GridPosition{}
	cluster := clusterPlan{ID: 0, Components: []placedComponent{{Position: start}}}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -8; q <= 8; q++ {
		for r := -8; r <= 8; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{{Q: 2, R: 0}: true}
	reserved := make(map[hexgrid.GridPosition]bool)

	traces := addDecorativeTraces(o, rand.New(rand.NewSource(9)), []clusterPlan{cluster}, allowed, blocked, reserved)
	if len(traces) > o.DecorativeTraceCount {
		t.Fatalf("got %d traces, exceeding target %d", len(traces), o.DecorativeTraceCount)
	}
	for _, trace := range traces {
		if trace.Start != start {
			t.Errorf("unexpected start %v", trace.Start)
		}
	}
}

func TestAddDecorativeTracesDisabled(t *testing.T) {
	o := DefaultOptions()
	clusters := []clusterPlan{{ID: 0, Components: []placedComponent{{Position: hexgrid.GridPosition{}}}}}
	allowed := map[hexgrid.GridPosition]bool{{Q: 0, R: 0}: true}
	if got := addDecorativeTraces(o, rand.New(rand.NewSource(1)), clusters, allowed, nil, nil); len(got) != 0 {
		t.Fatalf("disabled decorative traces produced %d routes", len(got))
	}
}
