package html

import (
	"strings"
	"testing"
)

func TestHTMLTitleAndVisibilityUpdatesRetainUnrelatedControls(t *testing.T) {
	const prefix = `<div style="height:40px"><button id="target"><span id="icon">Icon</span></button></div>`
	const suffix = `<aside><input id="other" value="draft"/></aside>`
	for _, attribute := range []struct{ id, name, value string }{
		{"target", "title", "Close in background"},
		{"icon", "style", "visibility:hidden"},
		{"target", "style", "visibility:hidden"},
	} {
		t.Run(attribute.id+"/"+attribute.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, prefix+suffix)
			node := incrementalHTMLNode(t, world, owner, attribute.id)
			other := incrementalControl(t, world, owner, "other")
			before, _ := world.Get[Control](other)
			for _, show := range []bool{false, true} {
				source := prefix + suffix
				if show {
					node.RemoveAttribute(attribute.name)
				} else {
					node.SetAttribute(attribute.name, attribute.value)
					id := `id="` + attribute.id + `"`
					source = strings.Replace(source, id, id+` `+attribute.name+`="`+attribute.value+`"`, 1)
				}
				incrementalSync(t, world)
				after, _ := world.Get[Control](other)
				if d := Diagnostics(world, owner); d.Path == SyncFull || after.RenderRevision != before.RenderRevision {
					t.Fatalf("attribute refreshed unrelated control: %+v, revision %d -> %d", d, before.RenderRevision, after.RenderRevision)
				}
				assertHTMLContextMatchesFresh(t, world, owner, source)
			}
		})
	}
}
