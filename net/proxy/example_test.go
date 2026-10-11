package proxy_test

import (
	"fmt"

	"github.com/lemon4ksan/mach/net/proxy"
)

func ExampleFromEnvironment() {
	// Creates a dialer based on HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables.
	dialer := proxy.FromEnvironment()
	if dialer != nil {
		fmt.Println("Dialer configured from environment")
	} else {
		fmt.Println("Dialer is nil")
	}

	// Output:
	// Dialer configured from environment
}
