package main

import (
	"os"
	"testing"
)

func TestParseOptionsAcceptsGenerationAttempts(t *testing.T) {
	options, err := parseOptions([]string{"--generation-attempts=6"}, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if options.Generator.GenerationAttempts != 6 {
		t.Fatalf("generation attempts = %d, want 6", options.Generator.GenerationAttempts)
	}
}

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
