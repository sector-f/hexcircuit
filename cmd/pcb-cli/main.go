// Command pcb-cli generates a randomized PCB-inspired hexagonal illustration as SVG.
// All dimensional options are expressed in inches.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sector-f/hexgrid/generators/pcb"
	"github.com/spf13/pflag"
)

// CLIOptions contains settings used only by the command-line wrapper. Output belongs
// here rather than in pcb.Options because the generator returns SVG text and performs
// no file I/O.
type CLIOptions struct {
	// Output is the path where the command-line wrapper writes the generated SVG.
	Output string
	// Generator contains the settings used to generate the SVG contents.
	Generator pcb.Options
}

// DefaultCLIOptions returns the command-line defaults, including the output filename.
func DefaultCLIOptions() CLIOptions {
	return CLIOptions{
		Output:    "pcb.svg",
		Generator: pcb.DefaultOptions(),
	}
}

// parseOptions maps command-line flags into separate CLI and generator options.
func parseOptions(arguments []string, stderr io.Writer) (CLIOptions, error) {
	opts := DefaultCLIOptions()

	// Use an explicit FlagSet instead of package-global flags so parsing is testable.
	flags := pflag.NewFlagSet("pcb-cli", pflag.ContinueOnError)
	flags.SetOutput(stderr)

	flags.StringVarP(&opts.Output, "output", "o", opts.Output, "Output SVG filename.")
	flags.Float64VarP(&opts.Generator.Width, "width", "W", opts.Generator.Width, "Canvas width in inches.")
	flags.Float64VarP(&opts.Generator.Height, "height", "H", opts.Generator.Height, "Canvas height in inches.")
	flags.StringVar(&opts.Generator.BaseColor, "base-color", opts.Generator.BaseColor, "Color for ordinary components, source contacts, and signal traces.")
	flags.StringVar(&opts.Generator.GridColor, "grid-color", opts.Generator.GridColor, "Background hexagon grid color; defaults to --base-color.")
	flags.StringVar(&opts.Generator.AccentColor, "accent-color", opts.Generator.AccentColor, "Color exclusively for power feeds and connected power-entry cores.")
	flags.StringVar(&opts.Generator.Background, "background", opts.Generator.Background, "Canvas background fill; use 'none' or 'transparent' for no fill.")

	flags.IntVar(&opts.Generator.ComponentCount, "component-count", opts.Generator.ComponentCount, "Number of ordinary components placed in clusters, excluding power-source contacts.")
	flags.IntVar(&opts.Generator.ClusterCount, "cluster-count", opts.Generator.ClusterCount, "Number of major component clusters.")
	flags.IntVar(&opts.Generator.MinComponentsPerCluster, "min-components-per-cluster", opts.Generator.MinComponentsPerCluster, "Minimum number of ordinary components in each cluster.")
	flags.IntVar(&opts.Generator.MinPowerSourcesPerCluster, "min-power-sources-per-cluster", opts.Generator.MinPowerSourcesPerCluster, "Minimum number of active power feeds per cluster; 0 permits unpowered clusters.")
	flags.IntVar(&opts.Generator.MaxPowerSourcesPerCluster, "max-power-sources-per-cluster", opts.Generator.MaxPowerSourcesPerCluster, "Maximum number of active power feeds per cluster (up to 6).")
	flags.IntVar(&opts.Generator.Connections, "connections", opts.Generator.Connections, "Target number of routed trace bundles.")
	flags.BoolVar(&opts.Generator.InterClusterTraces, "inter-cluster-traces", opts.Generator.InterClusterTraces, "Enable optional randomized accent-colored traces between clusters.")
	flags.IntVar(&opts.Generator.MaxParallel, "max-parallel", opts.Generator.MaxParallel, "Maximum number of parallel traces per bundle (1-3).")
	flags.IntVar(&opts.Generator.RouteAttempts, "route-attempts", opts.Generator.RouteAttempts, "Randomized power-feed routing passes per placement.")

	flags.Float64Var(&opts.Generator.Pitch, "pitch", opts.Generator.Pitch, "Center-to-center spacing of neighboring grid positions, in inches.")
	flags.Float64Var(&opts.Generator.ComponentRadius, "component-radius", opts.Generator.ComponentRadius, "Circumradius of full-size component hexagons, in inches.")
	flags.Float64Var(&opts.Generator.ComponentStrokeWidth, "component-stroke-width", opts.Generator.ComponentStrokeWidth, "Component outline width, in inches.")
	flags.Float64Var(&opts.Generator.ComponentMiddleHexagonScale, "component-middle-hexagon-scale", opts.Generator.ComponentMiddleHexagonScale, "Middle component hexagon size as a fraction of the outer radius.")
	flags.Float64Var(&opts.Generator.ComponentInnerHexagonScale, "component-inner-hexagon-scale", opts.Generator.ComponentInnerHexagonScale, "Inner component hexagon size as a fraction of the outer radius.")

	flags.Float64Var(&opts.Generator.TraceStrokeWidth, "trace-stroke-width", opts.Generator.TraceStrokeWidth, "Signal trace width, in inches.")
	flags.Float64Var(&opts.Generator.TraceSpacing, "trace-spacing", opts.Generator.TraceSpacing, "Perpendicular centerline spacing between parallel traces, in inches.")
	flags.Float64Var(&opts.Generator.NodeRadius, "node-radius", opts.Generator.NodeRadius, "Radius of hollow terminal pads, in inches.")

	flags.BoolVar(&opts.Generator.ShowBackgroundGrid, "show-background-grid", opts.Generator.ShowBackgroundGrid, "Draw the background hexagonal grid.")
	flags.BoolVar(&opts.Generator.OmitPartialGridHexagons, "omit-partial-grid-hexagons", opts.Generator.OmitPartialGridHexagons, "Omit background hexagons whose outlines would be clipped by the canvas edge.")
	flags.Float64Var(&opts.Generator.GridRadius, "grid-radius", opts.Generator.GridRadius, "Circumradius of background grid hexagons, in inches.")
	flags.Float64Var(&opts.Generator.GridStrokeWidth, "grid-stroke-width", opts.Generator.GridStrokeWidth, "Background grid outline width, in inches; zero disables grid outlines.")

	flags.IntVar(&opts.Generator.TopPowerSourceCount, "top-power-source-count", opts.Generator.TopPowerSourceCount, "Number of potential power-source positions along the top edge.")
	flags.IntVar(&opts.Generator.BottomPowerSourceCount, "bottom-power-source-count", opts.Generator.BottomPowerSourceCount, "Number of potential power-source positions along the bottom edge.")
	flags.IntVar(&opts.Generator.ConnectorCount, "connector-count", opts.Generator.ConnectorCount, "Deprecated alias for --bottom-power-source-count.")
	spacing := string(opts.Generator.PowerSourceSpacing)
	flags.StringVar(&spacing, "power-source-spacing", spacing, "Spacing between active contacts: 'separated' leaves an empty position between them; 'adjacent' permits neighbors.")
	style := string(opts.Generator.PowerSourceStyle)
	flags.StringVar(&style, "power-source-style", style, "Base-layer style for rendered power contacts: 'single' or 'double'.")
	flags.BoolVar(&opts.Generator.ConstrainComponentsToPowerSourceSpan, "constrain-components-to-power-source-span", opts.Generator.ConstrainComponentsToPowerSourceSpan, "Keep ordinary components and their traces horizontally between the leftmost and rightmost active power sources.")
	flags.BoolVar(&opts.Generator.ConstrainLayoutToPowerSourceTrapezoid, "constrain-layout-to-power-source-trapezoid", opts.Generator.ConstrainLayoutToPowerSourceTrapezoid, "Keep components and all routes inside the trapezoid bounded by the top and bottom power-source rows.")
	flags.Float64Var(&opts.Generator.ConnectorInset, "connector-inset", opts.Generator.ConnectorInset, "Requested gap between each enabled power-source row and its nearest canvas edge, in inches.")

	seedText := "random"
	flags.StringVar(&seedText, "seed", seedText, "Integer seed for reproducible output, or 'random' for a different pattern each run.")
	help := false
	flags.BoolVarP(&help, "help", "h", false, "Print this help.")

	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [flags]\n\nGenerate a randomized PCB-inspired illustration with clustered hexagonal components, local signal traces, and top/bottom power-source rows. All dimensional flags use inches.\n\nFlags:\n", flags.Name())
		flags.PrintDefaults()
	}

	if err := flags.Parse(arguments); err != nil {
		return opts, err
	}
	if help {
		flags.Usage()
		return opts, pflag.ErrHelp
	}
	if opts.Output == "" {
		return opts, errors.New("--output cannot be empty")
	}
	if flags.NArg() > 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	opts.Generator.PowerSourceSpacing = pcb.PowerSourceSpacing(spacing)
	opts.Generator.PowerSourceStyle = pcb.PowerSourceStyle(style)

	if seedText != "random" {
		seed, err := strconv.ParseInt(seedText, 10, 64)
		if err != nil {
			return opts, fmt.Errorf("--seed must be an integer or 'random': %w", err)
		}
		opts.Generator.Seed = seed
		opts.Generator.SeedSet = true
	}

	return opts, nil
}

// main is the CLI adapter: parse flags, call the core generator, and write the returned
// SVG. Generate performs no file I/O; the command wrapper owns output and error reporting.
func main() {
	opts, err := parseOptions(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	svgText, _, _, err := pcb.Generate(opts.Generator)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	file, err := os.Create(opts.Output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening file:", err)
		os.Exit(1)
	}
	if _, err := io.WriteString(file, svgText); err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, "error writing file:", err)
		os.Exit(1)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "error closing file:", err)
		os.Exit(1)
	}
}
