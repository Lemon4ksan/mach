package h1_test

import (
	"context"
	"net"
	"time"

	"github.com/lemon4ksan/mach/client/h1"
	"github.com/lemon4ksan/mach/proto/http"
)

func ExampleClientConn() {
	// Simulated network connection.
	c, _ := net.Dial("tcp", "127.0.0.1:80")

	// Initialize connection state (allocates once).
	conn := h1.NewClientConn(c)
	defer conn.Close()

	// Acquire message structures from pool.
	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)
	res := http.AcquireResponse()
	defer http.ReleaseResponse(res)

	// Configure request.
	req.Header.SetMethod("GET")
	req.SetRequestURI("/")

	// Execute with zero hot-path allocations.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_ = conn.Do(ctx, req, res)
}
