package circuit

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	hexgrid "github.com/sector-f/hexcircuit/hexgrid"
)

// componentLabelWords is intentionally hardcoded for now. A word's ordinal is
// advanced only after a label is placed, so a failed placement cannot create gaps
// such as "Filter 2" without a corresponding "Filter 1".
var componentLabelWords = []string{
	"VCA", // Voltage-Controlled Amplifier
	"VCO", // Voltage-Controlled Oscillator
	"BGA", // Ball Grid Array
	"PGA", // Pin Grid Array
	"LGA", // Land Grid Array
	"CPU", // Central Processing Unit
	"CAPACITOR",
	"EG",   // Envelope Generator
	"ADSR", // Attack Decay Sustain Release
	"RECTIFIER",
	"LPF", // Low Pass Filter
	"BPF", // Band Pass Filter
	"HPF", // High Pass Filter
	"CLOCK",
}

// labelRect is the measured glyph bounding box plus a small safety inset. All values
// are in inches, matching the geometry used by the SVG generator.
type labelRect = hexgrid.Rect

// componentLabel is a placed SVG label. X and Y identify the text origin and baseline;
// Bounds contains the measured text ink bounds plus safety padding. Inside is true when
// it sits in its component.
type componentLabel = hexgrid.Label

const (
	componentLabelSVGUnitsPerInch = 1000.0
	labelTextPadding              = 0.005 // inches of safety padding around measured glyph bounds
	labelObjectGap                = 0.006 // inches of clearance between labels and unrelated geometry
)

// componentLabelFontColor returns the explicit label font color when configured,
// otherwise preserving the generator's existing default color. FontColorSpecified
// is separate from FontColor so an omitted value does not override that default.
func componentLabelFontColor(o Options) string {
	if o.FontColorSpecified {
		return o.FontColor
	}
	return o.Color
}

// Label diagnostics are opt-in so normal generation remains quiet. Set
// HEXAGONS_DEBUG_LABELS=1 when investigating placements.
func labelDebugEnabled() bool {
	value := strings.TrimSpace(os.Getenv("HEXAGONS_DEBUG_LABELS"))
	return value != "" && value != "0" && !strings.EqualFold(value, "false")
}

func labelDebugf(format string, args ...any) {
	if labelDebugEnabled() {
		fmt.Fprintf(os.Stderr, "[component-labels] "+format+"\n", args...)
	}
}

// labelFont uses Fontconfig to resolve a system font family to the actual installed
// font file, then loads that file for direct glyph measurement. SVG output uses the
// resolved family name so the SVG viewer requests the same face Fontconfig selected.
type labelFont struct {
	Face   font.Face
	Family string
}

// labelMetrics contains the actual glyph bounds relative to the SVG text origin and
// baseline. The values are in model inches. SVG's initial user coordinate system uses
// CSS pixels; with a 96-DPI face, measured pixels correspond to SVG user units. The
// generator uses 1000 SVG user units per model inch, so measurements must be divided
// by that scale after conversion from fixed-point pixels.
type labelMetrics = hexgrid.TextMetrics

// loadLabelFont resolves a Linux system font through fc-match and reads its real
// glyph metrics. It returns an error instead of silently guessing if Fontconfig,
// the requested font, or the font file cannot be used.
func loadLabelFont(family string, sizePoints float64) (*labelFont, error) {
	format := "%{family}\\n%{file}\\n%{index}\\n"
	output, err := exec.Command("fc-match", "--format="+format, "--", family).Output()
	if err != nil {
		return nil, fmt.Errorf("resolve label font %q with fc-match: %w (install Fontconfig if it is missing)", family, err)
	}
	fields := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(fields) < 3 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
		return nil, fmt.Errorf("fc-match returned incomplete font information for %q", family)
	}
	families := strings.Split(fields[0], ",")
	resolvedFamily := strings.TrimSpace(families[0])
	// Fontconfig may report several family aliases for a face (for example,
	// "Lato,Lato Medium"). Preserve the requested alias when it is one of them
	// so SVG resolves the same face instead of silently selecting the regular style.
	for _, candidate := range families {
		candidate = strings.TrimSpace(candidate)
		if strings.EqualFold(candidate, strings.TrimSpace(family)) {
			resolvedFamily = candidate
			break
		}
	}
	fontPath := strings.TrimSpace(fields[1])
	fontIndexText := strings.TrimSpace(fields[2])
	// Fontconfig's index is normally an integer. Some builds append a named-instance
	// suffix after ':', while the first number remains the face index in a TTC/OTC.
	if colon := strings.IndexByte(fontIndexText, ':'); colon >= 0 {
		fontIndexText = fontIndexText[:colon]
	}
	fontIndex, err := strconv.Atoi(fontIndexText)
	if err != nil || fontIndex < 0 {
		return nil, fmt.Errorf("fc-match returned invalid font index %q for %q", fields[2], family)
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("read label font file %q: %w", fontPath, err)
	}
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, fmt.Errorf("parse label font file %q: %w", fontPath, err)
	}
	if fontIndex >= collection.NumFonts() {
		return nil, fmt.Errorf("fontconfig selected face %d from %q, but the file contains only %d faces", fontIndex, fontPath, collection.NumFonts())
	}
	fontData, err := collection.Font(fontIndex)
	if err != nil {
		return nil, fmt.Errorf("load face %d from label font file %q: %w", fontIndex, fontPath, err)
	}
	face, err := opentype.NewFace(fontData, &opentype.FaceOptions{
		Size: sizePoints,
		// SVG converts CSS points to CSS pixels at 96 DPI before applying the
		// viewBox transform. Match that conversion so the measured glyph bounds
		// use the same user-unit scale as the generated SVG text.
		DPI:     96,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("create label font face for %q: %w", family, err)
	}

	loaded := &labelFont{Face: face, Family: resolvedFamily}
	// The word list is ASCII. Validate every letter, space, and decimal digit up front
	// so unsupported glyphs cannot cause the measured bounds to disagree with SVG's
	// fallback-font rendering.
	characters := make(map[rune]struct{})
	for _, word := range componentLabelWords {
		for _, r := range word {
			characters[r] = struct{}{}
		}
	}
	for _, r := range " 0123456789" {
		characters[r] = struct{}{}
	}
	for r := range characters {
		if _, _, ok := face.GlyphBounds(r); !ok {
			_ = face.Close()
			return nil, fmt.Errorf("label font %q does not contain the required %q glyph", resolvedFamily, string(r))
		}
	}
	return loaded, nil
}

// measure returns the glyph ink bounds from the loaded font's actual outline
// metrics. BoundString applies the font's kerning data as part of string layout.
func (f *labelFont) measure(text string) labelMetrics {
	bounds, _ := font.BoundString(f.Face, text)
	const fixedPointScale = 64.0
	toInches := func(value fixed.Int26_6) float64 {
		// The 96-DPI face returns CSS-pixel-sized metrics, which map to SVG
		// user units. Convert those units to model inches (1000 units/inch).
		return float64(value) / (fixedPointScale * componentLabelSVGUnitsPerInch)
	}
	return labelMetrics{
		MinX: toInches(bounds.Min.X),
		MinY: toInches(bounds.Min.Y),
		MaxX: toInches(bounds.Max.X),
		MaxY: toInches(bounds.Max.Y),
	}
}

// placeComponentLabels selects words, finds collision-free positions, and numbers
// only labels that were actually placed. It uses its own RNG stream so enabling labels
// does not perturb component placement, route selection, or inner-detail randomness.
func placeComponentLabels(
	o Options,
	scene hexgrid.Scene,
	seed int64,
	baseAngle float64,
	font *labelFont,
) []componentLabel {
	if !o.ShowLabels || o.LabelProbability <= 0 || len(scene.Components) == 0 {
		return nil
	}

	labelRNG := rand.New(rand.NewSource(seed ^ int64(0x4c4142454c)))
	counts := make(map[string]int, len(componentLabelWords))
	labels := make([]componentLabel, 0, int(math.Round(float64(len(scene.Components))*o.LabelProbability)))
	placementScene := scene
	placementScene.Labels = append([]hexgrid.Label(nil), scene.Labels...)

	if labelDebugEnabled() {
		labelDebugf("begin: canvas=(%.6f x %.6f in; %.0f x %.0f SVG units), components=%d, probability=%.4f, distance=%.6f in, font=%q size=%.3f pt, baseAngle=%.6f rad",
			o.Width, o.Height, o.Width*componentLabelSVGUnitsPerInch, o.Height*componentLabelSVGUnitsPerInch, len(scene.Components), o.LabelProbability, o.LabelDistance, font.Family, o.LabelFontSize, baseAngle)
		for _, component := range scene.Components {
			labelDebugf("input component: index=%d center=(%.6f, %.6f) in / (%.3f, %.3f) SVG units, radius=%.6f in / %.3f SVG units",
				component.Index, component.X, component.Y, component.X*1000, component.Y*1000, component.Radius, component.Radius*1000)
		}
	}

	for _, component := range scene.Components {
		if labelRNG.Float64() >= o.LabelProbability {
			labelDebugf("skip component index=%d center=(%.6f, %.6f) in: probability check did not select it", component.Index, component.X, component.Y)
			continue
		}
		labelDebugf("trying component index=%d center=(%.6f, %.6f) in radius=%.6f in", component.Index, component.X, component.Y, component.Radius)

		// The generator chooses label text and side order. hexgrid owns the geometric
		// placement and returns an error when a candidate collides with occupied space.
		wordOrder := labelRNG.Perm(len(componentLabelWords))
		placed := false
		for _, wordIndex := range wordOrder {
			word := componentLabelWords[wordIndex]
			text := fmt.Sprintf("%s %d", word, counts[word]+1)
			tmetrics := font.measure(text)
			sideOrder := []int{0, 1, 2, 3, 4, 5}
			labelRNG.Shuffle(len(sideOrder), func(i, j int) {
				sideOrder[i], sideOrder[j] = sideOrder[j], sideOrder[i]
			})
			for _, side := range sideOrder {
				candidate, err := hexgrid.TryPlaceLabel(placementScene, hexgrid.LabelRequest{
					ComponentIndex: component.Index,
					Text:           text,
					Metrics:        tmetrics,
					Side:           side,
					Gap:            o.LabelDistance,
					Padding:        labelTextPadding,
					ObjectGap:      labelObjectGap,
				})
				if err != nil {
					continue
				}
				labels = append(labels, candidate)
				placementScene.Labels = append(placementScene.Labels, candidate)
				counts[word]++
				placed = true
				labelDebugf("PLACED: component_index=%d owner_center=(%.6f, %.6f) in / (%.3f, %.3f) SVG units radius=%.6f in; text=%q inside=%t anchor=%q; origin=(%.6f, %.6f) in / (%.3f, %.3f) SVG units; padded_bounds=[min=(%.6f, %.6f), max=(%.6f, %.6f)] in / [min=(%.3f, %.3f), max=(%.3f, %.3f)] SVG units",
					component.Index, component.X, component.Y, component.X*componentLabelSVGUnitsPerInch, component.Y*componentLabelSVGUnitsPerInch, component.Radius, candidate.Text, candidate.Inside, candidate.Anchor, candidate.X, candidate.Y, candidate.X*componentLabelSVGUnitsPerInch, candidate.Y*componentLabelSVGUnitsPerInch, candidate.Bounds.MinX, candidate.Bounds.MinY, candidate.Bounds.MaxX, candidate.Bounds.MaxY, candidate.Bounds.MinX*componentLabelSVGUnitsPerInch, candidate.Bounds.MinY*componentLabelSVGUnitsPerInch, candidate.Bounds.MaxX*componentLabelSVGUnitsPerInch, candidate.Bounds.MaxY*componentLabelSVGUnitsPerInch)
				break
			}
			if placed {
				break
			}
		}

		// Internal placement is a fallback: retain the same candidate word order,
		// but ask hexgrid to center the label and check the full padded rectangle.
		if !placed && o.LabelInsideComponents {
			labelDebugf("no external placement found for component index=%d; trying internal placement", component.Index)
			for _, wordIndex := range wordOrder {
				word := componentLabelWords[wordIndex]
				text := fmt.Sprintf("%s %d", word, counts[word]+1)
				candidate, err := hexgrid.TryPlaceLabel(placementScene, hexgrid.LabelRequest{
					ComponentIndex: component.Index,
					Text:           text,
					Metrics:        font.measure(text),
					Inside:         true,
					Gap:            o.LabelDistance,
					Padding:        labelTextPadding,
					ObjectGap:      labelObjectGap,
				})
				if err != nil {
					continue
				}
				labels = append(labels, candidate)
				placementScene.Labels = append(placementScene.Labels, candidate)
				counts[word]++
				placed = true
				labelDebugf("PLACED INSIDE: component_index=%d owner_center=(%.6f, %.6f) in / (%.3f, %.3f) SVG units; text=%q; origin=(%.6f, %.6f) in / (%.3f, %.3f) SVG units; padded_bounds=[min=(%.6f, %.6f), max=(%.6f, %.6f)] in",
					component.Index, component.X, component.Y, component.X*componentLabelSVGUnitsPerInch, component.Y*componentLabelSVGUnitsPerInch, candidate.Text, candidate.X, candidate.Y, candidate.X*componentLabelSVGUnitsPerInch, candidate.Y*componentLabelSVGUnitsPerInch, candidate.Bounds.MinX, candidate.Bounds.MinY, candidate.Bounds.MaxX, candidate.Bounds.MaxY)
				break
			}
		}
		if !placed {
			labelDebugf("NO LABEL: component index=%d center=(%.6f, %.6f) in; all attempted external positions%s were rejected", component.Index, component.X, component.Y, map[bool]string{true: " and internal positions", false: ""}[o.LabelInsideComponents])
		}
	}

	labelDebugf("end: placed %d label(s)", len(labels))
	return labels
}

// externalLabelCandidates is retained as a thin compatibility helper for the package's
// geometry tests. The candidate geometry itself is implemented by hexgrid.
func externalLabelCandidates(
	component Component,
	text string,
	metrics labelMetrics,
	width, height, gap, baseAngle float64,
) []componentLabel {
	return hexgrid.ExternalLabelCandidates(
		hexgrid.Component{
			Index: component.Index,
			X:     component.X, Y: component.Y,
			Radius: component.Radius,
		},
		text, metrics, width, height, gap, baseAngle, labelTextPadding,
	)
}

// axisSign is retained for the component-label geometry tests; hexgrid owns its logic.
func axisSign(value float64) float64 { return hexgrid.AxisSign(value) }
