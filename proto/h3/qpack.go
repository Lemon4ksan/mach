// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (

	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	coreheaders "github.com/lemon4ksan/foundation/net/headkit"
	h1 "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/foundation/net/qpack"
)

// PooledEncoder encapsulates a pooled buffer and QPACK encoder for zero-allocation serialization.
type PooledEncoder struct {
	buf *bytes.Buffer
	enc *qpack.Encoder
}

var encoderPool = generic.NewPool(func() *PooledEncoder {
	buf := new(bytes.Buffer)

	return &PooledEncoder{
		buf: buf,
		enc: qpack.NewEncoder(buf),
	}
})

// QPACKCodec manages zero-allocation QPACK header serialization and deserialization (RFC 9204 Â§2, Â§3 & Â§4).
type QPACKCodec struct {
	decoder *qpack.Decoder
}

// NewQPACKCodec instantiates a new QPACKCodec (RFC 9204 Â§2.2).
func NewQPACKCodec() *QPACKCodec {
	return &QPACKCodec{
		decoder: qpack.NewDecoder(),
	}
}

// AcquireEncoder obtains a pooled QPACK encoder for zero-allocation encoding.
func (q *QPACKCodec) AcquireEncoder() *PooledEncoder {
	p := encoderPool.Get()
	p.buf.Reset()
	p.enc.Reset(p.buf)

	return p
}

// ReleaseEncoder returns a pooled QPACK encoder back to the memory pool.
func (q *QPACKCodec) ReleaseEncoder(p *PooledEncoder) {
	if p != nil {
		encoderPool.Put(p)
	}
}

// WriteDecoderTable processes instructions received over the QPACK Encoder Stream (RFC 9204 Â§4.2 & Â§4.3).
//
// Note: Currently quic-go/qpack operates on static tables (RFC 9204 Appendix A) and does not expose
// dynamic table instructions, so incoming bytes are safely consumed.
func (q *QPACKCodec) WriteDecoderTable(_ []byte) error {
	return nil
}

// EncodeRequestHeadersPooled encodes request headers into the pooled encoder's buffer with 0 heap allocations.
func (q *QPACKCodec) EncodeRequestHeadersPooled(
	p *PooledEncoder,
	req *h1.Request,
	orderedKeys []string,
) ([]byte, error) {
	enc := p.enc

	method := bytesconv.B2S(req.Header.Method())
	_ = enc.WriteField(qpack.HeaderField{Name: ":method", Value: method})
	_ = enc.WriteField(qpack.HeaderField{Name: ":scheme", Value: bytesconv.B2S(req.URI().Scheme())})
	_ = enc.WriteField(qpack.HeaderField{Name: ":authority", Value: bytesconv.B2S(req.URI().Host())})
	_ = enc.WriteField(qpack.HeaderField{Name: ":path", Value: bytesconv.B2S(req.URI().RequestURI())})

	if protoVal := req.Header.Peek(":protocol"); len(protoVal) > 0 {
		_ = enc.WriteField(qpack.HeaderField{Name: ":protocol", Value: bytesconv.B2S(protoVal)})
	}

	if len(orderedKeys) > 0 {
		q.encodeOrderedHeaders(enc, req, orderedKeys)
	} else {
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

				keyStr = bytesconv.B2S(keyBuf)
			} else {
				keyStr = bytesconv.B2S(bytesconv.AppendToLower(nil, k))
			}

			_ = enc.WriteField(qpack.HeaderField{Name: keyStr, Value: bytesconv.B2S(v)})
		}
	}

	return p.buf.Bytes(), nil
}

// EncodeRequestHeaders encodes a fasthttp request header into a QPACK block (RFC 9204 Â§4.5),
// strictly maintaining the specified orderedKeys sequence for RFC 9220 Extended CONNECT.
func (q *QPACKCodec) EncodeRequestHeaders(w io.Writer, req *h1.Request, orderedKeys []string) error {
	p := q.AcquireEncoder()
	defer q.ReleaseEncoder(p)

	block, err := q.EncodeRequestHeadersPooled(p, req, orderedKeys)
	if err != nil {
		return err
	}

	_, err = w.Write(block)

	return err
}

// isForbiddenH3Header checks if a header field is prohibited in HTTP/3 (RFC 9114 Â§4.1, Â§4.3 & Â§4.5).
// Transfer-Encoding, Upgrade, Connection, and hop-by-hop headers MUST NOT be sent.
// TE header is only permitted if its value is "trailers".
func isForbiddenH3Header(key, val []byte) bool {
	if len(key) == 0 || key[0] == ':' {
		return true
	}

	keyStr := bytesconv.B2S(key)
	if bytesconv.EqualFoldASCII(keyStr, "connection") ||
		bytesconv.EqualFoldASCII(keyStr, "keep-alive") ||
		bytesconv.EqualFoldASCII(keyStr, "proxy-connection") ||
		bytesconv.EqualFoldASCII(keyStr, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(keyStr, "upgrade") ||
		bytesconv.EqualFoldASCII(keyStr, "sec-websocket-key") ||
		bytesconv.EqualFoldASCII(keyStr, "sec-websocket-accept") {
		return true
	}

	if bytesconv.EqualFoldASCII(keyStr, "te") {
		return !bytesconv.EqualFoldASCII(bytesconv.B2S(val), "trailers")
	}

	return false
}

func isForbiddenH3HeaderStr(key string, val []byte) bool {
	if key == "" || key[0] == ':' {
		return true
	}

	if bytesconv.EqualFoldASCII(key, "connection") ||
		bytesconv.EqualFoldASCII(key, "keep-alive") ||
		bytesconv.EqualFoldASCII(key, "proxy-connection") ||
		bytesconv.EqualFoldASCII(key, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(key, "upgrade") ||
		bytesconv.EqualFoldASCII(key, "sec-websocket-key") ||
		bytesconv.EqualFoldASCII(key, "sec-websocket-accept") {
		return true
	}

	if bytesconv.EqualFoldASCII(key, "te") {
		return !bytesconv.EqualFoldASCII(bytesconv.B2S(val), "trailers")
	}

	return false
}

func (q *QPACKCodec) encodeOrderedHeaders(enc *qpack.Encoder, req *h1.Request, orderedKeys []string) {
	var visitedBits uint64

	numOrdered := min(len(orderedKeys), 64)
	keys := orderedKeys[:numOrdered]

	for i := range numOrdered {
		key := keys[i]
		val := req.Header.Peek(key)

		if isForbiddenH3HeaderStr(key, val) {
			continue
		}

		if len(val) > 0 {
			_ = enc.WriteField(qpack.HeaderField{Name: key, Value: bytesconv.B2S(val)})

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

			_ = keyBuf[len(k)-1]
			for i := range k {
				keyBuf[i] = bytesconv.LowercaseByte(k[i])
			}

			keyStr = bytesconv.B2S(keyBuf)
		} else {
			keyStr = bytesconv.B2S(bytesconv.AppendToLower(nil, k))
		}

		_ = enc.WriteField(qpack.HeaderField{
			Name:  keyStr,
			Value: bytesconv.B2S(v),
		})
	}
}

// DecodeResponseHeaders parses a QPACK header block directly into fasthttp ResponseHeader (RFC 9204 Â§2.2 & Â§4.5),
// returning the parsed status code and ignoring 1xx informational frames (RFC 9114 Â§4.1).
func (q *QPACKCodec) DecodeResponseHeaders(headerBlock []byte, res *h1.ResponseHeader) (int, error) {
	var (
		hasStatus  bool
		statusCode int
		parseErr   error
	)

	err := q.decoder.DecodeFields(headerBlock, nil, func(hf qpack.HeaderField) bool {
		if hf.Name == ":status" {
			code, err := strconv.Atoi(hf.Value)
			if err != nil {
				parseErr = err
				return false
			}

			statusCode = code
			if statusCode < 100 || statusCode >= 200 || statusCode == 101 {
				res.SetStatusCode(statusCode)
			}

			hasStatus = true

			return true
		}

		if hf.IsPseudo() {
			return true
		}

		if hf.Name == "content-length" {
			if clen, err := strconv.Atoi(hf.Value); err == nil {
				res.SetContentLength(clen)
			}

			return true
		}

		res.AddBytesKV(bytesconv.S2B(hf.Name), bytesconv.S2B(hf.Value))

		return true
	})
	if err != nil {
		return 0, ErrQPACKDecompressFailed
	}

	if parseErr != nil {
		return 0, parseErr
	}

	if !hasStatus {
		return 0, ErrMissingStatusHeader
	}

	return statusCode, nil
}

// DecodeResponseTrailers decodes a QPACK header block containing response trailers into a key-value map (RFC 9204 Â§2.2 & Â§4.5).
func (q *QPACKCodec) DecodeResponseTrailers(headerBlock []byte) (map[string][]string, error) {
	trailers := make(map[string][]string)

	err := q.decoder.DecodeFields(headerBlock, nil, func(hf qpack.HeaderField) bool {
		if hf.IsPseudo() {
			return true
		}

		trailers[hf.Name] = append(trailers[hf.Name], hf.Value)

		return true
	})
	if err != nil {
		return nil, ErrQPACKDecompressFailed
	}

	return trailers, nil
}

func (q *QPACKCodec) DecodeRequestHeaders(
	headerBlock []byte,
	reqHeaders *coreheaders.Headers,
) (method, path, scheme, authority string, err error) {
	reqHeaders.Reset()

	var (
		hasSeenRegularHeader bool
		malformed            bool
	)

	arena := reqHeaders.GetRawBuf()

	decodeErr := q.decoder.DecodeFields(headerBlock, &arena, func(hf qpack.HeaderField) bool {
		k := hf.Name
		v := hf.Value

		// RFC 9114 §4.1.2: All field names MUST be lowercase ASCII
		for i := 0; i < len(k); i++ {
			if k[i] >= 'A' && k[i] <= 'Z' {
				malformed = true
				return false
			}
		}

		if hf.IsPseudo() {
			// RFC 9114 §4.3: Pseudo-headers MUST appear before regular headers
			if hasSeenRegularHeader {
				malformed = true
				return false
			}

			switch k {
			case ":method":
				if method != "" {
					malformed = true
					return false
				}

				method = v

			case ":path":
				if path != "" {
					malformed = true
					return false
				}

				path = v

			case ":scheme":
				if scheme != "" {
					malformed = true
					return false
				}

				scheme = v

			case ":authority":
				if authority != "" {
					malformed = true
					return false
				}

				authority = v

			case ":protocol":
				// RFC 9220: Extended CONNECT for WebSockets
				reqHeaders.Set(":protocol", v)
			default:
				malformed = true
				return false
			}
		} else {
			hasSeenRegularHeader = true

			// RFC 9114 Â§4.1.2 & Â§4.1: Prohibited hop-by-hop headers in HTTP/3
			switch k {
			case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
				malformed = true
				return false
			case "te":
				if v != "trailers" {
					malformed = true
					return false
				}
			}

			reqHeaders.Add(k, v)
		}

		return true
	})
	if decodeErr != nil {
		return "", "", "", "", ErrQPACKDecompressFailed
	}

	if malformed {
		return "", "", "", "", ErrMalformedHeader
	}

	if method == "" || (method != "CONNECT" && (scheme == "" || path == "")) {
		return "", "", "", "", ErrMissingMethodOrPath
	}

	reqHeaders.SetRawBuf(arena)

	return method, path, scheme, authority, nil
}

func (q *QPACKCodec) EncodeResponseHeaders(statusCode int, headers coreheaders.Headers, bodyLen int) []byte {
	pe := encoderPool.Get()
	defer encoderPool.Put(pe)

	pe.buf.Reset()
	pe.enc.Reset(pe.buf)

	// 1. :status pseudo-header
	_ = pe.enc.WriteField(qpack.HeaderField{
		Name:  ":status",
		Value: strconv.Itoa(statusCode),
	})

	// 2. content-length
	if bodyLen >= 0 {
		_ = pe.enc.WriteField(qpack.HeaderField{
			Name:  "content-length",
			Value: strconv.Itoa(bodyLen),
		})
	}

	for k, v := range headers.All() {
		kLower := strings.ToLower(k)

		if isForbiddenH3Header([]byte(kLower), []byte(v)) {
			continue // skip in VisitAll
		}

		_ = pe.enc.WriteField(qpack.HeaderField{
			Name:  kLower,
			Value: v,
		})
	}

	result := make([]byte, pe.buf.Len())
	copy(result, pe.buf.Bytes())

	return result
}
