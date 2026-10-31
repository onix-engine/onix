// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.go-html.
// Adapted for Onix: source views, direct DOM construction and reusable storage.

package dom

import (
	"io"
	"strings"
)

type span struct{ start, end int }

type htmlScanner struct {
	input                             string
	tt                                TokenType
	err                               error
	raw, data                         span
	pendingAttr                       [2]span
	attr                              [][2]span
	rawTag                            string
	textIsRaw, convertNUL, allowCDATA bool
}

func (z *htmlScanner) reset(input string)    { *z = htmlScanner{input: input, attr: z.attr[:0]} }
func (z *htmlScanner) AllowCDATA(allow bool) { z.allowCDATA = allow }
func (z *htmlScanner) NextIsNotRawText()     { z.rawTag = "" }
func (z *htmlScanner) readByte() byte {
	if z.raw.end == len(z.input) {
		z.err = io.EOF
		return 0
	}
	c := z.input[z.raw.end]
	z.raw.end++
	return c
}
func (z *htmlScanner) rawTagName() string {
	for _, name := range [...]string{"iframe", "noembed", "noframes", "noscript", "plaintext", "script", "style", "textarea", "title", "xmp"} {
		if z.startTagIn(name) {
			return name
		}
	}
	return ""
}

// skipWhiteSpace skips past any white space.
func (z *htmlScanner) skipWhiteSpace() {
	if z.err != nil {
		return
	}
	for {
		c := z.readByte()
		if z.err != nil {
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
loop:
	for {
		c := z.readByte()
		if z.err != nil {
			break loop
		}
		if c != '<' {
			continue loop
		}
		c = z.readByte()
		if z.err != nil {
			break loop
		}
		if c != '/' {
			z.raw.end--
			continue loop
		}
		if z.readRawEndTag() || z.err != nil {
			break loop
		}
	}
	z.data.end = z.raw.end
	z.textIsRaw = z.rawTag != "textarea" && z.rawTag != "title"
	z.rawTag = ""
}

func (z *htmlScanner) readRawEndTag() bool {
	for i := 0; i < len(z.rawTag); i++ {
		c := z.readByte()
		if z.err != nil {
			return false
		}
		if c != z.rawTag[i] && c != z.rawTag[i]-('a'-'A') {
			z.raw.end--
			return false
		}
	}
	c := z.readByte()
	if z.err != nil {
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
	if z.err != nil {
		return
	}
	if c == '<' {
		goto scriptDataLessThanSign
	}
	goto scriptData

scriptDataLessThanSign:
	c = z.readByte()
	if z.err != nil {
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
	if z.readRawEndTag() || z.err != nil {
		return
	}
	goto scriptData

scriptDataEscapeStart:
	c = z.readByte()
	if z.err != nil {
		return
	}
	if c == '-' {
		goto scriptDataEscapeStartDash
	}
	z.raw.end--
	goto scriptData

scriptDataEscapeStartDash:
	c = z.readByte()
	if z.err != nil {
		return
	}
	if c == '-' {
		goto scriptDataEscapedDashDash
	}
	z.raw.end--
	goto scriptData

scriptDataEscaped:
	c = z.readByte()
	if z.err != nil {
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
	if z.err != nil {
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
	if z.err != nil {
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
	if z.err != nil {
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
	if z.readRawEndTag() || z.err != nil {
		return
	}
	goto scriptDataEscaped

scriptDataDoubleEscapeStart:
	z.raw.end--
	for i := 0; i < len("script"); i++ {
		c = z.readByte()
		if z.err != nil {
			return
		}
		if c != "script"[i] && c != "SCRIPT"[i] {
			z.raw.end--
			goto scriptDataEscaped
		}
	}
	c = z.readByte()
	if z.err != nil {
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
	if z.err != nil {
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
	if z.err != nil {
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
	if z.err != nil {
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
	if z.err != nil {
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
	if z.err != nil {
		return
	}
	goto scriptDataDoubleEscaped
}

func (z *htmlScanner) readComment() {

	z.data.start = z.raw.end
	defer func() {
		if z.data.end < z.data.start {
			z.data.end = z.data.start
		}
	}()

	var dashCount int
	beginning := true
	for {
		c := z.readByte()
		if z.err != nil {
			z.data.end = z.calculateAbruptCommentDataEnd()
			return
		}
		switch c {
		case '-':
			dashCount++
			continue
		case '>':
			if dashCount >= 2 || beginning {
				z.data.end = z.raw.end - len("-->")
				return
			}
		case '!':
			if dashCount >= 2 {
				c = z.readByte()
				if z.err != nil {
					z.data.end = z.calculateAbruptCommentDataEnd()
					return
				} else if c == '>' {
					z.data.end = z.raw.end - len("--!>")
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

func (z *htmlScanner) calculateAbruptCommentDataEnd() int {
	raw := z.input[z.raw.start:z.raw.end]
	const prefixLen = len("<!--")
	if len(raw) >= prefixLen {
		raw = raw[prefixLen:]
		if strings.HasSuffix(raw, "--!") {
			return z.raw.end - 3
		} else if strings.HasSuffix(raw, "--") {
			return z.raw.end - 2
		} else if strings.HasSuffix(raw, "-") {
			return z.raw.end - 1
		}
	}
	return z.raw.end
}

func (z *htmlScanner) readUntilCloseAngle() {
	z.data.start = z.raw.end
	for {
		c := z.readByte()
		if z.err != nil {
			z.data.end = z.raw.end
			return
		}
		if c == '>' {
			z.data.end = z.raw.end - len(">")
			return
		}
	}
}

func (z *htmlScanner) readMarkupDeclaration() TokenType {
	z.data.start = z.raw.end
	var c [2]byte
	for i := 0; i < 2; i++ {
		c[i] = z.readByte()
		if z.err != nil {
			z.data.end = z.raw.end
			if i == 1 && c[0] == '>' {
				z.data.end--
			}
			return tokenComment
		}
	}
	if c[0] == '-' && c[1] == '-' {
		z.readComment()
		return tokenComment
	}
	z.raw.end -= 2
	if z.readDoctype() {
		return tokenDoctype
	}
	if z.allowCDATA && z.readCDATA() {
		z.convertNUL = true
		return TokenText
	}
	z.readUntilCloseAngle()
	return tokenComment
}

func (z *htmlScanner) readDoctype() bool {
	const s = "DOCTYPE"
	for i := 0; i < len(s); i++ {
		c := z.readByte()
		if z.err != nil {
			if z.err == io.EOF {
				z.raw.end = z.data.start
				z.err = nil
				return false
			}
			z.data.end = z.raw.end
			return false
		}
		if c != s[i] && c != s[i]+('a'-'A') {
			z.raw.end = z.data.start
			return false
		}
	}
	if z.skipWhiteSpace(); z.err != nil {
		z.data.start = z.raw.end
		z.data.end = z.raw.end
		return true
	}
	z.readUntilCloseAngle()
	return true
}

func (z *htmlScanner) readCDATA() bool {
	const s = "[CDATA["
	for i := 0; i < len(s); i++ {
		c := z.readByte()
		if z.err != nil {
			if z.err == io.EOF {
				z.raw.end = z.data.start
				z.err = nil
				return false
			}
			z.data.end = z.raw.end
			return false
		}
		if c != s[i] {
			z.raw.end = z.data.start
			return false
		}
	}
	z.data.start = z.raw.end
	brackets := 0
	for {
		c := z.readByte()
		if z.err != nil {
			z.data.end = z.raw.end
			return true
		}
		switch c {
		case ']':
			brackets++
		case '>':
			if brackets >= 2 {
				z.data.end = z.raw.end - len("]]>")
				return true
			}
			brackets = 0
		default:
			brackets = 0
		}
	}
}

func (z *htmlScanner) startTagIn(ss ...string) bool {
loop:
	for _, s := range ss {
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
		return true
	}
	return false
}

func (z *htmlScanner) readStartTag() TokenType {
	z.readTag(true)
	if z.err != nil {
		return TokenEOF
	}
	c, raw := z.input[z.data.start], false
	if 'A' <= c && c <= 'Z' {
		c += 'a' - 'A'
	}
	switch c {
	case 'i':
		raw = z.startTagIn("iframe")
	case 'n':
		raw = z.startTagIn("noembed", "noframes", "noscript")
	case 'p':
		raw = z.startTagIn("plaintext")
	case 's':
		raw = z.startTagIn("script", "style")
	case 't':
		raw = z.startTagIn("textarea", "title")
	case 'x':
		raw = z.startTagIn("xmp")
	}
	if raw {
		z.rawTag = z.rawTagName()
	}
	nAttrs := len(z.attr)
	if z.err == nil && z.input[z.raw.end-2] == '/' && (nAttrs == 0 || z.raw.end-2 != z.attr[nAttrs-1][1].end-1) {
		return TokenSelfClosingTag
	}
	return TokenStartTag
}

func (z *htmlScanner) readTag(saveAttr bool) {
	z.attr = z.attr[:0]
	z.readTagName()
	if z.skipWhiteSpace(); z.err != nil {
		return
	}
	for {
		c := z.readByte()
		if z.err != nil || c == '>' {
			break
		}
		z.raw.end--
		z.readTagAttrKey()
		z.readTagAttrVal()
		if saveAttr && z.pendingAttr[0].start != z.pendingAttr[0].end {
			z.attr = append(z.attr, z.pendingAttr)
		}
		if z.skipWhiteSpace(); z.err != nil {
			break
		}
	}
}

func (z *htmlScanner) readTagName() {
	z.data.start = z.raw.end - 1
	for {
		c := z.readByte()
		if z.err != nil {
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
		if z.err != nil {
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
	if z.skipWhiteSpace(); z.err != nil {
		return
	}
	c := z.readByte()
	if z.err != nil {
		return
	}
	if c == '/' {
		return
	}
	if c != '=' {
		z.raw.end--
		return
	}
	if z.skipWhiteSpace(); z.err != nil {
		return
	}
	quote := z.readByte()
	if z.err != nil {
		return
	}
	switch quote {
	case '>':
		z.raw.end--
		return

	case '\'', '"':
		z.pendingAttr[1].start = z.raw.end
		for {
			c := z.readByte()
			if z.err != nil {
				z.pendingAttr[1].end = z.raw.end
				return
			}
			if c == quote {
				z.pendingAttr[1].end = z.raw.end - 1
				return
			}
		}

	default:
		z.pendingAttr[1].start = z.raw.end - 1
		for {
			c := z.readByte()
			if z.err != nil {
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
	if z.err != nil {
		z.tt = TokenEOF
		return z.tt
	}
	if z.rawTag != "" {
		if z.rawTag == "plaintext" {
			z.raw.end = len(z.input)
			z.err = io.EOF
			z.data.end = z.raw.end
			z.textIsRaw = true
		} else {
			z.readRawOrRCDATA()
		}
		if z.data.end > z.data.start {
			z.tt = TokenText
			z.convertNUL = true
			return z.tt
		}
	}
	z.textIsRaw = false
	z.convertNUL = false

loop:
	for {
		next := strings.IndexByte(z.input[z.raw.end:], '<')
		if next < 0 {
			z.raw.end = len(z.input)
			z.err = io.EOF
			break loop
		}
		z.raw.end += next + 1
		c := z.readByte()
		if z.err != nil {
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
			z.tt = TokenText
			return z.tt
		}
		switch tokenType {
		case TokenStartTag:
			z.tt = z.readStartTag()
			return z.tt
		case TokenEndTag:
			c = z.readByte()
			if z.err != nil {
				break loop
			}
			if c == '>' {
				z.tt = tokenComment
				return z.tt
			}
			if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' {
				z.readTag(false)
				if z.err != nil {
					z.tt = TokenEOF
				} else {
					z.tt = TokenEndTag
				}
				return z.tt
			}
			z.raw.end--
			z.readUntilCloseAngle()
			z.tt = tokenComment
			return z.tt
		case tokenComment:
			if c == '!' {
				z.tt = z.readMarkupDeclaration()
				return z.tt
			}
			z.raw.end--
			z.readUntilCloseAngle()
			z.tt = tokenComment
			return z.tt
		}
	}
	if z.raw.start < z.raw.end {
		z.data.end = z.raw.end
		z.tt = TokenText
		return z.tt
	}
	z.tt = TokenEOF
	return z.tt
}
