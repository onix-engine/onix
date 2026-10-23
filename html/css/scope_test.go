package css

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/html/dom"
)

func TestScopeStylesheetKeepsDocumentAncestors(t *testing.T) {
	got, err := ScopeStylesheet(`body .card, :root .card, .card:not(.disabled) { color: red; }`, "onix-scope-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{
		"body .card.onix-scope-test",
		":root .card.onix-scope-test",
		".card.onix-scope-test:not(.disabled)",
	} {
		if !strings.Contains(got, selector) {
			t.Fatalf("scoped CSS %q does not contain %q", got, selector)
		}
	}
	if strings.Contains(got, "body.onix-scope-test") || strings.Contains(got, ":root.onix-scope-test") {
		t.Fatalf("scope leaked into document ancestors: %q", got)
	}
}

func TestScopeStylesheetSubjectClassMatchesComponentElement(t *testing.T) {
	got, err := ScopeStylesheet(`body .card:is(.active, .selected) { color: red; }`, "onix-scope-test")
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	document := dom.NewDocument()
	body := dom.NewElement("body", nil)
	card := dom.NewElement("div", map[string]string{"class": "card active onix-scope-test"})
	document.AppendChild(body)
	body.AppendChild(card)
	if !Match(stylesheet.Rules[0].Selectors[0], card) {
		t.Fatalf("scoped selector %q does not match component element", stylesheet.Rules[0].Selectors[0].String())
	}
}
