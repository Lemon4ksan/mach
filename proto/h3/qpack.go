package h3

import (
	"io"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/qpack"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/mach/proto/http"
)

type qpackDelegate struct{}

func (d qpackDelegate) OnEncoderStreamError(errorCode uint64, errorMessage string) {
	panic("qpack encoder stream error: " + errorMessage)
}

func (d qpackDelegate) OnDecoderStreamError(errorCode uint64, errorMessage string) {
	panic("qpack decoder stream error: " + errorMessage)
}

// QPACKCodec manages QPACK header serialization and deserialization.
type QPACKCodec struct {
	decoder *qpack.QpackDecoder
	encoder *qpack.QpackEncoder
}

// NewQPACKCodec instantiates a new QPACKCodec.
func NewQPACKCodec() *QPACKCodec {
	d := qpackDelegate{}
	return &QPACKCodec{
		decoder: qpack.NewQpackDecoder(4096, 100, d),
		encoder: qpack.NewQpackEncoderWithDefaults(d),
	}
}

func (q *QPACKCodec) Decoder() *qpack.QpackDecoder {
	return q.decoder
}

func (q *QPACKCodec) Encoder() *qpack.QpackEncoder {
	return q.encoder
}

// EncodeRequestHeaders encodes request headers into a QPACK block.
func (q *QPACKCodec) EncodeRequestHeaders(streamID uint64, w io.Writer, req *http.Request, orderedKeys []string) error {
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
				keyStr = bytesconv.B2S(keyBuf)
			} else {
				keyStr = bytesconv.B2S(bytesconv.AppendToLower(nil, k))
			}

			headers = append(headers, qpack.HeaderField{Name: keyStr, Value: bytesconv.B2S(v)})
		}
	}

	block := q.encoder.EncodeHeaderList(streamID, headers, nil)
	_, err := w.Write(block)
	return err
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
	var headers []qpack.HeaderField
	var visitedBits uint64

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
			keyStr = bytesconv.B2S(keyBuf)
		} else {
			keyStr = bytesconv.B2S(bytesconv.AppendToLower(nil, k))
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

func (q *QPACKCodec) DecodeResponseHeaders(streamID uint64, headerBlock []byte, res *http.ResponseHeader) (int, error) {
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

func (q *QPACKCodec) DecodeResponseTrailers(streamID uint64, headerBlock []byte) (map[string][]string, error) {
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

func (q *QPACKCodec) DecodeRequestHeaders(streamID uint64, headerBlock []byte, reqHeaders *headkit.Headers) (string, string, string, string, error) {
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

func (q *QPACKCodec) EncodeResponseHeaders(streamID uint64, statusCode int, headers headkit.Headers, bodyLen int) []byte {
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
