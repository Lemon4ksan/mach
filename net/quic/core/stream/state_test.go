// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"testing"
)

func TestSendState_Transitions(t *testing.T) {
	valid := [][2]SendState{
		{SendStateReady, SendStateSend},
		{SendStateReady, SendStateDataSent},
		{SendStateReady, SendStateResetSent},
		{SendStateSend, SendStateDataSent},
		{SendStateSend, SendStateResetSent},
		{SendStateDataSent, SendStateDataRecvd},
		{SendStateDataSent, SendStateResetSent},
		{SendStateResetSent, SendStateResetRecvd},
	}

	for _, tr := range valid {
		if !ValidateSendTransition(tr[0], tr[1]) {
			t.Errorf("expected valid send transition from %s to %s", tr[0], tr[1])
		}
	}

	invalid := [][2]SendState{
		{SendStateReady, SendStateDataRecvd},
		{SendStateReady, SendStateResetRecvd},
		{SendStateDataRecvd, SendStateSend},
		{SendStateDataRecvd, SendStateResetSent},
		{SendStateResetRecvd, SendStateReady},
		{SendStateResetRecvd, SendStateDataSent},
	}

	for _, tr := range invalid {
		if ValidateSendTransition(tr[0], tr[1]) {
			t.Errorf("expected invalid send transition from %s to %s", tr[0], tr[1])
		}
	}
}

func TestRecvState_Transitions(t *testing.T) {
	valid := [][2]RecvState{
		{RecvStateRecv, RecvStateSizeKnown},
		{RecvStateRecv, RecvStateDataRecvd},
		{RecvStateRecv, RecvStateResetRecvd},
		{RecvStateSizeKnown, RecvStateDataRecvd},
		{RecvStateSizeKnown, RecvStateResetRecvd},
		{RecvStateDataRecvd, RecvStateDataRead},
		{RecvStateDataRecvd, RecvStateResetRecvd},
		{RecvStateResetRecvd, RecvStateResetRead},
	}

	for _, tr := range valid {
		if !ValidateRecvTransition(tr[0], tr[1]) {
			t.Errorf("expected valid recv transition from %s to %s", tr[0], tr[1])
		}
	}

	invalid := [][2]RecvState{
		{RecvStateRecv, RecvStateDataRead},
		{RecvStateDataRead, RecvStateRecv},
		{RecvStateResetRead, RecvStateRecv},
		{RecvStateDataRecvd, RecvStateSizeKnown},
	}

	for _, tr := range invalid {
		if ValidateRecvTransition(tr[0], tr[1]) {
			t.Errorf("expected invalid recv transition from %s to %s", tr[0], tr[1])
		}
	}
}

func TestState_Properties(t *testing.T) {
	if !SendStateDataRecvd.IsTerminal() || !SendStateResetRecvd.IsTerminal() {
		t.Errorf("terminal send states failed check")
	}
	if SendStateReady.IsTerminal() || SendStateSend.IsTerminal() {
		t.Errorf("non-terminal send states failed check")
	}

	if !RecvStateDataRead.IsTerminal() || !RecvStateResetRead.IsTerminal() {
		t.Errorf("terminal recv states failed check")
	}
	if RecvStateRecv.IsTerminal() || RecvStateSizeKnown.IsTerminal() {
		t.Errorf("non-terminal recv states failed check")
	}

	if !SendStateReady.CanWrite() || !SendStateSend.CanWrite() {
		t.Errorf("can write states failed check")
	}
	if SendStateDataSent.CanWrite() || SendStateResetSent.CanWrite() {
		t.Errorf("cannot write states failed check")
	}

	if !RecvStateRecv.CanRead() || !RecvStateDataRecvd.CanRead() {
		t.Errorf("can read states failed check")
	}
	if RecvStateDataRead.CanRead() || RecvStateResetRead.CanRead() {
		t.Errorf("cannot read states failed check")
	}
}
