package circuit

import (
	"strings"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestGridSceneForRenderPassesGridColor(t *testing.T) {
	options := DefaultOptions()
	options.GridColor = "#abcdef"

	layout, err := gridSceneForRender(options, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Options.GridColor != options.GridColor {
		t.Fatalf("grid color = %q, want %q", layout.Options.GridColor, options.GridColor)
	}

	svg, err := hexgrid.RenderGrid(layout)
	if err != nil {
		t.Fatal(err)
	}
	gridStart := strings.Index(svg, `id="background-grid"`)
	tracesStart := strings.Index(svg, `id="traces"`)
	if gridStart < 0 || tracesStart <= gridStart {
		t.Fatal("could not locate background grid layer in SVG")
	}
	if !strings.Contains(svg[gridStart:tracesStart], `stroke="#abcdef"`) {
		t.Fatal("circuit background grid does not use GridColor")
	}
}
