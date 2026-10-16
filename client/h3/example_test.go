// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	"context"
	"time"

	"github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/quic"
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
