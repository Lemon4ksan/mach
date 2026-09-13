// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	"crypto/tls"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/quic"
)

func TestH3Client_InitializationAndClose(t *testing.T) {
	t.Parallel()

	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	client := h3.NewClient(tlsCfg, quic.WithDatagrams(true))
	require.NotNil(t, client)
	assert.Contains(t, client.TLSConfig.NextProtos, "h3")

	err := client.Close()
	assert.NoError(t, err)
}
