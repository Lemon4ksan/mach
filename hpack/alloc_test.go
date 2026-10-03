package hpack_test

import (
	"testing"

	"github.com/lemon4ksan/mach/hpack"
)

func TestAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		hf.Set(":method", "GET")
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}
