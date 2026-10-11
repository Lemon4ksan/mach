// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"sync/atomic"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

var (
	requestBodyPoolSizeLimit  atomic.Int64
	responseBodyPoolSizeLimit atomic.Int64

	requestStorage = pool.NewPerCorePool[*Request](func() *Request {
		return &Request{}
	})
	responseStorage = pool.NewPerCorePool[*Response](func() *Response {
		return &Response{}
	})

	requestBodyPool  = newByteBufferPerCorePool()
	responseBodyPool = newByteBufferPerCorePool()
)

type byteBufferPerCorePool struct {
	p *pool.PerCorePool[*bytesconv.ByteBuffer]
}

func newByteBufferPerCorePool() *byteBufferPerCorePool {
	return &byteBufferPerCorePool{
		p: pool.NewPerCorePool[*bytesconv.ByteBuffer](func() *bytesconv.ByteBuffer {
			return &bytesconv.ByteBuffer{}
		}),
	}
}

func (p *byteBufferPerCorePool) Get() *bytesconv.ByteBuffer {
	b := p.p.Get()
	b.Reset()
	return b
}

func (p *byteBufferPerCorePool) Put(b *bytesconv.ByteBuffer) {
	if b == nil {
		return
	}
	b.Reset()
	p.p.Put(b)
}

// SetBodySizePoolLimit sets the maximum byte size for request and response body buffers
// to be recycled in internal ByteBuffer pools.
//
// Buffers exceeding these limits are released to the garbage collector upon disposal
// rather than being retained in the pool, mitigating heap bloat from sporadic large payloads.
// Passing negative values disables the size cap (unlimited reuse).
//
// Concurrency: Thread-safe; uses atomic store operations.
func SetBodySizePoolLimit(reqBodyLimit, respBodyLimit int) {
	requestBodyPoolSizeLimit.Store(int64(reqBodyLimit))
	responseBodyPoolSizeLimit.Store(int64(respBodyLimit))
}

// AcquireRequest returns an empty Request instance from request pool.
//
// The returned Request instance may be passed to ReleaseRequest when it is
// no longer needed. This allows Request recycling, reduces GC pressure
// and usually improves performance.
func AcquireRequest() *Request {
	return requestStorage.Get()
}

// ReleaseRequest returns req acquired via AcquireRequest to request pool.
//
// It is forbidden accessing req and/or its' members after returning
// it to request pool.
func ReleaseRequest(req *Request) {
	if req == nil {
		return
	}
	req.Reset()
	requestStorage.Put(req)
}

// AcquireResponse returns an empty Response instance from response pool.
//
// The returned Response instance may be passed to ReleaseResponse when it is
// no longer needed. This allows Response recycling, reduces GC pressure
// and usually improves performance.
func AcquireResponse() *Response {
	return responseStorage.Get()
}

// ReleaseResponse returns resp acquired via AcquireResponse to response pool.
//
// It is forbidden accessing resp and/or its' members after returning
// it to response pool.
func ReleaseResponse(resp *Response) {
	if resp == nil {
		return
	}
	resp.Reset()
	responseStorage.Put(resp)
}

func init() {
	requestBodyPoolSizeLimit.Store(-1)
	responseBodyPoolSizeLimit.Store(-1)
}
