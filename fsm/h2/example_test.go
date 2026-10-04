package h2_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/fsm/h2"
)

type exampleHandler struct{}

func (h *exampleHandler) OnStreamData(streamID uint32, payload []byte)   {}
func (h *exampleHandler) OnHeadersReady(streamID uint32, endStream bool) {}
func (h *exampleHandler) OnStreamClosed(streamID uint32, err error)      {}
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
