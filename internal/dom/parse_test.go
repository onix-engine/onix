package dom

import (
	"strings"
	"testing"
)

func TestTokenizer(t *testing.T) {
	input := `<!doctype html><div id="app" class='hero'><img src="a.png" /><span>Hello</span></div>`
	tokenizer := NewTokenizer(input)

	var tokens []Token
	for {
		token, err := tokenizer.Next()
		if err != nil {
			t.Fatalf("next token: %v", err)
		}
		tokens = append(tokens, token)
		if token.Type == TokenEOF {
			break
		}
	}

	if len(tokens) != 7 {
		t.Fatalf("token count = %d, want 7", len(tokens))
	}

	if got := tokens[0]; got.Type != TokenStartTag || got.Data != "div" {
		t.Fatalf("first token = %#v", got)
	}

	if got, want := tokens[0].Attributes["id"], "app"; got != want {
		t.Fatalf("div id = %q, want %q", got, want)
	}

	if got := tokens[1]; got.Type != TokenSelfClosingTag || got.Data != "img" {
		t.Fatalf("second token = %#v", got)
	}

	if got := tokens[2]; got.Type != TokenStartTag || got.Data != "span" {
		t.Fatalf("third token = %#v", got)
	}

	if got := tokens[3]; got.Type != TokenText || got.Data != "Hello" {
		t.Fatalf("text token = %#v", got)
	}

	if got := tokens[4]; got.Type != TokenEndTag || got.Data != "span" {
		t.Fatalf("end span token = %#v", got)
	}

	if got := tokens[5]; got.Type != TokenEndTag || got.Data != "div" {
		t.Fatalf("end div token = %#v", got)
	}

	if got := tokens[6]; got.Type != TokenEOF {
		t.Fatalf("eof token = %#v", got)
	}
}

func TestTokenizerAcceptsSlashInUnquotedAttributeValue(t *testing.T) {
	token, err := NewTokenizer(`<img src=http://example.test/icon.png>`).Next()
	if err != nil {
		t.Fatal(err)
	}
	if token.Type != TokenStartTag || token.Data != "img" {
		t.Fatalf("token = %#v", token)
	}
	if got, want := token.Attributes["src"], "http://example.test/icon.png"; got != want {
		t.Fatalf("src = %q, want %q", got, want)
	}
}

func TestParseBuildsDOM(t *testing.T) {
	input := `<html><body><div id="app" class="hero"><p>Hello <span>renderer</span></p><img src="logo.png" /></div></body></html>`
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}

	if len(doc.Children) != 1 {
		t.Fatalf("document children = %d, want 1", len(doc.Children))
	}

	html := doc.Children[0]
	if html.Data != "html" {
		t.Fatalf("root element = %q, want html", html.Data)
	}

	if len(html.Children) != 2 || html.Children[0].Data != "head" || html.Children[1].Data != "body" {
		t.Fatalf("HTML5 document structure = %s", html.String())
	}
	body := html.Children[1]
	div := body.Children[0]

	if got, ok := div.Attr("id"); !ok || got != "app" {
		t.Fatalf("div id = %q, ok = %v", got, ok)
	}

	paragraph := div.Children[0]
	if paragraph.Data != "p" {
		t.Fatalf("first child tag = %q, want p", paragraph.Data)
	}

	if got := paragraph.Children[0].Data; got != "Hello " {
		t.Fatalf("text = %q, want %q", got, "Hello ")
	}

	if got := paragraph.Children[1].Data; got != "span" {
		t.Fatalf("inline child = %q, want span", got)
	}

	if got := div.Children[1].Data; got != "img" {
		t.Fatalf("self-closing child = %q, want img", got)
	}

	printed := doc.String()
	if !strings.Contains(printed, `<div class="hero" id="app">`) {
		t.Fatalf("printed tree missing div attrs: %s", printed)
	}

	if !strings.Contains(printed, `"renderer"`) {
		t.Fatalf("printed tree missing text node: %s", printed)
	}
}

func TestParseDocumentAfterLeadingCommentKeepsHTML5Wrappers(t *testing.T) {
	doc, err := Parse("<!--before-->\n<html><body><p>Text</p></body></html>")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 || doc.Children[0].Data != "html" || len(doc.Children[0].Children) != 2 || doc.Children[0].Children[0].Data != "head" || doc.Children[0].Children[1].Data != "body" {
		t.Fatalf("document lost HTML5 wrappers: %s", doc.String())
	}
}

func TestParseTreatsIMGAsVoidElement(t *testing.T) {
	doc, err := Parse(`<html><body><img src="logo.png"><p>after</p></body></html>`)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}

	body := doc.Children[0].Children[1]
	if len(body.Children) != 2 {
		t.Fatalf("body children = %d, want 2", len(body.Children))
	}
	if got := body.Children[0].Data; got != "img" {
		t.Fatalf("first child = %q, want img", got)
	}
	if got := body.Children[1].Data; got != "p" {
		t.Fatalf("second child = %q, want p", got)
	}
}

func TestParseSupportsFormControls(t *testing.T) {
	doc, err := Parse(`<html><body><button type="button">Press</button><input type="checkbox" checked><select><option>One</option><option selected>Two</option></select></body></html>`)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}

	body := doc.Children[0].Children[1]
	if len(body.Children) != 3 {
		t.Fatalf("body children = %d, want 3", len(body.Children))
	}

	button := body.Children[0]
	if got := button.Data; got != "button" {
		t.Fatalf("first child = %q, want button", got)
	}
	if len(button.Children) != 1 || button.Children[0].Data != "Press" {
		t.Fatalf("button children = %#v", button.Children)
	}

	input := body.Children[1]
	if got := input.Data; got != "input" {
		t.Fatalf("second child = %q, want input", got)
	}
	if _, ok := input.Attr("checked"); !ok {
		t.Fatal("checkbox input missing checked attribute")
	}
	if len(input.Children) != 0 {
		t.Fatalf("input children = %d, want 0", len(input.Children))
	}

	selectNode := body.Children[2]
	if got := selectNode.Data; got != "select" {
		t.Fatalf("third child = %q, want select", got)
	}
	if len(selectNode.Children) != 2 {
		t.Fatalf("select children = %d, want 2", len(selectNode.Children))
	}
	if _, ok := selectNode.Children[1].Attr("selected"); !ok {
		t.Fatal("selected option missing selected attribute")
	}
}

func TestParseHTML5RecoversOmittedEndTags(t *testing.T) {
	doc, err := Parse(`<p>one<p>two<table><tr><td>cell`)
	if err != nil {
		t.Fatalf("html5 parse: %v", err)
	}
	if len(doc.Children) != 3 || doc.Children[0].Data != "p" || doc.Children[1].Data != "p" || doc.Children[2].Data != "table" {
		t.Fatalf("recovered fragment tree = %#v", doc.Children)
	}
	table := doc.Children[2]
	if len(table.Children) != 1 || table.Children[0].Data != "tbody" || table.Children[0].Children[0].Data != "tr" {
		t.Fatalf("table repair tree = %s", table.String())
	}
}
