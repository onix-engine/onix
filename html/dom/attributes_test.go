package dom

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestAttributesMatchMapAcrossMutations(t *testing.T) {
	node := NewElement("div")
	want := make(map[string]string)
	random := rand.New(rand.NewPCG(11, 12))
	for step := range 3000 {
		name, value := fmt.Sprintf("data-%03d", random.IntN(128)), fmt.Sprint(step%7)
		switch random.IntN(4) {
		case 0:
			_, exists := want[name]
			if node.RemoveAttr(name) != exists {
				t.Fatal("removal returned the wrong change flag")
			}
			delete(want, name)
		case 1:
			expected, exists := want[name]
			if got, found := node.Attr(name); got != expected || found != exists {
				t.Fatalf("lookup %s = %q, %v; want %q, %v", name, got, found, expected, exists)
			}
		default:
			previous, exists := want[name]
			if node.SetAttr(name, value) != (!exists || previous != value) {
				t.Fatal("setting returned the wrong change flag")
			}
			want[name] = value
		}
		if node.AttrCount() != len(want) {
			t.Fatal("attribute count changed")
		}
		var previous string
		for name, value := range node.SortedAttributes() {
			if value != want[name] || name <= previous {
				t.Fatal("attribute order or contents changed")
			}
			previous = name
		}
	}
}

func TestAttributesBulkOwnershipAndReclamation(t *testing.T) {
	input := []Attribute{{"z", "last"}, {"a", "first"}, {"z", "duplicate"}, {"hidden", ""}}
	node := NewElement("div", input...)
	input[1].Value = "external edit"
	if node.AttrValue("a") != "first" || node.AttrValue("z") != "last" || node.AttrCount() != 3 {
		t.Fatal("bulk attributes borrowed mutable input or kept a duplicate")
	}
	if value, exists := node.Attr("hidden"); value != "" || !exists {
		t.Fatal("an empty boolean attribute became absent")
	}
	all := node.data.values[:cap(node.data.values)]
	node.SetAttributes(Attribute{"only", "value"})
	for _, attribute := range all[1:] {
		if attribute != (Attribute{}) {
			t.Fatal("bulk replacement retains retired strings")
		}
	}
	node.RemoveAttr("only")
	if !slices.Equal(all, make([]Attribute, len(all))) {
		t.Fatal("removal retains retired strings")
	}
}

func TestSharedAttributesSourceAndCloneIsolation(t *testing.T) {
	for _, change := range []func(*Node){
		func(n *Node) { n.SetAttr("class", "changed") },
		func(n *Node) { n.SetAttr("hidden", "") },
		func(n *Node) { n.RemoveAttr("class") },
		func(n *Node) { n.SetAttributes(Attribute{"id", "replacement"}) },
	} {
		source := NewElement("div", Attribute{"class", "original"})
		left, right := NewElement("div"), NewElement("div")
		left.ShareAttributes(source)
		right.ShareAttributes(source)
		change(source)
		change(left)
		if right.AttrValue("class") != "original" || right.AttrCount() != 1 {
			t.Fatal("a source or sibling mutation changed a shared clone")
		}
	}
}

func TestSharedAttributesConcurrentReadOnlyClones(t *testing.T) {
	source := NewElement("div", Attribute{"class", "original"})
	var tasks sync.WaitGroup
	for range 8 {
		tasks.Go(func() {
			for range 100 {
				clone := NewElement("div")
				clone.ShareAttributes(source)
				clone.SetAttr("class", "changed")
				if source.AttrValue("class") != "original" {
					t.Error("a concurrent clone changed its source")
				}
			}
		})
	}
	tasks.Wait()
}

func TestAttributesWarmMutationsAndIterationAllocs(t *testing.T) {
	node := NewElement("div", Attribute{"class", "original"}, Attribute{"hidden", ""})
	if allocations := testing.AllocsPerRun(100, func() {
		for name := range node.Attributes() {
			if name == "" {
				t.Fatal("empty attribute name")
			}
		}
		node.SetAttr("class", "changed")
		node.RemoveAttr("hidden")
		node.SetAttr("hidden", "")
		node.SetAttributes(Attribute{"class", "original"}, Attribute{"hidden", ""})
	}); allocations != 0 {
		t.Fatalf("warm attributes allocated %g", allocations)
	}
}

func TestAttributesPromotionAndRemovalDuringIteration(t *testing.T) {
	for _, count := range []int{3, 8, 32} {
		for _, shared := range []bool{false, true} {
			source := NewElement("div")
			for index := range count {
				source.SetAttr(fmt.Sprintf("data-%02d", index), "value")
			}
			node := source
			if shared {
				node = NewElement("div")
				node.ShareAttributes(source)
			}
			for name := range node.Attributes() {
				node.RemoveAttr(name)
			}
			if node.AttrCount() != 0 || shared && source.AttrCount() != count {
				t.Fatal("removal skipped attributes or changed a shared source")
			}
		}
	}
	node := NewElement("div")
	for index := range smallAttributeLimit {
		node.SetAttr(fmt.Sprintf("data-%02d", index), "value")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		node.SetAttr("overflow", "value")
		node.RemoveAttr("overflow")
	}); allocations != 0 {
		t.Fatalf("crossing the list/map boundary repeatedly allocated %g", allocations)
	}
}

func TestSharedLargeAttributesDetachOnMutation(t *testing.T) {
	attributes := make([]Attribute, 32)
	for index := range attributes {
		attributes[index] = Attribute{fmt.Sprintf("data-%02d", index), "original"}
	}
	source := NewElement("div", attributes...)
	left, right := NewElement("div"), NewElement("div")
	left.ShareAttributes(source)
	right.ShareAttributes(source)
	source.SetAttr("data-00", "source")
	left.RemoveAttr("data-01")
	if right.AttrValue("data-00") != "original" || right.AttrCount() != len(attributes) {
		t.Fatal("large-map edits escaped their node")
	}
	left.SetAttributes(Attribute{"only", "replacement"})
	if left.AttrCount() != 1 || left.AttrValue("only") != "replacement" || right.AttrCount() != len(attributes) {
		t.Fatal("replacing a large map retained old entries or changed a sibling")
	}
}

func TestParserLargeFormattingAttributesReuse(t *testing.T) {
	var source strings.Builder
	source.WriteString("<b")
	for index := range 32 {
		fmt.Fprintf(&source, " data-%02d=value", index)
	}
	source.WriteString("><p>bold<i>italic</b>after</i>")
	var parser Parser
	root, err := parser.Parse(source.String())
	if err != nil {
		t.Fatal(err)
	}
	want := root.String()
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := parser.Parse(source.String()); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("large formatting attributes allocated after reuse: %g", allocations)
	}
	if parser.builder.doc.String() != want {
		t.Fatal("formatting reconstruction changed the attributes")
	}
}

func TestParserAttributeMergingAndForeignNames(t *testing.T) {
	root, err := Parse(`<!doctype html><html id=first><html id=second lang=en><body class=first><body class=second title=extra><svg Z=last viewbox="0 0 1 1" xlink:href="#x" href="#y"><path/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	html := root.Children[0]
	body := html.Children[1]
	svg := body.Children[0]
	if html.AttrValue("id") != "first" || html.AttrValue("lang") != "en" || body.AttrValue("class") != "first" || body.AttrValue("title") != "extra" {
		t.Fatal("repeated html/body tags overwrote existing attributes or lost new ones")
	}
	if svg.AttrValue("viewBox") != "0 0 1 1" || svg.AttrValue("xlink:href") != "#x" || svg.AttrValue("href") != "#y" || svg.AttrValue("z") != "last" {
		t.Fatal("foreign canonical or qualified attribute names changed")
	}
}

func TestTokenizerAttributesRemainIndependent(t *testing.T) {
	tokenizer := NewTokenizer(`<div class=first><div class=second>`)
	first, _ := tokenizer.Next()
	second, _ := tokenizer.Next()
	second.SetAttr("class", "changed")
	if first.AttrValue("class") != "first" || second.AttrValue("class") != "changed" {
		t.Fatal("tokenizer reused a previously returned token's attributes")
	}
}
