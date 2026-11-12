package dom

import "testing"

func TestSharedAttributesDetachOnMutation(t *testing.T) {
	attributes := NewElement("td", Attribute{"class", "scope"}, Attribute{"hidden", ""})
	left, right := NewElement("td"), NewElement("td")
	left.ShareAttributes(attributes)
	right.ShareAttributes(attributes)
	if allocations := testing.AllocsPerRun(100, func() {
		if left.SetAttr("class", "scope") || left.RemoveAttr("missing") {
			t.Fatal("unchanged attributes reported a mutation")
		}
	}); allocations != 0 {
		t.Fatalf("unchanged shared attributes allocated %g", allocations)
	}
	if !left.SetAttr("class", "changed") || !right.RemoveAttr("hidden") {
		t.Fatal("attribute mutations were lost")
	}
	if left.AttrValue("class") != "changed" || right.AttrValue("class") != "scope" || attributes.AttrValue("class") != "scope" {
		t.Fatal("class edit changed another node or the shared source")
	}
	if _, ok := left.Attr("hidden"); !ok {
		t.Fatal("attribute removal changed another node")
	}
	if _, ok := attributes.Attr("hidden"); !ok {
		t.Fatal("attribute removal changed the shared source")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		left.SetAttr("class", "scope")
		left.SetAttr("class", "changed")
		right.SetAttr("hidden", "")
		right.RemoveAttr("hidden")
	}); allocations != 0 {
		t.Fatalf("detached attribute updates allocated %g", allocations)
	}
}

func TestParserResetReleasesSharedAttributes(t *testing.T) {
	var parser Parser
	root, err := parser.Parse(`<p></p><p></p>`)
	if err != nil {
		t.Fatal(err)
	}
	attributes := NewElement("p", Attribute{"class", "scope"})
	for _, child := range root.Children {
		child.ShareAttributes(attributes)
	}
	root, err = parser.Parse(`<p id="first"></p><p id="second"></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if attributes.AttrValue("class") != "scope" || attributes.AttrCount() != 1 {
		t.Fatal("parser reset cleared or reused shared attribute storage")
	}
	if root.Children[0].AttrValue("id") != "first" || root.Children[1].AttrValue("id") != "second" {
		t.Fatal("parser reused shared storage for independent elements")
	}
}
