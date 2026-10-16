// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/lemon4ksan/mach/server/h1"
	"github.com/lemon4ksan/mach/server/h2"
	"github.com/lemon4ksan/mach/server/h3"
)

// NewTestContextH1 exports acquire and init for H1 testing.
func (s *Server) NewTestContextH1(req *h1.Request, res *h1.Response) *Context {
	ctx := s.acquireContext()
	ctx.initH1(s, req, res)
	return ctx
}

// NewTestContextH2 exports acquire and init for H2 testing.
func (s *Server) NewTestContextH2(req *h2.ServerRequest, res *h2.ServerResponse) *Context {
	ctx := s.acquireContext()
	ctx.initH2(s, req, res)
	return ctx
}

// NewTestContextH3 exports acquire and init for H3 testing.
func (s *Server) NewTestContextH3(req *h3.ServerRequest, res *h3.ServerResponse) *Context {
	ctx := s.acquireContext()
	ctx.initH3(s, req, res)
	return ctx
}

// ReleaseTestContext exports releaseContext for testing.
func (s *Server) ReleaseTestContext(ctx *Context) {
	s.releaseContext(ctx)
}
