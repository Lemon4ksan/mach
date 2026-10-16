// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/codec/qpack"
	"github.com/lemon4ksan/mach/proto/headkit"
)

type requestHeaderHandler struct {
	reqHeaders           *headkit.Headers
	method               string
	path                 string
	scheme               string
	authority            string
	hasSeenRegularHeader bool
	malformed            bool
	err                  error
}

func (h *requestHeaderHandler) OnHeaderDecoded(k, v string) {
	if h.malformed || h.err != nil {
		return
	}

	for i := range len(k) {
		c := k[i]
		if c <= 0x20 || c >= 0x7f || (c >= 'A' && c <= 'Z') {
			h.malformed = true
			return
		}
	}

	for i := range len(v) {
		if v[i] == 0 {
			h.malformed = true
			return
		}
	}

	if len(k) > 0 && k[0] == ':' {
		if h.hasSeenRegularHeader {
			h.malformed = true
			return
		}

		switch k {
		case ":method":
			if h.method != "" {
				h.malformed = true
				return
			}

			h.method = v

		case ":path":
			if h.path != "" {
				h.malformed = true
				return
			}

			h.path = v

		case ":scheme":
			if h.scheme != "" {
				h.malformed = true
				return
			}

			h.scheme = v

		case ":authority":
			if h.authority != "" {
				h.malformed = true
				return
			}

			h.authority = v

		case ":protocol":
			h.reqHeaders.Set(":protocol", v)
		default:
			h.malformed = true
			return
		}
	} else {
		h.hasSeenRegularHeader = true
		switch k {
		case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
			h.malformed = true
			return
		case "te":
			if !bytesconv.EqualFoldASCII(v, "trailers") {
				h.malformed = true
				return
			}
		case "host":
			if h.authority != "" && v != h.authority {
				h.malformed = true
				return
			}
		}

		h.reqHeaders.Add(k, v)
	}
}

func (h *requestHeaderHandler) OnDecodingCompleted() {}
func (h *requestHeaderHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.err = ErrQPACKDecompressFailed
}

func statusCodeToString(code int) string {
	switch code {
	case 200:
		return "200"
	case 204:
		return "204"
	case 206:
		return "206"
	case 301:
		return "301"
	case 302:
		return "302"
	case 304:
		return "304"
	case 400:
		return "400"
	case 401:
		return "401"
	case 403:
		return "403"
	case 404:
		return "404"
	case 405:
		return "405"
	case 500:
		return "500"
	case 502:
		return "502"
	case 503:
		return "503"
	default:
		return strconv.Itoa(code)
	}
}

func contentLengthToString(l int) string {
	switch l {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	case 4:
		return "4"
	case 8:
		return "8"
	case 16:
		return "16"
	case 32:
		return "32"
	case 64:
		return "64"
	case 128:
		return "128"
	case 256:
		return "256"
	case 512:
		return "512"
	case 1024:
		return "1024"
	default:
		return strconv.Itoa(l)
	}
}

func toLowerASCII(s string) string {
	hasUpper := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
			break
		}
	}

	if !hasUpper {
		return s
	}

	return strings.ToLower(s)
}

// DecodeRequestHeaders decodes a QPACK-encoded request header block from a client stream (RFC 9204 §4.5, RFC 9114 §4.1.2).
//
// It validates mandatory pseudo-headers (:method, :scheme, :authority, :path) according to HTTP/3
// semantics, handles extended CONNECT tunnels (RFC 9114 §4.4), and populates reqHeaders with regular
// headers while discarding or rejecting prohibited hop-by-hop headers.
func (q *QPACKCodec) DecodeRequestHeaders(
	streamID uint64,
	headerBlock []byte,
	reqHeaders *headkit.Headers,
) (method, path, scheme, authority string, retErr error) {
	if err := q.Err(); err != nil {
		return "", "", "", "", err
	}

	q.decMu.Lock()
	defer q.decMu.Unlock()

	if err := q.Err(); err != nil {
		return "", "", "", "", err
	}

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("h3: panic recovered in DecodeRequestHeaders: %v", r)
			q.recordError(retErr)
		}
	}()

	var handler requestHeaderHandler

	handler.reqHeaders = reqHeaders
	decoder := q.decoder.CreateProgressiveDecoder(streamID, &handler)
	decoder.Decode(headerBlock)

	if handler.err != nil {
		return "", "", "", "", handler.err
	}

	if handler.malformed {
		return "", "", "", "", ErrMalformedHeader
	}

	if handler.method == "" {
		return "", "", "", "", ErrMissingMethodOrPath
	}

	if handler.method == "CONNECT" {
		isExtended := reqHeaders.Get(":protocol") != ""
		if isExtended {
			if handler.scheme == "" || handler.path == "" || handler.authority == "" {
				return "", "", "", "", ErrMissingMethodOrPath
			}
		} else {
			if handler.scheme != "" || handler.path != "" || handler.authority == "" {
				return "", "", "", "", ErrMalformedHeader
			}
		}
	} else if handler.path == "" || handler.scheme == "" {
		return "", "", "", "", ErrMissingMethodOrPath
	}

	return handler.method, handler.path, handler.scheme, handler.authority, nil
}

// EncodeResponseHeaders encodes server response headers into a QPACK-encoded field section (RFC 9204 §4.5, RFC 9114 §4.3).
//
// It serializes the :status pseudo-header, optional content-length header if bodyLen >= 0,
// and all supplied response headers, filtering out prohibited HTTP/3 connection-specific headers (RFC 9114 §4.1).
func (q *QPACKCodec) EncodeResponseHeaders(
	streamID uint64,
	statusCode int,
	headers headkit.Headers,
	bodyLen int,
) (block []byte) {
	q.encMu.Lock()
	defer q.encMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("h3: panic recovered in EncodeResponseHeaders: %v", r)
			q.recordError(err)
		}
	}()

	var initialList [16]qpack.HeaderField

	list := initialList[:0]

	list = append(list, qpack.HeaderField{Name: ":status", Value: statusCodeToString(statusCode)})
	if bodyLen >= 0 {
		list = append(list, qpack.HeaderField{Name: "content-length", Value: contentLengthToString(bodyLen)})
	}

	for k, v := range headers.All() {
		kLower := toLowerASCII(k)
		if isForbiddenH3HeaderString(kLower, v) {
			continue
		}

		list = append(list, qpack.HeaderField{Name: kLower, Value: v})
	}

	return q.encoder.EncodeHeaderList(streamID, list, nil)
}
