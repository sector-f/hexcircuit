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
	"github.com/sector-f/hexgrid/geometry"
	"github.com/sector-f/hexgrid/internal/gridgeom"
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

func TestBuildBoardUsesDoubleStyleWhenPowerCoreNeedsTheInnerRing(t *testing.T) {
	o := DefaultOptions()
	o.ComponentCount = 1
	o.ClusterCount = 1
	o.TopPowerSourceCount = 1
	o.BottomPowerSourceCount = 0
	o.Connections = 0
	o.MaxParallel = 1

	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(42)), top, bottom)
	if err != nil {
		t.Fatalf("build single-component powered cluster: %v", err)
	}
	if len(plan.Clusters) != 1 || len(plan.Clusters[0].PowerTargets) != 1 {
		t.Fatalf("got %d clusters and %d power targets, want one each",
			len(plan.Clusters), len(plan.Clusters[0].PowerTargets))
	}

	target := plan.Clusters[0].PowerTargets[0]
	if len(plan.PowerCores) != 2 {
		t.Fatalf("got %d power cores, want source and target cores", len(plan.PowerCores))
	}
	foundPowerCore := false
	for _, core := range plan.PowerCores {
		if core.Position == target {
			foundPowerCore = true
			break
		}
	}
	if !foundPowerCore {
		t.Fatalf("assigned target %v has no rendered power core", target)
	}

	if got := plan.Clusters[0].Components[0].Style; got != hexgrid.ComponentStyleDouble {
		t.Fatalf("cluster component style = %q, want double to clear the power core", got)
	}
	foundBaseComponent := false
	for _, component := range plan.Components {
		if component.Position != target {
			continue
		}
		foundBaseComponent = true
		if component.Style != hexgrid.ComponentStyleDouble {
			t.Fatalf("base component style at powered target %v = %q, want double", target, component.Style)
		}
		break
	}
	if !foundBaseComponent {
		t.Fatalf("powered target %v is missing from the base component layer", target)
	}
}

func TestAccentCoreStyleUsesGeometricClearance(t *testing.T) {
	o := DefaultOptions()
	tests := []struct {
		name                 string
		style                hexgrid.ComponentStyle
		baseRadiusScale      float64
		accentRadiusScale    float64
		want                 hexgrid.ComponentStyle
	}{
		{
			name: "preserves triple when both nested rings clear the accent core",
			style: hexgrid.ComponentStyleTriple, baseRadiusScale: 1, accentRadiusScale: 0.20,
			want: hexgrid.ComponentStyleTriple,
		},
		{
			name: "downgrades triple only when the innermost ring is too close",
			style: hexgrid.ComponentStyleTriple, baseRadiusScale: 1, accentRadiusScale: 0.22,
			want: hexgrid.ComponentStyleDouble,
		},
		{
			name: "downgrades double when the middle ring is too close",
			style: hexgrid.ComponentStyleDouble, baseRadiusScale: 0.72, accentRadiusScale: 0.36,
			want: hexgrid.ComponentStyleSingle,
		},
		{
			name: "preserves double when the middle ring has enough gap",
			style: hexgrid.ComponentStyleDouble, baseRadiusScale: 0.90, accentRadiusScale: 0.36,
			want: hexgrid.ComponentStyleDouble,
		},
		{
			name: "single has no nested outline to clear",
			style: hexgrid.ComponentStyleSingle, baseRadiusScale: 0.72, accentRadiusScale: 0.50,
			want: hexgrid.ComponentStyleSingle,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := accentCoreStyleWithClearance(o, test.style, test.baseRadiusScale, test.accentRadiusScale); got != test.want {
				t.Fatalf("style = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAccentCoreClearanceAccountsForStrokeWidths(t *testing.T) {
	o := DefaultOptions()
	clearance := accentCoreInnerHexClearance(o, 0.72, 0.36, o.ComponentMiddleHexagonScale)
	if clearance <= 0 {
		t.Fatalf("expected a positive geometric gap before applying the visible-gap threshold, got %.6f", clearance)
	}
	if clearance >= minimumAccentHexGap {
		t.Fatalf("clearance %.6f should be below the required %.6f-inch gap", clearance, minimumAccentHexGap)
	}

	o.ComponentStrokeWidth = 0
	withoutStroke := accentCoreInnerHexClearance(o, 0.72, 0.36, o.ComponentMiddleHexagonScale)
	if withoutStroke <= clearance {
		t.Fatalf("removing stroke widths did not increase clearance: got %.6f, with strokes %.6f", withoutStroke, clearance)
	}
}

func TestAdjustBaseComponentStylesForPowerAndInterclusterCores(t *testing.T) {
	o := DefaultOptions()
	positions := []hexgrid.GridPosition{
		{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0},
		{Q: 3, R: 0}, {Q: 4, R: 0}, {Q: 5, R: 0},
	}
	o.PowerSourceComponentRadius = 0.12
	sourceRadiusScale := powerSourceComponentRadiusScale(o)
	baseComponents := []hexgrid.GridComponent{
		{Position: positions[0], Style: hexgrid.ComponentStyleTriple, RadiusScale: 1},
		{Position: positions[1], Style: hexgrid.ComponentStyleTriple, RadiusScale: 1},
		{Position: positions[2], Style: hexgrid.ComponentStyleDouble, RadiusScale: 0.72},
		{Position: positions[3], Style: hexgrid.ComponentStyleDouble, RadiusScale: 0.90},
		{Position: positions[4], Style: hexgrid.ComponentStyleDouble, RadiusScale: sourceRadiusScale},
		{Position: positions[5], Style: hexgrid.ComponentStyleTriple, RadiusScale: 1},
	}
	clusters := []clusterPlan{
		{ID: 0, Components: []placedComponent{
			{Position: positions[0], Style: hexgrid.ComponentStyleTriple, RadiusScale: 1},
			{Position: positions[1], Style: hexgrid.ComponentStyleTriple, RadiusScale: 1},
			{Position: positions[2], Style: hexgrid.ComponentStyleDouble, RadiusScale: 0.72},
			{Position: positions[5], Style: hexgrid.ComponentStyleTriple, RadiusScale: 1},
		}},
		{ID: 1, Components: []placedComponent{
			{Position: positions[3], Style: hexgrid.ComponentStyleDouble, RadiusScale: 0.90},
		}},
	}
	powerCores := []hexgrid.GridComponent{
		{Position: positions[2], Style: hexgrid.ComponentStyleSingle, RadiusScale: 0.36},
		{Position: positions[4], Style: hexgrid.ComponentStyleSingle, RadiusScale: sourceRadiusScale * 0.36},
	}
	interclusterCores := []hexgrid.GridComponent{
		{Position: positions[0], Style: hexgrid.ComponentStyleSingle, RadiusScale: 0.20},
		{Position: positions[1], Style: hexgrid.ComponentStyleSingle, RadiusScale: 0.22},
		{Position: positions[3], Style: hexgrid.ComponentStyleSingle, RadiusScale: 0.36},
	}

	adjustBaseComponentStylesForAccentCores(o, baseComponents, clusters, powerCores, interclusterCores)

	want := []hexgrid.ComponentStyle{
		hexgrid.ComponentStyleTriple,
		hexgrid.ComponentStyleDouble,
		hexgrid.ComponentStyleSingle,
		hexgrid.ComponentStyleDouble,
		hexgrid.ComponentStyleSingle,
		hexgrid.ComponentStyleTriple,
	}
	wantByPosition := make(map[hexgrid.GridPosition]hexgrid.ComponentStyle, len(positions))
	for i := range want {
		wantByPosition[positions[i]] = want[i]
		if got := baseComponents[i].Style; got != want[i] {
			t.Errorf("base component %d style = %q, want %q", i, got, want[i])
		}
	}
	for clusterIndex := range clusters {
		for componentIndex := range clusters[clusterIndex].Components {
			component := clusters[clusterIndex].Components[componentIndex]
			if got, want := component.Style, wantByPosition[component.Position]; got != want {
				t.Errorf("cluster %d component at %v has style %q, want %q",
					clusterIndex, component.Position, got, want)
			}
		}
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
		sourcePoint := gridgeom.PositionToPoint(cluster.PowerSource.Q, cluster.PowerSource.R, o.Width, o.Height, o.Pitch, "pointy")
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


func TestNormalizeOptionsValidatesComponentRadiusBounds(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Options)
	}{
		{"negative minimum", func(o *Options) { o.ComponentMinRadius = -0.1 }},
		{"negative maximum", func(o *Options) { o.ComponentMaxRadius = -0.1 }},
		{"minimum exceeds maximum", func(o *Options) { o.ComponentMinRadius = 0.25; o.ComponentMaxRadius = 0.20 }},
		{"infinite maximum", func(o *Options) { o.ComponentMaxRadius = math.Inf(1) }},
		{"NaN minimum", func(o *Options) { o.ComponentMinRadius = math.NaN() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := DefaultOptions()
			test.edit(&o)
			if _, err := normalizeOptions(o); err == nil {
				t.Fatal("expected invalid component-radius bounds to be rejected")
			}
		})
	}
}

func TestUnsetComponentRadiusBoundsPreserveLegacyClusterSizes(t *testing.T) {
	o := DefaultOptions()
	if o.ComponentMinRadius != 0 || o.ComponentMaxRadius != 0 {
		t.Fatalf("default cluster radius bounds = (%g, %g), want both unset", o.ComponentMinRadius, o.ComponentMaxRadius)
	}
	if got := clusterComponentRadiusScale(o, rand.New(rand.NewSource(1)), true); got != 1 {
		t.Fatalf("legacy first component radius scale = %g, want 1", got)
	}
	got := clusterComponentRadiusScale(o, rand.New(rand.NewSource(1)), false)
	if got < 0.72 || got > 0.94 {
		t.Fatalf("legacy outer component radius scale = %g, want range [0.72, 0.94]", got)
	}
}

func TestComponentRadiusBoundsFallBackToComponentRadius(t *testing.T) {
	o := DefaultOptions()
	o.ComponentMinRadius = 0
	o.ComponentMaxRadius = 0
	minimum, maximum := clusterComponentRadiusBounds(o)
	if minimum != o.ComponentRadius || maximum != o.ComponentRadius {
		t.Fatalf("zero bounds resolved to (%g, %g), want ComponentRadius %g", minimum, maximum, o.ComponentRadius)
	}

	o.ComponentMinRadius = 0.18
	minimum, maximum = clusterComponentRadiusBounds(o)
	if minimum != 0.18 || maximum != o.ComponentRadius {
		t.Fatalf("partial bounds resolved to (%g, %g), want (0.18, %g)", minimum, o.ComponentRadius, o.ComponentRadius)
	}
}

func TestClusterComponentsUseConfiguredRandomRadiusRange(t *testing.T) {
	o := DefaultOptions()
	o.ComponentMinRadius = 0.18
	o.ComponentMaxRadius = 0.24
	o.Seed = 912
	o.SeedSet = true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}

	seenRadii := make(map[float64]bool)
	for _, cluster := range plan.Clusters {
		for _, component := range cluster.Components {
			radius := component.RadiusScale * o.ComponentRadius
			if radius < o.ComponentMinRadius-1e-9 || radius > o.ComponentMaxRadius+1e-9 {
				t.Errorf("cluster component at %v has radius %.6g outside [%.6g, %.6g]",
					component.Position, radius, o.ComponentMinRadius, o.ComponentMaxRadius)
			}
			seenRadii[radius] = true
		}
	}
	if len(seenRadii) < 2 {
		t.Fatalf("cluster components did not receive different radii: %v", seenRadii)
	}
}


func TestClusterPowerCoreRadiusFollowsHostWhenVariableSizesAreEnabled(t *testing.T) {
	o := DefaultOptions()
	if got := clusterPowerCoreRadiusScale(o, 0.8); got != powerCoreRadiusScale {
		t.Fatalf("legacy power core radius scale = %g, want %g", got, powerCoreRadiusScale)
	}
	o.ComponentMinRadius = 0.18
	o.ComponentMaxRadius = 0.24
	hostScale := 0.18 / o.ComponentRadius
	if got, want := clusterPowerCoreRadiusScale(o, hostScale), hostScale*powerCoreRadiusScale; got != want {
		t.Fatalf("variable-size power core scale = %g, want %g", got, want)
	}
}


func TestDefaultPowerSourceComponentRadiusMatchesComponentRadius(t *testing.T) {
	o := DefaultOptions()
	if o.PowerSourceComponentRadius != o.ComponentRadius {
		t.Fatalf("default power-source component radius = %g, want ComponentRadius %g",
			o.PowerSourceComponentRadius, o.ComponentRadius)
	}
}

func TestPowerSourceComponentsKeepFixedRadius(t *testing.T) {
	o := DefaultOptions()
	o.PowerSourceComponentRadius = 0.20
	o.ComponentMinRadius = 0.18
	o.ComponentMaxRadius = 0.24
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(912)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	wantScale := powerSourceComponentRadiusScale(o)
	sourcePositions := make(map[hexgrid.GridPosition]bool, len(top)+len(bottom))
	for _, position := range append(top, bottom...) {
		sourcePositions[position] = true
	}
	for _, component := range plan.Components[o.ComponentCount:] {
		if component.RadiusScale != wantScale {
			t.Errorf("power-source component at %v has radius scale %g; want %g for fixed power-source radius %g",
				component.Position, component.RadiusScale, wantScale, o.PowerSourceComponentRadius)
		}
	}
	for _, core := range plan.PowerCores {
		if sourcePositions[core.Position] {
			wantCoreScale := wantScale * powerCoreRadiusScale
			if core.RadiusScale != wantCoreScale {
				t.Errorf("power-source core at %v has radius scale %g; want %g", core.Position, core.RadiusScale, wantCoreScale)
			}
		}
	}
}

func TestNormalizeOptionsRejectsOverlappingComponentSizes(t *testing.T) {
	o := DefaultOptions()
	o.ComponentMinRadius = 0.26
	o.ComponentMaxRadius = 0.30
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected pitch that permits component overlap to be rejected")
	}

	// Keep the default top and bottom power-source rows enabled. Increasing
	// pitch reduces the number of cells across the canvas, so widen the canvas
	// slightly to preserve room for the existing ten-contact separated rows.
	o.Width = 10.4
	o.Pitch = 0.54
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("increased pitch and canvas width should accommodate component radii while retaining power-source rows: %v", err)
	}
}

func TestAdjacentPowerSourcesDoNotOverlapAtDefaultPitch(t *testing.T) {
	o := DefaultOptions()
	o.PowerSourceSpacing = PowerSourceSpacingAdjacent
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("adjacent pointy-top contacts should fit at default pitch: %v", err)
	}
	if _, _, err := powerSourcePositions(o); err != nil {
		t.Fatalf("adjacent source contacts overlap: %v", err)
	}
}



func TestNormalizeOptionsChecksSourceToClusterClearance(t *testing.T) {
	o := DefaultOptions()
	o.ComponentRadius = 1
	o.PowerSourceComponentRadius = 1
	o.ComponentMinRadius = 0.20
	o.ComponentMaxRadius = 0.25
	o.MinPowerSourcesPerCluster = 0
	o.MaxPowerSourcesPerCluster = 0
	o.TopPowerSourceCount = 1
	o.BottomPowerSourceCount = 0
	o.Pitch = 0.70
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected large fixed power contacts to require more vertical clearance from cluster components")
	}
	o.Pitch = 0.80
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("larger pitch should provide source-to-cluster clearance: %v", err)
	}
}

func TestGeneratedComponentsDoNotOverlap(t *testing.T) {
	configurations := []struct {
		name string
		edit func(*Options)
	}{
		{name: "legacy sizes"},
		{name: "randomized range", edit: func(o *Options) {
			o.ComponentMinRadius = 0.20
			o.ComponentMaxRadius = 0.30
			o.Pitch = 0.54
			o.Width = 10.4
		}},
	}
	for _, configuration := range configurations {
		t.Run(configuration.name, func(t *testing.T) {
			o := DefaultOptions()
			o.Seed = 912
			o.SeedSet = true
			if configuration.edit != nil {
				configuration.edit(&o)
			}
			top, bottom, err := powerSourcePositions(o)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
			if err != nil {
				t.Fatal(err)
			}

			radius := func(component hexgrid.GridComponent) float64 {
				if component.RadiusScale == 0 {
					return o.ComponentRadius
				}
				return o.ComponentRadius * component.RadiusScale
			}
			for i, a := range plan.Components {
				pointA := gridgeom.PositionToPoint(a.Position.Q, a.Position.R, o.Width, o.Height, o.Pitch, "pointy")
				for _, b := range plan.Components[i+1:] {
					pointB := gridgeom.PositionToPoint(b.Position.Q, b.Position.R, o.Width, o.Height, o.Pitch, "pointy")
					if regularHexagonsOverlap(
						pointA.X, pointA.Y, radius(a),
						pointB.X, pointB.Y, radius(b),
						o.ComponentStrokeWidth,
					) {
						t.Errorf("components at %v and %v overlap", a.Position, b.Position)
					}
				}
			}
		})
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


func TestPowerSourceContactsRejectOverlappingGeometry(t *testing.T) {
	o := DefaultOptions()
	o.TopPowerSourceCount = 3
	o.BottomPowerSourceCount = 0
	o.PowerSourceSpacing = PowerSourceSpacingAdjacent
	o.Pitch = 0.40
	if _, _, err := powerSourcePositions(o); err == nil {
		t.Fatal("expected adjacent power-source contacts with overlapping outlines to be rejected")
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
			x := gridgeom.PositionToPoint(cluster.PowerSource.Q, cluster.PowerSource.R, o.Width, o.Height, o.Pitch, "pointy").X
			minimumX = math.Min(minimumX, x)
			maximumX = math.Max(maximumX, x)
		}
		for _, cluster := range plan.Clusters {
			for _, component := range cluster.Components {
				x := gridgeom.PositionToPoint(component.Position.Q, component.Position.R, o.Width, o.Height, o.Pitch, "pointy").X
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
		y := gridgeom.PositionToPoint(position.Q, position.R, o.Width, o.Height, o.Pitch, "pointy").Y
		topY := gridgeom.PositionToPoint(top[0].Q, top[0].R, o.Width, o.Height, o.Pitch, "pointy").Y
		bottomY := gridgeom.PositionToPoint(bottom[0].Q, bottom[0].R, o.Width, o.Height, o.Pitch, "pointy").Y
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
		x := gridgeom.PositionToPoint(source.Position.Q, source.Position.R, o.Width, o.Height, o.Pitch, "pointy").X
		minimumX, maximumX = math.Min(minimumX, x), math.Max(maximumX, x)
	}
	allowed := routingCellsWithinPowerSourceSpan(routingCellsWithinPowerSourceCorridor(routingCellsWithoutPowerSourceRows(o, top, bottom), top, bottom, o), active, o)
	powerAllowed := copyAllowedRoutes(allowed)
	for _, source := range active {
		powerAllowed[source.Position] = true
	}
	for position := range allowed {
		x := gridgeom.PositionToPoint(position.Q, position.R, o.Width, o.Height, o.Pitch, "pointy").X
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
	cells := gridgeom.Cells(o.Width, o.Height, o.Pitch, "pointy", math.Max(o.PowerSourceComponentRadius+o.ComponentStrokeWidth/2, o.ConnectorInset))
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

	expectedRadiusScales := make(map[hexgrid.GridPosition]float64)
	for _, cluster := range plan.Clusters {
		for _, component := range cluster.Components {
			expectedRadiusScales[component.Position] = clusterPowerCoreRadiusScale(o, component.RadiusScale)
		}
	}
	accentCoreScales := make(map[hexgrid.GridPosition]float64)
	for _, core := range append(append([]hexgrid.GridComponent(nil), plan.PowerCores...), plan.InterclusterComponents...) {
		radiusScale := core.RadiusScale
		if radiusScale == 0 {
			radiusScale = 1
		}
		if radiusScale > accentCoreScales[core.Position] {
			accentCoreScales[core.Position] = radiusScale
		}
	}
	baseComponents := make(map[hexgrid.GridPosition]hexgrid.GridComponent, len(plan.Components))
	for _, component := range plan.Components {
		baseComponents[component.Position] = component
	}
	for position, coreScale := range accentCoreScales {
		base, ok := baseComponents[position]
		if !ok {
			t.Errorf("accent core at %v has no base-layer component", position)
			continue
		}
		if want := accentCoreStyleWithClearance(o, base.Style, base.RadiusScale, coreScale); base.Style != want {
			t.Errorf("base component at %v has style %q, which does not leave the required accent-core clearance (want at most %q)",
				position, base.Style, want)
		}
	}
	for _, cluster := range plan.Clusters {
		for _, component := range cluster.Components {
			if base, ok := baseComponents[component.Position]; ok && component.Style != base.Style {
				t.Errorf("cluster component at %v has style %q, base layer has %q",
					component.Position, component.Style, base.Style)
			}
		}
	}

	components := make(map[hexgrid.GridPosition]hexgrid.GridComponent, len(plan.InterclusterComponents))
	for _, component := range plan.InterclusterComponents {
		if _, exists := components[component.Position]; exists {
			t.Errorf("duplicate inter-cluster endpoint component at %v", component.Position)
		}
		components[component.Position] = component
		if component.Style != hexgrid.ComponentStyleSingle || component.RadiusScale != expectedRadiusScales[component.Position] {
			t.Errorf("endpoint component at %v has style/radius (%v, %v), want single/%v",
				component.Position, component.Style, component.RadiusScale, expectedRadiusScales[component.Position])
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

func TestDecorativeTracesDoNotChangePrimaryRoutes(t *testing.T) {
	o := DefaultOptions()
	o.Seed = 1365413531639368899
	o.SeedSet = true
	o.InterClusterTraces = true
	o.DecorativeTraceCount = 0

	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	withoutDecorative, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatalf("build board without decorative traces: %v", err)
	}

	o.DecorativeTraceCount = 6
	withDecorative, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatalf("build board with decorative traces: %v", err)
	}
	if len(withDecorative.DecorativeTraces) == 0 {
		t.Fatal("decorative-trace generation did not find any nearby existing routes")
	}
	if !reflect.DeepEqual(withoutDecorative.PowerRoutes, withDecorative.PowerRoutes) {
		t.Fatal("decorative traces changed the existing power routes")
	}
	if !reflect.DeepEqual(withoutDecorative.PowerTraces, withDecorative.PowerTraces) {
		t.Fatal("decorative traces changed the existing power bundles")
	}
	if !reflect.DeepEqual(withoutDecorative.InterClusterTraces, withDecorative.InterClusterTraces) {
		t.Fatal("decorative traces changed the existing inter-cluster routes")
	}
	if !reflect.DeepEqual(withoutDecorative.SignalTraces, withDecorative.SignalTraces) {
		t.Fatal("decorative traces changed the existing signal routes")
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


func TestChoosePowerTargetsPrefersLargestEligibleComponents(t *testing.T) {
	o := DefaultOptions()
	o.ComponentMinRadius = 0.04
	o.ComponentMaxRadius = 0.26
	o.TraceSpacing = 0.08
	components := []placedComponent{
		{Position: hexgrid.GridPosition{Q: 0, R: 0}, RadiusScale: 0.04 / o.ComponentRadius},
		{Position: hexgrid.GridPosition{Q: 1, R: 0}, RadiusScale: 0.14 / o.ComponentRadius},
		{Position: hexgrid.GridPosition{Q: 2, R: 0}, RadiusScale: 0.26 / o.ComponentRadius},
	}
	got := choosePowerTargets(components, 2, o, rand.New(rand.NewSource(7)))
	want := []hexgrid.GridPosition{{Q: 2, R: 0}, {Q: 1, R: 0}}
	if len(got) != len(want) {
		t.Fatalf("got %d destinations, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("destination %d = %v, want larger viable component %v", i, got[i], want[i])
		}
	}
	if got := powerFeedMaxParallel(o, components[0].RadiusScale); got != 0 {
		t.Fatalf("smallest component supports %d tracks, want 0", got)
	}
	if got := powerFeedMaxParallel(o, components[1].RadiusScale); got != 1 {
		t.Fatalf("medium component supports %d tracks, want 1", got)
	}
	if got := powerFeedMaxParallel(o, components[2].RadiusScale); got != 2 {
		t.Fatalf("largest component supports %d tracks, want 2", got)
	}
}



func TestPowerFeedLineCountsRespectEndpointCoreCapacity(t *testing.T) {
	o := DefaultOptions()
	o.ComponentMinRadius = 0.18
	o.ComponentMaxRadius = 0.24
	o.TraceSpacing = 0.08
	o.Seed = 912
	o.SeedSet = true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatalf("build board with adaptive power-feed widths: %v", err)
	}

	traceIndex := 0
	for _, cluster := range plan.Clusters {
		radiusScales := make(map[hexgrid.GridPosition]float64, len(cluster.Components))
		for _, component := range cluster.Components {
			radiusScales[component.Position] = component.RadiusScale
		}
		for _, target := range cluster.PowerTargets {
			if traceIndex >= len(plan.PowerTraces) {
				t.Fatal("missing power trace for assigned power target")
			}
			trace := plan.PowerTraces[traceIndex]
			traceIndex++
			maximum := powerFeedMaxParallel(o, radiusScales[target])
			if trace.Lines < 1 || trace.Lines > maximum {
				t.Errorf("power feed to %v uses %d tracks; endpoint cores support 1..%d",
					target, trace.Lines, maximum)
			}
		}
	}
	if traceIndex != len(plan.PowerTraces) {
		t.Fatalf("checked %d traces, plan contains %d", traceIndex, len(plan.PowerTraces))
	}
}

func TestChoosePowerTargetsDoesNotReuseSmallEligibleSet(t *testing.T) {
	o := DefaultOptions()
	o.ComponentMinRadius = 0.04
	o.ComponentMaxRadius = 0.26
	o.TraceSpacing = 0.08
	components := []placedComponent{
		{Position: hexgrid.GridPosition{Q: 0, R: 0}, RadiusScale: 0.04 / o.ComponentRadius},
		{Position: hexgrid.GridPosition{Q: 1, R: 0}, RadiusScale: 0.26 / o.ComponentRadius},
		{Position: hexgrid.GridPosition{Q: 2, R: 0}, RadiusScale: 0.04 / o.ComponentRadius},
	}
	if targets := choosePowerTargets(components, 2, o, rand.New(rand.NewSource(9))); len(targets) != 0 {
		t.Fatalf("got %d targets with only one viable destination, want no assignment", len(targets))
	}
}

func TestNormalizeOptionsRejectsOnlyWhenNoSinglePowerTrackFits(t *testing.T) {
	o := DefaultOptions()
	o.TraceSpacing = 0.20
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("one-track fallback should remain valid: %v", err)
	}
	o.PowerSourceComponentRadius = 0.03
	if _, err := normalizeOptions(o); err == nil {
		t.Fatal("expected an error when even one track cannot fit in a source core")
	}
}

func TestNormalizeOptionsAllowsPowerBundlesToShrinkToFit(t *testing.T) {
	o := DefaultOptions()
	o.TraceSpacing = 0.08
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("three-track requests should shrink to the two tracks that fit: %v", err)
	}
	if got := powerFeedMaxParallel(o, 1); got != 2 {
		t.Fatalf("effective maximum = %d, want 2 when only two tracks fit", got)
	}

	o.TraceSpacing = 0.20
	if _, err := normalizeOptions(o); err != nil {
		t.Fatalf("three-track requests should shrink to the single track that fits: %v", err)
	}
	if got := powerFeedMaxParallel(o, 1); got != 1 {
		t.Fatalf("effective maximum = %d, want 1 when only one track fits", got)
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
			hub := gridgeom.PositionToPoint(cluster.Hub.Q, cluster.Hub.R, o.Width, o.Height, o.Pitch, "pointy")
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

func TestGenerateUsesSeparateGridColor(t *testing.T) {
	o := DefaultOptions()
	o.ShowBackgroundGrid = true
	o.GridColor = "#abcdef"
	o.Seed = 13579
	o.SeedSet = true

	svg, _, _, err := Generate(o)
	if err != nil {
		t.Fatal(err)
	}
	gridStart := strings.Index(svg, `id="background-grid"`)
	tracesStart := strings.Index(svg, `id="traces"`)
	if gridStart < 0 || tracesStart <= gridStart {
		t.Fatal("could not locate background grid layer in generated SVG")
	}
	if !strings.Contains(svg[gridStart:tracesStart], `stroke="#abcdef"`) {
		t.Fatal("PCB background grid does not use GridColor")
	}
}

func TestDefaultGenerationAttempts(t *testing.T) {
	if got := DefaultOptions().GenerationAttempts; got != 4 {
		t.Fatalf("default generation attempts = %d, want 4", got)
	}
}

func TestNormalizeOptionsRejectsInvalidGenerationAttempts(t *testing.T) {
	for _, attempts := range []int{0, -1, maxGenerationAttempts + 1} {
		o := DefaultOptions()
		o.GenerationAttempts = attempts
		if _, err := normalizeOptions(o); err == nil {
			t.Errorf("generation attempts %d should be rejected", attempts)
		}
	}
}

func TestBuildBoardWithSeedAttemptsRetriesOnlyRetryableFailures(t *testing.T) {
	var seen []int64
	wantPlan := boardPlan{Components: []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 3, R: 2}}}}
	got, err := buildBoardWithSeedAttempts(3, 42, func(seed int64) (boardPlan, error) {
		seen = append(seen, seed)
		if len(seen) < 3 {
			return boardPlan{}, &retryableGenerationError{err: fmt.Errorf("placement exhausted")}
		}
		return wantPlan, nil
	})
	if err != nil {
		t.Fatalf("expected a later seed to succeed: %v", err)
	}
	if !reflect.DeepEqual(got, wantPlan) {
		t.Fatalf("returned board plan = %#v, want %#v", got, wantPlan)
	}
	wantSeeds := []int64{42, generationAttemptSeed(42, 1), generationAttemptSeed(42, 2)}
	if !reflect.DeepEqual(seen, wantSeeds) {
		t.Fatalf("attempt seeds = %v, want %v", seen, wantSeeds)
	}
	if seen[0] != 42 {
		t.Fatalf("first attempt seed = %d, want the supplied seed 42", seen[0])
	}
}

func TestBuildBoardWithSeedAttemptsDoNotRetryPermanentErrors(t *testing.T) {
	calls := 0
	permanentErr := fmt.Errorf("invalid board geometry")
	_, err := buildBoardWithSeedAttempts(4, 17, func(seed int64) (boardPlan, error) {
		calls++
		return boardPlan{}, permanentErr
	})
	if err != permanentErr {
		t.Fatalf("returned error = %v, want original error %v", err, permanentErr)
	}
	if calls != 1 {
		t.Fatalf("build callback called %d times for a permanent error, want once", calls)
	}
}

func TestBuildBoardWithSeedAttemptsReportsExhaustion(t *testing.T) {
	calls := 0
	_, err := buildBoardWithSeedAttempts(2, 17, func(seed int64) (boardPlan, error) {
		calls++
		return boardPlan{}, &retryableGenerationError{err: fmt.Errorf("placement exhausted")}
	})
	if err == nil {
		t.Fatal("expected an error after all seed attempts failed")
	}
	if calls != 2 {
		t.Fatalf("build callback called %d times, want 2", calls)
	}
	if !strings.Contains(err.Error(), "generation failed after 2 seed attempts") {
		t.Fatalf("exhaustion error does not report attempt count: %v", err)
	}
}

func TestDefaultRouteAttempts(t *testing.T) {
	if got := DefaultOptions().RouteAttempts; got != 12 {
		t.Fatalf("default route attempts = %d, want 12", got)
	}
}

func TestNormalizeOptionsRejectsInvalidRouteAttempts(t *testing.T) {
	for _, attempts := range []int{0, -1} {
		o := DefaultOptions()
		o.RouteAttempts = attempts
		if _, err := normalizeOptions(o); err == nil {
			t.Errorf("route attempts %d should be rejected", attempts)
		}
	}
}

func TestRoutePowerFeedsRetriesCompleteFeedSet(t *testing.T) {
	// A route attempt must start with empty reservations; failed partial paths
	// must never leak into the next pass.
	order := []powerRouteAssignment{
		{clusterIndex: 0, sourceIndex: 0, source: powerSource{Position: hexgrid.GridPosition{Q: 0, R: 0}}, target: hexgrid.GridPosition{Q: 3, R: 0}},
	}
	clusters := []clusterPlan{{PowerPaths: make([][]hexgrid.GridPosition, 1)}}
	allowed := map[hexgrid.GridPosition]bool{}
	for q := 0; q <= 3; q++ {
		allowed[hexgrid.GridPosition{Q: q, R: 0}] = true
	}
	blocked := map[hexgrid.GridPosition]bool{
		{Q: 0, R: 0}: true, {Q: 3, R: 0}: true,
	}
	if !routePowerFeeds(3, rand.New(rand.NewSource(1)), order, clusters, allowed, blocked) {
		t.Fatal("expected a valid feed route")
	}
	if len(clusters[0].PowerPaths[0]) < 2 {
		t.Fatalf("stored route = %v", clusters[0].PowerPaths[0])
	}
}

func TestPowerSourceRowsPreserveHorizontalSymmetry(t *testing.T) {
	tests := []struct {
		name    string
		rows    map[int][]gridgeom.Cell
		count   int
		top     bool
		spacing PowerSourceSpacing
		wantR   int
		wantQ   []int
	}{
		{
			name: "keeps symmetric outer row",
			rows: map[int][]gridgeom.Cell{
				0: makeSymmetryTestPowerSourceRow(0, 8, 0),
				1: makeSymmetryTestPowerSourceRow(1, 9, 1),
			},
			count: 4, top: true, spacing: PowerSourceSpacingAdjacent,
			wantR: 0, wantQ: []int{2, 3, 4, 5},
		},
		{
			name: "moves inward for separated spacing",
			rows: map[int][]gridgeom.Cell{
				0: makeSymmetryTestPowerSourceRow(0, 8, 0),
				1: makeSymmetryTestPowerSourceRow(1, 9, 1),
				2: makeSymmetryTestPowerSourceRow(2, 10, 2),
			},
			count: 4, top: true, spacing: PowerSourceSpacingSeparated,
			wantR: 1, wantQ: []int{1, 3, 5, 7},
		},
		{
			name: "moves inward for odd adjacent count",
			rows: map[int][]gridgeom.Cell{
				0: makeSymmetryTestPowerSourceRow(0, 8, 0),
				1: makeSymmetryTestPowerSourceRow(1, 9, 1),
			},
			count: 3, top: true, spacing: PowerSourceSpacingAdjacent,
			wantR: 1, wantQ: []int{3, 4, 5},
		},
		{
			name: "moves inward from bottom edge",
			rows: map[int][]gridgeom.Cell{
				0: makeSymmetryTestPowerSourceRow(0, 10, 0),
				1: makeSymmetryTestPowerSourceRow(1, 9, 1),
				2: makeSymmetryTestPowerSourceRow(2, 8, 2),
			},
			count: 4, top: false, spacing: PowerSourceSpacingSeparated,
			wantR: 1, wantQ: []int{1, 3, 5, 7},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectPowerSourceRow(test.rows, test.count, test.top, test.spacing)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.wantQ) {
				t.Fatalf("got %d positions, want %d: %v", len(got), len(test.wantQ), got)
			}
			for i, position := range got {
				if position.R != test.wantR || position.Q != test.wantQ[i] {
					t.Fatalf("position %d = %v, want {Q:%d R:%d}", i, position, test.wantQ[i], test.wantR)
				}
			}
			for i := range got {
				if got[i].Q+got[len(got)-1-i].Q != test.wantQ[0]+test.wantQ[len(test.wantQ)-1] {
					t.Fatalf("selected positions are not horizontally mirrored: %v", got)
				}
			}
		})
	}
}

func makeSymmetryTestPowerSourceRow(r, length int, y float64) []gridgeom.Cell {
	cells := make([]gridgeom.Cell, length)
	for q := range cells {
		cells[q] = gridgeom.Cell{
			Q: q, R: r,
			Center: geometry.Point{X: float64(q), Y: y},
		}
	}
	return cells
}

func TestGlowOptionsDefaultDisabledAndValidate(t *testing.T) {
	options := DefaultOptions()
	if options.BaseGlow || options.AccentGlow {
		t.Fatal("base and accent glow should both be disabled by default")
	}
	for _, settings := range []struct {
		name            string
		radius, opacity float64
	}{
		{"base", options.BaseGlowBlurRadius, options.BaseGlowOpacity},
		{"accent", options.AccentGlowBlurRadius, options.AccentGlowOpacity},
	} {
		if settings.radius <= 0 || settings.opacity <= 0 {
			t.Fatalf("default %s glow settings must be usable: radius=%v opacity=%v", settings.name, settings.radius, settings.opacity)
		}
	}

	zeroGlow := DefaultOptions()
	zeroGlow.BaseGlowBlurRadius, zeroGlow.BaseGlowOpacity = 0, 0
	zeroGlow.AccentGlowBlurRadius, zeroGlow.AccentGlowOpacity = 0, 0
	if _, err := normalizeOptions(zeroGlow); err != nil {
		t.Fatalf("zero-valued glow settings should be valid while both glows are disabled: %v", err)
	}

	tests := []struct {
		name string
		edit func(*Options)
	}{
		{"negative base blur radius", func(o *Options) { o.BaseGlowBlurRadius = -0.01 }},
		{"NaN base blur radius", func(o *Options) { o.BaseGlowBlurRadius = math.NaN() }},
		{"base opacity above one", func(o *Options) { o.BaseGlowOpacity = 1.1 }},
		{"enabled base glow with zero opacity", func(o *Options) { o.BaseGlow = true; o.BaseGlowOpacity = 0 }},
		{"enabled base glow with zero blur radius", func(o *Options) { o.BaseGlow = true; o.BaseGlowBlurRadius = 0 }},
		{"negative accent blur radius", func(o *Options) { o.AccentGlowBlurRadius = -0.01 }},
		{"NaN accent blur radius", func(o *Options) { o.AccentGlowBlurRadius = math.NaN() }},
		{"accent opacity above one", func(o *Options) { o.AccentGlowOpacity = 1.1 }},
		{"enabled accent glow with zero opacity", func(o *Options) { o.AccentGlow = true; o.AccentGlowOpacity = 0 }},
		{"enabled accent glow with zero blur radius", func(o *Options) { o.AccentGlow = true; o.AccentGlowBlurRadius = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := DefaultOptions()
			test.edit(&o)
			if _, err := normalizeOptions(o); err == nil {
				t.Fatal("expected invalid glow settings to be rejected")
			}
		})
	}
}

func TestRenderBoardUsesIndependentBaseAndAccentGlowSettings(t *testing.T) {
	o := DefaultOptions()
	o.BaseGlow = true
	o.BaseGlowBlurRadius = 0.03
	o.BaseGlowOpacity = 0.2
	o.AccentGlow = true
	o.AccentGlowBlurRadius = 0.08
	o.AccentGlowOpacity = 0.7
	plan := boardPlan{
		Components:             []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 0, R: 0}, Style: hexgrid.ComponentStyleSingle}},
		SignalTraces:           []hexgrid.GridTrace{{Start: hexgrid.GridPosition{Q: 0, R: 0}, Path: []hexgrid.GridPosition{{Q: 1, R: 0}}, Lines: 1}},
		PowerCores:             []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 0, R: 2}, Style: hexgrid.ComponentStyleSingle}},
		PowerTraces:            []hexgrid.GridTrace{{Start: hexgrid.GridPosition{Q: 0, R: 2}, Path: []hexgrid.GridPosition{{Q: 1, R: 2}}, Lines: 1}},
		InterclusterComponents: []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 0, R: -2}, Style: hexgrid.ComponentStyleSingle}},
		InterClusterTraces:     []hexgrid.GridTrace{{Start: hexgrid.GridPosition{Q: 0, R: -2}, Path: []hexgrid.GridPosition{{Q: 1, R: -2}}, Lines: 1}},
	}
	svg, err := renderBoard(o, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"hexgrid-neon-glow", "power-hexgrid-neon-glow", "inter-cluster-hexgrid-neon-glow"} {
		if !strings.Contains(svg, `id="`+id+`"`) {
			t.Errorf("combined SVG is missing filter definition %q", id)
		}
	}
	if !strings.Contains(svg, `stdDeviation="30.000"`) || !strings.Contains(svg, `slope="0.200"`) {
		t.Fatal("base layer does not use its configured glow radius and opacity")
	}
	if !strings.Contains(svg, `stdDeviation="80.000"`) || !strings.Contains(svg, `slope="0.700"`) {
		t.Fatal("accent layers do not use their configured glow radius and opacity")
	}
	if !strings.Contains(svg, `filter="url(#power-hexgrid-neon-glow)"`) ||
		!strings.Contains(svg, `filter="url(#inter-cluster-hexgrid-neon-glow)"`) {
		t.Fatal("accent layer groups contain unresolved glow filter references")
	}
}

func TestRenderBoardCanGlowOnlyOneColorLayer(t *testing.T) {
	plan := boardPlan{
		Components:   []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 0, R: 0}, Style: hexgrid.ComponentStyleSingle}},
		SignalTraces: []hexgrid.GridTrace{{Start: hexgrid.GridPosition{Q: 0, R: 0}, Path: []hexgrid.GridPosition{{Q: 1, R: 0}}, Lines: 1}},
		PowerCores:   []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 0, R: 2}, Style: hexgrid.ComponentStyleSingle}},
		PowerTraces:  []hexgrid.GridTrace{{Start: hexgrid.GridPosition{Q: 0, R: 2}, Path: []hexgrid.GridPosition{{Q: 1, R: 2}}, Lines: 1}},
	}
	tests := []struct {
		name                 string
		base, accent         bool
		wantBase, wantAccent bool
	}{
		{"base only", true, false, true, false},
		{"accent only", false, true, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := DefaultOptions()
			o.BaseGlow = test.base
			o.AccentGlow = test.accent
			svg, err := renderBoard(o, plan)
			if err != nil {
				t.Fatal(err)
			}
			baseHasFilter := strings.Contains(svg, `id="hexgrid-neon-glow"`)
			accentHasFilter := strings.Contains(svg, `id="power-hexgrid-neon-glow"`)
			if baseHasFilter != test.wantBase || accentHasFilter != test.wantAccent {
				t.Fatalf("filter presence = (base %v, accent %v), want (%v, %v)", baseHasFilter, accentHasFilter, test.wantBase, test.wantAccent)
			}
		})
	}
}

func TestRenderBoardDoesNotEmitGlowByDefault(t *testing.T) {
	o := DefaultOptions()
	plan := boardPlan{Components: []hexgrid.GridComponent{{Position: hexgrid.GridPosition{Q: 0, R: 0}, Style: hexgrid.ComponentStyleSingle}}}
	svg, err := renderBoard(o, plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(svg, "hexgrid-neon-glow") {
		t.Fatal("default PCB rendering unexpectedly contains neon glow filters")
	}
}

func TestNormalizeOptionsRejectsInvalidDecorativeTraceSettings(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Options)
	}{
		{"negative count", func(o *Options) { o.DecorativeTraceCount = -1 }},
		{"count too large", func(o *Options) { o.DecorativeTraceCount = maxDecorativeTraceCount + 1 }},
		{"negative decorative power count", func(o *Options) { o.DecorativePowerTraceCount = -1 }},
		{"decorative power count too large", func(o *Options) { o.DecorativePowerTraceCount = maxDecorativeTraceCount + 1 }},
		{"minimum below three", func(o *Options) { o.DecorativeTraceMinLength = 2 }},
		{"too many bends", func(o *Options) { o.DecorativeTraceMaxBends = 11 }},
		{"maximum below minimum", func(o *Options) { o.DecorativeTraceMaxLength = o.DecorativeTraceMinLength - 1 }},
		{"maximum over 100", func(o *Options) { o.DecorativeTraceMaxLength = 101 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := DefaultOptions()
			tt.edit(&o)
			if _, err := normalizeOptions(o); err == nil {
				t.Fatal("expected invalid decorative-trace settings to be rejected")
			}
		})
	}
}

func TestBuildBoardDoesNotAddDecorativeTracesWithoutExistingRoutes(t *testing.T) {
	o := DefaultOptions()
	o.ComponentCount = 12
	o.ClusterCount = 3
	o.MinPowerSourcesPerCluster = 0
	o.MaxPowerSourcesPerCluster = 0
	o.TopPowerSourceCount = 0
	o.BottomPowerSourceCount = 0
	o.Connections = 0
	o.InterClusterTraces = false
	o.DecorativeTraceCount = 3
	o.Seed, o.SeedSet = 381, true

	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.PowerTraces) != 0 || len(plan.InterClusterTraces) != 0 {
		t.Fatal("test fixture unexpectedly produced a primary route source")
	}
	if len(plan.DecorativeTraces) != 0 {
		t.Fatalf("got %d decorative traces without any power or inter-cluster source routes, want none", len(plan.DecorativeTraces))
	}
}

func TestBuildBoardAddsDecorativePowerTracesFromUnusedContacts(t *testing.T) {
	o := DefaultOptions()
	o.DecorativePowerTraceCount = 4
	o.Seed, o.SeedSet = 431, true
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(o.Seed)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	unused := unusedPowerSourcePositions(top, bottom, plan.Clusters)
	if len(unused) == 0 {
		t.Fatal("fixture has no unused physical power-source contacts")
	}
	for i, trace := range plan.DecorativePowerTraces {
		found := false
		for _, source := range unused {
			if trace.Start == source {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("decorative power trace %d starts at %v, which is not an unused source contact", i, trace.Start)
		}
		if trace.HideTerminal || len(trace.Path) < 2 {
			t.Errorf("decorative power trace %d has invalid free-ended geometry: %+v", i, trace)
		}
	}
	if len(plan.DecorativePowerTraces) > o.DecorativePowerTraceCount {
		t.Fatalf("got %d decorative power traces, exceeding best-effort target %d", len(plan.DecorativePowerTraces), o.DecorativePowerTraceCount)
	}
	_, _, count, err := Generate(o)
	if err != nil {
		t.Fatalf("Generate with decorative power traces: %v", err)
	}
	wantCount := len(plan.SignalTraces) + len(plan.PowerTraces) + len(plan.InterClusterTraces) + len(plan.DecorativeTraces) + len(plan.DecorativePowerTraces) + len(plan.DecorativePowerTraces)
	if count != wantCount {
		t.Fatalf("Generate trace count = %d, want %d", count, wantCount)
	}
}

func TestPowerSourceComponentRadiusIsIndependentAndChecksContactOverlap(t *testing.T) {
	o := DefaultOptions()
	o.PowerSourceSpacing = PowerSourceSpacingAdjacent
	o.PowerSourceComponentRadius = 0.30
	if _, _, err := powerSourcePositions(o); err == nil {
		t.Fatal("expected oversized adjacent power-source contacts to overlap")
	}

	o.PowerSourceComponentRadius = 0.20
	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatalf("smaller power-source radius should permit adjacent contacts: %v", err)
	}
	if len(top) != o.TopPowerSourceCount || len(bottom) != o.BottomPowerSourceCount {
		t.Fatalf("got %d top and %d bottom contacts, want %d and %d",
			len(top), len(bottom), o.TopPowerSourceCount, o.BottomPowerSourceCount)
	}
	if minimum, maximum := clusterComponentRadiusBounds(o); minimum != o.ComponentRadius || maximum != o.ComponentRadius {
		t.Fatalf("source radius changed cluster fallback bounds to (%g, %g), want ComponentRadius %g",
			minimum, maximum, o.ComponentRadius)
	}
}
