package main

import "testing"

func TestJobConcurrencyBounds(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]int{"": 2, "1": 1, "8": 8, "0": 2, "9": 2, "bad": 2} {
		if got := jobConcurrency(input); got != want {
			t.Errorf("jobConcurrency(%q) = %d, want %d", input, got, want)
		}
	}
}
