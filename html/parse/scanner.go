// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.go-html.
// Adapted for Onix: source views, direct DOM construction and reusable storage.

package parse

import "strings"

type span struct{ start, end int }

type htmlScanner struct {
	input                                  string
	rawTag                                 string
	attr                                   [][2]span
	pendingAttr                            [2]span
	raw, data                              span
	textIsRaw, convertNUL, allowCDATA, eof bool
}

func (z *htmlScanner) reset(input string) { *z = htmlScanner{input: input, attr: z.attr[:0]} }
func (z *htmlScanner) NextIsNotRawText()  { z.rawTag = "" }
func (z *htmlScanner) readByte() byte {
	if z.raw.end == len(z.input) {
		z.eof = true
		return 0
	}
	c := z.input[z.raw.end]
	z.raw.end++
	return c
}

// skipWhiteSpace skips past any white space.
func (z *htmlScanner) skipWhiteSpace() {
	if z.eof {
		return
	}
	for {
		c := z.readByte()
		if z.eof {
			return
		}
		switch c {
		case ' ', '\n', '\r', '\t', '\f':
		default:
			z.raw.end--
			return
		}
	}
}

func (z *htmlScanner) readRawOrRCDATA() {
	if z.rawTag == "script" {
		z.readScript()
		z.textIsRaw = true
		z.rawTag = ""
		return
	}
	for {
		next := strings.Index(z.input[z.raw.end:], "</")
		if next < 0 {
			z.raw.end = len(z.input)
			z.eof = true
			break
		}
		z.raw.end += next + len("</")
		if z.readRawEndTag() || z.eof {
			break
		}
	}
	z.data.end = z.raw.end
	z.textIsRaw = z.rawTag != "textarea" && z.rawTag != "title"
	z.rawTag = ""
}

func (z *htmlScanner) readRawEndTag() bool {
	for i := 0; i < len(z.rawTag); i++ {
		c := z.readByte()
		if z.eof {
			return false
		}
		if c != z.rawTag[i] && c != z.rawTag[i]-('a'-'A') {
			z.raw.end--
			return false
		}
	}
	c := z.readByte()
	if z.eof {
		return false
	}
	switch c {
	case ' ', '\n', '\r', '\t', '\f', '/', '>':
		z.raw.end -= 3 + len(z.rawTag)
		return true
	}
	z.raw.end--
	return false
}

func (z *htmlScanner) readScript() {
	defer func() {
		z.data.end = z.raw.end
	}()
	var c byte

scriptData:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '<' {
		goto scriptDataLessThanSign
	}
	goto scriptData

scriptDataLessThanSign:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '/':
		goto scriptDataEndTagOpen
	case '!':
		goto scriptDataEscapeStart
	}
	z.raw.end--
	goto scriptData

scriptDataEndTagOpen:
	if z.readRawEndTag() || z.eof {
		return
	}
	goto scriptData

scriptDataEscapeStart:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '-' {
		goto scriptDataEscapeStartDash
	}
	z.raw.end--
	goto scriptData

scriptDataEscapeStartDash:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '-' {
		goto scriptDataEscapedDashDash
	}
	z.raw.end--
	goto scriptData

scriptDataEscaped:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataEscapedDash
	case '<':
		goto scriptDataEscapedLessThanSign
	}
	goto scriptDataEscaped

scriptDataEscapedDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataEscapedDashDash
	case '<':
		goto scriptDataEscapedLessThanSign
	}
	goto scriptDataEscaped

scriptDataEscapedDashDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataEscapedDashDash
	case '<':
		goto scriptDataEscapedLessThanSign
	case '>':
		goto scriptData
	}
	goto scriptDataEscaped

scriptDataEscapedLessThanSign:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '/' {
		goto scriptDataEscapedEndTagOpen
	}
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' {
		goto scriptDataDoubleEscapeStart
	}
	z.raw.end--
	goto scriptData

scriptDataEscapedEndTagOpen:
	if z.readRawEndTag() || z.eof {
		return
	}
	goto scriptDataEscaped

scriptDataDoubleEscapeStart:
	z.raw.end--
	for i := 0; i < len("script"); i++ {
		c = z.readByte()
		if z.eof {
			return
		}
		if c != "script"[i] && c != "SCRIPT"[i] {
			z.raw.end--
			goto scriptDataEscaped
		}
	}
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case ' ', '\n', '\r', '\t', '\f', '/', '>':
		goto scriptDataDoubleEscaped
	}
	z.raw.end--
	goto scriptDataEscaped

scriptDataDoubleEscaped:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataDoubleEscapedDash
	case '<':
		goto scriptDataDoubleEscapedLessThanSign
	}
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapedDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataDoubleEscapedDashDash
	case '<':
		goto scriptDataDoubleEscapedLessThanSign
	}
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapedDashDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataDoubleEscapedDashDash
	case '<':
		goto scriptDataDoubleEscapedLessThanSign
	case '>':
		goto scriptData
	}
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapedLessThanSign:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '/' {
		goto scriptDataDoubleEscapeEnd
	}
	z.raw.end--
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapeEnd:
	if z.readRawEndTag() {
		z.raw.end += len("</script>")
		goto scriptDataEscaped
	}
	if z.eof {
		return
	}
	goto scriptDataDoubleEscaped
}

func (z *htmlScanner) readComment() {

	var dashCount int
	beginning := true
	for {
		c := z.readByte()
		if z.eof {
			return
		}
		switch c {
		case '-':
			dashCount++
			continue
		case '>':
			if dashCount >= 2 || beginning {
				return
			}
		case '!':
			if dashCount >= 2 {
				c = z.readByte()
				if z.eof {
					return
				} else if c == '>' {
					return
				} else if c == '-' {
					dashCount = 1
					beginning = false
					continue
				}
			}
		}
		dashCount = 0
		beginning = false
	}
}

// readUntil consumes a delimiter and returns the preceding source span.
func (z *htmlScanner) readUntil(delimiter byte) span {
	start := z.raw.end
	if offset := strings.IndexByte(z.input[start:], delimiter); offset >= 0 {
		end := start + offset
		z.raw.end = end + 1
		return span{start, end}
	}
	z.raw.end = len(z.input)
	z.eof = true
	return span{start, z.raw.end}
}

func (z *htmlScanner) readMarkupDeclaration() TokenType {
	rest := z.input[z.raw.end:]
	switch {
	case strings.HasPrefix(rest, "--"):
		z.raw.end += len("--")
		z.readComment()
	case len(rest) >= len("DOCTYPE") && strings.EqualFold(rest[:len("DOCTYPE")], "doctype"):
		z.raw.end += len("DOCTYPE")
		z.skipWhiteSpace()
		z.data = z.readUntil('>')
		return tokenDoctype
	case z.allowCDATA && strings.HasPrefix(rest, "[CDATA["):
		z.raw.end += len("[CDATA[")
		z.data.start = z.raw.end
		if offset := strings.Index(z.input[z.raw.end:], "]]>"); offset >= 0 {
			z.data.end = z.raw.end + offset
			z.raw.end = z.data.end + len("]]>")
		} else {
			z.raw.end = len(z.input)
			z.data.end = z.raw.end
			z.eof = true
		}
		z.convertNUL = true
		return TokenText
	default:
		z.readUntil('>')
	}
	return tokenComment
}

func (z *htmlScanner) matchingTag(names ...string) string {
loop:
	for _, s := range names {
		if z.data.end-z.data.start != len(s) {
			continue loop
		}
		for i := 0; i < len(s); i++ {
			c := z.input[z.data.start+i]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != s[i] {
				continue loop
			}
		}
		return s
	}
	return ""
}

func (z *htmlScanner) readStartTag() TokenType {
	z.readTag(true)
	if z.eof {
		return TokenEOF
	}
	c := z.input[z.data.start]
	if 'A' <= c && c <= 'Z' {
		c += 'a' - 'A'
	}
	switch c {
	case 'i':
		z.rawTag = z.matchingTag("iframe")
	case 'n':
		z.rawTag = z.matchingTag("noembed", "noframes", "noscript")
	case 'p':
		z.rawTag = z.matchingTag("plaintext")
	case 's':
		z.rawTag = z.matchingTag("script", "style")
	case 't':
		z.rawTag = z.matchingTag("textarea", "title")
	case 'x':
		z.rawTag = z.matchingTag("xmp")
	}
	nAttrs := len(z.attr)
	if z.input[z.raw.end-2] == '/' && (nAttrs == 0 || z.raw.end-2 != z.attr[nAttrs-1][1].end-1) {
		return TokenSelfClosingTag
	}
	return TokenStartTag
}

func (z *htmlScanner) readTag(saveAttr bool) {
	z.attr = z.attr[:0]
	z.readTagName()
	if z.skipWhiteSpace(); z.eof {
		return
	}
	for {
		c := z.readByte()
		if z.eof || c == '>' {
			break
		}
		z.raw.end--
		z.readTagAttrKey()
		z.readTagAttrVal()
		if saveAttr && z.pendingAttr[0].start != z.pendingAttr[0].end {
			z.attr = append(z.attr, z.pendingAttr)
		}
		if z.skipWhiteSpace(); z.eof {
			break
		}
	}
}

func (z *htmlScanner) readTagName() {
	z.data.start = z.raw.end - 1
	for {
		c := z.readByte()
		if z.eof {
			z.data.end = z.raw.end
			return
		}
		switch c {
		case ' ', '\n', '\r', '\t', '\f':
			z.data.end = z.raw.end - 1
			return
		case '/', '>':
			z.raw.end--
			z.data.end = z.raw.end
			return
		}
	}
}

func (z *htmlScanner) readTagAttrKey() {
	z.pendingAttr[0].start = z.raw.end
	for {
		c := z.readByte()
		if z.eof {
			z.pendingAttr[0].end = z.raw.end
			return
		}
		switch c {
		case '=':
			if z.pendingAttr[0].start+1 == z.raw.end {
				continue
			}
			fallthrough
		case ' ', '\n', '\r', '\t', '\f', '/', '>':
			z.raw.end--
			z.pendingAttr[0].end = z.raw.end
			return
		}
	}
}

func (z *htmlScanner) readTagAttrVal() {
	z.pendingAttr[1].start = z.raw.end
	z.pendingAttr[1].end = z.raw.end
	if z.skipWhiteSpace(); z.eof {
		return
	}
	c := z.readByte()
	if z.eof {
		return
	}
	if c == '/' {
		return
	}
	if c != '=' {
		z.raw.end--
		return
	}
	if z.skipWhiteSpace(); z.eof {
		return
	}
	quote := z.readByte()
	if z.eof {
		return
	}
	switch quote {
	case '>':
		z.raw.end--
		return

	case '\'', '"':
		z.pendingAttr[1] = z.readUntil(quote)

	default:
		z.pendingAttr[1].start = z.raw.end - 1
		for {
			c := z.readByte()
			if z.eof {
				z.pendingAttr[1].end = z.raw.end
				return
			}
			switch c {
			case ' ', '\n', '\r', '\t', '\f':
				z.pendingAttr[1].end = z.raw.end - 1
				return
			case '>':
				z.raw.end--
				z.pendingAttr[1].end = z.raw.end
				return
			}
		}
	}
}

func (z *htmlScanner) scan() TokenType {
	z.raw.start = z.raw.end
	z.data.start = z.raw.end
	z.data.end = z.raw.end
	if z.eof {
		return TokenEOF
	}
	if z.rawTag != "" {
		if z.rawTag == "plaintext" {
			z.raw.end = len(z.input)
			z.eof = true
			z.data.end = z.raw.end
			z.textIsRaw = true
		} else {
			z.readRawOrRCDATA()
		}
		if z.data.end > z.data.start {
			z.convertNUL = true
			return TokenText
		}
	}
	z.textIsRaw = false
	z.convertNUL = false

loop:
	for {
		next := strings.IndexByte(z.input[z.raw.end:], '<')
		if next < 0 {
			z.raw.end = len(z.input)
			z.eof = true
			break loop
		}
		z.raw.end += next + 1
		c := z.readByte()
		if z.eof {
			break loop
		}

		var tokenType TokenType
		switch {
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
			tokenType = TokenStartTag
		case c == '/':
			tokenType = TokenEndTag
		case c == '!' || c == '?':
			tokenType = tokenComment
		default:
			z.raw.end--
			continue
		}

		if x := z.raw.end - len("<a"); z.raw.start < x {
			z.raw.end = x
			z.data.end = x
			return TokenText
		}
		switch tokenType {
		case TokenStartTag:
			return z.readStartTag()
		case TokenEndTag:
			c = z.readByte()
			if z.eof {
				break loop
			}
			if c == '>' {
				return tokenComment
			}
			if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' {
				z.readTag(false)
				if z.eof {
					return TokenEOF
				}
				return TokenEndTag
			}
			z.raw.end--
			z.readUntil('>')
			return tokenComment
		case tokenComment:
			if c == '!' {
				return z.readMarkupDeclaration()
			}
			z.raw.end--
			z.readUntil('>')
			return tokenComment
		}
	}
	if z.raw.start < z.raw.end {
		z.data.end = z.raw.end
		return TokenText
	}
	return TokenEOF
}
