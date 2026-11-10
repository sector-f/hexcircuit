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
		Width: o.Width, Height: o.Height, BaseColor: o.BaseColor, AccentColor: o.AccentColor,
		Background: o.Background, ComponentCount: o.ComponentCount, ClusterCount: o.ClusterCount,
		MinComponentsPerCluster: o.MinComponentsPerCluster, ComponentRadius: o.ComponentRadius,
		ComponentStrokeWidth: o.ComponentStrokeWidth,
		ComponentMiddleHexagonScale: o.ComponentMiddleHexagonScale,
		ComponentInnerHexagonScale: o.ComponentInnerHexagonScale,
		Connections: o.Connections, MaxParallel: o.MaxParallel, Pitch: o.Pitch,
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
	req.AccentColor = "#abcdef"
	req.TopPowerSourceCount = 8
	req.BottomPowerSourceCount = 9
	req.PowerSourceSpacing = "adjacent"
	req.PowerSourceStyle = "double"
	req.ConstrainComponentsToPowerSourceSpan = true
	req.ConstrainLayoutToPowerSourceTrapezoid = true

	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseColor != req.BaseColor || got.AccentColor != req.AccentColor {
		t.Fatalf("colors = (%q, %q), want (%q, %q)", got.BaseColor, got.AccentColor, req.BaseColor, req.AccentColor)
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
	if !got.SeedSet || got.Seed != 42 {
		t.Fatalf("seed = (%d, %v), want (42, true)", got.Seed, got.SeedSet)
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
		{"unknown spacing", func(r *generateRequest) { r.PowerSourceSpacing = "wide" }},
		{"unknown source style", func(r *generateRequest) { r.PowerSourceStyle = "triple" }},
		{"too few source contacts", func(r *generateRequest) { r.TopPowerSourceCount = 1; r.BottomPowerSourceCount = 1 }},
		{"trapezoid without both rows", func(r *generateRequest) { r.ConstrainLayoutToPowerSourceTrapezoid = true; r.TopPowerSourceCount = 0 }},
		{"invalid component scales", func(r *generateRequest) { r.ComponentInnerHexagonScale = 0.8 }},
		{"invalid seed", func(r *generateRequest) { r.Seed = "seed" }},
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
	for _, want := range []string{"PCB Layout Generator", "constrainLayoutToPowerSourceTrapezoid", "powerSourceSpacing"} {
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
