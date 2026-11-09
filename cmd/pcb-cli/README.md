# pcb-cli

`pcb-cli` is the command-line wrapper for [`generators/pcb`](../../generators/pcb). It generates randomized PCB-inspired artwork as an SVG file. The composition uses hexagonal components, branched signal traces, highlighted power feeds, and a row of connector contacts along the bottom edge.

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

Generate a larger, denser board with more clusters and connections:

```sh
./pcb-cli \
  --width 12 \
  --height 9 \
  --component-count 52 \
  --cluster-count 8 \
  --connections 26 \
  --seed 42 \
  --output dense-board.svg
```

Change the signal and highlight colors:

```sh
./pcb-cli \
  --base-color '#396CF8' \
  --accent-color '#FF6A00' \
  --background '#141314' \
  --output blue-and-orange.svg
```

Adjust the connector row:

```sh
./pcb-cli \
  --connector-count 17 \
  --connector-inset 0.3 \
  --output wide-connector-row.svg
```

## Composition

The generator places several major component clusters and connects them with an irregular, branched trace network. Remaining space is populated with smaller components, many of which are not connected. A horizontal row of connector components is always placed along the bottom edge.

`--component-count` is the requested number of generated components and does **not** include the mandatory connector row. `--connector-count` controls the number of contacts in that row.

The base color is used for ordinary components and signal traces. The accent color highlights power feeds and selected component cores. The background hexagonal field is disabled by default; enable it with `--show-background-grid`.

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
| `--accent-color` | `#FF6A00` | Color for power feeds and selected component cores. |
| `--background` | `none` | Canvas fill. Use `none` or `transparent` for no fill. |

### Components and clusters

| Flag | Default | Description |
| --- | ---: | --- |
| `--component-count` | `38` | Target number of generated components, excluding the connector row. |
| `--cluster-count` | `6` | Number of major component clusters. |
| `--pitch` | `0.50` | Center-to-center spacing of neighboring grid positions. |
| `--component-radius` | `0.26` | Circumradius of full-size component hexagons. |
| `--component-stroke-width` | `0.012` | Component outline width. |
| `--component-middle-hexagon-scale` | `0.60` | Middle component hexagon size relative to the outer radius. |
| `--component-inner-hexagon-scale` | `0.34` | Inner component hexagon size relative to the outer radius. |

The inner and middle scales must satisfy `0 < inner < middle < 1`. The pitch must be greater than 1.4 times the component radius.

### Traces and terminal pads

| Flag | Default | Description |
| --- | ---: | --- |
| `--connections` | `18` | Target number of routed trace bundles. |
| `--max-parallel` | `3` | Maximum number of parallel traces in a bundle; allowed values are 1 through 3. |
| `--trace-stroke-width` | `0.009` | Width of each signal trace. |
| `--trace-spacing` | `0.032` | Perpendicular centerline spacing between parallel traces. |
| `--node-radius` | `0.014` | Radius of hollow terminal pads. |

The requested connection count is a target; the generator's routing constraints may prevent every candidate connection from being placed. The connection count must be at least the cluster count plus one.

### Background grid

| Flag | Default | Description |
| --- | ---: | --- |
| `--show-background-grid` | `false` | Draw the background hexagonal field. |
| `--omit-partial-grid-hexagons` | `false` | Omit background hexagons whose stroked outlines would be clipped by the canvas edge. |
| `--grid-radius` | `0.26` | Circumradius of background grid hexagons. |
| `--grid-stroke-width` | `0.0035` | Background hexagon outline width; zero disables the outlines. |

`--omit-partial-grid-hexagons` affects only the background grid. Component placement and routing already avoid edge cells.

### Bottom connector row

| Flag | Default | Description |
| --- | ---: | --- |
| `--connector-count` | `13` | Number of contacts in the mandatory bottom row. |
| `--connector-inset` | `0.25` | Requested gap between connector centers and the bottom canvas edge. |

The connector count must be at least 3. The inset must be less than half the canvas height. Lattice snapping can result in a larger actual gap than the requested inset.

### Randomness and help

| Flag | Default | Description |
| --- | --- | --- |
| `--seed` | `random` | Integer seed for reproducible output, or `random` for a fresh pattern. |
| `-h, --help` | `false` | Print command help. |

## Troubleshooting

**The command rejects the options.** The generator validates dimensions and geometric relationships before rendering. Check the constraints listed above and run `--help` to verify flag names and defaults.

**Fewer trace bundles appear than requested.** `--connections` is a target, not a guarantee. Routing candidates can be rejected when they cannot satisfy the geometry and collision constraints. Try increasing the canvas size, reducing the connection target, or adjusting component spacing and size.

**The background grid is not visible.** The grid is off by default. Enable it with `--show-background-grid`; `--grid-stroke-width` must also be greater than zero for visible outlines.

**Repeated runs produce different boards.** Supply the same integer `--seed` and the same options to reproduce a layout.
