package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/sector-f/hexgrid/generators/pcb"
)

func validRequest() generateRequest {
	o := pcb.DefaultOptions()
	return generateRequest{
		Width: o.Width, Height: o.Height, BaseColor: o.BaseColor, GridColor: o.GridColor, AccentColor: o.AccentColor,
		BaseGlow: o.BaseGlow, BaseGlowBlurRadius: o.BaseGlowBlurRadius, BaseGlowOpacity: o.BaseGlowOpacity,
		AccentGlow: o.AccentGlow, AccentGlowBlurRadius: o.AccentGlowBlurRadius, AccentGlowOpacity: o.AccentGlowOpacity,
		Background: o.Background, ComponentCount: o.ComponentCount, ClusterCount: o.ClusterCount,
		MinComponentsPerCluster: o.MinComponentsPerCluster,
		MinPowerSourcesPerCluster: o.MinPowerSourcesPerCluster, MaxPowerSourcesPerCluster: o.MaxPowerSourcesPerCluster,
		ComponentRadius: o.ComponentRadius,
		ComponentStrokeWidth: o.ComponentStrokeWidth,
		ComponentMiddleHexagonScale: o.ComponentMiddleHexagonScale,
		ComponentInnerHexagonScale: o.ComponentInnerHexagonScale,
		Connections: o.Connections, DecorativeTraceCount: o.DecorativeTraceCount,
		DecorativeTraceMinLength: o.DecorativeTraceMinLength, DecorativeTraceMaxLength: o.DecorativeTraceMaxLength,
		InterClusterTraces: o.InterClusterTraces, MaxParallel: o.MaxParallel,
		RouteAttempts: o.RouteAttempts, GenerationAttempts: o.GenerationAttempts, Pitch: o.Pitch,
		TraceStrokeWidth: o.TraceStrokeWidth, TraceSpacing: o.TraceSpacing, NodeRadius: o.NodeRadius,
		ShowBackgroundGrid: o.ShowBackgroundGrid, OmitPartialGridHexagons: o.OmitPartialGridHexagons,
		GridRadius: o.GridRadius, GridStrokeWidth: o.GridStrokeWidth,
		TopPowerSourceCount: o.TopPowerSourceCount, BottomPowerSourceCount: o.BottomPowerSourceCount,
		PowerSourceSpacing: string(o.PowerSourceSpacing), PowerSourceStyle: string(o.PowerSourceStyle),
		ConstrainComponentsToPowerSourceSpan: o.ConstrainComponentsToPowerSourceSpan,
		ConstrainLayoutToPowerSourceTrapezoid: o.ConstrainLayoutToPowerSourceTrapezoid,
		ConnectorInset: o.ConnectorInset, Seed: "42", SeedSet: true,
	}
}

func TestGenerateRequestMapsToPCBOptions(t *testing.T) {
	req := validRequest()
	req.BaseColor = "#123456"
	req.GridColor = "#fedcba"
	req.AccentColor = "#abcdef"
	req.TopPowerSourceCount = 8
	req.BottomPowerSourceCount = 9
	req.PowerSourceSpacing = "adjacent"
	req.PowerSourceStyle = "double"
	req.ConstrainComponentsToPowerSourceSpan = true
	req.ConstrainLayoutToPowerSourceTrapezoid = true
	req.InterClusterTraces = true
	req.DecorativeTraceCount = 5
	req.DecorativeTraceMinLength = 3
	req.DecorativeTraceMaxLength = 6
	req.DecorativeTraceMaxBends = 2
	req.RouteAttempts = 7
	req.GenerationAttempts = 6
	req.BaseGlow = true
	req.BaseGlowBlurRadius = 0.035
	req.BaseGlowOpacity = 0.25
	req.AccentGlow = true
	req.AccentGlowBlurRadius = 0.085
	req.AccentGlowOpacity = 0.75

	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseColor != req.BaseColor || got.AccentColor != req.AccentColor || got.GridColor != req.GridColor {
		t.Fatalf("colors = (base %q, grid %q, accent %q), want (base %q, grid %q, accent %q)",
			got.BaseColor, got.GridColor, got.AccentColor, req.BaseColor, req.GridColor, req.AccentColor)
	}
	if !got.BaseGlow || got.BaseGlowBlurRadius != req.BaseGlowBlurRadius || got.BaseGlowOpacity != req.BaseGlowOpacity {
		t.Fatalf("base glow settings = (%v, %v, %v), want (%v, %v, %v)",
			got.BaseGlow, got.BaseGlowBlurRadius, got.BaseGlowOpacity,
			req.BaseGlow, req.BaseGlowBlurRadius, req.BaseGlowOpacity)
	}
	if !got.AccentGlow || got.AccentGlowBlurRadius != req.AccentGlowBlurRadius || got.AccentGlowOpacity != req.AccentGlowOpacity {
		t.Fatalf("accent glow settings = (%v, %v, %v), want (%v, %v, %v)",
			got.AccentGlow, got.AccentGlowBlurRadius, got.AccentGlowOpacity,
			req.AccentGlow, req.AccentGlowBlurRadius, req.AccentGlowOpacity)
	}
	if got.MinPowerSourcesPerCluster != 1 || got.MaxPowerSourcesPerCluster != 1 {
		t.Fatalf("power-source range = %d..%d, want 1..1", got.MinPowerSourcesPerCluster, got.MaxPowerSourcesPerCluster)
	}
	if got.TopPowerSourceCount != 8 || got.BottomPowerSourceCount != 9 {
		t.Fatalf("power source counts = (%d, %d), want (8, 9)", got.TopPowerSourceCount, got.BottomPowerSourceCount)
	}
	if got.PowerSourceSpacing != pcb.PowerSourceSpacingAdjacent || got.PowerSourceStyle != pcb.PowerSourceStyleDouble {
		t.Fatalf("power-source enums = (%q, %q), want adjacent/double", got.PowerSourceSpacing, got.PowerSourceStyle)
	}
	if !got.ConstrainComponentsToPowerSourceSpan || !got.ConstrainLayoutToPowerSourceTrapezoid {
		t.Fatal("layout constraints were not mapped")
	}
	if !got.InterClusterTraces {
		t.Fatal("inter-cluster trace option was not mapped")
	}
	if got.DecorativeTraceCount != req.DecorativeTraceCount || got.DecorativeTraceMinLength != req.DecorativeTraceMinLength || got.DecorativeTraceMaxLength != req.DecorativeTraceMaxLength || got.DecorativeTraceMaxBends != req.DecorativeTraceMaxBends {
		t.Fatalf("decorative-trace options = (%d, %d, %d), want (%d, %d, %d)",
			got.DecorativeTraceCount, got.DecorativeTraceMinLength, got.DecorativeTraceMaxLength,
			req.DecorativeTraceCount, req.DecorativeTraceMinLength, req.DecorativeTraceMaxLength)
	}
	if got.RouteAttempts != 7 { t.Fatalf("route attempts = %d, want 7", got.RouteAttempts) }
	if got.GenerationAttempts != 6 { t.Fatalf("generation attempts = %d, want 6", got.GenerationAttempts) }
	if !got.SeedSet || got.Seed != 42 {
		t.Fatalf("seed = (%d, %v), want (42, true)", got.Seed, got.SeedSet)
	}
}

func TestGenerateRequestAllowsUnpoweredClusters(t *testing.T) {
	req := validRequest()
	req.MinPowerSourcesPerCluster = 0
	req.MaxPowerSourcesPerCluster = 0
	req.TopPowerSourceCount = 0
	req.BottomPowerSourceCount = 0

	got, err := req.options()
	if err != nil {
		t.Fatalf("zero-source request should be valid: %v", err)
	}
	if got.MinPowerSourcesPerCluster != 0 || got.MaxPowerSourcesPerCluster != 0 ||
		got.TopPowerSourceCount != 0 || got.BottomPowerSourceCount != 0 {
		t.Fatalf("zero-source options were not preserved: min/max=%d/%d, rows=%d/%d",
			got.MinPowerSourcesPerCluster, got.MaxPowerSourcesPerCluster,
			got.TopPowerSourceCount, got.BottomPowerSourceCount)
	}
}

func TestGenerateRequestDefaultsGenerationAttemptsWhenOmitted(t *testing.T) {
	req := validRequest()
	req.GenerationAttempts = 0
	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if got.GenerationAttempts != pcb.DefaultOptions().GenerationAttempts {
		t.Fatalf("generation attempts = %d, want %d", got.GenerationAttempts, pcb.DefaultOptions().GenerationAttempts)
	}
}

func TestGenerateRequestDefaultsRouteAttemptsWhenOmitted(t *testing.T) {
	req := validRequest()
	req.RouteAttempts = 0
	got, err := req.options()
	if err != nil { t.Fatal(err) }
	if got.RouteAttempts != pcb.DefaultOptions().RouteAttempts { t.Fatalf("route attempts = %d, want %d", got.RouteAttempts, pcb.DefaultOptions().RouteAttempts) }
}

func TestGenerateRequestDefaultsGlowParametersWhenOmitted(t *testing.T) {
	req := validRequest()
	req.BaseGlowBlurRadius = 0
	req.BaseGlowOpacity = 0
	req.AccentGlowBlurRadius = 0
	req.AccentGlowOpacity = 0

	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	defaults := pcb.DefaultOptions()
	if got.BaseGlowBlurRadius != defaults.BaseGlowBlurRadius ||
		got.BaseGlowOpacity != defaults.BaseGlowOpacity ||
		got.AccentGlowBlurRadius != defaults.AccentGlowBlurRadius ||
		got.AccentGlowOpacity != defaults.AccentGlowOpacity {
		t.Fatalf("glow defaults = (%v, %v, %v, %v), want (%v, %v, %v, %v)",
			got.BaseGlowBlurRadius, got.BaseGlowOpacity, got.AccentGlowBlurRadius, got.AccentGlowOpacity,
			defaults.BaseGlowBlurRadius, defaults.BaseGlowOpacity, defaults.AccentGlowBlurRadius, defaults.AccentGlowOpacity)
	}
}

func TestGenerateRequestAllowsRandomSeed(t *testing.T) {
	req := validRequest()
	req.SeedSet = false
	req.Seed = ""
	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if got.SeedSet || got.Seed != 0 {
		t.Fatalf("random seed state = (%d, %v), want (0, false)", got.Seed, got.SeedSet)
	}
}

func TestGenerateRequestRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*generateRequest)
	}{
		{"invalid color", func(r *generateRequest) { r.BaseColor = "not a color!" }},
		{"invalid grid color", func(r *generateRequest) { r.GridColor = "not a color!" }},
		{"unknown spacing", func(r *generateRequest) { r.PowerSourceSpacing = "wide" }},
		{"unknown source style", func(r *generateRequest) { r.PowerSourceStyle = "triple" }},
		{"too few source contacts", func(r *generateRequest) { r.TopPowerSourceCount = 1; r.BottomPowerSourceCount = 1 }},
		{"invalid power-source range", func(r *generateRequest) { r.MinPowerSourcesPerCluster = 3; r.MaxPowerSourcesPerCluster = 2 }},
		{"too many power feeds", func(r *generateRequest) { r.MaxPowerSourcesPerCluster = 7 }},
		{"trapezoid without both rows", func(r *generateRequest) { r.ConstrainLayoutToPowerSourceTrapezoid = true; r.TopPowerSourceCount = 0 }},
		{"invalid component scales", func(r *generateRequest) { r.ComponentInnerHexagonScale = 0.8 }},
		{"negative route attempts", func(r *generateRequest) { r.RouteAttempts = -1 }},
		{"too many route attempts", func(r *generateRequest) { r.RouteAttempts = 1001 }},
		{"negative generation attempts", func(r *generateRequest) { r.GenerationAttempts = -1 }},
		{"too many generation attempts", func(r *generateRequest) { r.GenerationAttempts = 21 }},
		{"negative decorative-trace count", func(r *generateRequest) { r.DecorativeTraceCount = -1 }},
		{"decorative-trace count too large", func(r *generateRequest) { r.DecorativeTraceCount = 1001 }},
		{"decorative-trace minimum too short", func(r *generateRequest) { r.DecorativeTraceMinLength = 2 }},
		{"decorative-trace max bends too large", func(r *generateRequest) { r.DecorativeTraceMaxBends = 11 }},
		{"decorative-trace maximum below minimum", func(r *generateRequest) { r.DecorativeTraceMinLength = 4; r.DecorativeTraceMaxLength = 3 }},
		{"decorative-trace maximum too large", func(r *generateRequest) { r.DecorativeTraceMaxLength = 101 }},
		{"invalid seed", func(r *generateRequest) { r.Seed = "seed" }},
		{"negative base glow blur radius", func(r *generateRequest) { r.BaseGlowBlurRadius = -0.1 }},
		{"base glow opacity above one", func(r *generateRequest) { r.BaseGlowOpacity = 1.1 }},
		{"negative accent glow blur radius", func(r *generateRequest) { r.AccentGlowBlurRadius = -0.1 }},
		{"accent glow opacity above one", func(r *generateRequest) { r.AccentGlowOpacity = 1.1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			tt.edit(&req)
			if _, err := req.options(); err == nil {
				t.Fatal("expected invalid request to be rejected")
			}
		})
	}
}


func TestHandleIndexRendersPCBInterface(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handleIndex(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want HTML", got)
	}
	for _, want := range []string{"PCB Layout Generator", "constrainLayoutToPowerSourceTrapezoid", "powerSourceSpacing", "minPowerSourcesPerCluster", "maxPowerSourcesPerCluster", "interClusterTraces", "decorativeTraceCount", "decorativeTraceMinLength", "decorativeTraceMaxLength", "decorativeTraceMaxBends", "gridColor", "routeAttempts", "generationAttempts", "baseGlowEnabled", "baseGlowBlurRadius", "baseGlowOpacity", "accentGlowEnabled", "accentGlowBlurRadius", "accentGlowOpacity"} {
		if !bytes.Contains(response.Body.Bytes(), []byte(want)) {
			t.Errorf("rendered HTML is missing %q", want)
		}
	}
	if bytes.Contains(response.Body.Bytes(), []byte("{{.Seed}}")) {
		t.Fatal("rendered HTML still contains the seed template placeholder")
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("value=\"")) {
		t.Fatal("rendered HTML does not contain form values")
	}
}

func TestHandleGenerateRejectsUnknownJSONFields(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewBufferString(`{"unexpected":true}`))
	response := httptest.NewRecorder()

	handleGenerate(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestHandleGenerateRejectsTrailingJSONValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewBufferString(`{} {}`))
	response := httptest.NewRecorder()

	handleGenerate(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestPositiveFinite(t *testing.T) {
	for _, value := range []float64{0, -1} {
		if positiveFinite(value) {
			t.Errorf("positiveFinite(%s) = true", strconv.FormatFloat(value, 'g', -1, 64))
		}
	}
}

func TestGenerateRequestDefaultsDecorativeTraceLengthsWhenOmitted(t *testing.T) {
	req := validRequest()
	req.DecorativeTraceMinLength = 0
	req.DecorativeTraceMaxLength = 0
	req.DecorativeTraceMaxBends = 0
	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	defaults := pcb.DefaultOptions()
	if got.DecorativeTraceMinLength != defaults.DecorativeTraceMinLength || got.DecorativeTraceMaxLength != defaults.DecorativeTraceMaxLength || got.DecorativeTraceMaxBends != defaults.DecorativeTraceMaxBends {
		t.Fatalf("decorative-trace defaults = (%d, %d, %d), want (%d, %d, %d)",
			got.DecorativeTraceMinLength, got.DecorativeTraceMaxLength, got.DecorativeTraceMaxBends,
			defaults.DecorativeTraceMinLength, defaults.DecorativeTraceMaxLength, defaults.DecorativeTraceMaxBends)
	}
}
