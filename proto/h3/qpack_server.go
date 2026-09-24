// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/qpack"
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

	for i := 0; i < len(k); i++ {
		c := k[i]
		if c <= 0x20 || c >= 0x7f || (c >= 'A' && c <= 'Z') {
			h.malformed = true
			return
		}
	}

	for i := 0; i < len(v); i++ {
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

// DecodeRequestHeaders decodes a QPACK-encoded request header block from a client stream (RFC 9204 §4.5, RFC 9114 §4.1.2).
//
// It validates mandatory pseudo-headers (:method, :scheme, :authority, :path) according to HTTP/3
// semantics, handles extended CONNECT tunnels (RFC 9114 §4.4), and populates reqHeaders with regular
// headers while discarding or rejecting prohibited hop-by-hop headers.
// Concurrency: Thread-safe; protected by the internal decoder mutex (decMu).
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

	handler := &requestHeaderHandler{reqHeaders: reqHeaders}
	decoder := q.decoder.CreateProgressiveDecoder(streamID, handler)
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
// Concurrency: Thread-safe; protected by the internal encoder mutex (encMu).
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

	var list []qpack.HeaderField

	list = append(list, qpack.HeaderField{Name: ":status", Value: strconv.Itoa(statusCode)})
	if bodyLen >= 0 {
		list = append(list, qpack.HeaderField{Name: "content-length", Value: strconv.Itoa(bodyLen)})
	}

	for k, v := range headers.All() {
		kLower := strings.ToLower(k)
		if isForbiddenH3Header([]byte(kLower), []byte(v)) {
			continue
		}

		list = append(list, qpack.HeaderField{Name: kLower, Value: v})
	}

	return q.encoder.EncodeHeaderList(streamID, list, nil)
}
