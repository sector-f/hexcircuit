package main

import (
	"io"
	"testing"

	"github.com/sector-f/hexgrid/generators/pcb"
)

func TestParseOptionsSupportsPowerSourceAndPlacementOptions(t *testing.T) {
	opts, err := parseOptions([]string{
		"--min-components-per-cluster=2",
		"--min-power-sources-per-cluster=0",
		"--max-power-sources-per-cluster=3",
		"--top-power-source-count=9",
		"--bottom-power-source-count=11",
		"--power-source-spacing=adjacent",
		"--power-source-style=double",
		"--constrain-components-to-power-source-span",
		"--constrain-layout-to-power-source-trapezoid",
		"--inter-cluster-traces",
		"--route-attempts=7",
		"--generation-attempts=6",
		"--decorative-trace-count=7",
		"--decorative-trace-min-length=3",
		"--decorative-trace-max-length=5",
		"--decorative-trace-max-bends=2",
		"--grid-color=#abcdef",
		"--base-glow",
		"--base-glow-blur-radius=0.03",
		"--base-glow-opacity=0.2",
		"--accent-glow",
		"--accent-glow-blur-radius=0.08",
		"--accent-glow-opacity=0.7",
		"--seed=42",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Generator.MinComponentsPerCluster != 2 {
		t.Errorf("minimum components per cluster = %d, want 2", opts.Generator.MinComponentsPerCluster)
	}
	if opts.Generator.MinPowerSourcesPerCluster != 0 || opts.Generator.MaxPowerSourcesPerCluster != 3 {
		t.Errorf("power-source range = %d..%d, want 0..3", opts.Generator.MinPowerSourcesPerCluster, opts.Generator.MaxPowerSourcesPerCluster)
	}
	if opts.Generator.TopPowerSourceCount != 9 || opts.Generator.BottomPowerSourceCount != 11 {
		t.Errorf("power-source counts = %d top, %d bottom; want 9 and 11", opts.Generator.TopPowerSourceCount, opts.Generator.BottomPowerSourceCount)
	}
	if opts.Generator.GridColor != "#abcdef" {
		t.Errorf("grid color = %q, want #abcdef", opts.Generator.GridColor)
	}
	if !opts.Generator.BaseGlow || opts.Generator.BaseGlowBlurRadius != 0.03 || opts.Generator.BaseGlowOpacity != 0.2 {
		t.Errorf("base glow settings = (%v, %v, %v), want (true, 0.03, 0.2)",
			opts.Generator.BaseGlow, opts.Generator.BaseGlowBlurRadius, opts.Generator.BaseGlowOpacity)
	}
	if !opts.Generator.AccentGlow || opts.Generator.AccentGlowBlurRadius != 0.08 || opts.Generator.AccentGlowOpacity != 0.7 {
		t.Errorf("accent glow settings = (%v, %v, %v), want (true, 0.08, 0.7)",
			opts.Generator.AccentGlow, opts.Generator.AccentGlowBlurRadius, opts.Generator.AccentGlowOpacity)
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
	if !opts.Generator.ConstrainLayoutToPowerSourceTrapezoid {
		t.Error("power-source trapezoid constraint was not enabled")
	}
	if !opts.Generator.InterClusterTraces {
		t.Error("inter-cluster traces were not enabled")
	}
	if opts.Generator.RouteAttempts != 7 {
		t.Errorf("route attempts = %d, want 7", opts.Generator.RouteAttempts)
	}
	if opts.Generator.GenerationAttempts != 6 {
		t.Errorf("generation attempts = %d, want 6", opts.Generator.GenerationAttempts)
	}
	if opts.Generator.DecorativeTraceCount != 7 || opts.Generator.DecorativeTraceMinLength != 3 || opts.Generator.DecorativeTraceMaxLength != 5 || opts.Generator.DecorativeTraceMaxBends != 2 {
		t.Errorf("decorative-trace options = (%d, %d, %d, %d), want (7, 3, 5, 2)",
			opts.Generator.DecorativeTraceCount, opts.Generator.DecorativeTraceMinLength, opts.Generator.DecorativeTraceMaxLength, opts.Generator.DecorativeTraceMaxBends)
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

func TestParseOptionsAllowsZeroFeedsPerCluster(t *testing.T) {
	opts, err := parseOptions([]string{
		"--top-power-source-count=0",
		"--bottom-power-source-count=0",
		"--min-power-sources-per-cluster=0",
		"--max-power-sources-per-cluster=0",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Generator.MinPowerSourcesPerCluster != 0 || opts.Generator.MaxPowerSourcesPerCluster != 0 {
		t.Fatalf("power-source range = %d..%d, want 0..0", opts.Generator.MinPowerSourcesPerCluster, opts.Generator.MaxPowerSourcesPerCluster)
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
