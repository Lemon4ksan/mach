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

	"github.com/lemon4ksan/mach/proto/semantic/auth"
)

var (
	// ErrDigestBadChallenge is returned when the server responds with a malformed Digest WWW-Authenticate challenge.
	ErrDigestBadChallenge = errors.New("digest: bad challenge")

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
		c, parseErr := ParseChallenge(headerVal)
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

// ParseChallenge parses WWW-Authenticate challenge parameters (RFC 7616 §3.3).
func ParseChallenge(input string) (*Challenge, error) {
	params, ok := auth.ExtractChallengeParams(input, "Digest")
	if !ok {
		return nil, ErrDigestBadChallenge
	}

	c := &Challenge{}
	for key, val := range params {
		if err := c.setValue(key, val); err != nil {
			return nil, err
		}
	}

	return c, nil
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
}

func (dc *Challenge) isQopSupported(qop string) bool {
	for _, item := range dc.Qop {
		if strings.TrimSpace(item) == qop {
			return true
		}
	}
	return false
}

func (dc *Challenge) setValue(k, v string) error {
	switch k {
	case "realm":
		dc.Realm = v
	case "domain":
		dc.Domain = v
	case "nonce":
		dc.Nonce = v
	case "opaque":
		dc.Opaque = v
	case "stale":
		dc.Stale = v
	case "algorithm":
		dc.Algorithm = v
	case "qop":
		if v != "" {
			parts := strings.Split(v, ",")
			dc.Qop = make([]string, len(parts))
			for i, p := range parts {
				dc.Qop[i] = strings.TrimSpace(p)
			}
		}
	case "charset":
		if v != "" && strings.ToUpper(v) != "UTF-8" {
			return ErrDigestInvalidCharset
		}
	case "nc":
		nc, err := strconv.ParseInt(v, 16, 32)
		if err != nil {
			return fmt.Errorf("digest: invalid nc: %w", err)
		}
		dc.NC = int(nc)
	case "userhash":
		dc.UserHash = strings.ToLower(v)
	default:
		// RFC 7616 §3.3: Unknown parameters MUST be ignored
		return nil
	}
	return nil
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
	normAlg := strings.ToUpper(strings.TrimSpace(dc.algorithm))
	if normAlg == "" {
		normAlg = "MD5"
	}

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
