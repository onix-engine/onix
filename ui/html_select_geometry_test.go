package ui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLSelectTextCenteredInToolbar(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	surface := htmlui.New(`<style>*{box-sizing:border-box} .toolbar{display:flex;align-items:center;gap:6px} select{width:160px;font-size:13px;padding:5px 7px;border:1px solid gray} button{height:28px}</style><div class="toolbar"><select id="mode"><option>Position X</option></select><button>Play</button></div>`)
	surface.Width, surface.Height, surface.Font = 320, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, surface)
	owner := world.Spawn(bundle)
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	control := htmlControlByID(t, world, owner, "mode")
	size := ecs.GetRef[scene.Size2D](world, control)
	for entity, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
		if access.A.Ref().Control != control {
			continue
		}
		text := access.B.Ref()
		position := ecs.GetRef[scene.UITransform](world, entity)
		if delta := position.Translation.Y.Value + text.LineHeight/2 - size.Height/2; math.Abs(float64(delta)) > .01 {
			t.Fatalf("select line box is not centered: y=%v line=%v control=%v", position.Translation.Y.Value, text.LineHeight, size.Height)
		}
		return
	}
	t.Fatal("select label missing")
}

func TestHTMLSelectLongLabelClipsBeforeArrowAndFitsPopup(t *testing.T) {
	for _, fixture := range []struct {
		name           string
		count, repeats int
	}{{"long", 2, 3}, {"scrollbar", 10, 3}, {"viewport limit", 2, 30}} {
		t.Run(fixture.name, func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			ecs.InsertResource(world, scene.ViewportSize{Width: 960, Height: 200})
			label := strings.TrimSpace(strings.Repeat("Language Server: Go (gopls) 中文 ", fixture.repeats))
			surface := htmlui.New(`<style>select{position:absolute;left:650px;top:10px;box-sizing:border-box;width:240px;height:28px;padding:0 8px;font-size:13px}</style><select id="mode"><option selected>` + label + `</option>` + strings.Repeat(`<option>Other</option>`, fixture.count-1) + `</select>`)
			surface.Width, surface.Height, surface.Font = 960, 200, notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, surface)
			owner := world.Spawn(bundle)
			sync := SyncSystem("")
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			control := htmlControlByID(t, world, owner, "mode")
			if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			ecs.GetMut[Selector](world, control).Open = true
			if err := sync.Run(world); err != nil {
				t.Fatal(err)
			}
			arrowX := float32(-1)
			for entity := range ecs.Query[scene.UIImage2D](world).All {
				if parent := ecs.GetRef[scene.Parent](world, entity); parent != nil && parent.Entity == control {
					arrowX = ecs.GetRef[scene.UITransform](world, entity).Translation.X.Value
				}
			}
			foundLabel := false
			for entity, access := range ecs.Query2[htmlui.ControlText, scene.Text2D](world).All {
				if access.A.Ref().Control != control {
					continue
				}
				clip := ecs.GetRef[scene.ClipRect2D](world, entity)
				x := ecs.GetRef[scene.UITransform](world, entity).Translation.X.Value
				if arrowX < 0 || clip == nil || clip.Width <= 0 || x+clip.Width > arrowX-4 {
					t.Fatalf("select label overlaps arrow: x=%v clip=%+v arrow=%v", x, clip, arrowX)
				}
				if text := access.B.Ref(); text.Value != label || text.MaxWidth != 0 || text.WrapWidth != 0 {
					t.Fatal("select label must clip without ellipsis or wrapping")
				}
				foundLabel = true
			}
			if !foundLabel {
				t.Fatal("select label missing")
			}
			runtime := ecs.GetRef[htmlSelectorRuntime](world, control)
			listSize := ecs.GetRef[scene.Size2D](world, runtime.List)
			listX := ecs.GetRef[scene.UITransform](world, runtime.List).Translation.X.Value
			if listSize.Width <= 240 || listSize.Width > 960 || 650+listX < 0 || 650+listX+listSize.Width > 960 {
				t.Fatalf("popup does not fit viewport: size=%+v x=%v", listSize, listX)
			}
			for _, part := range []ecs.Entity{findSurfacePart[SelectorPanelPart](world, runtime.List), findSurfacePart[SelectorPanelBorderPart](world, runtime.List)} {
				if size := ecs.GetRef[scene.Size2D](world, part); size == nil || *size != *listSize {
					t.Fatalf("popup surface size=%+v, want %+v", size, listSize)
				}
			}
			for _, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
				if access.A.Ref().Owner != runtime.Items[0] {
					continue
				}
				text := access.B.Ref()
				width, _, ok := scene.MeasureTextValue2D(text, text.Value)
				wantWidth := width + 24
				if fixture.count == 10 {
					wantWidth += selectorScrollBarSize
				}
				if !ok || math.Abs(float64(listSize.Width-min(wantWidth, 960))) > .01 {
					t.Fatalf("popup width=%v, want %v", listSize.Width, min(wantWidth, 960))
				}
			}
			if width := ecs.GetRef[scene.Size2D](world, control).Width; width != 240 {
				t.Fatalf("popup changed control width: %v", width)
			}
			if fixture.count == 10 {
				track := findSurfacePart[SelectorScrollTrackPart](world, runtime.List)
				x := ecs.GetRef[scene.UITransform](world, track).Translation.X.Value
				if x != listSize.Width-selectorScrollBarSize {
					t.Fatalf("scrollbar x=%v, popup width=%v", x, listSize.Width)
				}
			}
		})
	}
}

func TestHTMLSelectAutoWidthFitsLongestOptionAndTransparentRoundedPopup(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	label := strings.Repeat("Markdown 编辑器 ", 4)
	surface := htmlui.New(`<style>*{box-sizing:border-box} .toolbar{display:flex} select{width:auto;height:22px;font-size:12px;padding:0 24px 0 6px;border:1px solid transparent;border-radius:6px;background:transparent}</style><div class="toolbar"><select id="mode"><option>文本编辑器</option><optgroup label="扩展"><option>` + label + `</option></optgroup></select></div>`)
	surface.Width, surface.Height, surface.Font = 1000, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, surface)
	owner := world.Spawn(bundle)
	sync := SyncSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	control := htmlControlByID(t, world, owner, "mode")
	initialWidth := ecs.GetRef[scene.Size2D](world, control).Width
	ecs.GetMut[Selector](world, control).Open = true
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	runtime := ecs.GetRef[htmlSelectorRuntime](world, control)
	clip := ecs.GetRef[scene.RoundedClip2D](world, runtime.List)
	if clip == nil || clip.Radius != 6 || clip.Width != initialWidth {
		t.Fatalf("transparent select popup lost its rounded bounds: %+v", clip)
	}
	panel := findSurfacePart[SelectorPanelPart](world, runtime.List)
	background := ecs.GetRef[scene.UIBackground2D](world, panel)
	border := findSurfacePart[SelectorPanelBorderPart](world, runtime.List)
	outline := ecs.GetRef[scene.UIBorder2D](world, border)
	if background == nil || outline == nil || background.Shape.Radius != 6 || outline.Shape.Radius != 6 {
		t.Fatalf("popup surfaces lost their radius: background=%+v border=%+v", background, outline)
	}
	count := 0
	for _, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
		if access.A.Ref().Owner != runtime.Items[1] {
			continue
		}
		text := access.B.Ref()
		width, _, ok := scene.MeasureTextValue2D(text, text.Value)
		if !ok || text.FontSize != 12 || width+24 > initialWidth {
			t.Fatalf("option text does not fit automatic width: text=%v font=%v control=%v", width, text.FontSize, initialWidth)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("long option has %d labels, want 1", count)
	}
	if !SetHTMLControlSelected(world, control, 1) {
		t.Fatal("select long option")
	}
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	if width := ecs.GetRef[scene.Size2D](world, control).Width; width != initialWidth {
		t.Fatalf("selection changed intrinsic width: %v -> %v", initialWidth, width)
	}
}

func TestHTMLSelectPopupScrollbarOnlyWhenOverflowing(t *testing.T) {
	for _, fixture := range []struct{ count, viewportHeight int }{{2, 400}, {6, 400}, {8, 400}, {8, 100}} {
		t.Run(fmt.Sprintf("%d-options-%d-height", fixture.count, fixture.viewportHeight), func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			ecs.InsertResource(world, scene.ViewportSize{Width: 320, Height: float32(fixture.viewportHeight)})
			surface := htmlui.New(`<select id="mode" style="box-sizing:border-box;width:160px;height:28px">` + strings.Repeat(`<option>Position X</option>`, fixture.count) + `</select>`)
			surface.Width, surface.Height, surface.Font = 320, 400, notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, surface)
			owner := world.Spawn(bundle)
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			control := htmlControlByID(t, world, owner, "mode")
			if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			ecs.GetMut[Selector](world, control).Open = true
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			runtime := ecs.GetRef[htmlSelectorRuntime](world, control)
			track := findSurfacePart[SelectorScrollTrackPart](world, runtime.List)
			thumb := findSurfacePart[SelectorScrollThumbPart](world, runtime.List)
			if track == 0 || thumb == 0 {
				t.Fatal("popup scrollbar parts missing")
			}
			scroll := ecs.GetRef[scene.UIScroll](world, runtime.List)
			overflow := scroll.ContentSize.Y() > scroll.ViewportSize.Y()
			if overflow != (fixture.viewportHeight == 100) {
				t.Fatalf("unexpected popup overflow: viewport=%v content=%v", scroll.ViewportSize, scroll.ContentSize)
			}
			for _, part := range []ecs.Entity{track, thumb} {
				partSize := ecs.GetRef[scene.Size2D](world, part)
				if (partSize.Width > 0 && partSize.Height > 0) != overflow {
					t.Fatalf("scrollbar geometry=%+v overflow=%v", partSize, overflow)
				}
			}
			item := ecs.GetRef[scene.Size2D](world, runtime.Items[0])
			wantWidth := float32(160)
			if overflow {
				wantWidth -= selectorScrollBarSize
			}
			if item.Width != wantWidth {
				t.Fatalf("option width=%v, want %v", item.Width, wantWidth)
			}
			if overflow {
				if !SetSelectorScrollOffset(world, control, 30) {
					t.Fatal("could not scroll popup")
				}
				if err := SyncAfterLayoutSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
				if position := ecs.GetRef[scene.UITransform](world, thumb); position.Translation.Y.Value <= 0 {
					t.Fatal("scrollbar thumb did not follow popup scroll offset")
				}
			}
		})
	}
}
