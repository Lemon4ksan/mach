// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
)

// NoCopy may be embedded into structs which must not be copied after the first use.
// The zero value is ready to use.
type NoCopy struct{}

// Lock is a no-op method used by the -copylocks checker from go vet.
func (*NoCopy) Lock()   {}

// Unlock is a no-op method used by the -copylocks checker from go vet.
func (*NoCopy) Unlock() {}

// CopyZeroAlloc copies data from r to w by invoking [io.ReaderFrom] or [io.WriterTo] when available.
func CopyZeroAlloc(w io.Writer, r io.Reader) (int64, error) {
	var readerIsFile, readerIsConn bool

	switch r := r.(type) {
	case *os.File:
		readerIsFile = true
	case *net.TCPConn:
		readerIsConn = true
	case io.WriterTo:
		return r.WriteTo(w)
	}

	switch w := w.(type) {
	case *os.File:
		if readerIsConn {
			return w.ReadFrom(r)
		}
	case *net.TCPConn:
		if readerIsFile {
			if rt, ok := r.(io.WriterTo); ok {
				return rt.WriteTo(w)
			}

			return w.ReadFrom(r)
		}

	case io.ReaderFrom:
		return w.ReadFrom(r)
	}

	vbuf := CopyBufPool.Get()
	buf := vbuf.([]byte)
	n, err := CopyBuffer(w, r, buf)

	CopyBufPool.Put(vbuf)

	return n, err
}

// CopyBuffer copies from src to dst using the provided buffer slice buf.
func CopyBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0

				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}

			written += int64(nw)

			if ew != nil {
				err = ew
				break
			}

			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}

		if er != nil {
			if er != io.EOF {
				err = er
			}

			break
		}
	}

	return written, err
}

// CopyBufPool provides recycled 4096-byte buffers for streaming copy operations.
var CopyBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 4096)
	},
}
