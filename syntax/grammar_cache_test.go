package syntax

import (
	"runtime"
	"testing"

	"github.com/eugenioenko/textmate-go/grammars"
)

func TestGrammarArchiveContainsCatalog(t *testing.T) {
	files, err := grammarFiles()
	if err != nil {
		t.Fatal(err)
	}
	scopes := grammars.Scopes()
	if len(files.File) != len(scopes) {
		t.Fatalf("packed %d scopes, want %d", len(files.File), len(scopes))
	}
	for _, scope := range scopes {
		f, err := files.Open(scope)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

func TestGrammarCacheFollowsDocumentLifetime(t *testing.T) {
	var first, second Document
	const src = "# Title\n\n```go\npackage demo\n```\n"
	if err := first.Update("one.md", src); err != nil {
		t.Fatal(err)
	}
	if err := second.Update("two.md", src); err != nil {
		t.Fatal(err)
	}
	if first.grammar != second.grammar {
		t.Fatal("two live documents compiled the same grammar twice")
	}
	grammarCache.Lock()
	compiled := grammarCache.compiled["text.html.markdown"]
	raw := grammarCache.raw["text.html.markdown"]
	grammarCache.Unlock()
	first.Close()
	runtime.GC()
	if compiled.Value() != second.grammar || raw.Value() == nil {
		t.Fatal("active document lost its compiled grammar or raw rules")
	}
	second.Close()
	runtime.GC()
	if compiled.Value() != nil || raw.Value() != nil {
		t.Fatal("closed documents retained the grammar")
	}
}
