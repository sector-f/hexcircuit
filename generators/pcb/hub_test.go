package pcb

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/sector-f/hexgrid"
)

func TestBuildBoardCreatesVisibleHubsAndHubSpokes(t *testing.T) {
	o := DefaultOptions()
	o.ComponentCount = 12
	o.ClusterCount = 3
	o.MinComponentsPerCluster = 2
	o.MinPowerSourcesPerCluster = 0
	o.MaxPowerSourcesPerCluster = 0
	o.TopPowerSourceCount = 0
	o.BottomPowerSourceCount = 0
	o.ConnectorCount = 0
	o.Connections = 0
	o.InterClusterTraces = false
	o.DecorativeTraceCount = 0

	top, bottom, err := powerSourcePositions(o)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildBoard(o, rand.New(rand.NewSource(8128)), top, bottom)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.HubComponents) != len(plan.Clusters) {
		t.Fatalf("got %d visible hubs for %d clusters", len(plan.HubComponents), len(plan.Clusters))
	}

	wantSpokes := 0
	for _, cluster := range plan.Clusters {
		wantSpokes += len(cluster.Components) - 1
	}
	if len(plan.SignalTraces) != wantSpokes {
		t.Fatalf("got %d hub traces, want %d", len(plan.SignalTraces), wantSpokes)
	}

	seen := make(map[hexgrid.GridPosition]bool, len(plan.SignalTraces))
	for _, trace := range plan.SignalTraces {
		if len(trace.Path) == 0 {
			t.Fatalf("hub trace from %v has no destination", trace.Start)
		}
		if trace.Start == trace.Path[len(trace.Path)-1] {
			t.Fatalf("hub trace at %v loops back to itself", trace.Start)
		}
		endpoint := trace.Path[len(trace.Path)-1]
		matchedCluster := false
		for _, cluster := range plan.Clusters {
			if trace.Start != cluster.Hub {
				continue
			}
			matchedCluster = true
			found := false
			for _, component := range cluster.Components {
				if component.Position == endpoint && endpoint != cluster.Hub {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("hub trace from %v ends at non-member position %v", trace.Start, endpoint)
			}
		}
		if !matchedCluster {
			t.Fatalf("hub trace starts at unknown cluster position %v", trace.Start)
		}
		if seen[endpoint] {
			t.Fatalf("multiple hub traces terminate at %v", endpoint)
		}
		seen[endpoint] = true
	}

	svg, err := renderBoard(o, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svg, `id="hub-components"`) {
		t.Fatal("rendered SVG is missing the dedicated hub component layer")
	}
	if !strings.Contains(svg, `stroke="`+o.AccentColor+`"`) {
		t.Fatal("rendered SVG does not draw hubs in the accent color")
	}
}

func TestChoosePowerTargetsPrefersOuterComponents(t *testing.T) {
	hub := hexgrid.GridPosition{Q: 0, R: 0}
	parent := hub
	components := []placedComponent{
		{Position: hub},
		{Position: hexgrid.GridPosition{Q: 1, R: 0}, Parent: &parent},
		{Position: hexgrid.GridPosition{Q: 0, R: 1}, Parent: &parent},
	}
	targets := choosePowerTargets(components, 8, rand.New(rand.NewSource(41)))
	for _, target := range targets {
		if target == hub {
			t.Fatalf("power feed targets include central hub %v despite available outer components", hub)
		}
	}

	singleton := choosePowerTargets([]placedComponent{{Position: hub}}, 1, rand.New(rand.NewSource(41)))
	if len(singleton) != 1 || singleton[0] != hub {
		t.Fatalf("singleton cluster power target = %v, want hub position %v", singleton, hub)
	}
}
