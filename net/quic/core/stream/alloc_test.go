// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"testing"
)

func TestAlloc_ReassemblyBuffer_ContiguousRead(t *testing.T) {
	rb := NewReassemblyBuffer()
	payload := make([]byte, 1024*1024)
	_ = rb.Push(0, payload, true)

	readBuf := make([]byte, 4096)

	// In-order contiguous reading from pre-buffered data
	allocs := testing.AllocsPerRun(500, func() {
		_, _ = rb.Read(readBuf)
	})

	t.Logf("ReassemblyBuffer contiguous Read allocs per run: %.2f", allocs)
	if allocs > 0 {
		t.Errorf("expected 0 allocs on contiguous Read fast path, got %.2f", allocs)
	}
}

func TestAlloc_ReassemblyBuffer_Peek(t *testing.T) {
	rb := NewReassemblyBuffer()
	payload := make([]byte, 4096)
	_ = rb.Push(0, payload, false)

	peekBuf := make([]byte, 512)

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = rb.Peek(peekBuf)
	})

	t.Logf("ReassemblyBuffer Peek allocs per run: %.2f", allocs)
	if allocs > 0 {
		t.Errorf("expected 0 allocs on Peek fast path, got %.2f", allocs)
	}
}

func TestAlloc_FlowController_CreditCheck(t *testing.T) {
	cfc := NewConnectionFlowController(1024*1024, 2*1024*1024, 1024*1024, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 1024*1024, 2*1024*1024, 1024*1024, nil, nil)

	allocs := testing.AllocsPerRun(1000, func() {
		_, _, _ = sfc.SendCreditAvailable()
	})

	t.Logf("StreamFlowController.SendCreditAvailable allocs per run: %.2f", allocs)
	if allocs > 0 {
		t.Errorf("expected 0 allocs on SendCreditAvailable, got %.2f", allocs)
	}
}

func TestAlloc_SendStream_TryWriteAll(t *testing.T) {
	cfc := NewConnectionFlowController(10*1024*1024, 20*1024*1024, 10*1024*1024, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 10*1024*1024, 20*1024*1024, 10*1024*1024, nil, nil)
	ss := NewSendStream(context.Background(), 0, nil, sfc)

	payload := make([]byte, 64)

	allocs := testing.AllocsPerRun(200, func() {
		_ = ss.TryWriteAll(payload)
	})

	t.Logf("SendStream.TryWriteAll allocs per run: %.2f", allocs)
	// TryWriteAll only appends to existing slice buffer; amortized close to zero
	if allocs > 1.0 {
		t.Errorf("expected <= 1 allocs on TryWriteAll, got %.2f", allocs)
	}
}
