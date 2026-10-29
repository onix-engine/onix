package editor

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

func animationEditorFixture(t *testing.T) *ModuleEditor {
	t.Helper()
	clip := project.AnimationClipSpec{Duration: 1, SampleRate: 25, FloatTracks: []project.AnimationTrackSpec[float32]{{Binding: scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX}, Keyframes: []project.AnimationKeyframeSpec[float32]{{Time: 0, Value: 4, Tween: scene.AnimationLinear}, {Time: 1, Value: 104}}}}}
	a := contentEditorFixture(t, project.ModComponents{Animation: &project.AnimationComponentSpec{Clip: clip, Speed: 1}})
	return a
}
func animationChange(t *testing.T, a *ModuleEditor, name, value string) {
	t.Helper()
	id := "animation:" + name
	if name == "value" || name == "visible" {
		id = animationDetailValueID(t, a)
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Value: value, Checked: value == "true"})
	workspaceShellTest(t, a)
}

func animationDetailValueID(t *testing.T, a *ModuleEditor) string {
	t.Helper()
	for id, field := range a.fields {
		if field.Animation && field.Pointer == a.animation.Detail.ValuePointer {
			return id
		}
	}
	t.Fatal("selected key has no component value field in Details")
	return ""
}
func animationClick(t *testing.T, a *ModuleEditor, name string) {
	t.Helper()
	a.handleEvent(htmlui.Event{ID: "animation:" + name, Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
}
func animationFixtureClip(a *ModuleEditor) project.AnimationClipSpec {
	return a.viewMod("button").Elements[0].Components.Animation.Clip
}

func TestWorkspaceAnimationDoubleClickTabsAndSelection(t *testing.T) {
	a := animationEditorFixture(t)
	action := a.treeRowControlID(workspaceTreeRow{Scene: "main.json", ID: "start", Element: "face"})
	a.time = 1
	a.handleEvent(htmlui.Event{ID: action, Kind: htmlui.EventClick})
	if a.animation.Open {
		t.Fatal("single click opened Animation")
	}
	a.time = 1.2
	a.handleEvent(htmlui.Event{ID: action, Kind: htmlui.EventClick})
	source := workspaceShellTest(t, a)
	if !a.animation.Active || !strings.Contains(source, `id="animation-tab"`) || !strings.Contains(source, `id="animation:key:float:0:1"`) || strings.Contains(source, ">Float Tracks<") {
		t.Fatal("double click did not open the timeline or left raw track JSON in Details")
	}
	animationChange(t, a, "value", "12")
	a.handleEvent(htmlui.Event{ID: "resources-tab", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if a.animation.Active || !a.hasDrafts() {
		t.Fatal("Resources tab lost animation draft")
	}
	a.handleEvent(htmlui.Event{ID: "animation-tab", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if !a.animation.Active || animationFixtureClip(a).FloatTracks[0].Keyframes[0].Value != 12 {
		t.Fatal("returning to Animation lost keyframe edits")
	}
	a.action("save")
	a.selectElement("start", "other")
	source = workspaceShellTest(t, a)
	if _, ok := a.animationClip(); ok {
		t.Fatal("animation tab retained unrelated selected element")
	}
	animationClick(t, a, "open")
	if a.animation.Active {
		t.Fatal("element without animation opened Animation")
	}
}

func TestWorkspaceAnimationTransitionPopupEditingAndIsolation(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	source := workspaceShellTest(t, a)
	for _, label := range []string{"Insert key", "Delete key", "Delete track"} {
		if !strings.Contains(source, ">"+label+"</span>") {
			t.Fatalf("toolbar is missing the visible %q caption", label)
		}
	}
	a.time = 1
	animationClick(t, a, "segment:float:0:0")
	if a.animation.CurveOpen {
		t.Fatal("single click opened the transition popup")
	}
	a.time = 1.8
	animationClick(t, a, "segment:float:0:0")
	if a.animation.CurveOpen {
		t.Fatal("separate clicks opened the transition popup")
	}
	a.time = 2
	animationClick(t, a, "segment:float:0:0")
	source = workspaceShellTest(t, a)
	if !a.animation.CurveOpen || !a.modalVisible() || !strings.Contains(source, `id="animation-curve-dialog"`) || !strings.Contains(source, `id="animation:tween"`) {
		t.Fatal("double click did not open the transition editor")
	}
	closeID := strings.Index(source, `id="animation:close-curve"`)
	if closeID < 0 {
		t.Fatal("popup close control missing")
	}
	closeStart := strings.LastIndex(source[:closeID], "<button ")
	if closeStart < 0 {
		t.Fatal("popup close control is not a button")
	}
	closeEnd := strings.Index(source[closeStart:], "</button>")
	if closeEnd < 0 {
		t.Fatal("popup close button markup is incomplete")
	}
	closeButton := source[closeStart : closeStart+closeEnd]
	if !strings.Contains(closeButton, `src="onix-titlebar/close.svg"`) || !strings.Contains(closeButton, `aria-label="Close dialog"`) || !strings.Contains(closeButton, `title="Close"`) || strings.Contains(closeButton, ">Close<") {
		t.Fatal("popup did not use the standard icon-only close button", closeButton)
	}
	a.handleEvent(htmlui.Event{ID: "resources-tab", Kind: htmlui.EventClick})
	animationClick(t, a, "delete-key")
	if !a.animation.Active || len(animationFixtureClip(a).FloatTracks[0].Keyframes) != 2 {
		t.Fatal("popup allowed an underlying action")
	}
	animationChange(t, a, "tween", "bezier")
	syncAnimationLayoutTest(t, a)
	in := input.NewState()
	a.updateAnimationCurveFocus(in)
	closeControl, _ := htmlui.ControlByID(a.world, a.shell, "animation:close-curve")
	focus, _ := a.world.GetResource[*scene.InputFocus]()
	if focus == nil || focus.Keyboard != closeControl {
		t.Fatal("popup did not focus its close button")
	}
	in.PressKey(input.KeyLeftShift)
	in.PressKey(input.KeyTab)
	a.updateAnimationCurveFocus(in)
	lastControl, _ := htmlui.ControlByID(a.world, a.shell, "animation:save-curve")
	if focus.Keyboard != lastControl {
		t.Fatal("reverse Tab escaped the popup")
	}
	in.EndFrame()
	in.ReleaseKey(input.KeyLeftShift)
	in.ReleaseKey(input.KeyTab)
	in.EndFrame()
	in.PressKey(input.KeyTab)
	a.updateAnimationCurveFocus(in)
	if focus.Keyboard != closeControl {
		t.Fatal("Tab escaped the popup")
	}
	in.EndFrame()
	in.ReleaseKey(input.KeyTab)
	in.EndFrame()
	a.updateAnimationCurveFocus(in)
	if allocations := testing.AllocsPerRun(20, func() { a.updateAnimationCurveFocus(in) }); allocations != 0 {
		t.Fatalf("idle popup focus handling allocated %g", allocations)
	}
	handle, ok := htmlui.NodeByID(a.world, a.shell, "animation:handle:1")
	if !ok {
		t.Fatal("Bezier handle missing from popup")
	}
	box, ok := handle.Bounds()
	if !ok || !a.updateAnimationPointer(input.Pointer{Pressed: true, Down: true, X: box.X, Y: box.Y + box.Height}) {
		t.Fatal("popup handle did not capture the pointer")
	}
	bounds := a.animation.CurveBounds
	if !a.updateAnimationPointer(input.Pointer{Down: true, X: bounds.X + bounds.W*.2, Y: bounds.Y + bounds.H*.4, DeltaX: 1}) {
		t.Fatal("popup blocked curve dragging")
	}
	curve := animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve
	if abs(curve.X1-.2) > .001 || abs(curve.Y1-.65) > .001 {
		t.Fatal("popup drag edited the wrong curve", curve)
	}
	a.updateAnimationPointer(input.Pointer{Released: true})
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "animation:x2", Kind: htmlui.EventInput, Value: "1.5"})
	animationClick(t, a, "close-curve")
	if !a.animation.CurveOpen || a.dialogError == "" || animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve.X2 != .58 {
		t.Fatal("invalid pending curve edit closed the popup or changed the clip")
	}
	a.handleEvent(htmlui.Event{ID: "animation:x2", Kind: htmlui.EventInput, Value: ".8"})
	animationClick(t, a, "close-curve")
	if a.animation.CurveOpen || a.modalVisible() || animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve.X2 != .8 {
		t.Fatalf("closing popup: open=%v modal=%v curve=%+v drafts=%v status=%s", a.animation.CurveOpen, a.modalVisible(), animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve, a.drafts, a.status)
	}
	a.action("save")
	loaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mods["button"].Spec.Elements[0].Components.Animation.Clip.FloatTracks[0].Keyframes[0].Curve.X2 != .8 {
		t.Fatal("popup curve edit did not persist")
	}
}

func TestWorkspaceAnimationDiscreteTransitionPopup(t *testing.T) {
	for _, kind := range []string{"texture", "bool"} {
		t.Run(kind, func(t *testing.T) {
			a := animationEditorFixture(t)
			animationClick(t, a, "open")
			target := "sprite.texture"
			if kind == "bool" {
				target = "visibility.visible"
			}
			animationChange(t, a, "target", target)
			animationClick(t, a, "add-track")
			animationClick(t, a, "seek:0.5")
			animationClick(t, a, "add-key")
			a.time = 1
			animationClick(t, a, "segment:"+kind+":0:0")
			a.time = 1.2
			animationClick(t, a, "segment:"+kind+":0:0")
			source := workspaceShellTest(t, a)
			if !a.animation.CurveOpen {
				t.Fatal("double-click did not open the transition popup")
			}
			if kind == "texture" {
				if !strings.Contains(source, "Sequence playback curve") || !strings.Contains(source, `id="animation:tween"`) || !strings.Contains(source, "Whole sequence:") {
					t.Fatal("frame sequence did not expose its playback curve")
				}
			} else if !strings.Contains(source, "Step interpolation") || strings.Contains(source, `id="animation:tween"`) {
				t.Fatal("visibility transition did not explain Step interpolation")
			}
			if allocations := testing.AllocsPerRun(20, func() { a.updateAnimationPointer(input.Pointer{}) }); allocations != 0 {
				t.Fatalf("idle popup pointer handling allocated %g", allocations)
			}
			a.closeWorkspaceDialog()
			if a.animation.CurveOpen || a.modalVisible() {
				t.Fatal("Escape close path did not dismiss the transition popup")
			}
		})
	}
}

func TestWorkspaceAnimationCurveSaveAppliesPendingInputs(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	a.time = 1
	animationClick(t, a, "segment:float:0:0")
	a.time = 1.2
	animationClick(t, a, "segment:float:0:0")
	animationClick(t, a, "custom-curve")
	syncAnimationLayoutTest(t, a)
	button, ok := htmlui.NodeByID(a.world, a.shell, "animation:save-curve")
	if !ok {
		t.Fatal("curve popup has no Save button")
	}
	bounds, _ := button.Bounds()
	panel, _ := htmlui.NodeByID(a.world, a.shell, "animation-curve-dialog")
	panelBounds, _ := panel.Bounds()
	if bounds.Y+bounds.Height+16 > panelBounds.Y+panelBounds.Height {
		t.Fatal("Save button does not fit in the popup")
	}
	a.handleEvent(htmlui.Event{ID: "animation:x2", Kind: htmlui.EventInput, Value: "1.5"})
	animationClick(t, a, "save-curve")
	if !a.animation.CurveOpen || a.dialogError == "" || !a.hasDrafts() {
		t.Fatal("Save accepted invalid coordinates or discarded pending edits")
	}
	file := a.Document.Absolute(string(modDocumentID("button")))
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(original, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	a.handleEvent(htmlui.Event{ID: "animation:x2", Kind: htmlui.EventInput, Value: ".8"})
	animationClick(t, a, "save-curve")
	if !a.animation.CurveOpen || !strings.Contains(a.dialogError, "changed outside") || !a.Document.Dirty() {
		t.Fatal("failed file save did not retain the popup and edits", a.dialogError)
	}
	if err := os.WriteFile(file, original, 0o644); err != nil {
		t.Fatal(err)
	}
	a.handleEvent(htmlui.Event{ID: "animation:y1", Kind: htmlui.EventInput, Value: ".7"})
	animationClick(t, a, "save-curve")
	if a.animation.CurveOpen || a.hasDrafts() || a.Document.Dirty() || a.status != "Animation curve saved" {
		t.Fatal("Save did not apply edits and close the popup", a.status)
	}
	loaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	key := loaded.Mods["button"].Spec.Elements[0].Components.Animation.Clip.FloatTracks[0].Keyframes[0]
	if key.Tween != scene.AnimationBezier || key.Curve.X2 != .8 || key.Curve.Y1 != .7 {
		t.Fatal("Save did not persist pending coordinates", key)
	}
}

func TestWorkspaceAnimationCurveSelectorScrollsOutsideControlClip(t *testing.T) {
	a := animationEditorFixture(t)
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](a.world)
	in := input.NewState()
	a.world.InsertResource(in)
	a.world.InsertResource(scene.ViewportSize{Width: a.width, Height: a.height})
	animationClick(t, a, "open")
	a.time = 1
	animationClick(t, a, "segment:float:0:0")
	a.time = 1.2
	animationClick(t, a, "segment:float:0:0")
	syncFrame := func() {
		t.Helper()
		workspaceShellTest(t, a)
		if err := engineui.SyncSystem("").Run(a.world); err != nil {
			t.Fatal(err)
		}
		if err := engineui.SyncAfterLayoutSystem("").Run(a.world); err != nil {
			t.Fatal(err)
		}
		if err := scene.SyncUIGlobalTransformSystem("").Run(a.world); err != nil {
			t.Fatal(err)
		}
	}
	syncFrame()
	control, ok := htmlui.ControlByID(a.world, a.shell, "animation:tween")
	if !ok {
		t.Fatal("curve preset selector is missing")
	}
	scene.DispatchInputEvent(a.world, scene.InputEvent{Entity: control, Kind: scene.InputEventTap})
	if err := engineui.UpdateSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	syncFrame()
	var list ecs.Entity
	for entity, access := range a.world.Query[engineui.SelectorList]().All {
		if access.Ref().Selector == control {
			list = entity
			break
		}
	}
	if list == 0 || !a.world.GetRef[engineui.Selector](control).Open {
		t.Fatal("curve preset dropdown did not open")
	}
	scroll := a.world.GetRef[scene.UIScroll](list)
	if scroll.DisableInput || scroll.ContentSize[1] <= scroll.ViewportSize[1] {
		t.Fatal("preset dropdown has no active scroll viewport", scroll)
	}
	var backgrounds []struct {
		entity   ecs.Entity
		position mgl32.Vec2
	}
	for entity, access := range a.world.Query[scene.UIScroll]().All {
		if entity != list {
			backgrounds = append(backgrounds, struct {
				entity   ecs.Entity
				position mgl32.Vec2
			}{entity, access.Ref().Position})
		}
	}
	global := a.world.GetRef[scene.UIGlobalTransform](list)
	point := global.Matrix.Mul4x1(mgl32.Vec4{20, scroll.ViewportSize[1] / 2, 0, 1})
	in.MoveMouse(float64(point.X()), float64(point.Y()))
	for _, wheel := range []float64{-1, -1, 1} {
		before := a.world.GetRef[scene.UIScroll](list).Position[1]
		in.ScrollMouse(0, wheel)
		if err := a.update(&ecs.SystemContext{World: a.world}); err != nil {
			t.Fatal(err)
		}
		if err := scene.UIScrollSystem("").Run(a.world); err != nil {
			t.Fatal(err)
		}
		after := a.world.GetRef[scene.UIScroll](list).Position[1]
		if wheel < 0 && after >= before || wheel > 0 && after <= before || in.Pointer().WheelY != 0 {
			t.Fatalf("preset wheel did not scroll: before=%g after=%g wheel=%g", before, after, in.Pointer().WheelY)
		}
		syncFrame()
		if got := a.world.GetRef[scene.UIScroll](list).Position[1]; got != after {
			t.Fatalf("popup reconciliation reset scroll from %g to %g", after, got)
		}
		for _, background := range backgrounds {
			if got := a.world.GetRef[scene.UIScroll](background.entity).Position; got != background.position {
				t.Fatal("preset wheel scrolled the popup or background panel")
			}
		}
		in.EndFrame()
	}
}

func TestWorkspaceAnimationEasingPresetsAndCustomCurve(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	a.time = 1
	animationClick(t, a, "segment:float:0:0")
	a.time = 1.2
	animationClick(t, a, "segment:float:0:0")
	source := workspaceShellTest(t, a)
	for i, tween := range animationTweens.Values {
		if !strings.Contains(source, `value="`+tween+`"`) || !strings.Contains(source, ">"+animationTweens.Options[i]+"</option>") {
			t.Fatalf("popup is missing preset %s", tween)
		}
		animationChange(t, a, "tween", tween)
		if animationFixtureClip(a).FloatTracks[0].Keyframes[0].Tween != scene.AnimationTween(tween) {
			t.Fatalf("selecting %s did not edit the interval", tween)
		}
	}
	animationChange(t, a, "tween", "bezier")
	animationChange(t, a, "y1", "3")
	animationChange(t, a, "y2", "-2")
	custom := animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve
	animationChange(t, a, "tween", string(scene.AnimationEaseOutElastic))
	animationChange(t, a, "tween", "bezier")
	syncAnimationLayoutTest(t, a)
	if animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve != custom || a.animation.CurveMinY >= -2 || a.animation.CurveMaxY <= 3 {
		t.Fatal("switching presets lost the custom curve or clipped its handles")
	}
	handle, _ := htmlui.NodeByID(a.world, a.shell, "animation:handle:1")
	box, _ := handle.Bounds()
	if !a.updateAnimationPointer(input.Pointer{Pressed: true, Down: true, X: box.X, Y: box.Y + box.Height}) {
		t.Fatal("custom handle did not capture the pointer")
	}
	bounds, low, high := a.animation.CurveBounds, a.animation.CurveMinY, a.animation.CurveMaxY
	a.updateAnimationPointer(input.Pointer{Down: true, X: bounds.X + bounds.W*.2, Y: bounds.Y + bounds.H*.4, DeltaX: 1})
	curve := animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve
	if abs(curve.X1-.2) > 1e-4 || abs(curve.Y1-(high-(high-low)*.4)) > 1e-4 {
		t.Fatal("dragging ignored the expanded preview range", curve)
	}
	a.updateAnimationPointer(input.Pointer{Released: true})
	animationChange(t, a, "tween", string(scene.AnimationEaseOutBounce))
	animationClick(t, a, "save-curve")
	loaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	keys := loaded.Mods["button"].Spec.Elements[0].Components.Animation.Clip.FloatTracks[0].Keyframes
	value, _ := (scene.AnimationTrack[float32]{Keyframes: []scene.AnimationKeyframe[float32]{
		{Time: keys[0].Time, Value: keys[0].Value, Tween: keys[0].Tween, Curve: keys[0].Curve},
		{Time: keys[1].Time, Value: keys[1].Value},
	}}).Sample(.5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if keys[0].Tween != scene.AnimationEaseOutBounce || abs(value-80.5625) > 1e-5 {
		t.Fatalf("saved preset did not play correctly: tween=%s value=%g", keys[0].Tween, value)
	}
}

func TestWorkspaceAnimationSolidCurveGeometry(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	a.time = 1
	animationClick(t, a, "segment:float:0:0")
	a.time = 1.2
	animationClick(t, a, "segment:float:0:0")
	for _, width := range []float32{1600, 420, 1600} {
		a.width = width
		for _, tween := range []scene.AnimationTween{scene.AnimationLinear, scene.AnimationEaseOutElastic, scene.AnimationBezier, scene.AnimationLinear, scene.AnimationBezier, scene.AnimationStep} {
			animationChange(t, a, "tween", string(tween))
			syncAnimationLayoutTest(t, a)
			graph, _ := htmlui.NodeByID(a.world, a.shell, "animation-curve")
			control, _ := htmlui.ControlByID(a.world, a.shell, "animation:tween")
			var scroll scene.UIScroll
			for entity := control; entity != 0; {
				if value, ok := a.world.Get[scene.UIScroll](entity); ok {
					scroll = value
					break
				}
				parent, _ := a.world.Get[scene.Parent](entity)
				entity = parent.Entity
			}
			ok := scroll.ViewportSize[0] > 0
			if !ok || scroll.Horizontal || scroll.ContentSize[0] > scroll.ViewportSize[0]+.01 {
				t.Fatalf("%s width=%g expanded popup horizontally: %+v", tween, width, scroll)
			}
			if width == 1600 && scroll.Vertical {
				t.Fatalf("%s unnecessarily expanded popup vertically: %+v", tween, scroll)
			}
			bounds, ok := graph.Bounds()
			if !ok || abs(bounds.Width-(a.dialogPanel(480, 460).W-42)) > .01 {
				t.Fatalf("curve does not fit resized popup: %+v", bounds)
			}
			var lines [][2]mgl32.Vec4
			for entity, access := range a.world.Query[scene.Sprite2D]().All {
				sprite := access.Get()
				if sprite.Shape.Fill != graphics.ColorFromRGB(132, 204, 255) || sprite.Height != 2 {
					continue
				}
				global := a.world.GetRef[scene.UIGlobalTransform](entity)
				lines = append(lines, [2]mgl32.Vec4{global.Matrix.Mul4x1(mgl32.Vec4{0, 1, 0, 1}), global.Matrix.Mul4x1(mgl32.Vec4{sprite.Width - .5, 1, 0, 1})})
			}
			count := 128
			if tween == scene.AnimationStep {
				count = 2
			}
			if len(lines) != count {
				t.Fatalf("%s has %d solid segments, want %d", tween, len(lines), count)
			}
			slices.SortFunc(lines, func(a, b [2]mgl32.Vec4) int {
				if a[0].X() < b[0].X() {
					return -1
				}
				if a[0].X() > b[0].X() {
					return 1
				}
				return 0
			})
			for i, line := range lines {
				if i > 0 && line[0].Sub(lines[i-1][1]).Len() > .01 {
					t.Fatalf("%s width=%g has a gap at segment %d: %v → %v", tween, width, i, lines[i-1][1], line[0])
				}
				for _, point := range line {
					if point.X() < bounds.X || point.X() > bounds.X+bounds.Width || point.Y() < bounds.Y || point.Y() > bounds.Y+bounds.Height {
						t.Fatalf("%s curve escaped the plot: %v, bounds=%+v", tween, point, bounds)
					}
				}
			}
			if tween == scene.AnimationStep && (abs(lines[0][0].Y()-lines[0][1].Y()) > .01 || abs(lines[1][0].X()-lines[1][1].X()) > .01) {
				t.Fatal("Step preview is not a horizontal hold followed by a vertical jump")
			}
		}
	}
}

func TestWorkspaceAnimationSequenceCurveEditingAndPreview(t *testing.T) {
	a := animationEditorFixture(t)
	var original []project.AnimationKeyframeSpec[project.TextureSpec]
	if err := a.Document.EditMod("button", "Frame sequence", func(mod *project.ModSpec) error {
		for i, color := range []string{"#ffffff", "#ff0000", "#00ff00", "#0000ff", "#000000"} {
			original = append(original, project.AnimationKeyframeSpec[project.TextureSpec]{Time: float32(i) / 4, Value: project.TextureSpec{Color: color}, Tween: scene.AnimationStep})
		}
		mod.Elements[0].Components.Animation.Clip.TextureTracks = []project.AnimationTrackSpec[project.TextureSpec]{{Binding: scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture}, Keyframes: original}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	animationClick(t, a, "open")
	a.time = 1
	animationClick(t, a, "segment:texture:0:1")
	a.time = 1.2
	animationClick(t, a, "segment:texture:0:1")
	source := workspaceShellTest(t, a)
	for _, text := range []string{"Sequence playback curve", "Whole sequence: 0 → 1 s", "5 frames", "Sequence progress", `id="animation:custom-curve"`} {
		if !strings.Contains(source, text) {
			t.Fatalf("sequence curve editor is missing %q", text)
		}
	}
	if strings.Contains(source, `value="cubicSpline"`) || strings.Contains(source, "Step interpolation") {
		t.Fatal("sequence editor contains value interpolation controls")
	}
	for _, test := range []struct {
		tween scene.AnimationTween
		frame int
	}{{scene.AnimationEaseIn, 1}, {scene.AnimationEaseOut, 3}, {scene.AnimationLinear, 2}} {
		animationChange(t, a, "tween", string(test.tween))
		if a.fields["animation:tween"].Pointer != "/elements/0/components/animation/clip/textureTracks/0/timeTween" {
			t.Fatal("playback selector is editing a keyframe's value tween")
		}
		a.syncDraftPreview()
		a.Preview.Sync(a.previewDocument(), a.Local)
		if err := a.syncViewport2D(); err != nil {
			t.Fatal(err)
		}
		entity := a.viewport2D.rows[0].targets["face"]
		a.animation.Time = .5
		a.syncAnimationPreview(0)
		if err := a.viewport2D.syncVisuals(false); err != nil {
			t.Fatal(err)
		}
		animator := a.viewport2D.world.GetRef[scene.Animator](entity)
		sprite := a.viewport2D.world.GetRef[scene.Sprite2D](entity)
		if sprite.Texture != animator.Clip.TextureTracks[0].Keyframes[test.frame].Value {
			t.Fatalf("%s preview selected the wrong halfway frame", test.tween)
		}
	}
	animationClick(t, a, "custom-curve")
	syncAnimationLayoutTest(t, a)
	panel, _ := htmlui.NodeByID(a.world, a.shell, "animation-curve-dialog")
	panelBounds, _ := panel.Bounds()
	lastField, _ := htmlui.NodeByID(a.world, a.shell, "animation:y2")
	fieldBounds, _ := lastField.Bounds()
	if fieldBounds.Y+fieldBounds.Height+16 > panelBounds.Y+panelBounds.Height {
		t.Fatalf("sequence curve controls are outside the popup: field=%+v popup=%+v", fieldBounds, panelBounds)
	}
	button, _ := htmlui.NodeByID(a.world, a.shell, "animation:save-curve")
	buttonBounds, _ := button.Bounds()
	if buttonBounds.Y+buttonBounds.Height+16 > panelBounds.Y+panelBounds.Height {
		t.Fatal("sequence curve Save button is outside the popup")
	}
	track := animationFixtureClip(a).TextureTracks[0]
	if track.TimeTween != scene.AnimationBezier || track.TimeCurve != scene.EaseInOutAnimationCurve() {
		t.Fatal("custom curve action did not initialize editable Bezier handles")
	}
	handle, _ := htmlui.NodeByID(a.world, a.shell, "animation:handle:2")
	box, _ := handle.Bounds()
	if !a.updateAnimationPointer(input.Pointer{Pressed: true, Down: true, X: box.X, Y: box.Y + box.Height}) {
		t.Fatal("sequence curve handle did not capture the pointer")
	}
	bounds := a.animation.CurveBounds
	a.updateAnimationPointer(input.Pointer{Down: true, X: bounds.X + bounds.W*.8, Y: bounds.Y + bounds.H*.3, DeltaX: 1})
	a.updateAnimationPointer(input.Pointer{Released: true})
	workspaceShellTest(t, a)
	curve := animationFixtureClip(a).TextureTracks[0].TimeCurve
	if abs(curve.X2-.8) > 1e-4 || abs(curve.Y2-.8) > 1e-4 {
		t.Fatal("dragging edited the wrong curve", curve)
	}
	if a.fields["animation:x1"].Pointer != "/elements/0/components/animation/clip/textureTracks/0/timeCurve/x1" {
		t.Fatal("Bezier coordinate field is not bound to the track's time curve")
	}
	a.handleEvent(htmlui.Event{ID: "animation:x1", Kind: htmlui.EventInput, Value: "1.5"})
	animationClick(t, a, "close-curve")
	if !a.animation.CurveOpen || a.dialogError == "" {
		t.Fatal("invalid time curve was accepted")
	}
	a.handleEvent(htmlui.Event{ID: "animation:x1", Kind: htmlui.EventInput, Value: ".2"})
	animationClick(t, a, "close-curve")
	if a.animation.CurveOpen {
		t.Fatal("valid time curve could not be committed")
	}
	a.time = 2
	animationClick(t, a, "segment:texture:0:2")
	a.time = 2.2
	animationClick(t, a, "segment:texture:0:2")
	if !a.animation.CurveOpen || abs(animationFixtureClip(a).TextureTracks[0].TimeCurve.X1-.2) > 1e-4 || abs(animationFixtureClip(a).TextureTracks[0].TimeCurve.X2-.8) > 1e-4 {
		t.Fatalf("another segment did not reopen the same sequence playback curve: open=%v values=%v curve=%+v", a.animation.CurveOpen, a.controlValues, animationFixtureClip(a).TextureTracks[0].TimeCurve)
	}
	animationClick(t, a, "save-curve")
	loaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	saved := loaded.Mods["button"].Spec.Elements[0].Components.Animation.Clip.TextureTracks[0]
	if saved.TimeTween != scene.AnimationBezier || abs(saved.TimeCurve.X1-.2) > 1e-4 || abs(saved.TimeCurve.X2-.8) > 1e-4 {
		t.Fatal("saving lost the frame playback curve")
	}
	for i, key := range saved.Keyframes {
		if key.Time != original[i].Time || key.Value.Color != original[i].Value.Color || key.Tween != scene.AnimationStep {
			t.Fatal("editing playback pace modified authored frame timing or values")
		}
	}
}

func TestWorkspaceAnimationKeyframesCurvesSaveAndUndo(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationClick(t, a, "segment:float:0:0")
	animationChange(t, a, "tween", "bezier")
	animationChange(t, a, "x1", "0.2")
	animationChange(t, a, "y1", "0.7")
	clip := animationFixtureClip(a)
	if clip.FloatTracks[0].Keyframes[0].Curve.X1 != .2 || clip.FloatTracks[0].Keyframes[0].Curve.Y1 != .7 {
		t.Fatal("custom curve was not staged")
	}
	animationChange(t, a, "tween", "linear")
	animationClick(t, a, "seek:0.5")
	animationClick(t, a, "add-key")
	keys := animationFixtureClip(a).FloatTracks[0].Keyframes
	if len(keys) != 3 || keys[1].Time != .5 || keys[1].Value != 54 {
		t.Fatalf("insertion did not sample the track: %+v", keys)
	}
	animationChange(t, a, "key-time", "0.75")
	animationChange(t, a, "value", "80")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Animation.Clip.FloatTracks[0].Keyframes[0].Value != 4 {
		t.Fatal("draft changed committed document")
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatal(a.status)
	}
	loaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mods["button"].Spec.Elements[0].Components.Animation.Clip.FloatTracks[0].Keyframes[1].Value != 80 {
		t.Fatal("keyframes did not persist")
	}
	a.action("undo")
	if len(animationFixtureClip(a).FloatTracks[0].Keyframes) != 2 {
		t.Fatal("undo did not restore original clip")
	}
	a.action("redo")
	workspaceShellTest(t, a)
	if len(animationFixtureClip(a).FloatTracks[0].Keyframes) != 3 {
		t.Fatal("redo lost inserted key")
	}
	animationClick(t, a, "key:float:0:1")
	animationClick(t, a, "delete-key")
	if len(animationFixtureClip(a).FloatTracks[0].Keyframes) != 2 {
		t.Fatal("delete did not remove selected key")
	}
}
func TestWorkspaceAnimationRejectsInvalidEditsAtomically(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	for _, edit := range [][2]string{{"key-time", "1"}, {"value", "NaN"}} {
		animationChange(t, a, edit[0], edit[1])
		clip := animationFixtureClip(a)
		if a.hasDrafts() || clip.Duration != 1 || clip.FloatTracks[0].Keyframes[0].Time != 0 || a.animation.Key != 0 {
			t.Fatalf("invalid %s changed the clip or selection", edit[0])
		}
	}
	animationChange(t, a, "tween", "bezier")
	animationChange(t, a, "x1", "1.5")
	if animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve.X1 != .42 {
		t.Fatal("invalid curve was accepted")
	}
	animationChange(t, a, "target", "transform.position.z")
	animationClick(t, a, "add-track")
	if len(animationFixtureClip(a).FloatTracks) != 1 {
		t.Fatal("UI animation accepted a world-only track")
	}
	animationChange(t, a, "target", "transform.scale.x")
	animationClick(t, a, "add-track")
	if keys := animationFixtureClip(a).FloatTracks[1].Keyframes; keys[0].Value != 1 {
		t.Fatal("scale did not start at one")
	}
	animationClick(t, a, "delete-track")
	animationClick(t, a, "delete-track")
	if len(animationFixtureClip(a).FloatTracks) != 1 {
		t.Fatal("last track was removed")
	}
}
func TestWorkspaceAnimationTextureDraftSurvivesKeySwitch(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationChange(t, a, "target", "sprite.texture")
	animationClick(t, a, "add-track")
	animationClick(t, a, "seek:0.5")
	animationClick(t, a, "add-key")
	target := modDocumentID("button")
	pointer := "/elements/0/components/animation/clip/textureTracks/0/keyframes/1/value/color"
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, pointer, ""), Kind: htmlui.EventInput, Value: "#ff0000"})
	animationClick(t, a, "key:texture:0:0")
	if animationFixtureClip(a).TextureTracks[0].Keyframes[1].Value.Color != "#ff0000" {
		t.Fatal("switching keyframe lost its texture draft")
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatal(a.status)
	}
}
func TestWorkspaceAnimationPreviewSeekPlaybackAndAllocations(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	a.syncDraftPreview()
	a.Preview.Sync(a.previewDocument(), a.Local)
	if err := a.syncViewport2D(); err != nil {
		t.Fatal(err)
	}
	entity := a.viewport2D.rows[0].targets["face"]
	a.animation.Time = .5
	a.syncAnimationPreview(0)
	if err := a.viewport2D.syncVisuals(false); err != nil {
		t.Fatal(err)
	}
	animator := a.viewport2D.world.GetRef[scene.Animator](entity)
	transform := a.viewport2D.world.GetRef[scene.UITransform](entity)
	if animator == nil || animator.Playing || math.Abs(float64(transform.Translation.X.Value-54)) > 1e-4 {
		t.Fatalf("seek did not preview selected element: %+v %+v", animator, transform)
	}
	a.animation.Playing = true
	a.syncAnimationPreview(.25)
	if animator.Time() != .75 {
		t.Fatalf("preview time=%g", animator.Time())
	}
	allocs := testing.AllocsPerRun(20, func() { a.syncAnimationPreview(0) })
	if allocs != 0 {
		t.Fatalf("animation preview allocates: %g", allocs)
	}
	a.animation.Active = false
	a.syncAnimationPreview(0)
	if !animator.Playing {
		t.Fatal("Resources tab did not release preview playback")
	}
}
func TestWorkspaceAnimationCurveDragAndNewComponent(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationChange(t, a, "tween", "bezier")
	a.animation.DragHandle = 1
	a.animation.CurveBounds = rect{W: 100, H: 100}
	a.updateAnimationPointer(input.Pointer{Down: true, X: 25, Y: 50, DeltaX: 1})
	curve := animationFixtureClip(a).FloatTracks[0].Keyframes[0].Curve
	if curve.X1 != .25 || curve.Y1 != .5 || a.animation.DragHandle != 1 {
		t.Fatal("curve dragging did not update the control point", curve)
	}
	a.updateAnimationPointer(input.Pointer{Released: true})
	if a.animation.DragHandle != 0 {
		t.Fatal("curve handle did not release")
	}
	b := contentEditorFixture(t, project.ModComponents{})
	b.action("content-add:button%2Fmod.json|/elements/0/components/animation")
	workspaceShellTest(t, b)
	b.openAnimation()
	clip, ok := b.animationClip()
	if !ok || clip.Validate() != nil || clip.FloatTracks[0].Keyframes[0].Value != 4 {
		t.Fatalf("new animation component was not editable: %s", b.status)
	}
	b.action("save")
	if b.hasDrafts() {
		t.Fatal(b.status)
	}
}

func TestWorkspaceAnimationAtlasFramesAndWorldChannels(t *testing.T) {
	a := animationEditorFixture(t)
	file := a.Document.Absolute("button/resources/sheet.json")
	if err := writeWorkspaceFile(file, []byte(`{"frames":{"face":{"frame":{"x":0,"y":0,"w":8,"h":8}},"wing":{"frame":{"x":8,"y":0,"w":8,"h":8}}},"meta":{"image":"sheet.png"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.EditMod("button", "Atlas and world fixture", func(mod *project.ModSpec) error {
		mod.Space = project.CoordinateSpaceWorld
		element := &mod.Elements[0]
		element.Transform.Rotation = []float32{.4}
		element.Components.Animation.Clip.TextureTracks = []project.AnimationTrackSpec[project.TextureSpec]{{Binding: scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture}, Keyframes: []project.AnimationKeyframeSpec[project.TextureSpec]{{Time: 0, Value: project.TextureSpec{Atlas: "resources/sheet.json", Frame: "face"}, Tween: scene.AnimationStep}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	animationClick(t, a, "open")
	animationClick(t, a, "track:texture:0")
	pointer := "/elements/0/components/animation/clip/textureTracks/0/keyframes/0/value/frame"
	id := workspaceFieldID(modDocumentID("button"), pointer, "")
	source := workspaceShellTest(t, a)
	if !strings.Contains(source, `<select id="`+id+`"`) || !strings.Contains(source, ">wing<") {
		t.Fatal("sprite frame did not offer atlas regions")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Value: "wing"})
	animationChange(t, a, "target", "transform.position.z")
	animationClick(t, a, "add-track")
	animationChange(t, a, "target", "transform.rotation.x")
	animationClick(t, a, "add-track")
	clip := animationFixtureClip(a)
	if len(clip.FloatTracks) != 3 || clip.FloatTracks[2].Keyframes[0].Value != 0 || clip.TextureTracks[0].Keyframes[0].Value.Frame != "wing" {
		t.Fatalf("world track or atlas frame edits were lost: %+v (%s)", clip, a.status)
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatal(a.status)
	}
}

func TestWorkspaceAnimationSplineTangentsAndVisibility(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationChange(t, a, "tween", "cubicSpline")
	animationChange(t, a, "outTangent", "200")
	animationChange(t, a, "inTangent", "0")
	animationClick(t, a, "seek:0.5")
	animationClick(t, a, "add-key")
	if animationFixtureClip(a).FloatTracks[0].Keyframes[1].Value != 79 {
		t.Fatal("inserted spline key did not use tangents")
	}
	animationChange(t, a, "target", "visibility.visible")
	animationClick(t, a, "add-track")
	animationChange(t, a, "visible", "false")
	if animationFixtureClip(a).BoolTracks[0].Keyframes[0].Value {
		t.Fatal("visibility key was not editable")
	}
	animationClick(t, a, "delete-track")
	if a.animation.Kind != "float" {
		t.Fatal("deleting the last visibility track did not select a remaining track")
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatal(a.status)
	}
}

func TestWorkspaceAnimationSaveFocusedInputsAndReorder(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationClick(t, a, "seek:0.5")
	animationClick(t, a, "add-key")
	a.handleEvent(htmlui.Event{ID: "animation:key-time", Kind: htmlui.EventInput, Value: "0.8"})
	a.handleEvent(htmlui.Event{ID: animationDetailValueID(t, a), Kind: htmlui.EventInput, Value: "88"})
	workspaceShellTest(t, a)
	if !a.hasDrafts() {
		t.Fatal("typing did not mark animation as modified")
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatal(a.status)
	}
	keys := animationFixtureClip(a).FloatTracks[0].Keyframes
	if keys[1].Time != .8 || keys[1].Value != 88 {
		t.Fatal("saving focused numeric fields lost their input", keys)
	}
	animationClick(t, a, "key:float:0:2")
	a.handleEvent(htmlui.Event{ID: "animation:key-time", Kind: htmlui.EventInput, Value: "0.2"})
	workspaceShellTest(t, a)
	a.action("save")
	keys = animationFixtureClip(a).FloatTracks[0].Keyframes
	if a.hasDrafts() || keys[1].Time != .2 || keys[1].Value != 104 || keys[2].Value != 88 || a.animation.Key != 1 {
		t.Fatal("saving focused key time did not preserve sorted values and selection", keys, a.status)
	}
	a.handleEvent(htmlui.Event{ID: animationDetailValueID(t, a), Kind: htmlui.EventInput, Value: "-"})
	workspaceShellTest(t, a)
	a.action("save")
	if !a.hasDrafts() || animationFixtureClip(a).FloatTracks[0].Keyframes[1].Value != 104 {
		t.Fatal("invalid focused input overwrote a key")
	}
}

func TestWorkspaceAnimationDetailsKeepsComponentAndKeyEditsSeparate(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationClick(t, a, "key:float:0:1")
	target := modDocumentID("button")
	x := workspaceFieldID(target, "/elements/0/transform/position", "X")
	y := workspaceFieldID(target, "/elements/0/transform/position", "Y")
	if field := a.fields[x]; !field.Animation || field.Axis != -1 || field.Value != "104" || field.Pointer != a.animation.Detail.ValuePointer {
		t.Fatalf("Details position did not bind the selected key: %+v", field)
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventInput, Value: "120"})
	a.handleEvent(htmlui.Event{ID: y, Kind: htmlui.EventInput, Value: "8"})
	a.handleEvent(htmlui.Event{ID: "resources-tab", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	element := a.viewMod("button").Elements[0]
	if element.Transform.Position[0] != 4 || element.Transform.Position[1] != 8 || element.Components.Animation.Clip.FloatTracks[0].Keyframes[1].Value != 120 {
		t.Fatal("leaving Animation wrote a key value into the component or lost an axis edit", element)
	}
	if field := a.fields[x]; field.Animation || field.Axis != 0 || field.Value != "4" {
		t.Fatal("Resources did not restore the authored component position", field)
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventInput, Value: "9"})
	a.handleEvent(htmlui.Event{ID: "animation-tab", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if a.fields[x].Value != "120" || a.viewMod("button").Elements[0].Transform.Position[0] != 9 {
		t.Fatal("returning to Animation applied a component draft to a key")
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventInput, Value: "140"})
	animationClick(t, a, "key:float:0:0")
	if a.fields[x].Value != "4" || animationFixtureClip(a).FloatTracks[0].Keyframes[1].Value != 140 {
		t.Fatal("switching key lost its pending Details value")
	}
	a.action("save")
	if a.hasDrafts() {
		t.Fatal(a.status)
	}
	a.action("undo")
	element = a.viewMod("button").Elements[0]
	if element.Transform.Position[0] != 4 || element.Transform.Position[1] != 5 || element.Components.Animation.Clip.FloatTracks[0].Keyframes[1].Value != 104 {
		t.Fatal("undo did not restore both the component and key", element)
	}
}

func TestWorkspaceAnimationTextureValuesOnlyAppearInDetails(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationChange(t, a, "target", "sprite.texture")
	animationClick(t, a, "add-track")
	root, err := htmlui.Root(a.world, a.shell)
	if err != nil {
		t.Fatal(err)
	}
	details, ok := root.FindByID("inspector")
	if !ok {
		t.Fatal("Details panel missing")
	}
	colorID := workspaceFieldID(modDocumentID("button"), a.animation.Detail.ValuePointer+"/color", "")
	if _, ok := details.FindByID("content-color:" + colorID); !ok {
		t.Fatal("Details sprite did not expose the selected frame")
	}
	animation, ok := details.FindByID("body:content:/elements/0/components/animation")
	if !ok {
		t.Fatal("Details Animation component missing")
	}
	if _, ok := animation.FindByID("animation:key-time"); !ok {
		t.Fatal("Animation component did not expose keyframe time")
	}
	if _, ok := details.FindByID("body:animation-key-time"); ok {
		t.Fatal("Details retained a separate Keyframe section")
	}
	panel, ok := root.FindByID("animation-panel")
	if !ok {
		t.Fatal("Animation panel missing")
	}
	if _, ok := panel.FindByID("content-color:" + colorID); ok {
		t.Fatal("Animation retained a duplicate texture editor")
	}
	if _, ok := panel.FindByID("animation:key-time"); ok {
		t.Fatal("Animation retained duplicate keyframe information")
	}
	a.handleEvent(htmlui.Event{ID: colorID, Kind: htmlui.EventChange, Value: "#123456"})
	workspaceShellTest(t, a)
	if clip := animationFixtureClip(a); clip.TextureTracks[0].Keyframes[0].Value.Color != "#123456" || a.viewMod("button").Elements[0].Components.Sprite != nil {
		t.Fatal("Details frame edit created or changed the authored sprite")
	}
}

func TestWorkspaceAnimationDetailsTargetsBoundElement(t *testing.T) {
	a := animationEditorFixture(t)
	if err := a.Document.EditMod("button", "Track target fixture", func(mod *project.ModSpec) error {
		mod.Elements[0].Components.Animation.Clip.FloatTracks[0].Binding.Path = "other"
		mod.Elements[1].Transform.Position = []float32{7, 8}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	animationClick(t, a, "open")
	animationClick(t, a, "key:float:0:1")
	id := workspaceFieldID(modDocumentID("button"), "/elements/1/transform/position", "X")
	if field := a.fields[id]; !field.Animation || field.Value != "104" {
		t.Fatal("Details did not resolve the track target", field)
	}
	animation, ok := htmlui.NodeByID(a.world, a.shell, "body:content:/elements/0/components/animation")
	if !ok {
		t.Fatal("bound target hid the owning Animation component")
	}
	if _, ok := animation.FindByID("animation:key-time"); !ok {
		t.Fatal("bound target hid the keyframe time in the owning Animation component")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Value: "150"})
	workspaceShellTest(t, a)
	if animationFixtureClip(a).FloatTracks[0].Keyframes[1].Value != 150 || a.viewMod("button").Elements[1].Transform.Position[0] != 7 || a.selectedElement != "face" {
		t.Fatal("editing a bound target changed its baseline or the animation owner selection")
	}
}

func syncAnimationLayoutTest(t *testing.T, a *ModuleEditor) {
	t.Helper()
	workspaceShellTest(t, a)
	if err := htmlui.Sync(a.world); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: a.world}
	if err := a.restoreAnimationScroll(context); err != nil {
		t.Fatal(err)
	}
	if err := engineui.SyncAfterLayoutSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceAnimationWheelZoomAnchorsTimeAndScrubbing(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	syncAnimationLayoutTest(t, a)
	ruler, ok := a.animationRulerBounds()
	if !ok {
		t.Fatal("timeline ruler missing")
	}
	x, y := ruler.X+ruler.W*.35, ruler.Y+ruler.H/2
	in := input.NewState()
	in.MoveMouse(float64(x), float64(y))
	for _, wheel := range []float64{4, 3, -2, -1} {
		before := a.animation.Zoom
		in.ScrollMouse(0, wheel)
		if !a.updateAnimationWheel(in) || in.Pointer().WheelY != 0 {
			t.Fatal("timeline did not consume its zoom gesture")
		}
		if wheel > 0 && a.animation.Zoom <= before || wheel < 0 && a.animation.Zoom >= before {
			t.Fatal("timeline zoom moved in the wrong direction", before, a.animation.Zoom)
		}
		syncAnimationLayoutTest(t, a)
		ruler, _ = a.animationRulerBounds()
		if abs(ruler.X+ruler.W*.35-x) > .01 {
			t.Fatal("zoom moved the time under the cursor", ruler, x)
		}
		in.EndFrame()
	}
	if a.hasDrafts() || a.animation.Time != 0 {
		t.Fatal("zoom edited the clip or moved its playhead")
	}
	if !a.updateAnimationPointer(input.Pointer{X: x, Y: y, Pressed: true, Down: true}) || abs(a.animation.Time-.35) > .001 {
		t.Fatal("scrubbing ignored the zoomed ruler's scroll offset", a.animation.Time)
	}
	a.updateAnimationPointer(input.Pointer{Released: true})
	if a.animation.DragTime {
		t.Fatal("scrubbing did not release")
	}
	scroll := a.animationTimelineScroll(a.world)
	if scroll == nil || !scroll.Horizontal || scroll.Position[0] >= 0 {
		t.Fatal("zoomed timeline has no horizontal scrolling", scroll)
	}
	position := scroll.Position
	animationClick(t, a, "key:float:0:1")
	syncAnimationLayoutTest(t, a)
	if a.animationTimelineScroll(a.world).Position != position {
		t.Fatal("key selection reset the zoomed timeline scroll")
	}
}

func TestWorkspaceAnimationHasOneHorizontalScrollContainer(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	a.animation.Zoom = 4
	for _, width := range []float32{1600, 900} {
		a.width = width
		syncAnimationLayoutTest(t, a)
		control, ok := htmlui.ControlByID(a.world, a.shell, "animation:seek-start")
		if !ok {
			t.Fatal("ruler control missing")
		}
		containers := 0
		for entity := control; entity != 0; {
			if scroll, ok := a.world.Get[scene.UIScroll](entity); ok && scroll.Horizontal {
				containers++
			}
			parent, ok := a.world.Get[scene.Parent](entity)
			if !ok {
				break
			}
			entity = parent.Entity
		}
		if containers != 1 {
			t.Fatalf("width %g: timeline has %d horizontal scroll containers", width, containers)
		}
	}
}

func TestWorkspaceAnimationWheelZoomBoundsAndAllocations(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	syncAnimationLayoutTest(t, a)
	bounds, _ := a.animationTimelineBounds()
	in := input.NewState()
	for _, point := range [][2]float32{{bounds.X - 1, bounds.Y + 10}, {bounds.X + 10, bounds.Y - 1}, {bounds.X + bounds.W, bounds.Y + 10}} {
		in.EndFrame()
		in.MoveMouse(float64(point[0]), float64(point[1]))
		in.ScrollMouse(0, 1)
		if a.updateAnimationWheel(in) || in.Pointer().WheelY != 1 || a.animation.Zoom != 1 {
			t.Fatal("timeline intercepted a wheel gesture outside its bounds", point)
		}
	}
	in.MoveMouse(float64(bounds.X+bounds.W/2), float64(bounds.Y+bounds.H/2))
	a.menuOpen = "file"
	if a.updateAnimationWheel(in) || a.animation.Zoom != 1 {
		t.Fatal("timeline zoomed under an open menu")
	}
	a.menuOpen = ""
	for _, sample := range []struct{ wheel, want float32 }{{1000, 64}, {-1000, 1}} {
		in.EndFrame()
		in.ScrollMouse(0, float64(sample.wheel))
		a.updateAnimationWheel(in)
		if a.animation.Zoom != sample.want || in.Pointer().WheelY != 0 {
			t.Fatal("timeline zoom exceeded its limits", a.animation.Zoom)
		}
		syncAnimationLayoutTest(t, a)
	}
	allocs := testing.AllocsPerRun(20, func() {
		in.EndFrame()
		a.animation.Zoom = 1
		in.ScrollMouse(0, 1)
		a.updateAnimationWheel(in)
	})
	if allocs != 0 {
		t.Fatalf("timeline zoom input allocates: %g", allocs)
	}
}

func animationTestNode(t *testing.T, a *ModuleEditor, id string) *dom.Node {
	t.Helper()
	source, err := htmlui.Serialize(a.world, a.shell)
	if err != nil {
		t.Fatal(err)
	}
	root, err := dom.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	var find func(*dom.Node) *dom.Node
	find = func(node *dom.Node) *dom.Node {
		if value, _ := node.Attr("id"); value == id {
			return node
		}
		for _, child := range node.Children {
			if found := find(child); found != nil {
				return found
			}
		}
		return nil
	}
	node := find(root)
	if node == nil {
		t.Fatalf("missing animation node %q", id)
	}
	return node
}

func TestWorkspaceAnimationDisplayRangeClipsTracksWithoutEditingClip(t *testing.T) {
	a := animationEditorFixture(t)
	if err := a.Document.EditMod("button", "Add range test keys", func(mod *project.ModSpec) error {
		clip := &mod.Elements[0].Components.Animation.Clip
		clip.FloatTracks[0].Keyframes = []project.AnimationKeyframeSpec[float32]{{Time: 0, Value: 4}, {Time: .5, Value: 54}, {Time: 1, Value: 104}}
		clip.TextureTracks = []project.AnimationTrackSpec[project.TextureSpec]{{Binding: scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture}, Keyframes: []project.AnimationKeyframeSpec[project.TextureSpec]{{Time: 0, Value: project.TextureSpec{Color: "#ffffff"}}, {Time: .5, Value: project.TextureSpec{Color: "#ff0000"}}, {Time: 1, Value: project.TextureSpec{Color: "#000000"}}}}}
		clip.BoolTracks = []project.AnimationTrackSpec[bool]{{Binding: scene.AnimationBinding{Target: scene.AnimationTargetVisibility}, Keyframes: []project.AnimationKeyframeSpec[bool]{{Time: 0, Value: true}, {Time: .5, Value: false}, {Time: 1, Value: true}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	animationClick(t, a, "open")
	before := encodeFormJSON(animationFixtureClip(a))
	a.animation.Time, a.animation.Playing = .5, true
	animationChange(t, a, "range-start", ".25")
	animationChange(t, a, "range-end", ".75")
	source := workspaceShellTest(t, a)
	for _, kind := range []string{"float", "texture", "bool"} {
		for _, key := range []string{"0", "2"} {
			if strings.Contains(source, `id="animation:key:`+kind+`:0:`+key+`"`) {
				t.Fatalf("%s track rendered key %s outside the view", kind, key)
			}
		}
		for _, sample := range []struct{ id, style string }{
			{"animation:key:" + kind + ":0:1", "left:50%"},
			{"animation:segment:" + kind + ":0:0", "left:0%;width:50%"},
			{"animation:segment:" + kind + ":0:1", "left:50%;width:50%"},
			{"animation:playhead:" + kind + ":0", "left:50%"},
		} {
			if style, _ := animationTestNode(t, a, sample.id).Attr("style"); style != sample.style {
				t.Fatalf("%s style=%q, want %q", sample.id, style, sample.style)
			}
		}
	}
	if _, ok := a.fields["animation:range-start"]; ok || a.hasDrafts() || encodeFormJSON(animationFixtureClip(a)) != before || a.animation.Time != .5 || !a.animation.Playing {
		t.Fatal("display range changed animation data, playback, or document drafts")
	}
	animationClick(t, a, "seek-start")
	if a.animation.Time != .25 || a.animation.Playing {
		t.Fatal("start tick did not seek to the view start")
	}
	animationClick(t, a, "seek:0.75")
	if a.animation.Time != .75 {
		t.Fatal("end tick did not seek to the view end")
	}
	a.handleEvent(htmlui.Event{ID: "resources-tab", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "animation-tab", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if start, end := a.animation.timelineRange(1); start != .25 || end != .75 {
		t.Fatal("switching tabs lost the view range", start, end)
	}
	animationClick(t, a, "range-reset")
	source = workspaceShellTest(t, a)
	if start, end := a.animation.timelineRange(1); start != 0 || end != 1 || !strings.Contains(source, `id="animation:key:float:0:0"`) || !strings.Contains(source, `id="animation:key:float:0:2"`) {
		t.Fatal("Show all did not restore the full timeline")
	}
}

func TestWorkspaceAnimationDisplayRangeValidationAndZoomedScrubbing(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationChange(t, a, "range-start", ".25")
	animationChange(t, a, "range-end", ".75")
	for _, sample := range []struct{ name, value string }{
		{"range-start", "-1"}, {"range-start", ".75"}, {"range-start", ".8"},
		{"range-end", ".25"}, {"range-end", ".1"}, {"range-end", "1.1"},
		{"range-start", "NaN"}, {"range-end", "+Inf"}, {"range-end", ""},
	} {
		a.status = ""
		animationChange(t, a, sample.name, sample.value)
		if start, end := a.animation.timelineRange(1); start != .25 || end != .75 || a.status == "" || a.hasDrafts() {
			t.Fatalf("invalid %s=%q changed view range or was not reported", sample.name, sample.value)
		}
	}
	syncAnimationLayoutTest(t, a)
	ruler, _ := a.animationRulerBounds()
	x, y := ruler.X+ruler.W*.35, ruler.Y+ruler.H/2
	in := input.NewState()
	in.MoveMouse(float64(x), float64(y))
	in.ScrollMouse(0, 4)
	if !a.updateAnimationWheel(in) {
		t.Fatal("range timeline did not zoom")
	}
	syncAnimationLayoutTest(t, a)
	ruler, _ = a.animationRulerBounds()
	if abs(ruler.X+ruler.W*.35-x) > .01 || a.animationTimelineScroll(a.world) == nil {
		t.Fatal("nonzero view start broke the zoom anchor or scrolling")
	}
	if !a.updateAnimationPointer(input.Pointer{X: x, Y: y, Pressed: true, Down: true}) || abs(a.animation.Time-.425) > .001 {
		t.Fatal("range scrubbing did not map to clip time", a.animation.Time)
	}
	for _, sample := range []struct{ x, want float32 }{{ruler.X - 10, .25}, {ruler.X + ruler.W + 10, .75}} {
		a.updateAnimationPointer(input.Pointer{X: sample.x, Y: y, Down: true})
		if a.animation.Time != sample.want {
			t.Fatal("range scrubbing exceeded view bounds", a.animation.Time)
		}
	}
	a.updateAnimationPointer(input.Pointer{Released: true})
	animationClick(t, a, "range-reset")
	syncAnimationLayoutTest(t, a)
	if a.animation.Zoom != 1 || a.animationTimelineScroll(a.world).Position[0] != 0 {
		t.Fatal("reset retained the previous view's zoom or horizontal scroll")
	}
	if start, end := a.animation.timelineRange(2); start != 0 || end != 2 {
		t.Fatal("full view did not follow the clip duration")
	}
	a.animation.RangeStart, a.animation.RangeEnd = .5, .75
	if start, end := a.animation.timelineRange(.4); start != 0 || end != .4 {
		t.Fatal("a shorter clip left an empty view range")
	}
	animationChange(t, a, "range-end", ".50000006")
	a.animation.Zoom = 64
	workspaceShellTest(t, a)
	buttons := 0
	for _, node := range animationTestNode(t, a, "animation-ruler").Children {
		if node.Data == "button" {
			buttons++
		}
	}
	if buttons != 2 {
		t.Fatalf("a range one float32 step wide rendered %d ticks instead of its two endpoints", buttons)
	}
}

func TestWorkspaceAnimationDisplayRangePreviewPlayhead(t *testing.T) {
	a := animationEditorFixture(t)
	animationClick(t, a, "open")
	animationChange(t, a, "range-start", ".25")
	animationChange(t, a, "range-end", ".75")
	a.syncDraftPreview()
	a.Preview.Sync(a.previewDocument(), a.Local)
	if err := a.syncViewport2D(); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		time  float32
		style string
	}{{.24, "display:none"}, {.25, "left:0%"}, {.5, "left:50%"}, {.75, "left:100%"}, {.76, "display:none"}} {
		a.animation.Time = sample.time
		a.syncAnimationPreview(0)
		a.updateAnimationPlaybackUI()
		for _, id := range []string{"animation:playhead:ruler", "animation:playhead:float:0"} {
			if style, _ := animationTestNode(t, a, id).Attr("style"); style != sample.style {
				t.Fatalf("time %g: %s style=%q, want %q", sample.time, id, style, sample.style)
			}
		}
	}
	a.animation.Time, a.animation.Playing = .75, true
	a.syncAnimationPreview(.05)
	if !a.animation.Playing || abs(a.animation.Time-.8) > .001 {
		t.Fatal("display range restricted preview playback")
	}
	a.updateAnimationPlaybackUI()
	if allocations := testing.AllocsPerRun(20, func() {
		a.syncAnimationPreview(0)
		a.updateAnimationPlaybackUI()
	}); allocations != 0 {
		t.Fatalf("idle preview with a display range allocated %g", allocations)
	}
}
