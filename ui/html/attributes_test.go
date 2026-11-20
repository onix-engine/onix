package html

import (
	"testing"

	"github.com/onix-engine/onix/html/parse"
	uinode "github.com/onix-engine/onix/scene/node"
)

func TestHTMLAttributeCloneIsolation(t *testing.T) {
	source, err := parse.Parse(`<div class=original><input checked type=checkbox></div>`)
	if err != nil {
		t.Fatal(err)
	}
	var nodes uinode.Arena
	left, right := nodes.Clone(source), nodes.Clone(source)
	source.Children[0].SetAttr("class", "source")
	left.Children[0].SetAttr("class", "left")
	left.Children[0].Children[0].RemoveAttr("checked")
	if right.Children[0].AttrValue("class") != "original" {
		t.Fatal("a clone or source edit modified its sibling")
	}
	if _, checked := right.Children[0].Children[0].Attr("checked"); !checked {
		t.Fatal("nested clone attributes lost isolation")
	}
}

func TestHTMLTemplateClonePreservesForeignNamespaces(t *testing.T) {
	source, err := parse.Parse(`<svg><foreignObject><div></div></foreignObject></svg><math><mi>x</mi></math>`)
	if err != nil {
		t.Fatal(err)
	}
	var nodes uinode.Arena
	doc := nodes.Clone(source)
	svg, math := doc.Children[0], doc.Children[1]
	foreign := svg.Children[0]
	if svg.Namespace != uinode.SVG || foreign.Namespace != uinode.SVG ||
		foreign.Children[0].Namespace != uinode.HTML || math.Namespace != uinode.MathML {
		t.Fatalf("template clone lost namespaces: %v %v %v %v", svg.Namespace, foreign.Namespace, foreign.Children[0].Namespace, math.Namespace)
	}
	if svg == source.Children[0] || svg.Parent != doc || foreign.Parent != svg {
		t.Fatal("clone shared nodes or retained source parents")
	}
}

func TestHTMLAttributeRemovalDoesNotSkipCompactedEntries(t *testing.T) {
	for _, keep := range [][]VNodeAttribute{nil, {{Name: "c", Value: "keep"}}} {
		world, owner := incrementalHTMLFixture(t, `<div id=target a b c d></div>`)
		node := incrementalHTMLNode(t, world, owner, "target").node
		clearHTMLComponentAttributes(world, owner, node, keep, "")
		_, retained := node.Attr("c")
		if node.AttrCount() != len(keep) || len(keep) != 0 && !retained {
			t.Fatal("attribute removal skipped an entry after compaction")
		}
	}
}
