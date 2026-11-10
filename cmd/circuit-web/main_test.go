package main

import (
	"testing"

	"github.com/sector-f/hexgrid/generators/circuit"
)

func validGenerateRequest() generateRequest {
	o := circuit.DefaultOptions()
	return generateRequest{
		Width: o.Width, Height: o.Height, Color: o.Color, Background: o.Background,
		Orientation: o.Orientation, ComponentCount: o.ComponentCount,
		ComponentRadius: o.ComponentRadius, RoutingPitch: o.RoutingPitch,
		AutoRoutingPitch: o.AutoRoutingPitch, GridRadius: o.GridRadius,
		GridGap: o.GridGap, GridWidth: o.GridWidth,
		OmitPartialGridHexagons: o.OmitPartialGridHexagons,
		Connections: o.Connections, MaxParallel: o.MaxParallel,
		TrackCountWeights: "11.1111,44.44445,44.44445",
		ParallelSpacing: o.ParallelSpacing, StrokeWidth: o.StrokeWidth,
		TraceWidth: o.TraceWidth, NodeRadius: o.NodeRadius,
		TraceClearance: o.TraceClearance, DetailProb: o.DetailProb,
		ShowLabels: o.ShowLabels, LabelFont: o.LabelFont,
		LabelFontSize: o.LabelFontSize, LabelProbability: o.LabelProbability,
		FontColor: o.FontColor, FontColorSpecified: o.FontColorSpecified,
		LabelDistance: o.LabelDistance, LabelInsideComponents: o.LabelInsideComponents,
		UseDeterministicRouteCost: o.UseDeterministicRouteCost,
		FallbackEdgeOrder: string(o.FallbackEdgeOrder), MaxRouteSteps: o.MaxRouteSteps,
		RouteAttempts: o.RouteAttempts, Seed: "42", SeedSet: true,
	}
}

func TestGenerateRequestMapsGridColor(t *testing.T) {
	req := validGenerateRequest()
	req.Color = "#123456"
	req.GridColor = "#abcdef"

	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if got.Color != req.Color || got.GridColor != req.GridColor {
		t.Fatalf("colors = (%q, %q), want (%q, %q)", got.Color, got.GridColor, req.Color, req.GridColor)
	}
}

func TestGenerateRequestAllowsUnspecifiedGridColor(t *testing.T) {
	req := validGenerateRequest()
	req.GridColor = ""

	got, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if got.GridColor != "" {
		t.Fatalf("unspecified grid color = %q, want empty fallback value", got.GridColor)
	}
}

func TestGenerateRequestRejectsInvalidGridColor(t *testing.T) {
	req := validGenerateRequest()
	req.GridColor = "not a color"

	if _, err := req.options(); err == nil {
		t.Fatal("expected invalid grid color to be rejected")
	}
}
