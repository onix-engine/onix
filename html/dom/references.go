package dom

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func detectionRune(s string) (rune, int) {
	if s[0] < utf8.RuneSelf {
		return rune(s[0]), 1
	}
	return utf8.DecodeRuneInString(s)
}

func isDetectionSpace(r rune) bool { return unicode.IsSpace(r) }

func (p *Parser) lower(s string) string {
	return p.decode(s, false, false, true, true)
}

// decode only writes when preprocessing, entity expansion or ASCII folding is
// necessary. Writes append to document storage, so earlier views stay immutable.
func (p *Parser) decode(s string, attribute, references, replaceNUL, lower bool) string {
	first := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' && !lower || c == 0 && replaceNUL || c == '&' && references || lower && c >= 'A' && c <= 'Z' {
			first = i
			break
		}
	}
	if first < 0 {
		return s
	}
	start := len(p.decoded)
	// Reserve once for a typical transformation, especially long text. Existing
	// string views keep their old backing array alive if this grows storage.
	if cap(p.decoded)-start < len(s) {
		buffer := make([]byte, start, max(2*cap(p.decoded), start+len(s)))
		copy(buffer, p.decoded)
		p.decoded = buffer
	}
	p.decoded = append(p.decoded, s[:first]...)
	for i := first; i < len(s); {
		if !lower {
			next := strings.IndexAny(s[i:], "&\r\x00")
			if next < 0 {
				p.decoded = append(p.decoded, s[i:]...)
				break
			}
			p.decoded = append(p.decoded, s[i:i+next]...)
			i += next
		}
		c := s[i]
		i++
		switch {
		case c == '\r' && !lower:
			if i < len(s) && s[i] == '\n' {
				i++
			}
			p.decoded = append(p.decoded, '\n')
		case c == 0 && replaceNUL:
			p.decoded = append(p.decoded, "\ufffd"...)
		case c == '&' && references:
			r1, r2, size := consumeReference(s[i-1:], attribute)
			p.decoded = utf8.AppendRune(p.decoded, r1)
			if r2 != 0 {
				p.decoded = utf8.AppendRune(p.decoded, r2)
			}
			i += size - 1
		case lower && c >= 'A' && c <= 'Z':
			p.decoded = append(p.decoded, c+'a'-'A')
		default:
			p.decoded = append(p.decoded, c)
		}
	}
	return byteString(p.decoded[start:])
}

// consumeReference implements the character-reference state without temporary
// strings, including ambiguous ampersands in attributes and legacy numeric IDs.
func consumeReference(s string, attribute bool) (rune, rune, int) {
	if len(s) < 2 {
		return '&', 0, 1
	}
	if s[1] == '#' {
		i, base := 2, rune(10)
		if i < len(s) && (s[i] == 'x' || s[i] == 'X') {
			i++
			base = 16
		}
		start, value := i, rune(0)
		for i < len(s) {
			digit := rune(-1)
			if s[i] >= '0' && s[i] <= '9' {
				digit = rune(s[i]) - '0'
			}
			if base == 16 {
				if s[i] >= 'a' && s[i] <= 'f' {
					digit = rune(s[i]) - 'a' + 10
				} else if s[i] >= 'A' && s[i] <= 'F' {
					digit = rune(s[i]) - 'A' + 10
				}
			}
			if digit < 0 || digit >= base {
				break
			}
			if value <= utf8.MaxRune {
				value = value*base + digit
			}
			i++
		}
		if i == start {
			return '&', 0, 1
		}
		if i < len(s) && s[i] == ';' {
			i++
		}
		if value >= 0x80 && value <= 0x9f {
			value = numericReplacements[value-0x80]
		} else if value == 0 || value > utf8.MaxRune || value >= 0xd800 && value <= 0xdfff {
			value = utf8.RuneError
		}
		return value, 0, i
	}
	// Named references have at most 32 bytes. Never rescan a long unknown
	// alphanumeric suffix once per ampersand.
	end := 1
	for end < len(s) && end <= 32 {
		c := s[end]
		if !asciiAlphanumeric(c) {
			if c == ';' {
				end++
			}
			break
		}
		end++
	}
	for length := end - 1; length > 0; length-- {
		// Only the legacy names of six characters or fewer omit a semicolon.
		if length > 6 && s[length] != ';' {
			continue
		}
		name := s[1 : length+1]
		value := referenceValue(name)
		if value == "" {
			continue
		}
		if attribute && s[length] != ';' && length+1 < len(s) && (s[length+1] == '=' || asciiAlphanumeric(s[length+1])) {
			return '&', 0, 1
		}
		r1, size := utf8.DecodeRuneInString(value)
		var r2 rune
		if size < len(value) {
			r2, _ = utf8.DecodeRuneInString(value[size:])
		}
		return r1, r2, length + 1
	}
	return '&', 0, 1
}

func referenceValue(name string) string {
	switch name {
	case "amp;", "amp", "AMP;", "AMP":
		return "&"
	case "lt;", "lt", "LT;", "LT":
		return "<"
	case "gt;", "gt", "GT;", "GT":
		return ">"
	case "quot;", "quot", "QUOT;", "QUOT":
		return "\""
	case "apos;":
		return "'"
	case "nbsp;", "nbsp":
		return "\u00a0"
	}
	low, high := 0, len(namedReferences)
	for low < high {
		mid := int(uint(low+high) >> 1)
		if namedReferences[mid].name < name {
			low = mid + 1
		} else {
			high = mid
		}
	}
	if low < len(namedReferences) && namedReferences[low].name == name {
		return namedReferences[low].value
	}
	return ""
}

func asciiAlphanumeric(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

var numericReplacements = [...]rune{
	0x20ac, 0x81, 0x201a, 0x192, 0x201e, 0x2026, 0x2020, 0x2021,
	0x2c6, 0x2030, 0x160, 0x2039, 0x152, 0x8d, 0x17d, 0x8f,
	0x90, 0x2018, 0x2019, 0x201c, 0x201d, 0x2022, 0x2013, 0x2014,
	0x2dc, 0x2122, 0x161, 0x203a, 0x153, 0x9d, 0x17e, 0x178,
}
