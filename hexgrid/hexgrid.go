// Package hexgrid provides geometry and rendering primitives for hexagonal circuit art.
// It deliberately does not validate whether traces collide or whether a scene is a
// sensible circuit; callers own those generation rules.
package hexgrid

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"

	svgo "github.com/ajstarks/svgo"
)

const (
	unitsPerInch     = 1000.0
	cssPixelsPerInch = 96.0
	sqrt3            = 1.7320508075688772935
	epsilon          = 1e-9
	vectorEpsilon    = 1e-12
	hexSideCount     = 6
	degreesPerSide   = 60.0

	largeInnerDetailScale  = 0.60
	smallInnerDetailScale  = 0.34
	innerDetailStrokeScale = 0.78
)

// Point is a position in model-space inches.
type Point struct {
	X, Y float64
}

// Rect is an axis-aligned rectangle in model-space inches.
type Rect struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

// TextMetrics describes the glyph's ink bounds relative to its text origin and baseline.
// Its values use model-space inches, matching the rest of the geometry API.
type TextMetrics struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

func (m TextMetrics) Width() float64  { return m.MaxX - m.MinX }
func (m TextMetrics) Height() float64 { return m.MaxY - m.MinY }

// Grid describes the visible hexagonal lattice. Dimensions are in inches.
type Grid struct {
	Pitch       float64
	Radius      float64
	StrokeWidth float64
	OmitPartial bool
}

// Cell is one axial grid cell and its position in model-space inches.
type Cell struct {
	Q, R   int
	Center Point
}

// ComponentDetail specifies which optional internal motif the renderer should draw.
// The caller chooses the detail; rendering itself is deterministic.
type ComponentDetail uint8

const (
	ComponentDetailNone ComponentDetail = iota
	ComponentDetailNestedHexagons
)

// Component is the visual geometry of one circuit component. Index is an opaque ID
// used to associate labels with components; it need not be sequential.
type Component struct {
	Index  int
	X, Y   float64
	Radius float64
	Detail ComponentDetail
}

// Trace is one polyline. A scene may contain traces that cross components or each other;
// the renderer draws the provided geometry without applying circuit-validity rules.
type Trace struct {
	Points []Point
}

// Terminal is a hollow circular trace endpoint.
type Terminal struct {
	X, Y   float64
	Radius float64
}

// Label is an already placed label. X and Y are the text origin and baseline; Bounds
// is the padded label rectangle used by TryPlaceLabel to keep later labels separated.
type Label struct {
	ComponentIndex int
	Text           string
	X, Y           float64
	Anchor         string
	Bounds         Rect
	Inside         bool
}

// Scene describes both the geometry to draw and its visual styling. Coordinates and
// dimensions use inches. An empty LabelColor falls back to Color. The renderer preserves
// the established layer order: grid,
// traces, components, labels, then terminal pads.
type Scene struct {
	Width, Height float64
	Orientation   string
	Color         string
	Background    string

	Grid       Grid
	Components []Component
	Traces     []Trace
	Terminals  []Terminal
	Labels     []Label

	ComponentStrokeWidth float64
	TraceStrokeWidth     float64
	NodeRadius           float64

	LabelColor      string
	LabelFontFamily string
	LabelFontSize   float64
}

// LabelRequest describes one attempt to place text beside a component or inside it.
// Side is 0..5 for external placement; it is ignored when Inside is true. Metrics are
// the measured glyph bounds relative to the text origin and baseline.
type LabelRequest struct {
	ComponentIndex int
	Text           string
	Metrics        TextMetrics
	Side           int
	Inside         bool
	Gap            float64
	Padding        float64
	ObjectGap      float64
}

// TryPlaceLabel calculates and validates one label placement. It returns an error when
// the requested position does not fit on the canvas or intersects occupied geometry.
// It does not mutate the scene; append the returned label to Scene.Labels after success.
func TryPlaceLabel(scene Scene, request LabelRequest) (Label, error) {
	var owner Component
	found := false
	for _, component := range scene.Components {
		if component.Index == request.ComponentIndex {
			owner, found = component, true
			break
		}
	}
	if !found {
		return Label{}, fmt.Errorf("component index %d does not exist in the scene", request.ComponentIndex)
	}
	for _, value := range []float64{
		request.Metrics.MinX, request.Metrics.MinY, request.Metrics.MaxX, request.Metrics.MaxY,
		request.Gap, request.Padding, request.ObjectGap,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Label{}, fmt.Errorf("label placement measurements and gaps must be finite")
		}
	}
	if request.Metrics.Width() < 0 || request.Metrics.Height() < 0 || request.Gap < 0 || request.Padding < 0 || request.ObjectGap < 0 {
		return Label{}, fmt.Errorf("label dimensions and gaps must be non-negative")
	}
	if !request.Inside && (request.Side < 0 || request.Side >= hexSideCount) {
		return Label{}, fmt.Errorf("label side %d is outside the valid range 0..5", request.Side)
	}

	width := request.Metrics.Width() + 2*request.Padding
	height := request.Metrics.Height() + 2*request.Padding
	bounds := Rect{}
	if request.Inside {
		bounds = Rect{
			MinX: owner.X - width/2,
			MinY: owner.Y - height/2,
			MaxX: owner.X + width/2,
			MaxY: owner.Y + height/2,
		}
	} else {
		candidate := externalLabelCandidate(owner, request.Text, request.Metrics, width, height, request.Gap, orientationBaseAngle(scene.Orientation), request.Padding, request.Side)
		bounds = candidate.Bounds
	}

	label := Label{
		ComponentIndex: owner.Index,
		Text:           request.Text,
		X:              bounds.MinX + request.Padding - request.Metrics.MinX,
		Y:              bounds.MinY + request.Padding - request.Metrics.MinY,
		Anchor:         "start",
		Bounds:         bounds,
		Inside:         request.Inside,
	}
	if err := labelFits(scene, owner, label, request); err != nil {
		return Label{}, fmt.Errorf("label %q does not fit: %w", request.Text, err)
	}
	return label, nil
}

// ExternalLabelCandidates returns the six unvalidated placements for a text rectangle.
// width and height include any requested text padding; padding is used to calculate
// the SVG text origin from measured glyph metrics. This is useful when a caller wants
// to inspect or order candidate positions before checking each one with TryPlaceLabel.
func ExternalLabelCandidates(component Component, text string, metrics TextMetrics, width, height, gap, baseAngle, padding float64) []Label {
	positions := make([]Label, 0, hexSideCount)
	for side := 0; side < hexSideCount; side++ {
		positions = append(positions, externalLabelCandidate(component, text, metrics, width, height, gap, baseAngle, padding, side))
	}
	return positions
}

func externalLabelCandidate(component Component, text string, metrics TextMetrics, width, height, gap, baseAngle, padding float64, side int) Label {
	theta := baseAngle + (degreesPerSide*float64(side))*math.Pi/180.0
	normal := Point{X: math.Cos(theta), Y: math.Sin(theta)}
	apothem := component.Radius * sqrt3 / 2
	sidePoint := Point{X: component.X + apothem*normal.X, Y: component.Y + apothem*normal.Y}
	centerX := sidePoint.X + normal.X*gap + AxisSign(normal.X)*width/2
	centerY := sidePoint.Y + normal.Y*gap + AxisSign(normal.Y)*height/2
	bounds := Rect{
		MinX: centerX - width/2,
		MinY: centerY - height/2,
		MaxX: centerX + width/2,
		MaxY: centerY + height/2,
	}
	return Label{
		ComponentIndex: component.Index,
		Text:           text,
		X:              bounds.MinX + padding - metrics.MinX,
		Y:              bounds.MinY + padding - metrics.MinY,
		Anchor:         "start",
		Bounds:         bounds,
	}
}

// AxisSign returns -1, 0, or 1, treating floating-point residues near zero as zero.
func AxisSign(value float64) float64 { return axisSign(value) }

func labelFits(scene Scene, owner Component, candidate Label, request LabelRequest) error {
	bounds := candidate.Bounds
	for _, value := range []float64{bounds.MinX, bounds.MinY, bounds.MaxX, bounds.MaxY} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("label bounds are not finite")
		}
	}
	if bounds.MinX < request.ObjectGap || bounds.MinY < request.ObjectGap ||
		bounds.MaxX > scene.Width-request.ObjectGap || bounds.MaxY > scene.Height-request.ObjectGap {
		return fmt.Errorf("label extends beyond the canvas")
	}

	baseAngle := orientationBaseAngle(scene.Orientation)
	if candidate.Inside {
		inset := math.Max(request.Gap, scene.ComponentStrokeWidth/2+request.ObjectGap)
		if !rectFitsInsideHex(bounds, owner, baseAngle, inset) {
			return fmt.Errorf("label does not fit inside its component")
		}
	}

	// Treat the entire component polygon as occupied, not just its thin outline.
	componentCheck := expandRect(bounds, request.ObjectGap)
	for _, component := range scene.Components {
		if candidate.Inside && component.Index == owner.Index {
			continue
		}
		if rectIntersectsPolygon(componentCheck, HexVertices(component.X, component.Y, component.Radius, scene.Orientation)) {
			return fmt.Errorf("label intersects component %d", component.Index)
		}
	}

	for _, existing := range scene.Labels {
		if rectsOverlap(expandRect(bounds, request.ObjectGap), existing.Bounds) {
			return fmt.Errorf("label overlaps an existing label")
		}
	}

	traceClearance := scene.TraceStrokeWidth/2 + request.ObjectGap
	for _, trace := range scene.Traces {
		if trackIntersectsRect(trace.Points, bounds, traceClearance) {
			return fmt.Errorf("label intersects a trace")
		}
	}
	for _, terminal := range scene.Terminals {
		padClearance := terminal.Radius + scene.TraceStrokeWidth/2 + request.ObjectGap
		if pointRectDistance(Point{X: terminal.X, Y: terminal.Y}, bounds) <= padClearance {
			return fmt.Errorf("label intersects a terminal")
		}
	}
	return nil
}

// Render serializes a complete scene to SVG. It intentionally does not enforce
// topology or collision constraints; every provided component, trace and terminal is
// drawn in order, even when the geometry overlaps.
func Render(scene Scene) string {
	viewWidth := scene.Width * unitsPerInch
	viewHeight := scene.Height * unitsPerInch

	var out strings.Builder
	canvas := svgo.New(&out)
	canvas.Startraw(
		fmt.Sprintf(`viewBox="0 0 %.3f %.3f"`, viewWidth, viewHeight),
		fmt.Sprintf(`width="%.3f"`, scene.Width*cssPixelsPerInch),
		fmt.Sprintf(`height="%.3f"`, scene.Height*cssPixelsPerInch),
	)

	if bg := strings.ToLower(strings.TrimSpace(scene.Background)); bg != "none" &&
		bg != "transparent" && bg != "" {
		canvas.Group(`id="background-fill"`)
		canvas.Rect(
			0, 0,
			int(math.Round(viewWidth)),
			int(math.Round(viewHeight)),
			fmt.Sprintf("fill:%s;stroke:none", html.EscapeString(scene.Background)),
		)
		canvas.Gend()
	}

	// Layer 1: continue grid hexagons beyond the canvas edge unless configured otherwise.
	canvas.Group(
		`id="background-grid"`,
		`fill="none"`,
		fmt.Sprintf(`stroke="%s"`, html.EscapeString(scene.Color)),
		fmt.Sprintf(`stroke-width="%d"`, userUnits(scene.Grid.StrokeWidth)),
		`stroke-linejoin="round"`,
	)
	for _, cell := range GridCells(scene.Width, scene.Height, scene.Grid.Pitch, scene.Orientation, -scene.Grid.Radius) {
		center := cell.Center
		if scene.Grid.OmitPartial && !hexagonFullyInsideCanvas(
			center.X, center.Y, scene.Grid.Radius, scene.Grid.StrokeWidth,
			scene.Width, scene.Height, scene.Orientation,
		) {
			continue
		}
		drawPolygon(canvas, HexVertices(center.X, center.Y, scene.Grid.Radius, scene.Orientation), "")
	}
	canvas.Gend()

	// Layer 2: shorten trace ends so each trace stops at the edge of its terminal pad.
	canvas.Group(
		`id="traces"`,
		`fill="none"`,
		fmt.Sprintf(`stroke="%s"`, html.EscapeString(scene.Color)),
		fmt.Sprintf(`stroke-width="%d"`, userUnits(scene.TraceStrokeWidth)),
		`stroke-linecap="round"`,
		`stroke-linejoin="round"`,
	)
	for _, trace := range scene.Traces {
		drawPolyline(canvas, TrimEndpoint(trace.Points, scene.NodeRadius), "")
	}
	canvas.Gend()

	// Layer 3: component outlines and explicitly selected internal details.
	canvas.Group(
		`id="components"`,
		`fill="none"`,
		fmt.Sprintf(`stroke="%s"`, html.EscapeString(scene.Color)),
		fmt.Sprintf(`stroke-width="%d"`, userUnits(scene.ComponentStrokeWidth)),
		`stroke-linejoin="round"`,
	)
	for _, component := range scene.Components {
		drawPolygon(canvas, HexVertices(component.X, component.Y, component.Radius, scene.Orientation), "")
		if component.Detail == ComponentDetailNestedHexagons {
			for _, scale := range []float64{largeInnerDetailScale, smallInnerDetailScale} {
				inner := HexVertices(component.X, component.Y, component.Radius*scale, scene.Orientation)
				drawPolygon(canvas, inner, fmt.Sprintf("stroke-width:%d", userUnits(scene.ComponentStrokeWidth*innerDetailStrokeScale)))
			}
		}
	}
	canvas.Gend()

	labelColor := scene.LabelColor
	if labelColor == "" {
		labelColor = scene.Color
	}
	canvas.Group(
		`id="component-labels"`,
		fmt.Sprintf(`fill="%s"`, html.EscapeString(labelColor)),
		`stroke="none"`,
	)
	for _, label := range scene.Labels {
		style := fmt.Sprintf(
			"font-family:%s;font-size:%spt;font-kerning:normal;font-variant-ligatures:none;text-anchor:%s",
			cssFontFamily(scene.LabelFontFamily), formatFloat(scene.LabelFontSize), label.Anchor,
		)
		canvas.Text(userUnits(label.X), userUnits(label.Y), html.EscapeString(label.Text), html.EscapeString(style))
	}
	canvas.Gend()

	// Layer 4: hollow terminal pads are drawn last to keep their outlines visible.
	canvas.Group(
		`id="terminal-pads"`,
		`fill="none"`,
		fmt.Sprintf(`stroke="%s"`, html.EscapeString(scene.Color)),
		fmt.Sprintf(`stroke-width="%d"`, userUnits(scene.TraceStrokeWidth)),
	)
	for _, terminal := range scene.Terminals {
		canvas.Circle(userUnits(terminal.X), userUnits(terminal.Y), userUnits(terminal.Radius))
	}
	canvas.Gend()

	canvas.End()
	return out.String()
}

// HexVertices returns the six corners of a regular hexagon. Pointy orientation starts
// at the top; flat orientation starts at the right.
func HexVertices(cx, cy, radius float64, orientation string) []Point {
	start := -90.0
	if orientation == "flat" {
		start = 0
	}
	vertices := make([]Point, hexSideCount)
	for i := range vertices {
		a := (start + degreesPerSide*float64(i)) * math.Pi / 180
		vertices[i] = Point{X: cx + radius*math.Cos(a), Y: cy + radius*math.Sin(a)}
	}
	return vertices
}

// PointInsideHexWithMargin checks whether a point is within a regular hexagon's
// six half-planes after insetting every side by margin.
func PointInsideHexWithMargin(point Point, center Point, radius, baseAngle, margin float64) bool {
	allowed := radius*sqrt3/2 - margin
	if allowed <= 0 {
		return false
	}
	delta := Point{X: point.X - center.X, Y: point.Y - center.Y}
	for side := 0; side < hexSideCount; side++ {
		theta := baseAngle + (degreesPerSide*float64(side))*math.Pi/180.0
		normal := Point{X: math.Cos(theta), Y: math.Sin(theta)}
		if dot(delta, normal) > allowed+epsilon {
			return false
		}
	}
	return true
}

// PointInPolygon reports whether point lies inside a simple polygon using ray casting.
func PointInPolygon(point Point, polygon []Point) bool {
	x, y := point.X, point.Y
	inside := false
	j := len(polygon) - 1
	for i := 0; i < len(polygon); i++ {
		xi, yi, xj, yj := polygon[i].X, polygon[i].Y, polygon[j].X, polygon[j].Y
		if (yi > y) != (yj > y) {
			xIntersect := (xj-xi)*(y-yi)/(yj-yi) + xi
			if x < xIntersect {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// SegmentsIntersect treats touching endpoints and collinear overlap as intersections.
func SegmentsIntersect(a, b, c, d Point) bool {
	ab, cd, ac := sub(b, a), sub(d, c), sub(c, a)
	denominator := cross(ab, cd)
	if math.Abs(denominator) < epsilon {
		if math.Abs(cross(ac, ab)) > epsilon {
			return false
		}
		axis := 0
		if math.Abs(ab.Y) > math.Abs(ab.X) {
			axis = 1
		}
		var aa, bb, cc, dd float64
		if axis == 0 {
			aa, bb, cc, dd = a.X, b.X, c.X, d.X
		} else {
			aa, bb, cc, dd = a.Y, b.Y, c.Y, d.Y
		}
		if aa > bb {
			aa, bb = bb, aa
		}
		if cc > dd {
			cc, dd = dd, cc
		}
		return math.Max(aa, cc) <= math.Min(bb, dd)+epsilon
	}
	t := cross(ac, cd) / denominator
	u := cross(ac, ab) / denominator
	return t >= -epsilon && t <= 1+epsilon && u >= -epsilon && u <= 1+epsilon
}

// GridCells returns lattice cells whose centers fall within the canvas rectangle inset
// by margin. A negative margin deliberately includes off-canvas cells so the grid can
// continue seamlessly to the image edges. Cells are returned in stable q-then-r order.
func GridCells(width, height, pitch float64, orientation string, margin float64) []Cell {
	// A non-positive or non-finite pitch means there is no enumerable lattice.
	// Treat it as an empty grid rather than allowing a division by zero to create
	// an effectively unbounded search range.
	if pitch <= 0 || math.IsNaN(pitch) || math.IsInf(pitch, 0) ||
		width < 0 || height < 0 || math.IsNaN(width) || math.IsNaN(height) ||
		math.IsInf(width, 0) || math.IsInf(height, 0) {
		return nil
	}
	limit := int(math.Ceil((width+height)/pitch)) + 3
	baseAngle := orientationBaseAngle(orientation)
	theta1 := baseAngle + math.Pi/3
	result := make([]Cell, 0)
	for q := -limit; q <= limit; q++ {
		for r := -limit; r <= limit; r++ {
			qf, rf := float64(q), float64(r)
			center := Point{
				X: width/2.0 + pitch*(qf*math.Cos(baseAngle)+rf*math.Cos(theta1)),
				Y: height/2.0 + pitch*(qf*math.Sin(baseAngle)+rf*math.Sin(theta1)),
			}
			if margin <= center.X && center.X <= width-margin && margin <= center.Y && center.Y <= height-margin {
				result = append(result, Cell{Q: q, R: r, Center: center})
			}
		}
	}
	return result
}

func hexagonFullyInsideCanvas(cx, cy, radius, strokeWidth, width, height float64, orientation string) bool {
	margin := strokeWidth / 2
	for _, vertex := range HexVertices(cx, cy, radius, orientation) {
		if vertex.X < margin || vertex.X > width-margin || vertex.Y < margin || vertex.Y > height-margin {
			return false
		}
	}
	return true
}

func orientationBaseAngle(orientation string) float64 {
	if orientation == "flat" {
		return 30.0 * math.Pi / 180.0
	}
	return -60.0 * math.Pi / 180.0
}

// TrimEndpoint returns a copy of a trace shortened at its final point by radius.
// The original trace is never modified.
func TrimEndpoint(track []Point, radius float64) []Point {
	trimmed := append([]Point(nil), track...)
	if len(trimmed) < 2 {
		return trimmed
	}
	end := trimmed[len(trimmed)-1]
	prev := trimmed[len(trimmed)-2]
	dx, dy := end.X-prev.X, end.Y-prev.Y
	segmentLength := math.Hypot(dx, dy)
	if segmentLength <= radius {
		return trimmed[:len(trimmed)-1]
	}
	trimmed[len(trimmed)-1] = Point{
		X: end.X - dx/segmentLength*radius,
		Y: end.Y - dy/segmentLength*radius,
	}
	return trimmed
}

func drawPolygon(canvas *svgo.SVG, points []Point, style string) {
	x, y := make([]int, len(points)), make([]int, len(points))
	for i, point := range points {
		x[i], y[i] = userUnits(point.X), userUnits(point.Y)
	}
	if style == "" {
		canvas.Polygon(x, y)
	} else {
		canvas.Polygon(x, y, style)
	}
}

func drawPolyline(canvas *svgo.SVG, points []Point, style string) {
	x, y := make([]int, len(points)), make([]int, len(points))
	for i, point := range points {
		x[i], y[i] = userUnits(point.X), userUnits(point.Y)
	}
	if style == "" {
		canvas.Polyline(x, y)
	} else {
		canvas.Polyline(x, y, style)
	}
}

func userUnits(inches float64) int { return int(math.Round(inches * unitsPerInch)) }
func formatFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func cssFontFamily(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	switch lower {
	case "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui",
		"ui-serif", "ui-sans-serif", "ui-monospace", "ui-rounded", "emoji", "math", "fangsong":
		return lower
	}
	quoted := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		"\n", " ",
		"\r", " ",
		"\t", " ",
	).Replace(name)
	return "'" + quoted + "'"
}

func axisSign(value float64) float64 {
	if value > epsilon {
		return 1
	}
	if value < -epsilon {
		return -1
	}
	return 0
}

func rectFitsInsideHex(rect Rect, component Component, baseAngle, margin float64) bool {
	for _, corner := range rectCorners(rect) {
		if !PointInsideHexWithMargin(corner, Point{X: component.X, Y: component.Y}, component.Radius, baseAngle, margin) {
			return false
		}
	}
	return true
}

func rectIntersectsPolygon(rect Rect, polygon []Point) bool {
	corners := rectCorners(rect)
	for _, corner := range corners {
		if PointInPolygon(corner, polygon) {
			return true
		}
	}
	for _, vertex := range polygon {
		if pointInRect(vertex, rect) {
			return true
		}
	}
	rectEdges := rectEdgePairs(rect)
	for i, vertex := range polygon {
		next := polygon[(i+1)%len(polygon)]
		for _, edge := range rectEdges {
			if SegmentsIntersect(vertex, next, edge[0], edge[1]) {
				return true
			}
		}
	}
	return false
}

func trackIntersectsRect(track []Point, rect Rect, clearance float64) bool {
	checked := expandRect(rect, clearance)
	for i := 0; i+1 < len(track); i++ {
		a, b := track[i], track[i+1]
		if pointInRect(a, checked) || pointInRect(b, checked) {
			return true
		}
		for _, edge := range rectEdgePairs(checked) {
			if SegmentsIntersect(a, b, edge[0], edge[1]) {
				return true
			}
		}
	}
	return false
}

func pointRectDistance(point Point, rect Rect) float64 {
	closest := Point{
		X: math.Max(rect.MinX, math.Min(rect.MaxX, point.X)),
		Y: math.Max(rect.MinY, math.Min(rect.MaxY, point.Y)),
	}
	return math.Hypot(point.X-closest.X, point.Y-closest.Y)
}

func pointInRect(point Point, rect Rect) bool {
	return point.X >= rect.MinX && point.X <= rect.MaxX &&
		point.Y >= rect.MinY && point.Y <= rect.MaxY
}

func rectsOverlap(a, b Rect) bool {
	return a.MinX < b.MaxX && a.MaxX > b.MinX &&
		a.MinY < b.MaxY && a.MaxY > b.MinY
}

func expandRect(rect Rect, amount float64) Rect {
	return Rect{MinX: rect.MinX - amount, MinY: rect.MinY - amount, MaxX: rect.MaxX + amount, MaxY: rect.MaxY + amount}
}

func rectCorners(rect Rect) []Point {
	return []Point{
		{X: rect.MinX, Y: rect.MinY},
		{X: rect.MaxX, Y: rect.MinY},
		{X: rect.MaxX, Y: rect.MaxY},
		{X: rect.MinX, Y: rect.MaxY},
	}
}

func rectEdgePairs(rect Rect) [][2]Point {
	corners := rectCorners(rect)
	return [][2]Point{
		{corners[0], corners[1]},
		{corners[1], corners[2]},
		{corners[2], corners[3]},
		{corners[3], corners[0]},
	}
}

func dot(a, b Point) float64   { return a.X*b.X + a.Y*b.Y }
func cross(a, b Point) float64 { return a.X*b.Y - a.Y*b.X }
func sub(a, b Point) Point     { return Point{X: a.X - b.X, Y: a.Y - b.Y} }
