package ws

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Trailing bytes stripped by permessage-deflate (RFC 7692 Section 7.2.1).
var flateTail = []byte{0x00, 0x00, 0xff, 0xff}

// Trailing bytes appended to complete the stream with an empty BFINAL=1 block for flate.Reader.
var flateReaderTail = []byte{0x00, 0x00, 0xff, 0xff, 0x01, 0x00, 0x00, 0xff, 0xff}

// ErrInvalidCompressedPayload is returned when decompressed data fails.
var ErrInvalidCompressedPayload = errors.New("websocket: invalid compressed payload")

// DeflateOptions configures permessage-deflate extension (RFC 7692).
type DeflateOptions struct {
	Level                   int  // Compression level (flate.DefaultCompression, etc.)
	ServerNoContextTakeover bool // server_no_context_takeover
	ClientNoContextTakeover bool // client_no_context_takeover
}

// DefaultDeflateOptions returns standard permessage-deflate options.
func DefaultDeflateOptions() DeflateOptions {
	return DeflateOptions{
		Level:                   flate.DefaultCompression,
		ServerNoContextTakeover: true,
		ClientNoContextTakeover: true,
	}
}

type flateWriterPool struct {
	pool sync.Pool
}

func newFlateWriterPool() *flateWriterPool {
	return &flateWriterPool{
		pool: sync.Pool{
			New: func() any {
				w, _ := flate.NewWriter(io.Discard, flate.DefaultCompression)
				return w
			},
		},
	}
}

func (p *flateWriterPool) get(w io.Writer, level int) *flate.Writer {
	fw := p.pool.Get().(*flate.Writer)
	fw.Reset(w)
	return fw
}

func (p *flateWriterPool) put(fw *flate.Writer) {
	p.pool.Put(fw)
}

var globalWriterPool = newFlateWriterPool()

// CompressDeflate compresses data using RFC 7692 raw DEFLATE:
// - Uses raw DEFLATE (flate) without zlib header
// - Flushes compressor
// - Strips trailing 0x00 0x00 0xff 0xff bytes
func CompressDeflate(src []byte, level int) ([]byte, error) {
	if level == 0 {
		level = flate.DefaultCompression
	}

	var buf bytes.Buffer

	fw := globalWriterPool.get(&buf, level)
	defer globalWriterPool.put(fw)

	if _, err := fw.Write(src); err != nil {
		return nil, err
	}

	if err := fw.Flush(); err != nil {
		return nil, err
	}

	compressed := buf.Bytes()
	// Strip trailing 0x00, 0x00, 0xff, 0xff per RFC 7692 §7.2.1
	if len(compressed) >= 4 && bytes.HasSuffix(compressed, flateTail) {
		compressed = compressed[:len(compressed)-4]
	}

	result := make([]byte, len(compressed))
	copy(result, compressed)

	return result, nil
}

// DecompressDeflate decompresses data using RFC 7692 raw DEFLATE:
// - Appends trailing 0x00 0x00 0xff 0xff bytes
// - Decompresses raw DEFLATE stream
func DecompressDeflate(src []byte, limit int64) ([]byte, error) {
	// Reconstruct tail with BFINAL=1 termination
	tailReader := io.MultiReader(bytes.NewReader(src), bytes.NewReader(flateReaderTail))

	fr := flate.NewReader(tailReader)
	defer fr.Close()

	var (
		buf bytes.Buffer
		r   io.Reader = fr
	)
	if limit > 0 {
		r = io.LimitReader(fr, limit+1)
	}

	if _, err := buf.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCompressedPayload, err)
	}

	if limit > 0 && int64(buf.Len()) > limit {
		return nil, ErrMessageTooBig
	}

	return buf.Bytes(), nil
}
