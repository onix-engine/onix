package html

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func htmlBindingArea(world *ecs.World, x, y float32) ecs.Entity {
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, scene.UIScroll{Position: mgl32.Vec2{x, y}, Horizontal: true, Vertical: true})
	return world.Spawn(bundle)
}

func TestHTMLBindingLocalMatchesFull(t *testing.T) {
	for _, body := range []string{
		`<div id="outer"><div id="target">Text</div></div>`,
		`<div id="outer"><div><input id="target" value="Value"/></div></div>`,
		`<div id="outer" tabindex="0"><div id="target">Focus</div></div>`,
		`<div id="outer" style="overflow:auto;height:90px"><div id="target">Overflow</div></div>`,
		`<div id="outer"><div style="overflow:auto;height:90px"><div id="target">Nested clip</div></div></div>`,
	} {
		t.Run(body, func(t *testing.T) {
			source := `<style>.wide{width:180px;color:red}</style>` + body + `<div><input id="other"/></div>`
			world, owner := incrementalHTMLFixture(t, source)
			reference, referenceOwner := incrementalHTMLFixture(t, source)
			for _, fixture := range []struct {
				world *ecs.World
				owner ecs.Entity
			}{{world, owner}, {reference, referenceOwner}} {
				area := htmlBindingArea(fixture.world, -4, -8)
				SetNodeOffset(fixture.world, fixture.owner, "outer", 3, 7)
				SetNodeOffset(fixture.world, fixture.owner, "target", 5, 11)
				SetNodeScroll(fixture.world, fixture.owner, "outer", area, 13, 17)
				// Establish suppression of any CSS clip on the externally bound node.
				incrementalHTMLNode(t, fixture.world, fixture.owner, "target").SetAttribute("class", "wide")
				incrementalSync(t, fixture.world)
			}
			target := incrementalHTMLNode(t, world, owner, "target")
			other := incrementalControl(t, world, owner, "other")
			before, _ := ecs.Get[Control](world, other)
			for _, class := range []string{"", "wide", "", "wide"} {
				target.SetAttribute("class", class)
				incrementalHTMLNode(t, reference, referenceOwner, "target").SetAttribute("class", class)
				refState := ecs.GetMut[htmlRenderState](reference, referenceOwner)
				refState.PartialValid = false
				incrementalSync(t, world)
				incrementalSync(t, reference)
				state, _ := ecs.Get[htmlRenderState](world, owner)
				want, _ := ecs.Get[htmlRenderState](reference, referenceOwner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatalf("fell back: %+v", state.SyncStats)
				}
				if got, expected := htmlPrimitiveSnapshot(world, state), htmlPrimitiveSnapshot(reference, want); !reflect.DeepEqual(got, expected) {
					for key, value := range got {
						if value != expected[key] {
							t.Fatalf("primitive %+v differs:\ngot %+v\nwant %+v", key, value, expected[key])
						}
					}
					t.Fatal("primitive membership differs")
				}
				after, _ := ecs.Get[Control](world, other)
				if after.RenderRevision != before.RenderRevision {
					t.Fatal("unrelated control rewritten")
				}
			}
		})
	}
}

func TestHTMLBindingRebaseAndRuntimeOffset(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div><input id="target"/></div>`)
	entity := incrementalControl(t, world, owner, "target")
	base, _ := ecs.Get[scene.UITransform](world, entity)
	area := htmlBindingArea(world, -3, -7)
	next := htmlBindingArea(world, -9, -15)
	SetNodeOffset(world, owner, "target", 5, 11)
	SetNodeScroll(world, owner, "target", area, 13, 17)
	check := func(area ecs.Entity, origin, offset, position mgl32.Vec2) {
		t.Helper()
		content, _ := ecs.Get[ScrollContent](world, entity)
		parent, _ := ecs.Get[scene.Parent](world, entity)
		transform, _ := ecs.Get[scene.UITransform](world, entity)
		x, y := base.Translation.X.Value+offset.X()-origin.X(), base.Translation.Y.Value+offset.Y()-origin.Y()
		if content.Area != area || parent.Entity != area || content.X != x || content.Y != y || transform.Translation.X.Value != x+position.X() || transform.Translation.Y.Value != y+position.Y() {
			t.Fatalf("incorrect rebase: content=%+v parent=%v transform=%+v; want (%v,%v)", content, parent, transform, x, y)
		}
	}
	check(area, mgl32.Vec2{13, 17}, mgl32.Vec2{5, 11}, mgl32.Vec2{-3, -7})
	for range 3 {
		if SetNodeScroll(world, owner, "target", area, 13, 17) {
			t.Fatal("identical binding changed coordinates")
		}
		SetNodeOffset(world, owner, "target", 7, 19)
		check(area, mgl32.Vec2{13, 17}, mgl32.Vec2{7, 19}, mgl32.Vec2{-3, -7})
		SetNodeScroll(world, owner, "target", next, 23, 29)
		check(next, mgl32.Vec2{23, 29}, mgl32.Vec2{7, 19}, mgl32.Vec2{-9, -15})
		SetNodeScroll(world, owner, "target", area, 13, 17)
	}
	// Children generated for the input keep their control-local parent chain.
	state, _ := ecs.Get[htmlRenderState](world, owner)
	for _, childEntity := range state.Children {
		parent, _ := ecs.Get[scene.Parent](world, childEntity)
		if parent.Entity == entity && ecs.Has[ScrollContent](world, childEntity) {
			t.Fatal("control child acquired independent scroll")
		}
	}
}

func TestHTMLBindingRegistrationOrder(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="outer"><input id="target"/></div>`)
	entity := incrementalControl(t, world, owner, "target")
	first, second := htmlBindingArea(world, 0, 0), htmlBindingArea(world, 0, 0)
	SetNodeOffset(world, owner, "outer", 3, 7)
	SetNodeOffset(world, owner, "target", 5, 11)
	SetNodeOffset(world, owner, "outer", 13, 17)
	SetNodeScroll(world, owner, "outer", first, 0, 0)
	SetNodeScroll(world, owner, "target", second, 0, 0)
	child, _ := ecs.Get[htmlChild](world, entity)
	parent, _ := ecs.Get[scene.Parent](world, entity)
	if child.offset != (mgl32.Vec2{5, 11}) || parent.Entity != first {
		t.Fatal("registration precedence changed")
	}
}

func TestHTMLBindingControlChildrenInheritOffsetOnce(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="track"><input id="target" value="Value"/></div>`)
	control := incrementalControl(t, world, owner, "target")
	var visual ecs.Entity
	for entity, access := range ecs.Query2[scene.Parent, scene.Text2D](world).All {
		if access.A.Get().Entity == control {
			visual = entity
			break
		}
	}
	if visual == 0 {
		t.Fatal("missing control text")
	}
	before, _ := ecs.Get[scene.UITransform](world, visual)
	for _, offset := range []mgl32.Vec2{{0, -5000}, {3, -4990}, {0, 0}} {
		SetNodeOffset(world, owner, "track", offset.X(), offset.Y())
		after, _ := ecs.Get[scene.UITransform](world, visual)
		if after.Translation != before.Translation {
			t.Fatalf("control child applied inherited offset twice: before=%+v after=%+v", before, after)
		}
	}
}

func BenchmarkHTMLBindingLocalResize(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>.wide{width:180px}</style><div><input id="target" value="Value"/></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, count))
			area := htmlBindingArea(world, 0, -7)
			SetNodeOffset(world, owner, "target", 3, 11)
			SetNodeScroll(world, owner, "target", area, 0, 13)
			target := incrementalHTMLNode(b, world, owner, "target")
			for _, class := range []string{"wide", ""} {
				target.SetAttribute("class", class)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					target.SetAttribute("class", "wide")
				} else {
					target.SetAttribute("class", "")
				}
				incrementalSync(b, world)
			}
		})
	}
}

func TestHTMLBindingsFollowIDsAndClearOnSourceReplacement(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="outer"><input id="target"/></div><div><input id="other"/></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	other := incrementalHTMLNode(t, world, owner, "other")
	entity := incrementalControl(t, world, owner, "target")
	otherEntity := incrementalControl(t, world, owner, "other")
	area := htmlBindingArea(world, 0, -7)
	SetNodeOffset(world, owner, "target", 3, 11)
	SetNodeScroll(world, owner, "target", area, 0, 13)
	target.SetAttribute("id", "retired")
	other.SetAttribute("id", "target")
	incrementalSync(t, world)
	child, _ := ecs.Get[htmlChild](world, entity)
	parent, _ := ecs.Get[scene.Parent](world, entity)
	if child.offset != (mgl32.Vec2{}) || parent.Entity != owner || ecs.Has[ScrollContent](world, entity) {
		t.Fatal("old ID retained binding")
	}
	otherChild, _ := ecs.Get[htmlChild](world, otherEntity)
	otherParent, _ := ecs.Get[scene.Parent](world, otherEntity)
	if otherChild.offset != (mgl32.Vec2{3, 11}) || otherParent.Entity != area {
		t.Fatal("new ID did not receive binding")
	}
	html := ecs.GetMut[Surface](world, owner)
	html.Value = `<div><input id="target" value="Replacement"/></div>`
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if len(state.NodeOffsets) != 0 || len(state.NodeScrolls) != 0 {
		t.Fatal("source replacement retained bindings")
	}
	entity = incrementalControl(t, world, owner, "target")
	parent, _ = ecs.Get[scene.Parent](world, entity)
	if parent.Entity != owner || ecs.Has[ScrollContent](world, entity) {
		t.Fatal("replacement inherited old viewport")
	}
}

func TestHTMLWarmBindingResizeDoesNotAllocate(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.wide{width:180px}</style><div><input id="target" value="Value"/></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, 10))
	area := htmlBindingArea(world, 0, -7)
	SetNodeOffset(world, owner, "target", 3, 11)
	SetNodeScroll(world, owner, "target", area, 0, 13)
	target := incrementalHTMLNode(t, world, owner, "target")
	update := func() {
		target.SetAttribute("class", "wide")
		incrementalSync(t, world)
		target.SetAttribute("class", "")
		incrementalSync(t, world)
	}
	for range 10 {
		update()
	}
	// Interning costs one instance per newly distinct style, so both directions
	// of the class toggle allocate. Unchanged nodes stay allocation-free.
	if n := testing.AllocsPerRun(100, update); n > 2 {
		t.Fatalf("warm binding update allocated %v times", n)
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.BindingEntities >= len(state.Children) || state.SyncStats.BindingEntities == 0 {
		t.Fatalf("binding replay did not stay local: %+v, children=%d", state.SyncStats, len(state.Children))
	}
}

func TestHTMLOffsetTextFragmentsMatchFull(t *testing.T) {
	source := `<div id="target" style="width:60px">Before</div><div>After</div>`
	world, owner := incrementalHTMLFixture(t, source)
	reference, referenceOwner := incrementalHTMLFixture(t, source)
	SetNodeOffset(world, owner, "target", 3, 11)
	SetNodeOffset(reference, referenceOwner, "target", 3, 11)
	for _, value := range []string{"Several words wrap across multiple lines", "Short", "New text"} {
		incrementalHTMLNode(t, world, owner, "target").SetText(value)
		incrementalHTMLNode(t, reference, referenceOwner, "target").SetText(value)
		ref := ecs.GetMut[htmlRenderState](reference, referenceOwner)
		ref.PartialValid = false
		incrementalSync(t, world)
		incrementalSync(t, reference)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		want, _ := ecs.Get[htmlRenderState](reference, referenceOwner)
		if state.SyncStats.PartialReconciles != 1 || !reflect.DeepEqual(htmlPrimitiveSnapshot(world, state), htmlPrimitiveSnapshot(reference, want)) {
			t.Fatalf("offset text update differs: %+v", state.SyncStats)
		}
	}
}
