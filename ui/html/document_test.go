package html

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/element"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/internal/reuse"
	uinode "github.com/onix-engine/onix/scene/node"
)

func TestHTMLDocumentReclaimsRetainedMapsAfterShrink(t *testing.T) {
	source := func(rows int, color string) string {
		return `<p id="target" style="color:` + color + `">Target</p>` + strings.Repeat("<p>Row</p>", rows)
	}
	world, owner := incrementalHTMLFixture(t, source(512, "blue"))
	mapIdentities := func() [4]unsafe.Pointer {
		state := world.GetRef[htmlRenderState](owner)
		return [4]unsafe.Pointer{
			reflect.ValueOf(state.AnimationBaselines).UnsafePointer(),
			reflect.ValueOf(state.OverflowExtents).UnsafePointer(),
			reflect.ValueOf(state.BoxRenderings).UnsafePointer(),
			reflect.ValueOf(state.Styles).UnsafePointer(),
		}
	}
	large := mapIdentities()
	world.GetMut[Surface](owner).Value = source(512, "red")
	incrementalSync(t, world)
	if got := mapIdentities(); got != large {
		t.Fatal("same-size replacement discarded warm tables")
	}
	world.GetMut[Surface](owner).Value = source(4, "blue")
	for range reuse.RetentionFrames {
		incrementalSync(t, world)
	}
	for i, got := range mapIdentities() {
		if got == nil || got == large[i] {
			t.Fatalf("table %d did not reclaim its old storage", i)
		}
	}
	target := incrementalHTMLNode(t, world, owner, "target")
	target.SetText("Updated")
	incrementalSync(t, world)
	assertHTMLPartialMatchesFresh(t, world, owner, strings.Replace(source(4, "blue"), "Target", "Updated", 1))
}

func TestHTMLDocumentInitializesIndexAndFormsAfterMetadata(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<!doctype html><html><head><style>.x{color:red}</style></head><body>
		<div id="same" class="x">First</div><div id="same">Second</div>
		<select id="select"><option>A</option><option selected>B</option></select>
		<input id="radio-a" type="radio" name="g" checked><input id="radio-b" type="radio" name="g" checked>
		<style>.x{color:blue}</style></body></html>`)
	first := incrementalHTMLNode(t, world, owner, "same")
	selectNode := incrementalHTMLNode(t, world, owner, "select")
	firstRadio := incrementalHTMLNode(t, world, owner, "radio-a")
	lastRadio := incrementalHTMLNode(t, world, owner, "radio-b")
	state := world.GetRef[htmlRenderState](owner)
	if first.node.Children[0].Data != "First" || first.node.Children[0].Order != first.node.Order+1 ||
		state.Styles[first.node].Color != style.ColorValue(style.Color{B: 255, A: 255}) {
		t.Fatal("metadata changed document order, duplicate-ID lookup, or the cascade")
	}
	options := selectNode.node.Children
	if element.IsChecked(options[0]) || !element.IsChecked(options[1]) ||
		element.IsChecked(firstRadio.node) || !element.IsChecked(lastRadio.node) {
		t.Fatal("form state was not normalized on the retained tree")
	}
}

func TestHTMLCompactDOMReleasesSourceAndRetainsStyles(t *testing.T) {
	source := "<!--" + strings.Repeat("discarded", 32768) + `--><style>#live{color:red}</style><p id="live" data-value="shared">shared</p><p data-value="shared">shared</p>`
	world := ecs.NewWorld()
	spawner := world.NewSpawner1[Surface]()
	surface := New(source)
	surface.CompactDOM = true
	owner := spawner.Spawn(surface)
	// Root can initialize a document before the surface renderer sees it.
	live := incrementalHTMLNode(t, world, owner, "live")
	incrementalSync(t, world)
	state := world.GetRef[htmlRenderState](owner)
	if world.GetRef[Surface](owner).Value != "" || state.Source != "" || state.Key.Value != "" ||
		state.Styles[live.node].Color != style.ColorValue(style.Color{R: 255, A: 255}) {
		t.Fatal("compact document retained the authored source or lost embedded CSS")
	}
	inputStart := uintptr(unsafe.Pointer(unsafe.StringData(source)))
	for _, value := range []string{live.node.Data, live.node.AttrValue("id"), live.node.AttrValue("data-value"), live.node.Children[0].Data, state.EmbeddedCSS[0]} {
		start := uintptr(unsafe.Pointer(unsafe.StringData(value)))
		if start >= inputStart && start < inputStart+uintptr(len(source)) {
			t.Fatalf("value %q still borrows the input buffer", value)
		}
	}
	if unsafe.StringData(live.node.Children[0].Data) != unsafe.StringData(live.node.AttrValue("data-value")) {
		t.Fatal("equal live strings did not share the compact backing store")
	}
	live.SetText("Edited")
	incrementalSync(t, world)
	if current := incrementalHTMLNode(t, world, owner, "live"); current.node != live.node || current.node.Children[0].Data != "Edited" {
		t.Fatal("sync replaced a compact node or lost a live edit")
	}
	live.SetText("Pending")
	Set(world, owner, `<p id="new">Replacement</p>`)
	incrementalSync(t, world)
	incrementalHTMLNode(t, world, owner, "new")
	if _, found := NodeByID(world, owner, "live"); found {
		t.Fatal("replacement retained an old id")
	}
	Set(world, owner, "")
	incrementalSync(t, world)
	if _, found := NodeByID(world, owner, "new"); found {
		t.Fatal("an explicit empty source did not clear the compact document")
	}
}

func TestHTMLSetReplacesOwnedDocument(t *testing.T) {
	world := ecs.NewWorld()
	document := uinode.NewDocument()
	original := uinode.NewElement("p", uinode.Attribute{Name: "id", Value: "original"})
	document.AppendChild(original)
	spawner := world.NewSpawner1[Surface]()
	owner := spawner.Spawn(Surface{Document: document})
	node := incrementalHTMLNode(t, world, owner, "original")
	if node.node != original {
		t.Fatal("owned node was copied")
	}
	node.SetText("local edit")
	incrementalSync(t, world)
	if state := world.GetRef[htmlRenderState](owner); state.Root != document || original.Children[0].Data != "local edit" {
		t.Fatal("local edit replaced the supplied document")
	}
	Set(world, owner, `<p id="replacement">new</p>`)
	incrementalSync(t, world)
	incrementalHTMLNode(t, world, owner, "replacement")
	if _, ok := NodeByID(world, owner, "original"); ok || world.GetRef[Surface](owner).Document != nil {
		t.Fatal("source replacement retained the supplied document")
	}
}
