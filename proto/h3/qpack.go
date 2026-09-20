// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/qpack"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http"
)

// QPACKStreamError represents a fatal protocol error detected on a QPACK unidirectional stream (RFC 9204 §6).
type QPACKStreamError struct {
	Code         ErrorCode // ErrCodeQpackEncoderStreamError (0x0201) or ErrCodeQpackDecoderStreamError (0x0202)
	InternalCode uint64    // Raw internal QPACK error code from foundation/net/qpack
	Message      string    // Human-readable diagnostic message
	IsEncoder    bool      // true if error occurred on the encoder stream, false if on decoder stream
}

func (e *QPACKStreamError) Error() string {
	if e == nil {
		return "<nil>"
	}

	return fmt.Sprintf("h3: QPACK %s stream error (code: 0x%04x, internal: %d): %s",
		generic.Ternary(e.IsEncoder, "encoder", "decoder"), uint64(e.Code), e.InternalCode, e.Message)
}

func (e *QPACKStreamError) Is(target error) bool {
	if e == nil {
		return false
	}

	if target == ErrQPACKDecompressFailed && e.Code == ErrCodeQpackDecompressionFailed {
		return true
	}

	return false
}

// QPACKCodec manages QPACK header serialization and deserialization with dual mutex synchronization.
type QPACKCodec struct {
	encMu sync.Mutex
	decMu sync.Mutex

	decoder *qpack.Decoder
	encoder *qpack.Encoder

	errMu   sync.RWMutex
	lastErr error
	errCh   chan error
	errHook func(err error)
}

func (q *QPACKCodec) recordError(err error) {
	q.errMu.Lock()
	if q.lastErr == nil {
		q.lastErr = err
	}

	hook := q.errHook
	q.errMu.Unlock()

	if hook != nil {
		hook(err)
	}

	select {
	case q.errCh <- err:
	default:
		// Prevent blocking if buffer is saturated
	}
}

// Err returns the first fatal stream error recorded by the codec, or nil if healthy.
func (q *QPACKCodec) Err() error {
	q.errMu.RLock()
	defer q.errMu.RUnlock()
	return q.lastErr
}

// ErrChan returns a receive-only channel signaling codec stream errors.
func (q *QPACKCodec) ErrChan() <-chan error {
	return q.errCh
}

// SetErrorHandler registers a callback triggered immediately upon a QPACK stream error.
func (q *QPACKCodec) SetErrorHandler(fn func(err error)) {
	q.errMu.Lock()
	defer q.errMu.Unlock()

	q.errHook = fn
}

// NewQPACKCodec instantiates a thread-safe, panic-free QPACKCodec with defaults.
func NewQPACKCodec() *QPACKCodec {
	return NewQPACKCodecWithOptions(4096, 100, nil)
}

// NewQPACKCodecWithOptions constructs a QPACKCodec with custom capacity and error callback.
func NewQPACKCodecWithOptions(maxDynamicTableCapacity, maxBlockedStreams uint64, onError func(error)) *QPACKCodec {
	codec := &QPACKCodec{
		errCh:   make(chan error, 8),
		errHook: onError,
	}

	onEncoderErr := func(errorCode uint64, errorMessage string) {
		codec.recordError(&QPACKStreamError{
			Code:         ErrCodeQpackEncoderStreamError,
			InternalCode: errorCode,
			Message:      errorMessage,
			IsEncoder:    true,
		})
	}

	onDecoderErr := func(errorCode uint64, errorMessage string) {
		codec.recordError(&QPACKStreamError{
			Code:         ErrCodeQpackDecoderStreamError,
			InternalCode: errorCode,
			Message:      errorMessage,
			IsEncoder:    false,
		})
	}

	codec.decoder = qpack.NewDecoder(maxDynamicTableCapacity, maxBlockedStreams, onEncoderErr)
	codec.encoder = qpack.NewEncoderWithDefaults(onDecoderErr)

	return codec
}

func (q *QPACKCodec) Decoder() *qpack.Decoder {
	return q.decoder
}

func (q *QPACKCodec) Encoder() *qpack.Encoder {
	return q.encoder
}

// EncodeRequestHeaders encodes request headers into a QPACK block.
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

// isForbiddenH3Header checks if a header field is prohibited in HTTP/3 (RFC 9114 §4.1, §4.3 & §4.5).
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

func (q *QPACKCodec) getOrderedHeaders(req *http.Request, orderedKeys []string) []qpack.HeaderField {
	var (
		headers     []qpack.HeaderField
		visitedBits uint64
	)

	numOrdered := min(len(orderedKeys), 64)
	keys := orderedKeys[:numOrdered]

	for i := 0; i < numOrdered; i++ {
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
		for i := 0; i < numOrdered; i++ {
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
	res        *http.ResponseHeader
	hasStatus  bool
	statusCode int
	parseErr   error
}

func (h *responseHeaderHandler) OnHeaderDecoded(name, value string) {
	if h.parseErr != nil {
		return
	}

	if name == ":status" {
		if h.hasStatus {
			h.parseErr = ErrMalformedHeader
			return
		}

		code, err := strconv.Atoi(value)
		if err != nil {
			h.parseErr = err
			return
		}

		h.statusCode = code
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

	// Pseudo-headers check (simplified)
	if len(name) > 0 && name[0] == ':' {
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
	if len(name) > 0 && name[0] == ':' {
		return
	}

	h.trailers[name] = append(h.trailers[name], value)
}
func (h *trailersHandler) OnDecodingCompleted() {}
func (h *trailersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.err = ErrQPACKDecompressFailed
}

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

	reqHeaders.Reset()
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
