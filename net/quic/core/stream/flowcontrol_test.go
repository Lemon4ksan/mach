// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"errors"
	"testing"
	"time"
)

func TestStreamFlowController_CreditEnforcement(t *testing.T) {
	cfc := NewConnectionFlowController(10000, 20000, 10000, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 1000, 5000, 1000, nil, nil)

	_, _, effective := sfc.SendCreditAvailable()
	if effective != 1000 {
		t.Fatalf("expected 1000 credit, got %d", effective)
	}

	// Consume 600 bytes
	sfc.AddBytesSent(600)
	_, _, effective = sfc.SendCreditAvailable()
	if effective != 400 {
		t.Fatalf("expected 400 credit, got %d", effective)
	}

	// Consume remaining 400 bytes
	sfc.AddBytesSent(400)
	_, _, effective = sfc.SendCreditAvailable()
	if effective != 0 {
		t.Fatalf("expected 0 credit, got %d", effective)
	}

	// Verify blocked signaling (RFC 9000 §4.1)
	offset, shouldSend := sfc.CheckAndQueueStreamDataBlocked()
	if !shouldSend || offset != 1000 {
		t.Fatalf("expected stream data blocked signal at offset 1000, got (%d, %v)", offset, shouldSend)
	}

	// Subsequent check before window update should not duplicate signal
	_, shouldSendAgain := sfc.CheckAndQueueStreamDataBlocked()
	if shouldSendAgain {
		t.Fatalf("should not duplicate STREAM_DATA_BLOCKED without window change")
	}

	// Waiter registration and unblocking
	waiter := sfc.RegisterSendWaiter()
	select {
	case <-waiter:
		t.Fatal("waiter should not be triggered before window update")
	default:
	}

	// Update window to 2500
	sfc.UpdateSendWindow(2500)

	select {
	case <-waiter:
		// success: unblocked
	case <-time.After(100 * time.Millisecond):
		t.Fatal("waiter was not unblocked on send window update")
	}

	_, _, effective = sfc.SendCreditAvailable()
	if effective != 1500 { // 2500 - 1000 sent = 1500
		t.Fatalf("expected 1500 credit after update, got %d", effective)
	}
}

func TestConnectionFlowController_CreditEnforcement(t *testing.T) {
	// Connection limit is smaller than stream limit
	cfc := NewConnectionFlowController(10000, 20000, 500, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 10000, 20000, 2000, nil, nil)

	streamCredit, connCredit, effective := sfc.SendCreditAvailable()
	if streamCredit != 2000 || connCredit != 500 || effective != 500 {
		t.Fatalf("unexpected credits: stream=%d, conn=%d, effective=%d", streamCredit, connCredit, effective)
	}

	// Send 500 bytes: exhausting connection credit
	sfc.AddBytesSent(500)
	_, connCredit, effective = sfc.SendCreditAvailable()
	if connCredit != 0 || effective != 0 {
		t.Fatalf("connection credit should be 0, got %d", effective)
	}

	offset, shouldSend := cfc.CheckAndQueueDataBlocked()
	if !shouldSend || offset != 500 {
		t.Fatalf("expected DATA_BLOCKED at 500, got (%d, %v)", offset, shouldSend)
	}

	waiter := cfc.RegisterSendWaiter()
	cfc.UpdateSendWindow(1500)

	select {
	case <-waiter:
		// success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("conn waiter not unblocked")
	}

	_, _, effective = sfc.SendCreditAvailable()
	if effective != 1000 { // conn: 1500 - 500 = 1000; stream: 2000 - 500 = 1500; min is 1000
		t.Fatalf("expected effective 1000, got %d", effective)
	}
}

func TestFlowController_Violation(t *testing.T) {
	cfc := NewConnectionFlowController(1000, 2000, 1000, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 500, 1000, 1000, nil, nil)

	// Stream violation: offset 501 > window 500
	err := sfc.UpdateHighestReceived(501)
	if err == nil {
		t.Fatal("expected FlowControlError for stream limit violation")
	}
	var tErr *TransportError
	if !errors.As(err, &tErr) || tErr.ErrorCode != FlowControlError {
		t.Fatalf("expected FlowControlError, got %v", err)
	}

	// Connection violation
	cfc2 := NewConnectionFlowController(100, 200, 100, nil, nil)
	err = cfc2.UpdateHighestReceived(101)
	if err == nil {
		t.Fatal("expected FlowControlError for connection limit violation")
	}
	if !errors.As(err, &tErr) || tErr.ErrorCode != FlowControlError {
		t.Fatalf("expected FlowControlError, got %v", err)
	}
}

func TestFlowController_AutoTuning(t *testing.T) {
	// RTT is 100ms
	rtt := staticRTT(100 * time.Millisecond)

	sfc := NewStreamFlowController(
		0,
		nil,
		1000, // initial window
		4000, // max window
		1000,
		rtt,
		nil,
	)

	// Read 300 bytes (< 50% threshold): no window update yet
	sfc.AddBytesRead(300)
	_, ok := sfc.MaybeGetWindowUpdate()
	if ok {
		t.Fatal("did not expect window update when consuming < 50%")
	}

	// Consume another 300 bytes (total 600 > 50% threshold) immediately (well within 4*RTT)
	sfc.AddBytesRead(300)
	newWindow, ok := sfc.MaybeGetWindowUpdate()
	if !ok {
		t.Fatal("expected window update after consuming > 50%")
	}

	// Window size should double from 1000 to 2000
	// New limit should be bytesRead (600) + newWindowSize (2000) = 2600
	if newWindow != 2600 {
		t.Fatalf("expected new window 2600 (auto-tuned 2x), got %d", newWindow)
	}

	// Fast consumption again: consume 1100 bytes (total 1700 out of 2600)
	sfc.AddBytesRead(1100)
	newWindow2, ok := sfc.MaybeGetWindowUpdate()
	if !ok {
		t.Fatal("expected second window update")
	}

	// Window size should double from 2000 to max 4000
	// New limit should be 1700 + 4000 = 5700
	if newWindow2 != 5700 {
		t.Fatalf("expected new window 5700 (capped at max 4000), got %d", newWindow2)
	}
}

func TestFlowController_AllowIncreaseHook(t *testing.T) {
	rtt := staticRTT(10 * time.Millisecond)

	var hookCalled bool
	allowHook := func(delta uint64) bool {
		hookCalled = true
		return false // reject increase
	}

	sfc := NewStreamFlowController(0, nil, 1000, 5000, 1000, rtt, allowHook)

	sfc.AddBytesRead(600)
	newWindow, ok := sfc.MaybeGetWindowUpdate()
	if !ok {
		t.Fatal("expected window update")
	}
	if !hookCalled {
		t.Fatal("expected allowWindowIncrease hook to be called")
	}

	// Window should not have doubled because hook rejected it: windowSize remains 1000
	// 600 + 1000 = 1600
	if newWindow != 1600 {
		t.Fatalf("expected window 1600 (no increase), got %d", newWindow)
	}
}
