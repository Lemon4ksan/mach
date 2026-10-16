// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package digest

import (
	"bytes"
	"crypto/md5" //nolint:gosec
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var (
	// ErrDigestBadChallenge is returned when the server responds with a malformed Digest WWW-Authenticate challenge.
	ErrDigestBadChallenge = errors.New("digest: bad challenge")

	// ErrDigestBadCredentials is returned when the client sends malformed Digest Authorization credentials.
	ErrDigestBadCredentials = errors.New("digest: bad credentials")

	// ErrDigestBadAuthInfo is returned when the server sends malformed Authentication-Info.
	ErrDigestBadAuthInfo = errors.New("digest: bad authentication info")

	// ErrDigestInvalidCharset is returned when the server challenge specifies an unsupported character encoding.
	ErrDigestInvalidCharset = errors.New("digest: invalid charset")

	// ErrDigestAlgNotSupported is returned when the server requires an unsupported hash algorithm.
	ErrDigestAlgNotSupported = errors.New("digest: algorithm not supported")

	// ErrDigestQopNotSupported is returned when the server requires an unsupported quality of protection mode.
	ErrDigestQopNotSupported = errors.New("digest: qop not supported")
)

// digestHashFuncs registers hash algorithms for HTTP Digest Authentication (RFC 7616 §6.1).
var digestHashFuncs = map[string]func() hash.Hash{
	"":                 md5.New,
	"MD5":              md5.New,
	"MD5-SESS":         md5.New,
	"SHA-256":          sha256.New,
	"SHA-256-SESS":     sha256.New,
	"SHA-512":          sha512.New,
	"SHA-512-SESS":     sha512.New,
	"SHA-512-256":      sha512.New512_256,
	"SHA-512-256-SESS": sha512.New512_256,
}

const (
	qopAuth    = "auth"
	qopAuthInt = "auth-int"
)

// Transport wraps an [http.RoundTripper] to automatically resolve HTTP 401 Digest Authentication challenges.
type Transport struct {
	Username  string
	Password  string
	Transport http.RoundTripper
}

// RoundTrip executes requests, handling HTTP 401 Digest challenge-response flows transparently.
func (dt *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr := dt.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}

	req1 := dt.cloneReq(req)

	res, err := tr.RoundTrip(req1)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		return res, err
	}

	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()

	// RFC 7616 Section 3.7: Iterate over all WWW-Authenticate challenges
	headers := res.Header.Values("WWW-Authenticate")
	if len(headers) == 0 {
		if h := res.Header.Get("WWW-Authenticate"); h != "" {
			headers = []string{h}
		}
	}

	if len(headers) == 0 {
		return nil, ErrDigestBadChallenge
	}

	var (
		cha     *Challenge
		lastErr error
	)

	for _, headerVal := range headers {
		c, parseErr := Parse(headerVal)
		if parseErr == nil {
			cha = c
			break
		}

		lastErr = parseErr
	}

	if cha == nil {
		if lastErr != nil {
			return nil, lastErr
		}

		return nil, ErrDigestBadChallenge
	}

	req2 := dt.cloneReq(req)

	cred, err := dt.createCredentials(cha, req2)
	if err != nil {
		return nil, err
	}

	authHdr, err := cred.digest(cha)
	if err != nil {
		return nil, err
	}

	req2.Header.Set("Authorization", authHdr)

	return tr.RoundTrip(req2)
}

// Unwrap returns the underlying wrapped [http.RoundTripper].
func (dt *Transport) Unwrap() http.RoundTripper {
	return dt.Transport
}

// CloneTransport creates a deep copy of the [Transport] wrapping the specified next [http.RoundTripper].
func (dt *Transport) CloneTransport(next http.RoundTripper) http.RoundTripper {
	return &Transport{
		Username:  dt.Username,
		Password:  dt.Password,
		Transport: next,
	}
}

func (dt *Transport) cloneReq(r *http.Request) *http.Request {
	return r.Clone(r.Context())
}

// Parse parses WWW-Authenticate challenge parameters ([string] or [[]byte]) (RFC 7616 §3.3).
func Parse[S ~string | ~[]byte](input S) (*Challenge, error) {
	return ParseChallenge(input)
}

// ParseInto parses WWW-Authenticate challenge parameters into dst without heap allocations.
func ParseInto[S ~string | ~[]byte](dst *Challenge, input S) error {
	return ParseChallengeInto(dst, input)
}

// ParseChallenge parses WWW-Authenticate challenge parameters ([string] or [[]byte]) (RFC 7616 §3.3).
func ParseChallenge[S ~string | ~[]byte](input S) (*Challenge, error) {
	c := &Challenge{}
	if err := ParseChallengeInto(c, input); err != nil {
		return nil, err
	}

	return c, nil
}

// ParseChallengeInto parses WWW-Authenticate challenge parameters into dst without heap allocations.
func ParseChallengeInto[S ~string | ~[]byte](dst *Challenge, input S) error {
	raw := stringOrBytes(input)

	var parseErr error

	foundAny := false

	ok := forEachAuthParam(raw, "Digest", func(key, val string) bool {
		foundAny = true

		if err := dst.setValue(key, val); err != nil {
			parseErr = err
			return false
		}

		return true
	})

	if parseErr != nil {
		return parseErr
	}

	if !ok || !foundAny {
		return ErrDigestBadChallenge
	}

	return nil
}

// ParseCredentials parses Digest Authorization credentials ([string] or [[]byte]) (RFC 7616 §3.4).
func ParseCredentials[S ~string | ~[]byte](input S) (*Credentials, error) {
	cred := &Credentials{}
	if err := ParseCredentialsInto(cred, input); err != nil {
		return nil, err
	}

	return cred, nil
}

// ParseCredentialsInto parses Digest Authorization credentials into dst without heap allocations.
func ParseCredentialsInto[S ~string | ~[]byte](dst *Credentials, input S) error {
	raw := stringOrBytes(input)

	var parseErr error

	foundAny := false

	ok := forEachAuthParam(raw, "Digest", func(key, val string) bool {
		foundAny = true

		if err := dst.setValue(key, val); err != nil {
			parseErr = err
			return false
		}

		return true
	})

	if parseErr != nil {
		return parseErr
	}

	if !ok || !foundAny {
		return ErrDigestBadCredentials
	}

	return nil
}

// ParseAuthenticationInfo parses Authentication-Info header parameters ([string] or [[]byte]) (RFC 7616 §3.5).
func ParseAuthenticationInfo[S ~string | ~[]byte](input S) (*AuthenticationInfo, error) {
	ai := &AuthenticationInfo{}
	if err := ParseAuthenticationInfoInto(ai, input); err != nil {
		return nil, err
	}

	return ai, nil
}

// ParseAuthenticationInfoInto parses Authentication-Info header parameters into dst without heap allocations.
func ParseAuthenticationInfoInto[S ~string | ~[]byte](dst *AuthenticationInfo, input S) error {
	raw := stringOrBytes(input)

	var parseErr error

	foundAny := false

	ok := forEachAuthParam(raw, "", func(key, val string) bool {
		foundAny = true

		if err := dst.setValue(key, val); err != nil {
			parseErr = err
			return false
		}

		return true
	})

	if parseErr != nil {
		return parseErr
	}

	if !ok || !foundAny {
		return ErrDigestBadAuthInfo
	}

	return nil
}

// createCredentials constructs digest credentials for the request (RFC 7616 §3.4).
func (dt *Transport) createCredentials(cha *Challenge, req *http.Request) (*digestCredentials, error) {
	cred := &digestCredentials{
		username:      dt.Username,
		password:      dt.Password,
		uri:           req.URL.RequestURI(),
		method:        req.Method,
		realm:         cha.Realm,
		nonce:         cha.Nonce,
		nc:            cha.NC,
		algorithm:     cha.Algorithm,
		sessAlgorithm: strings.HasSuffix(strings.ToLower(cha.Algorithm), "-sess"),
		opaque:        cha.Opaque,
		userHash:      cha.UserHash,
	}

	if err := cred.parseQop(cha); err != nil {
		return nil, err
	}

	if cred.qop == qopAuthInt {
		if err := dt.prepareBody(req); err != nil {
			return nil, fmt.Errorf("digest: failed to prepare body: %w", err)
		}

		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("digest: failed to get body: %w", err)
		}

		h := newHashFunc(cha.Algorithm)
		if body != nil && body != http.NoBody {
			defer body.Close()

			if _, err := io.Copy(h, body); err != nil {
				return nil, fmt.Errorf("digest: failed to hash body: %w", err)
			}
		}

		cred.bodyHash = hex.EncodeToString(h.Sum(nil))
	}

	return cred, nil
}

func (dt *Transport) prepareBody(req *http.Request) error {
	if req.GetBody != nil {
		return nil
	}

	if req.Body == nil || req.Body == http.NoBody {
		req.GetBody = func() (io.ReadCloser, error) {
			return http.NoBody, nil
		}

		return nil
	}

	b, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("digest: failed to read body: %w", err)
	}

	_ = req.Body.Close()
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(b))

	return nil
}

// Challenge represents an RFC 7616 Digest challenge parsed from WWW-Authenticate.
type Challenge struct {
	Realm     string
	Domain    string
	Nonce     string
	Opaque    string
	Stale     string
	Algorithm string
	Qop       []string
	NC        int
	UserHash  string
	Charset   string
}

var (
	qopAuthSlice        = []string{qopAuth}
	qopAuthIntSlice     = []string{qopAuthInt}
	qopAuthBothSlice    = []string{qopAuth, qopAuthInt}
	qopAuthBothRevSlice = []string{qopAuthInt, qopAuth}
)

func (dc *Challenge) isQopSupported(qop string) bool {
	for _, item := range dc.Qop {
		if strings.TrimSpace(item) == qop {
			return true
		}
	}

	return false
}

func (dc *Challenge) setValue(k, v string) error {
	switch {
	case strings.EqualFold(k, "realm"):
		dc.Realm = v
	case strings.EqualFold(k, "domain"):
		dc.Domain = v
	case strings.EqualFold(k, "nonce"):
		dc.Nonce = v
	case strings.EqualFold(k, "opaque"):
		dc.Opaque = v
	case strings.EqualFold(k, "stale"):
		dc.Stale = v
	case strings.EqualFold(k, "algorithm"):
		dc.Algorithm = v
	case strings.EqualFold(k, "qop"):
		vTrim := strings.TrimSpace(v)
		switch {
		case strings.EqualFold(vTrim, "auth"):
			dc.Qop = qopAuthSlice
		case strings.EqualFold(vTrim, "auth-int"):
			dc.Qop = qopAuthIntSlice
		case strings.EqualFold(vTrim, "auth, auth-int") || strings.EqualFold(vTrim, "auth,auth-int"):
			dc.Qop = qopAuthBothSlice
		case strings.EqualFold(vTrim, "auth-int, auth") || strings.EqualFold(vTrim, "auth-int,auth"):
			dc.Qop = qopAuthBothRevSlice
		default:
			if vTrim != "" {
				parts := strings.Split(vTrim, ",")

				dc.Qop = make([]string, len(parts))
				for i, p := range parts {
					dc.Qop[i] = strings.TrimSpace(p)
				}
			}
		}

	case strings.EqualFold(k, "charset"):
		if v != "" {
			if !strings.EqualFold(v, "UTF-8") {
				return ErrDigestInvalidCharset
			}

			dc.Charset = "UTF-8"
		}

	case strings.EqualFold(k, "nc"):
		nc, err := strconv.ParseInt(v, 16, 32)
		if err != nil {
			return fmt.Errorf("digest: invalid nc: %w", err)
		}

		dc.NC = int(nc)

	case strings.EqualFold(k, "userhash"):
		switch {
		case strings.EqualFold(v, "true"):
			dc.UserHash = "true"
		case strings.EqualFold(v, "false"):
			dc.UserHash = "false"
		default:
			dc.UserHash = v
		}

	default:
		// RFC 7616 §3.3: Unknown parameters MUST be ignored
		return nil
	}

	return nil
}

// Parse parses WWW-Authenticate challenge parameters into dc.
//
// Deprecated: use [ParseInto] or [ParseChallengeInto] instead.
func (dc *Challenge) Parse(input string) error {
	return ParseChallengeInto(dc, input)
}

// ParseBytes parses WWW-Authenticate challenge parameters into dc.
//
// Deprecated: use [ParseInto] or [ParseChallengeInto] instead.
func (dc *Challenge) ParseBytes(input []byte) error {
	return ParseChallengeInto(dc, input)
}

// Credentials represents parsed RFC 7616 Digest Authorization credentials.
type Credentials struct {
	Username  string
	Realm     string
	Nonce     string
	URI       string
	Response  string
	Algorithm string
	Cnonce    string
	Opaque    string
	Qop       string
	NC        int
	UserHash  string
}

func (c *Credentials) setValue(k, v string) error {
	switch {
	case strings.EqualFold(k, "username"):
		c.Username = v
	case strings.EqualFold(k, "realm"):
		c.Realm = v
	case strings.EqualFold(k, "nonce"):
		c.Nonce = v
	case strings.EqualFold(k, "uri"):
		c.URI = v
	case strings.EqualFold(k, "response"):
		c.Response = v
	case strings.EqualFold(k, "algorithm"):
		c.Algorithm = v
	case strings.EqualFold(k, "cnonce"):
		c.Cnonce = v
	case strings.EqualFold(k, "opaque"):
		c.Opaque = v
	case strings.EqualFold(k, "qop"):
		c.Qop = v
	case strings.EqualFold(k, "nc"):
		nc, err := strconv.ParseInt(v, 16, 32)
		if err == nil {
			c.NC = int(nc)
		}
	case strings.EqualFold(k, "userhash"):
		switch {
		case strings.EqualFold(v, "true"):
			c.UserHash = "true"
		case strings.EqualFold(v, "false"):
			c.UserHash = "false"
		default:
			c.UserHash = v
		}
	}

	return nil
}

// Parse parses Digest Authorization credentials into c.
//
// Deprecated: use [ParseCredentialsInto] instead.
func (c *Credentials) Parse(input string) error {
	return ParseCredentialsInto(c, input)
}

// ParseBytes parses Digest Authorization credentials into c.
//
// Deprecated: use [ParseCredentialsInto] instead.
func (c *Credentials) ParseBytes(input []byte) error {
	return ParseCredentialsInto(c, input)
}

// AuthenticationInfo represents parsed RFC 7616 Authentication-Info parameters.
type AuthenticationInfo struct {
	NextNonce string
	Qop       string
	RspAuth   string
	CNonce    string
	NC        int
	UserHash  string
}

func (ai *AuthenticationInfo) setValue(k, v string) error {
	switch {
	case strings.EqualFold(k, "nextnonce"):
		ai.NextNonce = v
	case strings.EqualFold(k, "qop"):
		ai.Qop = v
	case strings.EqualFold(k, "rspauth"):
		ai.RspAuth = v
	case strings.EqualFold(k, "cnonce"):
		ai.CNonce = v
	case strings.EqualFold(k, "nc"):
		nc, err := strconv.ParseInt(v, 16, 32)
		if err == nil {
			ai.NC = int(nc)
		}
	case strings.EqualFold(k, "userhash"):
		switch {
		case strings.EqualFold(v, "true"):
			ai.UserHash = "true"
		case strings.EqualFold(v, "false"):
			ai.UserHash = "false"
		default:
			ai.UserHash = v
		}
	}

	return nil
}

// Parse parses Authentication-Info header parameters into ai.
//
// Deprecated: use [ParseAuthenticationInfoInto] instead.
func (ai *AuthenticationInfo) Parse(input string) error {
	return ParseAuthenticationInfoInto(ai, input)
}

// ParseBytes parses Authentication-Info header parameters into ai.
//
// Deprecated: use [ParseAuthenticationInfoInto] instead.
func (ai *AuthenticationInfo) ParseBytes(input []byte) error {
	return ParseAuthenticationInfoInto(ai, input)
}

type digestCredentials struct {
	username      string
	password      string
	userHash      string
	method        string
	uri           string
	realm         string
	nonce         string
	algorithm     string
	sessAlgorithm bool
	cnonce        string
	opaque        string
	qop           string
	nc            int
	response      string
	bodyHash      string
}

func (dc *digestCredentials) parseQop(cha *Challenge) error {
	if len(cha.Qop) == 0 {
		return nil
	}

	if cha.isQopSupported(qopAuth) {
		dc.qop = qopAuth
		return nil
	}

	if cha.isQopSupported(qopAuthInt) {
		dc.qop = qopAuthInt
		return nil
	}

	return ErrDigestQopNotSupported
}

func (dc *digestCredentials) h(data string) string {
	h := newHashFunc(dc.algorithm)
	_, _ = h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// digest computes the cryptographic response digest (RFC 7616 §3.4.1).
func (dc *digestCredentials) digest(cha *Challenge) (string, error) {
	normAlg := generic.Coalesce(strings.ToUpper(strings.TrimSpace(dc.algorithm)), "MD5")

	if _, ok := digestHashFuncs[normAlg]; !ok {
		return "", ErrDigestAlgNotSupported
	}

	if err := dc.parseQop(cha); err != nil {
		return "", err
	}

	dc.nc++

	var b [16]byte

	_, _ = io.ReadFull(rand.Reader, b[:])
	dc.cnonce = hex.EncodeToString(b[:])

	ha1 := dc.ha1()
	ha2 := dc.ha2()

	var resp string
	switch dc.qop {
	case "":
		resp = ha1 + ":" + dc.nonce + ":" + ha2
	case qopAuth, qopAuthInt:
		resp = ha1 + ":" + dc.nonce + ":" + fmt.Sprintf("%08x", dc.nc) + ":" + dc.cnonce + ":" + dc.qop + ":" + ha2
	}

	dc.response = dc.h(resp)

	return "Digest " + dc.String(), nil
}

// ha1 computes the A1 hash (RFC 7616 §3.4.2).
func (dc *digestCredentials) ha1() string {
	a1 := dc.h(dc.username + ":" + dc.realm + ":" + dc.password)
	if dc.sessAlgorithm {
		return dc.h(a1 + ":" + dc.nonce + ":" + dc.cnonce)
	}

	return a1
}

// ha2 computes the A2 hash (RFC 7616 §3.4.3).
func (dc *digestCredentials) ha2() string {
	if dc.qop == qopAuthInt {
		return dc.h(dc.method + ":" + dc.uri + ":" + dc.bodyHash)
	}

	return dc.h(dc.method + ":" + dc.uri)
}

func (dc *digestCredentials) String() string {
	sl := make([]string, 0, 10)

	if dc.userHash == "true" {
		displayUsername := dc.h(dc.username + ":" + dc.realm)
		sl = append(sl, `username="`+displayUsername+`"`)
	} else {
		isASCII := true
		for _, r := range dc.username {
			if r > 127 || r == '"' || r == '\\' {
				isASCII = false
				break
			}
		}

		userStr := "username*=UTF-8''" + encodeRFC8187Simple(dc.username)
		if isASCII {
			userStr = `username="` + dc.username + `"`
		}

		sl = append(sl, userStr)
	}

	sl = append(sl, `realm="`+dc.realm+`"`)
	sl = append(sl, `nonce="`+dc.nonce+`"`)
	sl = append(sl, `uri="`+dc.uri+`"`)

	if dc.algorithm != "" {
		sl = append(sl, "algorithm="+dc.algorithm)
	}

	if dc.opaque != "" {
		sl = append(sl, `opaque="`+dc.opaque+`"`)
	}

	if dc.qop != "" {
		sl = append(sl, "qop="+dc.qop)
		sl = append(sl, fmt.Sprintf("nc=%08x", dc.nc))
		sl = append(sl, `cnonce="`+dc.cnonce+`"`)
	}

	if dc.userHash != "" {
		sl = append(sl, "userhash="+dc.userHash)
	}

	sl = append(sl, `response="`+dc.response+`"`)

	return strings.Join(sl, ", ")
}

func encodeRFC8187Simple(s string) string {
	const hexChars = "0123456789ABCDEF"

	var buf [256]byte

	b := buf[:0]

	for i := range len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b = append(b, c)
			continue
		}

		b = append(b, '%', hexChars[c>>4], hexChars[c&0x0F])
	}

	return string(b)
}

func newHashFunc(algorithm string) hash.Hash {
	normAlg := strings.ToUpper(strings.TrimSpace(algorithm))

	hf := digestHashFuncs[normAlg]
	if hf == nil {
		hf = md5.New
	}

	h := hf()
	h.Reset()

	return h
}

func stringOrBytes[S ~string | ~[]byte](s S) string {
	switch v := any(s).(type) {
	case string:
		return v
	case []byte:
		return bytesconv.B2S(v)
	default:
		return string(s)
	}
}

func findScheme(raw, scheme string) (int, bool) {
	sLen := len(scheme)
	for i := 0; i <= len(raw)-sLen; i++ {
		if (i == 0 || raw[i-1] == ' ' || raw[i-1] == ',' || raw[i-1] == '\t') &&
			strings.EqualFold(raw[i:i+sLen], scheme) {
			rest := raw[i+sLen:]
			if len(rest) == 0 || rest[0] == ' ' || rest[0] == '\t' || rest[0] == ',' || rest[0] == '=' {
				return i, true
			}
		}
	}

	return -1, false
}

func forEachAuthParam(raw, scheme string, yield func(key, val string) bool) bool {
	raw = strings.TrimSpace(raw)
	if scheme != "" {
		idx, found := findScheme(raw, scheme)
		if !found {
			return false
		}

		raw = strings.TrimSpace(raw[idx+len(scheme):])
	} else {
		trimmed := strings.TrimSpace(raw)
		if len(trimmed) >= 7 && strings.EqualFold(trimmed[:7], "Digest ") {
			raw = strings.TrimSpace(trimmed[7:])
		}
	}

	for len(raw) > 0 {
		raw = strings.TrimLeft(raw, " ,\t\r\n")
		if len(raw) == 0 {
			break
		}

		eqIdx := strings.IndexByte(raw, '=')
		if eqIdx < 0 {
			break
		}

		spaceIdx := strings.IndexByte(raw, ' ')
		if spaceIdx >= 0 && spaceIdx < eqIdx {
			break
		}

		key := strings.TrimSpace(raw[:eqIdx])
		raw = strings.TrimSpace(raw[eqIdx+1:])

		var val string
		if len(raw) > 0 && raw[0] == '"' {
			raw = raw[1:]

			quoteIdx := strings.IndexByte(raw, '"')
			if quoteIdx == -1 {
				val = raw
				raw = ""
			} else {
				hasEscape := false
				for i := 0; i < quoteIdx; i++ {
					if raw[i] == '\\' {
						hasEscape = true
						break
					}
				}

				if !hasEscape {
					val = raw[:quoteIdx]
					raw = raw[quoteIdx+1:]
				} else {
					var sb strings.Builder

					escaped := false

					foundQuote := false
					for i, c := range raw {
						if escaped {
							sb.WriteRune(c)

							escaped = false

							continue
						}

						if c == '\\' {
							escaped = true
							continue
						}

						if c == '"' {
							raw = raw[i+1:]
							foundQuote = true
							break
						}

						sb.WriteRune(c)
					}

					if !foundQuote {
						raw = ""
					}

					val = sb.String()
				}
			}
		} else {
			commaIdx := strings.IndexByte(raw, ',')
			spaceIdx2 := strings.IndexByte(raw, ' ')

			endIdx := commaIdx
			if spaceIdx2 >= 0 && (endIdx < 0 || spaceIdx2 < endIdx) {
				endIdx = spaceIdx2
			}

			if endIdx < 0 {
				val = strings.TrimSpace(raw)
				raw = ""
			} else {
				val = strings.TrimSpace(raw[:endIdx])
				raw = raw[endIdx:]
			}
		}

		if !yield(key, val) {
			return false
		}
	}

	return true
}
