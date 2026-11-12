package geometry

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// LabelCandidate is one text alternative in a label-placement request. SideOrder
// controls the order in which the component's six sides are tried. SideOrderProvider
// is an optional lazy equivalent, useful when side order is selected randomly.
type LabelCandidate struct {
	Text               string
	SideOrder          []int
	SideOrderProvider  func() []int
}

// LabelGroupRequest asks geometry to place at most one label for a component.
// Candidates and their sides are tried in order. CandidateProvider is an optional
// lazy alternative to Candidates. Numbered appends a per-text 1-based counter,
// incremented only after a successful placement. InsideFallback tries a centered
// placement after all external candidates fail.
type LabelGroupRequest struct {
	ComponentIndex   int
	Candidates       []LabelCandidate
	CandidateProvider func() []LabelCandidate
	Numbered         bool
	InsideFallback   bool
	Gap              float64
	Padding          float64
	ObjectGap        float64
}

type labelFont struct {
	Face   font.Face
	Family string
}

// PlaceLabelGroups measures candidate text with the configured font, then attempts
// collision-aware placement in request order. Candidate and side-order providers are
// invoked only when their candidates are reached, allowing deterministic random
// choices to preserve their intended consumption order. Previously placed labels
// are included when positioning later requests. It returns only labels placed here.
func PlaceLabelGroups(scene *Scene, requests []LabelGroupRequest) ([]Label, error) {
	if scene == nil {
		return nil, fmt.Errorf("label scene cannot be nil")
	}
	if len(requests) == 0 {
		return nil, nil
	}
	for requestIndex, request := range requests {
		if request.ComponentIndex < 0 || request.ComponentIndex >= len(scene.Components) {
			return nil, fmt.Errorf("label request %d refers to invalid component index %d", requestIndex, request.ComponentIndex)
		}
		for _, value := range []float64{request.Gap, request.Padding, request.ObjectGap} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return nil, fmt.Errorf("label request %d has a non-finite or negative gap or padding", requestIndex)
			}
		}
	}
	family := strings.TrimSpace(scene.LabelFontFamily)
	if family == "" {
		family = "monospace"
	}
	if math.IsNaN(scene.LabelFontSize) || math.IsInf(scene.LabelFontSize, 0) || scene.LabelFontSize <= 0 {
		return nil, fmt.Errorf("label font size must be finite and greater than zero")
	}
	loaded, err := loadLabelFont(family, scene.LabelFontSize)
	if err != nil {
		return nil, err
	}
	defer loaded.Face.Close()
	if err := loaded.validateCharacters(" 0123456789"); err != nil {
		return nil, err
	}
	scene.LabelFontFamily = loaded.Family

	counts := make(map[string]int)
	placed := make([]Label, 0, len(requests))
	for _, request := range requests {
		candidates := request.Candidates
		if request.CandidateProvider != nil {
			candidates = request.CandidateProvider()
		}
		if len(candidates) == 0 {
			continue
		}
		for _, candidate := range candidates {
			if err := loaded.validateCharacters(candidate.Text); err != nil {
				return nil, err
			}
		}

		placeCandidate := func(candidate LabelCandidate, inside bool) (Label, bool, error) {
			text := candidate.Text
			if request.Numbered {
				text = fmt.Sprintf("%s %d", text, counts[candidate.Text]+1)
			}
			metrics, measureErr := loaded.measure(text)
			if measureErr != nil {
				return Label{}, false, measureErr
			}
			if inside {
				label, placeErr := TryPlaceLabel(*scene, LabelRequest{
					ComponentIndex: request.ComponentIndex,
					Text: text, Metrics: metrics, Inside: true,
					Gap: request.Gap, Padding: request.Padding, ObjectGap: request.ObjectGap,
				})
				if placeErr == nil {
					return label, true, nil
				}
				return Label{}, false, nil
			}

			sides := candidate.SideOrder
			if candidate.SideOrderProvider != nil {
				sides = candidate.SideOrderProvider()
			}
			if len(sides) == 0 {
				sides = []int{0, 1, 2, 3, 4, 5}
			}
			for _, side := range sides {
				if side < 0 || side >= hexSideCount {
					return Label{}, false, fmt.Errorf("label candidate %q has invalid side %d; sides must be between 0 and 5", candidate.Text, side)
				}
				label, placeErr := TryPlaceLabel(*scene, LabelRequest{
					ComponentIndex: request.ComponentIndex,
					Text: text, Metrics: metrics, Side: side,
					Gap: request.Gap, Padding: request.Padding, ObjectGap: request.ObjectGap,
				})
				if placeErr == nil {
					return label, true, nil
				}
			}
			return Label{}, false, nil
		}

		accepted := false
		for _, candidate := range candidates {
			label, ok, placeErr := placeCandidate(candidate, false)
			if placeErr != nil {
				return nil, placeErr
			}
			if !ok {
				continue
			}
			scene.Labels = append(scene.Labels, label)
			placed = append(placed, label)
			counts[candidate.Text]++
			accepted = true
			break
		}
		if accepted || !request.InsideFallback {
			continue
		}
		for _, candidate := range candidates {
			label, ok, placeErr := placeCandidate(candidate, true)
			if placeErr != nil {
				return nil, placeErr
			}
			if !ok {
				continue
			}
			scene.Labels = append(scene.Labels, label)
			placed = append(placed, label)
			counts[candidate.Text]++
			break
		}
	}
	return placed, nil
}

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
	for _, candidate := range families {
		candidate = strings.TrimSpace(candidate)
		if strings.EqualFold(candidate, strings.TrimSpace(family)) {
			resolvedFamily = candidate
			break
		}
	}
	fontPath := strings.TrimSpace(fields[1])
	fontIndexText := strings.TrimSpace(fields[2])
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
		Size: sizePoints, DPI: 96, Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("create label font face for %q: %w", family, err)
	}
	return &labelFont{Face: face, Family: resolvedFamily}, nil
}

func (f *labelFont) validateCharacters(text string) error {
	for _, r := range text {
		if _, _, ok := f.Face.GlyphBounds(r); !ok {
			return fmt.Errorf("label font %q does not contain the required %q glyph", f.Family, string(r))
		}
	}
	return nil
}

func (f *labelFont) measure(text string) (TextMetrics, error) {
	if err := f.validateCharacters(text); err != nil {
		return TextMetrics{}, err
	}
	bounds, _ := font.BoundString(f.Face, text)
	const fixedPointScale = 64.0
	toInches := func(value fixed.Int26_6) float64 {
		return float64(value) / (fixedPointScale * 1000.0)
	}
	return TextMetrics{
		MinX: toInches(bounds.Min.X), MinY: toInches(bounds.Min.Y),
		MaxX: toInches(bounds.Max.X), MaxY: toInches(bounds.Max.Y),
	}, nil
}
