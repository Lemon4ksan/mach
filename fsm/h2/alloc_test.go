package h2_test

import (
	"testing"

	"github.com/lemon4ksan/mach/fsm/h2"
)

type dummyHandler struct{}

func (d *dummyHandler) OnStreamData(streamID uint32, payload []byte)   {}
func (d *dummyHandler) OnHeadersReady(streamID uint32, endStream bool) {}
func (d *dummyHandler) OnStreamClosed(streamID uint32, err error)      {}
func (d *dummyHandler) OnWrite(payload []byte)                         {}

func TestAlloc(t *testing.T) {
	sm := h2.NewStateMachine(&dummyHandler{})
	data := []byte{}

	allocs := testing.AllocsPerRun(1000, func() {
		sm.Feed(data)
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}
