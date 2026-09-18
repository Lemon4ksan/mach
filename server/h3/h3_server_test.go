// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	"github.com/lemon4ksan/foundation/net/qpack"
	"github.com/lemon4ksan/foundation/net/quic"
	"github.com/lemon4ksan/mach/server/h3"
)

func generateTestTLSConfig(t *testing.T) (*tls.Config, *tls.Config) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Sein Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"h3"},
	}

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3"},
	}

	return serverTLS, clientTLS
}

func TestH3Server_EndToEnd(t *testing.T) {
	serverTLS, clientTLS := generateTestTLSConfig(t)

	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)

	defer listener.Close() //nolint:errcheck

	addr := listener.Addr().String()

	handler := func(req *h3.ServerRequest, res *h3.ServerResponse) error {
		switch req.Path {
		case "/hello":
			res.StatusCode = http.StatusOK
			res.Headers.Set("Content-Type", "text/plain")
			res.Body = []byte("Hello HTTP/3 QUIC World!")

			return nil

		case "/echo":
			res.StatusCode = http.StatusOK
			res.Headers.Set("Content-Type", "application/octet-stream")

			res.Body = append([]byte("H3 Echo: "), req.Body...)

			return nil

		default:
			res.StatusCode = http.StatusNotFound
			res.Body = []byte("404 Not Found")
			return nil
		}
	}

	// Server accept loop
	go func() {
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				return
			}

			sc := h3.NewServerConn(conn, handler)
			go func() {
				_ = sc.Serve()
			}()
		}
	}()

	// 1. Dial client QUIC connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, err := quic.DialAddr(ctx, addr, clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)

	defer clientConn.CloseWithError(0, "") //nolint:errcheck

	// 2. Open client unidirectional Control Stream and send SETTINGS (RFC 9114 §6.2.1)
	clientCtrl, err := clientConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	var typeBuf [8]byte

	n := varint.Append(typeBuf[:0], coreh3.StreamTypeControl)
	_, err = clientCtrl.Write(n)
	require.NoError(t, err)

	clientSettings := &coreh3.Settings{}
	_, err = clientCtrl.Write(clientSettings.Encode())
	require.NoError(t, err)

	// 3. Perform GET /hello request on bidirectional stream
	reqStream, err := clientConn.OpenStreamSync(ctx)
	require.NoError(t, err)

	var qpackBuf bytes.Buffer

	enc := qpack.NewEncoder(&qpackBuf)
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":method", Value: "GET"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":path", Value: "/hello"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":scheme", Value: "https"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":authority", Value: addr}))

	var frameHdr [16]byte

	hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
	hdrBytes = varint.Append(hdrBytes, uint64(qpackBuf.Len()))

	_, err = reqStream.Write(hdrBytes)
	require.NoError(t, err)
	_, err = reqStream.Write(qpackBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, reqStream.Close()) // Close write side to signal end of stream

	// 4. Read server response on stream
	qr := varint.NewReader(reqStream)

	// Read HEADERS frame
	respFrameType, err := varint.Read(qr)
	require.NoError(t, err)
	assert.Equal(t, coreh3.FrameTypeHeaders, respFrameType)

	respHeaderLen, err := varint.Read(qr)
	require.NoError(t, err)

	respHeaderBytes := make([]byte, respHeaderLen)
	_, err = io.ReadFull(reqStream, respHeaderBytes)
	require.NoError(t, err)

	dec := qpack.NewDecoder()

	var (
		statusCode  string
		contentType string
	)

	err = dec.DecodeFields(respHeaderBytes, nil, func(hf qpack.HeaderField) bool {
		if hf.Name == ":status" {
			statusCode = hf.Value
		}

		if hf.Name == "content-type" {
			contentType = hf.Value
		}

		return true
	})
	require.NoError(t, err)
	assert.Equal(t, "200", statusCode)
	assert.Equal(t, "text/plain", contentType)

	// Read DATA frame
	respDataFrameType, err := varint.Read(qr)
	require.NoError(t, err)
	assert.Equal(t, coreh3.FrameTypeData, respDataFrameType)

	respDataLen, err := varint.Read(qr)
	require.NoError(t, err)

	respBody := make([]byte, respDataLen)
	_, err = io.ReadFull(reqStream, respBody)
	require.NoError(t, err)
	assert.Equal(t, "Hello HTTP/3 QUIC World!", string(respBody))
}
