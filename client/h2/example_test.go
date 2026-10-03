package h2_test

import (
	"context"
	"net"
	"time"

	"github.com/lemon4ksan/mach/client/h2"
	"github.com/lemon4ksan/mach/proto/http"
)

func ExampleConn() {
	c, _ := net.Dial("tcp", "127.0.0.1:443")
	conn := h2.NewConn(c, h2.ConnOpts{})

	if err := conn.Handshake(); err != nil {
		return
	}
	defer conn.Close()

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)
	res := http.AcquireResponse()
	defer http.ReleaseResponse(res)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_ = conn.Do(ctx, req, res)
}
