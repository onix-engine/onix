package dom

import (
	"html"
	"strings"
	"testing"
)

func TestHTMLDocumentDetection(t *testing.T) {
	for _, test := range []struct {
		source   string
		document bool
	}{
		{"", false}, {" \t\r\n", false}, {"<!doctype html>", true},
		{"<!-- lead -->text", false}, {"<!--> <html>", true},
		{"&#32;<body>", true}, {"\u2003<html>", true},
		{"<![CDATA[text]]><html>", true}, {"<!--\x00-->&#x20;<head>", true},
		{"<!-- leading -->&amp;<html>", false}, {"</body><html>", false},
		{"<!DoCtYpE><p>", true}, {"<!--unterminated <html>", false},
		{"<htmlish>", false}, {"<HTML/>", true}, {"<html", false},
	} {
		if got := isHTMLDocument(test.source); got != test.document {
			t.Errorf("document detection for %q = %v; want %v", test.source, got, test.document)
		}
	}
}

func TestLongTextDocumentDetection(t *testing.T) {
	text := strings.Repeat("Long document 中文 &amp; text. ", 8192)
	for _, prefix := range []string{"", " \n", "<!-- leading -->\n", "<!-- first --><!-- second -->\t"} {
		if isHTMLDocument(prefix + text) {
			t.Fatal("plain fragment was classified as a document")
		}
		root, err := Parse(prefix + text)
		if err != nil {
			t.Fatal(err)
		}
		var got strings.Builder
		for _, child := range root.Children {
			if child.Type != TextNode {
				t.Fatal("plain fragment gained an element wrapper")
			}
			got.WriteString(child.Data)
		}
		if strings.TrimLeft(got.String(), " \n\t") != html.UnescapeString(text) {
			t.Fatal("document detection changed long text or entity decoding")
		}
	}
	for _, source := range []string{
		"<!-- leading -->\n<!doctype html><p>Text</p>",
		"\u2003<!-- leading --> &#32;&#10;<html><body>Text</body></html>",
		"<!-- leading -->&#x20;<body>Text</body>",
	} {
		if !isHTMLDocument(source) {
			t.Fatalf("document wrappers were lost after whitespace/comments: %q", source)
		}
	}
	if isHTMLDocument("<!-- leading -->&amp;<html></html>") {
		t.Fatal("an entity producing visible text was ignored")
	}
}

func BenchmarkParseLongText(b *testing.B) {
	text := strings.Repeat("Long document 中文文本 &amp; plain words. ", 8192)
	for _, test := range []struct{ name, source string }{
		{"plain", text},
		{"comment-plain", "<!-- leading -->\n" + text},
		{"article", "<!doctype html><html><body><article>" + text + "</article></body></html>"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(test.source)))
			for b.Loop() {
				if _, err := Parse(test.source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
