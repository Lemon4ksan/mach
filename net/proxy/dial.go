// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"context"
	"net"
)

// A ContextDialer dials using a context.
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Dial works like DialContext on net.Dialer but using a dialer returned by FromEnvironment.
//
// The passed ctx is only used for returning the Conn, not the lifetime of the Conn.
// A Conn returned from a successful Dial after the context has been cancelled will be immediately closed.
func Dial(ctx context.Context, network, address string) (net.Conn, error) {
	d := FromEnvironment()
	if xd, ok := d.(ContextDialer); ok {
		return xd.DialContext(ctx, network, address)
	}

	return dialContext(ctx, d, network, address)
}

func dialContext(ctx context.Context, d Dialer, network, address string) (net.Conn, error) {
	if cd, ok := d.(ContextDialer); ok {
		return cd.DialContext(ctx, network, address)
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	type dialResult struct {
		conn net.Conn
		err  error
	}

	resCh := make(chan dialResult, 1)
	go func() {
		c, err := d.Dial(network, address)
		resCh <- dialResult{conn: c, err: err}
	}()

	select {
	case <-ctx.Done():
		go func() {
			res := <-resCh
			if res.conn != nil {
				_ = res.conn.Close()
			}
		}()

		return nil, ctx.Err()

	case res := <-resCh:
		if res.err != nil {
			return nil, res.err
		}

		if ctx.Err() != nil {
			if res.conn != nil {
				_ = res.conn.Close()
			}

			return nil, ctx.Err()
		}

		return res.conn, nil
	}
}
