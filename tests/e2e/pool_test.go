// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/proto/http/status"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client"
	h1client "github.com/lemon4ksan/mach/client/h1"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	h1server "github.com/lemon4ksan/mach/server/h1"
)

type mockConn struct {
	id      int
	addr    string
	healthy atomic.Bool
}

func newMockConn(id int, addr string) *mockConn {
	c := &mockConn{
		id:   id,
		addr: addr,
	}
	c.healthy.Store(true)
	return c
}

func TestPoolManager_ConnectionReuse(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockConn]()
	dialCount := 0

	pm.Dial = func(ctx context.Context, addr string) (*mockConn, error) {
		dialCount++
		return newMockConn(dialCount, addr), nil
	}

	ctx := context.Background()

	// 1. Initial Get dials conn #1
	c1, err := pm.Get(ctx, "api.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, 1, c1.id)
	assert.Equal(t, 1, dialCount)

	// 2. Put conn #1 back
	pm.Put("api.example.com:443", c1)

	// 3. Subsequent Get reuses conn #1 without dialing
	c2, err := pm.Get(ctx, "api.example.com:443")
	require.NoError(t, err)
	assert.Equal(t, c1.id, c2.id)
	assert.Equal(t, 1, dialCount)
}

func TestPoolManager_HostIsolation(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockConn]()
	dialCounter := atomic.Int32{}

	pm.Dial = func(ctx context.Context, addr string) (*mockConn, error) {
		id := int(dialCounter.Add(1))
		return newMockConn(id, addr), nil
	}

	ctx := context.Background()

	// Get for Host A
	cA, err := pm.Get(ctx, "host-a.com:443")
	require.NoError(t, err)
	assert.Equal(t, "host-a.com:443", cA.addr)

	// Get for Host B
	cB, err := pm.Get(ctx, "host-b.com:443")
	require.NoError(t, err)
	assert.Equal(t, "host-b.com:443", cB.addr)
	assert.NotEqual(t, cA.id, cB.id)

	// Return both to pool
	pm.Put("host-a.com:443", cA)
	pm.Put("host-b.com:443", cB)

	// Verify Host A only returns Host A's connection
	cA2, err := pm.Get(ctx, "host-a.com:443")
	require.NoError(t, err)
	assert.Equal(t, cA.id, cA2.id)
	assert.Equal(t, "host-a.com:443", cA2.addr)

	// Verify Host B only returns Host B's connection
	cB2, err := pm.Get(ctx, "host-b.com:443")
	require.NoError(t, err)
	assert.Equal(t, cB.id, cB2.id)
	assert.Equal(t, "host-b.com:443", cB2.addr)
}

func TestPoolManager_MaxConnsPerHost(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockConn]()
	pm.MaxConnsPerHost = 2

	var idGen int
	pm.Dial = func(ctx context.Context, addr string) (*mockConn, error) {
		idGen++
		return newMockConn(idGen, addr), nil
	}

	ctx := context.Background()

	c1, err := pm.Get(ctx, "capped.host:443")
	require.NoError(t, err)
	c2, err := pm.Get(ctx, "capped.host:443")
	require.NoError(t, err)

	// 3rd Get exceeds MaxConnsPerHost (2)
	c3, err := pm.Get(ctx, "capped.host:443")
	assert.Nil(t, c3)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Return c1, now Get succeeds by reusing c1
	pm.Put("capped.host:443", c1)
	cReused, err := pm.Get(ctx, "capped.host:443")
	require.NoError(t, err)
	assert.Equal(t, c1.id, cReused.id)

	// Clean up
	pm.Put("capped.host:443", c2)
	pm.Put("capped.host:443", cReused)
}

func TestPoolManager_IdleTimeoutEviction(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockConn]()
	pm.IdleTimeout = 50 * time.Millisecond

	var idGen int
	pm.Dial = func(ctx context.Context, addr string) (*mockConn, error) {
		idGen++
		return newMockConn(idGen, addr), nil
	}

	ctx := context.Background()

	c1, err := pm.Get(ctx, "idle.host:443")
	require.NoError(t, err)
	assert.Equal(t, 1, c1.id)

	pm.Put("idle.host:443", c1)

	// Sleep past IdleTimeout
	time.Sleep(100 * time.Millisecond)

	// Next Get should evict expired connection and dial new one
	c2, err := pm.Get(ctx, "idle.host:443")
	require.NoError(t, err)
	assert.Equal(t, 2, c2.id)
	assert.NotEqual(t, c1.id, c2.id)
}

func TestPoolManager_HealthCheckEviction(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockConn]()
	pm.IsHealthy = func(c *mockConn) bool {
		return c != nil && c.healthy.Load()
	}

	var idGen int
	pm.Dial = func(ctx context.Context, addr string) (*mockConn, error) {
		idGen++
		return newMockConn(idGen, addr), nil
	}

	ctx := context.Background()

	// 1. Eviction on Get
	c1, err := pm.Get(ctx, "health.host:443")
	require.NoError(t, err)
	assert.Equal(t, 1, c1.id)

	pm.Put("health.host:443", c1)

	// Mark connection unhealthy while idle in pool
	c1.healthy.Store(false)

	// Subsequent Get detects unhealthy state and dials new conn #2
	c2, err := pm.Get(ctx, "health.host:443")
	require.NoError(t, err)
	assert.Equal(t, 2, c2.id)

	// 2. Rejection on Put
	c3, err := pm.Get(ctx, "health.host:443")
	require.NoError(t, err)
	assert.Equal(t, 3, c3.id)

	c3.healthy.Store(false)
	pm.Put("health.host:443", c3) // Rejected on put, count decremented

	// Subsequent Get dials new conn #4 instead of returning c3
	c4, err := pm.Get(ctx, "health.host:443")
	require.NoError(t, err)
	assert.Equal(t, 4, c4.id)
}

func TestPoolManager_ConcurrentGetPut(t *testing.T) {
	t.Parallel()

	pm := client.NewPoolManager[*mockConn]()
	pm.MaxConnsPerHost = 50

	var idCounter atomic.Int32
	pm.Dial = func(ctx context.Context, addr string) (*mockConn, error) {
		id := int(idCounter.Add(1))
		return newMockConn(id, addr), nil
	}

	concurrency := 40
	iterations := 20
	hosts := []string{"h1.tf:443", "h2.tf:443", "h3.tf:443", "h4.tf:443"}

	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			ctx := context.Background()
			host := hosts[workerID%len(hosts)]

			for j := 0; j < iterations; j++ {
				c, err := pm.Get(ctx, host)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						time.Sleep(2 * time.Millisecond)
						continue
					}
					errs[workerID] = err
					return
				}

				if c.addr != host {
					errs[workerID] = fmt.Errorf("host mismatch: expected %s, got %s", host, c.addr)
					return
				}

				time.Sleep(1 * time.Millisecond)
				pm.Put(host, c)
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d failed", i)
	}
}

func TestPoolManager_RealH1ClientConnIntegration(t *testing.T) {
	t.Parallel()

	reqCounter := atomic.Int32{}
	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		cnt := reqCounter.Add(1)
		res.StatusCode = status.OK
		res.Body = []byte(fmt.Sprintf("pool-response-%d", cnt))

		return nil
	})
	defer cleanup()

	pm := client.NewPoolManager[*h1client.ClientConn]()
	pm.MaxConnsPerHost = 10

	var openSocketsMu sync.Mutex
	var openSockets []net.Conn

	pm.Dial = func(ctx context.Context, dialAddr string) (*h1client.ClientConn, error) {
		c, err := net.DialTimeout("tcp", dialAddr, 2*time.Second)
		if err != nil {
			return nil, err
		}

		openSocketsMu.Lock()
		openSockets = append(openSockets, c)
		openSocketsMu.Unlock()

		return h1client.NewClientConn(c), nil
	}

	defer func() {
		openSocketsMu.Lock()
		for _, s := range openSockets {
			_ = s.Close()
		}
		openSocketsMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. First request acquires connection, performs request, returns to pool
	c1, err := pm.Get(ctx, addr)
	require.NoError(t, err)

	req1 := machhttp.AcquireRequest()
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req1)
	defer machhttp.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("/pool-first")
	req1.Header.SetHost(addr)

	err = c1.Do(ctx, req1, resp1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp1.StatusCode())
	assert.Equal(t, "pool-response-1", string(resp1.Body()))

	pm.Put(addr, c1)

	// 2. Second request retrieves pooled connection, performs second request
	c2, err := pm.Get(ctx, addr)
	require.NoError(t, err)
	assert.Equal(t, c1, c2) // Exact same pointer instance reused

	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("/pool-second")
	req2.Header.SetHost(addr)

	err = c2.Do(ctx, req2, resp2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode())
	assert.Equal(t, "pool-response-2", string(resp2.Body()))

	pm.Put(addr, c2)
}
