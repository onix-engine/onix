package html

import "testing"

func TestHTMLAnimatedBackgroundReflowRetainsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>@keyframes fade{from{opacity:0}to{opacity:1}}.row{display:flex;height:40px}.dot{width:2px;height:2px;background:red;animation:fade 1s linear infinite}</style><div class="row"><span id="title">Chat</span><div id="dot" class="dot"></div></div><input id="other" value="draft">`)
	title := incrementalHTMLNode(t, world, owner, "title")
	dot := incrementalHTMLNode(t, world, owner, "dot")
	other := incrementalControl(t, world, owner, "other")
	before, _ := world.Get[Control](other)
	for _, text := range []string{"Generated conversation title", "Short", "Another title"} {
		title.SetText(text)
		incrementalSync(t, world)
		state := world.GetRef[htmlRenderState](owner)
		after, _ := world.Get[Control](other)
		if d := Diagnostics(world, owner); d.Path != SyncPartial || after.RenderRevision != before.RenderRevision {
			t.Fatalf("animated background forced full repaint: %+v", d)
		}
		if state.BoxRenderings[dot.node].Composite || state.Textures.rasters[dot.node] != nil {
			t.Fatal("single background retained an unnecessary composite")
		}
	}
}
