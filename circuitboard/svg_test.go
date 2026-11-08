package circuitboard

import (
	"encoding/xml"
	"strings"
	"testing"

	hexgrid "github.com/sector-f/hexcircuit/hexgrid"
)

func TestGenerateDefaultSVG(t *testing.T) {
	opts := DefaultOptions()
	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		XMLName xml.Name `xml:"svg"`
	}
	if err := xml.Unmarshal([]byte(got), &document); err != nil {
		t.Fatalf("Generate returned invalid XML: %v", err)
	}
	if !strings.Contains(got, `stroke="`+opts.BaseColor+`"`) {
		t.Fatalf("SVG does not contain base color %q", opts.BaseColor)
	}
	if !strings.Contains(got, `stroke="`+opts.AccentColor+`"`) {
		t.Fatalf("SVG does not contain accent color %q", opts.AccentColor)
	}
	if gotCount := strings.Count(svgGroup(got, `id="background-grid"`), "<polygon"); gotCount != 0 {
		t.Fatalf("background grid is enabled by default: found %d hexagons", gotCount)
	}
}

func TestBackgroundGridIsOptIn(t *testing.T) {
	opts := DefaultOptions()
	opts.ShowBackgroundGrid = true
	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(svgGroup(got, `id="background-grid"`), "<polygon"); count == 0 {
		t.Fatal("enabled background grid contains no hexagons")
	}
	if count := strings.Count(svgGroup(got, `id="background-grid-layer-2"`), "<polygon"); count != 0 {
		t.Fatalf("accent layer unexpectedly renders %d background hexagons", count)
	}
}

func TestBottomConnectorRowIsAlwaysPresent(t *testing.T) {
	opts := DefaultOptions()
	base, _ := buildBoard(opts)
	row := connectorPositions(opts)
	if len(row) != opts.ConnectorCount {
		t.Fatalf("connector row has %d contacts, want %d", len(row), opts.ConnectorCount)
	}
	for index, position := range row {
		if !base.cells[position] {
			t.Errorf("connector %d at (%d, %d) is missing", index, position.Q, position.R)
		}
		if index > 0 && position.R != row[index-1].R {
			t.Errorf("connector row is not horizontal: %v then %v", row[index-1], position)
		}
	}
	if row[0] != (hexgrid.GridPosition{Q: -13, R: 15}) || row[len(row)-1] != (hexgrid.GridPosition{Q: -2, R: 15}) {
		t.Fatalf("default connector row changed unexpectedly: first=%+v last=%+v", row[0], row[len(row)-1])
	}
}

func TestGenerateRejectsPanelTooNarrowForConnectorRow(t *testing.T) {
	opts := DefaultOptions()
	opts.ConnectorCount = 20
	if _, _, _, err := Generate(opts); err == nil {
		t.Fatal("Generate accepted a panel too narrow for its required connector row")
	}
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
