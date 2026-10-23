package syntax

import "testing"

func TestDocumentPlainTextTransitions(t *testing.T) {
	var document Document
	defer document.Close()
	theme := testTheme(t, "dark-plus")
	for _, filename := range []string{"main.go", "notes.txt", "main.go", "NOTES.TXT", "main.go"} {
		if err := document.Update(filename, "package main\nvar Message = \"hello\"\n"); err != nil {
			t.Fatal(err)
		}
		tokens := document.Tokens(theme, nil)
		if filename == "main.go" {
			if document.Language() != "go" || document.Tree() == nil || len(tokens) == 0 {
				t.Fatalf("Go parsing was not restored after plain text: %s", filename)
			}
		} else if document.Language() != "" || document.Tree() != nil || document.highlighter != nil || len(tokens) != 0 {
			t.Fatalf("plain text retained a parser or highlights: %s", filename)
		}
	}
}

func TestDocumentNamedTextLanguages(t *testing.T) {
	for _, fixture := range []struct{ filename, language, source string }{
		{"CMakeLists.txt", "cmake", "project(example)\n"},
		{"vimdoc", "vimdoc", "*help-topic*\nSome help.\n"},
	} {
		t.Run(fixture.filename, func(t *testing.T) {
			var document Document
			defer document.Close()
			if err := document.Update(fixture.filename, fixture.source); err != nil {
				t.Fatal(err)
			}
			if document.Language() != fixture.language || document.Tree() == nil {
				t.Fatalf("named language %s lost its grammar", fixture.filename)
			}
		})
	}
}
