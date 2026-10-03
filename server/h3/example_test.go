package h3_test

import (
	"fmt"

	"github.com/lemon4ksan/foundation/net/quic"
	"github.com/lemon4ksan/mach/server/h3"
)

func ExampleServerConn() {
	handler := func(req *h3.ServerRequest, res *h3.ServerResponse) error {
		res.StatusCode = 200
		res.Body = append(res.Body[:0], []byte("Hello, HTTP/3!")...)
		return nil
	}

	// In a real application, you accept a QUIC connection:
	// listener, _ := quic.Listen("udp", ":443", tlsConfig)
	// conn, _ := listener.Accept()
	// sc := h3.NewServerConn(conn, handler)
	// _ = sc.Serve(context.Background())

	var fakeConn *quic.Conn // Example placeholder
	sc := h3.NewServerConn(fakeConn, handler)

	_ = sc
	fmt.Println("Example initialized")
	// Output: Example initialized
}
