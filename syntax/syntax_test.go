package syntax

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func testTheme(t testing.TB, id string) *Theme {
	t.Helper()
	theme, err := BuiltinTheme(id)
	if err != nil {
		t.Fatal(err)
	}
	return theme
}

func TestDocumentLanguages(t *testing.T) {
	theme := testTheme(t, "dark-plus")
	for _, fixture := range []struct{ name, source, colored string }{
		{"main.go", "package main\nfunc main() { println(\"中文\") }\n", "\"中文\""},
		{"app.js", "const message = \"hello\"; function greet() { return message; }", "\"hello\""},
		{"app.ts", "const message: string = \"hello\";", "\"hello\""},
		{"view.tsx", "const view = <div title=\"hello\">Hello</div>;", "hello"},
		{"main.py", "def greet():\n    return \"hello\"\n", "\"hello\""},
		{"main.rs", "fn main() { println!(\"hello\"); }", "hello"},
		{"main.c", "int main() { return 42; }", "42"},
		{"main.cpp", "int main() { return 42; }", "42"},
		{"index.html", "<div title=\"hello\">Hello</div>", "hello"},
		{"style.css", "body { color: red; width: 42px; }", "42"},
		{"data.json", "{\"message\": \"hello\", \"n\": 42}", "hello"},
		{"data.yaml", "message: \"hello\"\nn: 42\n", "hello"},
		{"script.sh", "#!/bin/sh\necho \"hello\"\n", "hello"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var document Document
			defer document.Close()
			if err := document.Update(fixture.name, fixture.source); err != nil {
				t.Fatal(err)
			}
			if document.Tree() == nil || document.Tree().RootNode() == nil {
				t.Fatal("language did not produce an AST")
			}
			tokens := document.Tokens(theme, nil)
			position := strings.Index(fixture.source, fixture.colored)
			found := false
			end := 0
			for _, token := range tokens {
				if token.Start < end || token.Start >= token.End || token.End > len(fixture.source) {
					t.Fatalf("invalid byte ranges: %+v", tokens)
				}
				end = token.End
				if token.Start <= position && position < token.End {
					found = true
				}
			}
			if !found {
				t.Fatalf("%q is uncolored: %+v", fixture.colored, tokens)
			}
		})
	}
}

func TestDocumentIncrementalMatchesFreshParse(t *testing.T) {
	var document Document
	defer document.Close()
	theme := testTheme(t, "monokai")
	for _, source := range []string{
		"package main\n// 中文\nfunc main() { println(\"hello\") }\n",
		"package main\n// 中新增文\nfunc main() { println(\"hello\") }\n",
		"package main\n// 中\n// 文\nfunc main() { println(\"你好\") }\n",
		"package main\nfunc main() { println(\"你好\") }\n",
		"package main\nfunc main() { println(\"你好\")\n",
		"",
		"package main\nfunc main() {}\n",
	} {
		if err := document.Update("main.go", source); err != nil {
			t.Fatal(err)
		}
		var fresh Document
		if err := fresh.Update("main.go", source); err != nil {
			t.Fatal(err)
		}
		if actual, expected := document.Tokens(theme, nil), fresh.Tokens(theme, nil); !slices.Equal(actual, expected) {
			t.Fatalf("incremental highlights differ for %q:\n%+v\n%+v", source, actual, expected)
		}
		fresh.Close()
	}
}

func TestDocumentSwitchReusesLanguageAndMatchesFreshParse(t *testing.T) {
	var document Document
	defer document.Close()
	theme := testTheme(t, "dark-plus")
	for _, fixture := range []struct{ name, source string }{
		{"a/main.go", "package a\nfunc First() { println(1) }\n"},
		{"b/main.go", "package b\nvar Second = \"二\"\n"},
		{"a/main.go", "package a\nfunc First() { println(1) }\n"},
		{"app.py", "def first():\n    return 2\n"},
		{"other.py", "value = 'second'\n"},
		{"notes.unknown", "plain text"},
		{"again.go", "package again\nvar Value = 3\n"},
	} {
		previous, language := document.highlighter, document.Language()
		if err := document.Update(fixture.name, fixture.source); err != nil {
			t.Fatal(err)
		}
		if language == document.Language() && previous != nil && previous != document.highlighter {
			t.Fatal("switching files recreated the same language's highlighter")
		}
		var fresh Document
		if err := fresh.Update(fixture.name, fixture.source); err != nil {
			t.Fatal(err)
		}
		if actual, expected := document.Tokens(theme, nil), fresh.Tokens(theme, nil); !slices.Equal(actual, expected) || document.rootScope != fresh.rootScope {
			t.Fatalf("switching to %s changed syntax colors or scopes", fixture.name)
		}
		fresh.Close()
		ReleaseParserMemory()
	}
	document.Close()
	document.Close()
}

func TestDocumentCacheAndThemeSwitch(t *testing.T) {
	var document Document
	defer document.Close()
	const source = "package main\n// comment\nfunc main() {}\n"
	if err := document.Update("main.go", source); err != nil {
		t.Fatal(err)
	}
	monokai := testTheme(t, "monokai")
	dark := testTheme(t, "dark-plus")
	first := append([]Token(nil), document.Tokens(monokai, nil)...)
	tree := document.Tree()
	second := document.Tokens(dark, nil)
	if document.Tree() != tree || slices.Equal(first, second) {
		t.Fatal("theme switch reparsed the document or did not recolor tokens")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := document.Update("main.go", source); err != nil {
			panic(err)
		}
		document.Tokens(dark, nil)
	}); allocs != 0 {
		t.Fatalf("cached document allocated %g times", allocs)
	}
	if err := document.Update("notes.unknown", source); err != nil {
		t.Fatal(err)
	}
	if document.Tree() != nil || len(document.Tokens(dark, nil)) != 0 {
		t.Fatal("unknown language retained the previous file's tree or colors")
	}
}

func TestThemeJSONCIncludesAndScopes(t *testing.T) {
	files := fstest.MapFS{
		"resources/base.json": {Data: []byte(`{
			"colors": {"editor.foreground":"#fff", "editor.background":"#1234"},
			"tokenColors": [
				{"scope":"entity.name", "settings":{"foreground":"#111111","fontStyle":"italic"}},
				{"scope":"entity.name.function", "settings":{"foreground":"#222222"}}
			]
		}`)},
		"resources/child.jsonc": {Data: []byte(`{
			// VS Code permits both comments and trailing commas.
			"include": "base.json",
			"name": "Custom // theme",
			"tokenColors": [
				{"scope":"entity", "settings":{"foreground":"#999999"}},
				{"scope":["string, comment", "source.go entity.name.function"], "settings":{"foreground":"#abcdef","fontStyle":"bold underline"}},
				{"scope":"source.go entity.name.function", "settings":{"foreground":"#fedcba"}},
				{"scope":"source entity.name.function", "settings":{"foreground":"#999999"}},
				{"scope":"variable", "settings":{"fontStyle":"italic"}},
				{"scope":"variable.parameter", "settings":{"fontStyle":""}},
			], /* block comment */
		}`)},
	}
	theme, err := LoadTheme(files, "resources/child.jsonc")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Name != "Custom // theme" || theme.Color("editor.background").Hex() != "#11223344" {
		t.Fatal("include or quoted comment marker was corrupted")
	}
	goStyle := theme.Resolve("source.go", "entity.name.function.go")
	if goStyle.Foreground.Hex() != "#fedcba" || goStyle.FontStyle != Bold|Underline {
		t.Fatalf("parent scope, property inheritance, or last-rule precedence is wrong: %+v", goStyle)
	}
	python := theme.Resolve("source.python", "entity.name.function.python")
	if python.Foreground.Hex() != "#999999" || python.FontStyle != Italic {
		t.Fatalf("source selector or inherited font style is wrong: %+v", python)
	}
	if style := theme.Resolve("source.go", "variable.parameter.go"); style.FontStyle != 0 {
		t.Fatal("empty fontStyle did not clear inherited style")
	}
	if allocs := testing.AllocsPerRun(100, func() { theme.Resolve("source.go", "entity.name.function.go") }); allocs != 0 {
		t.Fatalf("scope matching allocated %g times", allocs)
	}
}

func TestContextualCapturesOverrideGenericIdentifiers(t *testing.T) {
	var document Document
	defer document.Close()
	const source = "package main\nfunc greet() { world.Spawn(); println(\"hello\") }\n"
	if err := document.Update("main.go", source); err != nil {
		t.Fatal(err)
	}
	tokens := document.Tokens(testTheme(t, "monokai"), nil)
	for _, fixture := range []struct{ word, capture string }{
		{"greet", "function"}, {"world", "variable"}, {"Spawn", "function.method"}, {"println", "function.builtin"},
	} {
		position := strings.Index(source, fixture.word)
		found := false
		for _, token := range tokens {
			if token.Start == position && token.Capture == fixture.capture {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s did not receive %s: %+v", fixture.word, fixture.capture, tokens)
		}
	}
}

func TestThemeRejectsInvalidConfiguration(t *testing.T) {
	for _, source := range []string{
		`{"include":"theme.json"}`,
		`{"colors":{"editor.foreground":"#zzzzzz"}}`,
		`{"tokenColors":[{"settings":{"fontStyle":"unknown"}}]}`,
		`{"tokenColors":[,]}`,
		`{"tokenColors":[{"settings":,}]}`,
		`{}/*`,
	} {
		_, err := LoadTheme(fstest.MapFS{"theme.json": {Data: []byte(source)}}, "theme.json")
		if err == nil {
			t.Fatalf("invalid theme was accepted: %s", source)
		}
	}
}

func TestGutterBackgroundInheritsEditorBackground(t *testing.T) {
	theme, err := LoadTheme(fstest.MapFS{"theme.json": {Data: []byte(`{"type":"light","colors":{"editor.background":"#f0f0f0"}}`)}}, "theme.json")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Color("editorGutter.background") != theme.Color("editor.background") {
		t.Fatal("gutter did not inherit the configured editor background")
	}
}

func TestCaptureDefinitions(t *testing.T) {
	definitions, err := ParseDefinitions([]byte(`{"captures":{"function":"entity.name.function","function.method":"entity.name.function.method"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if definitions.Scope("function.method.call") != "entity.name.function.method" || definitions.Scope("unknown") != "" {
		t.Fatal("capture prefix resolution is incorrect")
	}
}
