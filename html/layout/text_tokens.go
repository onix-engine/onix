package layout

import (
	"iter"
	"unicode"
	"unicode/utf8"
)

type textToken struct {
	Text    string
	IsSpace bool
	IsBreak bool
	IsTab   bool
}

// textTokens yields views of the source as layout consumes them. A long
// paragraph needs no token array, and single-line controls can stop early.
func textTokens(input, mode string, suppressLeadingSpace, breakAll bool) iter.Seq[textToken] {
	return func(yield func(textToken) bool) {
		switch mode {
		case "pre", "pre-wrap":
			yieldPreformattedText(input, yield)
		default:
			yieldNormalText(input, suppressLeadingSpace, breakAll, mode == "pre-line", yield)
		}
	}
}

func yieldPreformattedText(input string, yield func(textToken) bool) bool {
	start := -1
	for offset, r := range input {
		if r != '\r' && r != '\n' && r != '\t' && r != ' ' {
			if start < 0 {
				start = offset
			}
			continue
		}
		if start >= 0 {
			if !yield(textToken{Text: input[start:offset]}) {
				return false
			}
			start = -1
		}
		var token textToken
		switch r {
		case '\r':
			continue
		case '\n':
			token.IsBreak = true
		case '\t':
			token.IsTab = true
		case ' ':
			token.Text, token.IsSpace = " ", true
		}
		if !yield(token) {
			return false
		}
	}
	return start < 0 || yield(textToken{Text: input[start:]})
}

func yieldNormalText(input string, suppressLeadingSpace, breakAll, preserveBreaks bool, yield func(textToken) bool) bool {
	spacePending := suppressLeadingSpace
	emitted := false
	start := -1
	flushWord := func(end int) bool {
		if start < 0 {
			return true
		}
		if !yield(textToken{Text: input[start:end]}) {
			return false
		}
		start, emitted = -1, true
		return true
	}
	for offset := 0; offset < len(input); {
		r, size := utf8.DecodeRuneInString(input[offset:])
		next := offset + size
		if preserveBreaks && (r == '\n' || r == '\r' && next < len(input) && input[next] == '\n') {
			if !flushWord(offset) || !yield(textToken{IsBreak: true}) {
				return false
			}
			spacePending, emitted = true, false
			offset = next
			if r == '\r' {
				offset++
			}
			continue
		}
		switch {
		case unicode.IsSpace(r):
			if !flushWord(offset) {
				return false
			}
			spacePending = true
		case breakAll || isCJK(r):
			if !flushWord(offset) {
				return false
			}
			if spacePending && emitted && !yield(textToken{Text: " ", IsSpace: true}) {
				return false
			}
			spacePending = false
			if !yield(textToken{Text: input[offset:next]}) {
				return false
			}
			emitted = true
		default:
			if spacePending {
				if !flushWord(offset) || emitted && !yield(textToken{Text: " ", IsSpace: true}) {
					return false
				}
				spacePending = false
			}
			if start < 0 {
				start = offset
			}
		}
		offset = next
	}
	return flushWord(len(input))
}
