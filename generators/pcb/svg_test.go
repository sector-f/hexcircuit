package pcb

import (
	"encoding/xml"
	"math/rand"
	"strings"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestGenerateIsDeterministicWhenSeeded(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 42
	opts.SeedSet = true
	first, components, traces, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	second, secondComponents, secondTraces, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("same seed and options produced different SVG")
	}
	if components != secondComponents || traces != secondTraces {
		t.Fatal("same seed produced different counts")
	}
}

func TestDefaultOptionsAcrossSeeds(t *testing.T) {
	opts := DefaultOptions()
	opts.SeedSet = true
	for seed := int64(0); seed < 30; seed++ {
		opts.Seed = seed
		_, components, traces, err := Generate(opts)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if components != opts.ComponentCount+opts.ConnectorCount {
			t.Fatalf("seed %d: generated %d components, want %d", seed, components, opts.ComponentCount+opts.ConnectorCount)
		}
		if traces == 0 {
			t.Fatalf("seed %d: generated no trace bundles", seed)
		}
	}
}

func TestGenerateChangesWithSeed(t *testing.T) {
	opts := DefaultOptions()
	opts.SeedSet = true
	opts.Seed = 10
	first, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.Seed = 11
	second, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different seeds produced identical artwork")
	}
}

func TestGenerateReturnsValidSVGAndTwoColors(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed, opts.SeedSet = 7, true
	got, components, traces, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		XMLName xml.Name `xml:"svg"`
	}
	if err := xml.Unmarshal([]byte(got), &document); err != nil {
		t.Fatalf("invalid SVG XML: %v", err)
	}
	if document.XMLName.Local != "svg" {
		t.Fatalf("root element is %q, want svg", document.XMLName.Local)
	}
	if components <= opts.ComponentCount {
		t.Fatalf("component count %d does not include the required connector row", components)
	}
	if traces < 1 {
		t.Fatal("generated no traces")
	}
	if !strings.Contains(got, `stroke="`+opts.BaseColor+`"`) {
		t.Fatal("base color missing from SVG")
	}
	if !strings.Contains(got, `stroke="`+opts.AccentColor+`"`) {
		t.Fatal("accent color missing from SVG")
	}
}

func TestBackgroundGridIsOptIn(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed, opts.SeedSet = 1, true
	without, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(svgGroup(without, `id="background-grid"`), "<polygon"); count != 0 {
		t.Fatalf("background grid is enabled by default: found %d hexagons", count)
	}
	opts.ShowBackgroundGrid = true
	with, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(svgGroup(with, `id="background-grid"`), "<polygon"); count == 0 {
		t.Fatal("enabled background grid contains no hexagons")
	}
	if count := strings.Count(svgGroup(with, `id="background-grid-layer-2"`), "<polygon"); count != 0 {
		t.Fatalf("accent layer unexpectedly renders %d background hexagons", count)
	}
}

func TestBottomConnectorRowIsAlwaysPresent(t *testing.T) {
	opts := DefaultOptions()
	base, _, err := generateLayoutForTest(opts)
	if err != nil {
		t.Fatal(err)
	}
	row, err := connectorPositions(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, position := range row {
		if !base.cells[position] {
			t.Errorf("bottom contact at (%d,%d) is missing", position.Q, position.R)
		}
	}
	for i := 1; i < len(row); i++ {
		if row[i].R != row[i-1].R || row[i].Q != row[i-1].Q+1 {
			t.Fatalf("connector row is not contiguous and horizontal: %v then %v", row[i-1], row[i])
		}
	}
}

func TestConnectorRowRejectsTooNarrowCanvas(t *testing.T) {
	opts := DefaultOptions()
	opts.Width = 5
	if _, _, _, err := Generate(opts); err == nil {
		t.Fatal("accepted a canvas too narrow for the required connector row")
	}
}

func TestGeneratedLayoutUsesSeparateBaseAndAccentLayers(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed, opts.SeedSet = 23, true
	rng := rand.New(rand.NewSource(opts.Seed))
	base, accent, err := generateLayout(opts, rng)
	if err != nil {
		t.Fatal(err)
	}
	if len(base.scene.Traces) == 0 || len(accent.scene.Traces) == 0 {
		t.Fatal("expected both signal and power traces")
	}
	if len(accent.scene.Components) == 0 {
		t.Fatal("expected accent component cores")
	}
}

func TestRoutesDoNotIntersectAcrossSeeds(t *testing.T) {
	options := DefaultOptions()
	options.SeedSet = true

	for seed := int64(0); seed < 60; seed++ {
		options.Seed = seed
		base, _, err := generateLayout(options, rand.New(rand.NewSource(seed)))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}

		componentPositions := make(map[hexgrid.GridPosition]bool, len(base.scene.Components))
		for _, component := range base.scene.Components {
			componentPositions[component.Position] = true
		}
		type routedSegment struct {
			from, to hexgrid.GridPosition
			trace    int
		}
		occupiedInterior := make(map[hexgrid.GridPosition]int)
		occupiedEdges := make(map[string]int)
		segments := make([]routedSegment, 0)

		for traceIndex, trace := range base.scene.Traces {
			path := make([]hexgrid.GridPosition, 1, len(trace.Path)+1)
			path[0] = trace.Start
			path = append(path, trace.Path...)

			if previousTrace, exists := occupiedInterior[path[0]]; exists {
				t.Fatalf("seed %d: trace %d starts inside trace %d", seed, traceIndex, previousTrace)
			}

			for index := 1; index < len(path); index++ {
				from, to := path[index-1], path[index]
				fromPoint, toPoint := point(from, options), point(to, options)
				for _, previous := range segments {
					if previous.trace == traceIndex {
						continue
					}
					previousFrom := point(previous.from, options)
					previousTo := point(previous.to, options)
					if !hexgrid.SegmentsIntersect(fromPoint, toPoint, previousFrom, previousTo) {
						continue
					}
					sharedComponent := (from == previous.from || from == previous.to) && componentPositions[from] ||
						(to == previous.from || to == previous.to) && componentPositions[to]
					if !sharedComponent {
						t.Fatalf("seed %d: trace %d geometrically intersects trace %d between (%d,%d)-(%d,%d) and (%d,%d)-(%d,%d)",
							seed, traceIndex, previous.trace,
							from.Q, from.R, to.Q, to.R,
							previous.from.Q, previous.from.R, previous.to.Q, previous.to.R)
					}
				}
				segments = append(segments, routedSegment{from: from, to: to, trace: traceIndex})

				edge := pairKey(from, to)
				if previousTrace, exists := occupiedEdges[edge]; exists {
					t.Fatalf("seed %d: trace %d reuses edge %s from trace %d", seed, traceIndex, edge, previousTrace)
				}
				occupiedEdges[edge] = traceIndex

				// A destination component can be a shared junction, but an interior
				// lattice cell must belong to exactly one independent trace bundle.
				if index < len(path)-1 {
					if previousTrace, exists := occupiedInterior[to]; exists {
						t.Fatalf("seed %d: trace %d crosses trace %d at cell (%d,%d)", seed, traceIndex, previousTrace, to.Q, to.R)
					}
					if componentPositions[to] {
						t.Fatalf("seed %d: trace %d passes through component (%d,%d)", seed, traceIndex, to.Q, to.R)
					}
					occupiedInterior[to] = traceIndex
				}
			}
		}
	}
}

func TestOmitPartialGridHexagons(t *testing.T) {
	opts := DefaultOptions()
	if opts.OmitPartialGridHexagons {
		t.Fatal("OmitPartialGridHexagons should be disabled by default")
	}
	opts.Seed, opts.SeedSet = 31, true
	opts.ShowBackgroundGrid = true

	withoutOmission, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	partialCount := strings.Count(svgGroup(withoutOmission, `id="background-grid"`), "<polygon")

	opts.OmitPartialGridHexagons = true
	withOmission, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	completeCount := strings.Count(svgGroup(withOmission, `id="background-grid"`), "<polygon")
	if completeCount == 0 {
		t.Fatal("omitting partial hexagons removed the entire background grid")
	}
	if completeCount >= partialCount {
		t.Fatalf("OmitPartialGridHexagons left %d hexagons; expected fewer than %d", completeCount, partialCount)
	}
}

func TestComponentsAndRoutesStayInsideEdgeSafeCells(t *testing.T) {
	opts := DefaultOptions()
	for seed := int64(0); seed < 40; seed++ {
		opts.Seed, opts.SeedSet = seed, true
		base, _, err := generateLayout(opts, rand.New(rand.NewSource(seed)))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}

		for index, component := range base.scene.Components {
			center := point(component.Position, opts)
			if !hexagonFullyInsideCanvas(center, opts.GridRadius, opts.GridStrokeWidth, opts) {
				t.Fatalf("seed %d: component %d occupies a partial background-grid cell at (%d,%d)", seed, index, component.Position.Q, component.Position.R)
			}
			if !hexagonFullyInsideCanvas(center, opts.ComponentRadius, opts.ComponentStrokeWidth, opts) {
				t.Fatalf("seed %d: component %d extends beyond the canvas at (%d,%d)", seed, index, component.Position.Q, component.Position.R)
			}
		}

		clearance := edgeCellClearance(opts)
		for traceIndex, trace := range base.scene.Traces {
			positions := append([]hexgrid.GridPosition{trace.Start}, trace.Path...)
			for _, position := range positions {
				if !hexagonFullyInsideCanvas(point(position, opts), clearance, 0, opts) {
					t.Fatalf("seed %d: trace %d travels through an edge cell at (%d,%d)", seed, traceIndex, position.Q, position.R)
				}
			}
		}
	}
}

func TestConnectorTracesInitiallyExitNortheastOrNorthwest(t *testing.T) {
	opts := DefaultOptions()
	connectors, err := connectorPositions(opts)
	if err != nil {
		t.Fatal(err)
	}
	connectorSet := make(map[hexgrid.GridPosition]bool, len(connectors))
	for _, connector := range connectors {
		connectorSet[connector] = true
	}

	for seed := int64(0); seed < 40; seed++ {
		opts.Seed, opts.SeedSet = seed, true
		base, _, err := generateLayout(opts, rand.New(rand.NewSource(seed)))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		found := false
		for traceIndex, trace := range base.scene.Traces {
			if !connectorSet[trace.Start] {
				for _, position := range trace.Path {
					if connectorSet[position] {
						t.Fatalf("seed %d: trace %d terminates at a connector instead of originating there", seed, traceIndex)
					}
				}
				continue
			}
			if len(trace.Path) == 0 {
				t.Fatalf("seed %d: connector trace %d has no first step", seed, traceIndex)
			}
			firstDirection := hexgrid.GridPosition{
				Q: trace.Path[0].Q - trace.Start.Q,
				R: trace.Path[0].R - trace.Start.R,
			}
			if !isConnectorExitDirection(firstDirection) {
				t.Fatalf("seed %d: connector trace %d initially exits in direction (%d,%d)", seed, traceIndex, firstDirection.Q, firstDirection.R)
			}
			found = true
		}
		if !found {
			t.Fatalf("seed %d: no connector-originating trace was generated", seed)
		}
	}
}

func generateLayoutForTest(opts Options) (*design, *design, error) {
	return generateLayout(opts, rand.New(rand.NewSource(opts.Seed+17)))
}

func svgGroup(svg, idAttribute string) string {
	start := strings.Index(svg, idAttribute)
	if start < 0 {
		return ""
	}
	openEnd := strings.IndexByte(svg[start:], '>')
	if openEnd < 0 {
		return ""
	}
	bodyStart := start + openEnd + 1
	close := strings.Index(svg[bodyStart:], "</g>")
	if close < 0 {
		return ""
	}
	return svg[bodyStart : bodyStart+close]
}
