# circuitboard

`circuitboard` is a hand-composed hexagonal PCB illustration generator. It shares
`hexgrid`'s geometry and SVG renderer with `circuit`, but it does not use the
circuit package's randomized component placement or general-purpose route search.
Its component clusters, buses, and highlighted power paths are specified as a
separate board layout.

```go
package main

import (
    "os"

    "github.com/sector-f/hexcircuit/circuitboard"
)

func main() {
    options := circuitboard.DefaultOptions()
    options.ShowBackgroundGrid = true // false by default

    svg, _, _, err := circuitboard.Generate(options)
    if err != nil {
        panic(err)
    }
    if err := os.WriteFile("circuit-board.svg", []byte(svg), 0644); err != nil {
        panic(err)
    }
}
```

The bottom connector bank is always generated. `ConnectorCount` controls its
number of contacts, and `ConnectorInset` controls how far their centers sit from
the bottom edge. The default 12-contact row matches the original design.

All dimensions and stroke widths are in inches. `TopWidth` and `BottomWidth`
define the tapered panel outline; generated content, including the optional
background hex field, is clipped to that outline.
