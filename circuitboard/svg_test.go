package circuitboard

import (
	"encoding/xml"
	"math/rand"
	"strings"
	"testing"
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
