package h3_test

import (
	"testing"

	"github.com/lemon4ksan/mach/server/h3"
)

func TestNewServerConnAllocations(t *testing.T) {
	handler := func(req *h3.ServerRequest, res *h3.ServerResponse) error {
		return nil
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = h3.NewServerConn(nil, handler)
	})

	if allocs != 28 {
		t.Errorf("NewServerConn allocated %v times, expected 28", allocs)
	}
}
