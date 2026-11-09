package pcb

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
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

func TestChoosePowerSourceIndicesSeparated(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	indices := choosePowerSourceIndices(13, 6, PowerSourceSpacingSeparated, rng)
	if len(indices) != 6 {
		t.Fatalf("got %d sources, want 6", len(indices))
	}
	for i := 1; i < len(indices); i++ {
		if indices[i]-indices[i-1] < 2 {
			t.Fatalf("sources %d and %d are adjacent; want at least one unused contact between them", indices[i-1], indices[i])
		}
	}
}

func TestChoosePowerSourceIndicesAdjacentAllowsNeighbors(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	indices := choosePowerSourceIndices(6, 6, PowerSourceSpacingAdjacent, rng)
	if !reflect.DeepEqual(indices, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("adjacent mode selected %v, want all six consecutive contacts", indices)
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

func TestDefaultOptionsUseSingleSeparatedPowerContacts(t *testing.T) {
	o := DefaultOptions()
	if o.MinComponentsPerCluster != 1 {
		t.Fatalf("default minimum components per cluster = %d, want 1", o.MinComponentsPerCluster)
	}
	if o.PowerSourceSpacing != PowerSourceSpacingSeparated {
		t.Fatalf("default power-source spacing = %q, want separated", o.PowerSourceSpacing)
	}
	if o.PowerSourceStyle != PowerSourceStyleSingle {
		t.Fatalf("default power-source style = %q, want single", o.PowerSourceStyle)
	}
}

func TestLegacyConnectorCountMapsToBottomRow(t *testing.T) {
	o := DefaultOptions()
	o.ConnectorCount = 9
	normalized, err := normalizeOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BottomPowerSourceCount != 9 {
		t.Fatalf("legacy connector count mapped to bottom count %d, want 9", normalized.BottomPowerSourceCount)
	}
	o = DefaultOptions()
	o.BottomPowerSourceCount = 7
	normalized, err = normalizeOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BottomPowerSourceCount != 7 {
		t.Fatalf("explicit bottom source count was overridden: got %d, want 7", normalized.BottomPowerSourceCount)
	}
}

func TestNormalizeOptionsRejectsTooFewPowerSources(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount = 2
	o.BottomPowerSourceCount = 3
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected an error when total power-source contacts are fewer than clusters")
	}
}

func TestNormalizeOptionsRejectsInsufficientSeparatedCapacity(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount = 2
	o.BottomPowerSourceCount = 2
	o.ClusterCount = 4
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected an error when separated spacing cannot accommodate all clusters")
	}
	o.PowerSourceSpacing = PowerSourceSpacingAdjacent
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("adjacent spacing should accommodate four clusters in six contacts: %v", err)
	}
}

func TestPowerFeedsUseTopAndBottomRowsWithoutCrossing(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 73
	o.SeedSet = true
	rng := rand.New(rand.NewSource(o.Seed))
	topSources, bottomSources, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rng, topSources, bottomSources)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.PowerTraces) != o.ClusterCount {
		t.Fatalf("got %d power feeds, want %d", len(plan.PowerTraces), o.ClusterCount)
	}
	if len(plan.PowerCores) != o.ClusterCount*2 {
		t.Fatalf("got %d power cores, want %d", len(plan.PowerCores), o.ClusterCount*2)
	}
	if len(plan.PowerRoutes) != o.ClusterCount {
		t.Fatalf("got %d full power paths, want %d", len(plan.PowerRoutes), o.ClusterCount)
	}

	topSet := make(map[hexgrid.GridPosition]bool, len(topSources))
	bottomSet := make(map[hexgrid.GridPosition]bool, len(bottomSources))
	for _, source := range topSources {
		topSet[source] = true
	}
	for _, source := range bottomSources {
		bottomSet[source] = true
	}
	usedSources := make(map[hexgrid.GridPosition]bool, len(plan.PowerTraces))
	usedHubs := make(map[hexgrid.GridPosition]bool, len(plan.PowerTraces))
	usedPathCells := make(map[hexgrid.GridPosition]int)
	usedTop, usedBottom := 0, 0

	previousSourceX := -1.0
	for i, cluster := range plan.Clusters {
		sourcePoint := hexgrid.GridPositionToPoint(cluster.PowerSource, o.Width, o.Height, o.Pitch, "pointy")
		if i > 0 && sourcePoint.X < previousSourceX {
			t.Errorf("power sources are not assigned left-to-right with cluster hubs")
		}
		previousSourceX = sourcePoint.X
		source := cluster.PowerSource
		if usedSources[source] {
			t.Errorf("multiple clusters use the same source %v", source)
		}
		usedSources[source] = true
		if topSet[source] {
			usedTop++
			if cluster.PowerSide != powerSideTop {
				t.Errorf("cluster %d records top source %v with side %v", i, source, cluster.PowerSide)
			}
		} else if bottomSet[source] {
			usedBottom++
			if cluster.PowerSide != powerSideBottom {
				t.Errorf("cluster %d records bottom source %v with side %v", i, source, cluster.PowerSide)
			}
		} else {
			t.Errorf("cluster %d uses source %v outside the top/bottom rows", i, source)
		}
		if usedHubs[cluster.Hub] {
			t.Errorf("multiple power feeds terminate at hub %v", cluster.Hub)
		}
		usedHubs[cluster.Hub] = true

		path := plan.PowerRoutes[i]
		if len(path) < 2 {
			t.Errorf("power feed %d has an incomplete route", i)
			continue
		}
		if path[0] != source {
			t.Errorf("power feed %d starts at %v, want source %v", i, path[0], source)
		}
		if path[len(path)-1] != cluster.Hub {
			t.Errorf("power feed %d ends at %v, want hub %v", i, path[len(path)-1], cluster.Hub)
		}
		first := path[1]
		direction := hexgrid.GridPosition{Q: first.Q - source.Q, R: first.R - source.R}
		if !allowedPowerDeparture(direction, cluster.PowerSide) {
			t.Errorf("power feed %d leaves source %v in invalid direction %v for side %v", i, source, direction, cluster.PowerSide)
		}
		local := make(map[hexgrid.GridPosition]bool)
		for _, position := range path {
			if local[position] {
				t.Errorf("power feed %d revisits lattice position %v", i, position)
			}
			local[position] = true
			if previousRoute, exists := usedPathCells[position]; exists {
				t.Errorf("power feeds %d and %d share lattice position %v; feeds must never cross", previousRoute, i, position)
			}
			usedPathCells[position] = i
		}
		trace := plan.PowerTraces[i]
		if trace.Start != source || len(trace.Path) == 0 || trace.Path[len(trace.Path)-1] != cluster.Hub {
			t.Errorf("rendered power trace %d does not match its source and hub", i)
		}
	}
	if usedTop == 0 || usedBottom == 0 {
		t.Fatalf("expected active feeds from both rows, got %d top and %d bottom", usedTop, usedBottom)
	}
}

func TestGenerateIsDeterministicForExplicitSeed(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 912
	o.SeedSet = true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}

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
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	wantComponents := len(plan.Components)
	if firstComponents != wantComponents {
		t.Fatalf("component count = %d, want %d", firstComponents, wantComponents)
	}
}

func TestPowerSourceSelectionVariesAcrossSeeds(t *testing.T) {
	top, bottom := makePositions(13, 0), makePositions(13, 10)
	seen := make(map[string]bool)
	for seed := int64(0); seed < 16; seed++ {
		sources, err := choosePowerSourceAssignments(top, bottom, 6, PowerSourceSpacingSeparated, rand.New(rand.NewSource(seed)))
		if err != nil {
			t.Fatal(err)
		}
		key := fmt.Sprint(sources)
		seen[key] = true
	}
	if len(seen) < 4 {
		t.Fatalf("power source assignments did not vary enough across seeds: got %d distinct patterns", len(seen))
	}
}

func TestSourceStyleControlsBaseLayerComponents(t *testing.T) {
	for _, test := range []struct {
		style PowerSourceStyle
		want  hexgrid.ComponentStyle
	}{
		{PowerSourceStyleSingle, hexgrid.ComponentStyleSingle},
		{PowerSourceStyleDouble, hexgrid.ComponentStyleDouble},
	} {
		o := DefaultOptions()
		o.PowerSourceStyle = test.style
		top, bottom, err := powerSourcePositions(o)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildBoard(o, rand.New(rand.NewSource(41)), top, bottom)
		if err != nil {
			t.Fatal(err)
		}
		active := make(map[hexgrid.GridPosition]bool, len(plan.Clusters))
		for _, cluster := range plan.Clusters {
			active[cluster.PowerSource] = true
		}
		wantSources := append(
			visiblePowerSourceContacts(top, active, o.PowerSourceSpacing),
			visiblePowerSourceContacts(bottom, active, o.PowerSourceSpacing)...,
		)
		if got, want := len(plan.Components)-o.ComponentCount, len(wantSources); got != want {
			t.Fatalf("got %d rendered source components, want %d visible contacts", got, want)
		}
		wantPositions := make(map[hexgrid.GridPosition]bool, len(wantSources))
		for _, position := range wantSources {
			wantPositions[position] = true
		}
		for _, component := range plan.Components[o.ComponentCount:] {
			if !wantPositions[component.Position] {
				t.Errorf("unexpected source component at %v", component.Position)
			}
			if component.Style != test.want {
				t.Fatalf("source component style = %v, want %v", component.Style, test.want)
			}
		}
	}
}

func TestPowerSourceContactsLeaveVisibleSeparatedGaps(t *testing.T) {
	row := makePositions(9, 0)
	active := map[hexgrid.GridPosition]bool{row[1]: true, row[4]: true, row[7]: true}
	got := visiblePowerSourceContacts(row, active, PowerSourceSpacingSeparated)
	want := []hexgrid.GridPosition{row[0], row[1], row[4], row[7], row[8]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible source contacts = %v, want %v", got, want)
	}

	got = visiblePowerSourceContacts(row, active, PowerSourceSpacingAdjacent)
	if !reflect.DeepEqual(got, row) {
		t.Fatalf("adjacent mode hid contacts: got %v, want %v", got, row)
	}
}

func TestAllPowerSourceContactsOutsideSeparatedGapsAreRendered(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount = 11
	o.BottomPowerSourceCount = 9
	o.Seed = 818
	o.SeedSet = true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}

	active := make(map[hexgrid.GridPosition]bool, o.ClusterCount)
	for _, cluster := range plan.Clusters {
		active[cluster.PowerSource] = true
	}
	powerCores := make(map[hexgrid.GridPosition]bool, len(plan.PowerCores))
	for _, component := range plan.PowerCores {
		powerCores[component.Position] = true
	}
	drawn := make(map[hexgrid.GridPosition]hexgrid.GridComponent, len(plan.Components))
	for _, component := range plan.Components {
		drawn[component.Position] = component
	}

	wantTop := visiblePowerSourceContacts(top, active, o.PowerSourceSpacing)
	wantBottom := visiblePowerSourceContacts(bottom, active, o.PowerSourceSpacing)
	expectedVisible := make(map[hexgrid.GridPosition]bool, len(wantTop)+len(wantBottom))
	for _, source := range append(wantTop, wantBottom...) {
		expectedVisible[source] = true
	}
	allSources := append(append([]hexgrid.GridPosition(nil), top...), bottom...)
	for _, source := range allSources {
		component, exists := drawn[source]
		if exists != expectedVisible[source] {
			t.Errorf("source %v rendered=%v, want %v", source, exists, expectedVisible[source])
		}
		if exists && component.Style != powerSourceComponentStyle(o.PowerSourceStyle) {
			t.Errorf("power source %v has style %v, want %v", source, component.Style, powerSourceComponentStyle(o.PowerSourceStyle))
		}
		if active[source] && !exists {
			t.Errorf("active power source %v was not rendered", source)
		}
		if active[source] != powerCores[source] {
			t.Errorf("source %v active=%v but accent power core present=%v", source, active[source], powerCores[source])
		}
	}
	if got, want := len(plan.Components), o.ComponentCount+len(expectedVisible); got != want {
		t.Fatalf("rendered base component count = %d, want %d ordinary components + %d visible power contacts = %d", got, o.ComponentCount, len(expectedVisible), want)
	}
	if len(active) != o.ClusterCount {
		t.Fatalf("got %d active sources, want one for each of %d clusters", len(active), o.ClusterCount)
	}
}

func TestConstrainComponentsToActivePowerSourceSpan(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		o := DefaultOptions()
		o.ConstrainComponentsToPowerSourceSpan = true
		top, bottom, err := powerSourcePositions(o)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildBoard(o, rand.New(rand.NewSource(seed)), top, bottom)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		minimumX, maximumX := math.Inf(1), math.Inf(-1)
		for _, cluster := range plan.Clusters {
			x := hexgrid.GridPositionToPoint(cluster.PowerSource, o.Width, o.Height, o.Pitch, "pointy").X
			minimumX = math.Min(minimumX, x)
			maximumX = math.Max(maximumX, x)
		}
		for _, cluster := range plan.Clusters {
			for _, component := range cluster.Components {
				x := hexgrid.GridPositionToPoint(component.Position, o.Width, o.Height, o.Pitch, "pointy").X
				if x < minimumX-1e-9 || x > maximumX+1e-9 {
					t.Fatalf("seed %d: component %v at x=%f is outside active source span [%f, %f]", seed, component.Position, x, minimumX, maximumX)
				}
			}
		}
	}
}

func TestConstrainComponentsToPowerSourceSpanCanBeDisabled(t *testing.T) {
	o := DefaultOptions()
	if o.ConstrainComponentsToPowerSourceSpan {
		t.Fatal("horizontal power-source span constraint should default to disabled")
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
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
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
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	first, err := buildBoard(o, rand.New(rand.NewSource(118)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildBoard(o, rand.New(rand.NewSource(118)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("board topology differs between generations with the same seed")
	}
}

func TestNonCrossingFeedsAcrossManySeeds(t *testing.T) {
	for seed := int64(0); seed < 150; seed++ {
		o := DefaultOptions()
		top, bottom, err := powerSourcePositions(o)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildBoard(o, rand.New(rand.NewSource(seed)), top, bottom)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		seen := make(map[hexgrid.GridPosition]int)
		for routeIndex, route := range plan.PowerRoutes {
			for _, position := range route {
				if other, exists := seen[position]; exists {
					t.Fatalf("seed %d: power feeds %d and %d intersect at %v", seed, other, routeIndex, position)
				}
				seen[position] = routeIndex
			}
		}
	}
}

func TestPowerRowsCanBeConfiguredIndependently(t *testing.T) {
	for _, test := range []struct {
		name   string
		top    int
		bottom int
	}{
		{name: "top only", top: 13, bottom: 0},
		{name: "bottom only", top: 0, bottom: 13},
		{name: "both rows", top: 9, bottom: 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := DefaultOptions()
			o.TopPowerSourceCount = test.top
			o.BottomPowerSourceCount = test.bottom
			top, bottom, err := powerSourcePositions(o)
			if err != nil {
				t.Fatal(err)
			}
			if len(top) != test.top || len(bottom) != test.bottom {
				t.Fatalf("got %d top and %d bottom positions, want %d and %d", len(top), len(bottom), test.top, test.bottom)
			}
			plan, err := buildBoard(o, rand.New(rand.NewSource(29)), top, bottom)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.PowerTraces) != o.ClusterCount {
				t.Fatalf("got %d feeds, want %d", len(plan.PowerTraces), o.ClusterCount)
			}
		})
	}
}

func TestRoutingConfigurationMatrix(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Options)
	}{
		{name: "default"},
		{name: "adjacent sources", edit: func(o *Options) { o.PowerSourceSpacing = PowerSourceSpacingAdjacent }},
		{name: "double contacts", edit: func(o *Options) { o.PowerSourceStyle = PowerSourceStyleDouble }},
		{name: "dense clusters", edit: func(o *Options) {
			o.ComponentCount = 58
			o.ClusterCount = 8
			o.TopPowerSourceCount = 13
			o.BottomPowerSourceCount = 13
		}},
		{name: "larger minimum", edit: func(o *Options) { o.ComponentCount = 42; o.MinComponentsPerCluster = 3 }},
		{name: "single cluster", edit: func(o *Options) { o.ClusterCount = 1; o.ComponentCount = 18 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for seed := int64(0); seed < 15; seed++ {
				o := DefaultOptions()
				if test.edit != nil {
					test.edit(&o)
				}
				top, bottom, err := powerSourcePositions(o)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := buildBoard(o, rand.New(rand.NewSource(seed)), top, bottom)
				if err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
				seen := make(map[hexgrid.GridPosition]int)
				for routeIndex, route := range plan.PowerRoutes {
					for _, position := range route {
						if other, exists := seen[position]; exists {
							t.Fatalf("seed %d: routes %d and %d share %v", seed, other, routeIndex, position)
						}
						seen[position] = routeIndex
					}
				}
			}
		})
	}
}

func TestTopAndBottomSourcesCanFeedClustersAcrossTheBoard(t *testing.T) {
	o := DefaultOptions()
	topFeedsDownward, bottomFeedsUpward := false, false
	for seed := int64(0); seed < 60; seed++ {
		top, bottom, err := powerSourcePositions(o)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildBoard(o, rand.New(rand.NewSource(seed)), top, bottom)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for _, cluster := range plan.Clusters {
			hub := hexgrid.GridPositionToPoint(cluster.Hub, o.Width, o.Height, o.Pitch, "pointy")
			if cluster.PowerSide == powerSideTop && hub.Y > o.Height*0.62 {
				topFeedsDownward = true
			}
			if cluster.PowerSide == powerSideBottom && hub.Y < o.Height*0.38 {
				bottomFeedsUpward = true
			}
		}
		if topFeedsDownward && bottomFeedsUpward {
			return
		}
	}
	if !topFeedsDownward || !bottomFeedsUpward {
		t.Fatalf("expected both cross-board feed directions across seeds; top-to-lower=%v bottom-to-upper=%v", topFeedsDownward, bottomFeedsUpward)
	}
}

func makePositions(count, r int) []hexgrid.GridPosition {
	positions := make([]hexgrid.GridPosition, count)
	for i := range positions {
		positions[i] = hexgrid.GridPosition{Q: i, R: r}
	}
	return positions
}
