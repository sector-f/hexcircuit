# pcb-web

pcb-web is a browser-based interface for the PCB generator in ../../generators/pcb. It serves a single-page HTML interface, generates PCB-inspired SVG artwork on the server, previews the result in the browser, and downloads the currently displayed SVG.

## Run

From the repository root:

```sh
go run ./cmd/pcb-web
```

Open http://127.0.0.1:8080 in a browser.

Build a standalone executable:

```sh
go build -o pcb-web ./cmd/pcb-web
./pcb-web
```

Use -addr to listen on a different address:

```sh
./pcb-web -addr 0.0.0.0:8080
```

## Interface

The controls cover the PCB generator options: canvas dimensions, base/accent colors, an independent background-grid color (defaulting to the base color when unspecified), independent base-color and accent-color neon glow controls (enablement, blur radius, and opacity for each), component and cluster counts, component geometry (including independent minimum/maximum cluster-component radii and a fixed power-source radius), minimum and maximum active power feeds per cluster (including zero), optional randomized inter-cluster traces, bent decorative traces (target count, inclusive minimum/maximum length, and maximum bends; parallel-line count uses the max-parallel setting), top/bottom power-source rows, contact spacing and style, the active-source span and power-source trapezoid constraints, routing dimensions, per-placement power-route attempts, complete-generation seed attempts, background grid settings, and reproducible seeds. Complete-generation retries use deterministic fallback seeds derived from the current seed and occur only after randomized placement/routing exhaustion.

Generation is debounced while settings change. When newer settings trigger generation, the browser aborts the previous fetch and ignores any stale response. The page saves the SVG currently displayed in the preview. A fixed seed keeps the randomized composition reproducible; the Generate new layout button changes that seed when fixed-seed mode is enabled.

The server accepts JSON at POST /api/generate and returns an SVG string. Request bodies are limited to 1 MiB, unknown JSON fields are rejected, and the server applies restrictive browser security headers.
