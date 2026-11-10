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
	if o.MinPowerSourcesPerCluster != 1 || o.MaxPowerSourcesPerCluster != 1 {
		t.Fatalf("default power-source range = %d..%d, want 1..1", o.MinPowerSourcesPerCluster, o.MaxPowerSourcesPerCluster)
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

func TestNormalizeOptionsAcceptsZeroPowerSourcesPerCluster(t *testing.T) {
	o := DefaultOptions()
	o.MinPowerSourcesPerCluster = 0
	o.MaxPowerSourcesPerCluster = 0
	o.TopPowerSourceCount = 0
	o.BottomPowerSourceCount = 0
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("zero-source configuration should be valid: %v", err)
	}
}

func TestNormalizeOptionsRejectsInvalidPowerSourceRanges(t *testing.T) {
	for _, edit := range []struct {
		name string
		fn   func(*Options)
	}{
		{"negative minimum", func(o *Options) { o.MinPowerSourcesPerCluster = -1 }},
		{"maximum below minimum", func(o *Options) { o.MaxPowerSourcesPerCluster = 0 }},
		{"more than six feeds", func(o *Options) { o.MaxPowerSourcesPerCluster = 7 }},
		{"insufficient physical contacts", func(o *Options) {
			o.MinPowerSourcesPerCluster = 2
			o.MaxPowerSourcesPerCluster = 2
			o.TopPowerSourceCount = 2
			o.BottomPowerSourceCount = 2
		}},
	} {
		t.Run(edit.name, func(t *testing.T) {
			o := DefaultOptions()
			edit.fn(&o)
			if _, err := normalizeOptions(o); err == nil {
				t.Fatal("expected invalid power-source range to be rejected")
			}
		})
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

func TestNormalizeOptionsAllowsSeparatedPhysicalContactsToAllBeActive(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount, o.BottomPowerSourceCount, o.ClusterCount = 2, 2, 4
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("four separated physical contacts should support four active sources: %v", err)
	}
	o.PowerSourceSpacing = PowerSourceSpacingAdjacent
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("adjacent spacing should also accommodate four contacts: %v", err)
	}
}

func TestAllocatePowerSourceCountsRespectsConfiguredBounds(t *testing.T) {
	for seed := int64(0); seed < 32; seed++ {
		counts := allocatePowerSourceCounts(8, 1, 3, 15, rand.New(rand.NewSource(seed)))
		if len(counts) != 8 {
			t.Fatalf("seed %d: got %d cluster counts, want 8", seed, len(counts))
		}
		total := 0
		for _, count := range counts {
			if count < 1 || count > 3 {
				t.Fatalf("seed %d: cluster feed count %d outside [1, 3]", seed, count)
			}
			total += count
		}
		if total > 15 {
			t.Fatalf("seed %d: selected %d contacts, exceeding available 15", seed, total)
		}
	}
}

func TestAllocatePowerSourceCountsAllowsZero(t *testing.T) {
	for seed := int64(0); seed < 8; seed++ {
		counts := allocatePowerSourceCounts(6, 0, 6, 0, rand.New(rand.NewSource(seed)))
		for _, count := range counts {
			if count != 0 {
				t.Fatalf("seed %d: selected %d feeds with no available contacts", seed, count)
			}
		}
	}
}

func TestBuildBoardAllowsUnpoweredClusters(t *testing.T) {
	o := DefaultOptions()
	o.ComponentCount = 12
	o.ClusterCount = 3
	o.MinPowerSourcesPerCluster = 0
	o.MaxPowerSourcesPerCluster = 0
	o.TopPowerSourceCount = 0
	o.BottomPowerSourceCount = 0
	o.Connections = 0
	o.Seed, o.SeedSet = 17, true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(17)), top, bottom)
	if err != nil {
		t.Fatalf("build board without power sources: %v", err)
	}
	if len(plan.Clusters) != o.ClusterCount {
		t.Fatalf("got %d clusters, want %d", len(plan.Clusters), o.ClusterCount)
	}
	if len(plan.PowerTraces) != 0 || len(plan.PowerCores) != 0 || len(plan.PowerRoutes) != 0 {
		t.Fatalf("unpowered board unexpectedly contains power geometry: %d traces, %d cores, %d routes",
			len(plan.PowerTraces), len(plan.PowerCores), len(plan.PowerRoutes))
	}
	for _, cluster := range plan.Clusters {
		if len(cluster.PowerSources) != 0 {
			t.Fatalf("unpowered cluster %d has %d source assignments", cluster.ID, len(cluster.PowerSources))
		}
	}
	if _, _, _, err := Generate(o); err != nil {
		t.Fatalf("generate board without power sources: %v", err)
	}
}

func TestBuildBoardSupportsMultipleFeedsPerCluster(t *testing.T) {
	o := DefaultOptions()
	o.Width = 14
	o.Height = 10
	o.ComponentCount = 10
	o.ClusterCount = 2
	o.MinComponentsPerCluster = 3
	o.MinPowerSourcesPerCluster = 2
	o.MaxPowerSourcesPerCluster = 2
	o.MaxParallel = 1
	o.Connections = 0
	o.Seed, o.SeedSet = 52, true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(52)), top, bottom)
	if err != nil {
		t.Fatalf("build board with two feeds per cluster: %v", err)
	}
	if len(plan.PowerTraces) != 4 || len(plan.PowerRoutes) != 4 {
		t.Fatalf("got %d power traces and %d routes, want 4 each", len(plan.PowerTraces), len(plan.PowerRoutes))
	}
	if len(plan.PowerCores) != 8 {
		t.Fatalf("got %d power cores, want 4 source cores and 4 selected component endpoints", len(plan.PowerCores))
	}
	powerCorePositions := make(map[hexgrid.GridPosition]bool, len(plan.PowerCores))
	for _, core := range plan.PowerCores {
		powerCorePositions[core.Position] = true
	}
	usedSources := make(map[hexgrid.GridPosition]bool)
	usedRouteCells := make(map[hexgrid.GridPosition]hexgrid.GridPosition)
	for _, cluster := range plan.Clusters {
		if len(cluster.PowerSources) != 2 || len(cluster.PowerPaths) != 2 || len(cluster.PowerTargets) != 2 {
			t.Fatalf("cluster %d has %d sources, %d targets, and %d paths, want 2 each",
				cluster.ID, len(cluster.PowerSources), len(cluster.PowerTargets), len(cluster.PowerPaths))
		}
		componentPositions := make(map[hexgrid.GridPosition]bool, len(cluster.Components))
		for _, component := range cluster.Components {
			componentPositions[component.Position] = true
		}
		seenTargets := make(map[hexgrid.GridPosition]bool)
		for i, source := range cluster.PowerSources {
			if usedSources[source.Position] {
				t.Errorf("source %v assigned to more than one feed", source.Position)
			}
			usedSources[source.Position] = true
			target := cluster.PowerTargets[i]
			if !componentPositions[target] {
				t.Errorf("cluster %d feed %d targets %v outside its component set", cluster.ID, i, target)
			}
			if !powerCorePositions[target] {
				t.Errorf("cluster %d feed %d destination %v has no accent power core", cluster.ID, i, target)
			}
			if seenTargets[target] {
				t.Errorf("cluster %d repeats component destination %v", cluster.ID, target)
			}
			seenTargets[target] = true
			path := cluster.PowerPaths[i]
			if len(path) < 2 || path[0] != source.Position || path[len(path)-1] != target {
				t.Errorf("cluster %d feed %d does not run from source %v to component %v", cluster.ID, i, source.Position, target)
				continue
			}
			for _, position := range path {
				if previousTarget, exists := usedRouteCells[position]; exists && (position != target || position != previousTarget) {
					t.Errorf("power routes share non-endpoint cell %v", position)
				}
				usedRouteCells[position] = target
			}
		}
		if cluster.PowerTargets[0] == cluster.Hub && cluster.PowerTargets[1] == cluster.Hub {
			t.Errorf("cluster %d routes every feed to its hub", cluster.ID)
		}
	}
	if _, _, _, err := Generate(o); err != nil {
		t.Fatalf("render board with multiple feeds per cluster: %v", err)
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
	usedTargets := make(map[hexgrid.GridPosition]bool, len(plan.PowerTraces))
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
		target := cluster.PowerTargets[0]
		componentPositions := make(map[hexgrid.GridPosition]bool, len(cluster.Components))
		for _, component := range cluster.Components {
			componentPositions[component.Position] = true
		}
		if !componentPositions[target] {
			t.Errorf("power feed target %v is not a component in cluster %d", target, cluster.ID)
		}
		if usedTargets[target] {
			t.Errorf("multiple clusters target the same component %v", target)
		}
		usedTargets[target] = true

		path := plan.PowerRoutes[i]
		if len(path) < 2 {
			t.Errorf("power feed %d has an incomplete route", i)
			continue
		}
		if path[0] != source {
			t.Errorf("power feed %d starts at %v, want source %v", i, path[0], source)
		}
		if path[len(path)-1] != target {
			t.Errorf("power feed %d ends at %v, want component %v", i, path[len(path)-1], target)
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
		if trace.Start != source || len(trace.Path) == 0 || trace.Path[len(trace.Path)-1] != target {
			t.Errorf("rendered power trace %d does not match its source and component target", i)
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
		sources, err := choosePowerSourceAssignments(top, bottom, 6, rand.New(rand.NewSource(seed)))
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
		if got, want := len(plan.Components)-o.ComponentCount, len(top)+len(bottom); got != want {
			t.Fatalf("got %d rendered source components, want all %d configured contacts", got, want)
		}
		for _, component := range plan.Components[o.ComponentCount:] {
			if component.Style != test.want {
				t.Fatalf("source component style = %v, want %v", component.Style, test.want)
			}
		}
	}
}

func TestPowerSourceRowsApplyPhysicalSpacing(t *testing.T) {
	for _, test := range []struct {
		spacing PowerSourceSpacing
		step    int
	}{
		{PowerSourceSpacingAdjacent, 1}, {PowerSourceSpacingSeparated, 2},
	} {
		o := DefaultOptions()
		o.TopPowerSourceCount, o.BottomPowerSourceCount = 4, 0
		o.PowerSourceSpacing = test.spacing
		top, _, err := powerSourcePositions(o)
		if err != nil {
			t.Fatal(err)
		}
		if len(top) != o.TopPowerSourceCount {
			t.Fatalf("got %d contacts, want %d", len(top), o.TopPowerSourceCount)
		}
		for i := 1; i < len(top); i++ {
			if got := top[i].Q - top[i-1].Q; got != test.step {
				t.Fatalf("%q spacing Q step = %d, want %d", test.spacing, got, test.step)
			}
		}
	}
}

func TestEveryConfiguredPowerSourceContactIsRendered(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount, o.BottomPowerSourceCount = 10, 9
	o.Seed, o.SeedSet = 818, true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	drawn := make(map[hexgrid.GridPosition]hexgrid.GridComponent, len(plan.Components))
	for _, component := range plan.Components {
		drawn[component.Position] = component
	}
	active := make(map[hexgrid.GridPosition]bool, len(plan.PowerTraces))
	for _, cluster := range plan.Clusters {
		for _, source := range cluster.PowerSources {
			active[source.Position] = true
		}
	}
	cores := make(map[hexgrid.GridPosition]bool, len(plan.PowerCores))
	for _, component := range plan.PowerCores {
		cores[component.Position] = true
	}
	all := append(append([]hexgrid.GridPosition(nil), top...), bottom...)
	for _, source := range all {
		component, ok := drawn[source]
		if !ok {
			t.Errorf("configured physical contact %v was not rendered", source)
			continue
		}
		if component.Style != powerSourceComponentStyle(o.PowerSourceStyle) {
			t.Errorf("wrong style at %v", source)
		}
		if active[source] != cores[source] {
			t.Errorf("source %v active=%v, power core=%v", source, active[source], cores[source])
		}
	}
	if got, want := len(plan.Components), o.ComponentCount+len(all); got != want {
		t.Fatalf("rendered %d components, want %d", got, want)
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

func TestPowerSourceRowsAreReservedForPowerSources(t *testing.T) {
	o := DefaultOptions()
	o.Seed, o.SeedSet = 726, true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	rows := powerSourceRowIDs(top, bottom)
	for _, cluster := range plan.Clusters {
		for _, component := range cluster.Components {
			if rows[component.Position.R] {
				t.Errorf("ordinary component %v occupies a power-source row", component.Position)
			}
		}
		for _, path := range cluster.PowerPaths {
			for i, position := range path {
				if i > 0 && rows[position.R] {
					t.Errorf("power route re-enters a power-source row at %v", position)
				}
			}
		}
	}
	allowed := routingCellsWithinPowerSourceCorridor(
		routingCellsWithoutPowerSourceRows(o, top, bottom), top, bottom, o,
	)
	for position := range allowed {
		if rows[position.R] {
			t.Errorf("ordinary route map permits source row cell %v", position)
		}
		y := hexgrid.GridPositionToPoint(position, o.Width, o.Height, o.Pitch, "pointy").Y
		topY := hexgrid.GridPositionToPoint(top[0], o.Width, o.Height, o.Pitch, "pointy").Y
		bottomY := hexgrid.GridPositionToPoint(bottom[0], o.Width, o.Height, o.Pitch, "pointy").Y
		if y <= topY || y >= bottomY {
			t.Errorf("ordinary route map permits cell %v outside source-row corridor", position)
		}
	}
}

func TestPowerSourceSpanConstrainsAllRoutes(t *testing.T) {
	o := DefaultOptions()
	o.ConstrainComponentsToPowerSourceSpan = true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(92)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	active := make([]powerSource, 0, len(plan.Clusters))
	for _, cluster := range plan.Clusters {
		active = append(active, powerSource{Position: cluster.PowerSource})
	}
	minimumX, maximumX := math.Inf(1), math.Inf(-1)
	for _, source := range active {
		x := hexgrid.GridPositionToPoint(source.Position, o.Width, o.Height, o.Pitch, "pointy").X
		minimumX, maximumX = math.Min(minimumX, x), math.Max(maximumX, x)
	}
	allowed := routingCellsWithinPowerSourceSpan(routingCellsWithinPowerSourceCorridor(routingCellsWithoutPowerSourceRows(o, top, bottom), top, bottom, o), active, o)
	powerAllowed := copyAllowedRoutes(allowed)
	for _, source := range active {
		powerAllowed[source.Position] = true
	}
	for position := range allowed {
		x := hexgrid.GridPositionToPoint(position, o.Width, o.Height, o.Pitch, "pointy").X
		if x < minimumX-1e-9 || x > maximumX+1e-9 {
			t.Errorf("route map includes %v outside [%f,%f]", position, minimumX, maximumX)
		}
	}
	for _, route := range plan.PowerRoutes {
		for _, position := range route {
			if !powerAllowed[position] {
				t.Errorf("power route uses %v outside constrained route map", position)
			}
		}
	}
}

func TestPowerSourceTrapezoidConstrainsLayoutAndRoutes(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount, o.BottomPowerSourceCount = 5, 9
	o.ConstrainLayoutToPowerSourceTrapezoid = true
	o.Seed, o.SeedSet = 473, true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := newPowerSourceTrapezoid(top, bottom, o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	for _, cluster := range plan.Clusters {
		for _, component := range cluster.Components {
			if !bounds.contains(component.Position, o) {
				t.Errorf("component %v outside trapezoid", component.Position)
			}
		}
		for _, path := range cluster.PowerPaths {
			for _, position := range path {
				if !bounds.contains(position, o) {
					t.Errorf("power route cell %v outside trapezoid", position)
				}
			}
		}
	}
	cells := hexgrid.GridCells(o.Width, o.Height, o.Pitch, "pointy", math.Max(o.ComponentRadius+o.ComponentStrokeWidth/2, o.ConnectorInset))
	globalLeft, globalRight := math.Min(bounds.topLeft, bounds.bottomLeft), math.Max(bounds.topRight, bounds.bottomRight)
	foundOutsideCorner := false
	for _, cell := range cells {
		position := hexgrid.GridPosition{Q: cell.Q, R: cell.R}
		if cell.Center.Y > bounds.topY && cell.Center.Y < bounds.bottomY &&
			cell.Center.X >= globalLeft-1e-9 && cell.Center.X <= globalRight+1e-9 &&
			!bounds.contains(position, o) {
			foundOutsideCorner = true
			break
		}
	}
	if !foundOutsideCorner {
		t.Fatal("test geometry has no inner rectangular corner cells outside the trapezoid")
	}
	filtered := routingCellsWithinPowerSourceTrapezoid(routingCellsWithinPowerSourceCorridor(routingCellsWithoutPowerSourceRows(o, top, bottom), top, bottom, o), bounds, o)
	for position := range filtered {
		if !bounds.contains(position, o) {
			t.Errorf("trapezoid route mask includes %v outside bounds", position)
		}
	}
}

func TestPowerSourceTrapezoidRequiresBothRows(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount = 0
	o.ConstrainLayoutToPowerSourceTrapezoid = true
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected missing top source row to be rejected")
	}
}

func TestPowerSourceTrapezoidConstraintDefaultsOff(t *testing.T) {
	if DefaultOptions().ConstrainLayoutToPowerSourceTrapezoid {
		t.Fatal("trapezoid constraint must default to disabled")
	}
}

func TestInterClusterTracesDefaultDisabled(t *testing.T) {
	if DefaultOptions().InterClusterTraces {
		t.Fatal("inter-cluster traces should default to disabled")
	}
}

func TestAddInterClusterConnectionsIsRandomAndUsesDifferentClusters(t *testing.T) {
	o := DefaultOptions()
	clusters := []clusterPlan{
		{ID: 0, Components: []placedComponent{{Position: hexgrid.GridPosition{Q: 0, R: 0}}}},
		{ID: 1, Components: []placedComponent{{Position: hexgrid.GridPosition{Q: 5, R: 0}}}},
	}
	allowed := make(map[hexgrid.GridPosition]bool, 6)
	for q := 0; q <= 5; q++ {
		allowed[hexgrid.GridPosition{Q: q, R: 0}] = true
	}
	blocked := map[hexgrid.GridPosition]bool{
		{Q: 0, R: 0}: true,
		{Q: 5, R: 0}: true,
	}

	sawEmpty, sawTrace := false, false
	for seed := int64(0); seed < 64; seed++ {
		reserved := make(map[hexgrid.GridPosition]bool)
		traces := addInterClusterConnections(
			o, rand.New(rand.NewSource(seed)), clusters, allowed, blocked, reserved,
		)
		if len(traces) == 0 {
			sawEmpty = true
			continue
		}
		if len(traces) != 1 {
			t.Fatalf("two clusters produced %d inter-cluster traces, want at most one", len(traces))
		}
		trace := traces[0]
		if trace.Start != clusters[0].Components[0].Position ||
			len(trace.Path) == 0 || trace.Path[len(trace.Path)-1] != clusters[1].Components[0].Position {
			t.Fatalf("trace does not connect the two distinct clusters: %#v", trace)
		}
		if trace.Lines < 1 || trace.Lines > o.MaxParallel {
			t.Errorf("trace line count %d outside [1, %d]", trace.Lines, o.MaxParallel)
		}
		for q := 1; q < 5; q++ {
			if !reserved[hexgrid.GridPosition{Q: q, R: 0}] {
				t.Errorf("route interior cell %v was not reserved", hexgrid.GridPosition{Q: q, R: 0})
			}
		}
		sawTrace = true
	}
	if !sawEmpty || !sawTrace {
		t.Fatalf("random generation did not demonstrate both optional outcomes: empty=%v trace=%v", sawEmpty, sawTrace)
	}
}

func TestRenderBoardKeepsInterClusterTracesInAccentLayer(t *testing.T) {
	o := DefaultOptions()
	plan := boardPlan{
		InterclusterComponents: []hexgrid.GridComponent{
			{Position: hexgrid.GridPosition{Q: 0, R: 0}, Style: hexgrid.ComponentStyleSingle, RadiusScale: powerCoreRadiusScale},
			{Position: hexgrid.GridPosition{Q: 2, R: 0}, Style: hexgrid.ComponentStyleSingle, RadiusScale: powerCoreRadiusScale},
		},
		InterClusterTraces: []hexgrid.GridTrace{{
			Start:        hexgrid.GridPosition{Q: 0, R: 0},
			Path:         []hexgrid.GridPosition{{Q: 2, R: 0}},
			Lines:        2,
			HideTerminal: true,
		}},
	}
	svg, err := renderBoard(o, plan)
	if err != nil {
		t.Fatalf("render inter-cluster trace layer: %v", err)
	}
	if !strings.Contains(svg, `id="inter-cluster-traces"`) {
		t.Fatalf("SVG is missing the separately namespaced inter-cluster trace layer")
	}
	if !strings.Contains(svg, `id="inter-cluster-components"`) {
		t.Fatalf("SVG is missing the separately namespaced inter-cluster endpoint components")
	}
	if !strings.Contains(svg, o.AccentColor) {
		t.Fatalf("inter-cluster layer does not use accent color %q", o.AccentColor)
	}
}

func TestInterClusterTraceSeedRendersEndpointComponents(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 1365413531639368899
	o.SeedSet = true
	o.InterClusterTraces = true

	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.InterClusterTraces) == 0 {
		t.Fatal("regression seed did not produce an inter-cluster trace")
	}

	components := make(map[hexgrid.GridPosition]hexgrid.GridComponent, len(plan.InterclusterComponents))
	for _, component := range plan.InterclusterComponents {
		if _, exists := components[component.Position]; exists {
			t.Errorf("duplicate inter-cluster endpoint component at %v", component.Position)
		}
		components[component.Position] = component
		if component.Style != hexgrid.ComponentStyleSingle || component.RadiusScale != powerCoreRadiusScale {
			t.Errorf("endpoint component at %v has style/radius (%v, %v), want single/%v",
				component.Position, component.Style, component.RadiusScale, powerCoreRadiusScale)
		}
	}
	for i, trace := range plan.InterClusterTraces {
		if _, ok := components[trace.Start]; !ok {
			t.Errorf("inter-cluster trace %d starts at %v without an endpoint component", i, trace.Start)
		}
		if len(trace.Path) == 0 {
			t.Errorf("inter-cluster trace %d has an empty path", i)
			continue
		}
		endpoint := trace.Path[len(trace.Path)-1]
		if _, ok := components[endpoint]; !ok {
			t.Errorf("inter-cluster trace %d ends at %v without an endpoint component", i, endpoint)
		}
	}

	if _, err := renderBoard(o, plan); err != nil {
		t.Fatalf("render regression seed: %v", err)
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
			o.PowerSourceSpacing = PowerSourceSpacingAdjacent
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
			o.TopPowerSourceCount = 10
			o.BottomPowerSourceCount = 10
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
