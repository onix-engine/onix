package syntax

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

func TestDocumentRetypedMethodMatchesFreshParse(t *testing.T) {
	const prefix = "package demo\nfunc (app *App) Frame() error {\n\t_, err := app"
	const suffix = "\n\treturn err\n}\n"
	theme := testTheme(t, "monokai")
	for _, edits := range [][]string{
		{".Step()", "", ".Step()"},
		{".Step()", ".Step(", ".Step", ".Ste", ".St", ".S", ".", "", ".", ".S", ".St", ".Ste", ".Step", ".Step(", ".Step()"},
	} {
		var document Document
		for _, edit := range edits {
			source := prefix + edit + suffix
			if err := document.Update("main.go", source); err != nil {
				t.Fatal(err)
			}
			var fresh Document
			if err := fresh.Update("main.go", source); err != nil {
				t.Fatal(err)
			}
			actual, expected := document.Tokens(theme, nil), fresh.Tokens(theme, nil)
			if !slices.Equal(actual, expected) {
				t.Errorf("retyping %q changed captures:\nactual: %+v\nexpected: %+v", edit, actual, expected)
			}
			fresh.Close()
		}
		document.Close()
	}
}

func TestDocumentLargeGoFileHighlightsThroughEnd(t *testing.T) {
	var source strings.Builder
	source.WriteString("package demo\n")
	for index := range 3000 {
		fmt.Fprintf(&source, "func Value%d() string { return \"value%d\" }\n", index, index)
	}
	content := source.String()
	var document Document
	defer document.Close()
	if err := document.Update("large.go", content); err != nil {
		t.Fatal(err)
	}
	if document.Tree() == nil || document.Tree().ParseStoppedEarly() {
		t.Fatal("large Go file did not finish parsing")
	}
	tokens := document.Tokens(testTheme(t, "monokai"), nil)
	for _, literal := range []string{"\"value0\"", "\"value1500\"", "\"value2999\""} {
		offset := strings.Index(content, literal)
		found := false
		for _, token := range tokens {
			if token.Start <= offset && offset < token.End {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("large Go file lost highlighting for %s", literal)
		}
	}
}

type delayedSyntaxTokenSource struct{}

func (delayedSyntaxTokenSource) Next() gotreesitter.Token {
	time.Sleep(2 * time.Millisecond)
	return gotreesitter.Token{}
}

func TestDocumentInterruptedParseCanRetrySameSource(t *testing.T) {
	for _, filename := range []string{"main.go", "other.go"} {
		t.Run(filename, func(t *testing.T) {
			var document Document
			defer document.Close()
			if err := document.Update("main.go", "package demo\nvar Before = 1\n"); err != nil {
				t.Fatal(err)
			}
			// Force a timeout during both an incremental edit and a new file.
			gotreesitter.WithHighlighterTimeoutMicros(1)(document.highlighter)
			gotreesitter.WithTokenSourceFactory(func([]byte) gotreesitter.TokenSource {
				return delayedSyntaxTokenSource{}
			})(document.highlighter)
			const source = "package demo\nvar After = \"complete\"\n"
			if err := document.Update(filename, source); !errors.Is(err, gotreesitter.ErrParseStoppedEarly) {
				t.Fatalf("interrupted parse error = %v", err)
			}
			if document.initialized || document.Tree() != nil || len(document.Tokens(testTheme(t, "monokai"), nil)) != 0 {
				t.Fatal("interrupted parse retained a successful snapshot or partial highlights")
			}
			if err := document.Update(filename, source); err != nil {
				t.Fatal(err)
			}
			if document.Tree() == nil || document.Tree().ParseStoppedEarly() || len(document.Tokens(testTheme(t, "monokai"), nil)) == 0 {
				t.Fatal("retrying the same source reused the interrupted parse")
			}
		})
	}
}
