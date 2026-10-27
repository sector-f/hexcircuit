// Command circuit_motif generates a circuit-board-inspired hexagonal pattern as SVG.
//
// The generator is intentionally self-contained. The Options type and GenerateSVG
// function form the boundary for extracting the core into a reusable package later.
// All dimensional Options fields are expressed in inches.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sector-f/hexcircuit/circuit"
	"github.com/spf13/pflag"
)

// CLIOptions contains settings used only by the command-line wrapper. Output belongs
// here rather than in Options because the generation API returns SVG text and performs
// no file I/O.
type CLIOptions struct {
	// Output is the path where the command-line wrapper writes the generated file.
	Output string
	// Generator contains the settings used to generate the SVG contents.
	Generator circuit.Options
}

// DefaultCLIOptions returns the command-line defaults, including the output filename.
func DefaultCLIOptions() CLIOptions {
	return CLIOptions{Output: "circuit_motif.svg", Generator: circuit.DefaultOptions()}
}

// parseFloatList parses comma-separated floating-point values without imposing
// domain-specific constraints. normalizeOptions validates the percentage semantics.
func parseFloatList(value string) ([]float64, error) {
	parts := strings.Split(value, ",")
	values := make([]float64, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("item %d is empty", i+1)
		}
		parsed, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("item %d (%q) is not a number: %w", i+1, part, err)
		}
		values[i] = parsed
	}
	return values, nil
}

// formatFloatList serializes numbers as a comma-separated list suitable for a flag default.
func formatFloatList(values []float64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	return strings.Join(parts, ",")
}

// parseOptions maps command-line flags into separate CLI and generator options.
// Defaults come from DefaultCLIOptions; the generated SVG itself does not use Output.
func parseOptions(arguments []string, stderr *os.File) (CLIOptions, error) {
	opts := DefaultCLIOptions()
	// Use an explicit FlagSet instead of package-global flags so parsing is testable
	// and can later be reused by a small command-line wrapper around a library package.
	flags := pflag.NewFlagSet("circuit_motif", pflag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVarP(&opts.Output, "output", "o", opts.Output, "Output SVG filename.")
	flags.Float64VarP(&opts.Generator.Width, "width", "W", opts.Generator.Width, "Canvas width in inches.")
	flags.Float64VarP(&opts.Generator.Height, "height", "H", opts.Generator.Height, "Canvas height in inches.")
	flags.StringVar(&opts.Generator.Color, "color", opts.Generator.Color, "Stroke color for components and traces.")
	flags.StringVar(&opts.Generator.Background, "background", opts.Generator.Background, "Background fill; use 'none' for transparency.")
	flags.StringVar(&opts.Generator.Orientation, "orientation", opts.Generator.Orientation, "Hexagon orientation: pointy or flat.")
	flags.IntVar(&opts.Generator.ComponentCount, "component-count", opts.Generator.ComponentCount, "Number of hexagonal components.")
	flags.Float64Var(&opts.Generator.ComponentRadius, "component-radius", opts.Generator.ComponentRadius, "Maximum component radius in inches.")
	flags.Float64Var(&opts.Generator.RoutingPitch, "routing-pitch", opts.Generator.RoutingPitch, "Distance between routing lattice points in inches; must accommodate --grid-radius and its gap.")
	flags.Float64Var(&opts.Generator.GridRadius, "grid-radius", opts.Generator.GridRadius, "Circumradius of background grid hexagons in inches; increasing this also requires more room between lattice points.")
	flags.Float64Var(&opts.Generator.GridGap, "grid-gap", opts.Generator.GridGap, "Visible gap between neighboring background hexagon outlines in inches.")
	flags.Float64Var(&opts.Generator.GridWidth, "grid-width", opts.Generator.GridWidth, "Background hexagon outline width in inches.")
	flags.BoolVar(&opts.Generator.OmitPartialGridHexagons, "omit-partial-grid-hexagons", opts.Generator.OmitPartialGridHexagons, "Omit background hexagons whose outlines would be clipped by the canvas edge.")
	flags.IntVar(&opts.Generator.Connections, "connections", opts.Generator.Connections, "Target number of component-to-component trace bundles.")
	flags.IntVar(&opts.Generator.MaxParallel, "max-parallel", opts.Generator.MaxParallel, "Maximum number of parallel traces per bundle.")
	trackCountWeightsText := formatFloatList(opts.Generator.TrackCountWeights)
	flags.StringVar(&trackCountWeightsText, "track-count-weights", trackCountWeightsText, "Comma-separated percentage weights for 1..--max-parallel tracks; count must match --max-parallel and total must be 100.")
	flags.Float64Var(&opts.Generator.ParallelSpacing, "parallel-spacing", opts.Generator.ParallelSpacing, "Perpendicular spacing between parallel traces in inches.")
	flags.Float64Var(&opts.Generator.StrokeWidth, "stroke-width", opts.Generator.StrokeWidth, "Hexagon component outline width in inches.")
	flags.Float64Var(&opts.Generator.TraceWidth, "trace-width", opts.Generator.TraceWidth, "Circuit trace width in inches.")
	flags.Float64Var(&opts.Generator.NodeRadius, "node-radius", opts.Generator.NodeRadius, "Radius of hollow terminal circles in inches.")
	flags.Float64Var(&opts.Generator.TraceClearance, "trace-clearance", opts.Generator.TraceClearance, "Minimum centerline clearance between separate trace routes in inches.")
	flags.Float64Var(&opts.Generator.DetailProb, "detail-prob", opts.Generator.DetailProb, "Probability that a sufficiently large component gets inner hexagonal details (0 to 1).")
	flags.IntVar(&opts.Generator.MaxRouteSteps, "max-route-steps", opts.Generator.MaxRouteSteps, "Maximum number of lattice steps in a route.")
	flags.IntVar(&opts.Generator.RouteAttempts, "route-attempts", opts.Generator.RouteAttempts, "Routing attempts per candidate connection.")

	flags.BoolVar(&opts.Generator.ShowLabels, "show-labels", false, "Label components")
	flags.StringVar(&opts.Generator.LabelFont, "label-font", "monospace", "Specify font used for labels")
	flags.Float64Var(&opts.Generator.LabelProbability, "label-probability", 0.35, "Specify chance of a component being labeled")

	seedText := "random"
	flags.StringVar(&seedText, "seed", seedText, "Random seed for reproducible output, or 'random' for a different pattern each run.")
	help := false
	flags.BoolVarP(&help, "help", "h", false, "Print this help.")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [flags]\n\nGenerate a sparse circuit-board motif made from hexagonal components, bundled traces, and hollow terminal pads. All dimension flags use inches.\n\nFlags:\n", flags.Name())
		flags.PrintDefaults()
	}
	// Parse flags first, then interpret the special seed value and reject positional
	// arguments. pflag's default help rendering includes each flag's default value.
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
	trackCountWeights, err := parseFloatList(trackCountWeightsText)
	if err != nil {
		return opts, fmt.Errorf("invalid --track-count-weights: %w", err)
	}
	opts.Generator.TrackCountWeights = trackCountWeights
	// Preserve the previous CLI behavior: if the user sets --routing-pitch, validate
	// that exact pitch; otherwise allow normalizeOptions to increase it when necessary.
	opts.Generator.AutoRoutingPitch = !flags.Changed("routing-pitch")
	if seedText != "random" {
		seed, err := strconv.ParseInt(seedText, 10, 64)
		if err != nil {
			return opts, fmt.Errorf("--seed must be an integer or 'random': %w", err)
		}
		opts.Generator.Seed, opts.Generator.SeedSet = seed, true
	}
	if flags.NArg() > 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	return opts, nil
}

// main is the CLI adapter: parse flags, call the core generator, write the returned SVG,
// and report how many candidate connections succeeded. GenerateSVG itself does no I/O.
func main() {
	opts, err := parseOptions(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	opts.Generator.FallbackEdgeOrder = circuit.FallbackEdgeOrderRanked

	// Keep I/O out of GenerateSVG: the core returns an SVG string and stats, while main
	// decides where to write it and how to present errors to a command-line user.
	svgText, accepted, _, err := circuit.Generate(opts.Generator)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	file, err := os.Create(opts.Output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening file:", err)
		os.Exit(1)
	}
	defer file.Close()

	_, err = io.WriteString(file, svgText)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error writing file:", err)
		os.Exit(1)
	}

	if accepted == 0 {
		fmt.Fprintln(os.Stderr, "Warning: no bundles fit the collision and routing constraints; try increasing the canvas or routing pitch.")
	}
}
