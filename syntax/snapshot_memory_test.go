package syntax

import (
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/text/piecetree"
)

func TestSnapshotIncrementalHighlightAndShrink(t *testing.T) {
	source := "package demo\r\n" + strings.Repeat("var 名字 = 42 // 中🙂\n", 10000)
	snapshot := piecetree.FromString(source)
	var doc Document
	defer doc.Close()
	if err := doc.Update("main.go", snapshot); err != nil {
		t.Fatal(err)
	}
	offset := strings.Index(source, "42")
	edited := snapshot.Replace(offset, offset+2, "123")
	if err := doc.UpdateRange("main.go", edited, offset, offset+2, offset+3); err != nil {
		t.Fatal(err)
	}
	var fresh Document
	defer fresh.Close()
	if err := fresh.Update("main.go", source[:offset]+"123"+source[offset+2:]); err != nil {
		t.Fatal(err)
	}
	if edited.Materialized() || !slices.Equal(doc.Captures(), fresh.Captures()) || doc.retokenized > 2 {
		t.Fatal("snapshot failed to reuse the unchanged suffix")
	}
	theme, _ := BuiltinTheme("monokai")
	doc.Tokens(theme, nil)
	if err := doc.Update("main.go", "package demo\n"); err != nil {
		t.Fatal(err)
	}
	doc.Tokens(theme, nil)
	for _, capacity := range []int{cap(doc.ranges), cap(doc.scratch), cap(doc.lines), cap(doc.lineScratch), cap(doc.tokens), cap(doc.painter.tokens)} {
		if capacity > 4096 {
			t.Fatalf("short document retained %d entries", capacity)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { doc.Update("main.go", "package demo\n"); doc.Tokens(theme, nil) }); allocs != 0 {
		t.Fatalf("unchanged highlight allocated %g", allocs)
	}
}

func TestSnapshotLongLineKeepsFallbackAndLexicalState(t *testing.T) {
	base := "package demo\n/* comment\n" + strings.Repeat("中", 10000) + "*/\nvar value=42\n"
	offset := strings.Index(base, "中") + 6000
	edited := piecetree.FromString(base).Replace(offset, offset+3, "文")
	var snapshot, flat Document
	defer snapshot.Close()
	defer flat.Close()
	if err := snapshot.Update("main.go", edited); err != nil {
		t.Fatal(err)
	}
	if err := flat.Update("main.go", base[:offset]+"文"+base[offset+3:]); err != nil {
		t.Fatal(err)
	}
	if edited.Materialized() || !slices.Equal(snapshot.Captures(), flat.Captures()) {
		t.Fatal("fragmented long-line fallback changed lexical state or flattened its source")
	}
}
