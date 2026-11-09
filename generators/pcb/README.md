# `generators/pcb`

The `pcb` package generates PCB-inspired SVG artwork on the shared hexagonal grid.
The generator plans an explicit board topology before rendering it: components are
placed in compact local clusters, signal traces connect the components within each
cluster, and every cluster receives a separate power feed from a selected contact in
the bottom connector row.

The package uses `github.com/sector-f/hexgrid` for grid geometry and SVG rendering.
It does not model real electrical components, pin assignments, or manufacturable PCB
layouts; it generates procedural artwork.

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
	options.MinComponentsPerCluster = 2
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

`Generate` returns the SVG document, the number of components including the mandatory
bottom connector row, the number of routed bundles (signal plus power), and an error.
It does not perform file I/O. Set `SeedSet` to true to make output reproducible;
otherwise each call uses a fresh seed.

## Generation model

1. **Connector row.** The generator creates the bottom row first. It selects one
   distinct power contact per cluster. When the connector count permits it, selected
   contacts are separated by at least one unused contact. The selected positions are
   randomized for each seed, so used contacts are interspersed with unused contacts
   instead of forming a contiguous block. Selected contacts are sorted left-to-right
   only for assignment to similarly ordered cluster hubs; this keeps the feeds from
   crossing without making the selected contacts themselves contiguous.
2. **Cluster placement.** The generator allocates the requested component count across
   clusters, respecting `MinComponentsPerCluster`. Cluster centers are distributed
   using farthest-point sampling. Additional components grow outward from each hub
   through adjacent lattice cells. Cluster footprints use a compact, size-scaled
   radius and retain a one-cell buffer between neighboring clusters.
3. **Signal topology.** The placement parent relationships form a spanning tree, so
   every component in a multi-component cluster is connected. `Connections` requests
   additional local signal connections beyond these required tree edges. Optional
   links never connect different clusters.
4. **Power topology.** Every cluster hub receives one route from its assigned bottom
   connector. A* routing follows legal hex-grid neighbors, blocks occupied components,
   avoids previously routed paths, and requires a power feed's first move to head
   northward from the connector row.
5. **Rendering.** Components and signal traces use `BaseColor`. `AccentColor` is used
   only for power traces and the small power-entry cores at used connector contacts and
   cluster hubs. Every accent element therefore belongs to a visible power path rooted
   at the bottom connector row. Power-core sizing is included in bundle validation so
   generated parallel tracks fit inside their smaller endpoint cores.

## Options

All physical dimensions are in inches.

| Option | Meaning |
|---|---|
| `Width`, `Height` | SVG canvas dimensions |
| `ComponentCount` | Number of clustered components, excluding the connector row |
| `ClusterCount` | Number of component clusters |
| `MinComponentsPerCluster` | Minimum cluster size; defaults to `1`, allowing singleton clusters |
| `Connections` | Target number of additional intra-cluster signal connections, beyond required spanning trees |
| `ConnectorCount` | Number of contacts in the mandatory bottom row; must be at least the cluster count |
| `MaxParallel` | Maximum number of parallel tracks per bundle (`1`–`3`) |
| `BaseColor` | Components, signal traces, and optional background grid |
| `AccentColor` | Power feeds and their connected power-entry cores only |
| `ShowBackgroundGrid` | Enables the optional low-contrast hexagonal background grid |
| `Seed`, `SeedSet` | Reproducible generation settings |

The remaining options control grid pitch, component geometry, trace geometry, connector
inset, canvas background, and partial background-grid clipping. See `Options` for their
full documentation.

`MinComponentsPerCluster` must be at least one, and the requested component count must
be sufficient to satisfy that minimum across all clusters. `ConnectorCount` must be at
least `ClusterCount`, because each cluster receives its own power source. When there are
too few contacts to leave an unused contact between every selected source, the source
selection relaxes that spacing constraint while still choosing distinct contacts.
