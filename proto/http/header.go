// Code automatically split by refactoring script

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	coreheaders "github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

const (
	rChar = byte('\r')
	nChar = byte('\n')
)

type header struct {
	h                     coreheaders.Headers
	cookies               []zerocopy.ArgsKV
	bufK                  []byte
	bufV                  []byte
	contentLengthBytes    []byte
	contentType           []byte
	protocol              []byte
	mulHeader             [][]byte
	trailer               [][]byte
	contentLength         int
	disableNormalizing    bool
	SecureErrorLogMessage bool
	noHTTP11              bool
	connectionClose       bool
	noDefaultContentType  bool
}

func (h *header) ConnectionClose() bool {
	return h.connectionClose
} // ConnectionClose returns true if 'Connection: close' header is set.

func (h *header) SetConnectionClose() {
	h.connectionClose = true
} // SetConnectionClose sets 'Connection: close' header.

func (h *header) ResetConnectionClose() {
	if h.connectionClose {
		h.connectionClose = false
		h.h.Del(HeaderConnection)
	}
} // ResetConnectionClose clears 'Connection: close' header if it exists.

func (h *header) SetContentType(contentType string) {
	h.contentType = zerocopy.InitHeaderValueString(h.contentType, contentType)
} // SetContentType sets Content-Type header value.

func (h *header) SetContentTypeBytes(contentType []byte) { // SetContentTypeBytes sets Content-Type header value.

	h.contentType = zerocopy.InitHeaderValueBytes(h.contentType, contentType)
}

func (h *header) SetTrailer(trailer string) error {
	return h.SetTrailerBytes(bytesconv.S2B(trailer))
} // SetTrailer sets header Trailer value for chunked response
// to indicate which headers will be sent after the body.
//
// Use Set to set the trailer header later.
//
// Trailers are only supported with chunked transfer.
// Trailers allow the sender to include additional headers at the end of chunked messages.
//
// The following trailers are forbidden:
// 1. necessary for message framing (e.g., Transfer-Encoding and Content-Length),
// 2. routing (e.g., Host),
// 3. request modifiers (e.g., controls and conditionals in Section 5 of [RFC7231]),
// 4. authentication (e.g., see [RFC7235] and [RFC6265]),
// 5. response control data (e.g., see Section 7.1 of [RFC7231]),
// 6. determining how to process the payload (e.g., Content-Encoding, Content-Type, Content-Range, and Trailer)
//
// Return ErrBadTrailer if contain any forbidden trailers.

func (h *header) SetTrailerBytes(trailer []byte) error { // SetTrailerBytes sets Trailer header value for chunked response
	// to indicate which headers will be sent after the body.
	//
	// Use Set to set the trailer header later.
	//
	// Trailers are only supported with chunked transfer.
	// Trailers allow the sender to include additional headers at the end of chunked messages.
	//
	// The following trailers are forbidden:
	// 1. necessary for message framing (e.g., Transfer-Encoding and Content-Length),
	// 2. routing (e.g., Host),
	// 3. request modifiers (e.g., controls and conditionals in Section 5 of [RFC7231]),
	// 4. authentication (e.g., see [RFC7235] and [RFC6265]),
	// 5. response control data (e.g., see Section 7.1 of [RFC7231]),
	// 6. determining how to process the payload (e.g., Content-Encoding, Content-Type, Content-Range, and Trailer)
	//
	// Return ErrBadTrailer if contain any forbidden trailers.

	h.trailer = h.trailer[:0]
	return h.AddTrailerBytes(trailer)
}

func (h *header) AddTrailer(trailer string) error {
	return h.AddTrailerBytes(bytesconv.S2B(trailer))
} // AddTrailer add Trailer header value for chunked response
// to indicate which headers will be sent after the body.
//
// Use Set to set the trailer header later.
//
// Trailers are only supported with chunked transfer.
// Trailers allow the sender to include additional headers at the end of chunked messages.
//
// The following trailers are forbidden:
// 1. necessary for message framing (e.g., Transfer-Encoding and Content-Length),
// 2. routing (e.g., Host),
// 3. request modifiers (e.g., controls and conditionals in Section 5 of [RFC7231]),
// 4. authentication (e.g., see [RFC7235] and [RFC6265]),
// 5. response control data (e.g., see Section 7.1 of [RFC7231]),
// 6. determining how to process the payload (e.g., Content-Encoding, Content-Type, Content-Range, and Trailer)
//
// Return ErrBadTrailer if contain any forbidden trailers.

var (
	ErrBadTrailer                    = errors.New("mach: contain forbidden trailer")
	ErrReadingResponseHeaders        = errors.New("mach: error when reading response headers")
	ErrReadingResponseTrailer        = errors.New("mach: error when reading response trailer")
	ErrResponseFirstLineMissingSpace = errors.New("mach: cannot find whitespace in the first line of response")
	ErrUnexpectedStatusCodeChar      = errors.New("mach: unexpected char at the end of status code")
	ErrMissingRequestMethod          = errors.New("mach: cannot find http request method")
	ErrUnsupportedRequestMethod      = errors.New("mach: unsupported http request method")
	ErrExtraWhitespaceInRequestLine  = errors.New("mach: extra whitespace in request line")
	ErrEmptyRequestURI               = errors.New("mach: requesturi cannot be empty")
	ErrDuplicateContentLength        = errors.New("mach: duplicate content-length header")
	ErrUnsupportedTransferEncoding   = errors.New("mach: unsupported transfer-encoding")
	ErrNonNumericChars               = errors.New("mach: non-numeric chars found")
	ErrNeedMore                      = errors.New("mach: need more data: cannot find trailing lf")
	ErrSmallReadBuffer               = errors.New("mach: small read buffer. increase readbuffersize")
)

func (h *header) AddTrailerBytes(trailer []byte) ( // AddTrailerBytes add Trailer header value for chunked response
	// to indicate which headers will be sent after the body.
	//
	// Use Set to set the trailer header later.
	//
	// Trailers are only supported with chunked transfer.
	// Trailers allow the sender to include additional headers at the end of chunked messages.
	//
	// The following trailers are forbidden:
	// 1. necessary for message framing (e.g., Transfer-Encoding and Content-Length),
	// 2. routing (e.g., Host),
	// 3. request modifiers (e.g., controls and conditionals in Section 5 of [RFC7231]),
	// 4. authentication (e.g., see [RFC7235] and [RFC6265]),
	// 5. response control data (e.g., see Section 7.1 of [RFC7231]),
	// 6. determining how to process the payload (e.g., Content-Encoding, Content-Type, Content-Range, and Trailer)
	//
	// Return ErrBadTrailer if contain any forbidden trailers.
	err error) {
	for i := -1; i+1 < len(trailer); {
		trailer = trailer[i+1:]
		i = bytes.IndexByte(trailer, ',')
		if i < 0 {
			i = len(trailer)
		}
		key := trim(trailer[:i])
		if !isValidTrailerKey(key) || isBadTrailer(key) {
			err = ErrBadTrailer
			continue
		}
		h.bufK = append(h.bufK[:0], key...)
		zerocopy.NormalizeHeaderKeyValidated(h.bufK, h.disableNormalizing)
		if cap(h.trailer) > len(h.trailer) {
			h.trailer = h.trailer[:len(h.trailer)+1]
			h.trailer[len(h.trailer)-1] = append(h.trailer[len(h.trailer)-1][:0], h.bufK...)
		} else {
			key = make([]byte, len(h.bufK))
			copy(key, h.bufK)
			h.trailer = append(h.trailer, key)
		}
	}
	return err
}

func isValidTrailerKey(key []byte) bool {
	if len(key) == 0 {
		return false
	}
	for _, c := range key {
		if !zerocopy.ValidHeaderFieldByte(c) {
			return false
		}
	}
	return true
}

func validHeaderValueByte(c byte) bool {
	return zerocopy.ValidHeaderValueByteTable[c] == 1
} // validHeaderValueByte returns true if c valid header value byte
// as defined by RFC 7230.

func isValidHeaderKey(a []byte) (valid, innerSpace bool) {
	if len(a) == 0 {
		return false, false
	}
	for _, c := range a {
		if !zerocopy.ValidHeaderFieldByte(c) {
			return false, false
		}
	}
	return true, false
}

func VisitHeaderParams(b []byte, // VisitHeaderParams calls f for each parameter in the given header bytes.
	// It stops processing when f returns false or an invalid parameter is found.
	// Parameter values may be quoted, in which case \ is treated as an escape
	// character, and the value is unquoted before being passed to value.
	// See: https://www.rfc-editor.org/rfc/rfc9110#section-5.6.6
	//
	// f must not retain references to key and/or value after returning.
	// Copy key and/or value contents before returning if you need retaining them.
	f func(key, value []byte) bool) {
	for len(b) > 0 {
		idxSemi := 0
		for idxSemi < len(b) && b[idxSemi] != ';' {
			idxSemi++
		}
		if idxSemi >= len(b) {
			return
		}
		b = b[idxSemi+1:]
		for len(b) > 0 && b[0] == ' ' {
			b = b[1:]
		}
		n := 0
		if len(b) == 0 || !zerocopy.ValidHeaderFieldByte(b[n]) {
			return
		}
		n++
		for n < len(b) && zerocopy.ValidHeaderFieldByte(b[n]) {
			n++
		}
		if n >= len(b)-1 || b[n] != '=' {
			return
		}
		param := b[:n]
		n++
		switch {
		case zerocopy.ValidHeaderFieldByte(b[n]):
			m := n
			n++
			for n < len(b) && zerocopy.ValidHeaderFieldByte(b[n]) {
				n++
			}
			if !f(param, b[m:n]) {
				return
			}
		case b[n] == '"':
			foundEndQuote := false
			escaping := false
			n++
			m := n
			for ; n < len(b); n++ {
				if b[n] == '"' && !escaping {
					foundEndQuote = true
					break
				}
				escaping = (b[n] == '\\' && !escaping)
			}
			if !foundEndQuote {
				return
			}
			if !f(param, b[m:n]) {
				return
			}
			n++
		default:
			return
		}
		b = b[n:]
	}
}

func (h *header) Protocol() []byte { // Protocol returns HTTP protocol.

	if len(h.protocol) == 0 {
		return zerocopy.StrHTTP11
	}
	return h.protocol
}

func (h *header) IsHTTP11() bool {
	return !h.noHTTP11
} // IsHTTP11 returns true if the header is HTTP/1.1.

func (h *header) DisableNormalizing() bool {
	orig := h.disableNormalizing
	h.disableNormalizing = true
	return orig
} // DisableNormalizing disables header names' normalization.
//
// By default all the header names are normalized by uppercasing
// the first letter and all the first letters following dashes,
// while lowercasing all the other letters.
// Examples:
//
//   - CONNECTION -> Connection
//   - conteNT-tYPE -> Content-Type
//   - foo-bar-baz -> Foo-Bar-Baz
//
// Disable header names' normalization only if know what are you doing.
// The previous setting is returned.

func (h *header) EnableNormalizing() bool {
	orig := h.disableNormalizing
	h.disableNormalizing = false
	return orig
} // EnableNormalizing enables header names' normalization.
//
// Header names are normalized by uppercasing the first letter and
// all the first letters following dashes, while lowercasing all
// the other letters.
// Examples:
//
//   - CONNECTION -> Connection
//   - conteNT-tYPE -> Content-Type
//   - foo-bar-baz -> Foo-Bar-Baz
//
// This is enabled by default unless disabled using DisableNormalizing().
// The previous setting is returned.

func (h *header) SetNoDefaultContentType(noDefaultContentType bool) {
	h.noDefaultContentType = noDefaultContentType
} // SetNoDefaultContentType allows you to control if a default Content-Type header will be set (false) or not (true).

func (h *header) copyTo(dst *header) {
	dst.disableNormalizing = h.disableNormalizing
	dst.noHTTP11 = h.noHTTP11
	dst.connectionClose = h.connectionClose
	dst.noDefaultContentType = h.noDefaultContentType
	dst.contentLength = h.contentLength
	dst.contentLengthBytes = append(dst.contentLengthBytes, h.contentLengthBytes...)
	dst.protocol = append(dst.protocol, h.protocol...)
	dst.contentType = append(dst.contentType, h.contentType...)
	dst.trailer = copyTrailer(dst.trailer, h.trailer)
	dst.cookies = zerocopy.CopyArgs(dst.cookies, h.cookies)
	copyHeaders(&dst.h, &h.h)
}

func (h *header) Trailers() iter.Seq[[]byte] { // Trailers returns an iterator over trailers in h.
	//
	// The value of trailer may invalid outside the iteration loop.

	return func(yield func([]byte) bool) {
		for i := range h.trailer {
			if !yield(h.trailer[i]) {
				break
			}
		}
	}
}

func (h *header) setNonSpecial(key, value []byte) { // setNonSpecial directly put into map i.e. not a basic header.

	setArgBytesHeaders(&h.h, key, value, zerocopy.ArgsHasValue)
}

func (h *header) PeekTrailerKeys() [][]byte { // PeekTrailerKeys return all trailer keys.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseResponse or your request handler returning.
	// Any future calls to the Peek* will modify the returned value.
	// Do not store references to returned value. Make copies instead.

	return h.trailer
}

func (h *header) ReadTrailer(r *bufio.Reader) error {
	n := 1
	for {
		err := h.tryReadTrailer(r, n)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNeedMore) {
			return err
		}
		n = r.Buffered() + 1
	}
} // ReadTrailer reads response trailer header from r.
//
// io.EOF is returned if r is closed before reading the first byte.

func (h *header) tryReadTrailer(r *bufio.Reader, n int) error {
	b, err := r.Peek(n)
	if len(b) == 0 {
		if x, ok := err.(interface{ Timeout() bool }); ok && x.Timeout() {
			return ErrTimeout
		}
		if n == 1 || err == io.EOF {
			return io.EOF
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			if h.SecureErrorLogMessage {
				return &ErrSmallBuffer{error: ErrReadingResponseTrailer}
			}
			return &ErrSmallBuffer{error: fmt.Errorf("error when reading response trailer: %w", ErrSmallReadBuffer)}
		}
		return fmt.Errorf("error when reading response trailer: %w", err)
	}
	b = mustPeekBuffered(r)
	headersLen, errParse := parseTrailerHeaders(b, &h.h, h.disableNormalizing)
	if errParse != nil {
		if err == io.EOF {
			return err
		}
		return headerError("response", err, errParse, b, h.SecureErrorLogMessage)
	}
	mustDiscard(r, headersLen)
	return nil
}

func headerError(typ string, err, errParse error, b []byte, SecureErrorLogMessage bool) error {
	if !errors.Is(errParse, ErrNeedMore) {
		return headerErrorMsg(typ, errParse, b, SecureErrorLogMessage)
	}
	if err == nil {
		return ErrNeedMore
	}
	if isOnlyCRLF(b) {
		return io.EOF
	}
	if !errors.Is(err, bufio.ErrBufferFull) {
		return headerErrorMsg(typ, err, b, SecureErrorLogMessage)
	}
	return &ErrSmallBuffer{error: headerErrorMsg(typ, ErrSmallReadBuffer, b, SecureErrorLogMessage)}
}

func headerErrorMsg(typ string, err error, b []byte, SecureErrorLogMessage bool) error {
	if SecureErrorLogMessage {
		return fmt.Errorf("error when reading %s headers: %w: buffer size=%d", typ, err, len(b))
	}
	return fmt.Errorf("error when reading %s headers: %w: buffer size=%d, contents: %s", typ, err, len(b), bufferSnippet(b))
}

func bufferSnippet(b []byte) string {
	n := len(b)
	start := 200
	end := n - start
	if start >= end {
		start = n
		end = n
	}
	bStart, bEnd := b[:start], b[end:]
	if len(bEnd) == 0 {
		return fmt.Sprintf("%q", b)
	}
	return fmt.Sprintf("%q...%q", bStart, bEnd)
}

func isOnlyCRLF(b []byte) bool {
	for _, ch := range b {
		if ch != rChar && ch != nChar {
			return false
		}
	}
	return true
}

func updateServerDate() {
	refreshServerDate()
	go func() {
		for {
			time.Sleep(time.Second)
			refreshServerDate()
		}
	}()
}

var (
	serverDate     atomic.Pointer[[]byte]
	serverDateOnce sync.Once
) // serverDateOnce.Do(updateServerDate)

func refreshServerDate() {
	b := zerocopy.AppendHTTPDate(nil, time.Now())
	serverDate.Store(&b)
}

func appendHeaderLine(dst, key, value []byte) []byte {
	dst = append(dst, key...)
	dst = append(dst, zerocopy.StrColonSpace...)
	dst = append(dst, value...)
	return append(dst, zerocopy.StrCRLF...)
}

func parseTrailer(src []byte, //nolint:unused
	dest []zerocopy.ArgsKV, disableNormalizing bool) ([]zerocopy.ArgsKV, int, error) {
	var s headerScanner
	s.b = src
	for s.next() {
		s.key = trimTrailingSpace(s.key)
		if len(s.key) == 0 {
			continue
		}
		disable := disableNormalizing || s.keyHasSpace
		if isBadTrailer(s.key) {
			return dest, 0, fmt.Errorf("forbidden trailer key %q", s.key)
		}
		for _, ch := range s.value {
			if !validHeaderValueByte(ch) {
				return dest, 0, fmt.Errorf("invalid trailer value %q", s.value)
			}
		}
		zerocopy.NormalizeHeaderKeyValidated(s.key, disable)
		dest = zerocopy.AppendArgBytes(dest, s.key, s.value, zerocopy.ArgsHasValue)
	}
	if s.err != nil {
		return dest, 0, s.err
	}
	return dest, s.r, nil
}

func isBadTrailer(key []byte) bool {
	if len(key) == 0 {
		return true
	}
	switch key[0] | 0x20 {
	case 'a':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrAuthorization)
	case 'c':
		if len(key) >= len(HeaderContentType) && zerocopy.CaseInsensitiveCompare(key[:8], zerocopy.StrContentType[:8]) {
			return zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentEncoding[8:]) || zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentLength[8:]) || zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentType[8:]) || zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentRange[8:])
		}
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrConnection) || zerocopy.CaseInsensitiveCompare(key, zerocopy.StrCookie)
	case 'e':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrExpect)
	case 'h':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrHost)
	case 'k':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrKeepAlive)
	case 'l':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrLocation)
	case 'm':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrMaxForwards)
	case 'p':
		if len(key) >= len(HeaderProxyConnection) && zerocopy.CaseInsensitiveCompare(key[:6], zerocopy.StrProxyConnection[:6]) {
			return zerocopy.CaseInsensitiveCompare(key[6:], zerocopy.StrProxyConnection[6:]) || zerocopy.CaseInsensitiveCompare(key[6:], zerocopy.StrProxyAuthenticate[6:]) || zerocopy.CaseInsensitiveCompare(key[6:], zerocopy.StrProxyAuthorization[6:])
		}
	case 'r':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrRange)
	case 's':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrSetCookie)
	case 't':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrTE) || zerocopy.CaseInsensitiveCompare(key, zerocopy.StrTrailer) || zerocopy.CaseInsensitiveCompare(key, zerocopy.StrTransferEncoding)
	case 'w':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrWWWAuthenticate)
	case 'x':
		return (len(key) >= 11 && zerocopy.CaseInsensitiveCompare(key[:11], []byte("x-forwarded"))) || (len(key) >= 9 && zerocopy.CaseInsensitiveCompare(key[:9], []byte("x-real-ip")))
	}
	return false
}

func isHTTPVersion(proto []byte) bool {
	return len(proto) == len(zerocopy.StrHTTP11) && bytes.HasPrefix(proto, zerocopy.StrHTTP11[:5]) && proto[6] == '.' && proto[5] >= '0' && proto[5] <= '9' && proto[7] >= '0' && proto[7] <= '9'
}

func isValidMethod(method []byte) bool {
	for _, ch := range method {
		if zerocopy.ValidMethodValueByteTable[ch] == 0 {
			return false
		}
	}
	return true
}

func validateRequestURI(method, requestURI []byte) error {
	if zerocopy.StringContainsCTLByte(requestURI) {
		return zerocopy.ErrorInvalidURI
	}
	if len(requestURI) == 1 && requestURI[0] == '*' {
		return nil
	}
	if len(requestURI) > 0 && requestURI[0] == '/' {
		return nil
	}
	if before, _, ok := bytes.Cut(requestURI, zerocopy.StrColonSlashSlash); ok {
		if !zerocopy.IsValidScheme(before) {
			return zerocopy.ErrorInvalidURI
		}
		return nil
	}
	if bytes.Equal(method, zerocopy.StrConnect) {
		return nil
	}
	return zerocopy.ErrorInvalidURI
}

func readRawHeaders(dst, buf []byte) ([]byte, int, error) {
	if len(buf) == 0 {
		return dst[:0], 0, ErrNeedMore
	}
	if simd.MatchCRLF(buf) {
		return dst, 2, nil
	}
	if buf[0] == nChar {
		return dst, 1, nil
	}
	idx := simd.IndexCRLFCRLF(buf)
	if idx < 0 {
		return dst, 0, ErrNeedMore
	}
	dst = append(dst, buf[:idx]...)
	return dst, idx, nil
}

func parseContentLength(b []byte) (int, error) {
	v, n, err := zerocopy.ParseUintBuf(b)
	if err != nil {
		return -1, fmt.Errorf("cannot parse content-length: %w", err)
	}
	if n != len(b) {
		return -1, fmt.Errorf("cannot parse content-length: %w", ErrNonNumericChars)
	}
	return v, nil
}

type headerValueScanner struct {
	b     []byte
	value []byte
}

func (s *headerValueScanner) next() bool {
	b := s.b
	if len(b) == 0 {
		return false
	}
	before, after, ok := bytes.Cut(b, []byte{','})
	if !ok {
		s.value = stripSpace(b)
		s.b = b[len(b):]
		return true
	}
	s.value = stripSpace(before)
	s.b = after
	return true
}

func stripSpace(b []byte) []byte {
	for len(b) > 0 && b[0] == ' ' {
		b = b[1:]
	}
	for len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	return b
}

func hasHeaderValue(s, value []byte) bool {
	var vs headerValueScanner
	vs.b = s
	for vs.next() {
		if zerocopy.CaseInsensitiveCompare(vs.value, value) {
			return true
		}
	}
	return false
}

func nextLine(b []byte) ([]byte, []byte, error) {
	nNext := simd.IndexByteVector(b, nChar)
	if nNext < 0 {
		return nil, nil, ErrNeedMore
	}
	n := nNext
	if n > 0 && b[n-1] == rChar {
		n--
	}
	return b[:n], b[nNext+1:], nil
}

func AppendNormalizedHeaderKey(dst []byte, // AppendNormalizedHeaderKey appends normalized header key (name) to dst
	// and returns the resulting dst.
	//
	// Normalized header key starts with uppercase letter. The first letters
	// after dashes are also uppercased. All the other letters are lowercased.
	// Examples:
	//
	//   - coNTENT-TYPe -> Content-Type
	//   - HOST -> Host
	//   - foo-bar-baz -> Foo-Bar-Baz
	key string) []byte {
	dst = append(dst, key...)
	zerocopy.NormalizeHeaderKey(dst[len(dst)-len(key):], false)
	return dst
}

func AppendNormalizedHeaderKeyBytes(dst, key []byte) []byte { // AppendNormalizedHeaderKeyBytes appends normalized header key (name) to dst
	// and returns the resulting dst.
	//
	// Normalized header key starts with uppercase letter. The first letters
	// after dashes are also uppercased. All the other letters are lowercased.
	// Examples:
	//
	//   - coNTENT-TYPe -> Content-Type
	//   - HOST -> Host
	//   - foo-bar-baz -> Foo-Bar-Baz

	return AppendNormalizedHeaderKey(dst, bytesconv.B2S(key))
}

func appendTrailerBytes(dst []byte, trailer [][]byte, sep []byte) []byte {
	for i, n := 0, len(trailer); i < n; i++ {
		dst = append(dst, trailer[i]...)
		if i+1 < n {
			dst = append(dst, sep...)
		}
	}
	return dst
}

func copyTrailer(dst, src [][]byte) [][]byte {
	if cap(dst) >= len(src) {
		dst = dst[:len(src)]
	} else {
		dst = make([][]byte, len(src))
	}
	for i := range dst {
		l := len(src[i])
		if cap(dst[i]) >= l {
			dst[i] = dst[i][:l]
		} else {
			dst[i] = make([]byte, l)
		}
		copy(dst[i], src[i])
	}
	return dst
}

type ErrNothingRead struct{ error } // ErrNothingRead is returned when a keep-alive connection is closed,
// either because the remote closed it or because of a read timeout.

type ErrSmallBuffer struct{ error } // ErrSmallBuffer is returned when the provided buffer size is too small
// for reading request and/or response headers.
//
// ReadBufferSize value from Server or clients should reduce the number
// of such errors.

func mustPeekBuffered(r *bufio.Reader) []byte {
	buf, err := r.Peek(r.Buffered())
	if len(buf) == 0 || err != nil {
		panic(fmt.Sprintf("bufio.Reader.Peek() returned unexpected data (%q, %v)", buf, err))
	}
	return buf
}

func mustDiscard(r *bufio.Reader, n int) {
	if _, err := r.Discard(n); err != nil {
		panic(fmt.Sprintf("bufio.Reader.Discard(%d) failed: %v", n, err))
	}
}
