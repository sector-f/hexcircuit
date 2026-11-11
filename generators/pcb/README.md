# `generators/pcb`

The `pcb` package generates PCB-inspired SVG artwork on the shared hexagonal grid. It plans an explicit board topology before rendering: ordinary components are placed in compact clusters, signal traces connect components within each cluster, and each cluster receives a randomized number of power feeds from active contacts on the top or bottom edge. Each feed independently selects a destination component within its assigned cluster. A cluster may receive no power feeds. An optional inter-cluster trace layer can add occasional accent-colored connections between separate clusters.

The package uses `github.com/sector-f/hexgrid` for grid geometry and SVG rendering. It does not model real electrical components, pin assignments, or manufacturable PCB layouts; it generates procedural artwork.

## Usage

```go
package main

import (
	"log"
	"os"

	"github.com/sector-f/hexgrid/generators/pcb"
)

func main() {
	options := pcb.DefaultOptions()
	options.ShowBackgroundGrid = true
	options.MinComponentsPerCluster = 1
	options.MinPowerSourcesPerCluster = 0
	options.MaxPowerSourcesPerCluster = 2
	options.TopPowerSourceCount = 9
	options.BottomPowerSourceCount = 10
	options.PowerSourceSpacing = pcb.PowerSourceSpacingSeparated
	options.PowerSourceStyle = pcb.PowerSourceStyleSingle
	options.ConstrainComponentsToPowerSourceSpan = true
	options.ConstrainLayoutToPowerSourceTrapezoid = true
	options.Seed = 1234
	options.SeedSet = true

	svg, componentCount, traceCount, err := pcb.Generate(options)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("rendered %d components and %d trace bundles", componentCount, traceCount)
	if err := os.WriteFile("board.svg", []byte(svg), 0644); err != nil {
		log.Fatal(err)
	}
}
```

`Generate` returns the SVG document, the number of rendered base-layer components (ordinary cluster components plus all configured top- and bottom-row power-source contacts), the number of routed bundles (signal, power, plus optional inter-cluster traces), and an error. Every configured physical contact is rendered. The number of active contacts assigned to each cluster is selected between `MinPowerSourcesPerCluster` and `MaxPowerSourcesPerCluster`, inclusive. Defaults of `1..1` preserve the original one-feed-per-cluster behavior; a minimum of zero permits unpowered clusters. Each feed terminates at a randomly selected component in its cluster rather than always terminating at the cluster hub. The total active contact count never exceeds the configured physical contacts, and validation rejects a minimum that cannot be satisfied. Selected contacts receive an accent-colored power core and a power route; unselected contacts remain base-color-only. In separated mode, neighboring physical contacts are placed two lattice cells apart, leaving one empty lattice cell between them. The function does not perform file I/O. Set `SeedSet` to true to make output reproducible; otherwise each call uses a fresh seed. If randomized placement and mandatory power routing exhaust the attempts for one seed, `GenerationAttempts` retries complete board construction with fallback seeds derived deterministically from the original seed. The default is four total seed attempts, including the original. Validation and other non-random errors are returned without retrying.

## Generation model

1. **Power-source rows.** The generator defines potential contact positions along the top and/or bottom edge. `TopPowerSourceCount` and `BottomPowerSourceCount` control how many positions are available on each row. Active contacts are selected within each cluster's configured minimum/maximum range. Rows are horizontally centered; if the outermost row with enough room cannot produce a symmetric placement at the configured spacing, the contacts move one lattice row inward. Unselected contacts remain visible in the base color; when both limits are zero, no physical source rows are required.
2. **Source selection.** The generator randomly chooses an active feed count for each cluster, then selects that many unique contacts from the combined top and bottom rows. When the random total exceeds the available contacts, random extra feeds are removed until the selection fits; every cluster's minimum is preserved. With `PowerSourceSpacingSeparated`, neighboring physical contacts are placed two lattice cells apart; the intervening lattice cells are empty and are not contacts. `PowerSourceSpacingAdjacent` allows neighboring contacts. After selection, contacts are matched to cluster hubs from left to right and grouped by cluster. This keeps source-to-cluster pairing monotonic while leaving the choice of top versus bottom source independent of the cluster's vertical position.
3. **Cluster placement.** The generator allocates the requested ordinary component count across clusters, respecting `MinComponentsPerCluster`. Cluster centers are distributed using farthest-point sampling. Additional components grow outward from each hub through adjacent lattice cells. Cluster footprints retain separation from neighboring clusters. Power feeds are routed after all clusters have been placed, around component footprints. Ordinary components are excluded from the physical top/bottom source rows. When `ConstrainComponentsToPowerSourceSpan` is enabled, candidate positions and all signal/power routes are restricted to the horizontal interval between the leftmost and rightmost active source contacts. If no feeds are active, the configured physical contacts define the span.
4. **Signal topology.** The placement parent relationships form a spanning tree, so every component in a multi-component cluster is connected. `Connections` requests additional local signal connections beyond these required tree edges. Optional links never connect different clusters, avoid power-route cells, and stay inside the same source-bounded routing region as the components.
5. **Power routing.** Each feed starts at its selected top or bottom contact and routes to a randomly selected component in its assigned cluster. The destination is selected from the cluster's complete component set, with distinct destinations preferred until every component has been used once. A top feed initially travels downward; a bottom feed initially travels upward. Routes cannot pass through other component footprints, and separate feeds do not share route vertices except when the number of feeds exceeds the number of components and a destination must be reused. The A* router uses the axial hex lattice and reserves every cell used by a completed power route. Because grid-axis segments on this lattice can intersect only at lattice vertices, reserving all visited vertices prevents power-feed crossings. For each placement, the generator makes up to `RouteAttempts` randomized passes over the complete feed set. A different path for an earlier feed may leave room for a later feed. If no pass routes every feed without crossings, the placement is discarded and another placement is tried. If all placement attempts for that seed fail, complete board generation may be retried with a derived seed, up to `GenerationAttempts` total attempts. The generator returns an error rather than rendering crossing power feeds. The power layer draws an accent-colored core at each selected source and each unique destination component. Clusters with no assigned feeds have no power routes or accent-colored destination core.
6. **Inter-cluster routing.** When `InterClusterTraces` is enabled, the generator randomly chooses zero or more distinct cluster pairs, up to one fewer than the number of clusters. For each selected pair it shuffles component endpoints and tries to route one trace between the clusters. Pairs with no valid route are skipped; no inter-cluster trace is guaranteed. Each accepted route adds small accent-colored endpoint components to the separate inter-cluster overlay. Routes reuse the same collision and layout constraints as the other route types, and reserve their interior cells so they do not cross other routed traces.
7. **Rendering and color semantics.** Ordinary components, source-contact outlines, and intra-cluster signal traces use `BaseColor`. The optional background hexagon grid uses `GridColor`, falling back to `BaseColor` when left empty. `AccentColor` is used for power feeds, power-entry cores, and optional inter-cluster traces. Power feeds and their entry cores originate at the two edge rows; optional inter-cluster traces are separate accent-colored links between component clusters. The base-layer style of active source contacts is controlled by `PowerSourceStyle`.

## Options

All physical dimensions are in inches.

| Option | Meaning |
|---|---|
| `Width`, `Height` | SVG canvas dimensions |
| `ComponentCount` | Number of ordinary clustered components, excluding all power-source row contacts |
| `ClusterCount` | Number of component clusters |
| `MinComponentsPerCluster` | Minimum cluster size; defaults to `1`, allowing singleton clusters |
| `MinPowerSourcesPerCluster` | Minimum active feeds assigned to each cluster; defaults to `1`, and `0` allows unpowered clusters |
| `MaxPowerSourcesPerCluster` | Maximum active feeds assigned to each cluster; defaults to `1`, with a supported limit of six |
| `Connections` | Target number of additional intra-cluster signal connections beyond required spanning trees |
| `InterClusterTraces` | Enables opportunistic accent-colored routes between components in different clusters; disabled by default |
| `TopPowerSourceCount` | Number of physical power-source contacts along the top row |
| `BottomPowerSourceCount` | Number of physical power-source contacts along the bottom row |
| `PowerSourceSpacing` | `PowerSourceSpacingSeparated` places physical contacts two lattice cells apart; `PowerSourceSpacingAdjacent` places them in neighboring cells |
| `PowerSourceStyle` | `PowerSourceStyleSingle` or `PowerSourceStyleDouble` base-layer hexagons for rendered source contacts |
| `ConstrainComponentsToPowerSourceSpan` | Restricts ordinary components and every route to the horizontal span between the leftmost and rightmost active power-source contacts; disabled by default |
| `ConstrainLayoutToPowerSourceTrapezoid` | Restricts ordinary components and every route to the trapezoid connecting the left/right extents of the top and bottom power-source rows; disabled by default and requires both rows |
| `MaxParallel` | Maximum number of parallel tracks per bundle (`1`–`3`) |
| `RouteAttempts` | Randomized complete power-feed routing passes per component placement; defaults to `12` |
| `GenerationAttempts` | Complete board-generation attempts with deterministic fallback seeds; defaults to `4` total attempts including the original seed |
| `BaseColor` | Ordinary components, rendered source contact outlines (including unused contacts outside separated gaps), and signal traces |
| `GridColor` | Optional background hexagon grid; an empty value falls back to `BaseColor` |
| `AccentColor` | Power feeds, their connected power-entry cores, and optional inter-cluster traces |
| `ShowBackgroundGrid` | Enables the optional low-contrast hexagonal background grid |
| `Seed`, `SeedSet` | Reproducible generation settings; fallback seeds are derived from the first seed if generation retries |

The remaining options control grid pitch, component geometry, trace geometry, source-row inset, canvas background, and partial background-grid clipping. See `Options` for their full documentation.

Every configured physical contact is rendered in both modes. In separated mode, one empty lattice cell is left between neighboring contacts; in adjacent mode, contacts occupy neighboring lattice cells.

`TopPowerSourceCount` and `BottomPowerSourceCount` may be zero individually, but their sum must be at least `ClusterCount × MinPowerSourcesPerCluster`. If the minimum is zero, both physical rows may be empty even when the maximum is nonzero; with no configured contacts, no feeds can be assigned. Each enabled row must fit on its corresponding canvas edge. In separated mode, physical contacts are spaced two lattice cells apart; in adjacent mode, they occupy neighboring cells.

`ConstrainComponentsToPowerSourceSpan` uses the leftmost and rightmost **active** sources, not unselected contacts. When enabled, components and their signal/power routes stay within that horizontal span. If the selected source span is too narrow to fit the requested connected clusters, generation returns an error. Disabling the option allows the regular full-width component placement region.

`ConstrainLayoutToPowerSourceTrapezoid` is an independent, opt-in constraint. It uses the leftmost and rightmost configured physical contacts on each source row, then linearly interpolates the left and right boundaries between those rows. It requires both the top and bottom source rows. Components, cluster growth, signal traces, and power feeds are restricted to the resulting trapezoid. When both layout constraints are enabled, their allowed regions are intersected.

`MinComponentsPerCluster` must be at least one, and the requested ordinary component count must be sufficient to satisfy that minimum across all clusters. Power-source limits must satisfy `0 <= MinPowerSourcesPerCluster <= MaxPowerSourcesPerCluster <= 6`. The current option limit is six feeds per cluster. Component density and route clearance may make lower practical maxima necessary.

`ConnectorCount` is retained as a deprecated compatibility alias for `BottomPowerSourceCount`. New code should use the explicit top and bottom count fields.
