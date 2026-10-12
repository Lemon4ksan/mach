// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"fmt"
	"net"

	"github.com/lemon4ksan/mach/proto/ws"
)

// ExampleWebSocket_Echo demonstrates high-speed WebSocket echo handling
// using zero-allocation wire framing and vectorized SWAR masking.
func ExampleWebSocket_Echo() {
	// 1. Establish an in-process TCP listener to pair client and server endpoints
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Printf("listen error: %v\n", err)
		return
	}
	defer ln.Close()

	serverCh := make(chan *ws.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverCh <- ws.NewServerConn(conn)
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		fmt.Printf("dial error: %v\n", err)
		return
	}

	client := ws.NewClientConn(clientConn)
	server := <-serverCh
	defer client.Close()
	defer server.Close()

	// 2. Start the server echo worker
	go func() {
		op, payload, err := server.ReadMessage()
		if err != nil {
			return
		}
		_ = server.WriteMessage(op, payload)
	}()

	// 3. Client transmits a message and reads the echoed payload
	if err := client.WriteText("Hello, Mach WebSocket!"); err != nil {
		fmt.Printf("write error: %v\n", err)
		return
	}

	op, echo, err := client.ReadMessage()
	if err != nil {
		fmt.Printf("read error: %v\n", err)
		return
	}

	fmt.Printf("Opcode: %s\n", op)
	fmt.Printf("Echoed: %s\n", string(echo))

	// Output:
	// Opcode: text
	// Echoed: Hello, Mach WebSocket!
}
