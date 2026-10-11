package quic_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/net/quic"
)

func ExampleTransport() {
	// Example of initializing a Transport (a real application would provide a *net.UDPConn).
	t := &quic.Transport{}

	// Server and client would be established over this transport.
	fmt.Printf("Transport ready: %T\n", t)

	// Output:
	// Transport ready: *quic.Transport
}
