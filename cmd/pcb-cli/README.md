![PCB example image](example.svg)

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
  --decorative-trace-count 8 \
  --decorative-trace-min-length 4 \
  --decorative-trace-max-length 10 \
  --seed 42 \
  --output dense-board.svg
```

Configure both power-source rows, leave visible gaps between active sources, and constrain the components to the horizontal span between the active sources:

```sh
./pcb-cli \
  --top-power-source-count 10 \
  --bottom-power-source-count 10 \
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
  --bottom-power-source-count 10 \
  --output bottom-powered.svg
```

Change the signal and power colors:

```sh
./pcb-cli \
  --base-color '#396CF8' \
  --grid-color '#243c75' \
  --accent-color '#FF6A00' \
  --background '#141314' \
  --output blue-and-orange.svg
```

## Composition

The generator first assigns each cluster a random number of active contacts within the configured per-cluster minimum and maximum, then places compact component clusters and routes each selected feed to a randomly selected component in its assigned cluster. Source contacts are matched to clusters from left to right. A cluster may have no active feed. When a cluster has multiple feeds, distinct components are chosen as destinations before any component is reused.

Selected power-source contacts receive an accent-colored core and a power feed. Every configured physical contact is rendered. In `separated` mode, one empty lattice cell lies between neighboring contacts; in `adjacent` mode, contacts occupy neighboring cells.

`--inter-cluster-traces` optionally enables a randomized number of accent-colored traces between components in different clusters. The generator may produce no such traces for a given seed or layout.

`--decorative-trace-count` requests best-effort decorative base-color bundles that run alongside nearby power-feed or inter-cluster traces and end at hollow terminal pads. Each starts at a cluster leaf component, finds the nearest trace within two grid steps, steps onto a one-cell-offset copy, then follows that route. Decorative traces are generated after all other routes, and previously placed decorative traces can serve as sources for additional parallel runs. The default length range is 4–10 grid steps, with up to 3 bends; the line count is randomized up to `--max-parallel`. Paths blocked by components, existing power/inter-cluster/decorative routes, or the canvas boundary are skipped. Use `--decorative-trace-min-length` and `--decorative-trace-max-length` to configure the inclusive length range in grid steps, and `--decorative-trace-max-bends` to cap the number of bends. If no power-feed or inter-cluster routes exist, no decorative traces can be placed.

`--component-count` controls the number of ordinary components distributed among clusters. It excludes power-source row contacts. `--cluster-count` determines the number of clusters. `--min-components-per-cluster` sets the minimum number of ordinary components in every cluster; the default of `1` permits singleton clusters. `--min-power-sources-per-cluster` and `--max-power-sources-per-cluster` set the inclusive range of active power feeds per cluster. Both default to `1`; a minimum of `0` permits unpowered clusters, and a maximum of `6` is currently supported.

When `--constrain-components-to-power-source-span` is enabled, every ordinary component and every signal/power route stays horizontally between the leftmost and rightmost active power-source contacts. The option is disabled by default. Narrow source spans may leave insufficient room for the requested component count, in which case generation returns an error rather than placing components outside the requested bounds.

Power-source count flags describe the number of physical contact components on each edge, not the number that will be active. Active contacts are assigned to clusters according to the configured minimum and maximum feeds per cluster; multiple feeds within a cluster are routed to different components before destinations are reused. In `separated` mode, neighboring physical contacts are placed two lattice cells apart. In `adjacent` mode, they occupy neighboring lattice cells.

The base color is used for ordinary components, all source-contact outlines (including unused contacts), and signal traces. When either cluster-radius bound is set, every cluster component is randomized within the effective range; both bounds set to zero preserve the existing sizing behavior. All power-source contacts retain the fixed component radius. The optional background grid uses the grid color, which defaults to the base color unless --grid-color is set. The accent color is used for power feeds and their connected power-entry cores at source contacts and selected destination components, plus optional inter-cluster traces. The background hexagonal field is disabled by default; enable it with `--show-background-grid`. Base and accent neon glow are independently disabled by default. Use `--base-glow` and `--accent-glow` to enable them separately; each has its own blur-radius and opacity settings. Glow does not affect the background grid.

The generator uses randomized placement and routing. It first makes up to `--route-attempts` complete mandatory power-feed routing passes for each placement, and it tries up to `--generation-attempts` complete board generations if randomized placement/routing is exhausted. The latter defaults to 4 total attempts, including the original seed. Fallback seeds are derived deterministically from the first seed, so supplying an integer `--seed` keeps the entire retry sequence reproducible for the same generator version and options. Static validation and geometry errors are not retried. Without `--seed`, each run selects a fresh master seed.

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
| `--accent-color` | `#FF6A00` | Color for power feeds, connected power-entry cores, and optional inter-cluster traces. |
| `--background` | `none` | Canvas fill. Use `none` or `transparent` for no fill. |
| `--base-glow` | `false` | Enable a soft neon bloom on base-color components and signal traces. |
| `--base-glow-blur-radius` | `0.05` | Base-color glow Gaussian blur standard deviation, in inches. |
| `--base-glow-opacity` | `0.55` | Base-color glow opacity, from `0` to `1`. |
| `--accent-glow` | `false` | Enable a soft neon bloom on accent-color power and inter-cluster traces. |
| `--accent-glow-blur-radius` | `0.05` | Accent-color glow Gaussian blur standard deviation, in inches. |
| `--accent-glow-opacity` | `0.55` | Accent-color glow opacity, from `0` to `1`. |

### Components and clusters

| Flag | Default | Description |
| --- | ---: | --- |
| `--component-count` | `38` | Number of ordinary components distributed among clusters, excluding power contacts. |
| `--cluster-count` | `6` | Number of component clusters. |
| `--min-components-per-cluster` | `1` | Minimum number of ordinary components in each cluster. |
| `--min-power-sources-per-cluster` | `1` | Minimum active power feeds per cluster; 0 permits unpowered clusters. |
| `--max-power-sources-per-cluster` | `1` | Maximum active power feeds per cluster; values up to 6 are supported. |
| `--constrain-components-to-power-source-span` | `false` | Restrict components and all routes to the horizontal span between the leftmost and rightmost active power contacts. |
| `--constrain-layout-to-power-source-trapezoid` | `false` | Restrict components and all routes to the trapezoid connecting the left/right extents of the top and bottom source rows. Requires both rows. |
| `--pitch` | `0.50` | Center-to-center spacing of neighboring grid positions. |
| `--component-radius` | `0.26` | Base cluster-component circumradius and fallback for either unset cluster-radius bound. |
| `--power-source-component-radius` | `0.26` | Fixed circumradius for physical power-source contacts. |
| `--component-min-radius` | `0` | Minimum randomized cluster-component circumradius; zero falls back to `--component-radius`. |
| `--component-max-radius` | `0` | Maximum randomized cluster-component circumradius; zero falls back to `--component-radius`; both zero preserve legacy sizing. |
| `--component-stroke-width` | `0.012` | Component outline width. |
| `--component-middle-hexagon-scale` | `0.60` | Middle component hexagon size relative to the outer radius. |
| `--component-inner-hexagon-scale` | `0.34` | Inner component hexagon size relative to the outer radius. |

The inner and middle scales must satisfy `0 < inner < middle < 1`. Setting either radius bound randomizes every cluster component; an unset bound falls back to `--component-radius`. When both bounds are zero, the existing full-size center and randomized outer-component sizes are preserved. Power-source contacts use `--power-source-component-radius`, which defaults to the same value as `--component-radius`. Pitch validation and source-contact checks use the actual pointy-top hexagon outlines to prevent overlap. The minimum cluster size must be at least one, and `component-count` must be large enough to satisfy the requested minimum across all clusters.

### Traces and terminal pads

| Flag | Default | Description |
| --- | ---: | --- |
| `--connections` | `18` | Target number of additional intra-cluster signal connections beyond mandatory spanning-tree connections. |
| `--decorative-trace-count` | `0` | Best-effort count of decorative traces; zero disables them. |
| `--decorative-trace-min-length` | `4` | Minimum decorative trace length in grid steps. |
| `--decorative-trace-max-length` | `10` | Maximum decorative trace length in grid steps. |
| `--decorative-trace-max-bends` | `3` | Maximum number of direction changes per decorative trace. |
| `--inter-cluster-traces` | `false` | Enable optional randomized accent-colored traces between clusters. |
| `--max-parallel` | `3` | Maximum requested tracks in a bundle; power feeds automatically use fewer when an endpoint core is too small. |
| `--trace-stroke-width` | `0.009` | Width of each signal trace. |
| `--trace-spacing` | `0.032` | Perpendicular centerline spacing between parallel traces. |
| `--node-radius` | `0.014` | Radius of hollow terminal pads. |
| `--route-attempts` | `12` | Complete randomized power-feed routing passes per component placement. |
| `--generation-attempts` | `4` | Complete board-generation attempts using deterministic fallback seeds; includes the initial seed and accepts values from 1 to 20. |

Power-feed line counts are randomized up to the capacity of both the fixed source core and selected destination core, capped by `--max-parallel`. Smaller components that cannot fit one track are skipped as destinations; larger viable components are preferred. A three-track request can therefore fall back to two or one without requiring a separate option change. The requested decorative-trace count is best-effort. Traces start at cluster leaf components near a power-feed, inter-cluster, or earlier decorative route, then run parallel to it at a one-cell offset and end at hollow pads in empty cells. Earlier decorative traces can become sources for later ones. Their line count is randomized up to `--max-parallel`; the default length range is 4–10 grid steps and the default maximum is 3 bends. The requested connection count is a target; the generator's routing constraints may prevent every candidate connection from being placed. The connection count must be nonnegative. `--route-attempts` controls complete randomized passes over mandatory power feeds for one placement. `--generation-attempts` controls how many different deterministic seeds may be tried for the entire board when placement/routing exhausts its internal search; `1` disables seed fallback.

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
| `--top-power-source-count` | `10` | Number of physical power contacts along the top edge. |
| `--bottom-power-source-count` | `10` | Number of physical power contacts along the bottom edge. |
| `--power-source-spacing` | `separated` | `separated` places physical contacts two lattice cells apart; `adjacent` places them in neighboring cells. |
| `--power-source-style` | `single` | Base-layer style for all contacts: `single` or concentric `double` hexagons. |
| `--connector-inset` | `0.25` | Requested gap between each enabled source row and its nearest canvas edge. |
| `--connector-count` | `10` | Deprecated alias for `--bottom-power-source-count`. |

The two row counts may individually be zero. Their sum must be at least `cluster-count × min-power-sources-per-cluster`; contacts beyond the selected number remain visible but unused. When the minimum is zero, no physical power contacts are required even when the maximum is nonzero. Each enabled row must fit on its corresponding canvas edge. In separated mode, each physical contact occupies every other lattice position.

### Randomness and help

| Flag | Default | Description |
| --- | ---: | --- |
| `--seed` | `random` | Integer seed for reproducible output, or `random` for a fresh pattern. |
| `-h, --help` | `false` | Print this help. |

## Troubleshooting

**The command rejects the options.** The generator validates dimensions, source-row capacity for the minimum feed count, cluster minimums, and geometric relationships before rendering. Check that the configured contact count is at least `cluster-count × min-power-sources-per-cluster`, and run `--help` to verify flag names and defaults.

**Generation cannot fit the components inside the power-source span or trapezoid.** Disable the relevant constraint, increase the canvas width or source-row counts, or reduce the component count/cluster sizes. The trapezoid option also requires both power-source rows.

**Fewer additional signal traces appear than requested.** `--connections` is a target, not a guarantee. Routing candidates can be rejected when they cannot satisfy the geometry and collision constraints. Try increasing the canvas size or reducing the connection target.

**The background grid is not visible.** The grid is off by default. Enable it with `--show-background-grid`; `--grid-stroke-width` must also be greater than zero for visible outlines.

**Repeated runs produce different boards.** Supply the same integer `--seed` and the same options to reproduce a layout.
