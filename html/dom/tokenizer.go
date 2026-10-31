package dom

type TokenType int

const (
	TokenError TokenType = iota
	TokenEOF
	TokenText
	TokenStartTag
	TokenEndTag
	TokenSelfClosingTag
	tokenComment
	tokenDoctype
)

type Token struct {
	Type       TokenType
	Data       string
	Attributes map[string]string
}

// Tokenizer scans UTF-8 HTML, borrowing unchanged source text. It skips comments
// and declarations, recovers malformed input, and handles raw text elements.
// Returned tokens remain valid for the lifetime of the tokenizer.
type Tokenizer struct{ parser Parser }

func NewTokenizer(input string) *Tokenizer {
	t := &Tokenizer{}
	t.parser.scanner.reset(input)
	return t
}

func (t *Tokenizer) Next() (Token, error) {
	p := &t.parser
	for {
		kind := p.scanner.scan()
		if kind == tokenComment || kind == tokenDoctype {
			continue
		}
		source := p.token(kind)
		token := Token{Type: kind, Data: source.Data}
		if len(source.Attr) != 0 {
			token.Attributes = make(map[string]string, len(source.Attr))
			for _, attr := range source.Attr {
				token.Attributes[attr.Key] = attr.Val
			}
		}
		return token, nil
	}
}
