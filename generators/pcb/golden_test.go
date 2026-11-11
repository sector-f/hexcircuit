package pcb

import (
	"os"
	"testing"
)

func TestDefaultGolden(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 42
	opts.SeedSet = true

	filename := "testdata/golden_default.svg"

	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if got != string(want) {
		t.Fatal("Generated SVG file did not match expected SVG file")
	}
}

func TestOptionsGolden(t *testing.T) {
	opts := DefaultOptions()
	opts.Background = "#111111"
	opts.ClusterCount = 10
	opts.ComponentCount = 50
	opts.ConstrainLayoutToPowerSourceTrapezoid = true
	opts.BaseGlow = true
	opts.AccentGlow = true
	opts.InterClusterTraces = true
	opts.PowerSourceSpacing = PowerSourceSpacingAdjacent
	opts.TopPowerSourceCount = 15
	opts.MinPowerSourcesPerCluster = 0
	opts.DecorativeTraceCount = 10
	opts.PowerSourceStyle = PowerSourceStyleDouble
	opts.Seed = 42
	opts.SeedSet = true

	filename := "testdata/golden_options.svg"

	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if got != string(want) {
		t.Fatal("Generated SVG file did not match expected SVG file")
	}
}
