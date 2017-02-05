package main

import (
	"testing"
	"time"
)

func TestRandomDuration(t *testing.T) {
	testData := []struct{ start, end string }{
		{"100s", "125s"},
		{"10s", "12.5s"},
		{"1s", "1.25s"},
		{"21h", "26.25h"},
	}
	for j := 1; j <= 1024; j++ {
		for _, interval := range testData {
			start, err := time.ParseDuration(interval.start)
			if err != nil {
				t.Fatalf("Parsing test data: %v", err)
			}
			end, err := time.ParseDuration(interval.end)
			if err != nil {
				t.Fatalf("Parsing test data: %v", err)
			}
			random := randomize(start)
			if float64(random) < float64(start) {
				t.Fatalf("Random before expected range: %v < %v", random, start)
			}
			if float64(random) > float64(end) {
				t.Fatalf("Random beyond expected range: %v > %v", random, end)
			}
		}
	}
}
