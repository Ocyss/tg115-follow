package utils

// UTF16Slice returns s[offset:offset+length] where offset and length are
// measured in UTF-16 code units, which is how Telegram message entities are
// indexed, while Go strings are UTF-8. Runes outside the BMP count as two
// units. Returns "" when the range is out of bounds.
func UTF16Slice(s string, offset, length int) string {
	runes := []rune(s)
	start, end, ok := UTF16Bounds(runes, offset, length)
	if !ok {
		return ""
	}
	return string(runes[start:end])
}

// UTF16Bounds converts a UTF-16 code-unit offset/length into rune indices
// [start, end) over runes. ok is false when out of bounds.
func UTF16Bounds(runes []rune, offset, length int) (start, end int, ok bool) {
	if offset < 0 || length < 0 {
		return 0, 0, false
	}
	start, end = -1, -1
	units := 0
	for i, r := range runes {
		if units == offset && start < 0 {
			start = i
		}
		if units == offset+length {
			end = i
			break
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
	}
	if start < 0 {
		if units == offset {
			start = len(runes)
		} else {
			return 0, 0, false
		}
	}
	if end < 0 {
		if units == offset+length {
			end = len(runes)
		} else {
			return 0, 0, false
		}
	}
	return start, end, true
}
