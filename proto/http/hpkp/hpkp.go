// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpkp

import (
	"crypto/x509"
	"net/http"
	"time"

	"github.com/lemon4ksan/mach/proto/semantic/hpkp"
)

// Header field names forwarded from proto/semantic/hpkp.
const (
	HeaderPublicKeyPins           = hpkp.HeaderPublicKeyPins
	HeaderPublicKeyPinsReportOnly = hpkp.HeaderPublicKeyPinsReportOnly
)

// Directives forwarded from proto/semantic/hpkp.
const (
	DirectiveMaxAge            = hpkp.DirectiveMaxAge
	DirectiveIncludeSubDomains = hpkp.DirectiveIncludeSubDomains
	DirectiveReportURI         = hpkp.DirectiveReportURI
	PinPrefixSHA256            = hpkp.PinPrefixSHA256
)

// Common errors forwarded from proto/semantic/hpkp.
var (
	ErrEmptyPinningHeader = hpkp.ErrEmptyPinningHeader
	ErrMissingMaxAge      = hpkp.ErrMissingMaxAge
	ErrMissingPins        = hpkp.ErrMissingPins
	ErrNoMatchingPin      = hpkp.ErrNoMatchingPin
	ErrNoBackupPin        = hpkp.ErrNoBackupPin
	ErrInvalidPinFormat   = hpkp.ErrInvalidPinFormat
	ErrNoCertificates     = hpkp.ErrNoCertificates
)

// Types forwarded from proto/semantic/hpkp.
type (
	Pin              = hpkp.Pin
	Policy           = hpkp.Policy
	ValidationReport = hpkp.ValidationReport
)

var (
	AcquirePolicy = hpkp.AcquirePolicy
	ReleasePolicy = hpkp.ReleasePolicy
)

// NewPinSHA256 forwarded from proto/semantic/hpkp.
func NewPinSHA256(fingerprint string) (Pin, error) {
	return hpkp.NewPinSHA256(fingerprint)
}

// ComputeSPKIFingerprint forwarded from proto/semantic/hpkp.
func ComputeSPKIFingerprint(cert *x509.Certificate) string {
	return hpkp.ComputeSPKIFingerprint(cert)
}

// ComputeSPKIPin forwarded from proto/semantic/hpkp.
func ComputeSPKIPin(cert *x509.Certificate) string {
	return hpkp.ComputeSPKIPin(cert)
}

// ComputeSPKIFingerprintFromDER forwarded from proto/semantic/hpkp.
func ComputeSPKIFingerprintFromDER(rawCert []byte) (string, error) {
	return hpkp.ComputeSPKIFingerprintFromDER(rawCert)
}

// ComputeSPKIFingerprintFromSPKI forwarded from proto/semantic/hpkp.
func ComputeSPKIFingerprintFromSPKI(rawSPKI []byte) string {
	return hpkp.ComputeSPKIFingerprintFromSPKI(rawSPKI)
}

// AppendSPKIFingerprint forwarded from proto/semantic/hpkp.
func AppendSPKIFingerprint(dst []byte, rawSPKI []byte) []byte {
	return hpkp.AppendSPKIFingerprint(dst, rawSPKI)
}

// ParseHeader forwarded from proto/semantic/hpkp.
func ParseHeader(headerValue string) (*Policy, error) {
	return hpkp.ParseHeader(headerValue)
}

// FromResponse forwarded from proto/semantic/hpkp.
func FromResponse(resp *http.Response) (*Policy, error) {
	return hpkp.FromResponse(resp)
}

// FromHeader forwarded from proto/semantic/hpkp.
func FromHeader(header http.Header) (*Policy, error) {
	return hpkp.FromHeader(header)
}

// BuildValidationReport forwarded from proto/semantic/hpkp.
func BuildValidationReport(
	hostname string,
	port int,
	notedHostname string,
	policy *Policy,
	servedChain []*x509.Certificate,
	validatedChain []*x509.Certificate,
	observedAt time.Time,
) *ValidationReport {
	return hpkp.BuildValidationReport(hostname, port, notedHostname, policy, servedChain, validatedChain, observedAt)
}
