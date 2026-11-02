package syntax

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"
)

func TestDocumentLineStateConverges(t *testing.T) {
	source := "package demo\n" + strings.Repeat("func value() int { return 42 }\n", 2000)
	var doc Document
	defer doc.Close()
	if err := doc.Update("main.go", source); err != nil {
		t.Fatal(err)
	}
	offset := strings.Index(source, "42")
	edited := source[:offset] + "123" + source[offset+2:]
	if err := doc.UpdateRange("main.go", edited, offset, offset+2, offset+3); err != nil {
		t.Fatal(err)
	}
	if doc.retokenized != 1 {
		t.Fatalf("local edit tokenized %d lines, want 1", doc.retokenized)
	}
	var fresh Document
	defer fresh.Close()
	if err := fresh.Update("main.go", edited); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Captures(), fresh.Captures()) {
		t.Fatal("shifted cached suffix differs from fresh tokenization")
	}
	theme := testTheme(t, "dark-plus")
	doc.Tokens(theme, nil)
	if allocs := testing.AllocsPerRun(100, func() {
		if err := doc.Update("main.go", edited); err != nil {
			panic(err)
		}
		doc.Tokens(theme, nil)
	}); allocs != 0 {
		t.Fatalf("unchanged warm document allocated %g times", allocs)
	}
}

func TestDocumentMultilineStatesAndSplices(t *testing.T) {
	for _, test := range []struct{ path, source, find, replacement string }{
		{"main.go", "package demo\n/* open\nbody\n*/\nvar value=1\n", "/* open", "// open"},
		{"main.go", "package demo\nvar text=`first\nsecond\nthird`\nvar value=1\n", "second", "second`\n// inserted\nvar other=`"},
		{"main.py", "value = '''first\nsecond\nthird'''\nprint(value)\n", "second\n", "'''\n# comment\n'''"},
		{"main.js", "const value = `first\n${call(42)}\nthird`;\nconst after=1;\n", "${call(42)}", "middle\n${other(12)}"},
		{"main.sh", "cat <<EOF\nfirst\nsecond\nEOF\necho done\n", "first\n", "EOF\necho changed\ncat <<EOF\n"},
		{"main.md", "# Heading\n```go\nvar value=1\n```\nafter\n", "```go", "plain"},
		{"main.html", "<script>\nconst value = '🙂';\n</script>\n<div>中文</div>\n", "</script>", "// more JS"},
	} {
		t.Run(test.path, func(t *testing.T) {
			var doc Document
			defer doc.Close()
			for _, source := range []string{test.source, strings.Replace(test.source, test.find, test.replacement, 1), test.source, "\n" + test.source, strings.TrimSuffix(test.source, "\n"), "", test.source} {
				if err := doc.Update(test.path, source); err != nil {
					t.Fatal(err)
				}
				var fresh Document
				if err := fresh.Update(test.path, source); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(doc.Captures(), fresh.Captures()) {
					t.Fatalf("incremental scopes differ from fresh for %q", source)
				}
				fresh.Close()
			}
		})
	}
}

func TestDocumentUnicodeAndCRLFByteRanges(t *testing.T) {
	source := "package demo\r\nvar 名字 = \"中文🙂\" // 注释\r\n"
	var doc Document
	defer doc.Close()
	if err := doc.Update("main.go", source); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, c := range doc.Captures() {
		if !byteBoundary(source, int(c.StartByte)) || !byteBoundary(source, int(c.EndByte)) {
			t.Fatalf("split UTF-8 token: %+v", c)
		}
		if c.Capture == "string" && strings.Contains(source[c.StartByte:c.EndByte], "中文🙂") {
			seen = true
		}
	}
	if !seen {
		t.Fatal("Unicode literal was not colored as a string")
	}
	offset := strings.Index(source, "名字") + 1
	before := slices.Clone(doc.Captures())
	if err := doc.UpdateRange("main.go", source, offset, offset, offset); err == nil {
		t.Fatal("accepted an edit inside a UTF-8 code point")
	}
	if !slices.Equal(before, doc.Captures()) {
		t.Fatal("invalid edit mutated the document")
	}
}

func TestDocumentLongLineBudgetAndRecovery(t *testing.T) {
	source := "package demo\n// " + strings.Repeat("x", 20001) + "\nvar value=42\n"
	var doc Document
	defer doc.Close()
	if err := doc.Update("main.go", source); err != nil {
		t.Fatal(err)
	}
	offset := strings.Index(source, "42")
	if !slices.ContainsFunc(doc.Captures(), func(c Capture) bool { return int(c.StartByte) == offset && c.Capture == "number" }) {
		t.Fatal("long line disabled highlighting of the following normal line")
	}
	if err := doc.Update("main.go", strings.Replace(source, strings.Repeat("x", 20001), "short", 1)); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(doc.Captures(), func(c Capture) bool { return c.Capture == "comment" }) {
		t.Fatal("shortened line did not regain highlighting")
	}
}

func TestDocumentNativeScopesAndCategoryOverrides(t *testing.T) {
	var doc Document
	defer doc.Close()
	if err := doc.Update("main.go", "package demo\n// comment\n"); err != nil {
		t.Fatal(err)
	}
	theme, err := LoadTheme(fstest.MapFS{"theme.json": {Data: []byte(`{"colors":{"editor.foreground":"#eeeeee"},"tokenColors":[{"scope":"source.go comment.line.double-slash.go","settings":{"foreground":"#123456"}}]}`)}}, "theme.json")
	if err != nil {
		t.Fatal(err)
	}
	tokens := doc.Tokens(theme, nil)
	if !slices.ContainsFunc(tokens, func(token Token) bool { return token.Capture == "comment" && token.Style.Foreground.Hex() == "#123456" }) {
		t.Fatal("native parent/child TextMate scopes were lost")
	}
	overrides := DefaultDefinitions().WithOverrides(&Definitions{Captures: map[string]string{"comment": "string.quoted"}})
	tokens = doc.Tokens(testTheme(t, "monokai"), overrides)
	for _, token := range tokens {
		if token.Capture == "comment" && token.Style.Foreground != testTheme(t, "monokai").Resolve("source.go", "string.quoted.go").Foreground {
			t.Fatal("custom category override was ignored")
		}
	}
}

func FuzzDocumentIncrementalEdits(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte("comment\n中文🙂string"))
	f.Fuzz(func(t *testing.T, edits []byte) {
		if len(edits) > 96 {
			edits = edits[:96]
		}
		source := "package demo\nvar 名字 = `中文🙂`\n// comment\nfunc f(){println(名字)}\n"
		var doc Document
		defer doc.Close()
		if err := doc.Update("main.go", source); err != nil {
			t.Fatal(err)
		}
		inserts := []string{"", "/*", "*/", "\n", "`", "\"", "🙂", "\r\n", "//", "名"}
		for index, edit := range edits {
			start := int(edit) % (len(source) + 1)
			for !byteBoundary(source, start) {
				start--
			}
			end := start
			if index%2 == 0 && end < len(source) {
				_, size := utf8.DecodeRuneInString(source[end:])
				end += size
			}
			insert := inserts[int(edit)%len(inserts)]
			next := source[:start] + insert + source[end:]
			if err := doc.UpdateRange("main.go", next, start, end, start+len(insert)); err != nil {
				t.Fatal(err)
			}
			var fresh Document
			if err := fresh.Update("main.go", next); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(doc.Captures(), fresh.Captures()) {
				t.Fatalf("incremental mismatch after edit %d: %q", index, next)
			}
			fresh.Close()
			source = next
		}
	})
}

func BenchmarkDocumentLineEdit(b *testing.B) {
	source := "package demo\n" + strings.Repeat("func value() int { return 42 }\n", 2000)
	offset := strings.Index(source, "42")
	edited := source[:offset] + "24" + source[offset+2:]
	var doc Document
	defer doc.Close()
	doc.Update("main.go", source)
	doc.Update("main.go", edited)
	doc.Update("main.go", source)
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	for b.Loop() {
		doc.UpdateRange("main.go", edited, offset, offset+2, offset+2)
		doc.UpdateRange("main.go", source, offset, offset+2, offset+2)
	}
}
