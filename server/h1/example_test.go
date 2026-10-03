package h1_test

import (
	"fmt"
	"time"

	"github.com/lemon4ksan/mach/server/h1"
)

func ExampleConnHandler() {
	handler := &h1.ConnHandler{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
		MaxBodySize:  1024 * 1024,
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Body = append(res.Body[:0], []byte("Hello, World!")...)
			return nil
		},
	}

	// In a real application, you would accept connections in a loop
	// listener, _ := net.Listen("tcp", ":8080")
	// conn, _ := listener.Accept()
	// handler.ServeConn(conn)

	_ = handler // unused in this stub
	fmt.Println("Example initialized")
	// Output: Example initialized
}
