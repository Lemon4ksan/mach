package altsvc_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"

	"github.com/lemon4ksan/mach/proto/http/altsvc"
)

func TestParse(t *testing.T) {
	header := "h3=\":443\"; ma=2592000, h2=\"alt.example.com:8443\"; persist=1, clear"

	services := altsvc.Parse(header)

	assert.Equal(t, 2, len(services))

	assert.Equal(t, "h3", services[0].ProtocolID)
	assert.Equal(t, "", services[0].Host)
	assert.Equal(t, 443, services[0].Port)
	assert.Equal(t, 2592000*time.Second, services[0].MaxAge)
	assert.Equal(t, false, services[0].Persist)

	assert.Equal(t, "h2", services[1].ProtocolID)
	assert.Equal(t, "alt.example.com", services[1].Host)
	assert.Equal(t, 8443, services[1].Port)
	assert.Equal(t, 86400*time.Second, services[1].MaxAge)
	assert.Equal(t, true, services[1].Persist)
}

func TestParseClear(t *testing.T) {
	services := altsvc.Parse("clear")
	assert.Equal(t, 0, len(services))
}
