package client

import (
	"context"
	"sync"
	"time"
)

// PoolManager manages connection reuse, pooling, and eviction across different hosts globally.
// This fulfills the architectural requirement for a global connection pool (similar to Chromium's HttpStreamPool).
type PoolManager[T any] struct {
	mu sync.RWMutex

	// pools groups connections by host/addr.
	pools map[string]*hostPool[T]

	// IdleTimeout specifies how long a connection can be idle before eviction.
	IdleTimeout time.Duration

	// MaxConnsPerHost limits concurrent connections to a single host.
	MaxConnsPerHost int

	// Dial is the factory function to create a new protocol connection.
	Dial func(ctx context.Context, addr string) (T, error)

	// IsHealthy checks if a pooled connection is still alive and usable.
	IsHealthy func(c T) bool
}

type hostPool[T any] struct {
	conns []*idleConn[T]
	count int // active + idle
}

type idleConn[T any] struct {
	conn       T
	lastActive time.Time
}

// NewPoolManager creates a new global connection pool manager.
func NewPoolManager[T any]() *PoolManager[T] {
	return &PoolManager[T]{
		pools:           make(map[string]*hostPool[T]),
		IdleTimeout:     90 * time.Second,
		MaxConnsPerHost: 100,
	}
}

// Get acquires a connection to the given address. It reuses an idle connection if available and healthy,
// otherwise dials a new one subject to MaxConnsPerHost.
func (p *PoolManager[T]) Get(ctx context.Context, addr string) (T, error) {
	p.mu.Lock()
	hp, ok := p.pools[addr]
	if !ok {
		hp = &hostPool[T]{}
		p.pools[addr] = hp
	}

	// Try to pop an idle connection
	for len(hp.conns) > 0 {
		ic := hp.conns[len(hp.conns)-1]
		hp.conns = hp.conns[:len(hp.conns)-1]

		// Check idle timeout
		if p.IdleTimeout > 0 && time.Since(ic.lastActive) > p.IdleTimeout {
			hp.count--
			continue // evicted
		}

		p.mu.Unlock()
		if p.IsHealthy != nil && !p.IsHealthy(ic.conn) {
			p.mu.Lock()
			hp.count--
			continue
		}
		return ic.conn, nil
	}

	if p.MaxConnsPerHost > 0 && hp.count >= p.MaxConnsPerHost {
		p.mu.Unlock()
		var zero T
		return zero, context.DeadlineExceeded // Or a custom ErrMaxConnsPerHost
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

// Put returns a connection to the pool for reuse.
func (p *PoolManager[T]) Put(addr string, c T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	hp, ok := p.pools[addr]
	if !ok {
		return // Should not happen if Get was used, but safety first.
	}

	if p.IsHealthy != nil && !p.IsHealthy(c) {
		hp.count--
		return
	}

	hp.conns = append(hp.conns, &idleConn[T]{
		conn:       c,
		lastActive: time.Now(),
	})
}
