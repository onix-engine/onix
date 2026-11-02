package syntax

import (
	"fmt"
	"slices"
	"strings"
	"testing"
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
	if document.grammar == nil || len(document.lines) != 3002 {
		t.Fatal("large Go file did not finish tokenizing")
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
