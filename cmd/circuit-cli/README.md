# circuit-cli

`circuit-cli` is the command-line interface for [`generators/circuit`](../../generators/circuit/README.md). It creates a randomized circuit-board-inspired illustration and writes the result as an SVG file.

The CLI handles command-line parsing and file output. The generator package owns the composition and routing algorithm; it can also be called directly from Go without using this executable.

## Build and run

From the repository root:

```sh
go build -o circuit-cli ./cmd/circuit-cli
./circuit-cli -o circuit.svg
```

You can also run it without building a separate binary:

```sh
go run ./cmd/circuit-cli -o circuit.svg
```

See all flags and their current defaults with:

```sh
go run ./cmd/circuit-cli --help
```

The default output filename is `circuit_motif.svg`. The default canvas is 10 inches wide by 8 inches high. All dimension and spacing flags are expressed in inches.

## Examples

Generate a larger, denser composition:

```sh
./circuit-cli \
  --width 14 \
  --height 10 \
  --component-count 28 \
  --connections 30 \
  --seed 42 \
  -o dense-circuit.svg
```

Use a dark background and a blue stroke:

```sh
./circuit-cli \
  --background '#141314' \
  --color '#396cf8' \
  --grid-color '#263b70' \
  --seed 42 \
  -o blue-circuit.svg
```

Use flat-top hexagons and omit partially clipped background cells:

```sh
./circuit-cli \
  --orientation flat \
  --omit-partial-grid-hexagons \
  -o flat-circuit.svg
```

Enable component labels:

```sh
./circuit-cli \
  --show-labels \
  --label-font 'DejaVu Sans Mono' \
  --label-probability 0.6 \
  -o labeled-circuit.svg
```

Use a fixed seed to reproduce an output:

```sh
./circuit-cli --seed 2026 -o repeatable.svg
```

Omit `--seed` or pass `--seed random` to generate a fresh composition.

## Options

Run `--help` for the exact defaults printed by the executable. The following tables summarize the available flags.

### Output and canvas

| Flag | Default | Description |
| --- | --- | --- |
| `-o`, `--output` | `circuit_motif.svg` | Destination SVG filename. |
| `-W`, `--width` | `10` | Canvas width in inches. |
| `-H`, `--height` | `8` | Canvas height in inches. |
| `--background` | `none` | Canvas fill. `none` or `transparent` leaves the background transparent. |
| `--color` | `#C7834A` | Stroke color for components, traces, and terminal pads. |
| `--grid-color` | (same as `--color`) | Background hexagon grid color. |
| `--orientation` | `pointy` | Hexagon orientation: `pointy` or `flat`. |

### Grid and component layout

| Flag | Default | Description |
| --- | --- | --- |
| `--component-count` | `18` | Target number of components to place. |
| `--component-radius` | `0.4` | Maximum component radius in inches. Individual components may be smaller. |
| `--routing-pitch` | `0.62` | Center-to-center spacing of neighboring lattice positions, in inches. See the automatic adjustment note below. |
| `--grid-radius` | `0.33255` | Circumradius of background hexagons, in inches. |
| `--grid-gap` | `0.04` | Intended visible gap between neighboring background hexagon outlines, in inches. |
| `--grid-width` | `0.004` | Background hexagon outline width, in inches. |
| `--omit-partial-grid-hexagons` | `false` | Omit background hexagons whose outlines would be clipped by the canvas edge. |

By default, the CLI allows the generator to increase the routing pitch when necessary to accommodate the selected grid radius, gap, and outline width. Supplying `--routing-pitch` explicitly disables that automatic increase: the supplied value is validated as-is.

### Connections and drawing style

| Flag | Default | Description |
| --- | --- | --- |
| `--connections` | `18` | Target number of accepted component-to-component trace bundles. |
| `--max-parallel` | `3` | Maximum number of parallel tracks in one bundle. |
| `--track-count-weights` | `11.1111,44.44445,44.44445` | Percentage weights for bundles containing 1 through `--max-parallel` tracks. |
| `--parallel-spacing` | `0.035` | Perpendicular centerline spacing between tracks in a bundle, in inches. |
| `--stroke-width` | `0.012` | Component outline width, in inches. |
| `--trace-width` | `0.008` | Trace and terminal outline width, in inches. |
| `--node-radius` | `0.014` | Radius of hollow terminal pads, in inches. |
| `--trace-clearance` | `0.025` | Minimum requested centerline clearance between separate routes, in inches. |
| `--detail-prob` | `0.48` | Base probability that an eligible component receives inner hexagonal details. |

`--track-count-weights` must contain exactly one percentage for every possible bundle size from 1 through `--max-parallel`, and the percentages must total 100. For example, `--max-parallel 4` requires four values.

### Routing and labels

| Flag | Default | Description |
| --- | --- | --- |
| `--max-route-steps` | `24` | Maximum number of lattice steps explored in a candidate route. |
| `--route-attempts` | `12` | Alternative route attempts for a candidate component pair and direction. |
| `--show-labels` | `false` | Enable generated component labels. |
| `--label-font` | `monospace` | Font family used for labels. The font is not embedded in the SVG. |
| `--label-probability` | `0.35` | Probability that an eligible component receives a label. |
| `--seed` | `random` | Integer seed for repeatable generation, or `random` for a fresh pattern. |
| `-h`, `--help` | `false` | Print usage and flag descriptions. |

## How to interpret the output

The image combines a regular hexagonal background grid with larger hexagonal components and routed connections. Components are placed in loose clusters rather than filling every grid cell. Connections are drawn as one or more parallel tracks, and each track normally ends in a hollow circular terminal pad.

The requested connection count is a target, not a guarantee. The generator rejects routes that cannot satisfy its geometry and collision constraints. As a result, an output can contain fewer bundles than requested without being an error.

## Troubleshooting

**Fewer connections were generated than requested.** Increase the canvas or routing pitch, reduce `--connections`, increase `--route-attempts` or `--max-route-steps`, or give components and traces more room. Some component pairs may not admit a valid route under the current constraints.

**The routing pitch is rejected.** The pitch must leave room for the background hexagons and their gap and stroke. Usually the simplest approach is to omit `--routing-pitch` and let the CLI adjust it automatically. An explicitly supplied pitch must satisfy the geometric constraints.

**The track-count weights are rejected.** Supply exactly `--max-parallel` comma-separated percentages whose sum is 100. For example, `--max-parallel 4 --track-count-weights 10,20,30,40`.

**The result changes between runs.** Supply an integer `--seed`. Repeating the same seed with the same options and program version reproduces the composition.

**The grid is clipped at the canvas edges.** That is the default. Use `--omit-partial-grid-hexagons` to draw only complete background hexagons. This flag changes only the background grid; it does not change component or trace placement.

## Related documentation

- [Repository and `hexgrid` library](../../README.md)
- [`generators/circuit` package](../../generators/circuit/README.md)
