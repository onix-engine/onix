package css

import (
	"testing"

	"github.com/diiyw/onix/html/dom"
)

func TestTokenizer(t *testing.T) {
	tokenizer := NewTokenizer(`#app .card, div { color: #fff; }`)

	var tokens []Token
	for {
		token, err := tokenizer.Next()
		if err != nil {
			t.Fatalf("next token: %v", err)
		}
		tokens = append(tokens, token)
		if token.Type == TokenEOF {
			break
		}
	}

	if got := tokens[0]; got.Type != TokenHash || got.Value != "app" {
		t.Fatalf("first token = %#v", got)
	}

	if got := tokens[2]; got.Type != TokenDot {
		t.Fatalf("third token = %#v", got)
	}

	if got := tokens[3]; got.Type != TokenIdent || got.Value != "card" {
		t.Fatalf("fourth token = %#v", got)
	}

	if got := tokens[8]; got.Type != TokenLBrace {
		t.Fatalf("lbrace token = %#v", got)
	}
}

func TestAttributeOperatorsAndGeneralSibling(t *testing.T) {
	stylesheet, err := Parse(`[data-kind^="Card" i] ~ .target { color: red; }`)
	if err != nil {
		t.Fatalf("parse selector: %v", err)
	}
	doc, err := dom.Parse(`<div data-kind="cardinal"></div><span class="target"></span><span class="target"></span>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	selector := stylesheet.Rules[0].Selectors[0]
	if !Match(selector, doc.Children[1]) || !Match(selector, doc.Children[2]) {
		t.Fatal("general sibling selector did not match both following elements")
	}
	if got := selector.String(); got != `[data-kind^="Card" i] ~ .target` {
		t.Fatalf("selector serialization = %q", got)
	}
}

func TestDeclarationRecoveryAndImportant(t *testing.T) {
	stylesheet, err := Parse(`.card { color: red; broken; width: 10px !important; height: 4px; }`)
	if err != nil {
		t.Fatalf("parse declaration block: %v", err)
	}
	if len(stylesheet.Rules) != 1 || len(stylesheet.Rules[0].Declarations) != 3 {
		t.Fatalf("recovered declarations = %#v", stylesheet.Rules[0].Declarations)
	}
	declaration := stylesheet.Rules[0].Declarations[1]
	if declaration.Value != "10px" || !declaration.Important {
		t.Fatalf("important declaration = %#v", declaration)
	}
}

func TestImportantWhitespaceCommentsAndQuotedValues(t *testing.T) {
	for _, test := range []struct {
		input, value string
		important    bool
	}{
		{"red ! important", "red", true},
		{"red ! /*flag*/ IMPORTANT /*tail*/", "red", true},
		{"red/*tail*/", "red", false},
		{`"!important"`, `"!important"`, false},
		{`"/*text*/"`, `"/*text*/"`, false},
		{`var(--x, !important)`, `var(--x, !important)`, false},
		{`red \!important`, `red \!important`, false},
	} {
		value, important := SplitImportant(test.input)
		if value != test.value || important != test.important {
			t.Fatalf("%q => %q %v, want %q %v", test.input, value, important, test.value, test.important)
		}
	}
}

func TestParseDeclarationValuePreservesNestedDelimiters(t *testing.T) {
	stylesheet, err := Parse(`.card { content: "a;b}"; background-image: url(data:image/svg+xml;base64,abc;def); color: red /* ; */; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	declarations := stylesheet.Rules[0].Declarations
	if got, want := declarations[0].Value, `"a;b}"`; got != want {
		t.Fatalf("content value = %q, want %q", got, want)
	}
	if got, want := declarations[1].Value, "url(data:image/svg+xml;base64,abc;def)"; got != want {
		t.Fatalf("background value = %q, want %q", got, want)
	}
	if got, want := declarations[2].Value, "red"; got != want {
		t.Fatalf("color value = %q, want %q", got, want)
	}
}

func TestSelectorValuesAreCaseSensitive(t *testing.T) {
	doc, err := dom.Parse(`<div id="Card" class="Hero"></div>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	node := doc.Children[0]

	for _, selector := range []string{"#Card.Hero", "#card.hero"} {
		stylesheet, err := Parse(selector + ` { color: red; }`)
		if err != nil {
			t.Fatalf("parse selector %q: %v", selector, err)
		}
		want := selector == "#Card.Hero"
		if got := Match(stylesheet.Rules[0].Selectors[0], node); got != want {
			t.Fatalf("match(%q) = %v, want %v", selector, got, want)
		}
	}
}

func TestAttributePresenceSelectors(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="hidden" hidden></div><div id="shown"></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	hidden := doc.Children[0].Children[1].Children[0]
	shown := doc.Children[0].Children[1].Children[1]

	stylesheet, err := Parse(`div[hidden] { display: none; } :not([hidden]) { color: red; }`)
	if err != nil {
		t.Fatalf("parse attribute selectors: %v", err)
	}
	if got, want := stylesheet.Rules[0].Selectors[0].String(), "div[hidden]"; got != want {
		t.Fatalf("attribute selector string = %q, want %q", got, want)
	}
	if !Match(stylesheet.Rules[0].Selectors[0], hidden) || Match(stylesheet.Rules[0].Selectors[0], shown) {
		t.Fatal("[hidden] selector did not match attribute presence")
	}
	if Match(stylesheet.Rules[1].Selectors[0], hidden) || !Match(stylesheet.Rules[1].Selectors[0], shown) {
		t.Fatal(":not([hidden]) selector did not match attribute absence")
	}
	if got, want := CalculateSpecificity(stylesheet.Rules[0].Selectors[0]), (Specificity{Class: 1, Tag: 1}); got != want {
		t.Fatalf("attribute selector specificity = %#v, want %#v", got, want)
	}
}

func TestEmptyAndPlaceholderWhitespaceSemantics(t *testing.T) {
	stylesheet, err := Parse(`div:empty, textarea:placeholder-shown { color: red; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	doc, err := dom.Parse("<div>\n</div><textarea placeholder=\"type\"> </textarea>")
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	if Match(stylesheet.Rules[0].Selectors[0], doc.Children[0]) {
		t.Fatal(":empty should not match an element containing whitespace text")
	}
	if Match(stylesheet.Rules[0].Selectors[1], doc.Children[1]) {
		t.Fatal(":placeholder-shown should not match a textarea containing whitespace text")
	}
}

func TestMalformedRuleDoesNotDiscardFollowingRules(t *testing.T) {
	stylesheet, err := Parse(`#a#b { color: red; } .valid { color: blue; }`)
	if err != nil {
		t.Fatalf("recover malformed rule: %v", err)
	}
	if len(stylesheet.Rules) != 1 || stylesheet.Rules[0].Selectors[0].Parts[0].Classes[0] != "valid" {
		t.Fatalf("rules after recovery = %#v", stylesheet.Rules)
	}
}

func TestUniversalSelector(t *testing.T) {
	stylesheet, err := Parse(`* { box-sizing: border-box; } *.card { color: red; }`)
	if err != nil {
		t.Fatalf("parse universal selector: %v", err)
	}

	if got, want := stylesheet.Rules[0].Selectors[0].String(), "*"; got != want {
		t.Fatalf("universal selector string = %q, want %q", got, want)
	}
	if got := CalculateSpecificity(stylesheet.Rules[0].Selectors[0]); got != (Specificity{}) {
		t.Fatalf("universal selector specificity = %#v, want zero", got)
	}
	if got, want := stylesheet.Rules[1].Selectors[0].String(), "*.card"; got != want {
		t.Fatalf("universal compound selector string = %q, want %q", got, want)
	}

	doc, err := dom.Parse(`<div class="card"></div><span></span>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	if !Match(stylesheet.Rules[0].Selectors[0], doc.Children[0]) || !Match(stylesheet.Rules[0].Selectors[0], doc.Children[1]) {
		t.Fatal("universal selector should match every element")
	}
	if !Match(stylesheet.Rules[1].Selectors[0], doc.Children[0]) || Match(stylesheet.Rules[1].Selectors[0], doc.Children[1]) {
		t.Fatal("universal compound selector should match only its class")
	}
}

func TestChildCombinatorSelector(t *testing.T) {
	stylesheet, err := Parse(`.parent>.direct { color: red; }`)
	if err != nil {
		t.Fatalf("parse child selector: %v", err)
	}
	selector := stylesheet.Rules[0].Selectors[0]
	if got, want := selector.String(), ".parent > .direct"; got != want {
		t.Fatalf("child selector string = %q, want %q", got, want)
	}

	doc, err := dom.Parse(`<div class="parent"><span class="direct"><i class="direct"></i></span></div>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	direct := doc.Children[0].Children[0]
	nested := direct.Children[0]
	if !Match(selector, direct) {
		t.Fatal("child selector should match direct child")
	}
	if Match(selector, nested) {
		t.Fatal("child selector should not match deeper descendant")
	}
}

func TestClassMatchingDoesNotAllocate(t *testing.T) {
	stylesheet, err := Parse(`.card { color: red; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	doc, err := dom.Parse(`<div class="card"></div>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	node := doc.Children[0]
	selector := stylesheet.Rules[0].Selectors[0]
	if allocations := testing.AllocsPerRun(100, func() {
		if !Match(selector, node) {
			t.Fatal("selector should match")
		}
	}); allocations != 0 {
		t.Fatalf("class matching allocations = %v, want 0", allocations)
	}
}

func TestParseAndMatchSelectors(t *testing.T) {
	stylesheet, err := Parse(`body .hero, #app.card { background-color: #fff; color: black; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	if len(stylesheet.Rules) != 1 {
		t.Fatalf("rule count = %d, want 1", len(stylesheet.Rules))
	}

	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 2 {
		t.Fatalf("selector count = %d, want 2", len(rule.Selectors))
	}

	if len(rule.Declarations) != 2 {
		t.Fatalf("declaration count = %d, want 2", len(rule.Declarations))
	}

	if got, want := rule.Selectors[0].String(), "body .hero"; got != want {
		t.Fatalf("selector string = %q, want %q", got, want)
	}

	doc, err := dom.Parse(`<html><body><div id="app" class="card hero"><span>hi</span></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	target := doc.Children[0].Children[1].Children[0]
	if !Match(rule.Selectors[0], target) {
		t.Fatalf("selector %q should match target node", rule.Selectors[0].String())
	}

	if !Match(rule.Selectors[1], target) {
		t.Fatalf("selector %q should match target node", rule.Selectors[1].String())
	}

	child := target.Children[0]
	if Match(rule.Selectors[1], child) {
		t.Fatalf("selector %q should not match child node", rule.Selectors[1].String())
	}
}

func TestParseConditionalRulesFlattensNestedQueries(t *testing.T) {
	stylesheet, err := Parse(`@media (min-width: 400px) { .wide { color: red; } @container sidebar (min-width: 200px) { .card { color: blue; } } }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	if len(stylesheet.Rules) != 2 {
		t.Fatalf("rule count = %d, want 2", len(stylesheet.Rules))
	}
	if got := stylesheet.Rules[0].Selectors[0].String(); got != ".wide" {
		t.Fatalf("rule 0 selector = %q", got)
	}
	if len(stylesheet.Rules[0].Conditions) != 1 || stylesheet.Rules[0].Conditions[0].Kind != RuleConditionMedia {
		t.Fatalf("rule 0 conditions = %#v", stylesheet.Rules[0].Conditions)
	}
	if len(stylesheet.Rules[1].Conditions) != 2 {
		t.Fatalf("rule 1 conditions = %#v, want nested media+container", stylesheet.Rules[1].Conditions)
	}
	if stylesheet.Rules[1].Conditions[0].Kind != RuleConditionMedia || stylesheet.Rules[1].Conditions[1].Kind != RuleConditionContainer {
		t.Fatalf("rule 1 conditions = %#v", stylesheet.Rules[1].Conditions)
	}
	if stylesheet.Rules[1].Conditions[1].Prelude != "sidebar (min-width: 200px)" {
		t.Fatalf("container prelude = %q", stylesheet.Rules[1].Conditions[1].Prelude)
	}
}

func TestCalculateSpecificity(t *testing.T) {
	selector := Selector{
		Parts: []CompoundSelector{
			{Tag: "body"},
			{
				Tag:     "div",
				ID:      "app",
				Classes: []string{"hero", "primary"},
				PseudoClasses: []PseudoClass{
					{Name: "disabled"},
					{Name: "first-child"},
					{Name: "not", Selectors: []Selector{{Parts: []CompoundSelector{{Classes: []string{"skip"}}}}}},
				},
			},
		},
	}

	got := CalculateSpecificity(selector)
	if got != (Specificity{ID: 1, Class: 5, Tag: 2}) {
		t.Fatalf("specificity = %#v", got)
	}
}

func TestParsePseudoClasses(t *testing.T) {
	stylesheet, err := Parse(`html:root body input:disabled:first-child, option:checked:only-of-type { color: #333; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	if len(stylesheet.Rules) != 1 || len(stylesheet.Rules[0].Selectors) != 2 {
		t.Fatalf("parsed selectors = %#v", stylesheet.Rules)
	}

	if got, want := stylesheet.Rules[0].Selectors[0].String(), "html:root body input:disabled:first-child"; got != want {
		t.Fatalf("selector string = %q, want %q", got, want)
	}
	second := stylesheet.Rules[0].Selectors[1].Parts[0]
	if len(second.PseudoClasses) != 2 || second.PseudoClasses[0].Name != "checked" || second.PseudoClasses[1].Name != "only-of-type" {
		t.Fatalf("second selector pseudo-classes = %#v", second.PseudoClasses)
	}
}

func TestMatchStaticPseudoClasses(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="empty"></div><div id="group"><input id="first" type="checkbox" disabled checked><span id="mid"></span><input id="last"></div><select><option id="chosen" selected>One</option></select><section><p id="solo">x</p></section></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	html := doc.Children[0]
	empty := html.Children[1].Children[0]
	group := html.Children[1].Children[1]
	first := group.Children[0]
	mid := group.Children[1]
	last := group.Children[2]
	chosen := html.Children[1].Children[2].Children[0]
	solo := html.Children[1].Children[3].Children[0]

	tests := []struct {
		selector string
		node     *dom.Node
		want     bool
	}{
		{selector: "html:root", node: html, want: true},
		{selector: "div:empty", node: empty, want: true},
		{selector: "input:disabled:checked:first-child", node: first, want: true},
		{selector: "input:enabled:last-child", node: last, want: true},
		{selector: "span:first-of-type:last-of-type:only-of-type", node: mid, want: true},
		{selector: "option:checked:only-child:only-of-type", node: chosen, want: true},
		{selector: "p:only-child", node: solo, want: true},
		{selector: "input:last-child", node: first, want: false},
		{selector: "div:empty", node: group, want: false},
	}

	for _, test := range tests {
		stylesheet, err := Parse(test.selector + ` { color: #000; }`)
		if err != nil {
			t.Fatalf("parse selector %q: %v", test.selector, err)
		}
		got := Match(stylesheet.Rules[0].Selectors[0], test.node)
		if got != test.want {
			t.Fatalf("match(%q) = %v, want %v", test.selector, got, test.want)
		}
	}
}

func TestParseAndMatchFunctionalPseudoClasses(t *testing.T) {
	stylesheet, err := Parse(`li:not(.skip):nth-child(2n+1), div:is(.card, .hero), p:where(.lead), span:nth-of-type(2) { color: #000; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	if got, want := stylesheet.Rules[0].Selectors[0].String(), "li:not(.skip):nth-child(2n+1)"; got != want {
		t.Fatalf("selector string = %q, want %q", got, want)
	}

	doc, err := dom.Parse(`<html><body><ul><li class="skip" id="a"></li><li id="b"></li><li id="c"></li></ul><div id="hero" class="hero"></div><p id="lead" class="lead"></p><section><span id="s1"></span><span id="s2"></span></section></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	body := doc.Children[0].Children[1]
	b := body.Children[0].Children[1]
	c := body.Children[0].Children[2]
	hero := body.Children[1]
	lead := body.Children[2]
	s2 := body.Children[3].Children[1]

	if Match(stylesheet.Rules[0].Selectors[0], b) {
		t.Fatal("nth-child(2n+1) should not match second li")
	}
	if !Match(stylesheet.Rules[0].Selectors[0], c) {
		t.Fatal("nth-child(2n+1) should match third li")
	}
	if !Match(stylesheet.Rules[0].Selectors[1], hero) {
		t.Fatal(":is should match hero")
	}
	if !Match(stylesheet.Rules[0].Selectors[2], lead) {
		t.Fatal(":where should match lead")
	}
	if !Match(stylesheet.Rules[0].Selectors[3], s2) {
		t.Fatal("nth-of-type should match second span")
	}
}

func TestMatchNthLastAndPlaceholderShownPseudoClasses(t *testing.T) {
	stylesheet, err := Parse(`li:nth-last-child(2), span:nth-last-of-type(1), input:placeholder-shown, textarea:placeholder-shown { color: #000; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	if got, want := stylesheet.Rules[0].Selectors[0].String(), "li:nth-last-child(2)"; got != want {
		t.Fatalf("selector string = %q, want %q", got, want)
	}

	doc, err := dom.Parse(`<html><body><ul><li id="a"></li><li id="b"></li><li id="c"></li></ul><div><span id="s1"></span><em></em><span id="s2"></span></div><input id="shown" placeholder="Type here"><input id="filled" placeholder="Type here" value="x"><textarea id="area" placeholder="Describe"></textarea><textarea id="done" placeholder="Describe">ok</textarea></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	body := doc.Children[0].Children[1]
	b := body.Children[0].Children[1]
	s2 := body.Children[1].Children[2]
	shown := body.Children[2]
	filled := body.Children[3]
	area := body.Children[4]
	done := body.Children[5]

	if !Match(stylesheet.Rules[0].Selectors[0], b) {
		t.Fatal("nth-last-child(2) should match second li")
	}
	if !Match(stylesheet.Rules[0].Selectors[1], s2) {
		t.Fatal("nth-last-of-type(1) should match last span")
	}
	if !Match(stylesheet.Rules[0].Selectors[2], shown) {
		t.Fatal("placeholder-shown should match empty input with placeholder")
	}
	if Match(stylesheet.Rules[0].Selectors[2], filled) {
		t.Fatal("placeholder-shown should not match filled input")
	}
	if !Match(stylesheet.Rules[0].Selectors[3], area) {
		t.Fatal("placeholder-shown should match empty textarea")
	}
	if Match(stylesheet.Rules[0].Selectors[3], done) {
		t.Fatal("placeholder-shown should not match textarea with content")
	}
}

func TestMatchContextCachesChildIndicesPerParent(t *testing.T) {
	doc, err := dom.Parse(`<html><body><ul><li id="a"></li><span id="middle"></span><li id="b"></li><li id="c"></li></ul></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	list := doc.Children[0].Children[1].Children[0]
	selectorSheet, err := Parse(`li:nth-child(n), li:nth-last-child(n), li:nth-of-type(n), li:nth-last-of-type(n) { color: #000; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	ctx := NewMatchContext()
	for _, child := range list.Children {
		if child.Type != dom.ElementNode || child.Data != "li" {
			continue
		}
		for _, selector := range selectorSheet.Rules[0].Selectors {
			if !MatchWithStateAndContext(selector, child, nil, ctx) {
				t.Fatalf("selector %q did not match %s", selector.String(), child.Data)
			}
		}
	}
	globalForward, globalReverse := []int{1, 3, 4}, []int{4, 2, 1}
	for index, child := range []*dom.Node{list.Children[0], list.Children[2], list.Children[3]} {
		data := ctx.nodes[child]
		if data.indexFlags&childIndexLoaded == 0 || int(data.childIndex) != globalForward[index] || data.indexFlags&childReverseLoaded == 0 || int(data.childReverse) != globalReverse[index] {
			t.Fatalf("cached child position %d = %+v", index, data)
		}
		if data.indexFlags&typeIndexLoaded == 0 || int(data.typeIndex) != index+1 || data.indexFlags&typeReverseLoaded == 0 || int(data.typeReverse) != 3-index {
			t.Fatalf("cached type position %d = %+v", index, data)
		}
	}
}

func TestAuthoredAttributesDoNotCreateInputPseudoClasses(t *testing.T) {
	stylesheet, err := Parse(`button:focus, div:focus-within, a:hover, button:active { color: #000; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	doc, err := dom.Parse(`<html><body><button id="focus" data-focus>ok</button><div id="wrap"><span><input id="inside" data-focus></span></div><a id="hovered" data-hover>link</a><button id="pressed" data-active>go</button><div id="plain"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	body := doc.Children[0].Children[1]
	focus := body.Children[0]
	wrap := body.Children[1]
	hovered := body.Children[2]
	pressed := body.Children[3]
	plain := body.Children[4]

	if Match(stylesheet.Rules[0].Selectors[0], focus) {
		t.Fatal(":focus matched data-focus without document focus")
	}
	if Match(stylesheet.Rules[0].Selectors[1], wrap) {
		t.Fatal(":focus-within matched data-focus without document focus")
	}
	if Match(stylesheet.Rules[0].Selectors[2], hovered) {
		t.Fatal(":hover matched data-hover without pointer state")
	}
	if Match(stylesheet.Rules[0].Selectors[3], pressed) {
		t.Fatal(":active matched data-active without activation state")
	}
	if Match(stylesheet.Rules[0].Selectors[1], plain) {
		t.Fatal(":focus-within should not match unfocused subtree")
	}
}

func TestMatchInputPseudoClassesWithExplicitState(t *testing.T) {
	stylesheet, err := Parse(`button:focus, div:focus-within, a:hover, button:active { color: #000; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	doc, err := dom.Parse(`<html><body><button id="focus">ok</button><div id="wrap"><span><input id="inside"></span></div><a id="hovered">link</a><button id="pressed">go</button></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	body := doc.Children[0].Children[1]
	focus := body.Children[0]
	wrap := body.Children[1]
	inside := wrap.Children[0].Children[0]
	hovered := body.Children[2]
	pressed := body.Children[3]
	state := &State{Focused: inside, Hovered: hovered, Active: pressed}

	if MatchWithState(stylesheet.Rules[0].Selectors[0], focus, state) {
		t.Fatal(":focus matched an element other than the focus target")
	}
	if !MatchWithState(stylesheet.Rules[0].Selectors[1], wrap, state) {
		t.Fatal(":focus-within should match explicit state")
	}
	if !MatchWithState(stylesheet.Rules[0].Selectors[2], hovered, state) {
		t.Fatal(":hover should match explicit state")
	}
	if !MatchWithState(stylesheet.Rules[0].Selectors[3], pressed, state) {
		t.Fatal(":active should match explicit state")
	}
	ancestorRule, err := Parse(`:hover { color: #000; }`)
	if err != nil {
		t.Fatalf("parse ancestor hover selector: %v", err)
	}
	if !MatchWithState(ancestorRule.Rules[0].Selectors[0], body, &State{Hovered: hovered}) {
		t.Fatal(":hover should match an ancestor of the hover target")
	}
}

func TestAdjacentSelectorAndAncestorBacktracking(t *testing.T) {
	for _, tc := range []struct {
		selector, markup string
		want             bool
	}{
		{"div + span", `<div></div>text<!-- comment --><span id="target"></span>`, true},
		{"div+span", `<div></div><b></b><span id="target"></span>`, false},
		{"div + span + b", `<div></div><span></span><b id="target"></b>`, true},
		{"main > div span", `<main><div><div><span id="target"></span></div></div></main>`, true},
		{"div + span", `<div><span id="target"></span></div>`, false},
	} {
		t.Run(tc.selector+tc.markup, func(t *testing.T) {
			doc, err := dom.Parse(tc.markup)
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := Parse(tc.selector + " { color:red }")
			if err != nil {
				t.Fatal(err)
			}
			var target *dom.Node
			var visit func(*dom.Node)
			visit = func(n *dom.Node) {
				if id, _ := n.Attr("id"); id == "target" {
					target = n
				}
				for _, c := range n.Children {
					visit(c)
				}
			}
			visit(doc)
			if got := Match(sheet.Rules[0].Selectors[0], target); got != tc.want {
				t.Fatalf("match=%v want %v", got, tc.want)
			}
			if _, err := Parse(sheet.String()); err != nil {
				t.Fatal(err)
			}
			if allocations := testing.AllocsPerRun(100, func() { Match(sheet.Rules[0].Selectors[0], target) }); allocations != 0 {
				t.Fatalf("match allocated %v", allocations)
			}
		})
	}
}
