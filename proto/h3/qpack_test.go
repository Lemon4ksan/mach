// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/mach/qpack"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	h1 "github.com/lemon4ksan/mach/proto/http"
)

func TestQPACKEncodeRequestHeaders(t *testing.T) {
	t.Parallel()

	codec := NewQPACKCodec()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://api.example.com/v1/data")
	req.Header.Set("User-Agent", "aoni-h3-test")
	req.Header.Set("Content-Type", "application/json")

	var buf bytes.Buffer

	if err := codec.EncodeRequestHeaders(0, &buf, req, nil); err != nil {
		t.Fatalf("EncodeRequestHeaders failed: %v", err)
	}

	decoder := codec.Decoder()
	handler := &testHeadersHandler{}
	prog := decoder.CreateProgressiveDecoder(0, handler)
	prog.Decode(buf.Bytes())
	prog.EndHeaderBlock()

	if handler.err != nil {
		t.Fatalf("qpack decode failed: %v", handler.err)
	}

	decodedMap := make(map[string]string)
	for _, hf := range handler.headers {
		decodedMap[hf.Name] = hf.Value
	}

	if decodedMap[":method"] != "POST" {
		t.Errorf("got :method %q, want POST", decodedMap[":method"])
	}

	if decodedMap[":scheme"] != "https" {
		t.Errorf("got :scheme %q, want https", decodedMap[":scheme"])
	}

	if decodedMap[":authority"] != "api.example.com" {
		t.Errorf("got :authority %q, want api.example.com", decodedMap[":authority"])
	}

	if decodedMap[":path"] != "/v1/data" {
		t.Errorf("got :path %q, want /v1/data", decodedMap[":path"])
	}

	if decodedMap["user-agent"] != "aoni-h3-test" {
		t.Errorf("got user-agent %q, want aoni-h3-test", decodedMap["user-agent"])
	}
}

func TestQPACKOrderedHeadersSequence(t *testing.T) {
	t.Parallel()

	codec := NewQPACKCodec()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://example.com/test")

	req.Header.Set("x-header-c", "val-c")
	req.Header.Set("x-header-a", "val-a")
	req.Header.Set("x-header-b", "val-b")

	orderedKeys := []string{"x-header-a", "x-header-b", "x-header-c"}

	var buf bytes.Buffer

	if err := codec.EncodeRequestHeaders(0, &buf, req, orderedKeys); err != nil {
		t.Fatalf("EncodeRequestHeaders failed: %v", err)
	}

	decoder := codec.Decoder()
	handler := &testHeadersHandler{}
	prog := decoder.CreateProgressiveDecoder(0, handler)
	prog.Decode(buf.Bytes())
	prog.EndHeaderBlock()

	if handler.err != nil {
		t.Fatalf("qpack decode failed: %v", handler.err)
	}

	var capturedKeys []string
	for _, hf := range handler.headers {
		if !hf.IsPseudo() {
			capturedKeys = append(capturedKeys, hf.Name)
		}
	}

	if len(capturedKeys) < 3 {
		t.Fatalf("expected at least 3 regular headers, got %d", len(capturedKeys))
	}

	if capturedKeys[0] != "x-header-a" || capturedKeys[1] != "x-header-b" || capturedKeys[2] != "x-header-c" {
		t.Fatalf("QPACK header order sequence violated: got %v, want %v", capturedKeys, orderedKeys)
	}
}

func TestQPACKDecodeResponseHeaders(t *testing.T) {
	t.Parallel()

	codec := NewQPACKCodec()

	var buf bytes.Buffer

	headers := []qpack.HeaderField{
		{Name: ":status", Value: "201"},
		{Name: "content-type", Value: "application/json"},
		{Name: "content-length", Value: "128"},
	}
	block := codec.Encoder().EncodeHeaderList(0, headers, nil)
	buf.Write(block)

	var respHeader h1.ResponseHeader

	if _, err := codec.DecodeResponseHeaders(0, buf.Bytes(), &respHeader); err != nil {
		t.Fatalf("DecodeResponseHeaders failed: %v", err)
	}

	if respHeader.StatusCode() != 201 {
		t.Errorf("got status code %d, want 201", respHeader.StatusCode())
	}

	if string(respHeader.Peek("Content-Type")) != "application/json" {
		t.Errorf("got content-type %q, want application/json", respHeader.Peek("Content-Type"))
	}

	if respHeader.ContentLength() != 128 {
		t.Errorf("got content-length %d, want 128", respHeader.ContentLength())
	}
}

func TestQPACKDecodeResponseMissingStatus(t *testing.T) {
	t.Parallel()

	codec := NewQPACKCodec()

	var buf bytes.Buffer

	headers := []qpack.HeaderField{
		{Name: "content-type", Value: "text/plain"},
	}
	block := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)
	buf.Write(block)

	var respHeader h1.ResponseHeader

	_, err := codec.DecodeResponseHeaders(0, buf.Bytes(), &respHeader)
	if !errors.Is(err, ErrMissingStatusHeader) {
		t.Fatalf("expected ErrMissingStatusHeader, got %v", err)
	}
}

func TestQPACKEncodeExtendedCONNECTProtocolHeader(t *testing.T) {
	t.Parallel()

	codec := NewQPACKCodec()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)

	req.Header.SetMethod("CONNECT")
	req.SetRequestURI("https://example.com/ws")
	req.Header.Set(":protocol", "websocket")
	req.Header.Set("Sec-WebSocket-Protocol", "chat.v1")

	var buf bytes.Buffer

	err := codec.EncodeRequestHeaders(0, &buf, req, nil)
	require.NoError(t, err)

	decoder := qpack.NewDecoder(4096, 100, func(code uint64, msg string) {})
	handler := &testHeadersHandler{}
	prog := decoder.CreateProgressiveDecoder(0, handler)
	prog.Decode(buf.Bytes())
	prog.EndHeaderBlock()

	decodeFn := func() (qpack.HeaderField, error) {
		if len(handler.headers) == 0 {
			return qpack.HeaderField{}, io.EOF
		}

		hf := handler.headers[0]
		handler.headers = handler.headers[1:]

		return hf, nil
	}

	decodedMap := make(map[string]string)
	for {
		hf, err := decodeFn()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		decodedMap[hf.Name] = hf.Value
	}

	assert.Equal(t, "CONNECT", decodedMap[":method"])
	assert.Equal(t, "websocket", decodedMap[":protocol"])
	assert.Equal(t, "chat.v1", decodedMap["sec-websocket-protocol"])
}

func TestIsForbiddenH3Header(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"connection",
		"keep-alive",
		"proxy-connection",
		"transfer-encoding",
		"upgrade",
		"sec-websocket-key",
		"sec-websocket-accept",
		"CONNECTION",
		"Sec-WebSocket-Key",
	}

	for _, h := range forbidden {
		assert.Truef(
			t,
			isForbiddenH3Header([]byte(h), []byte("val")),
			"isForbiddenH3Header byte slice should return true for %s",
			h,
		)
		assert.Truef(
			t,
			isForbiddenH3HeaderStr(h, []byte("val")),
			"isForbiddenH3HeaderStr string should return true for %s",
			h,
		)
	}

	allowed := []string{
		"authorization",
		"user-agent",
		"sec-websocket-protocol",
		"sec-websocket-version",
		"x-custom-header",
	}

	for _, h := range allowed {
		assert.Falsef(
			t,
			isForbiddenH3Header([]byte(h), []byte("val")),
			"isForbiddenH3Header byte slice should return false for %s",
			h,
		)
		assert.Falsef(
			t,
			isForbiddenH3HeaderStr(h, []byte("val")),
			"isForbiddenH3HeaderStr string should return false for %s",
			h,
		)
	}
}

func TestQPACKForbiddenHeadersFilteringInEncode(t *testing.T) {
	t.Parallel()

	codec := NewQPACKCodec()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://example.com/ws")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Accept", "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=")
	req.Header.Set("User-Agent", "aoni-test")

	var buf bytes.Buffer

	err := codec.EncodeRequestHeaders(0, &buf, req, nil)
	require.NoError(t, err)

	decoder := qpack.NewDecoder(4096, 100, func(code uint64, msg string) {})
	handler := &testHeadersHandler{}
	prog := decoder.CreateProgressiveDecoder(0, handler)
	prog.Decode(buf.Bytes())
	prog.EndHeaderBlock()

	decodeFn := func() (qpack.HeaderField, error) {
		if len(handler.headers) == 0 {
			return qpack.HeaderField{}, io.EOF
		}

		hf := handler.headers[0]
		handler.headers = handler.headers[1:]

		return hf, nil
	}

	decodedMap := make(map[string]string)
	for {
		hf, err := decodeFn()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		decodedMap[hf.Name] = hf.Value
	}

	assert.Equal(t, "aoni-test", decodedMap["user-agent"])
	_, hasUpgrade := decodedMap["upgrade"]
	assert.False(t, hasUpgrade, "upgrade header should be filtered out")

	_, hasConn := decodedMap["connection"]
	assert.False(t, hasConn, "connection header should be filtered out")

	_, hasKey := decodedMap["sec-websocket-key"]
	assert.False(t, hasKey, "sec-websocket-key header should be filtered out")

	_, hasAccept := decodedMap["sec-websocket-accept"]
	assert.False(t, hasAccept, "sec-websocket-accept header should be filtered out")
}

func TestRFC9204AppendixBExamples(t *testing.T) {
	t.Parallel()

	// RFC 9204 Appendix B.1: Literal Field Line with Static Name Reference
	// Static table index 0 is ":authority".
	// Encoded field section prefix: Required Insert Count = 0, Sign = 0, Delta Base = 0 -> 0x00, 0x00
	// Literal with static name reference (RFC 9204 §4.5.4): '01' | 'N'=0 | 'T'=1 | Index=0 -> 0x50
	// String literal for value "www.example.com" without Huffman: length 15 (0x0f), "www.example.com"
	rawBlock := []byte{
		0x00, 0x00, // Prefix: RIC=0, Base=0
		0x50, // 0101 0000: Literal with static name ref (index 0 = :authority)
		0x0f, // Length 15
		'w', 'w', 'w', '.', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm',
		0xd1, // 1101 0001: Indexed static field line (RFC 9204 §4.5.2): T=1, index 17 = ":method: GET"
		0xd7, // 1101 0111: Indexed static field line (RFC 9204 §4.5.2): T=1, index 23 = ":scheme: https"
	}

	decoder := qpack.NewDecoder(4096, 100, func(code uint64, msg string) {})
	handler := &testHeadersHandler{}
	prog := decoder.CreateProgressiveDecoder(0, handler)
	prog.Decode(rawBlock)
	prog.EndHeaderBlock()

	decodeFn := func() (qpack.HeaderField, error) {
		if len(handler.headers) == 0 {
			return qpack.HeaderField{}, io.EOF
		}

		hf := handler.headers[0]
		handler.headers = handler.headers[1:]

		return hf, nil
	}

	fields := make(map[string]string)
	for {
		hf, err := decodeFn()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		fields[hf.Name] = hf.Value
	}

	assert.Equal(t, "www.example.com", fields[":authority"])
	assert.Equal(t, "GET", fields[":method"])
	assert.Equal(t, "https", fields[":scheme"])
}

func BenchmarkQPACKEncodeRequestHeaders(b *testing.B) {
	codec := NewQPACKCodec()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)

	req.Header.SetMethod("CONNECT")
	req.SetRequestURI("https://example.com/ws")
	req.Header.Set(":protocol", "websocket")
	req.Header.Set("User-Agent", "aoni-h3-bench")
	req.Header.Set("Sec-WebSocket-Protocol", "chat.v1")

	var buf bytes.Buffer

	b.ReportAllocs()

	for b.Loop() {
		buf.Reset()
		_ = codec.EncodeRequestHeaders(0, &buf, req, nil)
	}
}

func BenchmarkQPACKDecodeResponseHeaders(b *testing.B) {
	codec := NewQPACKCodec()

	var buf bytes.Buffer

	headers := []qpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "sec-websocket-version", Value: "13"},
		{Name: "sec-websocket-protocol", Value: "chat.v1"},
	}
	block := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)
	buf.Write(block)
	encoded := buf.Bytes()

	var respHeader h1.ResponseHeader

	b.ReportAllocs()

	for b.Loop() {
		respHeader.Reset()
		_, _ = codec.DecodeResponseHeaders(0, encoded, &respHeader)
	}
}

type testHeadersHandler struct {
	headers []qpack.HeaderField
	err     error
}

func (h *testHeadersHandler) OnHeaderDecoded(name, value string) {
	h.headers = append(h.headers, qpack.HeaderField{Name: name, Value: value})
}
func (h *testHeadersHandler) OnDecodingCompleted() {}
func (h *testHeadersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.err = errors.New(errorMessage)
}
