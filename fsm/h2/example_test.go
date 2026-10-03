package h2_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/fsm/h2"
)

func ExampleStateMachine() {
	sm := h2.NewStateMachine()
	sm.Feed([]byte{})
	event := sm.NextEvent()
	fmt.Println(event)

	// Output:
	// <nil>
}
