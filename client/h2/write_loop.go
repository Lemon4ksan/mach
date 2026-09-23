// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/lemon4ksan/foundation/silicon/clock"
	"github.com/lemon4ksan/foundation/silicon/sysnet"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (c *Conn) writeLoop() {
	var lastErr error

	defer func() { _ = c.Close() }()
	defer c.recoverWriteLoop(&lastErr)

	if c.pingInterval <= 0 {
		c.pingInterval = DefaultPingInterval
	}

	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		if stop, err := c.selectWriteEvent(ticker.C); stop {
			lastErr = err
			break
		}

		if !c.disableAcks && c.pingUnacks >= 5 {
			lastErr = coreh2.ErrTimeout
			break
		}
	}
}

func (c *Conn) selectWriteEvent(pingChan <-chan time.Time) (bool, error) {
	if fr := c.outRing.Pop(); fr != nil {
		c.writeMu.Lock()

		var batch [16]*coreh2.FrameHeader

		batch[0] = fr
		n := 1

		for n < 16 {
			next := c.outRing.Pop()
			if next == nil {
				break
			}

			batch[n] = next
			n++
		}

		var wErr error

		if n == 1 {
			_, wErr = fr.WriteTo(c.bw)
			if wErr == nil {
				wErr = c.bw.Flush()
			}

			coreh2.ReleaseFrameHeader(fr)
		} else {
			var bufs [][]byte

			for i := 0; i < n; i++ {
				_, _ = batch[i].WriteTo(c.bw)
				coreh2.ReleaseFrameHeader(batch[i])
			}

			if c.bw.Buffered() > 0 {
				writtenBuf := c.bw.AvailableBuffer()
				if len(writtenBuf) > 0 {
					bufs = [][]byte{writtenBuf}
					_, wErr = sysnet.WriteVectorBuffers(c.c, bufs)
				}

				if wErr == nil {
					wErr = c.bw.Flush()
				}
			}
		}

		c.writeMu.Unlock()

		if wErr != nil {
			return true, wErr
		}

		return false, nil
	}

	select {
	case ctx, ok := <-c.in:
		if !ok {
			return true, nil
		}

		if err := c.writeRequest(ctx); err != nil {
			ctx.Err <- err

			if errors.Is(err, coreh2.ErrNoAvailableStreams) {
				return false, nil
			}

			return true, err
		}

	case fr, ok := <-c.out:
		if !ok {
			return true, nil
		}

		defer coreh2.ReleaseFrameHeader(fr)

		c.writeMu.Lock()

		_, wErr := fr.WriteTo(c.bw)
		if wErr == nil {
			wErr = c.bw.Flush()
		}

		c.writeMu.Unlock()

		if wErr != nil {
			return true, wErr
		}

	case <-pingChan:
		if err := c.writePing(); err != nil {
			return true, err
		}
	}

	return false, nil
}

func (c *Conn) recoverWriteLoop(lastErr *error) {
	if r := recover(); r != nil && *lastErr == nil {
		if err, ok := r.(error); ok {
			*lastErr = err
		} else {
			*lastErr = fmt.Errorf("h2engine panic: %v", r)
		}
	}

	if *lastErr == nil {
		*lastErr = io.ErrUnexpectedEOF
	}

	c.broadcastErrorToAllStreams(*lastErr)
}

func (c *Conn) writePing() error {
	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	ping := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
	binary.BigEndian.PutUint64(ping.Data(), uint64(clock.CoarseNowNano())) //nolint:gosec // timestamp uint64 conversion
	fr.SetBody(ping)

	c.writeMu.Lock()

	_, err := fr.WriteTo(c.bw)
	if err == nil {
		err = c.bw.Flush()
		if err == nil {
			c.pingUnacks++
		}
	}

	c.writeMu.Unlock()

	return err
}
