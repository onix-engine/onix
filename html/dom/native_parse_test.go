package dom

import (
	"errors"
	"strings"
	"testing"
	"unsafe"
)

func TestParserReuseAllocs(t *testing.T) {
	for _, source := range []string{
		`<article>Hello <b>world</b>.</article>`,
		`<!doctype html><HTML><BODY><DIV ID="app" CLASS='page' data-x='a&amp;b'>中文 &notin; &#x1F600; &NotEqualTilde;<input type=checkbox checked><select><option>One<option selected>Two</select></DIV>`,
		`<b class=x><p>bold<i>italic</b>after</i><table>before<tr><td>A<td>B</table>`,
		`<svg viewBox="0 0 10 10"><foreignObject><div>x</div></foreignObject><path xlink:href="#x"/></svg>`,
		`<svg><foreignObject><template><div>inside</div></template><p>after`,
		"<!-- prefix -->\n<pre>\nA\r\nB<!-- gap -->C\x00</pre>",
		strings.Repeat("Long document 中文 &amp; text. ", 8192),
	} {
		var parser Parser
		root, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		want := root.String()
		// Warm once more to include any capacity needed by repairs.
		if _, err := parser.Parse(source); err != nil {
			t.Fatal(err)
		}
		allocs := testing.AllocsPerRun(20, func() {
			if _, err := parser.Parse(source); err != nil {
				panic(err)
			}
		})
		if allocs != 0 {
			t.Errorf("repeated parse: %g allocs for %.100q", allocs, source)
		}
		if got := parser.builder.doc.String(); got != want {
			t.Fatalf("reuse changed tree:\n%s\nwant:\n%s", got, want)
		}
	}
}

func TestParserResetAndOwnership(t *testing.T) {
	first, err := Parse(`<p id=old>first &amp; text</p>`)
	if err != nil {
		t.Fatal(err)
	}
	want := first.String()
	var parser Parser
	large := strings.Repeat(`<section id=x><b>hello &amp; world</b></section>`, 300)
	if _, err := parser.Parse(large); err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(`<input type=checkbox checked>`); err != nil {
		t.Fatal(err)
	}
	root, err := parser.Parse(`<div>new</div>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 1 || len(root.Children[0].Attributes) != 0 || root.Children[0].States != (StateSet[ElementState]{}) {
		t.Fatalf("retained previous state: %s", root.String())
	}
	var order uint32
	validateTree(t, root, nil, &order)
	parser.Reset()
	for _, block := range parser.blocks {
		for i := range block {
			n := &block[i].node
			if n.Data != "" || n.Parent != nil || len(n.Children) != 0 || len(n.Attributes) != 0 || n.States != (StateSet[ElementState]{}) {
				t.Fatal("reset retained source or tree references")
			}
		}
	}
	if parser.scanner.input != "" || parser.builder.doc != nil {
		t.Fatal("reset retained document")
	}
	if got := first.String(); got != want {
		t.Fatal("an independent Parse result was mutated")
	}
}

func TestParserBorrowsUnchangedSource(t *testing.T) {
	source := `<custom data-name="value">unchanged 中文 text</custom>`
	root, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	element := root.Children[0]
	for _, value := range []string{element.Data, element.Attributes["data-name"], element.Children[0].Data} {
		offset := strings.Index(source, value)
		if unsafe.StringData(value) != unsafe.StringData(source[offset:]) {
			t.Errorf("copied unchanged source %q", value)
		}
	}
}

func TestCharacterReferences(t *testing.T) {
	var parser Parser
	for _, ref := range namedReferences {
		source := "&" + ref.name
		if got := parser.decode(source, false, true, false, false); got != ref.value {
			t.Fatalf("%s = %q; want %q", source, got, ref.value)
		}
	}
	for _, test := range []struct{ source, text, attribute string }{
		{"&notit;", "¬it;", "&notit;"},
		{"&amp=", "&=", "&amp="},
		{"&notin;", "∉", "∉"},
		{"&#0;&#xD800;&#1114112;", "���", "���"},
		{"&#128;", "€", "€"},
		{"&#0A", "�A", "�A"},
		{"&#9999999999999999999999;", "�", "�"},
		{"&#x; &#; &unknown;", "&#x; &#; &unknown;", "&#x; &#; &unknown;"},
		{"&#13;\r\n", "\r\n", "\r\n"},
	} {
		if got := parser.decode(test.source, false, true, true, false); got != test.text {
			t.Errorf("text %q = %q; want %q", test.source, got, test.text)
		}
		if got := parser.decode(test.source, true, true, true, false); got != test.attribute {
			t.Errorf("attribute %q = %q; want %q", test.source, got, test.attribute)
		}
	}
}

func TestTokenizerHTMLRecovery(t *testing.T) {
	for _, test := range []struct {
		source string
		kind   TokenType
		data   string
		attrs  map[string]string
	}{
		{`<DIV A=first a=second>`, TokenStartTag, "div", map[string]string{"a": "first"}},
		{`<img src=http://example.test/icon.png/>`, TokenStartTag, "img", map[string]string{"src": "http://example.test/icon.png/"}},
		{"<div\u00a0id=x>", TokenStartTag, "div\u00a0id=x", nil},
		{`<div`, TokenEOF, "", nil},
		{`<p a="unterminated`, TokenEOF, "", nil},
		{`<`, TokenText, "<", nil},
		{`1 < 2`, TokenText, "1 < 2", nil},
		{`<!--unfinished`, TokenEOF, "", nil},
	} {
		token, err := NewTokenizer(test.source).Next()
		if err != nil || token.Type != test.kind || token.Data != test.data {
			t.Fatalf("%q => %#v, %v", test.source, token, err)
		}
		if len(token.Attributes) != len(test.attrs) {
			t.Fatalf("%q attrs = %#v", test.source, token.Attributes)
		}
		for key, value := range test.attrs {
			if token.Attributes[key] != value {
				t.Fatalf("%q attrs = %#v", test.source, token.Attributes)
			}
		}
	}
	// ASCII folding must leave non-ASCII tag/attribute letters untouched.
	token, _ := NewTokenizer(`<x-Ä DATA-Ä=ok>`).Next()
	if token.Data != "x-Ä" || token.Attributes["data-Ä"] != "ok" {
		t.Fatalf("non-ASCII folding: %#v", token)
	}
}

func TestParseDepthLimit(t *testing.T) {
	if _, err := Parse(strings.Repeat("<div>", 520)); !errors.Is(err, ErrNestingDepth) {
		t.Fatalf("depth error = %v", err)
	}
	var parser Parser
	if _, err := parser.Parse(strings.Repeat("<div>", 520)); !errors.Is(err, ErrNestingDepth) {
		t.Fatal(err)
	}
	if _, err := parser.Parse(`<p>recovered`); err != nil {
		t.Fatal(err)
	}
}

func TestDoctypeNameUsesASCIICaseAndNoReferences(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{`<!DOCTYPE HTML><p>one<table><tr><td>two`, `(html)(head)(/)(body)(p)"one"(/)(table)(tbody)(tr)(td)"two"(/)(/)(/)(/)(/)(/)`},
		{`<!DOCTYPE htm&#108;><p>one<table><tr><td>two`, `(html)(head)(/)(body)(p)"one"(table)(tbody)(tr)(td)"two"(/)(/)(/)(/)(/)(/)(/)`},
	} {
		root, err := Parse(test.source)
		if err != nil {
			t.Fatal(err)
		}
		if got := htmlTreeShape(root); got != test.want {
			t.Errorf("%s => %s; want %s", test.source, got, test.want)
		}
	}
}

func validateTree(t *testing.T, node, parent *Node, order *uint32) {
	t.Helper()
	if node == nil || node.Parent != parent || node.Order != *order {
		t.Fatalf("invalid parent/order at node %#v", node)
	}
	*order++
	for _, child := range node.Children {
		validateTree(t, child, node, order)
	}
}

func FuzzParse(f *testing.F) {
	for _, source := range []string{"", "<", "<p a='", `<b><p>one</b>two`, `<table>A<tr><td>B`, `<svg><foreignObject><p>x</svg>`, `<svg><foreignObject><template><table><td>x</template><p>after`, `<template><svg><foreignObject><template><div>nested`, "<pre><!--x-->\nA", `<script><!--<script></script>--></script>tail`, "<!-->\u2003&#32;<html>"} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 65536 {
			t.Skip()
		}
		first, err := Parse(source)
		if err != nil {
			if errors.Is(err, ErrNestingDepth) {
				return
			}
			t.Fatal(err)
		}
		var order uint32
		validateTree(t, first, nil, &order)
		var parser Parser
		for _, input := range []string{`<div id=old>old</div>`, source} {
			if _, err := parser.Parse(input); err != nil {
				t.Fatal(err)
			}
		}
		if first.String() != parser.builder.doc.String() {
			t.Fatalf("reuse mismatch for %q", source)
		}
	})
}

func BenchmarkParserReuse(b *testing.B) {
	for _, test := range []struct{ name, source string }{
		{"plain", strings.Repeat("Long document 中文文本 plain words. ", 8192)},
		{"entities", strings.Repeat("Long document 中文文本 &amp; plain words. ", 8192)},
		{"elements", `<!doctype html><html><body>` + strings.Repeat(`<div id="app" class="card"><p>A &amp; B</p><input type=checkbox checked></div>`, 100) + `</body></html>`},
	} {
		b.Run(test.name, func(b *testing.B) {
			var parser Parser
			if _, err := parser.Parse(test.source); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(test.source)))
			for b.Loop() {
				if _, err := parser.Parse(test.source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func FuzzTokenizerProgress(f *testing.F) {
	for _, source := range []string{"&amp; &notit; &#x1F600;", "&#128;", "&#0;", "&NotEqualTilde;"} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		tokenizer := NewTokenizer(source)
		for previous := 0; ; {
			token, err := tokenizer.Next()
			if err != nil {
				t.Fatal(err)
			}
			if token.Type == TokenEOF {
				break
			}
			end := tokenizer.parser.scanner.raw.end
			if end <= previous || end > len(source) {
				t.Fatalf("tokenizer failed to advance at %d in %q", end, source)
			}
			previous = end
		}
	})
}
