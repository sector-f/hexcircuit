# pcb-cli

`pcb-cli` is the command-line wrapper for [`generators/pcb`](../../generators/pcb). It generates randomized PCB-inspired artwork as an SVG file. The composition uses compact component clusters, connected local signal traces, and power feeds originating from contacts along the top and/or bottom edge.

All dimensional options are expressed in inches.

## Build

From the repository root:

```sh
go build -o pcb-cli ./cmd/pcb-cli
```

Or from this directory:

```sh
go build
```

## Basic usage

Generate an SVG using the default options:

```sh
./pcb-cli
```

The default output filename is `pcb.svg`. Choose another filename with `-o` or `--output`:

```sh
./pcb-cli --output board.svg
```

Run the command without building a binary:

```sh
go run . --output board.svg
```

## Examples

Enable the background hexagonal field and use a fixed seed to make the result reproducible:

```sh
./pcb-cli \
  --show-background-grid \
  --seed 42 \
  --output board.svg
```

Generate a larger board with more clusters and larger minimum cluster sizes:

```sh
./pcb-cli \
  --width 12 \
  --height 9 \
  --component-count 52 \
  --cluster-count 8 \
  --min-components-per-cluster 2 \
  --connections 26 \
  --seed 42 \
  --output dense-board.svg
```

Configure both power-source rows, leave visible gaps between active sources, and constrain the components to the horizontal span between the active sources:

```sh
./pcb-cli \
  --top-power-source-count 13 \
  --bottom-power-source-count 13 \
  --power-source-spacing separated \
  --power-source-style single \
  --constrain-components-to-power-source-span \
  --seed 42 \
  --output balanced-board.svg
```

Use adjacent double-hexagon power contacts instead:

```sh
./pcb-cli \
  --power-source-spacing adjacent \
  --power-source-style double \
  --output dense-power.svg
```

Only one edge needs to provide power. Set the count for the other edge to zero:

```sh
./pcb-cli \
  --top-power-source-count 0 \
  --bottom-power-source-count 13 \
  --output bottom-powered.svg
```

Change the signal and power colors:

```sh
./pcb-cli \
  --base-color '#396CF8' \
  --accent-color '#FF6A00' \
  --background '#141314' \
  --output blue-and-orange.svg
```

## Composition

The generator first chooses active contacts from the available top and bottom rows, then places compact component clusters and routes one power feed to each cluster. The active source contacts are matched to cluster hubs from left to right, while the selected active contacts and their top/bottom origins vary with the random seed. Power routes are routed without crossing one another.

Selected power-source contacts receive an accent-colored core and a power feed. In `separated` mode, inactive positions between active contacts on the same row are left empty so the gap is visible; unused contacts outside the active span remain rendered in the base color. In `adjacent` mode, all configured source positions are rendered.

`--component-count` controls the number of ordinary components distributed among clusters. It excludes power-source row contacts. `--cluster-count` determines the number of clusters and therefore the number of active power feeds. `--min-components-per-cluster` sets the minimum number of ordinary components in every cluster; the default of `1` permits singleton clusters.

When `--constrain-components-to-power-source-span` is enabled, every ordinary component must lie horizontally between the leftmost and rightmost active power-source contacts. The option is disabled by default. Narrow source spans may leave insufficient room for the requested component count, in which case generation returns an error rather than placing components outside the requested bounds.

Power-source count flags describe the number of potential contact positions on each edge, not the number that will be active. Exactly one position per cluster is selected as active. In `separated` mode, at least one inactive position is left between active contacts on the same row, and those intermediate positions are not rendered as components. Other unused contacts remain visible in the base color. In `adjacent` mode, active contacts may occupy neighboring positions and all configured positions are rendered.

The base color is used for ordinary components, all source-contact outlines (including unused contacts), signal traces, and the optional background grid. The accent color is reserved exclusively for power feeds and their connected power-entry cores. The background hexagonal field is disabled by default; enable it with `--show-background-grid`.

The generator uses randomized placement and routing. Without `--seed`, each run selects a fresh seed. Supplying an integer seed makes the output reproducible for the same generator version and options:

```sh
./pcb-cli --seed 1234 --output repeatable.svg
```

Use `--seed random` to explicitly request a new random result.

## Command-line options

Run `./pcb-cli --help` for the complete flag list and defaults reported by the program. Unless noted otherwise, dimensions and distances are in inches.

### Output and canvas

| Flag | Default | Description |
| --- | ---: | --- |
| `-o, --output` | `pcb.svg` | SVG file to write. |
| `-W, --width` | `10` | Canvas width. |
| `-H, --height` | `8` | Canvas height. |
| `--base-color` | `#396CF8` | Color for ordinary components and signal traces. |
| `--accent-color` | `#FF6A00` | Color exclusively for power feeds and connected power-entry cores. |
| `--background` | `none` | Canvas fill. Use `none` or `transparent` for no fill. |

### Components and clusters

| Flag | Default | Description |
| --- | ---: | --- |
| `--component-count` | `38` | Number of ordinary components distributed among clusters, excluding active power contacts. |
| `--cluster-count` | `6` | Number of clusters and active power feeds. |
| `--min-components-per-cluster` | `1` | Minimum number of ordinary components in each cluster. |
| `--constrain-components-to-power-source-span` | `false` | Restrict ordinary components to the horizontal span between the leftmost and rightmost active power contacts. |
| `--pitch` | `0.50` | Center-to-center spacing of neighboring grid positions. |
| `--component-radius` | `0.26` | Circumradius of full-size component hexagons. |
| `--component-stroke-width` | `0.012` | Component outline width. |
| `--component-middle-hexagon-scale` | `0.60` | Middle component hexagon size relative to the outer radius. |
| `--component-inner-hexagon-scale` | `0.34` | Inner component hexagon size relative to the outer radius. |

The inner and middle scales must satisfy `0 < inner < middle < 1`. The pitch must be greater than 1.4 times the component radius. The minimum cluster size must be at least one, and `component-count` must be large enough to satisfy the requested minimum across all clusters.

### Traces and terminal pads

| Flag | Default | Description |
| --- | ---: | --- |
| `--connections` | `18` | Target number of additional intra-cluster signal connections beyond mandatory spanning-tree connections. |
| `--max-parallel` | `3` | Maximum number of parallel traces in a bundle; allowed values are 1 through 3. |
| `--trace-stroke-width` | `0.009` | Width of each signal trace. |
| `--trace-spacing` | `0.032` | Perpendicular centerline spacing between parallel traces. |
| `--node-radius` | `0.014` | Radius of hollow terminal pads. |

The requested connection count is a target; the generator's routing constraints may prevent every candidate connection from being placed. The connection count must be nonnegative.

### Background grid

| Flag | Default | Description |
| --- | ---: | --- |
| `--show-background-grid` | `false` | Draw the background hexagonal field. |
| `--omit-partial-grid-hexagons` | `false` | Omit background hexagons whose stroked outlines would be clipped by the canvas edge. |
| `--grid-radius` | `0.26` | Circumradius of background grid hexagons. |
| `--grid-stroke-width` | `0.0035` | Background hexagon outline width; zero disables the outlines. |

`--omit-partial-grid-hexagons` affects only the background grid. Component placement and routing already avoid edge cells.

### Power-source rows

| Flag | Default | Description |
| --- | ---: | --- |
| `--top-power-source-count` | `13` | Number of visible contact components along the top edge. |
| `--bottom-power-source-count` | `13` | Number of visible contact components along the bottom edge. |
| `--power-source-spacing` | `separated` | `separated` leaves at least one empty position between active contacts and omits those positions from rendering; `adjacent` permits neighboring active contacts and renders all configured positions. |
| `--power-source-style` | `single` | Base-layer style for all contacts: `single` or concentric `double` hexagons. |
| `--connector-inset` | `0.25` | Requested gap between each enabled source row and its nearest canvas edge. |
| `--connector-count` | `13` | Deprecated alias for `--bottom-power-source-count`. |

The two row counts may individually be zero, but their sum must be at least `cluster-count`. Each enabled row must fit on its corresponding canvas edge. In separated mode, the combined rows must also have enough potential positions to leave an unused contact between active contacts on the same row.

### Randomness and help

| Flag | Default | Description |
| --- | ---: | --- |
| `--seed` | `random` | Integer seed for reproducible output, or `random` for a fresh pattern. |
| `-h, --help` | `false` | Print this help. |

## Troubleshooting

**The command rejects the options.** The generator validates dimensions, source-row capacity, cluster minimums, and geometric relationships before rendering. Check the constraints above and run `--help` to verify flag names and defaults.

**Generation cannot fit the components inside the power-source span.** Disable `--constrain-components-to-power-source-span`, increase the width or source-row counts, or reduce the component count/cluster sizes.

**Fewer additional signal traces appear than requested.** `--connections` is a target, not a guarantee. Routing candidates can be rejected when they cannot satisfy the geometry and collision constraints. Try increasing the canvas size or reducing the connection target.

**The background grid is not visible.** The grid is off by default. Enable it with `--show-background-grid`; `--grid-stroke-width` must also be greater than zero for visible outlines.

**Repeated runs produce different boards.** Supply the same integer `--seed` and the same options to reproduce a layout.
