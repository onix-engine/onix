package syntax

import (
	"runtime"
	"testing"
	"testing/synctest"
	"time"
	"weak"

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

func TestRecentlyClosedGrammarExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer ReleaseGrammarCache()
		var doc Document
		open := func(name string) {
			t.Helper()
			if err := doc.Update(name, "var value = 1"); err != nil {
				t.Fatal(err)
			}
		}
		open("one.go")
		first := weak.Make(doc.grammar)
		doc.CloseKeepingGrammar(time.Second)
		runtime.GC()
		open("two.go")
		if first.Value() != doc.grammar {
			t.Fatal("reopening recompiled a warm grammar")
		}
		open("other.css")
		second := weak.Make(doc.grammar)
		doc.CloseKeepingGrammar(2 * time.Second)
		runtime.GC()
		if first.Value() != nil {
			t.Fatal("second language kept the previous grammar warm")
		}
		time.Sleep(time.Second)
		runtime.GC()
		if second.Value() == nil {
			t.Fatal("replacement grammar expired early")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		runtime.GC()
		if second.Value() != nil {
			t.Fatal("idle grammar survived its grace period")
		}
	})
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
