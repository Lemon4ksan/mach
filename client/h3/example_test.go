package h3_test

import (
	"context"
	"time"

	"github.com/lemon4ksan/foundation/net/quic"

	"github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/proto/http"
)

func ExampleClientConn() {
	var qconn *quic.Conn // Assume established QUIC connection

	conn, err := h3.NewClientConn(qconn, nil)
	if err != nil {
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

	_, _ = conn.Do(ctx, req, res, nil)
}
