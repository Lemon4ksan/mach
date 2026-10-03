package h2_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/server/h2"
)

func ExampleServerConn() {
	handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = 200
		res.Body = append(res.Body[:0], []byte("Hello, HTTP/2!")...)
		return nil
	}

	// In a real application, accept connections and serve
	// listener, _ := net.Listen("tcp", ":8080")
	// conn, _ := listener.Accept()
	// sc := h2.NewServerConn(conn, handler)
	// _ = sc.Serve()

	_ = handler
	fmt.Println("Example initialized")
	// Output: Example initialized
}
