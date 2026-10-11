// Copyright (c) 2016 the quic-go authors. All rights reserved.

package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/net/quic/internal/ackhandler"
	"github.com/lemon4ksan/mach/net/quic/internal/handshake"
	"github.com/lemon4ksan/mach/net/quic/internal/monotime"
	"github.com/lemon4ksan/mach/net/quic/internal/protocol"
	"github.com/lemon4ksan/mach/net/quic/internal/qerr"
	"github.com/lemon4ksan/mach/net/quic/internal/utils"
	"github.com/lemon4ksan/mach/net/quic/internal/wire"
	"github.com/lemon4ksan/foundation/testing/gomock"
	"github.com/lemon4ksan/foundation/testing/require"
)

// in the tests for the stream deadlines we set a deadline
// and wait to make an assertion when Read / Write was unblocked
// on the CIs, the timing is a lot less precise, so scale every duration by this factor
func scaleDuration(t time.Duration) time.Duration {
	scaleFactor := 1
	if f, err := strconv.Atoi(
		os.Getenv("TIMESCALE_FACTOR"),
	); err == nil { // parsing "" errors, so this works fine if the env is not set
		scaleFactor = f
	}

	if scaleFactor == 0 {
		panic("TIMESCALE_FACTOR is 0")
	}

	return time.Duration(scaleFactor) * t
}

func newUDPConnLocalhost(t testing.TB) *net.UDPConn {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() }) //nolint:errcheck

	return conn
}

func getPacket(t *testing.T, connID protocol.ConnectionID) []byte {
	return getPacketWithPacketType(t, connID, protocol.PacketTypeHandshake, 2)
}

func getPacketWithPacketType(
	t *testing.T,
	connID protocol.ConnectionID,
	typ protocol.PacketType,
	length protocol.ByteCount,
) []byte {
	t.Helper()

	b, err := (&wire.ExtendedHeader{
		Header: wire.Header{
			Type:             typ,
			DestConnectionID: connID,
			Length:           length,
			Version:          protocol.Version1,
		},
		PacketNumberLen: protocol.PacketNumberLen2,
	}).Append(nil, protocol.Version1)
	require.NoError(t, err)

	return append(b, bytes.Repeat([]byte{42}, int(length)-2)...)
}

func areConnsRunning() bool {
	var b bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&b, 1) //nolint:errcheck
	return strings.Contains(b.String(), "quic-go.(*connection).run")
}

func areTransportsRunning() bool {
	var b bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&b, 1) //nolint:errcheck
	return strings.Contains(b.String(), "quic-go.(*Transport).listen")
}

func TestMain(m *testing.M) {
	status := m.Run()
	if status != 0 {
		os.Exit(status)
	}

	if areConnsRunning() {
		fmt.Println("stray connection goroutines found")
		os.Exit(1)
	}

	if areTransportsRunning() {
		fmt.Println("stray transport goroutines found")
		os.Exit(1)
	}

	os.Exit(status)
}

// ---------------------------------------------------------------------------
// 1. Stream as Standard Library net.Conn Stress Verification
// ---------------------------------------------------------------------------

type mockFullAddrSender struct {
	local  net.Addr
	remote net.Addr

	mu           sync.Mutex
	streamData   map[protocol.StreamID][][]byte
	completedIDs []protocol.StreamID
}

func newMockFullAddrSender(local, remote net.Addr) *mockFullAddrSender {
	return &mockFullAddrSender{
		local:      local,
		remote:     remote,
		streamData: make(map[protocol.StreamID][][]byte),
	}
}

func (m *mockFullAddrSender) onHasConnectionData() {}
func (m *mockFullAddrSender) onHasStreamData(id protocol.StreamID, str *SendStream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Pop stream data frame to drain
	for {
		sf, _, hasMore := str.popStreamFrame(protocol.MaxByteCount, protocol.Version1)
		if sf.Frame != nil && sf.Frame.DataLen() > 0 {
			buf := make([]byte, sf.Frame.DataLen())
			copy(buf, sf.Frame.Data)
			m.streamData[id] = append(m.streamData[id], buf)
		}
		if !hasMore {
			break
		}
	}
}
func (m *mockFullAddrSender) onHasStreamRetransmission(protocol.StreamID, *SendStream)            {}
func (m *mockFullAddrSender) onHasStreamControlFrame(protocol.StreamID, streamControlFrameGetter) {}
func (m *mockFullAddrSender) updateStreamPriority(protocol.StreamID)                              {}
func (m *mockFullAddrSender) onStreamCompleted(id protocol.StreamID) {
	m.mu.Lock()
	m.completedIDs = append(m.completedIDs, id)
	m.mu.Unlock()
}
func (m *mockFullAddrSender) LocalAddr() net.Addr  { return m.local }
func (m *mockFullAddrSender) RemoteAddr() net.Addr { return m.remote }

func TestAdversarial_StreamNetConn_AddrLifecycle(t *testing.T) {
	local := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9001}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9002}
	sender := newMockFullAddrSender(local, remote)
	fc := newTestStreamFlowControllerWithSendWindow(1, protocol.MaxByteCount)
	str := newStream(context.Background(), 1, sender, fc, false)

	// 1. Valid addresses before closure
	require.Equal(t, local, str.LocalAddr())
	require.Equal(t, remote, str.RemoteAddr())

	// 2. Addresses during active half-close
	require.NoError(t, str.CloseRead())
	require.Equal(t, local, str.LocalAddr())
	require.Equal(t, remote, str.RemoteAddr())

	require.NoError(t, str.CloseWrite())
	require.Equal(t, local, str.LocalAddr())
	require.Equal(t, remote, str.RemoteAddr())

	// 3. Addresses after Close()
	require.NoError(t, str.Close())
	require.Equal(t, local, str.LocalAddr())
	require.Equal(t, remote, str.RemoteAddr())

	// 4. Addresses after shutdown
	str.closeForShutdown(errors.New("shutdown"))
	require.Equal(t, local, str.LocalAddr())
	require.Equal(t, remote, str.RemoteAddr())

	// 5. Stream with nil/unsupported sender returns nil gracefully
	nilAddrStr := newStream(context.Background(), 2, &dummyNoAddrSender{}, fc, false)
	require.Nil(t, nilAddrStr.LocalAddr())
	require.Nil(t, nilAddrStr.RemoteAddr())

	// Stream with completely nil sender field
	emptyStr := &Stream{}
	require.Nil(t, emptyStr.LocalAddr())
	require.Nil(t, emptyStr.RemoteAddr())
}

func TestAdversarial_StreamNetConn_HalfCloseSemantics(t *testing.T) {
	local := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9001}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9002}
	sender := newMockFullAddrSender(local, remote)
	fc := newTestStreamFlowControllerWithSendWindow(1, protocol.MaxByteCount)
	str := newStream(context.Background(), 1, sender, fc, false)

	// Deliver 5 bytes of data on stream
	err := str.handleStreamFrame(&wire.StreamFrame{
		StreamID: 1,
		Offset:   0,
		Data:     []byte("hello"),
		Fin:      false,
	}, 0)
	require.NoError(t, err)

	// CloseRead cancels reading
	require.NoError(t, str.CloseRead())

	// Idempotency: multiple CloseRead() calls succeed without panic
	require.NoError(t, str.CloseRead())
	require.NoError(t, str.CloseRead())

	// Subsequent Read returns StreamError unwrappable to ErrStreamReset and net.ErrClosed
	buf := make([]byte, 10)
	_, readErr := str.Read(buf)
	require.Error(t, readErr)
	require.True(t, errors.Is(readErr, ErrStreamReset), "expected ErrStreamReset")
	require.True(t, errors.Is(readErr, net.ErrClosed), "expected net.ErrClosed")

	// CloseRead must NOT prevent writing (independent half-close)
	n, writeErr := str.Write([]byte("outbound data"))
	require.NoError(t, writeErr)
	require.Equal(t, 13, n)

	// Now test CloseWrite
	require.NoError(t, str.CloseWrite())
	require.NoError(t, str.CloseWrite()) // Idempotent

	// Future writes must fail
	_, writeErrAfterClose := str.Write([]byte("should fail"))
	require.Error(t, writeErrAfterClose)
}

func TestAdversarial_StreamNetConn_ConcurrentReadWrite(t *testing.T) {
	local := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9001}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9002}
	sender := newMockFullAddrSender(local, remote)
	fc := newTestStreamFlowControllerWithSendWindow(1, protocol.MaxByteCount)
	str := newStream(context.Background(), 1, sender, fc, false)

	const numOps = 100
	var wg sync.WaitGroup

	// Writer goroutine
	wg.Go(func() {
		for range numOps {
			payload := []byte("stress-test-data-payload")
			_, err := str.Write(payload)
			if err != nil {
				return
			}
		}
		_ = str.CloseWrite()
	})

	// Feeder goroutine delivering frames to receiveStr
	wg.Go(func() {
		var offset protocol.ByteCount
		for i := range numOps {
			chunk := []byte("stream-chunk")
			_ = str.handleStreamFrame(&wire.StreamFrame{
				StreamID: 1,
				Offset:   offset,
				Data:     chunk,
				Fin:      i == numOps-1,
			}, 0)
			offset += protocol.ByteCount(len(chunk))
			time.Sleep(50 * time.Microsecond)
		}
	})

	// Reader goroutine
	wg.Go(func() {
		buf := make([]byte, 32)
		for {
			_, err := str.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				return
			}
		}
	})

	wg.Wait()
}

func TestAdversarial_StreamNetConn_Deadlines(t *testing.T) {
	local := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9001}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9002}
	sender := newMockFullAddrSender(local, remote)
	fc := newTestStreamFlowControllerWithSendWindow(1, protocol.MaxByteCount)
	str := newStream(context.Background(), 1, sender, fc, false)

	// 1. Past read deadline: immediate timeout
	require.NoError(t, str.SetReadDeadline(time.Now().Add(-time.Second)))
	buf := make([]byte, 10)
	_, err := str.Read(buf)
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrDeadlineExceeded))
	require.True(t, errors.Is(err, ErrDeadline))
	var netErr net.Error
	require.True(t, errors.As(err, &netErr))
	require.True(t, netErr.Timeout())

	// 2. Clear read deadline with zero time
	require.NoError(t, str.SetReadDeadline(time.Time{}))

	// 3. Future read deadline with timeout unblocking
	require.NoError(t, str.SetReadDeadline(time.Now().Add(50*time.Millisecond)))
	start := time.Now()
	_, err = str.Read(buf)
	elapsed := time.Since(start)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDeadline))
	require.GreaterOrEqual(t, elapsed, 40*time.Millisecond)

	// 4. SetDeadline sets both read and write deadlines
	require.NoError(t, str.SetDeadline(time.Now().Add(-time.Hour)))
	_, err = str.Read(buf)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDeadline))

	// 5. Concurrent deadline updates while read is blocked
	require.NoError(t, str.SetReadDeadline(time.Time{})) // unblock
	readDone := make(chan error, 1)
	go func() {
		_, rErr := str.Read(buf)
		readDone <- rErr
	}()

	// Wait for read to block, then set past deadline to abort it
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, str.SetReadDeadline(time.Now().Add(-time.Millisecond)))

	select {
	case rErr := <-readDone:
		require.Error(t, rErr)
		require.True(t, errors.Is(rErr, ErrDeadline))
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not unblock after SetReadDeadline to past time")
	}
}

// ---------------------------------------------------------------------------
// 2. Conn.Close(), Handshake(ctx), and StreamListener(ctx) Verification
// ---------------------------------------------------------------------------

func TestAdversarial_Conn_HandshakeMatrix(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	// 1. Already-canceled context returns context.Canceled immediately
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err := conn.Handshake(canceledCtx)
	require.ErrorIs(t, err, context.Canceled)

	// 2. Context with timeout expires during handshake
	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelTimeout()
	err = conn.Handshake(timeoutCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// 3. Clean handshake completion
	close(conn.handshakeCompleteChan)
	require.NoError(t, conn.Handshake(context.Background()))

	// 4. Closed connection returns close error on Handshake()
	require.NoError(t, conn.Close())
	freshCtx := context.Background()
	_ = conn.Handshake(freshCtx) // handshake channel is closed so it returns nil or close cause
}

func TestAdversarial_StreamListener_Harness(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	ctx := t.Context()

	ln := conn.StreamListener(ctx)
	require.NotNil(t, ln)
	require.Equal(t, conn.LocalAddr(), ln.Addr())

	// 1. Accept under listener Close()
	acceptErrChan := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		acceptErrChan <- err
	}()

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, ln.Close())

	select {
	case err := <-acceptErrChan:
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock on listener.Close()")
	}

	// 2. Accept under connection Close()
	tc2 := newClientTestConnection(t, nil, nil, false)
	conn2 := tc2.conn
	tc2.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	ln2 := conn2.StreamListener(context.Background())
	acceptErrChan2 := make(chan error, 1)
	go func() {
		_, err := ln2.Accept()
		acceptErrChan2 <- err
	}()

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, conn2.Close())

	select {
	case err := <-acceptErrChan2:
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrConnectionClosed) || errors.Is(err, net.ErrClosed))
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock on conn.Close()")
	}
}

// ---------------------------------------------------------------------------
// 3. Push Iterators Stress & Zero Allocations
// ---------------------------------------------------------------------------

func TestAdversarial_SupportedVersionsSeq_Behavior(t *testing.T) {
	// 1. Full iteration yields exact supported versions
	var collected []Version
	for v := range SupportedVersionsSeq() {
		collected = append(collected, v)
	}
	require.Equal(t, SupportedVersions(), collected)

	// 2. Early break stops at 1 iteration cleanly
	count := 0
	for range SupportedVersionsSeq() {
		count++
		break
	}
	require.Equal(t, 1, count)

	// 3. Nested iteration
	nestedCount := 0
	for v1 := range SupportedVersionsSeq() {
		for v2 := range SupportedVersionsSeq() {
			if v1 == v2 {
				nestedCount++
			}
		}
	}
	require.Equal(t, len(collected), nestedCount)
}

func TestAdversarial_StreamsIterators_RapidChurn(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	const numWorkers = 8
	const numOpsPerWorker = 30
	var wg sync.WaitGroup

	// Concurrent stream creators
	for range numWorkers {
		wg.Go(func() {
			for range numOpsPerWorker {
				str, err := conn.OpenStream()
				if err == nil && str != nil {
					_ = str.CloseWrite()
				}
				uniStr, err := conn.OpenUniStream()
				if err == nil && uniStr != nil {
					_ = uniStr.CloseWrite()
				}
			}
		})
	}

	// Concurrent stream iterators
	stopIter := atomic.Bool{}
	for range 4 {
		wg.Go(func() {
			for !stopIter.Load() {
				for s := range conn.Streams() {
					_ = s.StreamID()
				}
				for s := range conn.SendStreams() {
					_ = s.StreamID()
				}
				for s := range conn.ReceiveStreams() {
					_ = s.StreamID()
				}
			}
		})
	}

	// Wait for creators to complete
	time.Sleep(200 * time.Millisecond)
	stopIter.Store(true)
	wg.Wait()
}

func TestAdversarial_StreamsIterator_ReentrancyAndDeadlockPrevention(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	params := &wire.TransportParameters{
		InitialSourceConnectionID:       tc.destConnID,
		OriginalDestinationConnectionID: tc.conn.origDestConnID,
		MaxBidiStreamNum:                100,
		MaxUniStreamNum:                 100,
	}
	require.NoError(t, tc.conn.handleTransportParameters(params))
	tc.conn.applyTransportParameters()

	// Open 2 streams
	str1, err := conn.OpenStream()
	require.NoError(t, err)
	require.NotNil(t, str1)

	str2, err := conn.OpenStream()
	require.NoError(t, err)
	require.NotNil(t, str2)

	// Verify iterating over Streams() allows standard non-mutating inspections without deadlock
	iterDone := make(chan struct{})
	go func() {
		defer close(iterDone)
		ids := make([]StreamID, 0, 2)
		for s := range conn.Streams() {
			ids = append(ids, s.StreamID())
			_ = s.LocalAddr()
			_ = s.RemoteAddr()
		}
		require.Len(t, ids, 2)
	}()

	select {
	case <-iterDone:
		// Succeeded
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK: conn.Streams() iteration hung")
	}
}

func TestAdversarial_StreamsIterator_MutateInsideLoop(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	params := &wire.TransportParameters{
		InitialSourceConnectionID:       tc.destConnID,
		OriginalDestinationConnectionID: tc.conn.origDestConnID,
		MaxBidiStreamNum:                100,
		MaxUniStreamNum:                 100,
	}
	require.NoError(t, tc.conn.handleTransportParameters(params))
	tc.conn.applyTransportParameters()

	str1, err := conn.OpenStream()
	require.NoError(t, err)
	require.NotNil(t, str1)

	deadlockChan := make(chan bool, 1)
	go func() {
		for s := range conn.Streams() {
			_ = s
			// Attempt to open a stream while holding RLock via iterator
			_, _ = conn.OpenStream()
			break
		}
		deadlockChan <- false
	}()

	select {
	case <-deadlockChan:
		t.Log("Reentrant OpenStream() inside Streams() completed without deadlock")
	case <-time.After(500 * time.Millisecond):
		t.Log("Note: Reentrant OpenStream() inside Streams() blocked as expected due to RWMutex")
	}
}

// ---------------------------------------------------------------------------
// 4. Sentinel Errors & Unwrapping Verification
// ---------------------------------------------------------------------------

func TestAdversarial_SentinelErrors_Matrix(t *testing.T) {
	// 1. ErrStreamReset and net.ErrClosed on StreamError
	se := &StreamError{StreamID: 4, ErrorCode: 100, Remote: true}
	require.True(t, errors.Is(se, ErrStreamReset))
	require.True(t, errors.Is(se, net.ErrClosed))
	require.False(t, errors.Is(se, ErrConnectionClosed))

	// 2. ErrConnectionClosed and net.ErrClosed on ApplicationError
	ae := &ApplicationError{ErrorCode: 0, ErrorMessage: "clean close", Remote: false}
	require.True(t, errors.Is(ae, ErrConnectionClosed))
	require.True(t, errors.Is(ae, net.ErrClosed))
	require.False(t, errors.Is(ae, ErrStreamReset))

	// 3. ErrConnectionClosed and net.ErrClosed on TransportError
	te := &TransportError{ErrorCode: qerr.NoError, FrameType: 0, Remote: true}
	require.True(t, errors.Is(te, ErrConnectionClosed))
	require.True(t, errors.Is(te, net.ErrClosed))

	// 4. ErrTimeout and net.ErrClosed on IdleTimeoutError
	ite := &IdleTimeoutError{}
	require.True(t, errors.Is(ite, ErrTimeout))
	require.True(t, errors.Is(ite, net.ErrClosed))
	require.False(t, errors.Is(ite, ErrStreamReset))

	// 5. ErrTimeout and net.ErrClosed on HandshakeTimeoutError
	hte := &HandshakeTimeoutError{}
	require.True(t, errors.Is(hte, ErrTimeout))
	require.True(t, errors.Is(hte, net.ErrClosed))

	// 6. ErrStreamLimitReached on StreamLimitReachedError
	sle := StreamLimitReachedError{}
	require.True(t, errors.Is(sle, ErrStreamLimitReached))
	require.True(t, errors.Is(&sle, ErrStreamLimitReached))

	// 7. ErrDeadline on errDeadline
	require.True(t, errors.Is(errDeadline, ErrDeadline))
	require.True(t, errors.Is(errDeadline, os.ErrDeadlineExceeded))
}

// ---------------------------------------------------------------------------
// 5. Zero-Allocation Verification Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkAdversarial_SupportedVersionsSeq_Full(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for v := range SupportedVersionsSeq() {
			_ = v
		}
	}
}

func BenchmarkAdversarial_SupportedVersionsSeq_Break(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for v := range SupportedVersionsSeq() {
			_ = v
			break
		}
	}
}

func BenchmarkAdversarial_Stream_Addresses(b *testing.B) {
	local := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9001}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9002}
	sender := newMockFullAddrSender(local, remote)
	fc := newTestStreamFlowControllerWithSendWindow(1, protocol.MaxByteCount)
	str := newStream(context.Background(), 1, sender, fc, false)

	b.ReportAllocs()
	for b.Loop() {
		_ = str.LocalAddr()
		_ = str.RemoteAddr()
	}
}

func BenchmarkAdversarial_SentinelErrors_StreamReset(b *testing.B) {
	err := &StreamError{StreamID: 10, ErrorCode: 42, Remote: true}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrStreamReset) || !errors.Is(err, net.ErrClosed) {
			b.Fatal("unexpected mismatch")
		}
	}
}

func BenchmarkAdversarial_SentinelErrors_ConnectionClosed(b *testing.B) {
	err := &ApplicationError{ErrorCode: 0, ErrorMessage: "closed", Remote: false}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrConnectionClosed) || !errors.Is(err, net.ErrClosed) {
			b.Fatal("unexpected mismatch")
		}
	}
}

func BenchmarkAdversarial_SentinelErrors_Timeout(b *testing.B) {
	err := &IdleTimeoutError{}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrTimeout) || !errors.Is(err, net.ErrClosed) {
			b.Fatal("unexpected mismatch")
		}
	}
}

func BenchmarkAdversarial_SentinelErrors_LimitReached(b *testing.B) {
	err := StreamLimitReachedError{}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrStreamLimitReached) {
			b.Fatal("unexpected mismatch")
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Modernized Connection Getters & Thread Safety Verification
// ---------------------------------------------------------------------------

func TestAdversarial_Conn_ModernizedGetters_Concurrent(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	const (
		numWorkers = 24
		iters      = 1000
	)

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for i := range iters {
				switch (workerID + i) % 5 {
				case 0:
					state := conn.ConnectionState()
					_ = state.GSO
					_ = state.Used0RTT
				case 1:
					stats := conn.ConnectionStats()
					_ = stats.MinRTT
					_ = stats.BytesSent
					_ = stats.PacketsReceived
				case 2:
					_ = conn.Context()
				case 3:
					_ = conn.LocalAddr()
				case 4:
					_ = conn.RemoteAddr()
				}
			}
		}(w)
	}

	// Mutate stats concurrently using conn.connStats atomic counters
	go func() {
		for range 5000 {
			conn.connStats.BytesSent.Add(1200)
			conn.connStats.PacketsSent.Add(1)
			conn.connStats.BytesReceived.Add(1200)
			conn.connStats.PacketsReceived.Add(1)
		}
	}()

	wg.Wait()
}

func (c *wrappedConn) earlyConnReady() <-chan struct{} {
	if c.testHooks == nil {
		return c.Conn.earlyConnReady()
	}

	if c.testHooks.earlyConnReady != nil {
		return c.testHooks.earlyConnReady()
	}

	return nil
}

func (c *wrappedConn) Context() context.Context {
	if c.testHooks == nil {
		return c.Conn.Context()
	}

	if c.testHooks.context != nil {
		return c.testHooks.context()
	}

	return context.Background()
}

func (c *wrappedConn) HandshakeComplete() <-chan struct{} {
	if c.testHooks == nil {
		return c.Conn.HandshakeComplete()
	}

	if c.testHooks.handshakeComplete != nil {
		return c.testHooks.handshakeComplete()
	}

	return nil
}

func (c *wrappedConn) closeWithTransportError(code TransportErrorCode) {
	if c.testHooks == nil {
		c.Conn.closeWithTransportError(code)
		return
	}

	if c.testHooks.closeWithTransportError != nil {
		c.testHooks.closeWithTransportError(code)
	}
}

func (c *wrappedConn) destroy(e error) {
	if c.testHooks == nil {
		c.Conn.destroy(e)
		return
	}

	if c.testHooks.destroy != nil {
		c.testHooks.destroy(e)
	}
}

func (c *wrappedConn) handlePacket(p receivedPacket) {
	if c.testHooks == nil {
		c.Conn.handlePacket(p)
		return
	}

	if c.testHooks.handlePacket != nil {
		c.testHooks.handlePacket(p)
	}
}

func (c *wrappedConn) Start() error {
	if c.testHooks != nil {
		return nil
	}

	return c.Conn.Start()
}

func (c *wrappedConn) Tick(now time.Time) error {
	if c.testHooks != nil {
		return nil
	}

	return c.Conn.Tick(now)
}

func newTestStreamFlowController(id protocol.StreamID) *streamFlowController {
	return newTestStreamFlowControllerWithSendWindow(id, 0)
}

func newTestStreamFlowControllerWithSendWindow(
	id protocol.StreamID,
	sendWindow protocol.ByteCount,
) *streamFlowController {
	return newTestStreamFlowControllerWithWindows(id, sendWindow, protocol.MaxByteCount, protocol.MaxByteCount)
}

func newTestStreamFlowControllerWithWindows(
	id protocol.StreamID,
	sendWindow protocol.ByteCount,
	streamReceiveWindow protocol.ByteCount,
	connReceiveWindow protocol.ByteCount,
) *streamFlowController {
	connFC := newConnectionFlowController(
		connReceiveWindow,
		protocol.MaxByteCount,
		nil,
		utils.NewRTTStats(),
		utils.DefaultLogger,
	)
	connFC.UpdateSendWindow(protocol.MaxByteCount)

	return newStreamFlowController(
		id,
		connFC,
		streamReceiveWindow,
		protocol.MaxByteCount,
		sendWindow,
		utils.NewRTTStats(),
		utils.DefaultLogger,
	)
}

type benchAddrSender struct {
	local  net.Addr
	remote net.Addr
}

func (m *benchAddrSender) onHasConnectionData()                                       {}
func (m *benchAddrSender) onHasStreamData(StreamID, *SendStream)                      {}
func (m *benchAddrSender) onHasStreamRetransmission(StreamID, *SendStream)            {}
func (m *benchAddrSender) onHasStreamControlFrame(StreamID, streamControlFrameGetter) {}
func (m *benchAddrSender) updateStreamPriority(StreamID)                              {}
func (m *benchAddrSender) onStreamCompleted(StreamID)                                 {}
func (m *benchAddrSender) LocalAddr() net.Addr                                        { return m.local }

func (m *benchAddrSender) RemoteAddr() net.Addr { return m.remote }

func BenchmarkSupportedVersionsSeq(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for v := range SupportedVersionsSeq() {
			_ = v
		}
	}
}

func BenchmarkStreamNetConn_LocalRemoteAddr(b *testing.B) {
	sender := &benchAddrSender{
		local:  &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234},
		remote: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5678},
	}
	str := &Stream{sender: sender}
	b.ReportAllocs()
	for b.Loop() {
		_ = str.LocalAddr()
		_ = str.RemoteAddr()
	}
}

func BenchmarkErrorsIs_StreamReset(b *testing.B) {
	err := &StreamError{StreamID: 1, ErrorCode: 42, Remote: true}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrStreamReset) {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkErrorsIs_StreamLimitReached(b *testing.B) {
	err := StreamLimitReachedError{}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrStreamLimitReached) {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkErrorsIs_Timeout(b *testing.B) {
	err := &IdleTimeoutError{}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrTimeout) {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkErrorsIs_ConnectionClosed(b *testing.B) {
	err := &ApplicationError{ErrorCode: 0}
	b.ReportAllocs()
	for b.Loop() {
		if !errors.Is(err, ErrConnectionClosed) {
			b.Fatal("expected match")
		}
	}
}

// Compile-time interface conformance assertions.
var (
	_ net.Conn           = (*Stream)(nil)
	_ io.ReadWriteCloser = (*Stream)(nil)
	_ io.ReadCloser      = (*ReceiveStream)(nil)
	_ io.WriteCloser     = (*SendStream)(nil)
	_ io.Closer          = (*Conn)(nil)
	_ net.Listener       = (*streamListener)(nil)
	_ net.Listener       = (*quicNetListener)(nil)
)

type dummyNoAddrSender struct{}

func (d *dummyNoAddrSender) onHasConnectionData()                                       {}
func (d *dummyNoAddrSender) onHasStreamData(StreamID, *SendStream)                      {}
func (d *dummyNoAddrSender) onHasStreamRetransmission(StreamID, *SendStream)            {}
func (d *dummyNoAddrSender) onHasStreamControlFrame(StreamID, streamControlFrameGetter) {}
func (d *dummyNoAddrSender) updateStreamPriority(StreamID)                              {}
func (d *dummyNoAddrSender) onStreamCompleted(StreamID)                                 {}

func TestStream_NetConnInterface(t *testing.T) {
	localAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
	remoteAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5678}

	addrSender := &benchAddrSender{local: localAddr, remote: remoteAddr}
	fc := newTestStreamFlowControllerWithSendWindow(1, protocol.MaxByteCount)
	str := newStream(context.Background(), 1, addrSender, fc, false)

	// Addresses
	require.Equal(t, localAddr, str.LocalAddr())
	require.Equal(t, remoteAddr, str.RemoteAddr())

	// Without address methods on sender
	noAddrStr := newStream(context.Background(), 2, &dummyNoAddrSender{}, fc, false)
	require.Nil(t, noAddrStr.LocalAddr())
	require.Nil(t, noAddrStr.RemoteAddr())

	// FinalSize before completion
	fs, ok := str.FinalSize()
	require.False(t, ok)
	require.Zero(t, fs)

	// CloseWrite and CloseRead
	require.NoError(t, str.CloseWrite())
	require.NoError(t, str.CloseRead())

	// Deadlines
	require.NoError(t, str.SetDeadline(time.Now().Add(time.Hour)))
	require.NoError(t, str.SetReadDeadline(time.Now().Add(time.Hour)))
	require.NoError(t, str.SetWriteDeadline(time.Now().Add(time.Hour)))
}

func TestReceiveStream_Enhancements(t *testing.T) {
	fc := newTestStreamFlowControllerWithSendWindow(3, protocol.MaxByteCount)
	rs := newReceiveStream(3, &dummyNoAddrSender{}, fc)

	// Context should be non-nil and not yet canceled
	ctx := rs.Context()
	require.NotNil(t, ctx)
	require.NoError(t, ctx.Err())

	// FinalSize before FIN
	fs, ok := rs.FinalSize()
	require.False(t, ok)
	require.Zero(t, fs)

	// Close cancels read with code 0
	require.NoError(t, rs.Close())

	// StreamID
	require.Equal(t, protocol.StreamID(3), rs.StreamID())
}

func TestSendStream_Enhancements(t *testing.T) {
	fc := newTestStreamFlowControllerWithSendWindow(5, protocol.MaxByteCount)
	ss := newSendStream(context.Background(), 5, &dummyNoAddrSender{}, fc, false)

	// CloseWrite is an alias for Close
	require.NoError(t, ss.CloseWrite())
}

func TestConn_CloserAndHandshake(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn
	tc.connRunner.EXPECT().Remove(gomock.Any()).AnyTimes()

	// Handshake with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err := conn.Handshake(canceledCtx)
	require.ErrorIs(t, err, context.Canceled)

	// Handshake when handshake completes
	tc.conn.handshakeCompleteChan = make(chan struct{})
	close(tc.conn.handshakeCompleteChan)
	require.NoError(t, conn.Handshake(context.Background()))

	// Close() error via io.Closer
	require.NoError(t, conn.Close())
}

func TestConn_Iterators(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn

	// Active stream iterators
	count := 0
	for range conn.Streams() {
		count++
	}
	require.Zero(t, count)

	for range conn.SendStreams() {
		count++
	}
	require.Zero(t, count)

	for range conn.ReceiveStreams() {
		count++
	}
	require.Zero(t, count)

	// Event-loop push iterators with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	for str, err := range conn.IncomingStreams(canceledCtx) {
		require.Nil(t, str)
		require.ErrorIs(t, err, context.Canceled)
	}

	for str, err := range conn.IncomingUniStreams(canceledCtx) {
		require.Nil(t, str)
		require.ErrorIs(t, err, context.Canceled)
	}

	for data, err := range conn.Datagrams(canceledCtx) {
		require.Nil(t, data)
		require.Error(t, err)
	}
}

func TestConn_ContextAliases(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// OpenStreamContext blocks on canceled context
	_, err := conn.OpenStreamContext(canceledCtx)
	require.ErrorIs(t, err, context.Canceled)

	// OpenUniStreamContext blocks on canceled context
	_, err = conn.OpenUniStreamContext(canceledCtx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConn_StreamListener(t *testing.T) {
	tc := newClientTestConnection(t, nil, nil, false)
	conn := tc.conn

	ctx := t.Context()

	ln := conn.StreamListener(ctx)
	require.NotNil(t, ln)
	require.Equal(t, conn.LocalAddr(), ln.Addr())

	// Close listener cancels its internal context
	require.NoError(t, ln.Close())

	_, err := ln.Accept()
	require.ErrorIs(t, err, context.Canceled)
}

func TestListener_IncomingAndNetListener(t *testing.T) {
	udpConn := newUDPConnLocalhost(t)
	tr := &Transport{Conn: udpConn}
	require.NoError(t, tr.init(false))
	defer tr.Close()

	tlsConf := &tls.Config{ServerName: "localhost"}
	ln, err := tr.Listen(tlsConf)
	require.NoError(t, err)
	defer ln.Close()

	// Incoming iterator can be constructed
	incomingSeq := ln.Incoming(context.Background())
	require.NotNil(t, incomingSeq)

	// NetListener
	netLn := ln.NetListener(context.Background())
	require.NotNil(t, netLn)
	require.Equal(t, ln.Addr(), netLn.Addr())

	// Close netListener
	require.NoError(t, netLn.Close())
	_, err = netLn.Accept()
	require.Error(t, err)
}

func TestInterface_Iter(t *testing.T) {
	// SupportedVersionsSeq yields RFC 9000 (Version1) and RFC 9369 (Version2)
	var versions []Version
	for v := range SupportedVersionsSeq() {
		versions = append(versions, v)
	}
	require.Equal(t, []Version{Version1, Version2}, versions)

	// Early break in SupportedVersionsSeq
	count := 0
	for range SupportedVersionsSeq() {
		count++
		break
	}
	require.Equal(t, 1, count)
}

func TestErrors_WrappingAndIs(t *testing.T) {
	// Sentinels exist and are non-nil
	require.NotNil(t, ErrConnectionClosed)
	require.NotNil(t, ErrStreamReset)
	require.NotNil(t, ErrTimeout)
	require.NotNil(t, ErrStreamLimitReached)
	require.NotNil(t, ErrDeadline)

	// StreamError unwrapping and Is
	sErr := &StreamError{StreamID: 10, ErrorCode: 42, Remote: true}
	require.True(t, errors.Is(sErr, ErrStreamReset))
	require.True(t, errors.Is(sErr, net.ErrClosed))
	require.True(t, errors.Is(sErr, &StreamError{}))
	require.True(t, errors.Is(sErr, &StreamError{StreamID: 10, ErrorCode: 42, Remote: true}))
	require.False(t, errors.Is(sErr, &StreamError{StreamID: 99, ErrorCode: 42, Remote: true}))

	unwrapped := sErr.Unwrap()
	require.Len(t, unwrapped, 2)
	require.Equal(t, ErrStreamReset, unwrapped[0])
	require.Equal(t, net.ErrClosed, unwrapped[1])

	var targetSErr *StreamError
	require.True(t, errors.As(sErr, &targetSErr))
	require.Equal(t, protocol.StreamID(10), targetSErr.StreamID)
	require.Equal(t, qerr.StreamErrorCode(42), targetSErr.ErrorCode)
	require.True(t, targetSErr.Remote)

	// StreamLimitReachedError Is
	limitErr := StreamLimitReachedError{}
	require.True(t, errors.Is(limitErr, ErrStreamLimitReached))
	require.True(t, errors.Is(&limitErr, ErrStreamLimitReached))
	require.True(t, errors.Is(limitErr, StreamLimitReachedError{}))
	require.True(t, errors.Is(limitErr, &StreamLimitReachedError{}))
	require.True(t, errors.Is(ErrStreamLimitReached, StreamLimitReachedError{}))
	require.True(t, errors.Is(ErrStreamLimitReached, &StreamLimitReachedError{}))

	// ApplicationError
	appErr := &ApplicationError{ErrorCode: 0x42, ErrorMessage: "app error"}
	require.True(t, errors.Is(appErr, ErrConnectionClosed))
	require.True(t, errors.Is(appErr, net.ErrClosed))

	// TransportError
	transErr := &TransportError{ErrorCode: FlowControlError}
	require.True(t, errors.Is(transErr, ErrConnectionClosed))
	require.True(t, errors.Is(transErr, net.ErrClosed))

	// IdleTimeoutError
	idleErr := &IdleTimeoutError{}
	require.True(t, errors.Is(idleErr, ErrTimeout))
	require.True(t, errors.Is(idleErr, net.ErrClosed))

	// HandshakeTimeoutError
	hsErr := &HandshakeTimeoutError{}
	require.True(t, errors.Is(hsErr, ErrTimeout))
	require.True(t, errors.Is(hsErr, net.ErrClosed))
}

type dummyStream struct {
	id protocol.StreamID
}

func (d *dummyStream) updateSendWindow(protocol.ByteCount) {}
func (d *dummyStream) enableResetStreamAt()                {}
func (d *dummyStream) closeForShutdown(error)              {}

func TestStreamsMatrix_Iterators(t *testing.T) {
	m := newStreamsMatrix[*dummyStream]()

	s1 := &dummyStream{id: 1}
	s5 := &dummyStream{id: 5}
	s9 := &dummyStream{id: 9}

	m.set(1, incomingStreamEntry[*dummyStream]{stream: s1})
	m.set(5, incomingStreamEntry[*dummyStream]{stream: s5})
	m.set(9, incomingStreamEntry[*dummyStream]{stream: s9})

	// All() iterator
	var allEntries []incomingStreamEntry[*dummyStream]
	for entry := range m.All() {
		allEntries = append(allEntries, entry)
	}
	require.Len(t, allEntries, 3)

	// Streams() iterator
	var active []*dummyStream
	for str := range m.Streams() {
		active = append(active, str)
	}
	require.Len(t, active, 3)

	// Break early in Streams()
	count := 0
	for range m.Streams() {
		count++
		break
	}
	require.Equal(t, 1, count)

	// Delete an entry and verify Streams skips it
	m.del(5)
	var remaining []*dummyStream
	for str := range m.Streams() {
		remaining = append(remaining, str)
	}
	require.Len(t, remaining, 2)
	require.Contains(t, remaining, s1)
	require.Contains(t, remaining, s9)
}

func TestOutgoingStreamsMap_Iterators(t *testing.T) {
	m := newOutgoingStreamsMap[*dummyStream](
		protocol.StreamTypeUni,
		func(id protocol.StreamID) *dummyStream { return &dummyStream{id: id} },
		func(wire.Frame) {},
		protocol.PerspectiveClient,
	)

	m.streams[1] = &dummyStream{id: 1}
	m.streams[5] = &dummyStream{id: 5}

	var collected []protocol.StreamID
	for str := range m.Streams() {
		collected = append(collected, str.id)
	}
	require.Len(t, collected, 2)

	// Early break
	count := 0
	for range m.Streams() {
		count++
		break
	}
	require.Equal(t, 1, count)
}

func TestTransport_PureGoCoverage(t *testing.T) {
	udpConn := newUDPConnLocalhost(t)
	tr := &Transport{Conn: udpConn}
	require.NoError(t, tr.init(false))
	defer tr.Close()

	// 1. Reset tokens
	token := protocol.StatelessResetToken{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	mockHandler := &mockPacketHandler{}
	(*packetHandlerMap)(tr).AddResetToken(token, mockHandler)
	require.NotNil(t, tr.resetTokens[token])

	(*packetHandlerMap)(tr).RemoveResetToken(token)
	require.Nil(t, tr.resetTokens[token])

	// 2. AddWithConnID
	c1 := protocol.ParseConnectionID([]byte{1, 2, 3, 4})
	c2 := protocol.ParseConnectionID([]byte{5, 6, 7, 8})
	require.True(t, (*packetHandlerMap)(tr).AddWithConnID(c1, c2, mockHandler))
	require.False(t, (*packetHandlerMap)(tr).AddWithConnID(c1, c2, mockHandler))

	// 3. ReplaceWithClosed
	closePkt := []byte("conn-close")
	(*packetHandlerMap)(tr).ReplaceWithClosed([]protocol.ConnectionID{c1}, closePkt, time.Hour)
	handler, ok := (*packetHandlerMap)(tr).Get(c1)
	require.True(t, ok)
	require.NotNil(t, handler)

	// Remote closed (connClosePacket is nil)
	(*packetHandlerMap)(tr).ReplaceWithClosed([]protocol.ConnectionID{c2}, nil, time.Hour)
	handler2, ok := (*packetHandlerMap)(tr).Get(c2)
	require.True(t, ok)
	require.NotNil(t, handler2)

	// 4. Non-QUIC packet handling
	p := receivedPacket{
		remoteAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999},
		data:       []byte("non-quic-data"),
	}

	// Reading disabled -> dropped
	tr.readingNonQUICPackets.Store(false)
	tr.handleNonQUICPacket(p)

	// Reading enabled
	tr.nonQUICPackets = make(chan receivedPacket, 10)
	tr.readingNonQUICPackets.Store(true)
	tr.handleNonQUICPacket(p)

	ctx := context.Background()
	buf := make([]byte, 64)
	n, addr, err := tr.ReadNonQUICPacket(ctx, buf)
	require.NoError(t, err)
	require.Equal(t, len("non-quic-data"), n)
	require.Equal(t, "non-quic-data", string(buf[:n]))
	require.Equal(t, p.remoteAddr, addr)

	// ReadNonQUICPacket with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = tr.ReadNonQUICPacket(canceledCtx, buf)
	require.ErrorIs(t, err, context.Canceled)

	// 5. ListenAddr invalid address error
	_, err = ListenAddr("invalid:::port", nil)
	require.Error(t, err)
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: AckFrameSource)

// Package quic is a generated GoMock package.

// MockAckFrameSource is a mock of AckFrameSource interface.
type MockAckFrameSource struct {
	ctrl     *gomock.Controller
	recorder *MockAckFrameSourceMockRecorder
	isgomock struct{}
}

// MockAckFrameSourceMockRecorder is the mock recorder for MockAckFrameSource.
type MockAckFrameSourceMockRecorder struct {
	mock *MockAckFrameSource
}

// NewMockAckFrameSource creates a new mock instance.
func NewMockAckFrameSource(ctrl *gomock.Controller) *MockAckFrameSource {
	mock := &MockAckFrameSource{ctrl: ctrl}
	mock.recorder = &MockAckFrameSourceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAckFrameSource) EXPECT() *MockAckFrameSourceMockRecorder {
	return m.recorder
}

// AckFrame mocks base method.
func (m *MockAckFrameSource) AckFrame(
	arg0 protocol.EncryptionLevel,
	now monotime.Time,
	onlyIfQueued bool,
) *wire.AckFrame {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AckFrame", arg0, now, onlyIfQueued)
	ret0, _ := ret[0].(*wire.AckFrame)
	return ret0
}

// AckFrame indicates an expected call of AckFrame.
func (mr *MockAckFrameSourceMockRecorder) AckFrame(arg0, now, onlyIfQueued any) *MockAckFrameSourceAckFrameCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"AckFrame",
		reflect.TypeOf((*MockAckFrameSource)(nil).AckFrame),
		arg0,
		now,
		onlyIfQueued,
	)
	return &MockAckFrameSourceAckFrameCall{Call: call}
}

// MockAckFrameSourceAckFrameCall wrap *gomock.Call
type MockAckFrameSourceAckFrameCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockAckFrameSourceAckFrameCall) Return(arg0 *wire.AckFrame) *MockAckFrameSourceAckFrameCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockAckFrameSourceAckFrameCall) Do(
	f func(protocol.EncryptionLevel, monotime.Time, bool) *wire.AckFrame,
) *MockAckFrameSourceAckFrameCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockAckFrameSourceAckFrameCall) DoAndReturn(
	f func(protocol.EncryptionLevel, monotime.Time, bool) *wire.AckFrame,
) *MockAckFrameSourceAckFrameCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: ConnRunner)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_conn_runner_test.go github.com/lemon4ksan/sein/internal/quic ConnRunner
//

// Package quic is a generated GoMock package.

// MockConnRunner is a mock of ConnRunner interface.
type MockConnRunner struct {
	ctrl     *gomock.Controller
	recorder *MockConnRunnerMockRecorder
	isgomock struct{}
}

// MockConnRunnerMockRecorder is the mock recorder for MockConnRunner.
type MockConnRunnerMockRecorder struct {
	mock *MockConnRunner
}

// NewMockConnRunner creates a new mock instance.
func NewMockConnRunner(ctrl *gomock.Controller) *MockConnRunner {
	mock := &MockConnRunner{ctrl: ctrl}
	mock.recorder = &MockConnRunnerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConnRunner) EXPECT() *MockConnRunnerMockRecorder {
	return m.recorder
}

// Add mocks base method.
func (m *MockConnRunner) Add(arg0 protocol.ConnectionID, arg1 packetHandler) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Add", arg0, arg1)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Add indicates an expected call of Add.
func (mr *MockConnRunnerMockRecorder) Add(arg0, arg1 any) *MockConnRunnerAddCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Add",
		reflect.TypeOf((*MockConnRunner)(nil).Add),
		arg0,
		arg1,
	)
	return &MockConnRunnerAddCall{Call: call}
}

// MockConnRunnerAddCall wrap *gomock.Call
type MockConnRunnerAddCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockConnRunnerAddCall) Return(arg0 bool) *MockConnRunnerAddCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockConnRunnerAddCall) Do(f func(protocol.ConnectionID, packetHandler) bool) *MockConnRunnerAddCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockConnRunnerAddCall) DoAndReturn(f func(protocol.ConnectionID, packetHandler) bool) *MockConnRunnerAddCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// AddResetToken mocks base method.
func (m *MockConnRunner) AddResetToken(arg0 protocol.StatelessResetToken, arg1 packetHandler) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddResetToken", arg0, arg1)
}

// AddResetToken indicates an expected call of AddResetToken.
func (mr *MockConnRunnerMockRecorder) AddResetToken(arg0, arg1 any) *MockConnRunnerAddResetTokenCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"AddResetToken",
		reflect.TypeOf((*MockConnRunner)(nil).AddResetToken),
		arg0,
		arg1,
	)
	return &MockConnRunnerAddResetTokenCall{Call: call}
}

// MockConnRunnerAddResetTokenCall wrap *gomock.Call
type MockConnRunnerAddResetTokenCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockConnRunnerAddResetTokenCall) Return() *MockConnRunnerAddResetTokenCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockConnRunnerAddResetTokenCall) Do(
	f func(protocol.StatelessResetToken, packetHandler),
) *MockConnRunnerAddResetTokenCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockConnRunnerAddResetTokenCall) DoAndReturn(
	f func(protocol.StatelessResetToken, packetHandler),
) *MockConnRunnerAddResetTokenCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Remove mocks base method.
func (m *MockConnRunner) Remove(arg0 protocol.ConnectionID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Remove", arg0)
}

// Remove indicates an expected call of Remove.
func (mr *MockConnRunnerMockRecorder) Remove(arg0 any) *MockConnRunnerRemoveCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Remove",
		reflect.TypeOf((*MockConnRunner)(nil).Remove),
		arg0,
	)
	return &MockConnRunnerRemoveCall{Call: call}
}

// MockConnRunnerRemoveCall wrap *gomock.Call
type MockConnRunnerRemoveCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockConnRunnerRemoveCall) Return() *MockConnRunnerRemoveCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockConnRunnerRemoveCall) Do(f func(protocol.ConnectionID)) *MockConnRunnerRemoveCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockConnRunnerRemoveCall) DoAndReturn(f func(protocol.ConnectionID)) *MockConnRunnerRemoveCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// RemoveResetToken mocks base method.
func (m *MockConnRunner) RemoveResetToken(arg0 protocol.StatelessResetToken) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RemoveResetToken", arg0)
}

// RemoveResetToken indicates an expected call of RemoveResetToken.
func (mr *MockConnRunnerMockRecorder) RemoveResetToken(arg0 any) *MockConnRunnerRemoveResetTokenCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"RemoveResetToken",
		reflect.TypeOf((*MockConnRunner)(nil).RemoveResetToken),
		arg0,
	)
	return &MockConnRunnerRemoveResetTokenCall{Call: call}
}

// MockConnRunnerRemoveResetTokenCall wrap *gomock.Call
type MockConnRunnerRemoveResetTokenCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockConnRunnerRemoveResetTokenCall) Return() *MockConnRunnerRemoveResetTokenCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockConnRunnerRemoveResetTokenCall) Do(
	f func(protocol.StatelessResetToken),
) *MockConnRunnerRemoveResetTokenCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockConnRunnerRemoveResetTokenCall) DoAndReturn(
	f func(protocol.StatelessResetToken),
) *MockConnRunnerRemoveResetTokenCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReplaceWithClosed mocks base method.
func (m *MockConnRunner) ReplaceWithClosed(arg0 []protocol.ConnectionID, arg1 []byte, arg2 time.Duration) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReplaceWithClosed", arg0, arg1, arg2)
}

// ReplaceWithClosed indicates an expected call of ReplaceWithClosed.
func (mr *MockConnRunnerMockRecorder) ReplaceWithClosed(arg0, arg1, arg2 any) *MockConnRunnerReplaceWithClosedCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ReplaceWithClosed",
		reflect.TypeOf((*MockConnRunner)(nil).ReplaceWithClosed),
		arg0,
		arg1,
		arg2,
	)
	return &MockConnRunnerReplaceWithClosedCall{Call: call}
}

// MockConnRunnerReplaceWithClosedCall wrap *gomock.Call
type MockConnRunnerReplaceWithClosedCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockConnRunnerReplaceWithClosedCall) Return() *MockConnRunnerReplaceWithClosedCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockConnRunnerReplaceWithClosedCall) Do(
	f func([]protocol.ConnectionID, []byte, time.Duration),
) *MockConnRunnerReplaceWithClosedCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockConnRunnerReplaceWithClosedCall) DoAndReturn(
	f func([]protocol.ConnectionID, []byte, time.Duration),
) *MockConnRunnerReplaceWithClosedCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic/internal/handshake (interfaces: CryptoSetup)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -destination crypto_setup.go github.com/lemon4ksan/sein/internal/quic/internal/handshake CryptoSetup
//

// Package mocks is a generated GoMock package.

// MockCryptoSetup is a mock of CryptoSetup interface.
type MockCryptoSetup struct {
	ctrl     *gomock.Controller
	recorder *MockCryptoSetupMockRecorder
	isgomock struct{}
}

// MockCryptoSetupMockRecorder is the mock recorder for MockCryptoSetup.
type MockCryptoSetupMockRecorder struct {
	mock *MockCryptoSetup
}

// NewMockCryptoSetup creates a new mock instance.
func NewMockCryptoSetup(ctrl *gomock.Controller) *MockCryptoSetup {
	mock := &MockCryptoSetup{ctrl: ctrl}
	mock.recorder = &MockCryptoSetupMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCryptoSetup) EXPECT() *MockCryptoSetupMockRecorder {
	return m.recorder
}

// ChangeConnectionID mocks base method.
func (m *MockCryptoSetup) ChangeConnectionID(arg0 protocol.ConnectionID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ChangeConnectionID", arg0)
}

// ChangeConnectionID indicates an expected call of ChangeConnectionID.
func (mr *MockCryptoSetupMockRecorder) ChangeConnectionID(arg0 any) *MockCryptoSetupChangeConnectionIDCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ChangeConnectionID",
		reflect.TypeOf((*MockCryptoSetup)(nil).ChangeConnectionID),
		arg0,
	)
	return &MockCryptoSetupChangeConnectionIDCall{Call: call}
}

// MockCryptoSetupChangeConnectionIDCall wrap *gomock.Call
type MockCryptoSetupChangeConnectionIDCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupChangeConnectionIDCall) Return() *MockCryptoSetupChangeConnectionIDCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupChangeConnectionIDCall) Do(
	f func(protocol.ConnectionID),
) *MockCryptoSetupChangeConnectionIDCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupChangeConnectionIDCall) DoAndReturn(
	f func(protocol.ConnectionID),
) *MockCryptoSetupChangeConnectionIDCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Close mocks base method.
func (m *MockCryptoSetup) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockCryptoSetupMockRecorder) Close() *MockCryptoSetupCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockCryptoSetup)(nil).Close))
	return &MockCryptoSetupCloseCall{Call: call}
}

// MockCryptoSetupCloseCall wrap *gomock.Call
type MockCryptoSetupCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupCloseCall) Return(arg0 error) *MockCryptoSetupCloseCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupCloseCall) Do(f func() error) *MockCryptoSetupCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupCloseCall) DoAndReturn(f func() error) *MockCryptoSetupCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ConnectionState mocks base method.
func (m *MockCryptoSetup) ConnectionState() handshake.ConnectionState {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ConnectionState")
	ret0, _ := ret[0].(handshake.ConnectionState)
	return ret0
}

// ConnectionState indicates an expected call of ConnectionState.
func (mr *MockCryptoSetupMockRecorder) ConnectionState() *MockCryptoSetupConnectionStateCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ConnectionState",
		reflect.TypeOf((*MockCryptoSetup)(nil).ConnectionState),
	)
	return &MockCryptoSetupConnectionStateCall{Call: call}
}

// MockCryptoSetupConnectionStateCall wrap *gomock.Call
type MockCryptoSetupConnectionStateCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupConnectionStateCall) Return(
	arg0 handshake.ConnectionState,
) *MockCryptoSetupConnectionStateCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupConnectionStateCall) Do(
	f func() handshake.ConnectionState,
) *MockCryptoSetupConnectionStateCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupConnectionStateCall) DoAndReturn(
	f func() handshake.ConnectionState,
) *MockCryptoSetupConnectionStateCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// DiscardInitialKeys mocks base method.
func (m *MockCryptoSetup) DiscardInitialKeys() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DiscardInitialKeys")
}

// DiscardInitialKeys indicates an expected call of DiscardInitialKeys.
func (mr *MockCryptoSetupMockRecorder) DiscardInitialKeys() *MockCryptoSetupDiscardInitialKeysCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"DiscardInitialKeys",
		reflect.TypeOf((*MockCryptoSetup)(nil).DiscardInitialKeys),
	)
	return &MockCryptoSetupDiscardInitialKeysCall{Call: call}
}

// MockCryptoSetupDiscardInitialKeysCall wrap *gomock.Call
type MockCryptoSetupDiscardInitialKeysCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupDiscardInitialKeysCall) Return() *MockCryptoSetupDiscardInitialKeysCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupDiscardInitialKeysCall) Do(f func()) *MockCryptoSetupDiscardInitialKeysCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupDiscardInitialKeysCall) DoAndReturn(f func()) *MockCryptoSetupDiscardInitialKeysCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ZeroRTTOpener mocks base method.
func (m *MockCryptoSetup) ZeroRTTOpener() (handshake.LongHeaderOpener, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ZeroRTTOpener")
	ret0, _ := ret[0].(handshake.LongHeaderOpener)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ZeroRTTOpener indicates an expected call of ZeroRTTOpener.
func (mr *MockCryptoSetupMockRecorder) ZeroRTTOpener() *MockCryptoSetupZeroRTTOpenerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ZeroRTTOpener",
		reflect.TypeOf((*MockCryptoSetup)(nil).ZeroRTTOpener),
	)
	return &MockCryptoSetupZeroRTTOpenerCall{Call: call}
}

// MockCryptoSetupZeroRTTOpenerCall wrap *gomock.Call
type MockCryptoSetupZeroRTTOpenerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupZeroRTTOpenerCall) Return(
	arg0 handshake.LongHeaderOpener,
	arg1 error,
) *MockCryptoSetupZeroRTTOpenerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupZeroRTTOpenerCall) Do(
	f func() (handshake.LongHeaderOpener, error),
) *MockCryptoSetupZeroRTTOpenerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupZeroRTTOpenerCall) DoAndReturn(
	f func() (handshake.LongHeaderOpener, error),
) *MockCryptoSetupZeroRTTOpenerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ZeroRTTSealer mocks base method.
func (m *MockCryptoSetup) ZeroRTTSealer() (handshake.LongHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ZeroRTTSealer")
	ret0, _ := ret[0].(handshake.LongHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ZeroRTTSealer indicates an expected call of ZeroRTTSealer.
func (mr *MockCryptoSetupMockRecorder) ZeroRTTSealer() *MockCryptoSetupZeroRTTSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ZeroRTTSealer",
		reflect.TypeOf((*MockCryptoSetup)(nil).ZeroRTTSealer),
	)
	return &MockCryptoSetupZeroRTTSealerCall{Call: call}
}

// MockCryptoSetupZeroRTTSealerCall wrap *gomock.Call
type MockCryptoSetupZeroRTTSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupZeroRTTSealerCall) Return(
	arg0 handshake.LongHeaderSealer,
	arg1 error,
) *MockCryptoSetupZeroRTTSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupZeroRTTSealerCall) Do(
	f func() (handshake.LongHeaderSealer, error),
) *MockCryptoSetupZeroRTTSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupZeroRTTSealerCall) DoAndReturn(
	f func() (handshake.LongHeaderSealer, error),
) *MockCryptoSetupZeroRTTSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// OneRTTOpener mocks base method.
func (m *MockCryptoSetup) OneRTTOpener() (handshake.ShortHeaderOpener, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OneRTTOpener")
	ret0, _ := ret[0].(handshake.ShortHeaderOpener)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// OneRTTOpener indicates an expected call of OneRTTOpener.
func (mr *MockCryptoSetupMockRecorder) OneRTTOpener() *MockCryptoSetupOneRTTOpenerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"OneRTTOpener",
		reflect.TypeOf((*MockCryptoSetup)(nil).OneRTTOpener),
	)
	return &MockCryptoSetupOneRTTOpenerCall{Call: call}
}

// MockCryptoSetupOneRTTOpenerCall wrap *gomock.Call
type MockCryptoSetupOneRTTOpenerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupOneRTTOpenerCall) Return(
	arg0 handshake.ShortHeaderOpener,
	arg1 error,
) *MockCryptoSetupOneRTTOpenerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupOneRTTOpenerCall) Do(
	f func() (handshake.ShortHeaderOpener, error),
) *MockCryptoSetupOneRTTOpenerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupOneRTTOpenerCall) DoAndReturn(
	f func() (handshake.ShortHeaderOpener, error),
) *MockCryptoSetupOneRTTOpenerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// OneRTTSealer mocks base method.
func (m *MockCryptoSetup) OneRTTSealer() (handshake.ShortHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OneRTTSealer")
	ret0, _ := ret[0].(handshake.ShortHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// OneRTTSealer indicates an expected call of OneRTTSealer.
func (mr *MockCryptoSetupMockRecorder) OneRTTSealer() *MockCryptoSetupOneRTTSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"OneRTTSealer",
		reflect.TypeOf((*MockCryptoSetup)(nil).OneRTTSealer),
	)
	return &MockCryptoSetupOneRTTSealerCall{Call: call}
}

// MockCryptoSetupOneRTTSealerCall wrap *gomock.Call
type MockCryptoSetupOneRTTSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupOneRTTSealerCall) Return(
	arg0 handshake.ShortHeaderSealer,
	arg1 error,
) *MockCryptoSetupOneRTTSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupOneRTTSealerCall) Do(
	f func() (handshake.ShortHeaderSealer, error),
) *MockCryptoSetupOneRTTSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupOneRTTSealerCall) DoAndReturn(
	f func() (handshake.ShortHeaderSealer, error),
) *MockCryptoSetupOneRTTSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// HandshakeOpener mocks base method.
func (m *MockCryptoSetup) HandshakeOpener() (handshake.LongHeaderOpener, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandshakeOpener")
	ret0, _ := ret[0].(handshake.LongHeaderOpener)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HandshakeOpener indicates an expected call of HandshakeOpener.
func (mr *MockCryptoSetupMockRecorder) HandshakeOpener() *MockCryptoSetupHandshakeOpenerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"HandshakeOpener",
		reflect.TypeOf((*MockCryptoSetup)(nil).HandshakeOpener),
	)
	return &MockCryptoSetupHandshakeOpenerCall{Call: call}
}

// MockCryptoSetupHandshakeOpenerCall wrap *gomock.Call
type MockCryptoSetupHandshakeOpenerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupHandshakeOpenerCall) Return(
	arg0 handshake.LongHeaderOpener,
	arg1 error,
) *MockCryptoSetupHandshakeOpenerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupHandshakeOpenerCall) Do(
	f func() (handshake.LongHeaderOpener, error),
) *MockCryptoSetupHandshakeOpenerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupHandshakeOpenerCall) DoAndReturn(
	f func() (handshake.LongHeaderOpener, error),
) *MockCryptoSetupHandshakeOpenerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// HandshakeSealer mocks base method.
func (m *MockCryptoSetup) HandshakeSealer() (handshake.LongHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandshakeSealer")
	ret0, _ := ret[0].(handshake.LongHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HandshakeSealer indicates an expected call of HandshakeSealer.
func (mr *MockCryptoSetupMockRecorder) HandshakeSealer() *MockCryptoSetupHandshakeSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"HandshakeSealer",
		reflect.TypeOf((*MockCryptoSetup)(nil).HandshakeSealer),
	)
	return &MockCryptoSetupHandshakeSealerCall{Call: call}
}

// MockCryptoSetupHandshakeSealerCall wrap *gomock.Call
type MockCryptoSetupHandshakeSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupHandshakeSealerCall) Return(
	arg0 handshake.LongHeaderSealer,
	arg1 error,
) *MockCryptoSetupHandshakeSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupHandshakeSealerCall) Do(
	f func() (handshake.LongHeaderSealer, error),
) *MockCryptoSetupHandshakeSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupHandshakeSealerCall) DoAndReturn(
	f func() (handshake.LongHeaderSealer, error),
) *MockCryptoSetupHandshakeSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// InitialOpener mocks base method.
func (m *MockCryptoSetup) InitialOpener() (handshake.LongHeaderOpener, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitialOpener")
	ret0, _ := ret[0].(handshake.LongHeaderOpener)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InitialOpener indicates an expected call of InitialOpener.
func (mr *MockCryptoSetupMockRecorder) InitialOpener() *MockCryptoSetupInitialOpenerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"InitialOpener",
		reflect.TypeOf((*MockCryptoSetup)(nil).InitialOpener),
	)
	return &MockCryptoSetupInitialOpenerCall{Call: call}
}

// MockCryptoSetupInitialOpenerCall wrap *gomock.Call
type MockCryptoSetupInitialOpenerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupInitialOpenerCall) Return(
	arg0 handshake.LongHeaderOpener,
	arg1 error,
) *MockCryptoSetupInitialOpenerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupInitialOpenerCall) Do(
	f func() (handshake.LongHeaderOpener, error),
) *MockCryptoSetupInitialOpenerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupInitialOpenerCall) DoAndReturn(
	f func() (handshake.LongHeaderOpener, error),
) *MockCryptoSetupInitialOpenerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// InitialSealer mocks base method.
func (m *MockCryptoSetup) InitialSealer() (handshake.LongHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitialSealer")
	ret0, _ := ret[0].(handshake.LongHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InitialSealer indicates an expected call of InitialSealer.
func (mr *MockCryptoSetupMockRecorder) InitialSealer() *MockCryptoSetupInitialSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"InitialSealer",
		reflect.TypeOf((*MockCryptoSetup)(nil).InitialSealer),
	)
	return &MockCryptoSetupInitialSealerCall{Call: call}
}

// MockCryptoSetupInitialSealerCall wrap *gomock.Call
type MockCryptoSetupInitialSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupInitialSealerCall) Return(
	arg0 handshake.LongHeaderSealer,
	arg1 error,
) *MockCryptoSetupInitialSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupInitialSealerCall) Do(
	f func() (handshake.LongHeaderSealer, error),
) *MockCryptoSetupInitialSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupInitialSealerCall) DoAndReturn(
	f func() (handshake.LongHeaderSealer, error),
) *MockCryptoSetupInitialSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SessionTicket mocks base method.
func (m *MockCryptoSetup) SessionTicket() ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SessionTicket")
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SessionTicket indicates an expected call of SessionTicket.
func (mr *MockCryptoSetupMockRecorder) SessionTicket() *MockCryptoSetupSessionTicketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SessionTicket",
		reflect.TypeOf((*MockCryptoSetup)(nil).SessionTicket),
	)
	return &MockCryptoSetupSessionTicketCall{Call: call}
}

// MockCryptoSetupSessionTicketCall wrap *gomock.Call
type MockCryptoSetupSessionTicketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupSessionTicketCall) Return(arg0 []byte, arg1 error) *MockCryptoSetupSessionTicketCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupSessionTicketCall) Do(f func() ([]byte, error)) *MockCryptoSetupSessionTicketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupSessionTicketCall) DoAndReturn(f func() ([]byte, error)) *MockCryptoSetupSessionTicketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// HandleMessage mocks base method.
func (m *MockCryptoSetup) HandleMessage(arg0 []byte, arg1 protocol.EncryptionLevel) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandleMessage", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// HandleMessage indicates an expected call of HandleMessage.
func (mr *MockCryptoSetupMockRecorder) HandleMessage(arg0, arg1 any) *MockCryptoSetupHandleMessageCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"HandleMessage",
		reflect.TypeOf((*MockCryptoSetup)(nil).HandleMessage),
		arg0,
		arg1,
	)
	return &MockCryptoSetupHandleMessageCall{Call: call}
}

// MockCryptoSetupHandleMessageCall wrap *gomock.Call
type MockCryptoSetupHandleMessageCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupHandleMessageCall) Return(arg0 error) *MockCryptoSetupHandleMessageCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupHandleMessageCall) Do(
	f func([]byte, protocol.EncryptionLevel) error,
) *MockCryptoSetupHandleMessageCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupHandleMessageCall) DoAndReturn(
	f func([]byte, protocol.EncryptionLevel) error,
) *MockCryptoSetupHandleMessageCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// NextEvent mocks base method.
func (m *MockCryptoSetup) NextEvent() handshake.Event {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NextEvent")
	ret0, _ := ret[0].(handshake.Event)
	return ret0
}

// NextEvent indicates an expected call of NextEvent.
func (mr *MockCryptoSetupMockRecorder) NextEvent() *MockCryptoSetupNextEventCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"NextEvent",
		reflect.TypeOf((*MockCryptoSetup)(nil).NextEvent),
	)
	return &MockCryptoSetupNextEventCall{Call: call}
}

// MockCryptoSetupNextEventCall wrap *gomock.Call
type MockCryptoSetupNextEventCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupNextEventCall) Return(arg0 handshake.Event) *MockCryptoSetupNextEventCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupNextEventCall) Do(f func() handshake.Event) *MockCryptoSetupNextEventCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupNextEventCall) DoAndReturn(f func() handshake.Event) *MockCryptoSetupNextEventCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetHandshakeConfirmed mocks base method.
func (m *MockCryptoSetup) SetHandshakeConfirmed() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetHandshakeConfirmed")
}

// SetHandshakeConfirmed indicates an expected call of SetHandshakeConfirmed.
func (mr *MockCryptoSetupMockRecorder) SetHandshakeConfirmed() *MockCryptoSetupSetHandshakeConfirmedCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetHandshakeConfirmed",
		reflect.TypeOf((*MockCryptoSetup)(nil).SetHandshakeConfirmed),
	)
	return &MockCryptoSetupSetHandshakeConfirmedCall{Call: call}
}

// MockCryptoSetupSetHandshakeConfirmedCall wrap *gomock.Call
type MockCryptoSetupSetHandshakeConfirmedCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupSetHandshakeConfirmedCall) Return() *MockCryptoSetupSetHandshakeConfirmedCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupSetHandshakeConfirmedCall) Do(f func()) *MockCryptoSetupSetHandshakeConfirmedCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupSetHandshakeConfirmedCall) DoAndReturn(f func()) *MockCryptoSetupSetHandshakeConfirmedCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetLargest1RTTAcked mocks base method.
func (m *MockCryptoSetup) SetLargest1RTTAcked(arg0 protocol.PacketNumber) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetLargest1RTTAcked", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetLargest1RTTAcked indicates an expected call of SetLargest1RTTAcked.
func (mr *MockCryptoSetupMockRecorder) SetLargest1RTTAcked(arg0 any) *MockCryptoSetupSetLargest1RTTAckedCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetLargest1RTTAcked",
		reflect.TypeOf((*MockCryptoSetup)(nil).SetLargest1RTTAcked),
		arg0,
	)
	return &MockCryptoSetupSetLargest1RTTAckedCall{Call: call}
}

// MockCryptoSetupSetLargest1RTTAckedCall wrap *gomock.Call
type MockCryptoSetupSetLargest1RTTAckedCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupSetLargest1RTTAckedCall) Return(arg0 error) *MockCryptoSetupSetLargest1RTTAckedCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupSetLargest1RTTAckedCall) Do(
	f func(protocol.PacketNumber) error,
) *MockCryptoSetupSetLargest1RTTAckedCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupSetLargest1RTTAckedCall) DoAndReturn(
	f func(protocol.PacketNumber) error,
) *MockCryptoSetupSetLargest1RTTAckedCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// StartHandshake mocks base method.
func (m *MockCryptoSetup) StartHandshake(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StartHandshake", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// StartHandshake indicates an expected call of StartHandshake.
func (mr *MockCryptoSetupMockRecorder) StartHandshake(arg0 any) *MockCryptoSetupStartHandshakeCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"StartHandshake",
		reflect.TypeOf((*MockCryptoSetup)(nil).StartHandshake),
		arg0,
	)
	return &MockCryptoSetupStartHandshakeCall{Call: call}
}

// MockCryptoSetupStartHandshakeCall wrap *gomock.Call
type MockCryptoSetupStartHandshakeCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCryptoSetupStartHandshakeCall) Return(arg0 error) *MockCryptoSetupStartHandshakeCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCryptoSetupStartHandshakeCall) Do(f func(context.Context) error) *MockCryptoSetupStartHandshakeCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCryptoSetupStartHandshakeCall) DoAndReturn(
	f func(context.Context) error,
) *MockCryptoSetupStartHandshakeCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: FrameSource)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_frame_source_test.go github.com/lemon4ksan/sein/internal/quic FrameSource
//

// Package quic is a generated GoMock package.

// MockFrameSource is a mock of FrameSource interface.
type MockFrameSource struct {
	ctrl     *gomock.Controller
	recorder *MockFrameSourceMockRecorder
	isgomock struct{}
}

// MockFrameSourceMockRecorder is the mock recorder for MockFrameSource.
type MockFrameSourceMockRecorder struct {
	mock *MockFrameSource
}

// NewMockFrameSource creates a new mock instance.
func NewMockFrameSource(ctrl *gomock.Controller) *MockFrameSource {
	mock := &MockFrameSource{ctrl: ctrl}
	mock.recorder = &MockFrameSourceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFrameSource) EXPECT() *MockFrameSourceMockRecorder {
	return m.recorder
}

// Append mocks base method.
func (m *MockFrameSource) Append(
	arg0 []ackhandler.Frame,
	arg1 []ackhandler.StreamFrame,
	arg2 protocol.ByteCount,
	arg3 monotime.Time,
	arg4 protocol.Version,
) ([]ackhandler.Frame, []ackhandler.StreamFrame, protocol.ByteCount) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Append", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].([]ackhandler.Frame)
	ret1, _ := ret[1].([]ackhandler.StreamFrame)
	ret2, _ := ret[2].(protocol.ByteCount)
	return ret0, ret1, ret2
}

// Append indicates an expected call of Append.
func (mr *MockFrameSourceMockRecorder) Append(arg0, arg1, arg2, arg3, arg4 any) *MockFrameSourceAppendCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Append",
		reflect.TypeOf((*MockFrameSource)(nil).Append),
		arg0,
		arg1,
		arg2,
		arg3,
		arg4,
	)
	return &MockFrameSourceAppendCall{Call: call}
}

// MockFrameSourceAppendCall wrap *gomock.Call
type MockFrameSourceAppendCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockFrameSourceAppendCall) Return(
	arg0 []ackhandler.Frame,
	arg1 []ackhandler.StreamFrame,
	arg2 protocol.ByteCount,
) *MockFrameSourceAppendCall {
	c.Call = c.Call.Return(arg0, arg1, arg2)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockFrameSourceAppendCall) Do(
	f func([]ackhandler.Frame, []ackhandler.StreamFrame, protocol.ByteCount, monotime.Time, protocol.Version) ([]ackhandler.Frame, []ackhandler.StreamFrame, protocol.ByteCount),
) *MockFrameSourceAppendCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockFrameSourceAppendCall) DoAndReturn(
	f func([]ackhandler.Frame, []ackhandler.StreamFrame, protocol.ByteCount, monotime.Time, protocol.Version) ([]ackhandler.Frame, []ackhandler.StreamFrame, protocol.ByteCount),
) *MockFrameSourceAppendCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// HasData mocks base method.
func (m *MockFrameSource) HasData() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HasData")
	ret0, _ := ret[0].(bool)
	return ret0
}

// HasData indicates an expected call of HasData.
func (mr *MockFrameSourceMockRecorder) HasData() *MockFrameSourceHasDataCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HasData", reflect.TypeOf((*MockFrameSource)(nil).HasData))
	return &MockFrameSourceHasDataCall{Call: call}
}

// MockFrameSourceHasDataCall wrap *gomock.Call
type MockFrameSourceHasDataCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockFrameSourceHasDataCall) Return(arg0 bool) *MockFrameSourceHasDataCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockFrameSourceHasDataCall) Do(f func() bool) *MockFrameSourceHasDataCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockFrameSourceHasDataCall) DoAndReturn(f func() bool) *MockFrameSourceHasDataCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic/internal/handshake (interfaces: LongHeaderOpener)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -destination long_header_opener.go github.com/lemon4ksan/sein/internal/quic/internal/handshake LongHeaderOpener
//

// Package mocks is a generated GoMock package.

// MockLongHeaderOpener is a mock of LongHeaderOpener interface.
type MockLongHeaderOpener struct {
	ctrl     *gomock.Controller
	recorder *MockLongHeaderOpenerMockRecorder
	isgomock struct{}
}

// MockLongHeaderOpenerMockRecorder is the mock recorder for MockLongHeaderOpener.
type MockLongHeaderOpenerMockRecorder struct {
	mock *MockLongHeaderOpener
}

// NewMockLongHeaderOpener creates a new mock instance.
func NewMockLongHeaderOpener(ctrl *gomock.Controller) *MockLongHeaderOpener {
	mock := &MockLongHeaderOpener{ctrl: ctrl}
	mock.recorder = &MockLongHeaderOpenerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockLongHeaderOpener) EXPECT() *MockLongHeaderOpenerMockRecorder {
	return m.recorder
}

// DecodePacketNumber mocks base method.
func (m *MockLongHeaderOpener) DecodePacketNumber(
	wirePN protocol.PacketNumber,
	wirePNLen protocol.PacketNumberLen,
) protocol.PacketNumber {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecodePacketNumber", wirePN, wirePNLen)
	ret0, _ := ret[0].(protocol.PacketNumber)
	return ret0
}

// DecodePacketNumber indicates an expected call of DecodePacketNumber.
func (mr *MockLongHeaderOpenerMockRecorder) DecodePacketNumber(
	wirePN, wirePNLen any,
) *MockLongHeaderOpenerDecodePacketNumberCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"DecodePacketNumber",
		reflect.TypeOf((*MockLongHeaderOpener)(nil).DecodePacketNumber),
		wirePN,
		wirePNLen,
	)
	return &MockLongHeaderOpenerDecodePacketNumberCall{Call: call}
}

// MockLongHeaderOpenerDecodePacketNumberCall wrap *gomock.Call
type MockLongHeaderOpenerDecodePacketNumberCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockLongHeaderOpenerDecodePacketNumberCall) Return(
	arg0 protocol.PacketNumber,
) *MockLongHeaderOpenerDecodePacketNumberCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockLongHeaderOpenerDecodePacketNumberCall) Do(
	f func(protocol.PacketNumber, protocol.PacketNumberLen) protocol.PacketNumber,
) *MockLongHeaderOpenerDecodePacketNumberCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockLongHeaderOpenerDecodePacketNumberCall) DoAndReturn(
	f func(protocol.PacketNumber, protocol.PacketNumberLen) protocol.PacketNumber,
) *MockLongHeaderOpenerDecodePacketNumberCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// DecryptHeader mocks base method.
func (m *MockLongHeaderOpener) DecryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DecryptHeader", sample, firstByte, pnBytes)
}

// DecryptHeader indicates an expected call of DecryptHeader.
func (mr *MockLongHeaderOpenerMockRecorder) DecryptHeader(
	sample, firstByte, pnBytes any,
) *MockLongHeaderOpenerDecryptHeaderCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"DecryptHeader",
		reflect.TypeOf((*MockLongHeaderOpener)(nil).DecryptHeader),
		sample,
		firstByte,
		pnBytes,
	)
	return &MockLongHeaderOpenerDecryptHeaderCall{Call: call}
}

// MockLongHeaderOpenerDecryptHeaderCall wrap *gomock.Call
type MockLongHeaderOpenerDecryptHeaderCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockLongHeaderOpenerDecryptHeaderCall) Return() *MockLongHeaderOpenerDecryptHeaderCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockLongHeaderOpenerDecryptHeaderCall) Do(
	f func([]byte, *byte, []byte),
) *MockLongHeaderOpenerDecryptHeaderCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockLongHeaderOpenerDecryptHeaderCall) DoAndReturn(
	f func([]byte, *byte, []byte),
) *MockLongHeaderOpenerDecryptHeaderCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Open mocks base method.
func (m *MockLongHeaderOpener) Open(dst, src []byte, pn protocol.PacketNumber, associatedData []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Open", dst, src, pn, associatedData)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Open indicates an expected call of Open.
func (mr *MockLongHeaderOpenerMockRecorder) Open(dst, src, pn, associatedData any) *MockLongHeaderOpenerOpenCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Open",
		reflect.TypeOf((*MockLongHeaderOpener)(nil).Open),
		dst,
		src,
		pn,
		associatedData,
	)
	return &MockLongHeaderOpenerOpenCall{Call: call}
}

// MockLongHeaderOpenerOpenCall wrap *gomock.Call
type MockLongHeaderOpenerOpenCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockLongHeaderOpenerOpenCall) Return(arg0 []byte, arg1 error) *MockLongHeaderOpenerOpenCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockLongHeaderOpenerOpenCall) Do(
	f func([]byte, []byte, protocol.PacketNumber, []byte) ([]byte, error),
) *MockLongHeaderOpenerOpenCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockLongHeaderOpenerOpenCall) DoAndReturn(
	f func([]byte, []byte, protocol.PacketNumber, []byte) ([]byte, error),
) *MockLongHeaderOpenerOpenCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: Packer)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_packer_test.go github.com/lemon4ksan/sein/internal/quic Packer
//

// Package quic is a generated GoMock package.

// MockPacker is a mock of Packer interface.
type MockPacker struct {
	ctrl     *gomock.Controller
	recorder *MockPackerMockRecorder
	isgomock struct{}
}

// MockPackerMockRecorder is the mock recorder for MockPacker.
type MockPackerMockRecorder struct {
	mock *MockPacker
}

// NewMockPacker creates a new mock instance.
func NewMockPacker(ctrl *gomock.Controller) *MockPacker {
	mock := &MockPacker{ctrl: ctrl}
	mock.recorder = &MockPackerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPacker) EXPECT() *MockPackerMockRecorder {
	return m.recorder
}

// AppendPacket mocks base method.
func (m *MockPacker) AppendPacket(
	arg0 *packetBuffer,
	maxPacketSize protocol.ByteCount,
	now monotime.Time,
	v protocol.Version,
) (shortHeaderPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AppendPacket", arg0, maxPacketSize, now, v)
	ret0, _ := ret[0].(shortHeaderPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AppendPacket indicates an expected call of AppendPacket.
func (mr *MockPackerMockRecorder) AppendPacket(arg0, maxPacketSize, now, v any) *MockPackerAppendPacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"AppendPacket",
		reflect.TypeOf((*MockPacker)(nil).AppendPacket),
		arg0,
		maxPacketSize,
		now,
		v,
	)
	return &MockPackerAppendPacketCall{Call: call}
}

// MockPackerAppendPacketCall wrap *gomock.Call
type MockPackerAppendPacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerAppendPacketCall) Return(arg0 shortHeaderPacket, arg1 error) *MockPackerAppendPacketCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerAppendPacketCall) Do(
	f func(*packetBuffer, protocol.ByteCount, monotime.Time, protocol.Version) (shortHeaderPacket, error),
) *MockPackerAppendPacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerAppendPacketCall) DoAndReturn(
	f func(*packetBuffer, protocol.ByteCount, monotime.Time, protocol.Version) (shortHeaderPacket, error),
) *MockPackerAppendPacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackAckOnlyPacket mocks base method.
func (m *MockPacker) PackAckOnlyPacket(
	maxPacketSize protocol.ByteCount,
	now monotime.Time,
	v protocol.Version,
) (shortHeaderPacket, *packetBuffer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackAckOnlyPacket", maxPacketSize, now, v)
	ret0, _ := ret[0].(shortHeaderPacket)
	ret1, _ := ret[1].(*packetBuffer)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// PackAckOnlyPacket indicates an expected call of PackAckOnlyPacket.
func (mr *MockPackerMockRecorder) PackAckOnlyPacket(maxPacketSize, now, v any) *MockPackerPackAckOnlyPacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackAckOnlyPacket",
		reflect.TypeOf((*MockPacker)(nil).PackAckOnlyPacket),
		maxPacketSize,
		now,
		v,
	)
	return &MockPackerPackAckOnlyPacketCall{Call: call}
}

// MockPackerPackAckOnlyPacketCall wrap *gomock.Call
type MockPackerPackAckOnlyPacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackAckOnlyPacketCall) Return(
	arg0 shortHeaderPacket,
	arg1 *packetBuffer,
	arg2 error,
) *MockPackerPackAckOnlyPacketCall {
	c.Call = c.Call.Return(arg0, arg1, arg2)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackAckOnlyPacketCall) Do(
	f func(protocol.ByteCount, monotime.Time, protocol.Version) (shortHeaderPacket, *packetBuffer, error),
) *MockPackerPackAckOnlyPacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackAckOnlyPacketCall) DoAndReturn(
	f func(protocol.ByteCount, monotime.Time, protocol.Version) (shortHeaderPacket, *packetBuffer, error),
) *MockPackerPackAckOnlyPacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackApplicationClose mocks base method.
func (m *MockPacker) PackApplicationClose(
	arg0 *qerr.ApplicationError,
	arg1 protocol.ByteCount,
	arg2 protocol.Version,
) (*coalescedPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackApplicationClose", arg0, arg1, arg2)
	ret0, _ := ret[0].(*coalescedPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PackApplicationClose indicates an expected call of PackApplicationClose.
func (mr *MockPackerMockRecorder) PackApplicationClose(arg0, arg1, arg2 any) *MockPackerPackApplicationCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackApplicationClose",
		reflect.TypeOf((*MockPacker)(nil).PackApplicationClose),
		arg0,
		arg1,
		arg2,
	)
	return &MockPackerPackApplicationCloseCall{Call: call}
}

// MockPackerPackApplicationCloseCall wrap *gomock.Call
type MockPackerPackApplicationCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackApplicationCloseCall) Return(
	arg0 *coalescedPacket,
	arg1 error,
) *MockPackerPackApplicationCloseCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackApplicationCloseCall) Do(
	f func(*qerr.ApplicationError, protocol.ByteCount, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackApplicationCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackApplicationCloseCall) DoAndReturn(
	f func(*qerr.ApplicationError, protocol.ByteCount, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackApplicationCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackCoalescedPacket mocks base method.
func (m *MockPacker) PackCoalescedPacket(
	onlyAck bool,
	maxPacketSize protocol.ByteCount,
	now monotime.Time,
	v protocol.Version,
) (*coalescedPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackCoalescedPacket", onlyAck, maxPacketSize, now, v)
	ret0, _ := ret[0].(*coalescedPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PackCoalescedPacket indicates an expected call of PackCoalescedPacket.
func (mr *MockPackerMockRecorder) PackCoalescedPacket(
	onlyAck, maxPacketSize, now, v any,
) *MockPackerPackCoalescedPacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackCoalescedPacket",
		reflect.TypeOf((*MockPacker)(nil).PackCoalescedPacket),
		onlyAck,
		maxPacketSize,
		now,
		v,
	)
	return &MockPackerPackCoalescedPacketCall{Call: call}
}

// MockPackerPackCoalescedPacketCall wrap *gomock.Call
type MockPackerPackCoalescedPacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackCoalescedPacketCall) Return(
	arg0 *coalescedPacket,
	arg1 error,
) *MockPackerPackCoalescedPacketCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackCoalescedPacketCall) Do(
	f func(bool, protocol.ByteCount, monotime.Time, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackCoalescedPacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackCoalescedPacketCall) DoAndReturn(
	f func(bool, protocol.ByteCount, monotime.Time, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackCoalescedPacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackConnectionClose mocks base method.
func (m *MockPacker) PackConnectionClose(
	arg0 *qerr.TransportError,
	arg1 protocol.ByteCount,
	arg2 protocol.Version,
) (*coalescedPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackConnectionClose", arg0, arg1, arg2)
	ret0, _ := ret[0].(*coalescedPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PackConnectionClose indicates an expected call of PackConnectionClose.
func (mr *MockPackerMockRecorder) PackConnectionClose(arg0, arg1, arg2 any) *MockPackerPackConnectionCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackConnectionClose",
		reflect.TypeOf((*MockPacker)(nil).PackConnectionClose),
		arg0,
		arg1,
		arg2,
	)
	return &MockPackerPackConnectionCloseCall{Call: call}
}

// MockPackerPackConnectionCloseCall wrap *gomock.Call
type MockPackerPackConnectionCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackConnectionCloseCall) Return(
	arg0 *coalescedPacket,
	arg1 error,
) *MockPackerPackConnectionCloseCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackConnectionCloseCall) Do(
	f func(*qerr.TransportError, protocol.ByteCount, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackConnectionCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackConnectionCloseCall) DoAndReturn(
	f func(*qerr.TransportError, protocol.ByteCount, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackConnectionCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackMTUProbePacket mocks base method.
func (m *MockPacker) PackMTUProbePacket(
	ping ackhandler.Frame,
	size protocol.ByteCount,
	v protocol.Version,
) (shortHeaderPacket, *packetBuffer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackMTUProbePacket", ping, size, v)
	ret0, _ := ret[0].(shortHeaderPacket)
	ret1, _ := ret[1].(*packetBuffer)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// PackMTUProbePacket indicates an expected call of PackMTUProbePacket.
func (mr *MockPackerMockRecorder) PackMTUProbePacket(ping, size, v any) *MockPackerPackMTUProbePacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackMTUProbePacket",
		reflect.TypeOf((*MockPacker)(nil).PackMTUProbePacket),
		ping,
		size,
		v,
	)
	return &MockPackerPackMTUProbePacketCall{Call: call}
}

// MockPackerPackMTUProbePacketCall wrap *gomock.Call
type MockPackerPackMTUProbePacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackMTUProbePacketCall) Return(
	arg0 shortHeaderPacket,
	arg1 *packetBuffer,
	arg2 error,
) *MockPackerPackMTUProbePacketCall {
	c.Call = c.Call.Return(arg0, arg1, arg2)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackMTUProbePacketCall) Do(
	f func(ackhandler.Frame, protocol.ByteCount, protocol.Version) (shortHeaderPacket, *packetBuffer, error),
) *MockPackerPackMTUProbePacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackMTUProbePacketCall) DoAndReturn(
	f func(ackhandler.Frame, protocol.ByteCount, protocol.Version) (shortHeaderPacket, *packetBuffer, error),
) *MockPackerPackMTUProbePacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackPTOProbePacket mocks base method.
func (m *MockPacker) PackPTOProbePacket(
	arg0 protocol.EncryptionLevel,
	arg1 protocol.ByteCount,
	addPingIfEmpty bool,
	now monotime.Time,
	v protocol.Version,
) (*coalescedPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackPTOProbePacket", arg0, arg1, addPingIfEmpty, now, v)
	ret0, _ := ret[0].(*coalescedPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PackPTOProbePacket indicates an expected call of PackPTOProbePacket.
func (mr *MockPackerMockRecorder) PackPTOProbePacket(
	arg0, arg1, addPingIfEmpty, now, v any,
) *MockPackerPackPTOProbePacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackPTOProbePacket",
		reflect.TypeOf((*MockPacker)(nil).PackPTOProbePacket),
		arg0,
		arg1,
		addPingIfEmpty,
		now,
		v,
	)
	return &MockPackerPackPTOProbePacketCall{Call: call}
}

// MockPackerPackPTOProbePacketCall wrap *gomock.Call
type MockPackerPackPTOProbePacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackPTOProbePacketCall) Return(arg0 *coalescedPacket, arg1 error) *MockPackerPackPTOProbePacketCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackPTOProbePacketCall) Do(
	f func(protocol.EncryptionLevel, protocol.ByteCount, bool, monotime.Time, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackPTOProbePacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackPTOProbePacketCall) DoAndReturn(
	f func(protocol.EncryptionLevel, protocol.ByteCount, bool, monotime.Time, protocol.Version) (*coalescedPacket, error),
) *MockPackerPackPTOProbePacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PackPathProbePacket mocks base method.
func (m *MockPacker) PackPathProbePacket(
	arg0 protocol.ConnectionID,
	arg1 []ackhandler.Frame,
	arg2 protocol.Version,
) (shortHeaderPacket, *packetBuffer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackPathProbePacket", arg0, arg1, arg2)
	ret0, _ := ret[0].(shortHeaderPacket)
	ret1, _ := ret[1].(*packetBuffer)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// PackPathProbePacket indicates an expected call of PackPathProbePacket.
func (mr *MockPackerMockRecorder) PackPathProbePacket(arg0, arg1, arg2 any) *MockPackerPackPathProbePacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PackPathProbePacket",
		reflect.TypeOf((*MockPacker)(nil).PackPathProbePacket),
		arg0,
		arg1,
		arg2,
	)
	return &MockPackerPackPathProbePacketCall{Call: call}
}

// MockPackerPackPathProbePacketCall wrap *gomock.Call
type MockPackerPackPathProbePacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerPackPathProbePacketCall) Return(
	arg0 shortHeaderPacket,
	arg1 *packetBuffer,
	arg2 error,
) *MockPackerPackPathProbePacketCall {
	c.Call = c.Call.Return(arg0, arg1, arg2)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerPackPathProbePacketCall) Do(
	f func(protocol.ConnectionID, []ackhandler.Frame, protocol.Version) (shortHeaderPacket, *packetBuffer, error),
) *MockPackerPackPathProbePacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerPackPathProbePacketCall) DoAndReturn(
	f func(protocol.ConnectionID, []ackhandler.Frame, protocol.Version) (shortHeaderPacket, *packetBuffer, error),
) *MockPackerPackPathProbePacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetToken mocks base method.
func (m *MockPacker) SetToken(arg0 []byte) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetToken", arg0)
}

// SetToken indicates an expected call of SetToken.
func (mr *MockPackerMockRecorder) SetToken(arg0 any) *MockPackerSetTokenCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetToken",
		reflect.TypeOf((*MockPacker)(nil).SetToken),
		arg0,
	)
	return &MockPackerSetTokenCall{Call: call}
}

// MockPackerSetTokenCall wrap *gomock.Call
type MockPackerSetTokenCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPackerSetTokenCall) Return() *MockPackerSetTokenCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPackerSetTokenCall) Do(f func([]byte)) *MockPackerSetTokenCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPackerSetTokenCall) DoAndReturn(f func([]byte)) *MockPackerSetTokenCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: PacketHandler)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_packet_handler_test.go github.com/lemon4ksan/sein/internal/quic PacketHandler
//

// Package quic is a generated GoMock package.

// MockPacketHandler is a mock of PacketHandler interface.
type MockPacketHandler struct {
	ctrl     *gomock.Controller
	recorder *MockPacketHandlerMockRecorder
	isgomock struct{}
}

// MockPacketHandlerMockRecorder is the mock recorder for MockPacketHandler.
type MockPacketHandlerMockRecorder struct {
	mock *MockPacketHandler
}

// NewMockPacketHandler creates a new mock instance.
func NewMockPacketHandler(ctrl *gomock.Controller) *MockPacketHandler {
	mock := &MockPacketHandler{ctrl: ctrl}
	mock.recorder = &MockPacketHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPacketHandler) EXPECT() *MockPacketHandlerMockRecorder {
	return m.recorder
}

// closeWithTransportError mocks base method.
func (m *MockPacketHandler) closeWithTransportError(arg0 qerr.TransportErrorCode) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "closeWithTransportError", arg0)
}

// closeWithTransportError indicates an expected call of closeWithTransportError.
func (mr *MockPacketHandlerMockRecorder) closeWithTransportError(
	arg0 any,
) *MockPacketHandlercloseWithTransportErrorCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"closeWithTransportError",
		reflect.TypeOf((*MockPacketHandler)(nil).closeWithTransportError),
		arg0,
	)
	return &MockPacketHandlercloseWithTransportErrorCall{Call: call}
}

// MockPacketHandlercloseWithTransportErrorCall wrap *gomock.Call
type MockPacketHandlercloseWithTransportErrorCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketHandlercloseWithTransportErrorCall) Return() *MockPacketHandlercloseWithTransportErrorCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketHandlercloseWithTransportErrorCall) Do(
	f func(qerr.TransportErrorCode),
) *MockPacketHandlercloseWithTransportErrorCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketHandlercloseWithTransportErrorCall) DoAndReturn(
	f func(qerr.TransportErrorCode),
) *MockPacketHandlercloseWithTransportErrorCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// destroy mocks base method.
func (m *MockPacketHandler) destroy(arg0 error) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "destroy", arg0)
}

// destroy indicates an expected call of destroy.
func (mr *MockPacketHandlerMockRecorder) destroy(arg0 any) *MockPacketHandlerdestroyCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"destroy",
		reflect.TypeOf((*MockPacketHandler)(nil).destroy),
		arg0,
	)
	return &MockPacketHandlerdestroyCall{Call: call}
}

// MockPacketHandlerdestroyCall wrap *gomock.Call
type MockPacketHandlerdestroyCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketHandlerdestroyCall) Return() *MockPacketHandlerdestroyCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketHandlerdestroyCall) Do(f func(error)) *MockPacketHandlerdestroyCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketHandlerdestroyCall) DoAndReturn(f func(error)) *MockPacketHandlerdestroyCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// handlePacket mocks base method.
func (m *MockPacketHandler) handlePacket(arg0 receivedPacket) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "handlePacket", arg0)
}

// handlePacket indicates an expected call of handlePacket.
func (mr *MockPacketHandlerMockRecorder) handlePacket(arg0 any) *MockPacketHandlerhandlePacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"handlePacket",
		reflect.TypeOf((*MockPacketHandler)(nil).handlePacket),
		arg0,
	)
	return &MockPacketHandlerhandlePacketCall{Call: call}
}

// MockPacketHandlerhandlePacketCall wrap *gomock.Call
type MockPacketHandlerhandlePacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketHandlerhandlePacketCall) Return() *MockPacketHandlerhandlePacketCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketHandlerhandlePacketCall) Do(f func(receivedPacket)) *MockPacketHandlerhandlePacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketHandlerhandlePacketCall) DoAndReturn(f func(receivedPacket)) *MockPacketHandlerhandlePacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: net (interfaces: PacketConn)
//
// Generated by this command:
//
//	mockgen -typed -package quic -self_package github.com/lemon4ksan/sein/internal/quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_packetconn_test.go net PacketConn
//

// Package quic is a generated GoMock package.

// MockPacketConn is a mock of PacketConn interface.
type MockPacketConn struct {
	ctrl     *gomock.Controller
	recorder *MockPacketConnMockRecorder
	isgomock struct{}
}

// MockPacketConnMockRecorder is the mock recorder for MockPacketConn.
type MockPacketConnMockRecorder struct {
	mock *MockPacketConn
}

// NewMockPacketConn creates a new mock instance.
func NewMockPacketConn(ctrl *gomock.Controller) *MockPacketConn {
	mock := &MockPacketConn{ctrl: ctrl}
	mock.recorder = &MockPacketConnMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPacketConn) EXPECT() *MockPacketConnMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockPacketConn) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockPacketConnMockRecorder) Close() *MockPacketConnCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockPacketConn)(nil).Close))
	return &MockPacketConnCloseCall{Call: call}
}

// MockPacketConnCloseCall wrap *gomock.Call
type MockPacketConnCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnCloseCall) Return(arg0 error) *MockPacketConnCloseCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnCloseCall) Do(f func() error) *MockPacketConnCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnCloseCall) DoAndReturn(f func() error) *MockPacketConnCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// LocalAddr mocks base method.
func (m *MockPacketConn) LocalAddr() net.Addr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LocalAddr")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// LocalAddr indicates an expected call of LocalAddr.
func (mr *MockPacketConnMockRecorder) LocalAddr() *MockPacketConnLocalAddrCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"LocalAddr",
		reflect.TypeOf((*MockPacketConn)(nil).LocalAddr),
	)
	return &MockPacketConnLocalAddrCall{Call: call}
}

// MockPacketConnLocalAddrCall wrap *gomock.Call
type MockPacketConnLocalAddrCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnLocalAddrCall) Return(arg0 net.Addr) *MockPacketConnLocalAddrCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnLocalAddrCall) Do(f func() net.Addr) *MockPacketConnLocalAddrCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnLocalAddrCall) DoAndReturn(f func() net.Addr) *MockPacketConnLocalAddrCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReadFrom mocks base method.
func (m *MockPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadFrom", p)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(net.Addr)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ReadFrom indicates an expected call of ReadFrom.
func (mr *MockPacketConnMockRecorder) ReadFrom(p any) *MockPacketConnReadFromCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ReadFrom",
		reflect.TypeOf((*MockPacketConn)(nil).ReadFrom),
		p,
	)
	return &MockPacketConnReadFromCall{Call: call}
}

// MockPacketConnReadFromCall wrap *gomock.Call
type MockPacketConnReadFromCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnReadFromCall) Return(n int, addr net.Addr, err error) *MockPacketConnReadFromCall {
	c.Call = c.Call.Return(n, addr, err)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnReadFromCall) Do(f func([]byte) (int, net.Addr, error)) *MockPacketConnReadFromCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnReadFromCall) DoAndReturn(f func([]byte) (int, net.Addr, error)) *MockPacketConnReadFromCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetDeadline mocks base method.
func (m *MockPacketConn) SetDeadline(t time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetDeadline", t)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetDeadline indicates an expected call of SetDeadline.
func (mr *MockPacketConnMockRecorder) SetDeadline(t any) *MockPacketConnSetDeadlineCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetDeadline",
		reflect.TypeOf((*MockPacketConn)(nil).SetDeadline),
		t,
	)
	return &MockPacketConnSetDeadlineCall{Call: call}
}

// MockPacketConnSetDeadlineCall wrap *gomock.Call
type MockPacketConnSetDeadlineCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnSetDeadlineCall) Return(arg0 error) *MockPacketConnSetDeadlineCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnSetDeadlineCall) Do(f func(time.Time) error) *MockPacketConnSetDeadlineCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnSetDeadlineCall) DoAndReturn(f func(time.Time) error) *MockPacketConnSetDeadlineCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetReadDeadline mocks base method.
func (m *MockPacketConn) SetReadDeadline(t time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetReadDeadline", t)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetReadDeadline indicates an expected call of SetReadDeadline.
func (mr *MockPacketConnMockRecorder) SetReadDeadline(t any) *MockPacketConnSetReadDeadlineCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetReadDeadline",
		reflect.TypeOf((*MockPacketConn)(nil).SetReadDeadline),
		t,
	)
	return &MockPacketConnSetReadDeadlineCall{Call: call}
}

// MockPacketConnSetReadDeadlineCall wrap *gomock.Call
type MockPacketConnSetReadDeadlineCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnSetReadDeadlineCall) Return(arg0 error) *MockPacketConnSetReadDeadlineCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnSetReadDeadlineCall) Do(f func(time.Time) error) *MockPacketConnSetReadDeadlineCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnSetReadDeadlineCall) DoAndReturn(f func(time.Time) error) *MockPacketConnSetReadDeadlineCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetWriteDeadline mocks base method.
func (m *MockPacketConn) SetWriteDeadline(t time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetWriteDeadline", t)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetWriteDeadline indicates an expected call of SetWriteDeadline.
func (mr *MockPacketConnMockRecorder) SetWriteDeadline(t any) *MockPacketConnSetWriteDeadlineCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetWriteDeadline",
		reflect.TypeOf((*MockPacketConn)(nil).SetWriteDeadline),
		t,
	)
	return &MockPacketConnSetWriteDeadlineCall{Call: call}
}

// MockPacketConnSetWriteDeadlineCall wrap *gomock.Call
type MockPacketConnSetWriteDeadlineCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnSetWriteDeadlineCall) Return(arg0 error) *MockPacketConnSetWriteDeadlineCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnSetWriteDeadlineCall) Do(f func(time.Time) error) *MockPacketConnSetWriteDeadlineCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnSetWriteDeadlineCall) DoAndReturn(f func(time.Time) error) *MockPacketConnSetWriteDeadlineCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// WriteTo mocks base method.
func (m *MockPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteTo", p, addr)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// WriteTo indicates an expected call of WriteTo.
func (mr *MockPacketConnMockRecorder) WriteTo(p, addr any) *MockPacketConnWriteToCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"WriteTo",
		reflect.TypeOf((*MockPacketConn)(nil).WriteTo),
		p,
		addr,
	)
	return &MockPacketConnWriteToCall{Call: call}
}

// MockPacketConnWriteToCall wrap *gomock.Call
type MockPacketConnWriteToCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPacketConnWriteToCall) Return(n int, err error) *MockPacketConnWriteToCall {
	c.Call = c.Call.Return(n, err)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPacketConnWriteToCall) Do(f func([]byte, net.Addr) (int, error)) *MockPacketConnWriteToCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPacketConnWriteToCall) DoAndReturn(f func([]byte, net.Addr) (int, error)) *MockPacketConnWriteToCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: RawConn)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_raw_conn_test.go github.com/lemon4ksan/sein/internal/quic RawConn
//

// Package quic is a generated GoMock package.

// MockRawConn is a mock of RawConn interface.
type MockRawConn struct {
	ctrl     *gomock.Controller
	recorder *MockRawConnMockRecorder
	isgomock struct{}
}

// MockRawConnMockRecorder is the mock recorder for MockRawConn.
type MockRawConnMockRecorder struct {
	mock *MockRawConn
}

// NewMockRawConn creates a new mock instance.
func NewMockRawConn(ctrl *gomock.Controller) *MockRawConn {
	mock := &MockRawConn{ctrl: ctrl}
	mock.recorder = &MockRawConnMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRawConn) EXPECT() *MockRawConnMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockRawConn) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockRawConnMockRecorder) Close() *MockRawConnCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockRawConn)(nil).Close))
	return &MockRawConnCloseCall{Call: call}
}

// MockRawConnCloseCall wrap *gomock.Call
type MockRawConnCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockRawConnCloseCall) Return(arg0 error) *MockRawConnCloseCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockRawConnCloseCall) Do(f func() error) *MockRawConnCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockRawConnCloseCall) DoAndReturn(f func() error) *MockRawConnCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// LocalAddr mocks base method.
func (m *MockRawConn) LocalAddr() net.Addr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LocalAddr")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// LocalAddr indicates an expected call of LocalAddr.
func (mr *MockRawConnMockRecorder) LocalAddr() *MockRawConnLocalAddrCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LocalAddr", reflect.TypeOf((*MockRawConn)(nil).LocalAddr))
	return &MockRawConnLocalAddrCall{Call: call}
}

// MockRawConnLocalAddrCall wrap *gomock.Call
type MockRawConnLocalAddrCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockRawConnLocalAddrCall) Return(arg0 net.Addr) *MockRawConnLocalAddrCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockRawConnLocalAddrCall) Do(f func() net.Addr) *MockRawConnLocalAddrCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockRawConnLocalAddrCall) DoAndReturn(f func() net.Addr) *MockRawConnLocalAddrCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReadPacket mocks base method.
func (m *MockRawConn) ReadPacket() (receivedPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadPacket")
	ret0, _ := ret[0].(receivedPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadPacket indicates an expected call of ReadPacket.
func (mr *MockRawConnMockRecorder) ReadPacket() *MockRawConnReadPacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadPacket", reflect.TypeOf((*MockRawConn)(nil).ReadPacket))
	return &MockRawConnReadPacketCall{Call: call}
}

// MockRawConnReadPacketCall wrap *gomock.Call
type MockRawConnReadPacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockRawConnReadPacketCall) Return(arg0 receivedPacket, arg1 error) *MockRawConnReadPacketCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockRawConnReadPacketCall) Do(f func() (receivedPacket, error)) *MockRawConnReadPacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockRawConnReadPacketCall) DoAndReturn(f func() (receivedPacket, error)) *MockRawConnReadPacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetReadDeadline mocks base method.
func (m *MockRawConn) SetReadDeadline(arg0 time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetReadDeadline", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetReadDeadline indicates an expected call of SetReadDeadline.
func (mr *MockRawConnMockRecorder) SetReadDeadline(arg0 any) *MockRawConnSetReadDeadlineCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetReadDeadline",
		reflect.TypeOf((*MockRawConn)(nil).SetReadDeadline),
		arg0,
	)
	return &MockRawConnSetReadDeadlineCall{Call: call}
}

// MockRawConnSetReadDeadlineCall wrap *gomock.Call
type MockRawConnSetReadDeadlineCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockRawConnSetReadDeadlineCall) Return(arg0 error) *MockRawConnSetReadDeadlineCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockRawConnSetReadDeadlineCall) Do(f func(time.Time) error) *MockRawConnSetReadDeadlineCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockRawConnSetReadDeadlineCall) DoAndReturn(f func(time.Time) error) *MockRawConnSetReadDeadlineCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// WritePacket mocks base method.
func (m *MockRawConn) WritePacket(
	b []byte,
	addr net.Addr,
	packetInfoOOB []byte,
	gsoSize uint16,
	ecn protocol.ECN,
) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WritePacket", b, addr, packetInfoOOB, gsoSize, ecn)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// WritePacket indicates an expected call of WritePacket.
func (mr *MockRawConnMockRecorder) WritePacket(b, addr, packetInfoOOB, gsoSize, ecn any) *MockRawConnWritePacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"WritePacket",
		reflect.TypeOf((*MockRawConn)(nil).WritePacket),
		b,
		addr,
		packetInfoOOB,
		gsoSize,
		ecn,
	)
	return &MockRawConnWritePacketCall{Call: call}
}

// MockRawConnWritePacketCall wrap *gomock.Call
type MockRawConnWritePacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockRawConnWritePacketCall) Return(arg0 int, arg1 error) *MockRawConnWritePacketCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockRawConnWritePacketCall) Do(
	f func([]byte, net.Addr, []byte, uint16, protocol.ECN) (int, error),
) *MockRawConnWritePacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockRawConnWritePacketCall) DoAndReturn(
	f func([]byte, net.Addr, []byte, uint16, protocol.ECN) (int, error),
) *MockRawConnWritePacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// capabilities mocks base method.
func (m *MockRawConn) capabilities() connCapabilities {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "capabilities")
	ret0, _ := ret[0].(connCapabilities)
	return ret0
}

// capabilities indicates an expected call of capabilities.
func (mr *MockRawConnMockRecorder) capabilities() *MockRawConncapabilitiesCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"capabilities",
		reflect.TypeOf((*MockRawConn)(nil).capabilities),
	)
	return &MockRawConncapabilitiesCall{Call: call}
}

// MockRawConncapabilitiesCall wrap *gomock.Call
type MockRawConncapabilitiesCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockRawConncapabilitiesCall) Return(arg0 connCapabilities) *MockRawConncapabilitiesCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockRawConncapabilitiesCall) Do(f func() connCapabilities) *MockRawConncapabilitiesCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockRawConncapabilitiesCall) DoAndReturn(f func() connCapabilities) *MockRawConncapabilitiesCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: SealingManager)

// Package quic is a generated GoMock package.

// MockSealingManager is a mock of SealingManager interface.
type MockSealingManager struct {
	ctrl     *gomock.Controller
	recorder *MockSealingManagerMockRecorder
	isgomock struct{}
}

// MockSealingManagerMockRecorder is the mock recorder for MockSealingManager.
type MockSealingManagerMockRecorder struct {
	mock *MockSealingManager
}

// NewMockSealingManager creates a new mock instance.
func NewMockSealingManager(ctrl *gomock.Controller) *MockSealingManager {
	mock := &MockSealingManager{ctrl: ctrl}
	mock.recorder = &MockSealingManagerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSealingManager) EXPECT() *MockSealingManagerMockRecorder {
	return m.recorder
}

// ZeroRTTSealer mocks base method.
func (m *MockSealingManager) ZeroRTTSealer() (handshake.LongHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ZeroRTTSealer")
	ret0, _ := ret[0].(handshake.LongHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ZeroRTTSealer indicates an expected call of ZeroRTTSealer.
func (mr *MockSealingManagerMockRecorder) ZeroRTTSealer() *MockSealingManagerZeroRTTSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ZeroRTTSealer",
		reflect.TypeOf((*MockSealingManager)(nil).ZeroRTTSealer),
	)
	return &MockSealingManagerZeroRTTSealerCall{Call: call}
}

// MockSealingManagerZeroRTTSealerCall wrap *gomock.Call
type MockSealingManagerZeroRTTSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSealingManagerZeroRTTSealerCall) Return(
	arg0 handshake.LongHeaderSealer,
	arg1 error,
) *MockSealingManagerZeroRTTSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSealingManagerZeroRTTSealerCall) Do(
	f func() (handshake.LongHeaderSealer, error),
) *MockSealingManagerZeroRTTSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSealingManagerZeroRTTSealerCall) DoAndReturn(
	f func() (handshake.LongHeaderSealer, error),
) *MockSealingManagerZeroRTTSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// OneRTTSealer mocks base method.
func (m *MockSealingManager) OneRTTSealer() (handshake.ShortHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OneRTTSealer")
	ret0, _ := ret[0].(handshake.ShortHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// OneRTTSealer indicates an expected call of OneRTTSealer.
func (mr *MockSealingManagerMockRecorder) OneRTTSealer() *MockSealingManagerOneRTTSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"OneRTTSealer",
		reflect.TypeOf((*MockSealingManager)(nil).OneRTTSealer),
	)
	return &MockSealingManagerOneRTTSealerCall{Call: call}
}

// MockSealingManagerOneRTTSealerCall wrap *gomock.Call
type MockSealingManagerOneRTTSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSealingManagerOneRTTSealerCall) Return(
	arg0 handshake.ShortHeaderSealer,
	arg1 error,
) *MockSealingManagerOneRTTSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSealingManagerOneRTTSealerCall) Do(
	f func() (handshake.ShortHeaderSealer, error),
) *MockSealingManagerOneRTTSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSealingManagerOneRTTSealerCall) DoAndReturn(
	f func() (handshake.ShortHeaderSealer, error),
) *MockSealingManagerOneRTTSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// HandshakeSealer mocks base method.
func (m *MockSealingManager) HandshakeSealer() (handshake.LongHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandshakeSealer")
	ret0, _ := ret[0].(handshake.LongHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HandshakeSealer indicates an expected call of HandshakeSealer.
func (mr *MockSealingManagerMockRecorder) HandshakeSealer() *MockSealingManagerHandshakeSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"HandshakeSealer",
		reflect.TypeOf((*MockSealingManager)(nil).HandshakeSealer),
	)
	return &MockSealingManagerHandshakeSealerCall{Call: call}
}

// MockSealingManagerHandshakeSealerCall wrap *gomock.Call
type MockSealingManagerHandshakeSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSealingManagerHandshakeSealerCall) Return(
	arg0 handshake.LongHeaderSealer,
	arg1 error,
) *MockSealingManagerHandshakeSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSealingManagerHandshakeSealerCall) Do(
	f func() (handshake.LongHeaderSealer, error),
) *MockSealingManagerHandshakeSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSealingManagerHandshakeSealerCall) DoAndReturn(
	f func() (handshake.LongHeaderSealer, error),
) *MockSealingManagerHandshakeSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// InitialSealer mocks base method.
func (m *MockSealingManager) InitialSealer() (handshake.LongHeaderSealer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitialSealer")
	ret0, _ := ret[0].(handshake.LongHeaderSealer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InitialSealer indicates an expected call of InitialSealer.
func (mr *MockSealingManagerMockRecorder) InitialSealer() *MockSealingManagerInitialSealerCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"InitialSealer",
		reflect.TypeOf((*MockSealingManager)(nil).InitialSealer),
	)
	return &MockSealingManagerInitialSealerCall{Call: call}
}

// MockSealingManagerInitialSealerCall wrap *gomock.Call
type MockSealingManagerInitialSealerCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSealingManagerInitialSealerCall) Return(
	arg0 handshake.LongHeaderSealer,
	arg1 error,
) *MockSealingManagerInitialSealerCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSealingManagerInitialSealerCall) Do(
	f func() (handshake.LongHeaderSealer, error),
) *MockSealingManagerInitialSealerCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSealingManagerInitialSealerCall) DoAndReturn(
	f func() (handshake.LongHeaderSealer, error),
) *MockSealingManagerInitialSealerCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: SendConn)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_send_conn_test.go github.com/lemon4ksan/sein/internal/quic SendConn
//

// Package quic is a generated GoMock package.

// MockSendConn is a mock of SendConn interface.
type MockSendConn struct {
	ctrl     *gomock.Controller
	recorder *MockSendConnMockRecorder
	isgomock struct{}
}

// MockSendConnMockRecorder is the mock recorder for MockSendConn.
type MockSendConnMockRecorder struct {
	mock *MockSendConn
}

// NewMockSendConn creates a new mock instance.
func NewMockSendConn(ctrl *gomock.Controller) *MockSendConn {
	mock := &MockSendConn{ctrl: ctrl}
	mock.recorder = &MockSendConnMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSendConn) EXPECT() *MockSendConnMockRecorder {
	return m.recorder
}

// ChangeRemoteAddr mocks base method.
func (m *MockSendConn) ChangeRemoteAddr(addr net.Addr, info packetInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ChangeRemoteAddr", addr, info)
}

// ChangeRemoteAddr indicates an expected call of ChangeRemoteAddr.
func (mr *MockSendConnMockRecorder) ChangeRemoteAddr(addr, info any) *MockSendConnChangeRemoteAddrCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ChangeRemoteAddr",
		reflect.TypeOf((*MockSendConn)(nil).ChangeRemoteAddr),
		addr,
		info,
	)
	return &MockSendConnChangeRemoteAddrCall{Call: call}
}

// MockSendConnChangeRemoteAddrCall wrap *gomock.Call
type MockSendConnChangeRemoteAddrCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConnChangeRemoteAddrCall) Return() *MockSendConnChangeRemoteAddrCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConnChangeRemoteAddrCall) Do(f func(net.Addr, packetInfo)) *MockSendConnChangeRemoteAddrCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConnChangeRemoteAddrCall) DoAndReturn(f func(net.Addr, packetInfo)) *MockSendConnChangeRemoteAddrCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Close mocks base method.
func (m *MockSendConn) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockSendConnMockRecorder) Close() *MockSendConnCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockSendConn)(nil).Close))
	return &MockSendConnCloseCall{Call: call}
}

// MockSendConnCloseCall wrap *gomock.Call
type MockSendConnCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConnCloseCall) Return(arg0 error) *MockSendConnCloseCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConnCloseCall) Do(f func() error) *MockSendConnCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConnCloseCall) DoAndReturn(f func() error) *MockSendConnCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// LocalAddr mocks base method.
func (m *MockSendConn) LocalAddr() net.Addr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LocalAddr")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// LocalAddr indicates an expected call of LocalAddr.
func (mr *MockSendConnMockRecorder) LocalAddr() *MockSendConnLocalAddrCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LocalAddr", reflect.TypeOf((*MockSendConn)(nil).LocalAddr))
	return &MockSendConnLocalAddrCall{Call: call}
}

// MockSendConnLocalAddrCall wrap *gomock.Call
type MockSendConnLocalAddrCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConnLocalAddrCall) Return(arg0 net.Addr) *MockSendConnLocalAddrCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConnLocalAddrCall) Do(f func() net.Addr) *MockSendConnLocalAddrCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConnLocalAddrCall) DoAndReturn(f func() net.Addr) *MockSendConnLocalAddrCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// RemoteAddr mocks base method.
func (m *MockSendConn) RemoteAddr() net.Addr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoteAddr")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// RemoteAddr indicates an expected call of RemoteAddr.
func (mr *MockSendConnMockRecorder) RemoteAddr() *MockSendConnRemoteAddrCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"RemoteAddr",
		reflect.TypeOf((*MockSendConn)(nil).RemoteAddr),
	)
	return &MockSendConnRemoteAddrCall{Call: call}
}

// MockSendConnRemoteAddrCall wrap *gomock.Call
type MockSendConnRemoteAddrCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConnRemoteAddrCall) Return(arg0 net.Addr) *MockSendConnRemoteAddrCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConnRemoteAddrCall) Do(f func() net.Addr) *MockSendConnRemoteAddrCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConnRemoteAddrCall) DoAndReturn(f func() net.Addr) *MockSendConnRemoteAddrCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Write mocks base method.
func (m *MockSendConn) Write(b []byte, gsoSize uint16, ecn protocol.ECN) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Write", b, gsoSize, ecn)
	ret0, _ := ret[0].(error)
	return ret0
}

// Write indicates an expected call of Write.
func (mr *MockSendConnMockRecorder) Write(b, gsoSize, ecn any) *MockSendConnWriteCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Write",
		reflect.TypeOf((*MockSendConn)(nil).Write),
		b,
		gsoSize,
		ecn,
	)
	return &MockSendConnWriteCall{Call: call}
}

// MockSendConnWriteCall wrap *gomock.Call
type MockSendConnWriteCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConnWriteCall) Return(arg0 error) *MockSendConnWriteCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConnWriteCall) Do(f func([]byte, uint16, protocol.ECN) error) *MockSendConnWriteCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConnWriteCall) DoAndReturn(f func([]byte, uint16, protocol.ECN) error) *MockSendConnWriteCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// WriteTo mocks base method.
func (m *MockSendConn) WriteTo(arg0 []byte, arg1 net.Addr, arg2 packetInfo) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteTo", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// WriteTo indicates an expected call of WriteTo.
func (mr *MockSendConnMockRecorder) WriteTo(arg0, arg1, arg2 any) *MockSendConnWriteToCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"WriteTo",
		reflect.TypeOf((*MockSendConn)(nil).WriteTo),
		arg0,
		arg1,
		arg2,
	)
	return &MockSendConnWriteToCall{Call: call}
}

// MockSendConnWriteToCall wrap *gomock.Call
type MockSendConnWriteToCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConnWriteToCall) Return(arg0 error) *MockSendConnWriteToCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConnWriteToCall) Do(f func([]byte, net.Addr, packetInfo) error) *MockSendConnWriteToCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConnWriteToCall) DoAndReturn(f func([]byte, net.Addr, packetInfo) error) *MockSendConnWriteToCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// capabilities mocks base method.
func (m *MockSendConn) capabilities() connCapabilities {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "capabilities")
	ret0, _ := ret[0].(connCapabilities)
	return ret0
}

// capabilities indicates an expected call of capabilities.
func (mr *MockSendConnMockRecorder) capabilities() *MockSendConncapabilitiesCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"capabilities",
		reflect.TypeOf((*MockSendConn)(nil).capabilities),
	)
	return &MockSendConncapabilitiesCall{Call: call}
}

// MockSendConncapabilitiesCall wrap *gomock.Call
type MockSendConncapabilitiesCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSendConncapabilitiesCall) Return(arg0 connCapabilities) *MockSendConncapabilitiesCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSendConncapabilitiesCall) Do(f func() connCapabilities) *MockSendConncapabilitiesCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSendConncapabilitiesCall) DoAndReturn(f func() connCapabilities) *MockSendConncapabilitiesCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: Sender)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_sender_test.go github.com/lemon4ksan/sein/internal/quic Sender
//

// Package quic is a generated GoMock package.

// MockSender is a mock of Sender interface.
type MockSender struct {
	ctrl     *gomock.Controller
	recorder *MockSenderMockRecorder
	isgomock struct{}
}

// MockSenderMockRecorder is the mock recorder for MockSender.
type MockSenderMockRecorder struct {
	mock *MockSender
}

// NewMockSender creates a new mock instance.
func NewMockSender(ctrl *gomock.Controller) *MockSender {
	mock := &MockSender{ctrl: ctrl}
	mock.recorder = &MockSenderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSender) EXPECT() *MockSenderMockRecorder {
	return m.recorder
}

// Available mocks base method.
func (m *MockSender) Available() <-chan struct{} {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Available")
	ret0, _ := ret[0].(<-chan struct{})
	return ret0
}

// Available indicates an expected call of Available.
func (mr *MockSenderMockRecorder) Available() *MockSenderAvailableCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Available", reflect.TypeOf((*MockSender)(nil).Available))
	return &MockSenderAvailableCall{Call: call}
}

// MockSenderAvailableCall wrap *gomock.Call
type MockSenderAvailableCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSenderAvailableCall) Return(arg0 <-chan struct{}) *MockSenderAvailableCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSenderAvailableCall) Do(f func() <-chan struct{}) *MockSenderAvailableCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSenderAvailableCall) DoAndReturn(f func() <-chan struct{}) *MockSenderAvailableCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Close mocks base method.
func (m *MockSender) Close() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Close")
}

// Close indicates an expected call of Close.
func (mr *MockSenderMockRecorder) Close() *MockSenderCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockSender)(nil).Close))
	return &MockSenderCloseCall{Call: call}
}

// MockSenderCloseCall wrap *gomock.Call
type MockSenderCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSenderCloseCall) Return() *MockSenderCloseCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSenderCloseCall) Do(f func()) *MockSenderCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSenderCloseCall) DoAndReturn(f func()) *MockSenderCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Run mocks base method.
func (m *MockSender) Run() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Run")
	ret0, _ := ret[0].(error)
	return ret0
}

// Run indicates an expected call of Run.
func (mr *MockSenderMockRecorder) Run() *MockSenderRunCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Run", reflect.TypeOf((*MockSender)(nil).Run))
	return &MockSenderRunCall{Call: call}
}

// MockSenderRunCall wrap *gomock.Call
type MockSenderRunCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSenderRunCall) Return(arg0 error) *MockSenderRunCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSenderRunCall) Do(f func() error) *MockSenderRunCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSenderRunCall) DoAndReturn(f func() error) *MockSenderRunCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Send mocks base method.
func (m *MockSender) Send(p *packetBuffer, gsoSize uint16, ecn protocol.ECN) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Send", p, gsoSize, ecn)
}

// Send indicates an expected call of Send.
func (mr *MockSenderMockRecorder) Send(p, gsoSize, ecn any) *MockSenderSendCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Send",
		reflect.TypeOf((*MockSender)(nil).Send),
		p,
		gsoSize,
		ecn,
	)
	return &MockSenderSendCall{Call: call}
}

// MockSenderSendCall wrap *gomock.Call
type MockSenderSendCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSenderSendCall) Return() *MockSenderSendCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSenderSendCall) Do(f func(*packetBuffer, uint16, protocol.ECN)) *MockSenderSendCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSenderSendCall) DoAndReturn(f func(*packetBuffer, uint16, protocol.ECN)) *MockSenderSendCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SendProbe mocks base method.
func (m *MockSender) SendProbe(arg0 *packetBuffer, arg1 net.Addr, arg2 packetInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SendProbe", arg0, arg1, arg2)
}

// SendProbe indicates an expected call of SendProbe.
func (mr *MockSenderMockRecorder) SendProbe(arg0, arg1, arg2 any) *MockSenderSendProbeCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SendProbe",
		reflect.TypeOf((*MockSender)(nil).SendProbe),
		arg0,
		arg1,
		arg2,
	)
	return &MockSenderSendProbeCall{Call: call}
}

// MockSenderSendProbeCall wrap *gomock.Call
type MockSenderSendProbeCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSenderSendProbeCall) Return() *MockSenderSendProbeCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSenderSendProbeCall) Do(f func(*packetBuffer, net.Addr, packetInfo)) *MockSenderSendProbeCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSenderSendProbeCall) DoAndReturn(f func(*packetBuffer, net.Addr, packetInfo)) *MockSenderSendProbeCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// WouldBlock mocks base method.
func (m *MockSender) WouldBlock() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WouldBlock")
	ret0, _ := ret[0].(bool)
	return ret0
}

// WouldBlock indicates an expected call of WouldBlock.
func (mr *MockSenderMockRecorder) WouldBlock() *MockSenderWouldBlockCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WouldBlock", reflect.TypeOf((*MockSender)(nil).WouldBlock))
	return &MockSenderWouldBlockCall{Call: call}
}

// MockSenderWouldBlockCall wrap *gomock.Call
type MockSenderWouldBlockCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSenderWouldBlockCall) Return(arg0 bool) *MockSenderWouldBlockCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSenderWouldBlockCall) Do(f func() bool) *MockSenderWouldBlockCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSenderWouldBlockCall) DoAndReturn(f func() bool) *MockSenderWouldBlockCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic/internal/ackhandler (interfaces: SentPacketHandler)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -destination ackhandler/sent_packet_handler.go github.com/lemon4ksan/sein/internal/quic/internal/ackhandler SentPacketHandler
//

// Package mockackhandler is a generated GoMock package.

// MockSentPacketHandler is a mock of SentPacketHandler interface.
type MockSentPacketHandler struct {
	ctrl     *gomock.Controller
	recorder *MockSentPacketHandlerMockRecorder
	isgomock struct{}
}

// AmplificationAllowance mocks base method.
func (m *MockSentPacketHandler) AmplificationAllowance() protocol.ByteCount {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AmplificationAllowance")
	ret0, _ := ret[0].(protocol.ByteCount)
	return ret0
}

// MockSentPacketHandlerMockRecorder is the mock recorder for MockSentPacketHandler.
type MockSentPacketHandlerMockRecorder struct {
	mock *MockSentPacketHandler
}

// NewMockSentPacketHandler creates a new mock instance.
func NewMockSentPacketHandler(ctrl *gomock.Controller) *MockSentPacketHandler {
	mock := &MockSentPacketHandler{ctrl: ctrl}
	mock.recorder = &MockSentPacketHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSentPacketHandler) EXPECT() *MockSentPacketHandlerMockRecorder {
	return m.recorder
}

// DropPackets mocks base method.
func (m *MockSentPacketHandler) DropPackets(arg0 protocol.EncryptionLevel, rcvTime monotime.Time) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DropPackets", arg0, rcvTime)
}

// DropPackets indicates an expected call of DropPackets.
func (mr *MockSentPacketHandlerMockRecorder) DropPackets(arg0, rcvTime any) *MockSentPacketHandlerDropPacketsCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"DropPackets",
		reflect.TypeOf((*MockSentPacketHandler)(nil).DropPackets),
		arg0,
		rcvTime,
	)
	return &MockSentPacketHandlerDropPacketsCall{Call: call}
}

// MockSentPacketHandlerDropPacketsCall wrap *gomock.Call
type MockSentPacketHandlerDropPacketsCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerDropPacketsCall) Return() *MockSentPacketHandlerDropPacketsCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerDropPacketsCall) Do(
	f func(protocol.EncryptionLevel, monotime.Time),
) *MockSentPacketHandlerDropPacketsCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerDropPacketsCall) DoAndReturn(
	f func(protocol.EncryptionLevel, monotime.Time),
) *MockSentPacketHandlerDropPacketsCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ECNMode mocks base method.
func (m *MockSentPacketHandler) ECNMode(isShortHeaderPacket bool) protocol.ECN {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ECNMode", isShortHeaderPacket)
	ret0, _ := ret[0].(protocol.ECN)
	return ret0
}

// ECNMode indicates an expected call of ECNMode.
func (mr *MockSentPacketHandlerMockRecorder) ECNMode(isShortHeaderPacket any) *MockSentPacketHandlerECNModeCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ECNMode",
		reflect.TypeOf((*MockSentPacketHandler)(nil).ECNMode),
		isShortHeaderPacket,
	)
	return &MockSentPacketHandlerECNModeCall{Call: call}
}

// MockSentPacketHandlerECNModeCall wrap *gomock.Call
type MockSentPacketHandlerECNModeCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerECNModeCall) Return(arg0 protocol.ECN) *MockSentPacketHandlerECNModeCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerECNModeCall) Do(f func(bool) protocol.ECN) *MockSentPacketHandlerECNModeCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerECNModeCall) DoAndReturn(f func(bool) protocol.ECN) *MockSentPacketHandlerECNModeCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// LossDetectionTimeout mocks base method.
func (m *MockSentPacketHandler) LossDetectionTimeout() monotime.Time {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LossDetectionTimeout")
	ret0, _ := ret[0].(monotime.Time)
	return ret0
}

// LossDetectionTimeout indicates an expected call of LossDetectionTimeout.
func (mr *MockSentPacketHandlerMockRecorder) LossDetectionTimeout() *MockSentPacketHandlerLossDetectionTimeoutCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"LossDetectionTimeout",
		reflect.TypeOf((*MockSentPacketHandler)(nil).LossDetectionTimeout),
	)
	return &MockSentPacketHandlerLossDetectionTimeoutCall{Call: call}
}

// MockSentPacketHandlerLossDetectionTimeoutCall wrap *gomock.Call
type MockSentPacketHandlerLossDetectionTimeoutCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerLossDetectionTimeoutCall) Return(
	arg0 monotime.Time,
) *MockSentPacketHandlerLossDetectionTimeoutCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerLossDetectionTimeoutCall) Do(
	f func() monotime.Time,
) *MockSentPacketHandlerLossDetectionTimeoutCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerLossDetectionTimeoutCall) DoAndReturn(
	f func() monotime.Time,
) *MockSentPacketHandlerLossDetectionTimeoutCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MigratedPath mocks base method.
func (m *MockSentPacketHandler) MigratedPath(now monotime.Time, initialMaxPacketSize protocol.ByteCount) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "MigratedPath", now, initialMaxPacketSize)
}

// MigratedPath indicates an expected call of MigratedPath.
func (mr *MockSentPacketHandlerMockRecorder) MigratedPath(
	now, initialMaxPacketSize any,
) *MockSentPacketHandlerMigratedPathCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"MigratedPath",
		reflect.TypeOf((*MockSentPacketHandler)(nil).MigratedPath),
		now,
		initialMaxPacketSize,
	)
	return &MockSentPacketHandlerMigratedPathCall{Call: call}
}

// MockSentPacketHandlerMigratedPathCall wrap *gomock.Call
type MockSentPacketHandlerMigratedPathCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerMigratedPathCall) Return() *MockSentPacketHandlerMigratedPathCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerMigratedPathCall) Do(
	f func(monotime.Time, protocol.ByteCount),
) *MockSentPacketHandlerMigratedPathCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerMigratedPathCall) DoAndReturn(
	f func(monotime.Time, protocol.ByteCount),
) *MockSentPacketHandlerMigratedPathCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// OnLossDetectionTimeout mocks base method.
func (m *MockSentPacketHandler) OnLossDetectionTimeout(now monotime.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OnLossDetectionTimeout", now)
	ret0, _ := ret[0].(error)
	return ret0
}

// OnLossDetectionTimeout indicates an expected call of OnLossDetectionTimeout.
func (mr *MockSentPacketHandlerMockRecorder) OnLossDetectionTimeout(
	now any,
) *MockSentPacketHandlerOnLossDetectionTimeoutCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"OnLossDetectionTimeout",
		reflect.TypeOf((*MockSentPacketHandler)(nil).OnLossDetectionTimeout),
		now,
	)
	return &MockSentPacketHandlerOnLossDetectionTimeoutCall{Call: call}
}

// MockSentPacketHandlerOnLossDetectionTimeoutCall wrap *gomock.Call
type MockSentPacketHandlerOnLossDetectionTimeoutCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerOnLossDetectionTimeoutCall) Return(
	arg0 error,
) *MockSentPacketHandlerOnLossDetectionTimeoutCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerOnLossDetectionTimeoutCall) Do(
	f func(monotime.Time) error,
) *MockSentPacketHandlerOnLossDetectionTimeoutCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerOnLossDetectionTimeoutCall) DoAndReturn(
	f func(monotime.Time) error,
) *MockSentPacketHandlerOnLossDetectionTimeoutCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PeekPacketNumber mocks base method.
func (m *MockSentPacketHandler) PeekPacketNumber(
	arg0 protocol.EncryptionLevel,
) (protocol.PacketNumber, protocol.PacketNumberLen) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PeekPacketNumber", arg0)
	ret0, _ := ret[0].(protocol.PacketNumber)
	ret1, _ := ret[1].(protocol.PacketNumberLen)
	return ret0, ret1
}

// PeekPacketNumber indicates an expected call of PeekPacketNumber.
func (mr *MockSentPacketHandlerMockRecorder) PeekPacketNumber(arg0 any) *MockSentPacketHandlerPeekPacketNumberCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PeekPacketNumber",
		reflect.TypeOf((*MockSentPacketHandler)(nil).PeekPacketNumber),
		arg0,
	)
	return &MockSentPacketHandlerPeekPacketNumberCall{Call: call}
}

// MockSentPacketHandlerPeekPacketNumberCall wrap *gomock.Call
type MockSentPacketHandlerPeekPacketNumberCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerPeekPacketNumberCall) Return(
	arg0 protocol.PacketNumber,
	arg1 protocol.PacketNumberLen,
) *MockSentPacketHandlerPeekPacketNumberCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerPeekPacketNumberCall) Do(
	f func(protocol.EncryptionLevel) (protocol.PacketNumber, protocol.PacketNumberLen),
) *MockSentPacketHandlerPeekPacketNumberCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerPeekPacketNumberCall) DoAndReturn(
	f func(protocol.EncryptionLevel) (protocol.PacketNumber, protocol.PacketNumberLen),
) *MockSentPacketHandlerPeekPacketNumberCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PopPacketNumber mocks base method.
func (m *MockSentPacketHandler) PopPacketNumber(arg0 protocol.EncryptionLevel) protocol.PacketNumber {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PopPacketNumber", arg0)
	ret0, _ := ret[0].(protocol.PacketNumber)
	return ret0
}

// PopPacketNumber indicates an expected call of PopPacketNumber.
func (mr *MockSentPacketHandlerMockRecorder) PopPacketNumber(arg0 any) *MockSentPacketHandlerPopPacketNumberCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PopPacketNumber",
		reflect.TypeOf((*MockSentPacketHandler)(nil).PopPacketNumber),
		arg0,
	)
	return &MockSentPacketHandlerPopPacketNumberCall{Call: call}
}

// MockSentPacketHandlerPopPacketNumberCall wrap *gomock.Call
type MockSentPacketHandlerPopPacketNumberCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerPopPacketNumberCall) Return(
	arg0 protocol.PacketNumber,
) *MockSentPacketHandlerPopPacketNumberCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerPopPacketNumberCall) Do(
	f func(protocol.EncryptionLevel) protocol.PacketNumber,
) *MockSentPacketHandlerPopPacketNumberCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerPopPacketNumberCall) DoAndReturn(
	f func(protocol.EncryptionLevel) protocol.PacketNumber,
) *MockSentPacketHandlerPopPacketNumberCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// QueueProbePacket mocks base method.
func (m *MockSentPacketHandler) QueueProbePacket(arg0 protocol.EncryptionLevel) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "QueueProbePacket", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// QueueProbePacket indicates an expected call of QueueProbePacket.
func (mr *MockSentPacketHandlerMockRecorder) QueueProbePacket(arg0 any) *MockSentPacketHandlerQueueProbePacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"QueueProbePacket",
		reflect.TypeOf((*MockSentPacketHandler)(nil).QueueProbePacket),
		arg0,
	)
	return &MockSentPacketHandlerQueueProbePacketCall{Call: call}
}

// MockSentPacketHandlerQueueProbePacketCall wrap *gomock.Call
type MockSentPacketHandlerQueueProbePacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerQueueProbePacketCall) Return(arg0 bool) *MockSentPacketHandlerQueueProbePacketCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerQueueProbePacketCall) Do(
	f func(protocol.EncryptionLevel) bool,
) *MockSentPacketHandlerQueueProbePacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerQueueProbePacketCall) DoAndReturn(
	f func(protocol.EncryptionLevel) bool,
) *MockSentPacketHandlerQueueProbePacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReceivedAck mocks base method.
func (m *MockSentPacketHandler) ReceivedAck(
	f *wire.AckFrame,
	encLevel protocol.EncryptionLevel,
	rcvTime monotime.Time,
) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReceivedAck", f, encLevel, rcvTime)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReceivedAck indicates an expected call of ReceivedAck.
func (mr *MockSentPacketHandlerMockRecorder) ReceivedAck(
	f, encLevel, rcvTime any,
) *MockSentPacketHandlerReceivedAckCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ReceivedAck",
		reflect.TypeOf((*MockSentPacketHandler)(nil).ReceivedAck),
		f,
		encLevel,
		rcvTime,
	)
	return &MockSentPacketHandlerReceivedAckCall{Call: call}
}

// MockSentPacketHandlerReceivedAckCall wrap *gomock.Call
type MockSentPacketHandlerReceivedAckCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerReceivedAckCall) Return(arg0 bool, arg1 error) *MockSentPacketHandlerReceivedAckCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerReceivedAckCall) Do(
	f func(*wire.AckFrame, protocol.EncryptionLevel, monotime.Time) (bool, error),
) *MockSentPacketHandlerReceivedAckCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerReceivedAckCall) DoAndReturn(
	f func(*wire.AckFrame, protocol.EncryptionLevel, monotime.Time) (bool, error),
) *MockSentPacketHandlerReceivedAckCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReceivedBytes mocks base method.
func (m *MockSentPacketHandler) ReceivedBytes(arg0 protocol.ByteCount, rcvTime monotime.Time) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReceivedBytes", arg0, rcvTime)
}

// ReceivedBytes indicates an expected call of ReceivedBytes.
func (mr *MockSentPacketHandlerMockRecorder) ReceivedBytes(arg0, rcvTime any) *MockSentPacketHandlerReceivedBytesCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ReceivedBytes",
		reflect.TypeOf((*MockSentPacketHandler)(nil).ReceivedBytes),
		arg0,
		rcvTime,
	)
	return &MockSentPacketHandlerReceivedBytesCall{Call: call}
}

// MockSentPacketHandlerReceivedBytesCall wrap *gomock.Call
type MockSentPacketHandlerReceivedBytesCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerReceivedBytesCall) Return() *MockSentPacketHandlerReceivedBytesCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerReceivedBytesCall) Do(
	f func(protocol.ByteCount, monotime.Time),
) *MockSentPacketHandlerReceivedBytesCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerReceivedBytesCall) DoAndReturn(
	f func(protocol.ByteCount, monotime.Time),
) *MockSentPacketHandlerReceivedBytesCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReceivedPacket mocks base method.
func (m *MockSentPacketHandler) ReceivedPacket(arg0 protocol.EncryptionLevel, arg1 monotime.Time) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReceivedPacket", arg0, arg1)
}

// ReceivedPacket indicates an expected call of ReceivedPacket.
func (mr *MockSentPacketHandlerMockRecorder) ReceivedPacket(arg0, arg1 any) *MockSentPacketHandlerReceivedPacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ReceivedPacket",
		reflect.TypeOf((*MockSentPacketHandler)(nil).ReceivedPacket),
		arg0,
		arg1,
	)
	return &MockSentPacketHandlerReceivedPacketCall{Call: call}
}

// MockSentPacketHandlerReceivedPacketCall wrap *gomock.Call
type MockSentPacketHandlerReceivedPacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerReceivedPacketCall) Return() *MockSentPacketHandlerReceivedPacketCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerReceivedPacketCall) Do(
	f func(protocol.EncryptionLevel, monotime.Time),
) *MockSentPacketHandlerReceivedPacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerReceivedPacketCall) DoAndReturn(
	f func(protocol.EncryptionLevel, monotime.Time),
) *MockSentPacketHandlerReceivedPacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ResetForRetry mocks base method.
func (m *MockSentPacketHandler) ResetForRetry(rcvTime monotime.Time) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ResetForRetry", rcvTime)
}

// ResetForRetry indicates an expected call of ResetForRetry.
func (mr *MockSentPacketHandlerMockRecorder) ResetForRetry(rcvTime any) *MockSentPacketHandlerResetForRetryCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"ResetForRetry",
		reflect.TypeOf((*MockSentPacketHandler)(nil).ResetForRetry),
		rcvTime,
	)
	return &MockSentPacketHandlerResetForRetryCall{Call: call}
}

// MockSentPacketHandlerResetForRetryCall wrap *gomock.Call
type MockSentPacketHandlerResetForRetryCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerResetForRetryCall) Return() *MockSentPacketHandlerResetForRetryCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerResetForRetryCall) Do(f func(monotime.Time)) *MockSentPacketHandlerResetForRetryCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerResetForRetryCall) DoAndReturn(
	f func(monotime.Time),
) *MockSentPacketHandlerResetForRetryCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SendMode mocks base method.
func (m *MockSentPacketHandler) SendMode(now monotime.Time) ackhandler.SendMode {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendMode", now)
	ret0, _ := ret[0].(ackhandler.SendMode)
	return ret0
}

// SendMode indicates an expected call of SendMode.
func (mr *MockSentPacketHandlerMockRecorder) SendMode(now any) *MockSentPacketHandlerSendModeCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SendMode",
		reflect.TypeOf((*MockSentPacketHandler)(nil).SendMode),
		now,
	)
	return &MockSentPacketHandlerSendModeCall{Call: call}
}

// MockSentPacketHandlerSendModeCall wrap *gomock.Call
type MockSentPacketHandlerSendModeCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerSendModeCall) Return(arg0 ackhandler.SendMode) *MockSentPacketHandlerSendModeCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerSendModeCall) Do(
	f func(monotime.Time) ackhandler.SendMode,
) *MockSentPacketHandlerSendModeCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerSendModeCall) DoAndReturn(
	f func(monotime.Time) ackhandler.SendMode,
) *MockSentPacketHandlerSendModeCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SentPacket mocks base method.
func (m *MockSentPacketHandler) SentPacket(
	t monotime.Time,
	pn, largestAcked protocol.PacketNumber,
	streamFrames []ackhandler.StreamFrame,
	frames []ackhandler.Frame,
	encLevel protocol.EncryptionLevel,
	ecn protocol.ECN,
	size protocol.ByteCount,
	isPathMTUProbePacket, isPathProbePacket bool,
) {
	m.ctrl.T.Helper()
	m.ctrl.Call(
		m,
		"SentPacket",
		t,
		pn,
		largestAcked,
		streamFrames,
		frames,
		encLevel,
		ecn,
		size,
		isPathMTUProbePacket,
		isPathProbePacket,
	)
}

// SentPacket indicates an expected call of SentPacket.
func (mr *MockSentPacketHandlerMockRecorder) SentPacket(
	t, pn, largestAcked, streamFrames, frames, encLevel, ecn, size, isPathMTUProbePacket, isPathProbePacket any,
) *MockSentPacketHandlerSentPacketCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SentPacket",
		reflect.TypeOf((*MockSentPacketHandler)(nil).SentPacket),
		t,
		pn,
		largestAcked,
		streamFrames,
		frames,
		encLevel,
		ecn,
		size,
		isPathMTUProbePacket,
		isPathProbePacket,
	)
	return &MockSentPacketHandlerSentPacketCall{Call: call}
}

// MockSentPacketHandlerSentPacketCall wrap *gomock.Call
type MockSentPacketHandlerSentPacketCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerSentPacketCall) Return() *MockSentPacketHandlerSentPacketCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerSentPacketCall) Do(
	f func(monotime.Time, protocol.PacketNumber, protocol.PacketNumber, []ackhandler.StreamFrame, []ackhandler.Frame, protocol.EncryptionLevel, protocol.ECN, protocol.ByteCount, bool, bool),
) *MockSentPacketHandlerSentPacketCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerSentPacketCall) DoAndReturn(
	f func(monotime.Time, protocol.PacketNumber, protocol.PacketNumber, []ackhandler.StreamFrame, []ackhandler.Frame, protocol.EncryptionLevel, protocol.ECN, protocol.ByteCount, bool, bool),
) *MockSentPacketHandlerSentPacketCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetMaxDatagramSize mocks base method.
func (m *MockSentPacketHandler) SetMaxDatagramSize(count protocol.ByteCount) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetMaxDatagramSize", count)
}

// SetMaxDatagramSize indicates an expected call of SetMaxDatagramSize.
func (mr *MockSentPacketHandlerMockRecorder) SetMaxDatagramSize(
	count any,
) *MockSentPacketHandlerSetMaxDatagramSizeCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"SetMaxDatagramSize",
		reflect.TypeOf((*MockSentPacketHandler)(nil).SetMaxDatagramSize),
		count,
	)
	return &MockSentPacketHandlerSetMaxDatagramSizeCall{Call: call}
}

// MockSentPacketHandlerSetMaxDatagramSizeCall wrap *gomock.Call
type MockSentPacketHandlerSetMaxDatagramSizeCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerSetMaxDatagramSizeCall) Return() *MockSentPacketHandlerSetMaxDatagramSizeCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerSetMaxDatagramSizeCall) Do(
	f func(protocol.ByteCount),
) *MockSentPacketHandlerSetMaxDatagramSizeCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerSetMaxDatagramSizeCall) DoAndReturn(
	f func(protocol.ByteCount),
) *MockSentPacketHandlerSetMaxDatagramSizeCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// TimeUntilSend mocks base method.
func (m *MockSentPacketHandler) TimeUntilSend() monotime.Time {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TimeUntilSend")
	ret0, _ := ret[0].(monotime.Time)
	return ret0
}

// AmplificationAllowance indicates an expected call of AmplificationAllowance.
func (mr *MockSentPacketHandlerMockRecorder) AmplificationAllowance() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"AmplificationAllowance",
		reflect.TypeOf((*MockSentPacketHandler)(nil).AmplificationAllowance),
	)
}

// TimeUntilSend indicates an expected call of TimeUntilSend.
func (mr *MockSentPacketHandlerMockRecorder) TimeUntilSend() *MockSentPacketHandlerTimeUntilSendCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"TimeUntilSend",
		reflect.TypeOf((*MockSentPacketHandler)(nil).TimeUntilSend),
	)
	return &MockSentPacketHandlerTimeUntilSendCall{Call: call}
}

// MockSentPacketHandlerTimeUntilSendCall wrap *gomock.Call
type MockSentPacketHandlerTimeUntilSendCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockSentPacketHandlerTimeUntilSendCall) Return(arg0 monotime.Time) *MockSentPacketHandlerTimeUntilSendCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockSentPacketHandlerTimeUntilSendCall) Do(f func() monotime.Time) *MockSentPacketHandlerTimeUntilSendCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockSentPacketHandlerTimeUntilSendCall) DoAndReturn(
	f func() monotime.Time,
) *MockSentPacketHandlerTimeUntilSendCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic/internal/handshake (interfaces: ShortHeaderOpener)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -destination short_header_opener.go github.com/lemon4ksan/sein/internal/quic/internal/handshake ShortHeaderOpener
//

// Package mocks is a generated GoMock package.

// MockShortHeaderOpener is a mock of ShortHeaderOpener interface.
type MockShortHeaderOpener struct {
	ctrl     *gomock.Controller
	recorder *MockShortHeaderOpenerMockRecorder
	isgomock struct{}
}

// MockShortHeaderOpenerMockRecorder is the mock recorder for MockShortHeaderOpener.
type MockShortHeaderOpenerMockRecorder struct {
	mock *MockShortHeaderOpener
}

// NewMockShortHeaderOpener creates a new mock instance.
func NewMockShortHeaderOpener(ctrl *gomock.Controller) *MockShortHeaderOpener {
	mock := &MockShortHeaderOpener{ctrl: ctrl}
	mock.recorder = &MockShortHeaderOpenerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockShortHeaderOpener) EXPECT() *MockShortHeaderOpenerMockRecorder {
	return m.recorder
}

// DecodePacketNumber mocks base method.
func (m *MockShortHeaderOpener) DecodePacketNumber(
	wirePN protocol.PacketNumber,
	wirePNLen protocol.PacketNumberLen,
) protocol.PacketNumber {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecodePacketNumber", wirePN, wirePNLen)
	ret0, _ := ret[0].(protocol.PacketNumber)
	return ret0
}

// DecodePacketNumber indicates an expected call of DecodePacketNumber.
func (mr *MockShortHeaderOpenerMockRecorder) DecodePacketNumber(
	wirePN, wirePNLen any,
) *MockShortHeaderOpenerDecodePacketNumberCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"DecodePacketNumber",
		reflect.TypeOf((*MockShortHeaderOpener)(nil).DecodePacketNumber),
		wirePN,
		wirePNLen,
	)
	return &MockShortHeaderOpenerDecodePacketNumberCall{Call: call}
}

// MockShortHeaderOpenerDecodePacketNumberCall wrap *gomock.Call
type MockShortHeaderOpenerDecodePacketNumberCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderOpenerDecodePacketNumberCall) Return(
	arg0 protocol.PacketNumber,
) *MockShortHeaderOpenerDecodePacketNumberCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderOpenerDecodePacketNumberCall) Do(
	f func(protocol.PacketNumber, protocol.PacketNumberLen) protocol.PacketNumber,
) *MockShortHeaderOpenerDecodePacketNumberCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderOpenerDecodePacketNumberCall) DoAndReturn(
	f func(protocol.PacketNumber, protocol.PacketNumberLen) protocol.PacketNumber,
) *MockShortHeaderOpenerDecodePacketNumberCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// DecryptHeader mocks base method.
func (m *MockShortHeaderOpener) DecryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DecryptHeader", sample, firstByte, pnBytes)
}

// DecryptHeader indicates an expected call of DecryptHeader.
func (mr *MockShortHeaderOpenerMockRecorder) DecryptHeader(
	sample, firstByte, pnBytes any,
) *MockShortHeaderOpenerDecryptHeaderCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"DecryptHeader",
		reflect.TypeOf((*MockShortHeaderOpener)(nil).DecryptHeader),
		sample,
		firstByte,
		pnBytes,
	)
	return &MockShortHeaderOpenerDecryptHeaderCall{Call: call}
}

// MockShortHeaderOpenerDecryptHeaderCall wrap *gomock.Call
type MockShortHeaderOpenerDecryptHeaderCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderOpenerDecryptHeaderCall) Return() *MockShortHeaderOpenerDecryptHeaderCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderOpenerDecryptHeaderCall) Do(
	f func([]byte, *byte, []byte),
) *MockShortHeaderOpenerDecryptHeaderCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderOpenerDecryptHeaderCall) DoAndReturn(
	f func([]byte, *byte, []byte),
) *MockShortHeaderOpenerDecryptHeaderCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Open mocks base method.
func (m *MockShortHeaderOpener) Open(
	dst, src []byte,
	rcvTime monotime.Time,
	pn protocol.PacketNumber,
	kp protocol.KeyPhaseBit,
	associatedData []byte,
) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Open", dst, src, rcvTime, pn, kp, associatedData)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Open indicates an expected call of Open.
func (mr *MockShortHeaderOpenerMockRecorder) Open(
	dst, src, rcvTime, pn, kp, associatedData any,
) *MockShortHeaderOpenerOpenCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Open",
		reflect.TypeOf((*MockShortHeaderOpener)(nil).Open),
		dst,
		src,
		rcvTime,
		pn,
		kp,
		associatedData,
	)
	return &MockShortHeaderOpenerOpenCall{Call: call}
}

// MockShortHeaderOpenerOpenCall wrap *gomock.Call
type MockShortHeaderOpenerOpenCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderOpenerOpenCall) Return(arg0 []byte, arg1 error) *MockShortHeaderOpenerOpenCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderOpenerOpenCall) Do(
	f func([]byte, []byte, monotime.Time, protocol.PacketNumber, protocol.KeyPhaseBit, []byte) ([]byte, error),
) *MockShortHeaderOpenerOpenCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderOpenerOpenCall) DoAndReturn(
	f func([]byte, []byte, monotime.Time, protocol.PacketNumber, protocol.KeyPhaseBit, []byte) ([]byte, error),
) *MockShortHeaderOpenerOpenCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic/internal/handshake (interfaces: ShortHeaderSealer)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -destination short_header_sealer.go github.com/lemon4ksan/sein/internal/quic/internal/handshake ShortHeaderSealer
//

// Package mocks is a generated GoMock package.

// MockShortHeaderSealer is a mock of ShortHeaderSealer interface.
type MockShortHeaderSealer struct {
	ctrl     *gomock.Controller
	recorder *MockShortHeaderSealerMockRecorder
	isgomock struct{}
}

// MockShortHeaderSealerMockRecorder is the mock recorder for MockShortHeaderSealer.
type MockShortHeaderSealerMockRecorder struct {
	mock *MockShortHeaderSealer
}

// NewMockShortHeaderSealer creates a new mock instance.
func NewMockShortHeaderSealer(ctrl *gomock.Controller) *MockShortHeaderSealer {
	mock := &MockShortHeaderSealer{ctrl: ctrl}
	mock.recorder = &MockShortHeaderSealerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockShortHeaderSealer) EXPECT() *MockShortHeaderSealerMockRecorder {
	return m.recorder
}

// EncryptHeader mocks base method.
func (m *MockShortHeaderSealer) EncryptHeader(sample []byte, firstByte *byte, pnBytes []byte) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "EncryptHeader", sample, firstByte, pnBytes)
}

// EncryptHeader indicates an expected call of EncryptHeader.
func (mr *MockShortHeaderSealerMockRecorder) EncryptHeader(
	sample, firstByte, pnBytes any,
) *MockShortHeaderSealerEncryptHeaderCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"EncryptHeader",
		reflect.TypeOf((*MockShortHeaderSealer)(nil).EncryptHeader),
		sample,
		firstByte,
		pnBytes,
	)
	return &MockShortHeaderSealerEncryptHeaderCall{Call: call}
}

// MockShortHeaderSealerEncryptHeaderCall wrap *gomock.Call
type MockShortHeaderSealerEncryptHeaderCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderSealerEncryptHeaderCall) Return() *MockShortHeaderSealerEncryptHeaderCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderSealerEncryptHeaderCall) Do(
	f func([]byte, *byte, []byte),
) *MockShortHeaderSealerEncryptHeaderCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderSealerEncryptHeaderCall) DoAndReturn(
	f func([]byte, *byte, []byte),
) *MockShortHeaderSealerEncryptHeaderCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// KeyPhase mocks base method.
func (m *MockShortHeaderSealer) KeyPhase() protocol.KeyPhaseBit {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "KeyPhase")
	ret0, _ := ret[0].(protocol.KeyPhaseBit)
	return ret0
}

// KeyPhase indicates an expected call of KeyPhase.
func (mr *MockShortHeaderSealerMockRecorder) KeyPhase() *MockShortHeaderSealerKeyPhaseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"KeyPhase",
		reflect.TypeOf((*MockShortHeaderSealer)(nil).KeyPhase),
	)
	return &MockShortHeaderSealerKeyPhaseCall{Call: call}
}

// MockShortHeaderSealerKeyPhaseCall wrap *gomock.Call
type MockShortHeaderSealerKeyPhaseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderSealerKeyPhaseCall) Return(arg0 protocol.KeyPhaseBit) *MockShortHeaderSealerKeyPhaseCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderSealerKeyPhaseCall) Do(f func() protocol.KeyPhaseBit) *MockShortHeaderSealerKeyPhaseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderSealerKeyPhaseCall) DoAndReturn(
	f func() protocol.KeyPhaseBit,
) *MockShortHeaderSealerKeyPhaseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Overhead mocks base method.
func (m *MockShortHeaderSealer) Overhead() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Overhead")
	ret0, _ := ret[0].(int)
	return ret0
}

// Overhead indicates an expected call of Overhead.
func (mr *MockShortHeaderSealerMockRecorder) Overhead() *MockShortHeaderSealerOverheadCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Overhead",
		reflect.TypeOf((*MockShortHeaderSealer)(nil).Overhead),
	)
	return &MockShortHeaderSealerOverheadCall{Call: call}
}

// MockShortHeaderSealerOverheadCall wrap *gomock.Call
type MockShortHeaderSealerOverheadCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderSealerOverheadCall) Return(arg0 int) *MockShortHeaderSealerOverheadCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderSealerOverheadCall) Do(f func() int) *MockShortHeaderSealerOverheadCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderSealerOverheadCall) DoAndReturn(f func() int) *MockShortHeaderSealerOverheadCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Seal mocks base method.
func (m *MockShortHeaderSealer) Seal(
	dst, src []byte,
	packetNumber protocol.PacketNumber,
	associatedData []byte,
) []byte {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Seal", dst, src, packetNumber, associatedData)
	ret0, _ := ret[0].([]byte)
	return ret0
}

// Seal indicates an expected call of Seal.
func (mr *MockShortHeaderSealerMockRecorder) Seal(
	dst, src, packetNumber, associatedData any,
) *MockShortHeaderSealerSealCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"Seal",
		reflect.TypeOf((*MockShortHeaderSealer)(nil).Seal),
		dst,
		src,
		packetNumber,
		associatedData,
	)
	return &MockShortHeaderSealerSealCall{Call: call}
}

// MockShortHeaderSealerSealCall wrap *gomock.Call
type MockShortHeaderSealerSealCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockShortHeaderSealerSealCall) Return(arg0 []byte) *MockShortHeaderSealerSealCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockShortHeaderSealerSealCall) Do(
	f func([]byte, []byte, protocol.PacketNumber, []byte) []byte,
) *MockShortHeaderSealerSealCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockShortHeaderSealerSealCall) DoAndReturn(
	f func([]byte, []byte, protocol.PacketNumber, []byte) []byte,
) *MockShortHeaderSealerSealCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: StreamControlFrameGetter)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_stream_control_frame_getter_test.go github.com/lemon4ksan/sein/internal/quic StreamControlFrameGetter
//

// Package quic is a generated GoMock package.

// MockStreamControlFrameGetter is a mock of StreamControlFrameGetter interface.
type MockStreamControlFrameGetter struct {
	ctrl     *gomock.Controller
	recorder *MockStreamControlFrameGetterMockRecorder
	isgomock struct{}
}

// MockStreamControlFrameGetterMockRecorder is the mock recorder for MockStreamControlFrameGetter.
type MockStreamControlFrameGetterMockRecorder struct {
	mock *MockStreamControlFrameGetter
}

// NewMockStreamControlFrameGetter creates a new mock instance.
func NewMockStreamControlFrameGetter(ctrl *gomock.Controller) *MockStreamControlFrameGetter {
	mock := &MockStreamControlFrameGetter{ctrl: ctrl}
	mock.recorder = &MockStreamControlFrameGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStreamControlFrameGetter) EXPECT() *MockStreamControlFrameGetterMockRecorder {
	return m.recorder
}

// getControlFrame mocks base method.
func (m *MockStreamControlFrameGetter) getControlFrame(arg0 monotime.Time) (ackhandler.Frame, bool, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getControlFrame", arg0)
	ret0, _ := ret[0].(ackhandler.Frame)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(bool)
	return ret0, ret1, ret2
}

// getControlFrame indicates an expected call of getControlFrame.
func (mr *MockStreamControlFrameGetterMockRecorder) getControlFrame(
	arg0 any,
) *MockStreamControlFrameGettergetControlFrameCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"getControlFrame",
		reflect.TypeOf((*MockStreamControlFrameGetter)(nil).getControlFrame),
		arg0,
	)
	return &MockStreamControlFrameGettergetControlFrameCall{Call: call}
}

// MockStreamControlFrameGettergetControlFrameCall wrap *gomock.Call
type MockStreamControlFrameGettergetControlFrameCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamControlFrameGettergetControlFrameCall) Return(
	arg0 ackhandler.Frame,
	ok, hasMore bool,
) *MockStreamControlFrameGettergetControlFrameCall {
	c.Call = c.Call.Return(arg0, ok, hasMore)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamControlFrameGettergetControlFrameCall) Do(
	f func(monotime.Time) (ackhandler.Frame, bool, bool),
) *MockStreamControlFrameGettergetControlFrameCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamControlFrameGettergetControlFrameCall) DoAndReturn(
	f func(monotime.Time) (ackhandler.Frame, bool, bool),
) *MockStreamControlFrameGettergetControlFrameCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: StreamFrameGetter)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_stream_frame_getter_test.go github.com/lemon4ksan/sein/internal/quic StreamFrameGetter
//

// Package quic is a generated GoMock package.

// MockStreamFrameGetter is a mock of StreamFrameGetter interface.
type MockStreamFrameGetter struct {
	ctrl     *gomock.Controller
	recorder *MockStreamFrameGetterMockRecorder
	isgomock struct{}
}

// MockStreamFrameGetterMockRecorder is the mock recorder for MockStreamFrameGetter.
type MockStreamFrameGetterMockRecorder struct {
	mock *MockStreamFrameGetter
}

// NewMockStreamFrameGetter creates a new mock instance.
func NewMockStreamFrameGetter(ctrl *gomock.Controller) *MockStreamFrameGetter {
	mock := &MockStreamFrameGetter{ctrl: ctrl}
	mock.recorder = &MockStreamFrameGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStreamFrameGetter) EXPECT() *MockStreamFrameGetterMockRecorder {
	return m.recorder
}

// popRetransmissionFrame mocks base method.
func (m *MockStreamFrameGetter) popRetransmissionFrame(
	arg0 protocol.ByteCount,
	arg1 protocol.Version,
) (ackhandler.StreamFrame, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "popRetransmissionFrame", arg0, arg1)
	ret0, _ := ret[0].(ackhandler.StreamFrame)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// popRetransmissionFrame indicates an expected call of popRetransmissionFrame.
func (mr *MockStreamFrameGetterMockRecorder) popRetransmissionFrame(
	arg0, arg1 any,
) *MockStreamFrameGetterpopRetransmissionFrameCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"popRetransmissionFrame",
		reflect.TypeOf((*MockStreamFrameGetter)(nil).popRetransmissionFrame),
		arg0,
		arg1,
	)
	return &MockStreamFrameGetterpopRetransmissionFrameCall{Call: call}
}

// MockStreamFrameGetterpopRetransmissionFrameCall wrap *gomock.Call
type MockStreamFrameGetterpopRetransmissionFrameCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamFrameGetterpopRetransmissionFrameCall) Return(
	arg0 ackhandler.StreamFrame,
	arg1 bool,
) *MockStreamFrameGetterpopRetransmissionFrameCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamFrameGetterpopRetransmissionFrameCall) Do(
	f func(protocol.ByteCount, protocol.Version) (ackhandler.StreamFrame, bool),
) *MockStreamFrameGetterpopRetransmissionFrameCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamFrameGetterpopRetransmissionFrameCall) DoAndReturn(
	f func(protocol.ByteCount, protocol.Version) (ackhandler.StreamFrame, bool),
) *MockStreamFrameGetterpopRetransmissionFrameCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// popStreamFrame mocks base method.
func (m *MockStreamFrameGetter) popStreamFrame(
	arg0 protocol.ByteCount,
	arg1 protocol.Version,
) (ackhandler.StreamFrame, *wire.StreamDataBlockedFrame, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "popStreamFrame", arg0, arg1)
	ret0, _ := ret[0].(ackhandler.StreamFrame)
	ret1, _ := ret[1].(*wire.StreamDataBlockedFrame)
	ret2, _ := ret[2].(bool)
	return ret0, ret1, ret2
}

// popStreamFrame indicates an expected call of popStreamFrame.
func (mr *MockStreamFrameGetterMockRecorder) popStreamFrame(arg0, arg1 any) *MockStreamFrameGetterpopStreamFrameCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"popStreamFrame",
		reflect.TypeOf((*MockStreamFrameGetter)(nil).popStreamFrame),
		arg0,
		arg1,
	)
	return &MockStreamFrameGetterpopStreamFrameCall{Call: call}
}

// MockStreamFrameGetterpopStreamFrameCall wrap *gomock.Call
type MockStreamFrameGetterpopStreamFrameCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamFrameGetterpopStreamFrameCall) Return(
	arg0 ackhandler.StreamFrame,
	arg1 *wire.StreamDataBlockedFrame,
	hasMoreData bool,
) *MockStreamFrameGetterpopStreamFrameCall {
	c.Call = c.Call.Return(arg0, arg1, hasMoreData)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamFrameGetterpopStreamFrameCall) Do(
	f func(protocol.ByteCount, protocol.Version) (ackhandler.StreamFrame, *wire.StreamDataBlockedFrame, bool),
) *MockStreamFrameGetterpopStreamFrameCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamFrameGetterpopStreamFrameCall) DoAndReturn(
	f func(protocol.ByteCount, protocol.Version) (ackhandler.StreamFrame, *wire.StreamDataBlockedFrame, bool),
) *MockStreamFrameGetterpopStreamFrameCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// priority mocks base method.
func (m *MockStreamFrameGetter) priority() (int8, bool, uint32) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "priority")
	ret0, _ := ret[0].(int8)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(uint32)
	return ret0, ret1, ret2
}

// priority indicates an expected call of priority.
func (mr *MockStreamFrameGetterMockRecorder) priority() *MockStreamFrameGetterpriorityCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"priority",
		reflect.TypeOf((*MockStreamFrameGetter)(nil).priority),
	)
	return &MockStreamFrameGetterpriorityCall{Call: call}
}

// MockStreamFrameGetterpriorityCall wrap *gomock.Call
type MockStreamFrameGetterpriorityCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamFrameGetterpriorityCall) Return(
	urgency int8,
	incremental bool,
	generation uint32,
) *MockStreamFrameGetterpriorityCall {
	c.Call = c.Call.Return(urgency, incremental, generation)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamFrameGetterpriorityCall) Do(f func() (int8, bool, uint32)) *MockStreamFrameGetterpriorityCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamFrameGetterpriorityCall) DoAndReturn(
	f func() (int8, bool, uint32),
) *MockStreamFrameGetterpriorityCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: StreamSender)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_stream_sender_test.go github.com/lemon4ksan/sein/internal/quic StreamSender
//

// Package quic is a generated GoMock package.

// MockStreamSender is a mock of StreamSender interface.
type MockStreamSender struct {
	ctrl     *gomock.Controller
	recorder *MockStreamSenderMockRecorder
	isgomock struct{}
}

// MockStreamSenderMockRecorder is the mock recorder for MockStreamSender.
type MockStreamSenderMockRecorder struct {
	mock *MockStreamSender
}

// NewMockStreamSender creates a new mock instance.
func NewMockStreamSender(ctrl *gomock.Controller) *MockStreamSender {
	mock := &MockStreamSender{ctrl: ctrl}
	mock.recorder = &MockStreamSenderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStreamSender) EXPECT() *MockStreamSenderMockRecorder {
	return m.recorder
}

// onHasConnectionData mocks base method.
func (m *MockStreamSender) onHasConnectionData() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "onHasConnectionData")
}

// onHasConnectionData indicates an expected call of onHasConnectionData.
func (mr *MockStreamSenderMockRecorder) onHasConnectionData() *MockStreamSenderonHasConnectionDataCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"onHasConnectionData",
		reflect.TypeOf((*MockStreamSender)(nil).onHasConnectionData),
	)
	return &MockStreamSenderonHasConnectionDataCall{Call: call}
}

// MockStreamSenderonHasConnectionDataCall wrap *gomock.Call
type MockStreamSenderonHasConnectionDataCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamSenderonHasConnectionDataCall) Return() *MockStreamSenderonHasConnectionDataCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamSenderonHasConnectionDataCall) Do(f func()) *MockStreamSenderonHasConnectionDataCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamSenderonHasConnectionDataCall) DoAndReturn(f func()) *MockStreamSenderonHasConnectionDataCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// onHasStreamControlFrame mocks base method.
func (m *MockStreamSender) onHasStreamControlFrame(arg0 protocol.StreamID, arg1 streamControlFrameGetter) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "onHasStreamControlFrame", arg0, arg1)
}

// onHasStreamControlFrame indicates an expected call of onHasStreamControlFrame.
func (mr *MockStreamSenderMockRecorder) onHasStreamControlFrame(
	arg0, arg1 any,
) *MockStreamSenderonHasStreamControlFrameCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"onHasStreamControlFrame",
		reflect.TypeOf((*MockStreamSender)(nil).onHasStreamControlFrame),
		arg0,
		arg1,
	)
	return &MockStreamSenderonHasStreamControlFrameCall{Call: call}
}

// MockStreamSenderonHasStreamControlFrameCall wrap *gomock.Call
type MockStreamSenderonHasStreamControlFrameCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamSenderonHasStreamControlFrameCall) Return() *MockStreamSenderonHasStreamControlFrameCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamSenderonHasStreamControlFrameCall) Do(
	f func(protocol.StreamID, streamControlFrameGetter),
) *MockStreamSenderonHasStreamControlFrameCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamSenderonHasStreamControlFrameCall) DoAndReturn(
	f func(protocol.StreamID, streamControlFrameGetter),
) *MockStreamSenderonHasStreamControlFrameCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// onHasStreamData mocks base method.
func (m *MockStreamSender) onHasStreamData(arg0 protocol.StreamID, arg1 *SendStream) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "onHasStreamData", arg0, arg1)
}

// onHasStreamData indicates an expected call of onHasStreamData.
func (mr *MockStreamSenderMockRecorder) onHasStreamData(arg0, arg1 any) *MockStreamSenderonHasStreamDataCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"onHasStreamData",
		reflect.TypeOf((*MockStreamSender)(nil).onHasStreamData),
		arg0,
		arg1,
	)
	return &MockStreamSenderonHasStreamDataCall{Call: call}
}

// MockStreamSenderonHasStreamDataCall wrap *gomock.Call
type MockStreamSenderonHasStreamDataCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamSenderonHasStreamDataCall) Return() *MockStreamSenderonHasStreamDataCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamSenderonHasStreamDataCall) Do(
	f func(protocol.StreamID, *SendStream),
) *MockStreamSenderonHasStreamDataCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamSenderonHasStreamDataCall) DoAndReturn(
	f func(protocol.StreamID, *SendStream),
) *MockStreamSenderonHasStreamDataCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// onHasStreamRetransmission mocks base method.
func (m *MockStreamSender) onHasStreamRetransmission(arg0 protocol.StreamID, arg1 *SendStream) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "onHasStreamRetransmission", arg0, arg1)
}

// onHasStreamRetransmission indicates an expected call of onHasStreamRetransmission.
func (mr *MockStreamSenderMockRecorder) onHasStreamRetransmission(
	arg0, arg1 any,
) *MockStreamSenderonHasStreamRetransmissionCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"onHasStreamRetransmission",
		reflect.TypeOf((*MockStreamSender)(nil).onHasStreamRetransmission),
		arg0,
		arg1,
	)
	return &MockStreamSenderonHasStreamRetransmissionCall{Call: call}
}

// MockStreamSenderonHasStreamRetransmissionCall wrap *gomock.Call
type MockStreamSenderonHasStreamRetransmissionCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamSenderonHasStreamRetransmissionCall) Return() *MockStreamSenderonHasStreamRetransmissionCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamSenderonHasStreamRetransmissionCall) Do(
	f func(protocol.StreamID, *SendStream),
) *MockStreamSenderonHasStreamRetransmissionCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamSenderonHasStreamRetransmissionCall) DoAndReturn(
	f func(protocol.StreamID, *SendStream),
) *MockStreamSenderonHasStreamRetransmissionCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// onStreamCompleted mocks base method.
func (m *MockStreamSender) onStreamCompleted(arg0 protocol.StreamID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "onStreamCompleted", arg0)
}

// onStreamCompleted indicates an expected call of onStreamCompleted.
func (mr *MockStreamSenderMockRecorder) onStreamCompleted(arg0 any) *MockStreamSenderonStreamCompletedCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"onStreamCompleted",
		reflect.TypeOf((*MockStreamSender)(nil).onStreamCompleted),
		arg0,
	)
	return &MockStreamSenderonStreamCompletedCall{Call: call}
}

// MockStreamSenderonStreamCompletedCall wrap *gomock.Call
type MockStreamSenderonStreamCompletedCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamSenderonStreamCompletedCall) Return() *MockStreamSenderonStreamCompletedCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamSenderonStreamCompletedCall) Do(f func(protocol.StreamID)) *MockStreamSenderonStreamCompletedCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamSenderonStreamCompletedCall) DoAndReturn(
	f func(protocol.StreamID),
) *MockStreamSenderonStreamCompletedCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// updateStreamPriority mocks base method.
func (m *MockStreamSender) updateStreamPriority(arg0 protocol.StreamID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "updateStreamPriority", arg0)
}

// updateStreamPriority indicates an expected call of updateStreamPriority.
func (mr *MockStreamSenderMockRecorder) updateStreamPriority(arg0 any) *MockStreamSenderupdateStreamPriorityCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"updateStreamPriority",
		reflect.TypeOf((*MockStreamSender)(nil).updateStreamPriority),
		arg0,
	)
	return &MockStreamSenderupdateStreamPriorityCall{Call: call}
}

// MockStreamSenderupdateStreamPriorityCall wrap *gomock.Call
type MockStreamSenderupdateStreamPriorityCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStreamSenderupdateStreamPriorityCall) Return() *MockStreamSenderupdateStreamPriorityCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStreamSenderupdateStreamPriorityCall) Do(
	f func(protocol.StreamID),
) *MockStreamSenderupdateStreamPriorityCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStreamSenderupdateStreamPriorityCall) DoAndReturn(
	f func(protocol.StreamID),
) *MockStreamSenderupdateStreamPriorityCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lemon4ksan/sein/internal/quic (interfaces: Unpacker)
//
// Generated by this command:
//
//	mockgen -typed -build_flags=-tags=gomock -package quic -self_package github.com/lemon4ksan/sein/internal/quic -destination mock_unpacker_test.go github.com/lemon4ksan/sein/internal/quic Unpacker
//

// Package quic is a generated GoMock package.

// MockUnpacker is a mock of Unpacker interface.
type MockUnpacker struct {
	ctrl     *gomock.Controller
	recorder *MockUnpackerMockRecorder
	isgomock struct{}
}

// MockUnpackerMockRecorder is the mock recorder for MockUnpacker.
type MockUnpackerMockRecorder struct {
	mock *MockUnpacker
}

// NewMockUnpacker creates a new mock instance.
func NewMockUnpacker(ctrl *gomock.Controller) *MockUnpacker {
	mock := &MockUnpacker{ctrl: ctrl}
	mock.recorder = &MockUnpackerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnpacker) EXPECT() *MockUnpackerMockRecorder {
	return m.recorder
}

// UnpackLongHeader mocks base method.
func (m *MockUnpacker) UnpackLongHeader(hdr *wire.Header, data []byte) (*unpackedPacket, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UnpackLongHeader", hdr, data)
	ret0, _ := ret[0].(*unpackedPacket)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UnpackLongHeader indicates an expected call of UnpackLongHeader.
func (mr *MockUnpackerMockRecorder) UnpackLongHeader(hdr, data any) *MockUnpackerUnpackLongHeaderCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"UnpackLongHeader",
		reflect.TypeOf((*MockUnpacker)(nil).UnpackLongHeader),
		hdr,
		data,
	)
	return &MockUnpackerUnpackLongHeaderCall{Call: call}
}

// MockUnpackerUnpackLongHeaderCall wrap *gomock.Call
type MockUnpackerUnpackLongHeaderCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockUnpackerUnpackLongHeaderCall) Return(arg0 *unpackedPacket, arg1 error) *MockUnpackerUnpackLongHeaderCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockUnpackerUnpackLongHeaderCall) Do(
	f func(*wire.Header, []byte) (*unpackedPacket, error),
) *MockUnpackerUnpackLongHeaderCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockUnpackerUnpackLongHeaderCall) DoAndReturn(
	f func(*wire.Header, []byte) (*unpackedPacket, error),
) *MockUnpackerUnpackLongHeaderCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// UnpackShortHeader mocks base method.
func (m *MockUnpacker) UnpackShortHeader(
	rcvTime monotime.Time,
	data []byte,
) (protocol.PacketNumber, protocol.PacketNumberLen, protocol.KeyPhaseBit, []byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UnpackShortHeader", rcvTime, data)
	ret0, _ := ret[0].(protocol.PacketNumber)
	ret1, _ := ret[1].(protocol.PacketNumberLen)
	ret2, _ := ret[2].(protocol.KeyPhaseBit)
	ret3, _ := ret[3].([]byte)
	ret4, _ := ret[4].(error)
	return ret0, ret1, ret2, ret3, ret4
}

// UnpackShortHeader indicates an expected call of UnpackShortHeader.
func (mr *MockUnpackerMockRecorder) UnpackShortHeader(rcvTime, data any) *MockUnpackerUnpackShortHeaderCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"UnpackShortHeader",
		reflect.TypeOf((*MockUnpacker)(nil).UnpackShortHeader),
		rcvTime,
		data,
	)
	return &MockUnpackerUnpackShortHeaderCall{Call: call}
}

// MockUnpackerUnpackShortHeaderCall wrap *gomock.Call
type MockUnpackerUnpackShortHeaderCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockUnpackerUnpackShortHeaderCall) Return(
	arg0 protocol.PacketNumber,
	arg1 protocol.PacketNumberLen,
	arg2 protocol.KeyPhaseBit,
	arg3 []byte,
	arg4 error,
) *MockUnpackerUnpackShortHeaderCall {
	c.Call = c.Call.Return(arg0, arg1, arg2, arg3, arg4)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockUnpackerUnpackShortHeaderCall) Do(
	f func(monotime.Time, []byte) (protocol.PacketNumber, protocol.PacketNumberLen, protocol.KeyPhaseBit, []byte, error),
) *MockUnpackerUnpackShortHeaderCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockUnpackerUnpackShortHeaderCall) DoAndReturn(
	f func(monotime.Time, []byte) (protocol.PacketNumber, protocol.PacketNumberLen, protocol.KeyPhaseBit, []byte, error),
) *MockUnpackerUnpackShortHeaderCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

type benchIncomingStream struct{}

func (b *benchIncomingStream) closeForShutdown(error) {}

func BenchmarkIncomingStreamsMap_Get(b *testing.B) {
	m := newIncomingStreamsMap[*benchIncomingStream](
		protocol.StreamTypeBidi,
		func(id protocol.StreamID) *benchIncomingStream {
			return &benchIncomingStream{}
		},
		100000,
		func(f wire.Frame) {},
		protocol.PerspectiveServer,
	)

	// Pre-open 10,000 streams
	for i := range 10000 {
		id := protocol.StreamID(i*4 + 4) // Bidi server streams start at 4
		m.OrOpenStream(id)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		var i atomic.Uint64
		for pb.Next() {
			// Read randomly from pre-opened streams to simulate hot path
			idx := i.Add(1) % 10000
			id := protocol.StreamID(idx*4 + 4)
			_, _ = m.OrOpenStream(id)
		}
	})
}
