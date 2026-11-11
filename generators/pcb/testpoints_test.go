package pcb

import (
	"math/rand"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestAddDecorativeTestPointsUsesLeafComponentsAndDistributesAcrossClusters(t *testing.T) {
	o := DefaultOptions()
	o.TestPointCount = 2
	o.TestPointMinLength = 2
	o.TestPointMaxLength = 4

	rootA := hexgrid.GridPosition{Q: 0, R: 0}
	leafA := hexgrid.GridPosition{Q: 1, R: 0}
	rootB := hexgrid.GridPosition{Q: 10, R: 0}
	leafB := hexgrid.GridPosition{Q: 11, R: 0}
	clusters := []clusterPlan{
		{ID: 0, Components: []placedComponent{
			{Position: rootA},
			{Position: leafA, Parent: &rootA},
		}},
		{ID: 1, Components: []placedComponent{
			{Position: rootB},
			{Position: leafB, Parent: &rootB},
		}},
	}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -10; q <= 22; q++ {
		for r := -10; r <= 10; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		rootA: true, leafA: true, rootB: true, leafB: true,
	}
	reserved := make(map[hexgrid.GridPosition]bool)

	traces := addDecorativeTestPoints(o, rand.New(rand.NewSource(23)), clusters, allowed, blocked, reserved)
	if len(traces) != 2 {
		t.Fatalf("got %d test points, want 2", len(traces))
	}
	starts := map[hexgrid.GridPosition]bool{}
	for _, trace := range traces {
		if trace.Start != leafA && trace.Start != leafB {
			t.Errorf("test point starts at internal/non-leaf component %v", trace.Start)
		}
		starts[trace.Start] = true
		if len(trace.Path) != 1 {
			t.Errorf("test point has %d waypoints, want one straight endpoint", len(trace.Path))
		}
		if trace.Lines != 1 || trace.HideTerminal {
			t.Errorf("test point trace properties = lines %d, hide terminal %v; want 1/false", trace.Lines, trace.HideTerminal)
		}
		end := trace.Path[0]
		distance := hexDistance(trace.Start, end)
		if distance < o.TestPointMinLength || distance > o.TestPointMaxLength {
			t.Errorf("test point length %d outside configured range", distance)
		}
		if !reserved[end] {
			t.Errorf("test point endpoint %v was not reserved", end)
		}
	}
	if !starts[leafA] || !starts[leafB] {
		t.Fatalf("test points were not distributed across both clusters: starts %v", starts)
	}
}

func TestAddDecorativeTestPointsSkipsBlockedCorridorsAndHonorsTarget(t *testing.T) {
	o := DefaultOptions()
	o.TestPointCount = 5
	o.TestPointMinLength = 2
	o.TestPointMaxLength = 3
	start := hexgrid.GridPosition{}
	cluster := clusterPlan{ID: 0, Components: []placedComponent{{Position: start}}}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -8; q <= 8; q++ {
		for r := -8; r <= 8; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{
		{Q: 2, R: 0}: true,
	}
	reserved := make(map[hexgrid.GridPosition]bool)

	traces := addDecorativeTestPoints(o, rand.New(rand.NewSource(9)), []clusterPlan{cluster}, allowed, blocked, reserved)
	if len(traces) == 0 || len(traces) > o.TestPointCount {
		t.Fatalf("got %d test points, want a best-effort count between 1 and %d", len(traces), o.TestPointCount)
	}
	for _, trace := range traces {
		if trace.Start != start {
			t.Errorf("unexpected start %v", trace.Start)
		}
		if trace.Path[0] == (hexgrid.GridPosition{Q: 2, R: 0}) {
			t.Error("test point ended on the blocked cell")
		}
	}
}

func TestAddDecorativeTestPointsDisabled(t *testing.T) {
	o := DefaultOptions()
	clusters := []clusterPlan{{ID: 0, Components: []placedComponent{{Position: hexgrid.GridPosition{}}}}}
	allowed := map[hexgrid.GridPosition]bool{{Q: 0, R: 0}: true}
	if got := addDecorativeTestPoints(o, rand.New(rand.NewSource(1)), clusters, allowed, nil, nil); len(got) != 0 {
		t.Fatalf("disabled test points produced %d traces", len(got))
	}
}
