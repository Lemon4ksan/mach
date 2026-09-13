package bytesutil

import "bytes"

func DecodeArgAppend(dst, src []byte) []byte {
	idxPercent := bytes.IndexByte(src, '%')
	idxPlus := bytes.IndexByte(src, '+')
	if idxPercent == -1 && idxPlus == -1 {
		return append(dst, src...)
	}

	var idx int
	switch {
	case idxPercent == -1:
		idx = idxPlus
	case idxPlus == -1:
		idx = idxPercent
	case idxPercent > idxPlus:
		idx = idxPlus
	default:
		idx = idxPercent
	}

	dst = append(dst, src[:idx]...)

	for i := uint(idx); i < uint(len(src)); i++ {
		c := src[i]
		switch c {
		case '%':
			end := i + 3
			if end > uint(len(src)) {
				return append(dst, src[i:]...)
			}
			chunk := src[i:end]
			x2 := Hex2intTable[chunk[2]]
			x1 := Hex2intTable[chunk[1]]
			if x1 == 16 || x2 == 16 {
				dst = append(dst, '%')
			} else {
				dst = append(dst, x1<<4|x2)
				i += 2
			}
		case '+':
			dst = append(dst, ' ')
		default:
			dst = append(dst, c)
		}
	}
	return dst
}

func DecodeArgAppendNoPlus(dst, src []byte) []byte {
	idx := bytes.IndexByte(src, '%')
	if idx < 0 {
		return append(dst, src...)
	}
	dst = append(dst, src[:idx]...)
	for i := uint(idx); i < uint(len(src)); i++ {
		c := src[i]
		if c == '%' {
			end := i + 3
			if end > uint(len(src)) {
				return append(dst, src[i:]...)
			}
			chunk := src[i:end]
			x2 := Hex2intTable[chunk[2]]
			x1 := Hex2intTable[chunk[1]]
			if x1 == 16 || x2 == 16 {
				dst = append(dst, '%')
			} else {
				dst = append(dst, x1<<4|x2)
				i += 2
			}
		} else {
			dst = append(dst, c)
		}
	}
	return dst
}
