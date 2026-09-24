// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "io"

// StreamSenderDelegate writes unidirectional encoder/decoder control stream data to the peer.
// Used to emit RFC 9204 encoder and decoder control stream bytes.
type StreamSenderDelegate interface {
	// WriteStreamData writes data on the unidirectional stream.
	WriteStreamData(data []byte)

	// NumBytesBuffered returns the number of bytes buffered due to underlying stream being blocked.
	NumBytesBuffered() uint64
}

type writerStreamSenderDelegate struct {
	w io.Writer
}

// StreamWriter adapts an io.Writer into a StreamSenderDelegate.
// Writes to the delegate are directly forwarded to w.
func StreamWriter(w io.Writer) StreamSenderDelegate {
	return &writerStreamSenderDelegate{w: w}
}

func (d *writerStreamSenderDelegate) WriteStreamData(data []byte) {
	_, _ = d.w.Write(data)
}

func (d *writerStreamSenderDelegate) NumBytesBuffered() uint64 {
	return 0
}
