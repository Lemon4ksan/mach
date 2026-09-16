package headers

import (
	"testing"
)

func TestParseHeaderBlockSWAR(t *testing.T) {
	h := NewWithCapacity(8)
	
	block := []byte("Host: localhost\r\nConnection: keep-alive\r\nX-Custom: val123\r\n")
	
	h.ParseHeaderBlockSWAR(block)

	if h.Get("Host") != "localhost" {
		t.Errorf("Expected Host=localhost, got %q", h.Get("Host"))
	}
	if h.Get("Connection") != "keep-alive" {
		t.Errorf("Expected Connection=keep-alive, got %q", h.Get("Connection"))
	}
	if h.Get("X-Custom") != "val123" {
		t.Errorf("Expected X-Custom=val123, got %q", h.Get("X-Custom"))
	}
}
