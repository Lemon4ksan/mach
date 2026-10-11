// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package status provides backward-compatible forwarders to RFC 9110 HTTP status codes,
// reason phrases, and status utilities defined in proto/semantic/status.
package status

import (
	"github.com/lemon4ksan/mach/proto/semantic/status"
)

// HTTP status codes forwarded from proto/semantic/status.
const (
	Continue           = status.Continue
	SwitchingProtocols = status.SwitchingProtocols
	Processing         = status.Processing
	EarlyHints         = status.EarlyHints

	OK                   = status.OK
	Created              = status.Created
	Accepted             = status.Accepted
	NonAuthoritativeInfo = status.NonAuthoritativeInfo
	NoContent            = status.NoContent
	ResetContent         = status.ResetContent
	PartialContent       = status.PartialContent
	MultiStatus          = status.MultiStatus
	AlreadyReported      = status.AlreadyReported
	IMUsed               = status.IMUsed

	MultipleChoices   = status.MultipleChoices
	MovedPermanently  = status.MovedPermanently
	Found             = status.Found
	SeeOther          = status.SeeOther
	NotModified       = status.NotModified
	UseProxy          = status.UseProxy
	TemporaryRedirect = status.TemporaryRedirect
	PermanentRedirect = status.PermanentRedirect

	BadRequest                   = status.BadRequest
	Unauthorized                 = status.Unauthorized
	PaymentRequired              = status.PaymentRequired
	Forbidden                    = status.Forbidden
	NotFound                     = status.NotFound
	MethodNotAllowed             = status.MethodNotAllowed
	NotAcceptable                = status.NotAcceptable
	ProxyAuthRequired            = status.ProxyAuthRequired
	RequestTimeout               = status.RequestTimeout
	Conflict                     = status.Conflict
	Gone                         = status.Gone
	LengthRequired               = status.LengthRequired
	PreconditionFailed           = status.PreconditionFailed
	RequestEntityTooLarge        = status.RequestEntityTooLarge
	RequestURITooLong            = status.RequestURITooLong
	UnsupportedMediaType         = status.UnsupportedMediaType
	RequestedRangeNotSatisfiable = status.RequestedRangeNotSatisfiable
	ExpectationFailed            = status.ExpectationFailed
	Teapot                       = status.Teapot
	MisdirectedRequest           = status.MisdirectedRequest
	UnprocessableEntity          = status.UnprocessableEntity
	Locked                       = status.Locked
	FailedDependency             = status.FailedDependency
	TooEarly                     = status.TooEarly
	UpgradeRequired              = status.UpgradeRequired
	PreconditionRequired         = status.PreconditionRequired
	TooManyRequests              = status.TooManyRequests
	RequestHeaderFieldsTooLarge  = status.RequestHeaderFieldsTooLarge
	UnavailableForLegalReasons   = status.UnavailableForLegalReasons

	InternalServerError           = status.InternalServerError
	NotImplemented                = status.NotImplemented
	BadGateway                    = status.BadGateway
	ServiceUnavailable            = status.ServiceUnavailable
	GatewayTimeout                = status.GatewayTimeout
	HTTPVersionNotSupported       = status.HTTPVersionNotSupported
	VariantAlsoNegotiates         = status.VariantAlsoNegotiates
	InsufficientStorage           = status.InsufficientStorage
	LoopDetected                  = status.LoopDetected
	NotExtended                   = status.NotExtended
	NetworkAuthenticationRequired = status.NetworkAuthenticationRequired
)

// Message returns HTTP status message for the given status code.
func Message(statusCode int) string {
	return status.Message(statusCode)
}

// IsInformational reports whether the status code is informational (1xx).
func IsInformational(statusCode int) bool {
	return status.IsInformational(statusCode)
}

// IsSuccess reports whether the status code indicates success (2xx).
func IsSuccess(statusCode int) bool {
	return status.IsSuccess(statusCode)
}

// IsRedirect reports whether the status code indicates redirection (3xx).
func IsRedirect(statusCode int) bool {
	return status.IsRedirect(statusCode)
}

// IsClientError reports whether the status code indicates a client error (4xx).
func IsClientError(statusCode int) bool {
	return status.IsClientError(statusCode)
}

// IsServerError reports whether the status code indicates a server error (5xx).
func IsServerError(statusCode int) bool {
	return status.IsServerError(statusCode)
}

// FormatLine appends formatted HTTP status line to dst.
func FormatLine(dst, protocol []byte, statusCode int, statusText []byte) []byte {
	return status.FormatLine(dst, protocol, statusCode, statusText)
}
