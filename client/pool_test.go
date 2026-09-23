// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client"
)

type mockCloseableConn struct {
	id     int
	closed atomic.Bool
}

func (m *mockCloseableConn) Close() error {
	m.closed.Store(true)
	return nil
}

func TestPoolManager_EvictionClosesSocket(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockCloseableConn]()
	pm.IdleTimeout = 10 * time.Millisecond

	var idGen int

	pm.Dial = func(ctx context.Context, addr string) (*mockCloseableConn, error) {
		idGen++
		return &mockCloseableConn{id: idGen}, nil
	}

	ctx := context.Background()

	c1, err := pm.Get(ctx, "evict.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, 1, c1.id)

	pm.Put("evict.example.com:443", c1)

	// Wait past idle timeout
	time.Sleep(30 * time.Millisecond)

	c2, err := pm.Get(ctx, "evict.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, 2, c2.id)

	// c1 should have been closed upon eviction
	assert.True(t, c1.closed.Load())
}

func TestPoolManager_HealthCheckFailureClosesSocket(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockCloseableConn]()

	var idGen int

	pm.Dial = func(ctx context.Context, addr string) (*mockCloseableConn, error) {
		idGen++
		return &mockCloseableConn{id: idGen}, nil
	}

	isHealthy := atomic.Bool{}
	isHealthy.Store(true)

	pm.IsHealthy = func(c *mockCloseableConn) bool {
		return isHealthy.Load()
	}

	ctx := context.Background()

	c1, err := pm.Get(ctx, "health.example.com:443")
	require.NoError(t, err)

	pm.Put("health.example.com:443", c1)

	// Mark unhealthy while idle
	isHealthy.Store(false)

	// Get triggers health failure on idle conn
	c2, err := pm.Get(ctx, "health.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, 2, c2.id)
	assert.True(t, c1.closed.Load())

	// Put rejection when unhealthy
	pm.Put("health.example.com:443", c2)
	assert.True(t, c2.closed.Load())
}

func TestPoolManager_CloseManagerDrainsAndCloses(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockCloseableConn]()

	var idGen int

	pm.Dial = func(ctx context.Context, addr string) (*mockCloseableConn, error) {
		idGen++
		return &mockCloseableConn{id: idGen}, nil
	}

	ctx := context.Background()

	c1, err := pm.Get(ctx, "host1:443")
	require.NoError(t, err)
	c2, err := pm.Get(ctx, "host2:443")
	require.NoError(t, err)

	pm.Put("host1:443", c1)
	pm.Put("host2:443", c2)

	err = pm.Close()
	require.NoError(t, err)

	assert.True(t, c1.closed.Load())
	assert.True(t, c2.closed.Load())
}

func TestPoolManager_CustomCloseConnHook(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockCloseableConn]()
	pm.IdleTimeout = 10 * time.Millisecond

	customCloseCalled := atomic.Bool{}
	pm.CloseConn = func(c *mockCloseableConn) error {
		customCloseCalled.Store(true)
		return c.Close()
	}

	var idGen int

	pm.Dial = func(ctx context.Context, addr string) (*mockCloseableConn, error) {
		idGen++
		return &mockCloseableConn{id: idGen}, nil
	}

	ctx := context.Background()

	c1, err := pm.Get(ctx, "custom.hook:443")
	require.NoError(t, err)

	pm.Put("custom.hook:443", c1)

	time.Sleep(30 * time.Millisecond)

	_, err = pm.Get(ctx, "custom.hook:443")
	require.NoError(t, err)

	assert.True(t, customCloseCalled.Load())
	assert.True(t, c1.closed.Load())
}

func TestPoolManager_DialErrorHandling(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockCloseableConn]()
	expectedErr := errors.New("dial error")
	pm.Dial = func(ctx context.Context, addr string) (*mockCloseableConn, error) {
		return nil, expectedErr
	}

	ctx := context.Background()
	c, err := pm.Get(ctx, "fail:443")
	assert.Nil(t, c)
	assert.ErrorIs(t, err, expectedErr)
}

func BenchmarkPoolManager_GetPut(b *testing.B) {
	pm := client.NewPoolManager[*mockCloseableConn]()
	pm.MaxConnsPerHost = 1000

	pm.Dial = func(ctx context.Context, addr string) (*mockCloseableConn, error) {
		return &mockCloseableConn{}, nil
	}

	ctx := context.Background()
	addr := "bench:443"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c, err := pm.Get(ctx, addr)
		if err != nil {
			b.Fatal(err)
		}

		pm.Put(addr, c)
	}
}
