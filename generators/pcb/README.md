# `generators/pcb`

The `pcb` package generates PCB-inspired SVG artwork on the shared hexagonal grid.
It plans an explicit board topology before rendering: components are placed in compact
clusters, signal traces connect components within each cluster, and every cluster receives
one power feed from either the top or bottom edge of the board.

The package uses `github.com/sector-f/hexgrid` for grid geometry and SVG rendering. It does
not model real electrical components, pin assignments, or manufacturable PCB layouts; it
generates procedural artwork.

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

`Generate` returns the SVG document, the number of components (including every contact in
both power-source rows), the number of routed bundles (signal plus power), and an error.
It does not perform file I/O. Set `SeedSet` to true to make output reproducible; otherwise
each call uses a fresh seed.

## Generation model

1. **Power-source rows.** The generator creates a row of potential contacts at the top
   and another at the bottom. `TopPowerSourceCount` and `BottomPowerSourceCount` control
   the number of contacts in each row. Exactly one contact per cluster is selected as an
   active source; unused contacts remain in the base color.
2. **Source selection.** Active contacts are selected randomly from the combined top and
   bottom rows. With `PowerSourceSpacingSeparated`, at least one unused contact is left
   between active contacts within each row. `PowerSourceSpacingAdjacent` allows adjacent
   active contacts. After selection, contacts are matched to cluster hubs from left to
   right. This preserves a non-crossing topology while keeping the choice of top versus
   bottom source independent of the cluster's vertical position.
3. **Cluster placement.** The generator allocates the requested component count across
   clusters, respecting `MinComponentsPerCluster`. Cluster centers are distributed using
   farthest-point sampling. Additional components grow outward from each hub through
   adjacent lattice cells. Cluster footprints retain separation from neighboring clusters
   and reserved power routes.
4. **Signal topology.** The placement parent relationships form a spanning tree, so every
   component in a multi-component cluster is connected. `Connections` requests additional
   local signal connections beyond these required tree edges. Optional links never connect
   different clusters and avoid power-route cells.
5. **Power routing.** Each power feed starts at its selected top or bottom contact. A top
   feed initially travels downward; a bottom feed initially travels upward. The A* router
   uses the axial hex lattice and reserves every cell used by a completed power route.
   Because grid-axis segments on this lattice can intersect only at lattice vertices,
   reserving all visited vertices prevents power-feed crossings. The router never falls back
   to a crossing path: if it cannot find a set of disjoint routes for a placement, it retries
   with another placement and eventually returns an error rather than rendering crossing
   power feeds. The router may send a top contact to a cluster near the bottom, or a bottom
   contact to a cluster near the top.
6. **Rendering and color semantics.** Components and signal traces use `BaseColor`.
   `AccentColor` is used only for power traces and the small power-entry cores at active
   source contacts and cluster hubs. Every accent-colored element belongs to a power feed
   rooted at one of the two edge rows. The base-layer style of all source contacts is
   controlled independently by `PowerSourceStyle`.

## Options

All physical dimensions are in inches.

| Option | Meaning |
|---|---|
| `Width`, `Height` | SVG canvas dimensions |
| `ComponentCount` | Number of clustered components, excluding both power-source rows |
| `ClusterCount` | Number of component clusters and active power feeds |
| `MinComponentsPerCluster` | Minimum cluster size; defaults to `1`, allowing singleton clusters |
| `Connections` | Target number of additional intra-cluster signal connections, beyond required spanning trees |
| `TopPowerSourceCount` | Number of potential contacts in the top row |
| `BottomPowerSourceCount` | Number of potential contacts in the bottom row |
| `PowerSourceSpacing` | `"separated"` leaves an unused contact between active sources within each row; `"adjacent"` permits neighboring active sources |
| `PowerSourceStyle` | `"single"` or `"double"` base-layer hexagons for all contacts in both rows |
| `MaxParallel` | Maximum number of parallel tracks per bundle (`1`–`3`) |
| `BaseColor` | Components, signal traces, unused source contacts, and optional background grid |
| `AccentColor` | Power feeds and their connected power-entry cores only |
| `ShowBackgroundGrid` | Enables the optional low-contrast hexagonal background grid |
| `Seed`, `SeedSet` | Reproducible generation settings |

The remaining options control grid pitch, component geometry, trace geometry, source-row
inset, canvas background, and partial background-grid clipping. See `Options` for their
full documentation.

`TopPowerSourceCount` and `BottomPowerSourceCount` may be zero individually, but their
sum must be at least `ClusterCount`. Each enabled row must fit on its corresponding canvas
edge. In separated mode, the combined rows must also have enough positions to leave an
unused contact between every pair of active sources on the same row; use adjacent mode
when a denser source arrangement is required.

`MinComponentsPerCluster` must be at least one, and the requested component count must be
sufficient to satisfy that minimum across all clusters.

`ConnectorCount` is retained as a deprecated compatibility alias for
`BottomPowerSourceCount`. New code should use the explicit top and bottom count fields.
