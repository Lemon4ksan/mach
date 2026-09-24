// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3

import (
	"fmt"
	"sync"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/mach/qpack"
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

// Is reports whether the receiver matches target under errors.Is semantics (RFC 9204 §6).
//
// Specifically, it matches ErrQPACKDecompressFailed when e.Code equals
// ErrCodeQpackDecompressionFailed (RFC 9114 §4.2, RFC 9204 §8.3).
// Concurrency: Safe for concurrent invocation across goroutines.
func (e *QPACKStreamError) Is(target error) bool {
	if e == nil {
		return false
	}

	if target == ErrQPACKDecompressFailed && e.Code == ErrCodeQpackDecompressionFailed {
		return true
	}

	return false
}

// QPACKCodec manages QPACK header serialization and deserialization with dual mutex synchronization (RFC 9204).
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

// NewQPACKCodec instantiates a thread-safe, panic-free QPACKCodec with defaults (RFC 9204).
func NewQPACKCodec() *QPACKCodec {
	return NewQPACKCodecWithOptions(4096, 100, nil)
}

// NewQPACKCodecWithOptions constructs a QPACKCodec with custom capacity and error callback (RFC 9204).
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

// Decoder returns the underlying QPACK progressive decoder instance (RFC 9204 §3).
//
// Callers may use the decoder to inspect dynamic table state or create progressive decoders.
// Concurrency: State inspection is thread-safe; progressive decoders created from this instance
// must be synchronized or managed per-stream.
func (q *QPACKCodec) Decoder() *qpack.Decoder {
	return q.decoder
}

// Encoder returns the underlying QPACK encoder instance (RFC 9204 §4).
//
// Callers may use the encoder to inspect header table capacity or encode raw header lists.
// Concurrency: Concurrent encoding operations must be synchronized via the codec's encoder mutex.
func (q *QPACKCodec) Encoder() *qpack.Encoder {
	return q.encoder
}
