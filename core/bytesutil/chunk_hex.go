package bytesutil

// FormatHexUint writes the hex representation of val into buf.
// Returns the number of bytes written.
func FormatHexUint(buf *[16]byte, val int) int {
	return formatHexUintFallback(buf, val)
}

func formatHexUintFallback(buf *[16]byte, val int) int {
	if val < 0 {
		panic("BUG: int must be positive")
	}

	if val == 0 {
		buf[0] = '0'
		return 1
	}

	i := 15
	for val > 0 {
		buf[i] = lowerhex[val&0xF]
		val >>= 4
		i--
	}

	count := 15 - i
	copy(buf[:count], buf[i+1:16])
	return count
}
