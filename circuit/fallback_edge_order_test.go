package circuit

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestFallbackEdgeOrderModes(t *testing.T) {
	components := []Component{
		{HexComponent: HexComponent{Index: 0}, Q: 0, R: 0, Cluster: 0},
		{HexComponent: HexComponent{Index: 1}, Q: 2, R: 1, Cluster: 0},
		{HexComponent: HexComponent{Index: 2}, Q: 0, R: 3, Cluster: 1},
		{HexComponent: HexComponent{Index: 3}, Q: 3, R: 2, Cluster: 1},
		{HexComponent: HexComponent{Index: 4}, Q: -2, R: 3, Cluster: 0},
		{HexComponent: HexComponent{Index: 5}, Q: 2, R: -2, Cluster: 1},
		{HexComponent: HexComponent{Index: 6}, Q: 4, R: 0, Cluster: 0},
	}
	preferred := [][2]int{{0, 1}}
	const seed = 81023

	getEdges := func(order FallbackEdgeOrder) [][2]int {
		opts := DefaultOptions()
		opts.FallbackEdgeOrder = order
		rt := newRouter(opts, components, preferred, 0, rand.New(rand.NewSource(seed)))
		return rt.edgeCandidates()
	}

	randomized := getEdges(FallbackEdgeOrderRandomized)
	randomizedAgain := getEdges(FallbackEdgeOrderRandomized)
	if !reflect.DeepEqual(randomized, randomizedAgain) {
		t.Fatal("randomized fallback ordering should remain repeatable for a fixed seed")
	}

	ranked := getEdges(FallbackEdgeOrderRanked)
	if len(ranked) == 0 || ranked[0] != ([2]int{0, 1}) {
		t.Fatalf("preferred edge should remain first in ranked mode; got %v", ranked)
	}
	if len(randomized) != len(ranked) {
		t.Fatalf("ordering modes returned different candidate counts: randomized %d, ranked %d", len(randomized), len(ranked))
	}

	// The ranked mode must order fallback pairs by cluster priority first, then
	// axial distance. Shuffling is only allowed to break ties in those values.
	for i := 2; i < len(ranked); i++ {
		previous := fallbackRank(components, ranked[i-1])
		current := fallbackRank(components, ranked[i])
		if current.priority < previous.priority ||
			(current.priority == previous.priority && current.distance < previous.distance) {
			t.Fatalf("ranked fallback order regressed at %d: edge %v (priority=%d distance=%d), followed by %v (priority=%d distance=%d)",
				i, ranked[i-1], previous.priority, previous.distance, ranked[i], current.priority, current.distance)
		}
	}

	// There are enough fallbacks here that the randomized style should produce a
	// visibly different ordering from the ranked style for this fixed seed.
	if reflect.DeepEqual(randomized[1:], ranked[1:]) {
		t.Fatal("randomized and ranked modes unexpectedly produced the same fallback order")
	}
}

type edgeRank struct {
	priority int
	distance int
}

func fallbackRank(components []Component, edge [2]int) edgeRank {
	a, b := components[edge[0]], components[edge[1]]
	priority := 1
	if a.Cluster == b.Cluster {
		priority = 0
	}
	return edgeRank{priority: priority, distance: axialDistance(a.Coord(), b.Coord())}
}

func TestFallbackEdgeOrderDefaultsToRandomized(t *testing.T) {
	if got := DefaultOptions().FallbackEdgeOrder; got != FallbackEdgeOrderRandomized {
		t.Fatalf("DefaultOptions().FallbackEdgeOrder = %q, want %q", got, FallbackEdgeOrderRandomized)
	}

	opts := DefaultOptions()
	opts.FallbackEdgeOrder = ""
	normalized, err := normalizeOptions(opts)
	if err != nil {
		t.Fatalf("normalizeOptions() rejected an empty fallback edge order: %v", err)
	}
	if normalized.FallbackEdgeOrder != FallbackEdgeOrderRandomized {
		t.Fatalf("empty fallback edge order normalized to %q, want %q", normalized.FallbackEdgeOrder, FallbackEdgeOrderRandomized)
	}

	opts.FallbackEdgeOrder = "invalid"
	if _, err := normalizeOptions(opts); err == nil {
		t.Fatal("normalizeOptions() accepted an invalid fallback edge order")
	}
}

func TestGenerateFallbackEdgeOrderChangesSeededOutput(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 20261025
	opts.SeedSet = true

	randomizedSVG, _, _, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate() with randomized fallback order failed: %v", err)
	}

	opts.FallbackEdgeOrder = FallbackEdgeOrderRanked
	rankedSVG, _, _, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate() with ranked fallback order failed: %v", err)
	}
	if randomizedSVG == rankedSVG {
		t.Fatal("randomized and ranked fallback ordering produced identical SVG for the comparison seed")
	}
}
