// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// FrameWithHeaders defines an interface for HTTP/2 frames that carry raw HPACK-encoded header fragments (RFC 9113 §4.3).
type FrameWithHeaders interface {
	Headers() []byte
}

func isForbiddenH2Header(key, value []byte) bool {
	if len(key) == 0 || key[0] == ':' {
		return true
	}

	keyStr := bytesconv.B2S(key)

	if bytesconv.EqualFoldASCII(keyStr, "te") {
		return !bytesconv.EqualFoldASCII(bytesconv.B2S(value), "trailers")
	}

	return bytesconv.EqualFoldASCII(keyStr, "connection") ||
		bytesconv.EqualFoldASCII(keyStr, "keep-alive") ||
		bytesconv.EqualFoldASCII(keyStr, "proxy-connection") ||
		bytesconv.EqualFoldASCII(keyStr, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(keyStr, "upgrade") ||
		bytesconv.EqualFoldASCII(keyStr, "host")
}

func isForbiddenH2HeaderStr(key string) bool {
	if key == "" || key[0] == ':' {
		return true
	}

	return bytesconv.EqualFoldASCII(key, "connection") ||
		bytesconv.EqualFoldASCII(key, "keep-alive") ||
		bytesconv.EqualFoldASCII(key, "proxy-connection") ||
		bytesconv.EqualFoldASCII(key, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(key, "upgrade") ||
		bytesconv.EqualFoldASCII(key, "host")
}

var defaultPseudoOrder = [4]string{":method", ":authority", ":scheme", ":path"}

func (c *Conn) encodeRequestHeaders(h *coreh2.Headers, req *h1.Request) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	enc := c.enc

	method := req.Header.Method()
	if len(method) == 0 {
		method = []byte("GET")
	}

	host := req.URI().Host()
	if idx := bytes.IndexByte(host, ':'); idx != -1 {
		if bytes.Equal(host[idx:], []byte(":443")) || bytes.Equal(host[idx:], []byte(":80")) {
			host = host[:idx]
		}
	}

	if len(host) == 0 {
		host = req.Header.Peek("Host")
	}

	scheme := req.URI().Scheme()
	if len(scheme) == 0 {
		scheme = []byte("https")
	}

	path := req.URI().RequestURI()
	if len(path) == 0 {
		path = []byte("/")
	}

	pseudoOrder := defaultPseudoOrder[:]
	if len(c.orderedKeys) > 0 {
		customPseudo := generic.Filter(c.orderedKeys, func(k string) bool {
			return len(k) > 0 && k[0] == ':'
		})

		if len(customPseudo) == 4 {
			pseudoOrder = customPseudo
		}
	}

	for _, pk := range pseudoOrder {
		switch pk {
		case ":method":
			hf.SetBytes(coreh2.StringMethod, method)
		case ":authority":
			hf.SetBytes(coreh2.StringAuthority, host)
		case ":scheme":
			hf.SetBytes(coreh2.StringScheme, scheme)
		case ":path":
			hf.SetBytes(coreh2.StringPath, path)
		}

		h.AppendHeaderField(enc, hf, true)
	}

	if len(c.orderedKeys) > 0 {
		c.appendOrderedHeaders(h, req, hf)
	} else {
		ua := req.Header.UserAgent()
		if len(ua) > 0 {
			hf.SetBytes(coreh2.StringUserAgent, ua)
			h.AppendHeaderField(enc, hf, true)
		}

		for k, v := range req.Header.All() {
			if isForbiddenH2Header(k, v) {
				continue
			}

			hf.SetBytes(coreh2.ToLowerCopy(k), v)
			h.AppendHeaderField(enc, hf, false)
		}
	}
}

func getFastHTTPCookieHeader(req *h1.Request) []byte {
	var sb strings.Builder

	for key, value := range req.Header.Cookies() {
		if sb.Len() > 0 {
			sb.WriteString("; ")
		}

		sb.Write(key)
		sb.WriteByte('=')
		sb.Write(value)
	}

	if sb.Len() > 0 {
		return bytesconv.S2B(sb.String())
	}

	if raw := req.Header.Peek("Cookie"); len(raw) > 0 {
		return raw
	}

	if raw := req.Header.Peek("cookie"); len(raw) > 0 {
		return raw
	}

	return nil
}

func peekHeaderCaseInsensitive(req *h1.Request, key string) []byte {
	for k, v := range req.Header.All() {
		if bytesconv.EqualFoldASCII(bytesconv.B2S(k), key) {
			return v
		}
	}

	return nil
}

func (c *Conn) appendOrderedHeaders(h *coreh2.Headers, req *h1.Request, hf *hpack.HeaderField) {
	var visitedBits uint64

	numOrdered := min(len(c.orderedKeys), 64)

	for i := range numOrdered {
		key := c.orderedKeys[i]
		if isForbiddenH2HeaderStr(key) {
			continue
		}

		var val []byte
		if bytesconv.EqualFoldASCII(key, "cookie") {
			val = getFastHTTPCookieHeader(req)
		} else {
			val = req.Header.Peek(key)
			if len(val) == 0 && len(key) > 0 {
				val = peekHeaderCaseInsensitive(req, key)
			}
		}

		if len(val) > 0 {
			hf.SetKey(key)
			hf.SetValueBytes(val)
			h.AppendHeaderField(c.enc, hf, false)

			visitedBits |= (1 << i)
		}
	}

	for k, v := range req.Header.All() {
		if isForbiddenH2Header(k, v) {
			continue
		}

		kStr := bytesconv.B2S(k)
		skip := false

		for i := range numOrdered {
			if (visitedBits&(1<<i)) != 0 && bytesconv.EqualFoldASCII(kStr, c.orderedKeys[i]) {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		hf.SetKeyBytes(bytesconv.AppendToLower(nil, k))
		hf.SetValueBytes(v)
		h.AppendHeaderField(c.enc, hf, false)
	}
}

const defaultMaxHeaderListSize = 10 * 1024 * 1024

func (c *Conn) readTrailers(b []byte, reqCtx *Context) error {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var (
		totalSize uint32
		maxList   = c.current.MaxHeaderListSize()
	)

	if maxList == 0 {
		maxList = defaultMaxHeaderListSize
	}

	if reqCtx.Trailers == nil {
		reqCtx.Trailers = make(map[string][]string)
	}

	for len(b) > 0 {
		var err error

		b, err = c.dec.Next(hf, b)
		if err != nil {
			return err
		}

		totalSize += hf.Size()
		if totalSize > maxList {
			return coreh2.ErrPayloadExceeds
		}

		if !hf.IsPseudo() && len(hf.KeyBytes()) > 0 {
			key := hf.Key()
			reqCtx.Trailers[key] = append(reqCtx.Trailers[key], hf.Value())
		}
	}

	return nil
}

func (c *Conn) readHeader(b []byte, res *h1.Response) (int, error) {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	var (
		err        error
		statusCode int
		totalSize  uint32
		maxList    = c.current.MaxHeaderListSize()
	)

	if maxList == 0 {
		maxList = defaultMaxHeaderListSize
	}

	informationalHeader := make(http.Header)

	for len(b) > 0 {
		b, err = c.dec.Next(hf, b)
		if err != nil {
			return 0, err
		}

		totalSize += hf.Size()
		if totalSize > maxList {
			return 0, coreh2.ErrPayloadExceeds
		}

		if hf.IsPseudo() && len(hf.KeyBytes()) > 1 && hf.KeyBytes()[1] == 's' {
			n, pErr := strconv.ParseInt(hf.Value(), 10, 64)
			if pErr != nil {
				return 0, pErr
			}

			statusCode = int(n)
			if statusCode < 100 || statusCode >= 200 || statusCode == 101 {
				res.SetStatusCode(statusCode)
			}

			continue
		}

		if statusCode >= 100 && statusCode < 200 && statusCode != 101 {
			informationalHeader.Add(hf.Key(), hf.Value())
			continue
		}

		if bytes.Equal(hf.KeyBytes(), coreh2.StringContentLength) {
			n, _ := strconv.Atoi(hf.Value())
			res.Header.SetContentLength(n)
		} else {
			res.Header.AddBytesKV(hf.KeyBytes(), hf.ValueBytes())
		}
	}

	if statusCode == 103 {
		trace := httptrace.ContextClientTrace(context.Background())
		if trace != nil && trace.Got1xxResponse != nil {
			_ = trace.Got1xxResponse(statusCode, textproto.MIMEHeader(informationalHeader))
		}
	}

	return statusCode, nil
}
