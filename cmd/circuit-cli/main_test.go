package main

import (
	"os"
	"testing"
)

func TestParseOptionsAcceptsSeparateGridColor(t *testing.T) {
	options, err := parseOptions([]string{"--color", "#123456", "--grid-color", "#abcdef"}, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if options.Generator.Color != "#123456" {
		t.Fatalf("component/trace color = %q, want #123456", options.Generator.Color)
	}
	if options.Generator.GridColor != "#abcdef" {
		t.Fatalf("grid color = %q, want #abcdef", options.Generator.GridColor)
	}
}
