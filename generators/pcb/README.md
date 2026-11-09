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
	options.TopPowerSourceCount = 11
	options.BottomPowerSourceCount = 13
	options.PowerSourceSpacing = pcb.PowerSourceSpacingSeparated
	options.PowerSourceStyle = pcb.PowerSourceStyleSingle
	options.ConstrainComponentsToPowerSourceSpan = true
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

`Generate` returns the SVG document, the number of rendered base-layer components (ordinary cluster components plus every configured top- and bottom-row power-source contact), the number of routed bundles (signal plus power), and an error. Every configured source contact is rendered in the base color whether or not it is selected to power a cluster. Selected contacts additionally receive an accent-colored power core and a power route; unselected contacts remain visible as unpowered contacts. The function does not perform file I/O. Set `SeedSet` to true to make output reproducible; otherwise each call uses a fresh seed.

## Generation model

1. **Power-source rows.** The generator creates contact components along the top and/or bottom edge. `TopPowerSourceCount` and `BottomPowerSourceCount` control how many contacts are rendered on each row. Every configured contact is rendered in the base color, whether or not it is selected to power a cluster. Exactly one contact per cluster is selected as an active source; active contacts additionally receive an accent-colored power core and power route.
2. **Source selection.** Active contacts are selected randomly from the combined top and bottom rows. With `PowerSourceSpacingSeparated`, at least one unused position is left between active contacts within each row. `PowerSourceSpacingAdjacent` allows neighboring active contacts. After selection, contacts are matched to cluster hubs from left to right. This keeps the source-to-cluster pairing monotonic while leaving the choice of top versus bottom source independent of the cluster's vertical position.
3. **Cluster placement.** The generator allocates the requested ordinary component count across clusters, respecting `MinComponentsPerCluster`. Cluster centers are distributed using farthest-point sampling. Additional components grow outward from each hub through adjacent lattice cells. Cluster footprints retain separation from neighboring clusters and reserved power routes. When `ConstrainComponentsToPowerSourceSpan` is enabled, candidate positions are also restricted to the horizontal interval between the leftmost and rightmost active source contacts.
4. **Signal topology.** The placement parent relationships form a spanning tree, so every component in a multi-component cluster is connected. `Connections` requests additional local signal connections beyond these required tree edges. Optional links never connect different clusters and avoid power-route cells.
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
| `TopPowerSourceCount` | Number of physical power-source contact components rendered in the top row, including unused contacts |
| `BottomPowerSourceCount` | Number of physical power-source contact components rendered in the bottom row, including unused contacts |
| `PowerSourceSpacing` | `PowerSourceSpacingSeparated` requires at least one unselected contact between active sources within a row; `PowerSourceSpacingAdjacent` permits neighboring active sources |
| `PowerSourceStyle` | `PowerSourceStyleSingle` or `PowerSourceStyleDouble` base-layer hexagons for all source contacts |
| `ConstrainComponentsToPowerSourceSpan` | Restricts all ordinary components to the horizontal span between the leftmost and rightmost active power-source contacts; disabled by default |
| `MaxParallel` | Maximum number of parallel tracks per bundle (`1`–`3`) |
| `BaseColor` | Ordinary components, all source contact outlines (including unused contacts), signal traces, and optional background grid |
| `AccentColor` | Power feeds and their connected power-entry cores only |
| `ShowBackgroundGrid` | Enables the optional low-contrast hexagonal background grid |
| `Seed`, `SeedSet` | Reproducible generation settings |

The remaining options control grid pitch, component geometry, trace geometry, source-row inset, canvas background, and partial background-grid clipping. See `Options` for their full documentation.

All configured source contacts are visible base-layer components. The spacing option affects which contacts are selected for active feeds; it does not hide the unselected contacts. In separated mode, unselected contacts between active feeds remain drawn in the base color, without an accent core or power trace.

`TopPowerSourceCount` and `BottomPowerSourceCount` may be zero individually, but their sum must be at least `ClusterCount`. Each enabled row must fit on its corresponding canvas edge. In separated mode, the combined rows must also have enough positions to leave an unused position between every pair of active sources on the same row; use adjacent mode when a denser source arrangement is required.

`ConstrainComponentsToPowerSourceSpan` uses the leftmost and rightmost **active** sources, not unselected contacts. If the selected source span is too narrow to fit the requested connected clusters, generation returns an error. Disabling the option allows the regular full-width component placement region.

`MinComponentsPerCluster` must be at least one, and the requested ordinary component count must be sufficient to satisfy that minimum across all clusters.

`ConnectorCount` is retained as a deprecated compatibility alias for `BottomPowerSourceCount`. New code should use the explicit top and bottom count fields.
