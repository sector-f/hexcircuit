package circuit

import (
	"os"
	"testing"
)

func TestDefaultGolden(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 42
	opts.SeedSet = true

	filename := "testdata/golden.svg"

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

func TestDefaultGoldenDeterministicRouteCost(t *testing.T) {
	opts := DefaultOptions()
	opts.UseDeterministicRouteCost = true
	opts.Seed = 42
	opts.SeedSet = true

	filename := "testdata/golden_deterministic.svg"

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

func TestDefaultGoldenWithLabels(t *testing.T) {
	opts := DefaultOptions()
	opts.ShowLabels = true
	opts.LabelProbability = 1.0
	opts.LabelFont = "DejaVu Sans Mono"
	opts.Seed = 42
	opts.SeedSet = true

	filename := "testdata/golden_labels.svg"

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
