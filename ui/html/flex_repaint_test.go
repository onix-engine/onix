package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLFlexExpansionRetainsUnchangedControls(t *testing.T) {
	const prefix = `<style>.shell{display:flex;width:300px;height:160px}.tree{width:100px;flex-shrink:0}.tree button{display:flex;width:100px;height:30px}.tree span{overflow:hidden;white-space:nowrap;text-overflow:ellipsis}.chat{width:200px;overflow:auto}</style><div class="shell"><aside class="tree"><button id="file"><span id="filename">main.go</span></button><textarea id="editor">Source</textarea></aside><div id="chat" class="chat"><div id="region">`
	const details = `<button id="command">Command</button><div style="height:200px">Output</div>`
	const suffix = `</div><p>Reply</p></div></div>`
	world, owner := incrementalHTMLFixture(t, prefix+details+suffix)
	region := incrementalHTMLNode(t, world, owner, "region")
	command := incrementalHTMLNode(t, world, owner, "command")
	output := Node{world: world, owner: owner, node: region.node.Children[1]}
	file := incrementalControl(t, world, owner, "file")
	editor := incrementalControl(t, world, owner, "editor")
	beforeFile, _ := ecs.Get[Control](world, file)
	beforeEditor, _ := ecs.Get[Control](world, editor)
	input := ecs.GetMut[scene.InputState2D](world, file)
	input.Pressed, input.Hovered = true, true
	clicks := 0
	for range 3 {
		command.Remove()
		output.Remove()
		incrementalSync(t, world)
		assertHTMLContextMatchesFresh(t, world, owner, prefix+suffix)
		region.Append(command)
		region.Append(output)
		command.OnClick(func(*PointerEvent) { clicks++ })
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 1 {
			t.Fatalf("expansion refreshed unrelated controls: %+v", state.SyncStats)
		}
		afterFile, _ := ecs.Get[Control](world, file)
		afterEditor, _ := ecs.Get[Control](world, editor)
		input = ecs.GetMut[scene.InputState2D](world, file)
		if afterFile.RenderRevision != beforeFile.RenderRevision || afterEditor.RenderRevision != beforeEditor.RenderRevision || !input.Pressed || !input.Hovered {
			t.Fatal("expansion reset the explorer or editor")
		}
		scene.DispatchInputEvent(world, scene.InputEvent{Entity: incrementalControl(t, world, owner, "command"), Kind: scene.InputEventTap})
		assertHTMLContextMatchesFresh(t, world, owner, prefix+details+suffix)
	}
	if clicks != 3 {
		t.Fatal("expanded command lost its click binding")
	}
}

func TestHTMLFlexReflowRefreshesRetainedFocusContext(t *testing.T) {
	source := func(text string) string {
		return `<div style="display:flex;width:300px"><div id="focus" tabindex="0" style="width:100px;height:0"><div style="display:flex"><div id="target" style="width:50px">` + text + `</div><div>Sibling</div></div></div><div>Other</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("Long wrapping text outside the focus box"))
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, text := range []string{"Short", "Long wrapping text outside the focus box", "Short"} {
		target.SetText(text)
		incrementalSync(t, world)
		assertHTMLContextMatchesFresh(t, world, owner, source(text))
	}
}

func TestHTMLFlexExpansionUpdatesRetainedSiblingOrder(t *testing.T) {
	const prefix = `<div style="display:flex;width:300px;height:100px"><div id="region" style="width:100px;height:80px">`
	const child = `<div id="child">Details</div>`
	const suffix = `</div><button id="other" style="width:100px;height:30px">Unchanged</button></div>`
	world, owner := incrementalHTMLFixture(t, prefix+child+suffix)
	region := incrementalHTMLNode(t, world, owner, "region")
	details := incrementalHTMLNode(t, world, owner, "child")
	control := incrementalControl(t, world, owner, "other")
	before, _ := ecs.Get[Control](world, control)
	for range 3 {
		details.Remove()
		incrementalSync(t, world)
		assertHTMLPartialMatchesFresh(t, world, owner, prefix+suffix)
		region.Append(details)
		incrementalSync(t, world)
		assertHTMLPartialMatchesFresh(t, world, owner, prefix+child+suffix)
		after, _ := ecs.Get[Control](world, control)
		if after.RenderRevision != before.RenderRevision {
			t.Fatal("paint order update redrew the unchanged sibling")
		}
	}
}
