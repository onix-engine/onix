package html

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func incrementalHTMLFixture(tb testing.TB, source string) (*ecs.World, ecs.Entity) {
	tb.Helper()
	world := ecs.NewWorld()
	value := New(source)
	value.Width, value.Height, value.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, value)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		tb.Fatal(err)
	}
	return world, owner
}

func incrementalHTMLNode(tb testing.TB, world *ecs.World, owner ecs.Entity, id string) Node {
	tb.Helper()
	node, ok := NodeByID(world, owner, id)
	if !ok {
		tb.Fatalf("missing node %q", id)
	}
	return node
}

func incrementalSync(tb testing.TB, world *ecs.World) {
	tb.Helper()
	if err := Sync(world); err != nil {
		tb.Fatal(err)
	}
}

func TestHTMLMutationPaintIsLocalAndAllocationFree(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.hot{color:#ff0000;background:#334455}#target{background:#112233}</style><div id="target">Target</div>`+strings.Repeat(`<div>Unchanged</div>`, 500))
	node := incrementalHTMLNode(t, world, owner, "target")
	before, _ := ecs.Get[htmlRenderState](world, owner)
	children := slices.Clone(before.Children)
	for range 2 {
		node.SetAttribute("class", "hot")
		incrementalSync(t, world)
		node.SetAttribute("class", "")
		incrementalSync(t, world)
	}
	allocations := testing.AllocsPerRun(100, func() {
		node.SetAttribute("class", "hot")
		if err := Sync(world); err != nil {
			panic(err)
		}
		node.SetAttribute("class", "")
		if err := Sync(world); err != nil {
			panic(err)
		}
	})
	after, _ := ecs.Get[htmlRenderState](world, owner)
	if allocations != 0 {
		t.Fatalf("warm class changes allocated %v objects", allocations)
	}
	if after.Boxes != before.Boxes || after.SyncStats.Layouts != 0 || after.SyncStats.Styled > 2 || after.SyncStats.PaintUpdated > 2 || after.SyncStats.PaintUpdated == 0 || after.SyncStats.Spawned != 0 || after.SyncStats.Despawned != 0 {
		t.Fatalf("local paint rebuilt or scanned unrelated branches: %+v", after.SyncStats)
	}
	if !slices.Equal(children, after.Children) {
		t.Fatal("paint changed entity identities")
	}
	node.SetAttribute("class", "hot")
	incrementalSync(t, world)
	after, _ = ecs.Get[htmlRenderState](world, owner)
	for _, entity := range after.RenderNodes[node.node.Children[0]].Entities {
		if text, ok := ecs.Get[scene.Text2D](world, entity); ok && text.Color != graphics.ColorFromRGB(255, 0, 0) {
			t.Fatalf("text paint = %v", text.Color)
		}
	}
}

func TestHTMLSiblingSelectorMutationRestylesOnlyLocalParent(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.trigger + .target{color:#ff0000}</style><div id="scope"><span id="trigger"></span><span id="target" class="target">Target</span></div><section>`+strings.Repeat(`<div>Unrelated</div>`, 120)+`</section>`)
	trigger := incrementalHTMLNode(t, world, owner, "trigger")
	target := incrementalHTMLNode(t, world, owner, "target")
	trigger.SetAttribute("class", "trigger")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if got, want := state.Styles[target.node].Color, style.ColorValue(style.Color{R: 255, A: 255}); got != want {
		t.Fatalf("sibling selector color = %#v, want %#v", got, want)
	}
	if state.SyncStats.Styled >= len(state.Styles)/4 {
		t.Fatalf("sibling mutation restyled beyond its local parent: styled=%d nodes=%d", state.SyncStats.Styled, len(state.Styles))
	}
}

func TestHTMLMutationBatchMergesAncestorRoots(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.hot{color:red}.cold{color:blue}</style><div id="parent"><span id="child">A</span>B</div><div>Unrelated</div>`)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	child.SetAttribute("class", "hot")
	parent.SetAttribute("class", "cold")
	child.SetAttribute("class", "cold")
	parent.SetAttribute("class", "hot")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.Styled != 4 || state.SyncStats.Layouts != 0 {
		t.Fatalf("batch work = %+v", state.SyncStats)
	}
	if state.Styles[child.node].Color != style.ColorValue(style.Color{B: 255, A: 255}) {
		t.Fatal("child override lost")
	}
}

func TestHTMLMutationReusesControlsTextAndScroll(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:60px;overflow:auto"><div id="label">Short</div><input id="field" value="initial"/><div style="height:200px">Long</div></div>`)
	label := incrementalHTMLNode(t, world, owner, "label")
	field := incrementalHTMLNode(t, world, owner, "field")
	scroller := incrementalHTMLNode(t, world, owner, "scroller")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	control := state.EntityGroups[htmlRenderIdentity{Node: field.node, Role: htmlRoleControl}].Bindings[0].Entity
	text := state.RenderNodes[label.node.Children[0]].Entities[0]
	clip := state.EntityGroups[htmlRenderIdentity{Node: scroller.node, Role: htmlRoleOverflow}].Bindings[0].Entity
	SetControlValue(world, control, "typed")
	live := ecs.GetMut[Control](world, control)
	live.SelectionAnchor, live.SelectionFocus = 1, 3
	input := ecs.GetMut[scene.InputState2D](world, control)
	input.Pressed, input.Hovered = true, true
	scroll := ecs.GetMut[scene.UIScroll](world, clip)
	scroll.Position[1] = -25
	label.SetText("Other")
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.Spawned != 0 || state.SyncStats.Despawned != 0 || state.SyncStats.Layouts != 1 {
		t.Fatalf("text mutation work = %+v", state.SyncStats)
	}
	if !world.IsAlive(control) || !world.IsAlive(text) || !world.IsAlive(clip) {
		t.Fatal("persistent primitives replaced")
	}
	live = ecs.GetMut[Control](world, control)
	input = ecs.GetMut[scene.InputState2D](world, control)
	scroll = ecs.GetMut[scene.UIScroll](world, clip)
	if live.Value != "typed" || live.SelectionAnchor != 1 || live.SelectionFocus != 3 || !input.Pressed || !input.Hovered || scroll.Position[1] != -25 {
		t.Fatalf("lost live state: control=%+v input=%+v scroll=%+v", live, input, scroll)
	}
	value, _ := ecs.Get[scene.Text2D](world, text)
	if value.Value != "Other" {
		t.Fatalf("retained text = %q", value.Value)
	}
}

func TestHTMLMutationStructurePreservesSiblingsAndIndex(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent"><button id="first">First</button><button id="second">Second</button></div>`)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	first := incrementalHTMLNode(t, world, owner, "first")
	second := incrementalHTMLNode(t, world, owner, "second")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	retained := slices.Clone(state.RenderNodes[second.node].Entities)
	removed := slices.Clone(state.RenderNodes[first.node].Entities)
	first.Remove()
	clone := second.Clone()
	clone.SetAttribute("id", "third")
	parent.Append(clone)
	if _, ok := NodeByID(world, owner, "first"); ok {
		t.Fatal("removed id still resolves before sync")
	}
	if _, ok := NodeByID(world, owner, "third"); !ok {
		t.Fatal("appended id missing before sync")
	}
	incrementalSync(t, world)
	for _, entity := range retained {
		if !world.IsAlive(entity) {
			t.Fatal("removing earlier sibling replaced later entity")
		}
	}
	for _, entity := range removed {
		if world.IsAlive(entity) {
			t.Fatal("removed primitive leaked")
		}
	}
	parent.SetText("Replacement")
	if _, ok := NodeByID(world, owner, "second"); ok {
		t.Fatal("SetText left stale descendant id")
	}
	incrementalSync(t, world)
	for _, entity := range retained {
		if world.IsAlive(entity) {
			t.Fatal("SetText did not delete subtree primitives")
		}
	}
}

// Compare incremental output with a fresh render of the final document. The
// oracle has no retained components, styles or layout caches to mask stale data.
func TestHTMLMutationMatchesFreshRender(t *testing.T) {
	cases := []struct {
		name, before, after string
		change              func(Node)
	}{
		{"inherited color", `<div id="target"><span>A</span>B</div>`, `<div id="target" style="color:red"><span>A</span>B</div>`, func(n Node) { n.SetAttribute("style", "color:red") }},
		{"background added", `<div id="target">A</div>`, `<div id="target" style="background:red">A</div>`, func(n Node) { n.SetAttribute("style", "background:red") }},
		{"background removed", `<div id="target" style="background:red">A</div>`, `<div id="target">A</div>`, func(n Node) { n.RemoveAttribute("style") }},
		{"decoration added", `<div id="target">A</div>`, `<div id="target" style="text-decoration:line-through">A</div>`, func(n Node) { n.SetAttribute("style", "text-decoration:line-through") }},
		{"mixed borders", `<div id="target" style="border-top:2px solid red">A</div>`, `<div id="target" style="border-top:2px solid blue">A</div>`, func(n Node) { n.SetAttribute("style", "border-top:2px solid blue") }},

		{"transparent border added", `<div id="target" style="border:2px solid transparent">A</div>`, `<div id="target" style="border:2px solid red">A</div>`, func(n Node) { n.SetAttribute("style", "border:2px solid red") }},
		{"border and text color", `<div id="target" style="border-top:2px solid red;color:red">A</div>`, `<div id="target" style="border-top:2px solid blue;color:blue">A</div>`, func(n Node) { n.SetAttribute("style", "border-top:2px solid blue;color:blue") }},
		{"transparent decoration", `<div id="target" style="color:transparent;text-decoration:line-through">A</div>`, `<div id="target" style="color:red;text-decoration:line-through">A</div>`, func(n Node) { n.SetAttribute("style", "color:red;text-decoration:line-through") }},
		{"text wrapping", `<div id="target" style="width:40px">A</div>`, `<div id="target" style="width:40px">Longer wrapping text</div>`, func(n Node) { n.SetText("Longer wrapping text") }},
		{"hidden", `<div id="target"><input value="A"/></div><div>B</div>`, `<div id="target" hidden><input value="A"/></div><div>B</div>`, func(n Node) { n.SetHidden(true) }},
		{"flex redistribution", `<div style="display:flex"><div id="target" style="width:20px">A</div><div style="flex:1">B</div></div>`, `<div style="display:flex"><div id="target" style="width:80px">A</div><div style="flex:1">B</div></div>`, func(n Node) { n.SetAttribute("style", "width:80px") }},
		{"overflow removed", `<div id="target" style="height:30px;overflow:auto"><div style="height:100px">A</div></div>`, `<div id="target" style="height:30px"><div style="height:100px">A</div></div>`, func(n Node) { n.SetAttribute("style", "height:30px") }},
		{"focusability removed", `<div id="target" tabindex="0"><span>A</span></div>`, `<div id="target"><span>A</span></div>`, func(n Node) { n.RemoveAttribute("tabindex") }},
		{"control type", `<input id="target" type="text" value="A"/>`, `<input id="target" type="checkbox" value="A"/>`, func(n Node) { n.SetAttribute("type", "checkbox") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, test.before)
			node := incrementalHTMLNode(t, world, owner, "target")
			test.change(node)
			incrementalSync(t, world)
			fresh, freshOwner := incrementalHTMLFixture(t, test.after)
			got, _ := ecs.Get[htmlRenderState](world, owner)
			want, _ := ecs.Get[htmlRenderState](fresh, freshOwner)
			compareHTMLLayout(t, got.Boxes, want.Boxes)
			if !equalHTMLPrimitiveSnapshots(htmlPrimitiveSnapshot(world, got), htmlPrimitiveSnapshot(fresh, want)) {
				t.Fatalf("generated primitives differ from fresh render\ngot: %#v\nwant: %#v", htmlPrimitiveSnapshot(world, got), htmlPrimitiveSnapshot(fresh, want))
			}
		})
	}
}

func compareHTMLLayout(t *testing.T, got, want *layout.Box) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatal("layout presence differs")
		}
		return
	}
	if !closeHTMLRect(got.Content, want.Content) || got.Border != want.Border || got.Margin != want.Margin || !reflect.DeepEqual(got.Style, want.Style) || len(got.Children) != len(want.Children) || len(got.TextFragments) != len(want.TextFragments) {
		t.Fatalf("layout differs for %s: got=%+v want=%+v", got.Node.Data, got, want)
	}
	for i, fragment := range got.TextFragments {
		expected := want.TextFragments[i]
		if !closeHTMLRect(fragment.Rect, expected.Rect) || fragment.Text != expected.Text || !reflect.DeepEqual(fragment.Style, expected.Style) {
			t.Fatal("text fragment differs from fresh layout")
		}
	}
	for i, child := range got.Children {
		compareHTMLLayout(t, child, want.Children[i])
	}
}

type htmlPrimitiveKey struct {
	order   uint32
	role    htmlRenderRole
	variant string
	slot    int
}
type htmlPrimitiveValue struct {
	TargetReceiver, BlockerReceiver, FocusOwner, EditableOwner htmlPrimitiveKey
	Stack                                                      scene.UIStackIndex
	Transform                                                  scene.UITransform
	Size                                                       scene.Size2D
	Shape                                                      scene.Shape2D
	HasSprite, HasText, HasTarget, HasBlocker, HasClip         bool
	Text                                                       string
	Color                                                      graphics.ColorRGBA
	Clip                                                       scene.ClipRect2D
	Target                                                     scene.InputTarget2D
	Blocker                                                    scene.InputBlocker2D
	Parent                                                     htmlPrimitiveKey
}

func htmlPrimitiveSnapshot(world *ecs.World, state htmlRenderState) map[htmlPrimitiveKey]htmlPrimitiveValue {
	keys := make(map[ecs.Entity]htmlPrimitiveKey)
	for identity, group := range state.EntityGroups {
		for slot, binding := range group.Bindings {
			keys[binding.Entity] = htmlPrimitiveKey{identity.Node.Order, identity.Role, identity.Variant, slot}
		}
	}
	result := make(map[htmlPrimitiveKey]htmlPrimitiveValue)
	for entity, key := range keys {
		var value htmlPrimitiveValue
		value.Transform, _ = ecs.Get[scene.UITransform](world, entity)
		value.Size, _ = ecs.Get[scene.Size2D](world, entity)
		value.Stack, _ = ecs.Get[scene.UIStackIndex](world, entity)
		sprite, ok := ecs.Get[scene.Sprite2D](world, entity)
		value.Shape, value.HasSprite = sprite.Shape, ok
		text, ok := ecs.Get[scene.Text2D](world, entity)
		value.Text, value.Color, value.HasText = text.Value, text.Color, ok
		value.Clip, value.HasClip = ecs.Get[scene.ClipRect2D](world, entity)
		value.Target, value.HasTarget = ecs.Get[scene.InputTarget2D](world, entity)
		value.TargetReceiver = keys[value.Target.Receiver]
		value.Target.Receiver = 0
		value.Blocker, value.HasBlocker = ecs.Get[scene.InputBlocker2D](world, entity)
		value.BlockerReceiver = keys[value.Blocker.Receiver]
		value.Blocker.Receiver = 0
		focus, _ := ecs.Get[FocusOwner](world, entity)
		value.FocusOwner = keys[focus.Entity]
		editable, _ := ecs.Get[EditableText](world, entity)
		value.EditableOwner = keys[editable.Editable]
		parent, _ := ecs.Get[scene.Parent](world, entity)
		value.Parent = keys[parent.Entity]
		result[key] = value
	}
	return result
}

func BenchmarkHTMLLocalClassMutation(b *testing.B) {
	for _, count := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>.hot{color:red}</style><div id="target">Target</div>`+strings.Repeat(`<div>Unchanged</div>`, count))
			node := incrementalHTMLNode(b, world, owner, "target")
			node.SetAttribute("class", "hot")
			incrementalSync(b, world)
			node.SetAttribute("class", "")
			incrementalSync(b, world)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i&1 == 0 {
					node.SetAttribute("class", "hot")
				} else {
					node.SetAttribute("class", "")
				}
				if err := Sync(world); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestHTMLMutationPaintTopologySkipsLayout(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Target</div><input id="field"/>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	initial, _ := ecs.Get[htmlRenderState](world, owner)
	target.SetAttribute("style", "background:red")
	incrementalSync(t, world)
	added, _ := ecs.Get[htmlRenderState](world, owner)
	if added.Boxes != initial.Boxes || added.SyncStats.Layouts != 0 || added.SyncStats.Spawned != 1 || added.SyncStats.Despawned != 0 {
		t.Fatalf("background addition work = %+v", added.SyncStats)
	}
	target.RemoveAttribute("style")
	incrementalSync(t, world)
	removed, _ := ecs.Get[htmlRenderState](world, owner)
	if removed.Boxes != initial.Boxes || removed.SyncStats.Layouts != 0 || removed.SyncStats.Spawned != 0 || removed.SyncStats.Despawned != 1 {
		t.Fatalf("background removal work = %+v", removed.SyncStats)
	}
}

func TestHTMLMutationClearsStaleScrollRouting(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="height:30px;overflow:auto"><div style="height:100px">Content</div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	target.SetAttribute("style", "height:30px")
	incrementalSync(t, world)
	for entity, access := range ecs.Query[ScrollContent](world).All {
		content := access.Get()

		t.Errorf("stale scroll binding on %d: %+v", entity, content)
	}
}

func TestHTMLMutationSharesLayoutInvalidationWithFocus(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>button:focus{width:150px}.hot{color:red}</style><button id="target">Target</button>`)
	node := incrementalHTMLNode(t, world, owner, "target")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	control := state.EntityGroups[htmlRenderIdentity{Node: node.node, Role: htmlRoleControl}].Bindings[0].Entity
	SetControlFocus(world, control, true)
	node.SetAttribute("class", "hot")
	incrementalSync(t, world)
	size, _ := ecs.Get[scene.Size2D](world, control)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if state.Styles[node.node].Width != style.LengthPx(150) || state.SyncStats.Layouts != 1 || size.Width < 150 {
		t.Fatalf("attribute update swallowed pending focus layout: width=%v", size.Width)
	}
}

// Translating retained fragments changes floating-point operation order.
func closeHTMLRect(a, b layout.Rect) bool {
	return math.Abs(a.X-b.X) < 1e-8 && math.Abs(a.Y-b.Y) < 1e-8 && math.Abs(a.Width-b.Width) < 1e-8 && math.Abs(a.Height-b.Height) < 1e-8
}

func equalHTMLPrimitiveSnapshots(a, b map[htmlPrimitiveKey]htmlPrimitiveValue) bool {
	if len(a) != len(b) {
		return false
	}
	for key, got := range a {
		want, exists := b[key]
		if !exists {
			return false
		}
		// ECS coordinates are float32; retained translation can differ at zero by a
		// few float64 ulps before conversion. Geometry itself is checked above.
		if math.Abs(float64(got.Transform.Translation.X.Value-want.Transform.Translation.X.Value)) < 1e-5 {
			got.Transform.Translation.X = want.Transform.Translation.X
		}
		if math.Abs(float64(got.Transform.Translation.Y.Value-want.Transform.Translation.Y.Value)) < 1e-5 {
			got.Transform.Translation.Y = want.Transform.Translation.Y
		}
		if got != want {
			return false
		}
	}
	return true
}

func TestHTMLTextUpdateRetainsUnaffectedLayout(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div><div id="sibling">Unchanged</div>`+strings.Repeat(`<div>Other text</div>`, 100))
	target := incrementalHTMLNode(t, world, owner, "target")
	sibling := incrementalHTMLNode(t, world, owner, "sibling")
	before, _ := ecs.Get[htmlRenderState](world, owner)
	root, siblingBox := before.Boxes, before.BoxesByNode[sibling.node]
	glyph := &siblingBox.TextFragments[0].Glyphs[0]
	for i := 0; i < 20; i++ {
		value := "After"
		if i%2 != 0 {
			value = "Before"
		}
		target.SetText(value)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.Boxes != root || state.BoxesByNode[sibling.node] != siblingBox || &siblingBox.TextFragments[0].Glyphs[0] != glyph {
			t.Fatal("unrelated layout/text storage replaced")
		}
		if state.SyncStats.Styled != 2 || state.SyncStats.LayoutWork.Reflowed != 2 || state.SyncStats.LayoutWork.Created != 0 || state.SyncStats.LayoutWork.Reused != 101 {
			t.Fatalf("local text update work: %+v", state.SyncStats)
		}
	}
}

func BenchmarkHTMLLocalTextMutation(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="target">Before</div>`+strings.Repeat(`<div>Unchanged</div>`, count))
			target := incrementalHTMLNode(b, world, owner, "target")
			target.SetText("After")
			incrementalSync(b, world)
			target.SetText("Before")
			incrementalSync(b, world)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					target.SetText("After")
				} else {
					target.SetText("Before")
				}
				if err := Sync(world); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestHTMLWarmTextMutationAllocations(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div>`+strings.Repeat(`<div>Unchanged</div>`, 10))
	target := incrementalHTMLNode(t, world, owner, "target")
	update := func() {
		target.SetText("After")
		incrementalSync(t, world)
		target.SetText("Before")
		incrementalSync(t, world)
	}
	for i := 0; i < 10; i++ {
		update()
	}
	if allocations := testing.AllocsPerRun(100, update); allocations != 0 {
		t.Fatalf("warm text mutation allocated %v times", allocations)
	}
}
