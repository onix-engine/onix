package css

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenWhitespace
	TokenIdent
	TokenHash
	TokenDot
	TokenLBrace
	TokenRBrace
	TokenColon
	TokenSemicolon
	TokenComma
	TokenAtKeyword
	TokenLBracket
	TokenRBracket
	TokenString
	TokenDelim
)

type Token struct {
	Type  TokenType
	Value string
}

type Tokenizer struct {
	input   string
	pos     int
	peek    Token
	hasPeek bool
}

func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{input: input}
}

func (t *Tokenizer) Next() (Token, error) {
	if t.hasPeek {
		token := t.peek
		t.hasPeek = false
		return token, nil
	}
	return t.scan()
}

func (t *Tokenizer) Peek() (Token, error) {
	if t.hasPeek {
		return t.peek, nil
	}
	token, err := t.scan()
	if err != nil {
		return Token{}, err
	}
	t.peek = token
	t.hasPeek = true
	return token, nil
}

func (t *Tokenizer) ReadDeclarationValue() string {
	start := t.pos
	var quote byte
	escaped := false
	comment := false
	parenDepth := 0
	bracketDepth := 0
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if comment {
			if ch == '*' && t.pos+1 < len(t.input) && t.input[t.pos+1] == '/' {
				t.pos += 2
				comment = false
				continue
			}
			t.pos++
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				t.pos++
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case quote:
				quote = 0
			}
			t.pos++
			continue
		}
		if ch == '/' && t.pos+1 < len(t.input) && t.input[t.pos+1] == '*' {
			comment = true
			t.pos += 2
			continue
		}
		if escaped {
			escaped = false
			t.pos++
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ';', '}':
			if parenDepth == 0 && bracketDepth == 0 {
				return strings.TrimSpace(t.input[start:t.pos])
			}
		}
		t.pos++
	}
	return strings.TrimSpace(t.input[start:t.pos])
}

func (t *Tokenizer) scan() (Token, error) {
	for {
		if t.pos >= len(t.input) {
			return Token{Type: TokenEOF}, nil
		}

		if strings.HasPrefix(t.input[t.pos:], "/*") {
			if err := t.skipComment(); err != nil {
				return Token{}, err
			}
			continue
		}

		if r, size := utf8.DecodeRuneInString(t.input[t.pos:]); isWhitespace(r) {
			start := t.pos
			t.pos += size
			for t.pos < len(t.input) {
				r, size = utf8.DecodeRuneInString(t.input[t.pos:])
				if !isWhitespace(r) {
					break
				}
				t.pos += size
			}
			return Token{Type: TokenWhitespace, Value: t.input[start:t.pos]}, nil
		}

		switch t.input[t.pos] {
		case '{':
			t.pos++
			return Token{Type: TokenLBrace, Value: "{"}, nil
		case '}':
			t.pos++
			return Token{Type: TokenRBrace, Value: "}"}, nil
		case ':':
			t.pos++
			return Token{Type: TokenColon, Value: ":"}, nil
		case ';':
			t.pos++
			return Token{Type: TokenSemicolon, Value: ";"}, nil
		case ',':
			t.pos++
			return Token{Type: TokenComma, Value: ","}, nil
		case '[':
			t.pos++
			return Token{Type: TokenLBracket, Value: "["}, nil
		case ']':
			t.pos++
			return Token{Type: TokenRBracket, Value: "]"}, nil
		case '\'', '"':
			return t.scanString()
		case '.':
			t.pos++
			return Token{Type: TokenDot, Value: "."}, nil
		case '#':
			return t.scanHash()
		case '@':
			return t.scanAtKeyword()
		}

		if isIdentStart(t.input[t.pos:]) {
			return t.scanIdent(), nil
		}

		token := Token{Type: TokenDelim, Value: string(t.input[t.pos])}
		t.pos++
		return token, nil
	}
}

func (t *Tokenizer) scanString() (Token, error) {
	quote := t.input[t.pos]
	t.pos++
	start := t.pos
	var builder strings.Builder
	for t.pos < len(t.input) {
		ch := t.input[t.pos]
		if ch == quote {
			if builder.Len() == 0 {
				value := t.input[start:t.pos]
				t.pos++
				return Token{Type: TokenString, Value: value}, nil
			}
			builder.WriteString(t.input[start:t.pos])
			t.pos++
			return Token{Type: TokenString, Value: builder.String()}, nil
		}
		if ch == '\\' {
			builder.WriteString(t.input[start:t.pos])
			t.pos++
			if t.pos >= len(t.input) {
				return Token{}, fmt.Errorf("unterminated css string")
			}
			builder.WriteByte(t.input[t.pos])
			t.pos++
			start = t.pos
			continue
		}
		t.pos++
	}
	return Token{}, fmt.Errorf("unterminated css string")
}

func (t *Tokenizer) scanAtKeyword() (Token, error) {
	t.pos++
	if t.pos >= len(t.input) || !isIdentStart(t.input[t.pos:]) {
		return Token{Type: TokenDelim, Value: "@"}, nil
	}
	token := t.scanIdent()
	token.Type = TokenAtKeyword
	token.Value = strings.ToLower(token.Value)
	return token, nil
}

func (t *Tokenizer) scanHash() (Token, error) {
	t.pos++
	if t.pos >= len(t.input) {
		return Token{}, fmt.Errorf("expected identifier after '#' at end of input")
	}

	start := t.pos
	for t.pos < len(t.input) {
		r, size := utf8.DecodeRuneInString(t.input[t.pos:])
		if !isIdentRune(r) {
			break
		}
		t.pos += size
	}

	if start == t.pos {
		return Token{}, fmt.Errorf("expected identifier after '#' at position %d", start)
	}

	return Token{Type: TokenHash, Value: t.input[start:t.pos]}, nil
}

func (t *Tokenizer) scanIdent() Token {
	start := t.pos
	for t.pos < len(t.input) {
		r, size := utf8.DecodeRuneInString(t.input[t.pos:])
		if !isIdentRune(r) {
			break
		}
		t.pos += size
	}
	return Token{Type: TokenIdent, Value: t.input[start:t.pos]}
}

func (t *Tokenizer) skipComment() error {
	end := strings.Index(t.input[t.pos+2:], "*/")
	if end < 0 {
		return fmt.Errorf("unterminated css comment")
	}
	t.pos += 2 + end + 2
	return nil
}

func isWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}

func isIdentStart(input string) bool {
	r, _ := utf8.DecodeRuneInString(input)
	return unicode.IsLetter(r) || r == '_' || r == '-'
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}
