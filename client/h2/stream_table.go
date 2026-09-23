// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"sync"
)

const (
	streamTableSize = 2048
	streamTableMask = streamTableSize - 1
	streamMaxProbes = 8
	streamNumShards = 16
	streamShardMask = streamNumShards - 1
)

type streamShard struct {
	mu       sync.RWMutex
	overflow map[uint32]*Context
}

func (c *Conn) getStream(streamID uint32) *Context {
	baseIdx := int((streamID / 2) & streamTableMask)
	for i := range streamMaxProbes {
		idx := (baseIdx + i) & streamTableMask
		if ctx := c.reqStreams[idx].Load(); ctx != nil && ctx.StreamID.Load() == streamID {
			return ctx
		}
	}

	shardIdx := int((streamID / 2) & streamShardMask)
	shard := &c.reqShards[shardIdx]
	shard.mu.RLock()
	ctx := shard.overflow[streamID]
	shard.mu.RUnlock()

	return ctx
}

func (c *Conn) storeStream(ctx *Context) {
	streamID := ctx.StreamID.Load()
	baseIdx := int((streamID / 2) & streamTableMask)

	for i := range streamMaxProbes {
		idx := (baseIdx + i) & streamTableMask
		if c.reqStreams[idx].CompareAndSwap(nil, ctx) {
			return
		}
	}

	shardIdx := int((streamID / 2) & streamShardMask)
	shard := &c.reqShards[shardIdx]
	shard.mu.Lock()

	if shard.overflow == nil {
		shard.overflow = make(map[uint32]*Context, 8)
	}

	shard.overflow[streamID] = ctx
	shard.mu.Unlock()
}

func (c *Conn) deleteStream(streamID uint32) {
	baseIdx := int((streamID / 2) & streamTableMask)
	for i := range streamMaxProbes {
		idx := (baseIdx + i) & streamTableMask
		if cur := c.reqStreams[idx].Load(); cur != nil && cur.StreamID.Load() == streamID {
			c.reqStreams[idx].Store(nil)
			return
		}
	}

	shardIdx := int((streamID / 2) & streamShardMask)
	shard := &c.reqShards[shardIdx]
	shard.mu.Lock()

	if shard.overflow != nil {
		delete(shard.overflow, streamID)
	}

	shard.mu.Unlock()
}

func (c *Conn) broadcastErrorToAllStreams(err error) {
	for i := range streamTableSize {
		if ctx := c.reqStreams[i].Load(); ctx != nil {
			select {
			case ctx.Err <- err:
			default:
			}
		}
	}

	for i := range streamNumShards {
		shard := &c.reqShards[i]
		shard.mu.RLock()

		for _, ctx := range shard.overflow {
			if ctx != nil {
				select {
				case ctx.Err <- err:
				default:
				}
			}
		}

		shard.mu.RUnlock()
	}
}

func (c *Conn) purgeStreamsAfterID(lastStreamID uint32, err error) {
	for i := range streamTableSize {
		if ctx := c.reqStreams[i].Load(); ctx != nil && ctx.StreamID.Load() > lastStreamID {
			c.reqStreams[i].Store(nil)

			select {
			case ctx.Err <- err:
			default:
			}
		}
	}

	for i := range streamNumShards {
		shard := &c.reqShards[i]
		shard.mu.Lock()

		for streamID, ctx := range shard.overflow {
			if ctx != nil && streamID > lastStreamID {
				delete(shard.overflow, streamID)

				select {
				case ctx.Err <- err:
				default:
				}
			}
		}

		shard.mu.Unlock()
	}
}
