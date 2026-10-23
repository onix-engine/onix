package dom

import (
	"fmt"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

type TokenType int

const (
	TokenError TokenType = iota
	TokenEOF
	TokenText
	TokenStartTag
	TokenEndTag
	TokenSelfClosingTag
)

type Token struct {
	Type       TokenType
	Data       string
	Attributes map[string]string
}

type Tokenizer struct {
	input string
	pos   int
}

func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{input: input}
}

func (t *Tokenizer) Next() (Token, error) {
	for {
		if t.eof() {
			return Token{Type: TokenEOF}, nil
		}

		if t.peek() != '<' {
			return Token{Type: TokenText, Data: t.readText()}, nil
		}

		if strings.HasPrefix(t.input[t.pos:], "<!--") {
			if err := t.skipComment(); err != nil {
				return Token{}, err
			}
			continue
		}

		if strings.HasPrefix(t.input[t.pos:], "<!") || strings.HasPrefix(t.input[t.pos:], "<?") {
			if err := t.skipDeclaration(); err != nil {
				return Token{}, err
			}
			continue
		}

		return t.readTag()
	}
}

func (t *Tokenizer) readText() string {
	start := t.pos
	for !t.eof() && t.peek() != '<' {
		_, size := utf8.DecodeRuneInString(t.input[t.pos:])
		t.pos += size
	}
	return unescapeHTMLReferences(t.input[start:t.pos])
}

func (t *Tokenizer) readTag() (Token, error) {
	t.pos++

	if t.eof() {
		return Token{}, fmt.Errorf("unexpected end of input after '<'")
	}

	if t.peek() == '/' {
		return t.readEndTag()
	}

	name, err := t.readTagName()
	if err != nil {
		return Token{}, err
	}

	attrs, selfClosing, err := t.readAttributes()
	if err != nil {
		return Token{}, err
	}

	tokenType := TokenStartTag
	if selfClosing {
		tokenType = TokenSelfClosingTag
	}

	return Token{Type: tokenType, Data: name, Attributes: attrs}, nil
}

func (t *Tokenizer) readEndTag() (Token, error) {
	t.pos++
	name, err := t.readTagName()
	if err != nil {
		return Token{}, err
	}

	t.skipWhitespace()
	if t.eof() || t.peek() != '>' {
		return Token{}, fmt.Errorf("expected '>' after end tag %q", name)
	}
	t.pos++

	return Token{Type: TokenEndTag, Data: name}, nil
}

func (t *Tokenizer) readAttributes() (map[string]string, bool, error) {
	var attrs map[string]string

	for {
		t.skipWhitespace()

		if t.eof() {
			return nil, false, fmt.Errorf("unexpected end of input inside tag")
		}

		switch t.peek() {
		case '>':
			t.pos++
			return attrs, false, nil
		case '/':
			t.pos++
			if t.eof() || t.peek() != '>' {
				return nil, false, fmt.Errorf("expected '/>' to close tag")
			}
			t.pos++
			return attrs, true, nil
		}

		name, value, err := t.readAttribute()
		if err != nil {
			return nil, false, err
		}
		if attrs == nil {
			attrs = make(map[string]string, 1)
		}
		attrs[name] = value
	}
}

func (t *Tokenizer) readAttribute() (string, string, error) {
	name, err := t.readName()
	if err != nil {
		return "", "", err
	}

	t.skipWhitespace()
	if t.eof() || t.peek() != '=' {
		return strings.ToLower(name), "", nil
	}
	t.pos++
	t.skipWhitespace()

	if t.eof() {
		return "", "", fmt.Errorf("missing value for attribute %q", name)
	}

	if quote := t.peek(); quote == '"' || quote == '\'' {
		t.pos++
		start := t.pos
		for !t.eof() && t.peek() != quote {
			_, size := utf8.DecodeRuneInString(t.input[t.pos:])
			t.pos += size
		}
		if t.eof() {
			return "", "", fmt.Errorf("unterminated quoted attribute %q", name)
		}
		value := unescapeHTMLReferences(t.input[start:t.pos])
		t.pos++
		return strings.ToLower(name), value, nil
	}

	start := t.pos
	for !t.eof() {
		ch := t.peek()
		if isWhitespace(rune(ch)) || ch == '>' || (ch == '/' && t.pos+1 < len(t.input) && t.input[t.pos+1] == '>') {
			break
		}
		t.pos++
	}

	return strings.ToLower(name), unescapeHTMLReferences(t.input[start:t.pos]), nil
}

func unescapeHTMLReferences(value string) string {
	if !strings.Contains(value, "&") {
		return value
	}
	return html.UnescapeString(value)
}

func (t *Tokenizer) readTagName() (string, error) {
	name, err := t.readName()
	if err != nil {
		return "", err
	}
	return strings.ToLower(name), nil
}

func (t *Tokenizer) readName() (string, error) {
	if t.eof() {
		return "", fmt.Errorf("unexpected end of input while reading name")
	}

	start := t.pos
	for !t.eof() {
		r, size := utf8.DecodeRuneInString(t.input[t.pos:])
		if !isNameRune(r) {
			break
		}
		t.pos += size
	}

	if start == t.pos {
		return "", fmt.Errorf("expected name at position %d", t.pos)
	}

	return t.input[start:t.pos], nil
}

func (t *Tokenizer) skipComment() error {
	end := strings.Index(t.input[t.pos+4:], "-->")
	if end < 0 {
		return fmt.Errorf("unterminated comment")
	}
	t.pos += 4 + end + 3
	return nil
}

func (t *Tokenizer) skipDeclaration() error {
	end := strings.IndexByte(t.input[t.pos:], '>')
	if end < 0 {
		return fmt.Errorf("unterminated declaration")
	}
	t.pos += end + 1
	return nil
}

func (t *Tokenizer) skipWhitespace() {
	for !t.eof() {
		r, size := utf8.DecodeRuneInString(t.input[t.pos:])
		if !isWhitespace(r) {
			return
		}
		t.pos += size
	}
}

func (t *Tokenizer) eof() bool {
	return t.pos >= len(t.input)
}

func (t *Tokenizer) peek() byte {
	return t.input[t.pos]
}

func isWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}

func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':'
}
