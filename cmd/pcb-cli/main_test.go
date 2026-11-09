package main

import (
	"io"
	"testing"

	"github.com/sector-f/hexgrid/generators/pcb"
)

func TestParseOptionsSupportsPowerSourceAndPlacementOptions(t *testing.T) {
	opts, err := parseOptions([]string{
		"--min-components-per-cluster=2",
		"--top-power-source-count=9",
		"--bottom-power-source-count=11",
		"--power-source-spacing=adjacent",
		"--power-source-style=double",
		"--constrain-components-to-power-source-span",
		"--seed=42",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Generator.MinComponentsPerCluster != 2 {
		t.Errorf("minimum components per cluster = %d, want 2", opts.Generator.MinComponentsPerCluster)
	}
	if opts.Generator.TopPowerSourceCount != 9 || opts.Generator.BottomPowerSourceCount != 11 {
		t.Errorf("power-source counts = %d top, %d bottom; want 9 and 11", opts.Generator.TopPowerSourceCount, opts.Generator.BottomPowerSourceCount)
	}
	if opts.Generator.PowerSourceSpacing != pcb.PowerSourceSpacingAdjacent {
		t.Errorf("power-source spacing = %q, want adjacent", opts.Generator.PowerSourceSpacing)
	}
	if opts.Generator.PowerSourceStyle != pcb.PowerSourceStyleDouble {
		t.Errorf("power-source style = %q, want double", opts.Generator.PowerSourceStyle)
	}
	if !opts.Generator.ConstrainComponentsToPowerSourceSpan {
		t.Error("component-span constraint was not enabled")
	}
	if !opts.Generator.SeedSet || opts.Generator.Seed != 42 {
		t.Errorf("seed state = (%d, %v), want (42, true)", opts.Generator.Seed, opts.Generator.SeedSet)
	}
}

func TestParseOptionsPreservesBottomConnectorCompatibilityAlias(t *testing.T) {
	opts, err := parseOptions([]string{"--connector-count=8"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Generator.ConnectorCount != 8 {
		t.Fatalf("legacy connector-count flag produced alias value %d, want 8", opts.Generator.ConnectorCount)
	}
}

func TestParseOptionsAllowsExplicitZeroPowerSourceRow(t *testing.T) {
	opts, err := parseOptions([]string{"--top-power-source-count=0", "--bottom-power-source-count=10"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Generator.TopPowerSourceCount != 0 || opts.Generator.BottomPowerSourceCount != 10 {
		t.Fatalf("source counts = %d top, %d bottom, want 0 and 10", opts.Generator.TopPowerSourceCount, opts.Generator.BottomPowerSourceCount)
	}
}
