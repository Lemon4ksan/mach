package h2_test

import (
	"testing"
)

func TestAllocations(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		// parsing boundaries
	})
	if allocs > 0 {
		t.Errorf("got %v allocs, want 0", allocs)
	}
}
