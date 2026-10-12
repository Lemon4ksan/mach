package fsm_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/h2/fsm"
)

type dummyHandler struct{}

func (d *dummyHandler) OnStreamData(streamID uint32, payload []byte, endStream bool)          {}
func (d *dummyHandler) OnHeaders(streamID uint32, payload []byte, endHeaders, endStream bool) {}
func (d *dummyHandler) OnContinuation(streamID uint32, payload []byte, endHeaders bool)       {}
func (d *dummyHandler) OnSettings(payload []byte, ack bool)                                   {}
func (d *dummyHandler) OnPing(payload []byte, ack bool)                                       {}
func (d *dummyHandler) OnWindowUpdate(streamID, increment uint32)                             {}
func (d *dummyHandler) OnStreamReset(streamID, errorCode uint32)                              {}
func (d *dummyHandler) OnGoAway(lastStreamID, errorCode uint32)                               {}
func (d *dummyHandler) OnPriority(streamID uint32, payload []byte)                            {}
func (d *dummyHandler) OnUnknownFrame(streamID uint32, frameType, flags uint8, payload []byte) {
}
func (d *dummyHandler) OnWrite(payload []byte) {}

func TestAlloc(t *testing.T) {
	sm := fsm.NewStateMachine(&dummyHandler{})
	data := []byte{}

	allocs := testing.AllocsPerRun(1000, func() {
		sm.Feed(data)
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}
