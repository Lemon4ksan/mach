// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"maps"
	"testing"
)

func TestRepro_Directives_EscapedQuotesWithDelimiter(t *testing.T) {
	// A header with an odd number of escaped quotes followed by a comma inside a quoted string, then another directive
	raw := `custom="say \"hello, world", other=456`

	parsed := maps.Collect(Directives(raw))

	if _, ok := parsed["other"]; !ok {
		t.Fatalf(
			"CRITICAL: 'other' directive was lost due to unhandled escaped quote in Directives! Parsed: %#v",
			parsed,
		)
	}

	if parsed["custom"] != `say \"hello, world` {
		t.Fatalf("unexpected custom value: %q", parsed["custom"])
	}

	if parsed["other"] != "456" {
		t.Fatalf("unexpected other value: %q", parsed["other"])
	}

	// Also test DirectivesBytes
	parsedBytes := make(map[string]string)
	for k, v := range DirectivesBytes([]byte(raw)) {
		parsedBytes[string(k)] = string(v)
	}
	if _, ok := parsedBytes["other"]; !ok {
		t.Fatalf("CRITICAL: 'other' directive was lost in DirectivesBytes! Parsed: %#v", parsedBytes)
	}

	// Also test ParamDirectives
	rawParam := `custom="say \"hello; world"; other=456`
	parsedParam := maps.Collect(ParamDirectives(rawParam))
	if _, ok := parsedParam["other"]; !ok {
		t.Fatalf("CRITICAL: 'other' directive was lost in ParamDirectives! Parsed: %#v", parsedParam)
	}
}
