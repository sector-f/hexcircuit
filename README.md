# Hexagonal Circuit Motif Generator

A Go command-line program that generates an SVG illustration of a circuit-board-inspired pattern. The design combines a tiled hexagon grid, larger hexagonal components, bundles of angular traces, and hollow circular terminals.

The generator writes vector artwork rather than a raster image. SVG files can be opened in a browser and edited in vector-graphics applications such as Inkscape.

## 1. Build and run

```sh
cd cmd/hexagon-cli/
go build
./hexagon-cli -o circuit.svg
```

## 2. Understand the picture

It helps to think of the image as a background grid with a circuit network drawn over it.

### Canvas

The canvas is the rectangular area of the final SVG. `--width` and `--height` set its physical size in inches. The default canvas is 10 inches wide by 8 inches high.

### Grid hexagons

The background is a regular, repeating grid of hexagons. These hexagons provide the visual texture and the coordinate system used to place components and route traces.

Each grid hexagon is described by three settings:

- **Grid radius** is the distance from the hexagon's center to any vertex. It determines the hexagon's overall size.
- **Grid gap** is the intended visible gap between neighboring hexagon outlines.
- **Grid width** is the stroke thickness used to draw those outlines.

The grid is regular: all of its hexagons have the same radius and are positioned on the same lattice. The `--orientation` option controls whether the hexagons have a vertex pointing up (`pointy`) or a vertical flat side at the top (`flat`).

By default, partially clipped background hexagons are allowed at the canvas edges, so the pattern can continue naturally to the border. Use `--omit-partial-grid-hexagons` to draw only background hexagons whose entire stroked outline fits inside the canvas. This option affects only the background grid, not traces or components.

### Routing pitch: the distance between grid centers

**Routing pitch** is the distance between neighboring lattice centers. It controls how far apart the background hexagon centers are, and it also sets the step size available to component placement and trace routing.

A smaller pitch puts more hexagons on the canvas and creates a denser routing lattice. A larger pitch produces fewer, more widely separated hexagons.

You normally do not need to set routing pitch yourself. By default, the program calculates it from grid radius, grid gap, and grid line width so the visible gap is preserved when you resize the hexagons. The calculation is approximately:

```text
routing pitch = sqrt(3) × grid radius + grid gap + grid width
```

This calculation accounts for the distance across a regular hexagon between opposite sides, plus the desired gap and outline width.

If you provide `--routing-pitch` explicitly, the program uses that fixed center-to-center distance instead of calculating it. The explicit pitch must be large enough for the chosen grid radius, gap, and line width.

### Components

Components are the larger hexagons placed at selected lattice centers. They act as circuit elements or hubs. Some are larger and serve as focal points; others are smaller. Some large components may contain smaller, concentric hexagon outlines.

`--component-count` controls how many components the generator tries to place. `--component-radius` sets their maximum radius. By default, the maximum component radius scales with routing pitch, keeping component size in proportion with the grid as the grid changes. Set `--component-radius` explicitly when you want a fixed component size instead.

The program spreads components across the canvas and arranges them into a few loose clusters. It does not place a component in every background cell.

### Traces and trace bundles

Traces are the angular paths that connect components. They start at a side of one component, travel outward, follow the six directions of the hexagonal lattice, and terminate inside another component. Each route must contain at least one bend. Direction changes are constrained to 60-degree increments, giving a 120-degree interior angle at each bend.

The generator tries candidate routes and rejects routes that violate its geometry constraints, collide with other components, or cross or approach existing traces too closely. Because of this, the actual number of successful connections can be lower than the requested number.

A connection is a **bundle**: one or more parallel traces following a shared route. The bundle's size and spacing are controlled separately:

- **Max parallel** sets the largest number of traces in a bundle.
- **Track-count weights** set the relative likelihood of selecting each track count, from 1 up to max parallel.
- **Parallel spacing** sets the perpendicular, centerline-to-centerline distance between tracks in a bundle.
- **Trace width** controls the thickness of each trace.
- **Trace clearance** controls the minimum separation enforced between unrelated routes. Since traces have width, the visible edge-to-edge gap is smaller than the centerline clearance.

Not every requested track count is always geometrically possible. For example, a wide bundle might not fit on a small component side. The generator only considers track counts that fit the source and destination components. It renormalizes the configured weights over the feasible counts; if all feasible counts have zero weight, that route candidate is rejected.

### Terminal circles and details

Each accepted track has a hollow circular terminal inside its destination component. `--node-radius` controls the terminal circle radius. Terminal circles are drawn last so they remain visible over the traces.

`--detail-prob` controls how often sufficiently large components receive nested hexagon details. Hubs are more likely to receive details than ordinary components. Set the value to `0` to disable these details; increase it to make them more common.

## 3. The easiest way to resize the grid

To make the background hexagons smaller while keeping their visible gaps approximately the same, change only `--grid-radius`:

```sh
go run . --grid-radius 0.25 -o smaller_hexagons.svg
```

The program recalculates routing pitch automatically and scales the maximum component radius proportionally. This gives you a denser grid without requiring you to adjust several related settings manually.

To make the grid larger instead:

```sh
go run . --grid-radius 0.40 -o larger_hexagons.svg
```

You can adjust the gap independently:

```sh
go run . --grid-radius 0.25 --grid-gap 0.06 -o wider_gaps.svg
```

These commands leave routing pitch automatic. If you explicitly set `--routing-pitch`, automatic pitch calculation is disabled for that run. If you explicitly set `--component-radius`, automatic component scaling is disabled for that run. These overrides operate independently.

For example, this keeps routing pitch fixed while letting component radius continue to scale automatically with it:

```sh
go run . --routing-pitch 0.65 -o fixed_pitch.svg
```

This fixes component size while allowing routing pitch to be calculated from the grid settings:

```sh
go run . --component-radius 0.30 -o fixed_component_size.svg
```

The chosen dimensions still need to satisfy the generator's geometric checks. In particular, routing pitch must exceed 1.4 times component radius, and component radius must be larger than the terminal radius plus trace width.

## 4. Control the circuit network

Request more components and connections:

```sh
go run . \
  --component-count 24 \
  --connections 22 \
  --seed 42 \
  -o more_connections.svg
```

`--connections` is a target, not a guarantee. The generator only counts a connection after it finds a route that passes the geometry and collision checks. If the output contains fewer bundles than requested, try a larger canvas, a lower connection target, more route attempts, or more room between components.

To make bundles with four possible track counts, give `--max-parallel` and `--track-count-weights` the same number of entries:

```sh
go run . \
  --max-parallel 4 \
  --track-count-weights 5,15,35,45 \
  -o wider_bundles.svg
```

These weights correspond to 1, 2, 3, and 4 tracks, respectively. The values must contain exactly one percentage per possible track count and must add up to 100. If you change `--max-parallel`, also update the weights list; the default list contains three values because the default maximum is three tracks.

Use `--orientation flat` to switch from pointy-top to flat-top hexagons:

```sh
go run . --orientation flat -o flat_hexagons.svg
```

## 5. Randomness and reproducibility

By default, the program chooses a new random seed for each run, so running the same command again can produce a different composition.

Provide an integer seed to make the output reproducible for the same program version and options:

```sh
go run . --seed 42 -o repeatable.svg
```

Running the same command with `--seed 42` again reproduces the same generated pattern. Use `--seed random` or omit `--seed` to get a new pattern.

## 6. Command-line options

Run `go run . --help` to see the options and defaults reported by the program. The main options are grouped below by what they control. Unless noted otherwise, all dimensions are in inches.

### Output and canvas

| Flag | Default shown by `--help` | Meaning |
|---|---:|---|
| `-o, --output` | `circuit_motif.svg` | File to write. |
| `-W, --width` | `10` | Canvas width in inches. |
| `-H, --height` | `8` | Canvas height in inches. |
| `--background` | `none` | Background fill. `none` or `transparent` leaves the SVG background transparent. |
| `--color` | `#C7834A` | Stroke color used by the grid, components, traces, and terminal circles. |
| `--orientation` | `pointy` | Hexagon orientation: `pointy` or `flat`. |

### Grid and component layout

| Flag | Default shown by `--help` | Meaning |
|---|---:|---|
| `--grid-radius` | `0.33255` | Background hexagon radius, measured from its center to a vertex. |
| `--grid-gap` | `0.04` | Desired gap between neighboring background hexagon outlines. |
| `--grid-width` | `0.004` | Background hexagon outline thickness. |
| `--omit-partial-grid-hexagons` | `false` | Omit background hexagons whose stroked outlines would be clipped at the canvas edge. |
| `--routing-pitch` | `0.62` | Explicit center-to-center lattice spacing. If omitted, it is calculated from grid radius, gap, and grid width. |
| `--component-count` | `18` | Number of components to place. |
| `--component-radius` | `0.4` | Explicit maximum component radius. If omitted, it scales with routing pitch. |

The `--routing-pitch` and `--component-radius` defaults shown above are the default values bound to those flags. In normal use, the program automatically derives both values because neither flag has been explicitly supplied. This distinction lets the values adjust when you resize the grid.

### Traces and terminals

| Flag | Default shown by `--help` | Meaning |
|---|---:|---|
| `--connections` | `18` | Target number of accepted trace bundles. Some candidates may be rejected. |
| `--max-parallel` | `3` | Maximum number of parallel tracks in a bundle. |
| `--track-count-weights` | `11.1111,44.44445,44.44445` | Percentage weights for bundle sizes of 1, 2, and 3 tracks. Must contain exactly `--max-parallel` values and total 100. |
| `--parallel-spacing` | `0.035` | Perpendicular centerline spacing between parallel tracks. |
| `--trace-width` | `0.008` | Trace stroke thickness. |
| `--trace-clearance` | `0.025` | Minimum clearance used when checking separate trace routes. |
| `--node-radius` | `0.014` | Radius of hollow terminal circles. |
| `--stroke-width` | `0.012` | Component outline thickness. |

### Routing and detail

| Flag | Default shown by `--help` | Meaning |
|---|---:|---|
| `--route-attempts` | `12` | Number of route attempts made for a candidate component pair. |
| `--max-route-steps` | `24` | Maximum number of lattice steps in a candidate route. |
| `--detail-prob` | `0.48` | Controls how often eligible components receive nested hexagon details; hubs are more likely to receive them. |
| `--seed` | `random` | Integer seed for reproducible output, or `random` for a new pattern. |
| `-h, --help` | `false` | Print help and exit. |

## 7. Troubleshooting

**The output has fewer connections than requested.** This is allowed: impossible or colliding routes are rejected. Try increasing the canvas size, reducing `--connections`, increasing `--route-attempts`, or adjusting component and grid spacing.

**The program rejects my routing pitch.** The pitch may be too small for the requested grid radius, gap, and grid line width. Usually the simplest solution is to omit `--routing-pitch` and let the program calculate it. If you need a fixed pitch, increase it until it satisfies the geometric constraints.

**I changed `--max-parallel` and validation failed.** `--track-count-weights` must have one percentage for each possible track count. For `--max-parallel 4`, provide four values, such as `10,20,30,40`.

**The pattern changes between runs.** Supply a fixed integer to `--seed` to reproduce a layout.

**The background grid is clipped at the edges.** This is the default behavior. Add `--omit-partial-grid-hexagons` to keep only complete background hexagons. Trace placement and component rendering are not changed by this flag.

## 8. Notes on output

The SVG stores its physical width and height in inches. Internally, the generator uses 1,000 SVG user units per inch to preserve fine line-width and spacing values while drawing with the integer-coordinate API provided by SVGo.

The generated SVG is layered in this order: optional background fill, background grid, traces, component outlines and details, then terminal circles. Later layers are drawn over earlier ones. The generator returns SVG text from `GenerateSVG`; the command-line wrapper is responsible for writing the text to the requested file.
