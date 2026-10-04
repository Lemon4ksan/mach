package h2_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/fsm/h2"
)

type exampleHandler struct{}

func (h *exampleHandler) OnStreamData(streamID uint32, payload []byte, endStream bool) {}
func (h *exampleHandler) OnHeaders(streamID uint32, payload []byte, endHeaders, endStream bool) {}
func (h *exampleHandler) OnContinuation(streamID uint32, payload []byte, endHeaders bool) {}
func (h *exampleHandler) OnSettings(payload []byte, ack bool) {}
func (h *exampleHandler) OnPing(payload []byte, ack bool) {}
func (h *exampleHandler) OnWindowUpdate(streamID uint32, increment uint32) {}
func (h *exampleHandler) OnStreamReset(streamID uint32, errorCode uint32) {}
func (h *exampleHandler) OnGoAway(lastStreamID uint32, errorCode uint32) {}
func (h *exampleHandler) OnWrite(payload []byte) {
	fmt.Printf("write: %d bytes\n", len(payload))
}

func ExampleStateMachine() {
	handler := &exampleHandler{}
	sm := h2.NewStateMachine(handler)
	sm.Feed([]byte{})
	// The FSM will trigger callbacks on the handler instead of returning an event.
	fmt.Println("done")

	// Output:
	// done
}
