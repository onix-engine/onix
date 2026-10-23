package style

import (
	"testing"

	"github.com/diiyw/onix/html/css"
)

func TestMutationDependenciesSeparateLocalSiblingRules(t *testing.T) {
	stylesheet, err := css.Parse(`.trigger + .target, li:not(:nth-child(2)) { color: red; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	cache := MutationCache{}
	if cache.NeedsDocumentStyle(stylesheet) {
		t.Fatal("sibling and positional selectors should not force document style recomputation")
	}
	if !cache.NeedsSiblingStyle(stylesheet) {
		t.Fatal("sibling and positional selectors need sibling-aware invalidation")
	}
}

func TestMutationDependenciesKeepConditionalRulesDocumentWide(t *testing.T) {
	stylesheet, err := css.Parse(`@container (min-width: 200px) { .target { color: red; } }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	cache := MutationCache{}
	if !cache.NeedsDocumentStyle(stylesheet) {
		t.Fatal("conditional rules must retain conservative document invalidation")
	}
}
