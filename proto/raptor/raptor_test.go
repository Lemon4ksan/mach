// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package raptor_test

import (
	"context"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/raptor"
)

func TestEncoder_Encode(t *testing.T) {
	t.Parallel()

	encoder := raptor.NewEncoder()

	// Empty packets
	assert.Nil(t, encoder.Encode(nil, 0))
	assert.Nil(t, encoder.Encode([][]byte{}, 0))
	assert.Nil(t, encoder.Encode([][]byte{{}, {}}, 0))

	// Valid block of packets
	packets := [][]byte{
		[]byte("hello"),
		[]byte("world!"),
		[]byte("raptor"),
	}

	repair := encoder.Encode(packets, 1)
	require.NotEmpty(t, repair)
	assert.Equal(t, 6, len(repair)) // Max packet length is 6 ("world!")

	// Scalar wrapping test
	repair2 := encoder.Encode(packets, 255)
	require.NotEmpty(t, repair2)
}

func TestDecoder_ProcessAsync(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	decoder := raptor.NewDecoder()
	decoder.ProcessAsync(ctx)

	// Feed non-repair datagram (ignored by repair recovery)
	decoder.Feed(raptor.Datagram{
		SymbolID: 1,
		Data:     []byte("data-packet"),
		IsRepair: false,
	})

	// Feed repair datagram
	decoder.Feed(raptor.Datagram{
		SymbolID: 2,
		Data:     []byte("repair-packet"),
		IsRepair: true,
	})

	select {
	case rec := <-decoder.Recovered():
		assert.Equal(t, len("repair-packet"), len(rec))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered packet")
	}
}
