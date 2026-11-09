package main

import (
	"log"
	"os"

	"github.com/sector-f/hexgrid"
)

func main() {
	options := hexgrid.DefaultOptions()
	options.Width = 8
	options.Height = 6
	options.Color = "#396cf8"

	layout := hexgrid.GridScene{
		Options: options,
		Components: []hexgrid.GridComponent{
			{Position: hexgrid.GridPosition{Q: -2, R: 0}, Style: hexgrid.ComponentStyleSingle},
			{Position: hexgrid.GridPosition{Q: 2, R: 0}, Style: hexgrid.ComponentStyleTriple},
			{Position: hexgrid.GridPosition{Q: -2, R: 3}, Style: hexgrid.ComponentStyleDouble},
		},
		Traces: []hexgrid.GridTrace{
			{
				Start: hexgrid.GridPosition{Q: -2, R: 0},
				Path: []hexgrid.GridPosition{
					// {Q: 2, R: -4},
					{Q: 2, R: 0},
				},
				Lines:        2,
				HideTerminal: true,
			},
			{
				Start: hexgrid.GridPosition{Q: -2, R: 3},
				Path: []hexgrid.GridPosition{
					{Q: 0, R: 3},
					{Q: 0, R: 1},
				},
				Lines: 1,
			},
		},
	}

	svg, err := hexgrid.RenderGrid(layout)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("refactored-circuit.svg", []byte(svg), 0644); err != nil {
		log.Fatal(err)
	}
}
