package h2_test

import (
	"testing"

	"github.com/lemon4ksan/mach/fsm/h2"
)

func TestAlloc(t *testing.T) {
	sm := h2.NewStateMachine()
	data := []byte{}

	allocs := testing.AllocsPerRun(1000, func() {
		sm.Feed(data)
		sm.NextEvent()
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}
