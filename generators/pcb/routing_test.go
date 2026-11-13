package pcb

import (
	"math/rand"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestTraceTurnAllowed(t *testing.T) {
	tests := []struct {
		name        string
		previous    int
		next        int
		component   bool
		want        bool
	}{
		{name: "straight", previous: 0, next: 0, want: true},
		{name: "sixty degree bend left", previous: 0, next: 1, want: true},
		{name: "sixty degree bend right", previous: 0, next: 5, want: true},
		{name: "acute empty-space corner", previous: 0, next: 2, want: false},
		{name: "reversal", previous: 0, next: 3, want: false},
		{name: "acute corner through component", previous: 0, next: 2, component: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := traceTurnAllowed(tt.previous, tt.next, tt.component); got != tt.want {
				t.Fatalf("traceTurnAllowed(%d, %d, %v) = %v, want %v",
					tt.previous, tt.next, tt.component, got, tt.want)
			}
		})
	}
}

func TestFindRouteAvoidsAcuteTurnsInEmptySpace(t *testing.T) {
	start := hexgrid.GridPosition{Q: 0, R: 0}
	goal := hexgrid.GridPosition{Q: 5, R: 2}
	allowed := make(map[hexgrid.GridPosition]bool)
	for q := -4; q <= 10; q++ {
		for r := -4; r <= 8; r++ {
			allowed[hexgrid.GridPosition{Q: q, R: r}] = true
		}
	}
	blocked := map[hexgrid.GridPosition]bool{start: true, goal: true}

	for seed := int64(1); seed <= 20; seed++ {
		path, ok := findRoute(start, goal, allowed, blocked, nil, powerSideAny, rand.New(rand.NewSource(seed)))
		if !ok {
			t.Fatalf("seed %d: no route found", seed)
		}
		if !hasNoSharpTraceTurns(path, blocked) {
			t.Fatalf("seed %d: route contains an acute free-space turn: %v", seed, path)
		}
		seen := make(map[hexgrid.GridPosition]bool, len(path))
		for _, position := range path {
			if seen[position] {
				t.Fatalf("seed %d: route revisits %v: %v", seed, position, path)
			}
			seen[position] = true
		}
	}
}

func TestHasNoSharpTraceTurnsAllowsComponentTransitions(t *testing.T) {
	path := []hexgrid.GridPosition{
		{Q: 0, R: 0},
		{Q: 1, R: 0},
		{Q: 0, R: 1}, // This makes an acute corner relative to the previous segment.
		{Q: -1, R: 1},
		{Q: -2, R: 1},
		{Q: -3, R: 1},
	}
	if hasNoSharpTraceTurns(path, nil) {
		t.Fatal("acute free-space corner was accepted")
	}
	components := map[hexgrid.GridPosition]bool{{Q: 1, R: 0}: true}
	if !hasNoSharpTraceTurns(path, components) {
		t.Fatal("acute turn through a component was rejected")
	}
}
