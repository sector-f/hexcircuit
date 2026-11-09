package pcb

import (
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestAllocateClusterSizesRespectsMinimum(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sizes := allocateClusterSizes(19, 5, 2, rng)
	if len(sizes) != 5 {
		t.Fatalf("got %d cluster sizes, want 5", len(sizes))
	}
	total := 0
	for _, size := range sizes {
		if size < 2 {
			t.Fatalf("cluster size %d is below minimum 2", size)
		}
		total += size
	}
	if total != 19 {
		t.Fatalf("cluster sizes total %d, want 19", total)
	}
}

func TestChoosePowerSourcesLeavesGapsWhenPossible(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	indices := choosePowerSourceIndices(13, 6, rng)
	if len(indices) != 6 {
		t.Fatalf("got %d sources, want 6", len(indices))
	}
	for i := 1; i < len(indices); i++ {
		if indices[i]-indices[i-1] < 2 {
			t.Fatalf("sources %d and %d are adjacent; want at least one unused contact between them", indices[i-1], indices[i])
		}
	}
	for i, index := range indices {
		if index < 0 || index >= 13 {
			t.Fatalf("source index %d is outside connector row", index)
		}
		if i > 0 && indices[i-1] == index {
			t.Fatalf("duplicate source index %d", index)
		}
	}
}

func TestChoosePowerSourcesRelaxesGapWhenNecessary(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	indices := choosePowerSourceIndices(6, 4, rng)
	if len(indices) != 4 {
		t.Fatalf("got %d sources, want 4", len(indices))
	}
	for i := 1; i < len(indices); i++ {
		if indices[i] <= indices[i-1] {
			t.Fatalf("source indices are not strictly increasing: %v", indices)
		}
	}
}

func TestCompressPathProducesAxisAlignedWaypoints(t *testing.T) {
	path := []hexgrid.GridPosition{
		{Q: 0, R: 0},
		{Q: 1, R: 0},
		{Q: 2, R: 0},
		{Q: 2, R: -1},
		{Q: 2, R: -2},
		{Q: 3, R: -3},
	}
	compressed := compressPath(path)
	if len(compressed) != 3 {
		t.Fatalf("got %d waypoints, want 3: %#v", len(compressed), compressed)
	}
	previous := path[0]
	for i, waypoint := range compressed {
		dq := waypoint.Q - previous.Q
		dr := waypoint.R - previous.R
		if !(dq == 0 || dr == 0 || dq+dr == 0) {
			t.Fatalf("segment %d is not grid-axis aligned: %v to %v", i, previous, waypoint)
		}
		previous = waypoint
	}
}

func TestNormalizeOptionsAllowsSingletonClusters(t *testing.T) {
	o := DefaultOptions()
	o.ComponentCount = 6
	o.ClusterCount = 6
	o.MinComponentsPerCluster = 1
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("singleton clusters should be valid: %v", err)
	}
}

func TestNormalizeOptionsRejectsImpossibleMinimum(t *testing.T) {
	o := DefaultOptions()
	o.ComponentCount = 7
	o.ClusterCount = 4
	o.MinComponentsPerCluster = 2
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected an error when the total count cannot satisfy the cluster minimum")
	}
}

func TestDefaultOptionsAllowSingletonClusters(t *testing.T) {
	if got := DefaultOptions().MinComponentsPerCluster; got != 1 {
		t.Fatalf("default minimum components per cluster = %d, want 1", got)
	}
}

func TestPowerFeedsOriginateAtBottomConnectors(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 73
	o.SeedSet = true
	rng := rand.New(rand.NewSource(o.Seed))
	connectors, err := connectorPositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rng, connectors)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.PowerTraces) != o.ClusterCount {
		t.Fatalf("got %d power feeds, want %d", len(plan.PowerTraces), o.ClusterCount)
	}
	if len(plan.PowerCores) != o.ClusterCount*2 {
		t.Fatalf("got %d power cores, want %d", len(plan.PowerCores), o.ClusterCount*2)
	}

	connectorSet := make(map[hexgrid.GridPosition]bool, len(connectors))
	for _, connector := range connectors {
		connectorSet[connector] = true
	}
	usedSources := make(map[hexgrid.GridPosition]bool, len(plan.PowerTraces))
	usedHubs := make(map[hexgrid.GridPosition]bool, len(plan.PowerTraces))
	for i, trace := range plan.PowerTraces {
		if !connectorSet[trace.Start] {
			t.Errorf("power feed %d starts at %v, which is not a bottom connector", i, trace.Start)
		}
		if usedSources[trace.Start] {
			t.Errorf("multiple clusters use the same connector %v", trace.Start)
		}
		usedSources[trace.Start] = true
		if len(trace.Path) == 0 {
			t.Errorf("power feed %d has no route", i)
			continue
		}
		hub := plan.PowerCores[i*2+1].Position
		if trace.Path[len(trace.Path)-1] != hub {
			t.Errorf("power feed %d ends at %v, want cluster hub %v", i, trace.Path[len(trace.Path)-1], hub)
		}
		if usedHubs[hub] {
			t.Errorf("multiple power feeds terminate at hub %v", hub)
		}
		usedHubs[hub] = true

		first := trace.Path[0]
		dq, dr := first.Q-trace.Start.Q, first.R-trace.Start.R
		if !(dr < 0 && (dq == 0 || dq+dr == 0)) {
			t.Errorf("power feed %d does not leave its connector northward: %v -> %v", i, trace.Start, first)
		}
		previous := trace.Start
		for _, waypoint := range trace.Path {
			dq, dr := waypoint.Q-previous.Q, waypoint.R-previous.R
			if !(dq == 0 || dr == 0 || dq+dr == 0) {
				t.Errorf("power feed %d has a non-axis-aligned segment: %v -> %v", i, previous, waypoint)
			}
			previous = waypoint
		}
	}
}

func TestGenerateIsDeterministicForExplicitSeed(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 912
	o.SeedSet = true

	firstSVG, firstComponents, firstTraces, err := Generate(o)
	if err != nil {
		t.Fatal(err)
	}
	secondSVG, secondComponents, secondTraces, err := Generate(o)
	if err != nil {
		t.Fatal(err)
	}
	if firstSVG != secondSVG {
		t.Fatal("generation with an explicit seed produced different SVG documents")
	}
	if firstComponents != secondComponents || firstTraces != secondTraces {
		t.Fatalf("generation counts changed for an explicit seed: (%d, %d) vs (%d, %d)", firstComponents, firstTraces, secondComponents, secondTraces)
	}
}

func TestPowerSourceSelectionVariesAcrossSeeds(t *testing.T) {
	seen := make(map[string]bool)
	for seed := int64(0); seed < 16; seed++ {
		indices := choosePowerSourceIndices(13, 6, rand.New(rand.NewSource(seed)))
		key := fmt.Sprint(indices)
		seen[key] = true
	}
	if len(seen) < 4 {
		t.Fatalf("power source positions did not vary enough across seeds: got %d distinct patterns", len(seen))
	}
}

func TestAppendSVGLayerNamespacesOverlayIDs(t *testing.T) {
	base := `<svg viewBox="0 0 1 1"><g id="background-grid"></g><g id="traces"></g></svg>`
	overlay := `<svg viewBox="0 0 1 1"><g id="background-grid"></g><g id="traces"></g></svg>`
	combined, err := appendSVGLayer(base, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(combined, `id="background-grid"`) != 1 || strings.Count(combined, `id="traces"`) != 1 {
		t.Fatalf("base layer IDs were duplicated: %s", combined)
	}
	if !strings.Contains(combined, `id="power-background-grid"`) || !strings.Contains(combined, `id="power-traces"`) {
		t.Fatalf("overlay layer IDs were not namespaced: %s", combined)
	}
	decoder := xml.NewDecoder(strings.NewReader(combined))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("combined SVG is not well-formed XML: %v", err)
		}
	}
}

func TestNormalizeOptionsRejectsPowerBundleTooWide(t *testing.T) {
	o := DefaultOptions()
	o.TraceSpacing = 0.20
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected an error when parallel traces cannot fit inside power-entry cores")
	}
}

func TestClusterPlacementProducesConnectedTrees(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 27
	o.SeedSet = true
	connectors, err := connectorPositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), connectors)
	if err != nil {
		t.Fatal(err)
	}
	componentTotal := 0
	for _, cluster := range plan.Clusters {
		componentTotal += len(cluster.Components)
		if len(cluster.Components) == 0 {
			t.Errorf("cluster %d is empty", cluster.ID)
			continue
		}
		positions := make(map[hexgrid.GridPosition]bool, len(cluster.Components))
		for _, component := range cluster.Components {
			positions[component.Position] = true
		}
		roots := 0
		for _, component := range cluster.Components {
			if component.Parent == nil {
				roots++
				if component.Position != cluster.Hub {
					t.Errorf("cluster %d root %v is not its hub %v", cluster.ID, component.Position, cluster.Hub)
				}
				continue
			}
			if !positions[*component.Parent] {
				t.Errorf("cluster %d component %v has a parent outside its cluster: %v", cluster.ID, component.Position, *component.Parent)
			}
			if hexDistance(component.Position, *component.Parent) != 1 {
				t.Errorf("cluster %d component %v is not adjacent to its parent %v", cluster.ID, component.Position, *component.Parent)
			}
		}
		if roots != 1 {
			t.Errorf("cluster %d has %d roots, want exactly one", cluster.ID, roots)
		}
	}
	if componentTotal != o.ComponentCount {
		t.Fatalf("clusters contain %d components, want %d", componentTotal, o.ComponentCount)
	}
}

func TestBuildBoardIsDeterministicForExplicitSeed(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 118
	o.SeedSet = true
	connectors, err := connectorPositions(o)
	if err != nil {
		t.Fatal(err)
	}
	first, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), connectors)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), connectors)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("board topology differs between generations with the same seed")
	}
}
