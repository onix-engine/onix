package html

import (
	"testing"

	"github.com/onix-engine/onix/html/dom"
)

func TestHTMLAttributeCloneIsolation(t *testing.T) {
	source, err := dom.Parse(`<div class=original><input checked type=checkbox></div>`)
	if err != nil {
		t.Fatal(err)
	}
	left, right := cloneHTMLNode(source, nil), cloneHTMLNode(source, nil)
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
