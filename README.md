# hexgrid

`hexgrid` is a Go library for describing hexagonal-grid artwork and rendering it as SVG. It provides the coordinate system, reusable geometry, scene model, and renderer used by the procedural generators in this repository.

The repository separates three concerns:

- **`hexgrid`** is the reusable geometry and rendering package. It knows how to position hexagons, components, traces, labels, and terminal pads, but it does not decide what an illustration should contain.
- **`generators/`** contains procedural composition algorithms that use `hexgrid` to create particular kinds of artwork. See [`generators/circuit`](generators/circuit/README.md).
- **`cmd/`** contains executable programs that expose those generators through user interfaces, including [`circuit-cli`](cmd/circuit-cli/README.md), [`pcb-cli`](cmd/pcb-cli/README.md), and [`pcb-web`](cmd/pcb-web/README.md).

The package produces vector graphics. Its output is SVG, which can be displayed in a browser or edited in a vector-graphics application such as Inkscape.

## Requirements

- Go 1.25 or later.

Import the library with:

```go
import "github.com/sector-f/hexgrid"
```

## The hexagonal grid

A grid is a regular lattice of equally sized hexagons. Each cell has a center, and the distance between neighboring centers is the **grid pitch**. The visible hexagons are drawn around those centers using the configured radius and stroke width.

The `hexgrid.Grid` type describes this lattice:

| Field | Meaning |
| --- | --- |
| `Pitch` | Center-to-center distance between neighboring cells, in inches. |
| `Radius` | Distance from a hexagon's center to one of its vertices, in inches. |
| `StrokeWidth` | Width of the background hexagon outlines, in inches. |
| `Color` | Background grid color; an empty value falls back to `Scene.Color`. |
| `OmitPartial` | When true, omit background hexagons whose stroked outlines would extend beyond the canvas. |

Grid pitch and hexagon radius are independent. The pitch determines the spacing of the coordinate lattice; the radius determines the size of the visible background hexagons. The radius, pitch, and stroke width should be chosen so neighboring outlines do not overlap unless that appearance is intentional.

The grid can use either `pointy` or `flat` hexagons. A pointy hexagon has a vertex at the top; a flat hexagon has a side at the top and vertices pointing left and right.

## Coordinate system

Grid positions use **axial coordinates**, represented by the pair `(Q, R)` in `hexgrid.GridPosition`. The origin `(0, 0)` is at the center of the canvas. Coordinates describe lattice locations, not distances in inches or SVG pixels.

For the pointy-top layout, increasing `Q` moves east and increasing `R` moves southeast. The third cube coordinate is implied:

```text
S = -Q - R
```

The six adjacent-cell directions are:

| `ΔQ` | `ΔR` | Direction |
| ---: | ---: | --- |
| 1 | 0 | east |
| 0 | 1 | southeast |
| -1 | 1 | southwest |
| -1 | 0 | west |
| 0 | -1 | northwest |
| 1 | -1 | northeast |

For example, `GridPosition{Q: -1, R: 0}` is one cell west of the origin. `GridPosition{Q: 0, R: 2}` is two cells southeast. `GridPositionToPoint` converts an axial position into a model-space `Point`, measured in inches, using the canvas dimensions, grid pitch, and orientation.

The same axial positions remain valid when changing the orientation. The orientation changes the rendered hexagon geometry; callers do not need to redesign their coordinate layout.

## Components

A `hexgrid.GridComponent` places a component at one grid position. Components are rendered as larger hexagons centered on their assigned cells. They are visual elements and connection anchors; the library does not assign them electrical meanings.

`ComponentStyle` controls the number of concentric outlines:

- `ComponentStyleSingle` draws one hexagon.
- `ComponentStyleDouble` draws two concentric hexagons.
- `ComponentStyleTriple` draws three concentric hexagons.

`Options.ComponentRadius` sets the outer radius for components. A component's optional `RadiusScale` multiplies that base radius; a zero value uses a scale of `1`. Component outline width and the scales of the inner outlines are configurable through `Options`.

A component is identified in a grid layout by its `GridPosition`. A `GridTrace` must start at a position occupied by a component.

## Traces

A `hexgrid.GridTrace` describes a connection that starts at a component and follows a series of grid-axis segments. Its `Path` contains one or more waypoints after `Start`.

Each segment must follow one of the three axial grid axes. A segment can span more than one cell, so intermediate cells do not need to be listed. A turn is introduced by adding another waypoint. For two successive positions `a` and `b`, the segment is axis-aligned when one of these conditions holds:

```text
b.Q - a.Q == 0
b.R - a.R == 0
(b.Q - a.Q) + (b.R - a.R) == 0
```

For example, a trace can move directly from `(0, 0)` to `(0, -2)` without listing `(0, -1)`. A trace that changes direction must include the location of the turn as a waypoint.

`Lines` controls how many parallel tracks make up the trace bundle. A value of `1` draws a single track. Larger values draw parallel tracks separated by `Options.TraceSpacing`, measured perpendicular to the route centerline. The renderer checks that the bundle and its offsets fit within the source and destination components.

The high-level layout API validates coordinate paths and bundle dimensions, but it does not find routes, enforce a circuit topology, or perform general collision avoidance. Those decisions belong to a generator or to the application constructing the layout.

## Terminal pads

By default, each track in a `GridTrace` ends in a hollow circular terminal pad. `Options.NodeRadius` controls the pad radius. The track is shortened so that it meets the edge of the pad rather than continuing underneath it.

A terminal can be placed at a grid position that does not contain a component. When the endpoint is a component position, the terminal pad is drawn inside that component.

Set `GridTrace.HideTerminal` to `true` when the trace should end at a component boundary instead. In this mode, the final `Path` position must contain a component, the terminal pad is omitted, and every track is clipped at the outer boundary of that destination component. This is useful when the trace should visually connect directly to the component outline.

At the lower-level scene API, terminals are represented explicitly by `hexgrid.Terminal` values in `hexgrid.Scene`.

## Rendering a layout

For a manually authored composition, use `GridScene` and `RenderGrid`:

1. Start with `DefaultOptions` and set the canvas, grid, component, and trace styling.
2. Add `GridComponent` values at the desired axial positions.
3. Add `GridTrace` values that connect those components or end at free terminal positions.
4. Pass the layout to `RenderGrid` to receive an SVG string.

`BuildScene` converts a `GridScene` into the renderer's lower-level `Scene` representation. That lower-level API uses explicit inch-based coordinates (`Point` values) instead of axial coordinates. `Render` serializes a `Scene` to SVG. This is useful when an application needs to construct geometry directly rather than using the grid-based layout API.

The renderer draws the supplied geometry; it does not rearrange objects or prevent overlaps. The normal layer order is background fill, background grid, traces, components, labels, and terminal pads. `Scene.Color` is the fallback color for each element. `Grid.Color`, `Component.Color`, `Trace.Color`, `Terminal.Color`, and `Label.Color` override it independently when set. In `GridScene`, `GridComponent.Color` and `GridTrace.Color` map to their corresponding scene elements; `GridTrace.TerminalColor` optionally overrides the generated terminal-pad color, which otherwise inherits the trace color.

## Use `hexgrid` directly for manually-authored artwork

The `hexgrid` package exposes a grid-based API for creating a fixed composition without using the randomized circuit generator. Components and trace waypoints use axial grid coordinates instead of inch-based `X`/`Y` values.

```go
package main

import (
    "log"
    "os"

    "github.com/sector-f/hexgrid"
)

func main() {
    options := hexgrid.DefaultOptions()
    options.Width = 8
    options.Height = 6
    options.Color = "#396cf8"
    layout := hexgrid.GridScene{
        Options: options,
        Components: []hexgrid.GridComponent{
            {Position: hexgrid.GridPosition{Q: -2, R: 0}, Style: hexgrid.ComponentStyleSingle},
            {Position: hexgrid.GridPosition{Q: 2, R: 0}, Style: hexgrid.ComponentStyleTriple},
            {Position: hexgrid.GridPosition{Q: -2, R: 3}, Style: hexgrid.ComponentStyleDouble},
        },
        Traces: []hexgrid.GridTrace{
            {
                Start:        hexgrid.GridPosition{Q: -2, R: 0},
                Path:         []hexgrid.GridPosition{{Q: 2, R: 0}},
                Lines:        2,
                HideTerminal: true,
            },
            {
                Start: hexgrid.GridPosition{Q: -2, R: 3},
                Path: []hexgrid.GridPosition{
                    {Q: 0, R: 3},
                    {Q: 0, R: 1},
                },
                Lines: 1,
            },
        },
    }

    svg, err := hexgrid.RenderGrid(layout)
    if err != nil {
        log.Fatal(err)
    }
    if err := os.WriteFile("manual-circuit.svg", []byte(svg), 0644); err != nil {
        log.Fatal(err)
    }
}
```
