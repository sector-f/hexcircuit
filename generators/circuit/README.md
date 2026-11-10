# `generators/circuit`

The `circuit` package generates randomized SVG artwork built from a hexagonal lattice, larger component hexagons, and routed bundles of parallel traces. It is a procedural composition generator: it decides where components go, which components should connect, and how to route those connections while respecting the available geometry.

The package uses [`github.com/sector-f/hexgrid`](https://github.com/sector-f/hexgrid) for grid-layout conversion and SVG rendering. `hexgrid` supplies the reusable geometry and renderer; this package supplies the circuit-specific placement, network construction, routing, and visual variation.

## Usage

`Generate` returns the SVG document as a string and does not perform file I/O:

```go
package main

import (
    "log"
    "os"

    "github.com/sector-f/hexgrid/generators/circuit"
)

func main() {
    options := circuit.DefaultOptions()
    options.Width = 12
    options.Height = 9
    options.Color = "#396cf8"
    options.Background = "#141314"
    options.Seed = 42
    options.SeedSet = true

    svg, accepted, attempts, err := circuit.Generate(options)
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("accepted %d trace bundles from %d candidate pairs", accepted, attempts)
    if err := os.WriteFile("circuit.svg", []byte(svg), 0644); err != nil {
        log.Fatal(err)
    }
}
```

The return values are:

```go
svg, acceptedBundles, candidatePairAttempts, err := circuit.Generate(options)
```

The second value is the number of successfully routed trace bundles. The third is the number of candidate component pairs attempted. The requested connection count is a target; it is not guaranteed that every candidate can be routed successfully.

## Generation approach

The generator works in the hexagonal lattice first and converts the accepted layout into drawable geometry only after placement and routing are complete.

### 1. Normalize the options and initialize randomness

`DefaultOptions` provides a complete starting configuration. `Generate` validates the supplied options and adjusts the routing pitch when automatic pitch adjustment is enabled. If `SeedSet` is true, the supplied `Seed` controls random placement, route choices, component details, and labels. Otherwise, the generator selects a fresh seed for the run.

All dimensional options are expressed in inches. The background grid's radius, gap, and outline width are separate from the routing pitch, which is the center-to-center distance between lattice positions.

### 2. Place components in loose clusters

The generator selects lattice cells for the requested number of components. Rather than filling the grid uniformly, it creates a small number of loose clusters. Components added near an existing component retain a preferred parent relationship; those relationships provide the first candidate connections for the network.

Component sizes vary. Some components are relatively small, while others are larger hubs. Component positions and sizes are chosen with the canvas boundaries and available routing space in mind. Some sufficiently large components later receive nested hexagonal details.

### 3. Build candidate connections

The generator first considers the relationships created during component placement. It then considers fallback component pairs to fill the requested connection count when preferred connections cannot be routed.

`FallbackEdgeOrder` controls how fallback pairs are considered:

- `FallbackEdgeOrderRandomized` randomizes the fallback order.
- `FallbackEdgeOrderRanked` prioritizes same-cluster pairs and then shorter axial distances.

The command-line program selects ranked fallback ordering. Direct users of the package get the default randomized ordering unless they override it.

### 4. Search for routes on the lattice

Each candidate connection is routed through the six directions of the axial hexagonal lattice. A route leaves the source component through a selected side and reaches the destination component. A valid route must contain at least three lattice moves and at least one bend; straight, direct connections are deliberately not used.

The route search uses a cost and distance heuristic to explore promising paths. The configured route-cost mode controls whether route cost is based on move count and turns alone or includes random jitter. Search is bounded by `MaxRouteSteps` and makes alternative attempts according to `RouteAttempts`.

Other components, cells reserved by earlier routes, and positions too close to the canvas boundary are excluded from candidate paths. This keeps routes from passing through unrelated components or running too close to the edge.

### 5. Turn each route into a trace bundle

A successful lattice route becomes a bundle of parallel tracks. The number of tracks is selected from 1 through `MaxParallel`, using `TrackCountWeights` as percentage weights. The generator only considers bundle sizes that fit the available source and destination component sides.

Tracks are offset perpendicular to the route centerline by `ParallelSpacing`. The resulting geometry is checked against other components, terminal pads, and previously accepted tracks. Component sides are reserved when used, preventing later connections from reusing the same connection face. A candidate is rejected when it violates the configured clearance or collision constraints.

Each track normally ends at a hollow terminal pad inside its destination component. Track width, terminal radius, and bundle spacing determine whether the bundle fits.

### 6. Render the accepted composition

Once placement and routing finish, the package converts the components and accepted connections into a `hexgrid.GridScene`, builds a `hexgrid.Scene`, and passes that scene to `hexgrid.Render`.

The shared renderer draws the optional background fill and grid, then traces, components and their details, labels, and terminal pads. Circuit-specific decisions remain in this package; the root `hexgrid` package does not know how many components to place or which paths form a useful circuit composition.

## Important behavior

### A target, not a fixed result

`Connections` is the desired number of accepted trace bundles. It is not a promise that the result will contain exactly that many. Some candidate pairs may have no route that satisfies all of the constraints. The returned `accepted` count reports how many bundles were actually produced.

When the result is too sparse, try increasing the canvas size, increasing the routing pitch, reducing the connection target, increasing route attempts or maximum route steps, or reducing the amount of space consumed by components and bundles.

### Reproducibility

Set both `Seed` and `SeedSet` to reproduce a composition:

```go
options.Seed = 42
options.SeedSet = true
```

The same seed, options, and program version produce the same generated composition. Leaving `SeedSet` false asks the generator to choose a new seed for each run. Set `SeedSet` to true even when the intended seed is zero.

### Automatic routing pitch

`AutoRoutingPitch` is enabled by default. When the requested pitch is too small to accommodate the background hexagon radius, gap, and stroke width, the generator raises it to the minimum valid value. Disable automatic adjustment when a fixed pitch is required; an invalid fixed pitch then returns an error.

The component radius must also leave enough room for terminal pads and trace width, and the routing pitch must leave room between neighboring components for routed traces. These geometric constraints are validated before generation.

### Labels

Labels are optional. Set `ShowLabels` to enable them. The generator chooses labels probabilistically according to `LabelProbability`, and attempts to place them beside components without colliding with the artwork. `LabelFont` selects the font family; label font sizes are measured during generation, while the SVG refers to the font family rather than embedding a font file. The package also supports custom label word lists, label color, label distance, and allowing labels inside components.

## Options

Use `DefaultOptions` and modify only the values that need to change. The primary option groups are:

| Group | Options | Purpose |
| --- | --- | --- |
| Canvas and styling | `Width`, `Height`, `Color`, `GridColor`, `Background`, `Orientation` | SVG dimensions, component/trace color, independent background-grid color, and pointy/flat orientation. An empty `GridColor` falls back to `Color`. |
| Grid geometry | `GridRadius`, `GridGap`, `GridWidth`, `RoutingPitch`, `AutoRoutingPitch`, `OmitPartialGridHexagons` | Background hexagon geometry and lattice spacing. |
| Components | `ComponentCount`, `ComponentRadius`, `StrokeWidth`, `DetailProb` | Component population, size, outline, and internal details. |
| Connections | `Connections`, `MaxParallel`, `TrackCountWeights`, `ParallelSpacing` | Target bundle count and distribution of bundle sizes. |
| Trace geometry | `TraceWidth`, `NodeRadius`, `TraceClearance` | Track thickness, terminal pads, and route separation. |
| Search | `MaxRouteSteps`, `RouteAttempts`, `UseDeterministicRouteCost`, `FallbackEdgeOrder` | Route search bounds and candidate ordering/cost behavior. |
| Reproducibility | `Seed`, `SeedSet` | Control the random sequence. |
| Labels | `ShowLabels`, `LabelWordList`, `LabelWordListSet`, `LabelFont`, `LabelFontSize`, `LabelProbability`, `FontColor`, `FontColorSpecified`, `LabelDistance`, `LabelInsideComponents` | Enable and customize component labels. |

See [`options.go`](options.go) for field-level documentation and defaults.

## Related documentation

- [Repository and `hexgrid` library](../../README.md)
- [`circuit-cli` command](../../cmd/circuit-cli/README.md)
