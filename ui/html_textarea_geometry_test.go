package ui

import (
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLTextareaScrollbarsInheritRoundedOverflowClip(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	value := strings.Repeat("long source ", 40) + "\n" + strings.Repeat("line\n", 40)
	html := htmlui.New(`<div id="frame" style="width:120px;height:80px;overflow:hidden;border-radius:8px"><textarea id="ed" style="display:block;box-sizing:border-box;width:140px;height:100px;margin:0;padding:0;border:0;white-space:pre;overflow:scroll">` + value + `</textarea></div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	control := htmlControlByID(t, world, owner, "ed")
	verifyClip := func(width, height float32) {
		t.Helper()
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		controlGlobal, _ := ecs.Get[scene.UIGlobalTransform](world, control)
		clip, ok := controlGlobal.ClipRect()
		if !ok || clip.Width != width || clip.Height != height {
			t.Fatalf("textarea lost its ancestor overflow bounds: %+v, ok=%v", clip, ok)
		}
		count := 0
		for entity, access := range ecs.Query2[htmlui.ControlVisual, scene.Size2D](world).All {
			visual, size := access.A.Get(), access.B.Get()
			if visual.Control != control || visual.Kind < htmlui.ControlVisualScrollTrackHorizontal || visual.Kind > htmlui.ControlVisualScrollThumbVertical {
				continue
			}
			if size.Width <= 0 || size.Height <= 0 {
				t.Fatalf("scrollbar part %v is not visible: %+v", visual.Kind, size)
			}
			global, _ := ecs.Get[scene.UIGlobalTransform](world, entity)
			want := controlGlobal
			want.Matrix = global.Matrix
			// Compare the full transform value so this checks the inherited
			// rounded mask as well as the rectangular scissor bounds.
			if global != want {
				t.Fatalf("scrollbar part %v escaped the textarea's inherited rounded clip", visual.Kind)
			}
			count++
		}
		if count != 4 {
			t.Fatalf("scrollbar parts = %d, want 4", count)
		}
	}
	verifyClip(120, 80)
	frame, _ := htmlui.NodeByID(world, owner, "frame")
	frame.SetAttribute("style", "width:110px;height:70px;overflow:hidden;border-radius:12px")
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	textarea := ecs.GetMut[Textarea](world, control)
	textarea.ScrollX, textarea.ScrollY = 1e6, 1e6
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	verifyClip(110, 70)
}

func TestHTMLTextareaScrollUsesCSSContentBox(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	const boxStyle = "box-sizing:border-box;margin:0;padding:7px 11px 19px 23px;border:3px solid red;font-size:13px;line-height:20px;white-space:pre;tab-size:2"
	value := strings.Repeat("long source ", 40) + "\n" + strings.Repeat("line\n", 40)
	html := htmlui.New(`<textarea id="ed" style="width:300px;height:140px;` + boxStyle + `">` + value + `</textarea>`)
	html.Width, html.Height, html.Font = 500, 300, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	control := htmlControlByID(t, world, owner, "ed")
	verifyTracks := func(width, height float32) {
		t.Helper()
		geometry, ok := textareaScrollGeometryFor(world, control)
		if !ok || geometry.trackX != 3 || geometry.trackY != 3 || geometry.verticalX+geometry.barSize != width-3 || geometry.horizontalY+geometry.barSize != height-3 {
			t.Fatalf("scrollbar tracks must follow the inside of the border: %+v", geometry)
		}
		if geometry.trackWidth != width-6-geometry.barSize || geometry.trackHeight != height-6-geometry.barSize {
			t.Fatalf("tracks must span the padding box and exclude the scrollbar corner: %+v", geometry)
		}
		if textareaScrollAxisAt(geometry, width-4, 4) != textareaScrollAxisVertical || textareaScrollAxisAt(geometry, 4, height-4) != textareaScrollAxisHorizontal {
			t.Fatal("scrollbars do not accept input along the inside border edges")
		}
		for entity, access := range ecs.Query3[Relation[SurfaceStyle], scene.UITransform, scene.Size2D](world).All {
			if access.A.Ref().Owner != control {
				continue
			}
			position, size := access.B.Get(), access.C.Get()
			if ecs.Has[Part[ScrollTrackVerticalPart]](world, entity) && (position.Translation.X.Value+size.Width != width-3 || position.Translation.Y.Value != 3 || size.Height != geometry.trackHeight) {
				t.Fatal("painted vertical track does not match the edge hit area")
			}
			if ecs.Has[Part[ScrollTrackHorizontalPart]](world, entity) && (position.Translation.Y.Value+size.Height != height-3 || position.Translation.X.Value != 3 || size.Width != geometry.trackWidth) {
				t.Fatal("painted horizontal track does not match the edge hit area")
			}
		}
	}
	verifyTracks(300, 140)
	field := ecs.GetMut[TextField](world, control)
	field.Caret, field.Anchor, field.Focused = 0, 0, true
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	textarea := ecs.GetMut[Textarea](world, control)
	textarea.ScrollX, textarea.ScrollY = 64, 40
	for range 5 {
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		_, transform := htmlTextareaValueText(t, world, control)
		if transform.Translation.X.Value != 26-64 || transform.Translation.Y.Value != 10-40 {
			t.Fatalf("CSS origin drifted after scrolling: %+v", transform.Translation)
		}
		caret, _ := ecs.Get[scene.UITransform](world, htmlCaretVisual(t, world, control))
		if caret.Translation != transform.Translation {
			t.Fatal("caret did not move with the value in the same layout pass")
		}
	}
	geometry, ok := textareaScrollGeometryFor(world, control)
	if !ok || geometry.viewportWidth != 260 || geometry.viewportHeight != 108 {
		t.Fatalf("asymmetric CSS content geometry = %+v, ok=%v", geometry, ok)
	}
	node, _ := htmlui.NodeByID(world, owner, "ed")
	node.SetAttribute("style", "width:340px;height:160px;"+boxStyle)
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	geometry, ok = textareaScrollGeometryFor(world, control)
	if !ok || geometry.viewportWidth != 300 || geometry.viewportHeight != 128 {
		t.Fatalf("resized CSS content geometry = %+v, ok=%v", geometry, ok)
	}
	verifyTracks(340, 160)
}

func TestHTMLTextareaFollowTailOnContentAndViewportChanges(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	html := htmlui.New(`<textarea id="log" readonly style="width:200px;height:100px;padding:7px 9px;white-space:pre-wrap">` + strings.Repeat("line\n", 30) + `</textarea>`)
	html.Width, html.Height, html.Font = 400, 300, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	control := htmlControlByID(t, world, owner, "log")
	ecs.GetMut[Textarea](world, control).FollowTail = true
	verifyTail := func() {
		t.Helper()
		if err := SyncAfterLayoutSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		geometry, ok := textareaScrollGeometryFor(world, control)
		textarea, _ := ecs.Get[Textarea](world, control)
		want := max(geometry.contentHeight-geometry.viewportHeight, 0)
		if !ok || want <= 0 || math.Abs(float64(textarea.ScrollY-want)) > .01 {
			t.Fatalf("tail offset = %g, want %g", textarea.ScrollY, want)
		}
	}
	verifyTail()
	ecs.GetMut[Textarea](world, control).ScrollY = 0
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if state, _ := ecs.Get[Textarea](world, control); state.ScrollY != 0 {
		t.Fatal("idle synchronization prevented reading earlier log lines")
	}
	field := ecs.GetMut[TextField](world, control)
	field.Focused, field.Caret, field.Anchor = true, 0, 0
	SetHTMLControlValue(world, control, field.Value+strings.Repeat("latest line wraps across the viewport ", 10))
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	verifyTail()
	node, _ := htmlui.NodeByID(world, owner, "log")
	node.SetAttribute("style", "width:120px;height:60px;padding:7px 9px;white-space:pre-wrap")
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	verifyTail()
}

func TestHTMLTextareaTabSizeUsesNumberAndLength(t *testing.T) {
	for _, size := range []string{"2", "30px", "0"} {
		world := ecs.NewWorld()
		RegisterRuntimeFeature[HTMLRenderer](world)
		html := htmlui.New(`<textarea id="ed" style="width:300px;height:100px;white-space:pre;tab-size:` + size + `">A&#9;B</textarea>`)
		html.Width, html.Height, html.Font = 400, 200, notosanssc.NotoSansSCRegular
		bundle := ecs.NewBundle(1)
		ecs.Put(&bundle, html)
		owner := world.Spawn(bundle)
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
		}
		control := htmlControlByID(t, world, owner, "ed")
		entity, _ := htmlTextareaValueText(t, world, control)
		text, _ := ecs.Get[scene.Text2D](world, entity)
		want := float32(30)
		if size == "2" {
			face := text.Font.FontFace(text.FontSize, text.DPI)
			advance, _ := face.GlyphAdvance(' ')
			want = float32(advance) / 64 * 2
		} else if size == "0" {
			want = 0
		}
		if math.Abs(float64(text.TabWidth-want)) > .001 {
			t.Fatalf("tab-size:%s interval = %v, want %v", size, text.TabWidth, want)
		}
	}
}
