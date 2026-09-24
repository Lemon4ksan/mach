// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/lemon4ksan/mach/proto/http/status"
)

func TestAllStatusCodesMessages(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		// 1xx Informational
		{status.Continue, "Continue"},
		{status.SwitchingProtocols, "Switching Protocols"},
		{status.Processing, "Processing"},
		{status.EarlyHints, "Early Hints"},

		// 2xx Success
		{status.OK, "OK"},
		{status.Created, "Created"},
		{status.Accepted, "Accepted"},
		{status.NonAuthoritativeInfo, "Non-Authoritative Information"},
		{status.NoContent, "No Content"},
		{status.ResetContent, "Reset Content"},
		{status.PartialContent, "Partial Content"},
		{status.MultiStatus, "Multi-Status"},
		{status.AlreadyReported, "Already Reported"},
		{status.IMUsed, "IM Used"},

		// 3xx Redirection
		{status.MultipleChoices, "Multiple Choices"},
		{status.MovedPermanently, "Moved Permanently"},
		{status.Found, "Found"},
		{status.SeeOther, "See Other"},
		{status.NotModified, "Not Modified"},
		{status.UseProxy, "Use Proxy"},
		{status.TemporaryRedirect, "Temporary Redirect"},
		{status.PermanentRedirect, "Permanent Redirect"},

		// 4xx Client Error
		{status.BadRequest, "Bad Request"},
		{status.Unauthorized, "Unauthorized"},
		{status.PaymentRequired, "Payment Required"},
		{status.Forbidden, "Forbidden"},
		{status.NotFound, "Not Found"},
		{status.MethodNotAllowed, "Method Not Allowed"},
		{status.NotAcceptable, "Not Acceptable"},
		{status.ProxyAuthRequired, "Proxy Authentication Required"},
		{status.RequestTimeout, "Request Timeout"},
		{status.Conflict, "Conflict"},
		{status.Gone, "Gone"},
		{status.LengthRequired, "Length Required"},
		{status.PreconditionFailed, "Precondition Failed"},
		{status.RequestEntityTooLarge, "Request Entity Too Large"},
		{status.RequestURITooLong, "Request-URI Too Long"},
		{status.UnsupportedMediaType, "Unsupported Media Type"},
		{status.RequestedRangeNotSatisfiable, "Requested Range Not Satisfiable"},
		{status.ExpectationFailed, "Expectation Failed"},
		{status.Teapot, "I'm a teapot"},
		{status.MisdirectedRequest, "Misdirected Request"},
		{status.UnprocessableEntity, "Unprocessable Entity"},
		{status.Locked, "Locked"},
		{status.FailedDependency, "Failed Dependency"},
		{status.TooEarly, "Too Early"},
		{status.UpgradeRequired, "Upgrade Required"},
		{status.PreconditionRequired, "Precondition Required"},
		{status.TooManyRequests, "Too Many Requests"},
		{status.RequestHeaderFieldsTooLarge, "Request Header Fields Too Large"},
		{status.UnavailableForLegalReasons, "Unavailable For Legal Reasons"},

		// 5xx Server Error
		{status.InternalServerError, "Internal Server Error"},
		{status.NotImplemented, "Not Implemented"},
		{status.BadGateway, "Bad Gateway"},
		{status.ServiceUnavailable, "Service Unavailable"},
		{status.GatewayTimeout, "Gateway Timeout"},
		{status.HTTPVersionNotSupported, "HTTP Version Not Supported"},
		{status.VariantAlsoNegotiates, "Variant Also Negotiates"},
		{status.InsufficientStorage, "Insufficient Storage"},
		{status.LoopDetected, "Loop Detected"},
		{status.NotExtended, "Not Extended"},
		{status.NetworkAuthenticationRequired, "Network Authentication Required"},
	}

	for _, tt := range tests {
		got := status.Message(tt.code)
		if got != tt.want {
			t.Errorf("status.Message(%d) = %q; want %q", tt.code, got, tt.want)
		}
	}
}

func TestStatusMessageMatchesNetHTTP(t *testing.T) {
	for code := 100; code <= 511; code++ {
		expected := http.StatusText(code)
		if expected != "" {
			got := status.Message(code)
			// Status 414 is standard "URI Too Long" in Go stdlib vs RFC 7231 "Request-URI Too Long"
			if code != status.RequestURITooLong && got != expected {
				t.Errorf("status.Message(%d) = %q, want %q", code, got, expected)
			}
		}
	}
}

func TestUnknownStatusCodes(t *testing.T) {
	unknownCodes := []int{
		-200, -1, 0, 1, 9, 42, 99,
		104, 209, 306, 309, 419, 499,
		509, 512, 599, 600, 999, 1000,
	}

	const want = "Unknown Status Code"
	for _, code := range unknownCodes {
		got := status.Message(code)
		if got != want {
			t.Errorf("status.Message(%d) = %q; want %q", code, got, want)
		}
	}
}

func TestPredicates(t *testing.T) {
	tests := []struct {
		code          int
		isInfo        bool
		isSuccess     bool
		isRedirect    bool
		isClientError bool
		isServerError bool
	}{
		{-1, false, false, false, false, false},
		{0, false, false, false, false, false},
		{99, false, false, false, false, false},
		{100, true, false, false, false, false},
		{101, true, false, false, false, false},
		{103, true, false, false, false, false},
		{199, true, false, false, false, false},
		{200, false, true, false, false, false},
		{201, false, true, false, false, false},
		{204, false, true, false, false, false},
		{299, false, true, false, false, false},
		{300, false, false, true, false, false},
		{301, false, false, true, false, false},
		{304, false, false, true, false, false},
		{307, false, false, true, false, false},
		{399, false, false, true, false, false},
		{400, false, false, false, true, false},
		{404, false, false, false, true, false},
		{418, false, false, false, true, false},
		{451, false, false, false, true, false},
		{499, false, false, false, true, false},
		{500, false, false, false, false, true},
		{502, false, false, false, false, true},
		{511, false, false, false, false, true},
		{599, false, false, false, false, true},
		{600, false, false, false, false, false},
		{700, false, false, false, false, false},
		{1000, false, false, false, false, false},
	}

	for _, tt := range tests {
		if got := status.IsInformational(tt.code); got != tt.isInfo {
			t.Errorf("IsInformational(%d) = %v; want %v", tt.code, got, tt.isInfo)
		}
		if got := status.IsSuccess(tt.code); got != tt.isSuccess {
			t.Errorf("IsSuccess(%d) = %v; want %v", tt.code, got, tt.isSuccess)
		}
		if got := status.IsRedirect(tt.code); got != tt.isRedirect {
			t.Errorf("IsRedirect(%d) = %v; want %v", tt.code, got, tt.isRedirect)
		}
		if got := status.IsClientError(tt.code); got != tt.isClientError {
			t.Errorf("IsClientError(%d) = %v; want %v", tt.code, got, tt.isClientError)
		}
		if got := status.IsServerError(tt.code); got != tt.isServerError {
			t.Errorf("IsServerError(%d) = %v; want %v", tt.code, got, tt.isServerError)
		}
	}
}

func TestFormatLine(t *testing.T) {
	protocol := []byte("HTTP/1.1")

	// Standard 3-digit status code with nil status text
	line := status.FormatLine(nil, protocol, status.OK, nil)
	expected := "HTTP/1.1 200 OK\r\n"
	if !bytes.Equal(line, []byte(expected)) {
		t.Fatalf("FormatLine = %q, want %q", string(line), expected)
	}

	// Standard 3-digit status code with custom text
	customText := []byte("Custom Status")
	line2 := status.FormatLine(nil, protocol, 299, customText)
	expected2 := "HTTP/1.1 299 Custom Status\r\n"
	if !bytes.Equal(line2, []byte(expected2)) {
		t.Fatalf("FormatLine custom = %q, want %q", string(line2), expected2)
	}

	// Single digit status code (code < 10)
	line3 := status.FormatLine(nil, protocol, 7, []byte("Single"))
	expected3 := "HTTP/1.1 7 Single\r\n"
	if !bytes.Equal(line3, []byte(expected3)) {
		t.Fatalf("FormatLine code=7 = %q, want %q", string(line3), expected3)
	}

	// Two digit status code (code < 100)
	line4 := status.FormatLine(nil, protocol, 42, []byte("Two"))
	expected4 := "HTTP/1.1 42 Two\r\n"
	if !bytes.Equal(line4, []byte(expected4)) {
		t.Fatalf("FormatLine code=42 = %q, want %q", string(line4), expected4)
	}

	// Four digit status code (code >= 1000)
	line5 := status.FormatLine(nil, protocol, 1000, []byte("Four"))
	expected5 := "HTTP/1.1 1000 Four\r\n"
	if !bytes.Equal(line5, []byte(expected5)) {
		t.Fatalf("FormatLine code=1000 = %q, want %q", string(line5), expected5)
	}

	// Negative status code (code < 0)
	line6 := status.FormatLine(nil, protocol, -200, []byte("Negative"))
	expected6 := "HTTP/1.1 -200 Negative\r\n"
	if !bytes.Equal(line6, []byte(expected6)) {
		t.Fatalf("FormatLine code=-200 = %q, want %q", string(line6), expected6)
	}

	// Small negative status code (single digit magnitude)
	line6b := status.FormatLine(nil, protocol, -5, []byte("NegSingle"))
	expected6b := "HTTP/1.1 -5 NegSingle\r\n"
	if !bytes.Equal(line6b, []byte(expected6b)) {
		t.Fatalf("FormatLine code=-5 = %q, want %q", string(line6b), expected6b)
	}

	// Appending to existing buffer with sufficient capacity
	buf := make([]byte, 0, 64)
	buf = append(buf, "HEADER: "...)
	line7 := status.FormatLine(buf, protocol, status.NotFound, nil)
	expected7 := "HEADER: HTTP/1.1 404 Not Found\r\n"
	if !bytes.Equal(line7, []byte(expected7)) {
		t.Fatalf("FormatLine append with cap = %q, want %q", string(line7), expected7)
	}

	// Appending to existing buffer with insufficient capacity (triggers cap(dst)-len(dst) < need)
	bufShort := make([]byte, 5, 6)
	copy(bufShort, "start")
	line8 := status.FormatLine(bufShort, protocol, status.InternalServerError, nil)
	expected8 := "startHTTP/1.1 500 Internal Server Error\r\n"
	if !bytes.Equal(line8, []byte(expected8)) {
		t.Fatalf("FormatLine append realloc = %q, want %q", string(line8), expected8)
	}
}

func BenchmarkMessage(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = status.Message(status.OK)
		_ = status.Message(status.NotFound)
		_ = status.Message(status.InternalServerError)
	}
}

func BenchmarkFormatLine(b *testing.B) {
	protocol := []byte("HTTP/1.1")
	dst := make([]byte, 0, 64)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = status.FormatLine(dst[:0], protocol, status.OK, nil)
	}
}

func BenchmarkPredicates(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = status.IsInformational(100)
		_ = status.IsSuccess(200)
		_ = status.IsRedirect(301)
		_ = status.IsClientError(404)
		_ = status.IsServerError(500)
	}
}
