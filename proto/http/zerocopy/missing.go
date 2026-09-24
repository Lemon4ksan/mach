package zerocopy

import "github.com/lemon4ksan/foundation/silicon/bytesconv"

func B2S(b []byte) string { return bytesconv.B2S(b) }
func S2B(s string) []byte { return bytesconv.S2B(s) }
