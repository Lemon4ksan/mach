package auth

import "strings"

func ExtractChallengeParams(header, scheme string) (map[string]string, bool) {
	header = strings.TrimSpace(header)
	schemeLower := strings.ToLower(scheme)

	idx := strings.Index(strings.ToLower(header), schemeLower)
	if idx < 0 {
		return nil, false
	}

	if idx > 0 {
		prev := header[idx-1]
		if prev != ' ' && prev != ',' && prev != '\t' {
			return nil, false
		}
	}

	rest := header[idx+len(scheme):]
	if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' && rest[0] != ',' && rest[0] != '=' {
		return nil, false
	}
	rest = strings.TrimSpace(rest)

	m := make(map[string]string)

	for len(rest) > 0 {
		rest = strings.TrimLeft(rest, " ,\t")
		if len(rest) == 0 {
			break
		}

		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx < 0 {
			break
		}

		spaceIdx := strings.IndexByte(rest, ' ')
		if spaceIdx >= 0 && spaceIdx < eqIdx {
			break
		}

		key := strings.ToLower(strings.TrimSpace(rest[:eqIdx]))
		rest = strings.TrimSpace(rest[eqIdx+1:])

		var val string
		if len(rest) > 0 && rest[0] == '"' {
			rest = rest[1:]
			var sb strings.Builder
			escaped := false
			foundQuote := false
			for i, c := range rest {
				if escaped {
					sb.WriteRune(c)
					escaped = false
					continue
				}
				if c == '\\' {
					escaped = true
					continue
				}
				if c == '"' {
					rest = rest[i+1:]
					foundQuote = true
					break
				}
				sb.WriteRune(c)
			}
			if !foundQuote {
				rest = ""
			}
			val = sb.String()
		} else {
			commaIdx := strings.IndexByte(rest, ',')
			spaceIdx2 := strings.IndexByte(rest, ' ')
			endIdx := commaIdx
			if spaceIdx2 >= 0 && (endIdx < 0 || spaceIdx2 < endIdx) {
				endIdx = spaceIdx2
			}

			if endIdx < 0 {
				val = strings.TrimSpace(rest)
				rest = ""
			} else {
				val = strings.TrimSpace(rest[:endIdx])
				rest = rest[endIdx:]
			}
		}

		m[key] = val
	}

	return m, true
}
