package main

import (
	"flag"
	"testing"
)

func TestParse(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	port := fs.Int("port", 1, "")
	yes := fs.Bool("yes", false, "")
	pos := parse(fs, []string{"dns-speed", "--port", "7878", "--yes"})
	if *port != 7878 || !*yes || len(pos) != 1 || pos[0] != "dns-speed" {
		t.Fatalf("port=%d yes=%v pos=%v", *port, *yes, pos)
	}
}

func TestHasFlag(t *testing.T) {
	if !hasFlag([]string{"set-dns", "--measure"}, "measure") || hasFlag([]string{"--measured"}, "measure") {
		t.Fatal("hasFlag")
	}
}
