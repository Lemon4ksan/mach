// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"context"
	"net"

	"github.com/lemon4ksan/mach/proto/http"
)

type ClientConn struct {
	conn net.Conn
	bw   *bufio.Writer
	br   *bufio.Reader
}

func NewClientConn(c net.Conn) *ClientConn {
	return &ClientConn{
		conn: c,
		bw:   bufio.NewWriter(c),
		br:   bufio.NewReader(c),
	}
}

func (cc *ClientConn) Do(ctx context.Context, req *http.Request, res *http.Response) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := req.Write(cc.bw); err != nil {
		return err
	}
	if err := cc.bw.Flush(); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- res.Read(cc.br)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (cc *ClientConn) Close() error {
	return cc.conn.Close()
}
