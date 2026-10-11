// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package altsvc_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"

	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
)

func TestParse(t *testing.T) {
	header := `h3=":443"; ma=2592000, h2="alt.example.com:8443"; persist=1, clear`

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

func TestParseBytes(t *testing.T) {
	header := []byte(`h3=":443"; ma=2592000, h2="alt.example.com:8443"; persist=1`)
	services := altsvc.Parse(header)

	assert.Equal(t, 2, len(services))
	assert.Equal(t, "h3", services[0].ProtocolID)
	assert.Equal(t, 443, services[0].Port)
	assert.Equal(t, "h2", services[1].ProtocolID)
	assert.Equal(t, 8443, services[1].Port)
}

func TestParseClear(t *testing.T) {
	services := altsvc.Parse("clear")
	assert.Equal(t, 0, len(services))

	services = altsvc.Parse("")
	assert.Equal(t, 0, len(services))
}

func TestFormat(t *testing.T) {
	services := []altsvc.Service{
		{ProtocolID: "h3", Host: "", Port: 443, MaxAge: 2592000 * time.Second, Persist: false},
		{ProtocolID: "h2", Host: "alt.example.com", Port: 8443, MaxAge: altsvc.DefaultMaxAge, Persist: true},
	}

	formatted := altsvc.Format(services...)
	parsed := altsvc.Parse(formatted)

	assert.Equal(t, len(services), len(parsed))
	assert.Equal(t, services[0].ProtocolID, parsed[0].ProtocolID)
	assert.Equal(t, services[0].Port, parsed[0].Port)
	assert.Equal(t, services[0].MaxAge, parsed[0].MaxAge)
	assert.Equal(t, services[1].ProtocolID, parsed[1].ProtocolID)
	assert.Equal(t, services[1].Host, parsed[1].Host)
	assert.Equal(t, services[1].Port, parsed[1].Port)
	assert.Equal(t, services[1].Persist, parsed[1].Persist)
}
