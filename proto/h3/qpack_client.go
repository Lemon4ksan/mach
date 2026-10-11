// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"fmt"
	"io"
	"strconv"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/qpack"
)

// EncodeRequestHeaders encodes request headers into a QPACK-encoded field section (RFC 9204 §4.5, RFC 9114 §4.3.1).
//
// It serializes mandatory pseudo-headers (:method, :scheme, :authority, :path, and optional :protocol for extended CONNECT),
// content-type, content-length, and custom headers in orderedKeys or natural iteration, omitting prohibited HTTP/3 headers.
//
// Complexity:
//   - Time: O(H + B) where H is the total header count and B is the aggregate byte size of field names and values.
//     Each header undergoes O(1) prohibited field verification (RFC 9114 §4.1). When orderedKeys is provided,
//     getOrderedHeaders runs in O(K * M + H) where K <= 64 via a bitmask-accelerated deduplication pass.
//   - Space: O(B) buffer allocation for the generated QPACK instruction block.
func (q *QPACKCodec) EncodeRequestHeaders(
	streamID uint64,
	w io.Writer,
	req *http.Request,
	orderedKeys []string,
) (retErr error) {
	if err := q.Err(); err != nil {
		return err
	}

	var block []byte

	encErr := func() (err error) {
		q.encMu.Lock()
		defer q.encMu.Unlock()

		if e := q.Err(); e != nil {
			return e
		}

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("h3: panic recovered in EncodeRequestHeaders: %v", r)
				q.recordError(err)
			}
		}()

		var headers []qpack.HeaderField

		method := bytesconv.B2S(req.Header.Method())
		headers = append(headers, qpack.HeaderField{Name: ":method", Value: method})
		headers = append(headers, qpack.HeaderField{Name: ":scheme", Value: bytesconv.B2S(req.URI().Scheme())})
		headers = append(headers, qpack.HeaderField{Name: ":authority", Value: bytesconv.B2S(req.URI().Host())})
		headers = append(headers, qpack.HeaderField{Name: ":path", Value: bytesconv.B2S(req.URI().RequestURI())})

		if protoVal := req.Header.Peek(":protocol"); len(protoVal) > 0 {
			headers = append(headers, qpack.HeaderField{Name: ":protocol", Value: bytesconv.B2S(protoVal)})
		}

		if len(orderedKeys) > 0 {
			headers = append(headers, q.getOrderedHeaders(req, orderedKeys)...)
		} else {
			if ct := req.Header.ContentType(); len(ct) > 0 {
				headers = append(headers, qpack.HeaderField{Name: "content-type", Value: bytesconv.B2S(ct)})
			}

			if cl := req.Header.ContentLength(); cl >= 0 {
				headers = append(headers, qpack.HeaderField{Name: "content-length", Value: strconv.Itoa(cl)})
			}

			var stackKeyBuf [128]byte
			for k, v := range req.Header.All() {
				if isForbiddenH3Header(k, v) {
					continue
				}

				var keyStr string
				if len(k) <= len(stackKeyBuf) {
					keyBuf := stackKeyBuf[:len(k)]
					for i := range k {
						keyBuf[i] = bytesconv.LowercaseByte(k[i])
					}

					keyStr = string(keyBuf)
				} else {
					keyStr = string(bytesconv.AppendToLower(nil, k))
				}

				headers = append(headers, qpack.HeaderField{Name: keyStr, Value: bytesconv.B2S(v)})
			}
		}

		block = q.encoder.EncodeHeaderList(streamID, headers, nil)

		return nil
	}()
	if encErr != nil {
		return encErr
	}

	_, writeErr := w.Write(block)

	return writeErr
}

func (q *QPACKCodec) getOrderedHeaders(req *http.Request, orderedKeys []string) []qpack.HeaderField {
	var (
		headers     []qpack.HeaderField
		visitedBits uint64
	)

	numOrdered := min(len(orderedKeys), 64)
	keys := orderedKeys[:numOrdered]

	for i := range numOrdered {
		key := keys[i]
		val := req.Header.Peek(key)

		if isForbiddenH3HeaderStr(key, val) {
			continue
		}

		if len(val) > 0 {
			headers = append(headers, qpack.HeaderField{Name: key, Value: bytesconv.B2S(val)})
			visitedBits |= (1 << i)
		}
	}

	for k, v := range req.Header.All() {
		kStr := bytesconv.B2S(k)
		if isForbiddenH3Header(k, v) {
			continue
		}

		skip := false
		for i := range numOrdered {
			if (visitedBits&(1<<i)) != 0 && bytesconv.EqualFoldASCII(kStr, keys[i]) {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		var (
			stackKeyBuf [128]byte
			keyStr      string
		)

		if len(k) <= len(stackKeyBuf) {
			keyBuf := stackKeyBuf[:len(k)]
			for i := range k {
				keyBuf[i] = bytesconv.LowercaseByte(k[i])
			}

			keyStr = string(keyBuf)
		} else {
			keyStr = string(bytesconv.AppendToLower(nil, k))
		}

		headers = append(headers, qpack.HeaderField{
			Name:  keyStr,
			Value: bytesconv.B2S(v),
		})
	}

	return headers
}

type responseHeaderHandler struct {
	res                  *http.ResponseHeader
	hasStatus            bool
	hasSeenRegularHeader bool
	statusCode           int
	parseErr             error
}

func (h *responseHeaderHandler) OnHeaderDecoded(name, value string) {
	if h.parseErr != nil {
		return
	}

	// RFC 9114 §4.2: Characters in field names MUST be converted to lowercase prior to encoding.
	// A request or response containing uppercase characters in field names MUST be treated as malformed.
	for i := range len(name) {
		c := name[i]
		if c <= 0x20 || c >= 0x7f || (c >= 'A' && c <= 'Z') {
			h.parseErr = ErrMalformedHeader
			return
		}
	}

	// RFC 9114 §4.1.2 & §10.3: Field values must not contain NUL (0x00).
	for i := range len(value) {
		if value[i] == 0 {
			h.parseErr = ErrMalformedHeader
			return
		}
	}

	if len(name) > 0 && name[0] == ':' {
		// RFC 9114 §4.3: All pseudo-header fields MUST appear in the header section before regular header fields.
		if h.hasSeenRegularHeader {
			h.parseErr = ErrMalformedHeader
			return
		}

		// RFC 9114 §4.3.2: For responses, a single ":status" pseudo-header field is defined.
		// Undefined or invalid pseudo-header fields MUST be treated as malformed (RFC 9114 §4.3).
		if name != ":status" {
			h.parseErr = ErrMalformedHeader
			return
		}

		if h.hasStatus {
			h.parseErr = ErrMalformedHeader
			return
		}

		code, err := strconv.Atoi(value)
		if err != nil {
			h.parseErr = ErrMalformedHeader
			return
		}

		h.statusCode = code
		// RFC 9114 §4.3.1: 101 Switching Protocols is not supported in HTTP/3.
		if h.statusCode == 101 {
			h.parseErr = ErrMalformedHeader
			return
		}

		if h.statusCode < 100 || h.statusCode >= 200 {
			h.res.SetStatusCode(h.statusCode)
		}

		h.hasStatus = true

		return
	}

	h.hasSeenRegularHeader = true

	// RFC 9114 §4.2: Connection-specific fields MUST NOT be present in HTTP/3 field sections;
	// any message containing connection-specific fields MUST be treated as malformed.
	switch name {
	case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade", "te":
		h.parseErr = ErrMalformedHeader
		return
	}

	if name == "content-length" {
		if clen, err := strconv.Atoi(value); err == nil {
			h.res.SetContentLength(clen)
		}

		return
	}

	h.res.AddBytesKV(bytesconv.S2B(name), bytesconv.S2B(value))
}

func (h *responseHeaderHandler) OnDecodingCompleted() {}
func (h *responseHeaderHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.parseErr = ErrQPACKDecompressFailed
}

// DecodeResponseHeaders decodes a QPACK-encoded response header block into res (RFC 9204 §4.5, RFC 9114 §4.2 & §4.3).
//
// It decodes the mandatory :status pseudo-header and populates response header fields,
// returning the parsed HTTP status code. If :status is missing or malformed (such as status 101,
// prohibited in HTTP/3 by RFC 9114 §4.3.1, or prohibited connection-specific fields per RFC 9114 §4.2),
// ErrMissingStatusHeader or ErrMalformedHeader is returned.
//
// Complexity:
//   - Time: O(B) progressive parsing across the header block of B bytes (RFC 9204 §4.5).
//   - Space: O(H) where H is the count of decoded response header entries stored in res.
func (q *QPACKCodec) DecodeResponseHeaders(
	streamID uint64,
	headerBlock []byte,
	res *http.ResponseHeader,
) (statusCode int, retErr error) {
	if err := q.Err(); err != nil {
		return 0, err
	}

	q.decMu.Lock()
	defer q.decMu.Unlock()

	if err := q.Err(); err != nil {
		return 0, err
	}

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("h3: panic recovered in DecodeResponseHeaders: %v", r)
			q.recordError(retErr)
		}
	}()

	handler := &responseHeaderHandler{res: res}
	decoder := q.decoder.CreateProgressiveDecoder(streamID, handler)
	decoder.Decode(headerBlock)

	if handler.parseErr != nil {
		return 0, handler.parseErr
	}

	if !handler.hasStatus {
		return 0, ErrMissingStatusHeader
	}

	return handler.statusCode, nil
}

type trailersHandler struct {
	trailers map[string][]string
	err      error
}

func (h *trailersHandler) OnHeaderDecoded(name, value string) {
	if h.err != nil {
		return
	}

	// RFC 9114 §4.3: Pseudo-header fields MUST NOT appear in trailer sections.
	if len(name) > 0 && name[0] == ':' {
		h.err = ErrMalformedHeader
		return
	}

	// RFC 9114 §4.2: Lowercase validation.
	for i := range len(name) {
		c := name[i]
		if c <= 0x20 || c >= 0x7f || (c >= 'A' && c <= 'Z') {
			h.err = ErrMalformedHeader
			return
		}
	}

	// RFC 9114 §4.2: Connection-specific fields forbidden in trailers.
	switch name {
	case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade", "te":
		h.err = ErrMalformedHeader
		return
	}

	h.trailers[name] = append(h.trailers[name], value)
}

func (h *trailersHandler) OnDecodingCompleted() {}
func (h *trailersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.err = ErrQPACKDecompressFailed
}

// DecodeResponseTrailers decodes a QPACK-encoded trailing field section (RFC 9204 §4.5, RFC 9114 §4.3.4).
//
// Trailing pseudo-headers (beginning with ':') are discarded per RFC 9114 §4.3.4.
//
// Complexity:
//   - Time: O(B) progressive parsing over the B trailer block bytes.
//   - Space: O(T) allocations for T decoded trailer entries stored in the map.
func (q *QPACKCodec) DecodeResponseTrailers(
	streamID uint64,
	headerBlock []byte,
) (trailers map[string][]string, retErr error) {
	if err := q.Err(); err != nil {
		return nil, err
	}

	q.decMu.Lock()
	defer q.decMu.Unlock()

	if err := q.Err(); err != nil {
		return nil, err
	}

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("h3: panic recovered in DecodeResponseTrailers: %v", r)
			q.recordError(retErr)
		}
	}()

	handler := &trailersHandler{trailers: make(map[string][]string)}
	decoder := q.decoder.CreateProgressiveDecoder(streamID, handler)
	decoder.Decode(headerBlock)

	if handler.err != nil {
		return nil, handler.err
	}

	return handler.trailers, nil
}
