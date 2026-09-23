// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package client provides high-performance client transport engines, connection pooling,
// and multi-protocol session abstractions across HTTP/1.1, HTTP/2, and HTTP/3.
package client

import (
	"context"
	"io"
	"sync"
	"time"
)

// PoolManager manages connection reuse, pooling, idle eviction, and capacity bounds
// across multiple target network addresses globally (RFC 9112 §9.3).
//
// Concurrency Model:
// PoolManager is fully safe for concurrent use by multiple goroutines. Internal synchronization
// is governed by a sync.RWMutex, protecting host-partitioned connection queues with minimal contention.
//
// Eviction & Health Checking:
// Idle connections exceeding IdleTimeout are evicted during Get operations. Custom health validation
// may be injected via IsHealthy to reject dead sockets before reuse or on return. Evicted connections
// that implement io.Closer are automatically closed to prevent socket descriptor leaks.
type PoolManager[T any] struct {
	mu sync.RWMutex

	// pools groups idle and active connections by target host/address.
	pools map[string]*hostPool[T]

	// IdleTimeout specifies the maximum duration a connection may remain idle before eviction.
	// Defaults to 90 seconds. If zero or negative, idle connections never expire.
	IdleTimeout time.Duration

	// MaxConnsPerHost limits the maximum number of concurrent active and idle connections
	// permitted for any single destination host. Defaults to 100.
	MaxConnsPerHost int

	// Dial is the user-supplied factory function invoked to establish a new protocol connection
	// when no suitable idle connection is available in the pool.
	Dial func(ctx context.Context, addr string) (T, error)

	// IsHealthy checks if a connection remains functional prior to acquisition or after return.
	// If IsHealthy returns false, the connection is evicted and closed.
	IsHealthy func(c T) bool

	// CloseConn optionally overrides connection closure on eviction or pool shutdown.
	// If nil, PoolManager checks if T implements io.Closer and calls Close().
	CloseConn func(c T) error
}

type hostPool[T any] struct {
	conns []*idleConn[T]
	count int // active + idle
}

type idleConn[T any] struct {
	conn       T
	lastActive time.Time
}

// NewPoolManager creates a new global connection pool manager initialized with sensible defaults:
// an IdleTimeout of 90 seconds and a MaxConnsPerHost limit of 100 connections.
func NewPoolManager[T any]() *PoolManager[T] {
	return &PoolManager[T]{
		pools:           make(map[string]*hostPool[T]),
		IdleTimeout:     90 * time.Second,
		MaxConnsPerHost: 100,
	}
}

// closeConnHelper releases underlying resources for a discarded connection.
func (p *PoolManager[T]) closeConnHelper(c T) {
	if p.CloseConn != nil {
		_ = p.CloseConn(c)
		return
	}

	if closer, ok := any(c).(io.Closer); ok && closer != nil {
		_ = closer.Close()
	}
}

// Get acquires a connection for the target address. It attempts to reuse the most recently active
// idle connection (LIFO order to promote socket warmth). If an idle connection is expired or
// fails IsHealthy, it is evicted and closed. If no valid idle connection is available, a new connection
// is dialed via Dial, provided hp.count does not exceed MaxConnsPerHost. If MaxConnsPerHost is reached,
// context.DeadlineExceeded is returned.
//
// Concurrency: Fully thread-safe.
func (p *PoolManager[T]) Get(ctx context.Context, addr string) (T, error) {
	p.mu.Lock()

	hp, ok := p.pools[addr]
	if !ok {
		hp = &hostPool[T]{}
		p.pools[addr] = hp
	}

	// Try to pop an idle connection (LIFO)
	for len(hp.conns) > 0 {
		ic := hp.conns[len(hp.conns)-1]
		hp.conns = hp.conns[:len(hp.conns)-1]

		// Check idle timeout
		if p.IdleTimeout > 0 && time.Since(ic.lastActive) > p.IdleTimeout {
			hp.count--

			p.closeConnHelper(ic.conn)

			continue // evicted
		}

		p.mu.Unlock()

		if p.IsHealthy != nil && !p.IsHealthy(ic.conn) {
			p.mu.Lock()
			hp.count--

			p.closeConnHelper(ic.conn)

			continue
		}

		return ic.conn, nil
	}

	if p.MaxConnsPerHost > 0 && hp.count >= p.MaxConnsPerHost {
		p.mu.Unlock()

		var zero T

		return zero, context.DeadlineExceeded
	}

	hp.count++
	p.mu.Unlock()

	c, err := p.Dial(ctx, addr)
	if err != nil {
		p.mu.Lock()
		hp.count--
		p.mu.Unlock()

		var zero T

		return zero, err
	}

	return c, nil
}

// Put returns a connection to the pool for the specified address, recording its last active timestamp.
// If IsHealthy is provided and reports false, the connection is rejected and closed instead of pooled.
//
// Concurrency: Fully thread-safe.
func (p *PoolManager[T]) Put(addr string, c T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	hp, ok := p.pools[addr]
	if !ok {
		p.closeConnHelper(c)

		return
	}

	if p.IsHealthy != nil && !p.IsHealthy(c) {
		hp.count--

		p.closeConnHelper(c)

		return
	}

	hp.conns = append(hp.conns, &idleConn[T]{
		conn:       c,
		lastActive: time.Now(),
	})
}

// Close closes all idle connections across all destination pools and clears the manager.
//
// Concurrency: Fully thread-safe.
func (p *PoolManager[T]) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, hp := range p.pools {
		for _, ic := range hp.conns {
			p.closeConnHelper(ic.conn)
		}

		hp.conns = nil
		hp.count = 0
	}

	clear(p.pools)

	return nil
}
