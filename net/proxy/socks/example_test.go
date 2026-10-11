package socks_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/net/proxy/socks"
)

func ExampleNewDialer() {
	dialer := socks.NewDialer("tcp", "127.0.0.1:1080")
	if dialer != nil {
		fmt.Println("SOCKS5 dialer created")
	}

	// Output:
	// SOCKS5 dialer created
}
