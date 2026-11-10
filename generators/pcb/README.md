# `generators/pcb`

The `pcb` package generates PCB-inspired SVG artwork on the shared hexagonal grid. It plans an explicit board topology before rendering: ordinary components are placed in compact clusters, signal traces connect components within each cluster, and every cluster receives one power feed from an active contact on the top or bottom edge.

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

`Generate` returns the SVG document, the number of rendered base-layer components (ordinary cluster components plus all configured top- and bottom-row power-source contacts), the number of routed bundles (signal plus power), and an error. Every configured physical contact is rendered. Selected contacts receive an accent-colored power core and a power route; unselected contacts remain base-color-only. In separated mode, neighboring physical contacts are placed two lattice cells apart, leaving one empty lattice cell between them. The function does not perform file I/O. Set `SeedSet` to true to make output reproducible; otherwise each call uses a fresh seed.

## Generation model

1. **Power-source rows.** The generator defines potential contact positions along the top and/or bottom edge. `TopPowerSourceCount` and `BottomPowerSourceCount` control how many positions are available on each row. Exactly one contact per cluster is selected as an active source; active contacts receive an accent-colored power core and power route. Unselected contacts remain visible in the base color.
2. **Source selection.** Active contacts are selected randomly from the combined top and bottom rows. With `PowerSourceSpacingSeparated`, neighboring physical contacts are placed two lattice cells apart; the intervening lattice cells are empty and are not contacts. `PowerSourceSpacingAdjacent` allows neighboring active contacts and renders all configured positions. After selection, contacts are matched to cluster hubs from left to right. This keeps the source-to-cluster pairing monotonic while leaving the choice of top versus bottom source independent of the cluster's vertical position.
3. **Cluster placement.** The generator allocates the requested ordinary component count across clusters, respecting `MinComponentsPerCluster`. Cluster centers are distributed using farthest-point sampling. Additional components grow outward from each hub through adjacent lattice cells. Cluster footprints retain separation from neighboring clusters and reserved power routes. Ordinary components are excluded from the physical top/bottom source rows. When `ConstrainComponentsToPowerSourceSpan` is enabled, candidate positions and all signal/power routes are restricted to the horizontal interval between the leftmost and rightmost active source contacts.
4. **Signal topology.** The placement parent relationships form a spanning tree, so every component in a multi-component cluster is connected. `Connections` requests additional local signal connections beyond these required tree edges. Optional links never connect different clusters, avoid power-route cells, and stay inside the same source-bounded routing region as the components.
5. **Power routing.** Each power feed starts at its selected top or bottom contact. A top feed initially travels downward; a bottom feed initially travels upward. The A* router uses the axial hex lattice and reserves every cell used by a completed power route. Because grid-axis segments on this lattice can intersect only at lattice vertices, reserving all visited vertices prevents power-feed crossings. The router never falls back to a crossing path: if it cannot find a set of disjoint routes for a placement, it retries with another placement and eventually returns an error rather than rendering crossing power feeds. A top contact may power a cluster near the bottom, or a bottom contact may power a cluster near the top.
6. **Rendering and color semantics.** Ordinary components and signal traces use `BaseColor`. `AccentColor` is used only for power traces and the small power-entry cores at active source contacts and cluster hubs. Every accent-colored element belongs to a power feed rooted at one of the two edge rows. The base-layer style of active source contacts is controlled by `PowerSourceStyle`.

## Options

All physical dimensions are in inches.

| Option | Meaning |
|---|---|
| `Width`, `Height` | SVG canvas dimensions |
| `ComponentCount` | Number of ordinary clustered components, excluding all power-source row contacts |
| `ClusterCount` | Number of component clusters and active power feeds |
| `MinComponentsPerCluster` | Minimum cluster size; defaults to `1`, allowing singleton clusters |
| `Connections` | Target number of additional intra-cluster signal connections beyond required spanning trees |
| `TopPowerSourceCount` | Number of physical power-source contacts along the top row |
| `BottomPowerSourceCount` | Number of physical power-source contacts along the bottom row |
| `PowerSourceSpacing` | `PowerSourceSpacingSeparated` places physical contacts two lattice cells apart; `PowerSourceSpacingAdjacent` places them in neighboring cells |
| `PowerSourceStyle` | `PowerSourceStyleSingle` or `PowerSourceStyleDouble` base-layer hexagons for rendered source contacts |
| `ConstrainComponentsToPowerSourceSpan` | Restricts ordinary components and every route to the horizontal span between the leftmost and rightmost active power-source contacts; disabled by default |
| `ConstrainLayoutToPowerSourceTrapezoid` | Restricts ordinary components and every route to the trapezoid connecting the left/right extents of the top and bottom power-source rows; disabled by default and requires both rows |
| `MaxParallel` | Maximum number of parallel tracks per bundle (`1`–`3`) |
| `BaseColor` | Ordinary components, rendered source contact outlines (including unused contacts outside separated gaps), signal traces, and optional background grid |
| `AccentColor` | Power feeds and their connected power-entry cores only |
| `ShowBackgroundGrid` | Enables the optional low-contrast hexagonal background grid |
| `Seed`, `SeedSet` | Reproducible generation settings |

The remaining options control grid pitch, component geometry, trace geometry, source-row inset, canvas background, and partial background-grid clipping. See `Options` for their full documentation.

Every configured physical contact is rendered in both modes. In separated mode, one empty lattice cell is left between neighboring contacts; in adjacent mode, contacts occupy neighboring lattice cells.

`TopPowerSourceCount` and `BottomPowerSourceCount` may be zero individually, but their sum must be at least `ClusterCount`. Each enabled row must fit on its corresponding canvas edge. In separated mode, the combined rows must also have enough positions to leave an unused position between every pair of active sources on the same row; use adjacent mode when a denser source arrangement is required.

`ConstrainComponentsToPowerSourceSpan` uses the leftmost and rightmost **active** sources, not unselected contacts. When enabled, components and their signal/power routes stay within that horizontal span. If the selected source span is too narrow to fit the requested connected clusters, generation returns an error. Disabling the option allows the regular full-width component placement region.

`ConstrainLayoutToPowerSourceTrapezoid` is an independent, opt-in constraint. It uses the leftmost and rightmost configured physical contacts on each source row, then linearly interpolates the left and right boundaries between those rows. It requires both the top and bottom source rows. Components, cluster growth, signal traces, and power feeds are restricted to the resulting trapezoid. When both layout constraints are enabled, their allowed regions are intersected.

`MinComponentsPerCluster` must be at least one, and the requested ordinary component count must be sufficient to satisfy that minimum across all clusters.

`ConnectorCount` is retained as a deprecated compatibility alias for `BottomPowerSourceCount`. New code should use the explicit top and bottom count fields.
