package dom

import "testing"

func TestSharedAttributesDetachOnMutation(t *testing.T) {
	attributes := map[string]string{"class": "scope", "hidden": ""}
	left, right := NewElement("td", nil), NewElement("td", nil)
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
	if left.Attributes["class"] != "changed" || right.Attributes["class"] != "scope" || attributes["class"] != "scope" {
		t.Fatal("class edit changed another node or the shared source")
	}
	if _, ok := left.Attr("hidden"); !ok {
		t.Fatal("attribute removal changed another node")
	}
	if _, ok := attributes["hidden"]; !ok {
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
	attributes := map[string]string{"class": "scope"}
	for _, child := range root.Children {
		child.ShareAttributes(attributes)
	}
	root, err = parser.Parse(`<p id="first"></p><p id="second"></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if attributes["class"] != "scope" || len(attributes) != 1 {
		t.Fatal("parser reset cleared or reused a shared attribute map")
	}
	if root.Children[0].Attributes["id"] != "first" || root.Children[1].Attributes["id"] != "second" {
		t.Fatal("parser reused shared storage for independent elements")
	}
}
