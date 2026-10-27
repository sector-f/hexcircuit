package circuit

import (
	"os"
	"testing"
)

func TestDefaultGolden(t *testing.T) {
	opts := DefaultOptions()
	opts.Seed = 42
	opts.SeedSet = true

	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile("testdata/golden.svg")
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

	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile("testdata/golden_deterministic.svg")

	if err != nil {
		t.Fatal(err)
	}

	if got != string(want) {
		t.Fatal("Generated SVG file did not match expected SVG file")
	}
}

// TODO: this test will probably start failing if we update the word list.
// Once we add an option to specify a word list at runtime, we should pass it in here
// so that it stays consistent.
func TestDefaultGoldenWithLabels(t *testing.T) {
	opts := DefaultOptions()
	opts.ShowLabels = true
	opts.LabelProbability = 1.0
	opts.LabelFont = "DejaVu Sans Mono"
	opts.Seed = 42
	opts.SeedSet = true

	got, _, _, err := Generate(opts)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile("testdata/golden_labels.svg")
	if err != nil {
		t.Fatal(err)
	}

	if got != string(want) {
		t.Fatal("Generated SVG file did not match expected SVG file")
	}

}
