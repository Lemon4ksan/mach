package hpack_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/hpack"
)

func ExampleHPACK() {
	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	hf.Set(":method", "GET")
	fmt.Println(hf.Key())

	// Output:
	// :method
}
