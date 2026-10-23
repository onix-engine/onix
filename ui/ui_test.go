package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
	"github.com/diiyw/onix/images/svg"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/go-gl/mathgl/mgl32"
)

func TestAllBoxUIComponentsSupportCornerRadius(t *testing.T) {
	world := newUIWorld(t)
	radius := CornerRadius(6)

	SpawnPanel(world, PanelOptions{Width: 120, Height: 80, Radius: radius})
	SpawnDialog(world, DialogOptions{Width: 120, Height: 80, Radius: radius})
	SpawnButton(world, ButtonOptions{Width: 80, Height: 32, Label: "Run", Radius: radius})
	SpawnInput(world, TextOptions{Width: 120, Height: 32, Radius: radius})
	SpawnNumberInput(world, NumberInputOptions{Width: 120, Height: 32, Radius: radius})
	SpawnTextarea(world, TextOptions{Width: 120, Height: 64, Radius: radius})
	SpawnMenu(world, MenuOptions{Width: 120, Height: 32, Items: []string{"A", "B"}, Radius: radius})
	SpawnContextMenu(world, ContextMenuOptions{Width: 120, ItemHeight: 32, Items: []string{"A", "B"}, Open: true, Radius: radius})
	SpawnCheckbox(world, CheckboxOptions{Width: 120, Height: 32, Label: "Enabled", Radius: radius})
	SpawnSelector(world, SelectorOptions{Width: 120, Height: 32, Options: []string{"A", "B"}, ListHeight: 64, Radius: radius})
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 80, Items: tabItems("A", "B"), Radius: radius})
	SpawnTabPanel(world, tabs, TabPanelOptions{Index: 0, Radius: radius})
	SpawnProgress(world, ProgressOptions{Width: 120, Height: 12, Min: 0, Max: 1, Value: 0.5, Radius: radius})
	SpawnScrollArea(world, ScrollAreaOptions{Width: 80, Height: 60, ContentWidth: 160, ContentHeight: 120, Radius: radius})
	SpawnTree(world, TreeOptions[string]{
		Width: 120, RowHeight: 32, Radius: radius,
		Nodes: []TreeNodeSpec[string]{{ID: "root", Label: "Root"}, {ID: "leaf", Label: "Leaf", Leaf: true}},
	})

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync ui: %v", err)
	}
	if err := SyncTreeSystem[string]("").Run(world); err != nil {
		t.Fatalf("sync tree: %v", err)
	}

	count := 0
	for entity, access := range ecs.Query[CornerRadius](world).All {
		current := access.Mut()

		if ecs.Has[ProgressThumb](world, entity) {
			continue
		}
		if element, ok := ecs.Get[Relation[SurfaceStyle]](world, entity); ok && ecs.Has[Part[BodyPart]](world, entity) && ecs.Has[ProgressThumb](world, element.Owner) {
			continue
		}
		if ecs.Has[Control](world, entity) && !ecs.Has[scene.UIBackground2D](world, entity) {
			continue
		}
		surface := uiSurfaceSnapshot(world, entity)
		if surface == nil {
			continue
		}
		count++
		if *current != radius {
			t.Fatalf("entity %v radius = %v, want %v", entity, *current, radius)
		}
		if surface.Shape.Kind != render.ShapeRoundedRect {
			t.Fatalf("entity %v did not use a rounded rectangle shape", entity)
		}
		want := radius.pixels(surface.Width, surface.Height)
		if surface.Shape.Radius != want {
			t.Fatalf("entity %v rendered radius = %v, want %v", entity, surface.Shape.Radius, want)
		}
	}
	if count != 38 {
		t.Fatalf("rounded surface count = %d, want 38", count)
	}
}

func TestWindowTitleBarLaysOutControlsAndEmitsAction(t *testing.T) {
	world := newUIWorld(t)
	bar := SpawnWindowTitleBar(world, WindowTitleBarOptions{
		X: 10, Y: 20, Width: 180, Height: 32,
		Items: []WindowTitleBarItem{
			{Action: WindowTitleBarMinimize},
			{Action: WindowTitleBarMaximize},
			{Action: WindowTitleBarClose},
		},
	})
	if !UpdateWindowTitleBar(world, bar, WindowTitleBarOptions{
		X: 10, Y: 20, Width: 180, Height: 32,
		Items: []WindowTitleBarItem{{Action: WindowTitleBarMinimize}, {Action: WindowTitleBarMaximize}, {Action: WindowTitleBarClose}},
	}) {
		t.Fatal("expected title bar update to succeed")
	}
	titleBar, _ := ecs.Get[WindowTitleBar](world, bar)
	if len(titleBar.Buttons) != 3 {
		t.Fatalf("title bar button count = %d, want 3", len(titleBar.Buttons))
	}
	transform, _ := ecs.Get[scene.UITransform](world, titleBar.Buttons[0])
	if transform.Translation.X.Value != 42 {
		t.Fatalf("first title bar button x = %v, want 42", transform.Translation.X.Value)
	}
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 150, 36)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press title bar button: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 150, 36)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release title bar button: %v", err)
	}
	events := (&ecs.MessageReader[WindowTitleBarEvent]{}).Read(world)
	if len(events) != 1 || events[0].Bar != bar || events[0].Action != WindowTitleBarClose {
		t.Fatalf("title bar events = %#v", events)
	}
}

func TestDialogIsRoundedOutlinedPanel(t *testing.T) {
	world := ecs.NewWorld()
	radius := CornerRadius(8)
	border := graphics.ColorRGBA{R: 1, G: 0.5, B: 0.25, A: 1}
	entity := SpawnDialog(world, DialogOptions{
		Width: 160, Height: 90, Radius: radius, Border: border, BorderWidth: 2,
		Theme: Theme{Background: graphics.ColorRGBA{R: 0.1, G: 0.2, B: 0.3, A: 1}},
	})

	if !ecs.Has[Panel](world, entity) || !ecs.Has[Dialog](world, entity) {
		t.Fatal("dialog must retain panel behavior and expose the dialog component")
	}
	background, backgroundOK := ecs.Get[scene.UIBackground2D](world, entity)
	outline, outlineOK := ecs.Get[scene.UIBorder2D](world, entity)
	if !backgroundOK || !outlineOK || background.Shape.Kind != render.ShapeRoundedRect || background.Shape.Radius != float32(radius) || outline.Shape.Border != border || outline.Shape.BorderWidth != 2 {
		t.Fatalf("unexpected dialog layers: background=%+v border=%+v", background, outline)
	}
}

func TestInputJoinedEdgeIsSquare(t *testing.T) {
	world := newUIWorld(t)
	input := SpawnInput(world, TextOptions{Width: 120, Height: 32, Radius: 6, JoinRight: true})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for entity, access := range ecs.Query2[joinedEdge, scene.UITransform](world).All {
		edge := access.A.Mut()
		transform := access.B.Mut()

		if edge.Owner != input || edge.Side != joinedEdgeRight {
			continue
		}
		found = true
		surface := uiSurfaceSnapshot(world, entity)
		if surface == nil || transform.Translation.X.Value != 114 || surface.Width != 6 || surface.Height != 32 || surface.Shape.Radius != 0 {
			t.Fatalf("joined input edge = transform=%+v surface=%+v", transform, surface)
		}
		break
	}
	if !found {
		t.Fatal("joined input edge was not spawned")
	}
}

func TestSurfaceButtonUsesSurfaceColors(t *testing.T) {
	world := newUIWorld(t)
	theme := DefaultTheme()
	button := SpawnButton(world, ButtonOptions{Width: 80, Height: 32, Label: "Up", Variant: ButtonSurface, Theme: theme})
	pointer := ecs.GetMut[scene.InputState2D](world, button)

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := uiSurfaceSnapshot(world, button).Shape.Fill; got != theme.Surface {
		t.Fatalf("surface button color = %+v, want %+v", got, theme.Surface)
	}
	pointer.Hovered = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := uiSurfaceSnapshot(world, button).Shape.Fill; got != theme.SurfaceHover {
		t.Fatalf("hovered surface button color = %+v, want %+v", got, theme.SurfaceHover)
	}
	pointer.Pressed = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := uiSurfaceSnapshot(world, button).Shape.Fill; got != theme.SurfaceActive {
		t.Fatalf("pressed surface button color = %+v, want %+v", got, theme.SurfaceActive)
	}
}

func TestCircleCornerRadiusUsesHalfTheSurfaceSize(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 40, Height: 40, Radius: Circle})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	surface := uiSurfaceSnapshot(world, button)
	if surface == nil || surface.Shape.Kind != render.ShapeRoundedRect || surface.Shape.Radius != 20 {
		t.Fatalf("circle surface = %#v", surface)
	}
}

func TestTabCornerRadiusKeepsJoinedEdgesSquare(t *testing.T) {
	world := newUIWorld(t)
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 40, Items: tabItems("A", "B"), Radius: 6})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	edgeCount := 0
	for entity, access := range ecs.Query2[joinedEdge, scene.UITransform](world).All {
		edge := access.A.Mut()
		transform := access.B.Mut()

		item, ok := ecs.Get[TabItem](world, edge.Owner)
		if !ok || item.Tabs != tabs {
			continue
		}
		edgeCount++
		width, height, _ := ControlBounds(world, edge.Owner)
		wantX := float32(0)
		if edge.Side == joinedEdgeRight {
			wantX = width - 6
		}
		surface := uiSurfaceSnapshot(world, entity)
		if surface == nil || transform.Translation.X.Value != wantX || surface.Width != 6 || surface.Height != height {
			t.Fatalf("joined edge layout = transform=%#v surface=%#v", transform, surface)
		}
		if surface.Shape.Kind != render.ShapeRoundedRect || surface.Shape.Radius != 0 {
			t.Fatalf("joined edge must be square: %#v", surface.Shape)
		}
	}
	if edgeCount != 2 {
		t.Fatalf("joined edge count = %d, want 2", edgeCount)
	}
}

func TestTabFrameBorderWrapsHeaderAboveItems(t *testing.T) {
	world := newUIWorld(t)
	borderColor := graphics.ColorFromRGBA(40, 50, 60, 255)
	style := DefaultSkin(DefaultTheme()).Tab
	style.Frame = style.Frame.WithBorder(
		scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, borderColor, 0, 1),
	)
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 80, Items: tabItems("A", "B"), Radius: 6, Style: &style})
	SpawnTabPanel(world, tabs, TabPanelOptions{Index: 0, Style: &style})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	borderEntity := findSurfacePart[FrameBorderPart](world, tabs)
	border, borderOK := ecs.Get[scene.UIBorder2D](world, borderEntity)
	size, sizeOK := ecs.Get[scene.Size2D](world, borderEntity)
	borderStack, stackOK := ecs.Get[scene.UIStackIndex](world, borderEntity)
	if borderEntity == 0 || !borderOK || border.Shape.Border != borderColor || border.Shape.BorderWidth != 1 || border.Shape.Radius != 6 {
		t.Fatalf("tab frame border = entity:%d border:%#v", borderEntity, border)
	}
	if !sizeOK || size.Width != 120 || size.Height != 36 || !stackOK {
		t.Fatalf("tab frame geometry = size:%#v stack:%d", size, borderStack)
	}
	rootBorder, _ := ecs.Get[scene.UIBorder2D](world, tabs)
	if rootBorder.Shape.Kind != 0 {
		t.Fatalf("tab card still has a border: %#v", rootBorder)
	}
	for entity, access := range ecs.Query[TabItem](world).All {
		item := access.Mut()

		if item.Tabs == tabs {
			itemStack, _ := ecs.Get[scene.UIStackIndex](world, entity)
			if borderStack <= itemStack {
				t.Fatalf("tab border stack %d does not cover item stack %d", borderStack, itemStack)
			}
		}
	}
}

func TestTabUsesConfiguredHighlightColor(t *testing.T) {
	world := newUIWorld(t)
	theme := DefaultTheme()
	highlight := graphics.ColorFromRGBA(18, 96, 144, 255)
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 40, Items: tabItems("A", "B"), Highlight: highlight, Theme: theme})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	selectedFound := false
	for entity, access := range ecs.Query[TabItem](world).All {
		item := access.Mut()

		if item.Tabs == tabs && item.Index == 0 {
			selectedFound = true
			if got := uiSurfaceSnapshot(world, entity).Shape.Fill; got != highlight {
				t.Fatalf("selected tab color = %+v, want %+v", got, highlight)
			}
		}
	}
	if !selectedFound {
		t.Fatal("selected tab item not found")
	}
}

func TestTabDefaultsToThemeAccentHighlight(t *testing.T) {
	world := newUIWorld(t)
	theme := DefaultTheme()
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 40, Items: tabItems("A", "B"), Theme: theme})
	tab, _ := ecs.Get[Tab](world, tabs)
	if tab.Highlight != theme.Accent {
		t.Fatalf("default tab highlight = %+v, want theme accent %+v", tab.Highlight, theme.Accent)
	}
}

func TestButtonGroupJoinsButtonsAndSupportsOptionalDividers(t *testing.T) {
	world := newUIWorld(t)
	frameColor := graphics.ColorFromRGBA(40, 50, 60, 255)
	dividerColor := graphics.ColorFromRGBA(90, 100, 110, 255)
	style := DefaultSkin(DefaultTheme()).ButtonGroup
	style.Frame = ColorSurface(graphics.ColorFromRGBA(240, 245, 250, 255)).WithBorder(
		scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, frameColor, 0, 1),
	)
	groupEntity := SpawnButtonGroup(world, ButtonGroupOptions[string]{
		X: 10, Y: 20, Height: 32, Radius: 6,
		Items: []ButtonGroupItem[string]{
			{Value: "save", Width: 40, Label: "Save"},
			{Value: "reload", Width: 50, Label: "Reload"},
			{Value: "focus", Width: 60, Label: "Focus"},
		},
		Divider: &ButtonGroupDividerOptions{Width: 1, Inset: 4, Color: dividerColor},
		Style:   &style,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	group, ok := ecs.Get[ButtonGroup](world, groupEntity)
	if !ok || len(group.Buttons) != 3 || group.Width != 150 || group.Height != 32 {
		t.Fatalf("button group layout = %#v", group)
	}
	frameEntity := findSurfacePart[FrameBorderPart](world, groupEntity)
	frameSize, sizeOK := ecs.Get[scene.Size2D](world, frameEntity)
	frame, frameOK := ecs.Get[scene.UIBorder2D](world, frameEntity)
	frameStack, stackOK := ecs.Get[scene.UIStackIndex](world, frameEntity)
	if !sizeOK || frameSize.Width != 150 || frameSize.Height != 32 || !frameOK || frame.Shape.Border != frameColor || frame.Shape.BorderWidth != 1 || frame.Shape.Radius != 6 {
		t.Fatalf("button group frame = size:%#v border:%#v", frameSize, frame)
	}
	values := []string{"save", "reload", "focus"}
	for index, wantX := range []float32{0, 40, 90} {
		button := group.Buttons[index]
		transform, _ := ecs.Get[scene.UITransform](world, button)
		value, _ := ecs.Get[ButtonGroupValue[string]](world, button)
		if transform.Translation.X.Value != wantX || transform.Translation.Y.Value != 0 {
			t.Fatalf("button %d position = %#v, want x=%v", index, transform, wantX)
		}
		if value.Group != groupEntity || value.Index != index || value.Value != values[index] {
			t.Fatalf("button %d group value = %#v", index, value)
		}
		buttonStack, _ := ecs.Get[scene.UIStackIndex](world, button)
		if !stackOK || frameStack <= buttonStack {
			t.Fatalf("button group frame stack %d does not cover button stack %d", frameStack, buttonStack)
		}
	}

	edgeCount := 0
	for _, access := range ecs.Query[joinedEdge](world).All {
		edge := access.Mut()

		item, ok := ecs.Get[buttonGroupItem](world, edge.Owner)
		if ok && item.Group == groupEntity {
			edgeCount++
		}
	}
	if edgeCount != 4 {
		t.Fatalf("joined edge count = %d, want 4", edgeCount)
	}

	dividerCount := 0
	for entity, access := range ecs.Query2[buttonGroupDivider, scene.UITransform](world).All {
		divider := access.A.Mut()
		transform := access.B.Mut()

		if divider.Group != groupEntity {
			continue
		}
		wantX := []float32{39.5, 89.5}[divider.Index]
		surface := uiSurfaceSnapshot(world, entity)
		if surface == nil || transform.Translation.X.Value != wantX || transform.Translation.Y.Value != 4 || surface.Width != 1 || surface.Height != 24 {
			t.Fatalf("divider %d layout = transform=%#v surface=%#v", divider.Index, transform, surface)
		}
		if surface.Shape.Fill != dividerColor || surface.Shape.Radius != 0 {
			t.Fatalf("divider %d shape = %#v", divider.Index, surface.Shape)
		}
		dividerCount++
	}
	if dividerCount != 2 {
		t.Fatalf("divider count = %d, want 2", dividerCount)
	}

	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 60, 30)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press grouped button: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 60, 30)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release grouped button: %v", err)
	}
	events := (&ecs.MessageReader[scene.InputEvent]{}).Read(world)
	if !hasInputEvent(events, group.Buttons[1], scene.InputEventTap) {
		t.Fatalf("grouped button did not receive a input tap: %#v", events)
	}

	plainGroup := SpawnButtonGroup(world, ButtonGroupOptions[int]{
		Height: 24,
		Items:  []ButtonGroupItem[int]{{Value: 1, Width: 30}, {Value: 2, Width: 30}},
	})
	plainFrame, plainFrameOK := ecs.Get[scene.UIBorder2D](world, findSurfacePart[FrameBorderPart](world, plainGroup))
	if plainFrameOK && plainFrame.Shape.Kind != 0 {
		t.Fatalf("default button group has a border: %#v", plainFrame)
	}
	for _, access := range ecs.Query[buttonGroupDivider](world).All {
		divider := access.Mut()

		if divider.Group == plainGroup {
			t.Fatal("button group without divider options rendered a divider")
		}
	}
}

func TestButtonReceivesInputTapOnRelease(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{X: 10, Y: 20, Width: 120, Height: 36, Label: "Run"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 24, 30)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 24, 30)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release: %v", err)
	}

	events := (&ecs.MessageReader[scene.InputEvent]{}).Read(world)
	if !hasInputEvent(events, button, scene.InputEventTap) {
		t.Fatalf("unexpected events: %#v", events)
	}
	for _, event := range events {
		if event.Entity == button && event.Kind == scene.InputEventTap && !event.Handled {
			t.Fatal("UI input tap was not marked handled")
		}
	}
}

func TestFocusedInputSystemRoutesAndConsumesTextFieldInput(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 120, Height: 32})
	handlerCalled := false
	if err := ecs.Insert(world, field, scene.FocusedInputHandler{Handle: func(context *scene.FocusedInputContext) {
		handlerCalled = context.Entity == field && context.Event.Kind == input.EventKeyDown
	}}); err != nil {
		t.Fatal(err)
	}
	ecs.InsertResource(world, &scene.InputFocus{Keyboard: field})
	state, _ := ecs.GetResource[*input.State](world)
	event := input.Event{Seq: 7, Kind: input.EventKeyDown, Key: input.KeyA}
	ecs.WriteMessage(world, event)

	if err := FocusedInputSystem("").Run(world); err != nil {
		t.Fatalf("dispatch focused input: %v", err)
	}
	events := (&ecs.MessageReader[scene.FocusedInputEvent]{}).Read(world)
	if len(events) != 1 || events[0].Entity != field || events[0].Event != event || !events[0].Handled {
		t.Fatalf("unexpected focused input events: %#v", events)
	}
	if !handlerCalled {
		t.Fatal("focused input handler was not called")
	}
	if !state.EventConsumed(event.Seq) {
		t.Fatal("focused text input did not consume raw event")
	}
}

func TestFocusedInputHonorsInheritedModuleActivity(t *testing.T) {
	w := newUIWorld(t)
	b := ecs.NewBundle(1)
	ecs.Put(&b, scene.Activity{Active: false})
	parent := w.Spawn(b)
	field := SpawnInput(w, TextOptions{Width: 120, Height: 32})
	if err := ecs.Insert(w, field, scene.Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := ecs.Insert(w, field, scene.FocusedInputHandler{Handle: func(*scene.FocusedInputContext) { calls++ }}); err != nil {
		t.Fatal(err)
	}
	ecs.InsertResource(w, &scene.InputFocus{Keyboard: field})
	system := FocusedInputSystem("")
	ecs.WriteMessage(w, input.Event{Seq: 1, Kind: input.EventKeyDown, Key: input.KeyA})
	if err := system.Run(w); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*input.State](w)
	if calls != 0 || state.EventConsumed(1) {
		t.Fatal("inactive focused control consumed keyboard input")
	}
	ecs.GetMut[scene.Activity](w, parent).Active = true
	if err := system.Run(w); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("resume replayed keyboard input from suspension")
	}
	ecs.WriteMessage(w, input.Event{Seq: 2, Kind: input.EventKeyDown, Key: input.KeyA})
	if err := system.Run(w); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("resumed focus stopped receiving keyboard input")
	}
}

func TestClickingDifferentButtonsEmitsEachTapOnFirstClick(t *testing.T) {
	world := newUIWorld(t)
	first := SpawnButton(world, ButtonOptions{Width: 80, Height: 32, Label: "1"})
	second := SpawnButton(world, ButtonOptions{X: 90, Width: 80, Height: 32, Label: "2"})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	reader := ecs.MessageReader[scene.InputEvent]{}
	click := func(entity ecs.Entity, x float64) {
		t.Helper()
		state.PressMouse(input.MouseLeft, x, 16)
		if err := runUIUpdate(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
		state.ReleaseMouse(input.MouseLeft, x, 16)
		if err := runUIUpdate(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
		if events := reader.Read(world); !hasInputEvent(events, entity, scene.InputEventTap) {
			t.Fatalf("entity %d did not receive tap: %#v", entity, events)
		}
	}
	click(first, 20)
	click(second, 110)
}

func TestPanelBlocksButtonHoverUnlessConfiguredToPassThrough(t *testing.T) {
	for _, passThrough := range []bool{false, true} {
		world := newUIWorld(t)
		button := SpawnButton(world, ButtonOptions{Width: 100, Height: 40, Label: "Run"})
		SpawnPanel(world, PanelOptions{
			Width: 100, Height: 40, PassThrough: passThrough,
			Theme: Theme{StackIndex: DefaultTheme().StackIndex + 1},
		})
		syncTransforms(t, world)
		state, _ := ecs.GetResource[*input.State](world)
		state.PressMouse(input.MouseLeft, 10, 10)
		if err := runUIUpdate(world); err != nil {
			t.Fatal(err)
		}
		pointer := ecs.GetMut[scene.InputState2D](world, button)
		if pointer.Hovered != passThrough {
			t.Fatalf("passThrough=%v produced hovered=%v", passThrough, pointer.Hovered)
		}
	}
}

func TestButtonWithoutWidthMeasuresItsContent(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Height: 36, Label: "Run", Icon: IconStyle{Fallback: ">", Width: 16}})
	width, _, ok := ControlBounds(world, button)
	if !ok || width <= 16+textIconX+textIconGap+14 {
		t.Fatalf("expected content-sized button width, got %v", width)
	}
	target := ecs.GetMut[scene.InputTarget2D](world, button)
	if target == nil || target.Width != width {
		t.Fatalf("expected input target to use measured width: width=%v target=%#v", width, target)
	}
}

func TestButtonEmitsTapForPressAndReleaseInOneFrame(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 100, Height: 32, Label: "Run"})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 10, 10)
	state.ReleaseMouse(input.MouseLeft, 10, 10)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("same-frame click: %v", err)
	}
	events := (&ecs.MessageReader[scene.InputEvent]{}).Read(world)
	if !hasInputEvent(events, button, scene.InputEventTap) {
		t.Fatalf("expected the first complete click to emit a input tap: %#v", events)
	}
}

func hasInputEvent(events []scene.InputEvent, entity ecs.Entity, kind scene.InputEventKind) bool {
	for _, event := range events {
		if event.Entity == entity && event.Kind == kind {
			return true
		}
	}
	return false
}

func TestButtonTextIsVerticallyCentered(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 120, Height: 40, Label: "Run", FontSize: 12})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	found := false
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner != button || !ecs.Has[Part[LabelPart]](world, entity) {
			continue
		}
		found = true
		_, lineHeight, ok := scene.MeasureTextValue2D(text, text.Value)
		if !ok {
			t.Fatal("expected button text to be measurable")
		}
		want := (float32(40) - lineHeight) * 0.5
		if math.Abs(float64(transform.Translation.Y.Value-want)) > 0.001 {
			t.Fatalf("expected centered button text y=%v, got %v", want, transform.Translation.Y.Value)
		}
		break
	}
	if !found {
		t.Fatal("expected button label text")
	}
}

func TestInputTextAndCaretAreVerticallyCentered(t *testing.T) {
	world := newUIWorld(t)
	const height = float32(28)
	field := SpawnInput(world, TextOptions{Width: 160, Height: height, Value: "value"})
	component := ecs.GetMut[TextField](world, field)
	component.Focused = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var textY, lineHeight float32
	foundText := false
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner != field || !ecs.Has[Part[ValuePart]](world, entity) {
			continue
		}
		_, lineHeight, _ = scene.MeasureTextValue2D(text, text.Value)
		textY = transform.Translation.Y.Value
		foundText = true
		break
	}
	if !foundText {
		t.Fatal("expected input value text")
	}
	wantY := (height - lineHeight) * 0.5
	if math.Abs(float64(textY-wantY)) > 0.001 {
		t.Fatalf("input text y=%v, want %v", textY, wantY)
	}

	var caretY float32
	foundCaret := false
	for entity, access := range ecs.Query3[Relation[SurfaceStyle], scene.UITransform, scene.Size2D](world).All {
		visual := access.A.Mut()
		transform := access.B.Mut()
		size := access.C.Mut()

		if visual.Owner != field || !ecs.Has[Part[CaretPart]](world, entity) {
			continue
		}
		caretY = transform.Translation.Y.Value
		if math.Abs(float64(size.Height-lineHeight)) > 0.1 {
			t.Fatalf("caret height=%v, want %v", size.Height, lineHeight)
		}
		foundCaret = true
		break
	}
	if !foundCaret {
		t.Fatal("expected input caret")
	}
	if math.Abs(float64(caretY-wantY)) > 0.001 {
		t.Fatalf("caret y=%v, want %v", caretY, wantY)
	}
}

func TestButtonWithoutIconCentersTextHorizontally(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 120, Height: 40, Label: "2D", FontSize: 12})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	found := false
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner != button || !ecs.Has[Part[LabelPart]](world, entity) {
			continue
		}
		found = true
		width, _, ok := scene.MeasureTextValue2D(text, text.Value)
		if !ok {
			t.Fatal("expected button text to be measurable")
		}
		want := (float32(120) - width) * 0.5
		if math.Abs(float64(transform.Translation.X.Value-want)) > 0.001 {
			t.Fatalf("expected centered button text x=%v, got %v", want, transform.Translation.X.Value)
		}
		break
	}
	if !found {
		t.Fatal("expected button label text")
	}
}

func TestButtonAndSelectorCanJoinOnlyTheirInnerEdges(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 40, Height: 30, Radius: 4, JoinLeft: true})
	selector := SpawnSelector(world, SelectorOptions{Width: 80, Height: 30, Options: []string{"D:\\"}, Radius: 4, JoinRight: true})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	want := map[ecs.Entity]joinedEdgeSide{button: joinedEdgeLeft, selector: joinedEdgeRight}
	got := map[ecs.Entity]joinedEdgeSide{}
	for entity, access := range ecs.Query[joinedEdge](world).All {
		edge := access.Mut()

		if side, ok := want[edge.Owner]; ok {
			surface := uiSurfaceSnapshot(world, entity)
			got[edge.Owner] = edge.Side
			if surface == nil || edge.Side != side || surface.Shape.Radius != 0 || surface.Width != 4 {
				t.Fatalf("joined edge = %+v surface=%+v", edge, surface)
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("joined edges = %v, want %v", got, want)
	}
}

func TestButtonTextKeepsGapAfterIcon(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 120, Height: 40, Label: "Run", Icon: TextIcon("WWW")})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var iconRight float32
	var labelX float32
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner != button {
			continue
		}
		switch {
		case ecs.Has[Part[IconPart]](world, entity):
			width, _, ok := scene.MeasureTextValue2D(text, text.Value)
			if !ok {
				t.Fatal("expected button icon to be measurable")
			}
			iconRight = transform.Translation.X.Value + width
		case ecs.Has[Part[LabelPart]](world, entity):
			labelX = transform.Translation.X.Value
		}
	}
	if math.Abs(float64(labelX-iconRight-textIconGap)) > 0.001 {
		t.Fatalf("expected button icon gap %v, got %v", textIconGap, labelX-iconRight)
	}
}

func TestButtonTextUsesExplicitIconWidth(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 120, Height: 40, Label: "Run", Icon: IconStyle{Fallback: " ", Width: 16}})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var labelX float32
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner == button && ecs.Has[Part[LabelPart]](world, entity) {
			labelX = transform.Translation.X.Value
		}
	}
	want := textIconX + 16 + textIconGap
	if labelX != want {
		t.Fatalf("expected label x=%v from explicit icon width, got %v", want, labelX)
	}
}

func TestCenteredButtonContentCentersTextureIconAndLabel(t *testing.T) {
	world := newUIWorld(t)
	icon := graphics.CreateTextureByColor(graphics.White)
	button := SpawnButton(world, ButtonOptions{
		Width: 160, Height: 40, Label: "Connect", Icon: TextureIcon(icon, 18, 18), ContentAlign: ButtonContentCenter,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	iconEntity := findSurfacePart[IconPart](world, button)
	iconTransform, transformOK := ecs.Get[scene.UITransform](world, iconEntity)
	iconSize, sizeOK := ecs.Get[scene.Size2D](world, iconEntity)
	var labelTransform *scene.UITransform
	var labelText *scene.Text2D
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		relation := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		if relation.Owner == button && ecs.Has[Part[LabelPart]](world, entity) {
			labelTransform, labelText = transform, text
			break
		}
	}
	if !transformOK || !sizeOK || labelTransform == nil || labelText == nil {
		t.Fatal("expected button icon and label visuals")
	}
	labelWidth, _, measured := scene.MeasureTextValue2D(labelText, labelText.Value)
	if !measured {
		t.Fatal("expected button label to be measurable")
	}
	contentCenter := (iconTransform.Translation.X.Value + labelTransform.Translation.X.Value + labelWidth) * 0.5
	if math.Abs(float64(contentCenter-80)) > 0.001 {
		t.Fatalf("content center = %v, want 80", contentCenter)
	}
	if labelTransform.Translation.X.Value-iconTransform.Translation.X.Value-iconSize.Width != textIconGap {
		t.Fatalf("icon-label gap = %v, want %v", labelTransform.Translation.X.Value-iconTransform.Translation.X.Value-iconSize.Width, textIconGap)
	}
}

func TestLabelSyncsTextValue(t *testing.T) {
	world := newUIWorld(t)
	label := SpawnLabel(world, LabelOptions{X: 12, Y: 16, Value: "Ready"})

	component := ecs.GetMut[Label](world, label)
	component.Value = "Running"
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	text := ecs.GetMut[scene.Text2D](world, label)
	if text.Value != "Running" {
		t.Fatalf("expected label text to sync, got %q", text.Value)
	}
}

func TestInputFocusEditAndSubmit(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{X: 0, Y: 0, Width: 160, Height: 36, Placeholder: "Name"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	state.CommitText("a")
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("type: %v", err)
	}
	state.EndFrame()
	state.PressKey(input.KeyEnter)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("submit: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	if component.Value != "a" || !component.Submitted {
		t.Fatalf("unexpected input state: %#v", component)
	}
}

func TestDoubleClickingInputSelectsAllText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "select all"})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)

	click := func() {
		t.Helper()
		state.PressMouse(input.MouseLeft, 24, 12)
		if err := runUIUpdate(world); err != nil {
			t.Fatalf("press input: %v", err)
		}
		state.EndFrame()
		state.ReleaseMouse(input.MouseLeft, 24, 12)
		if err := runUIUpdate(world); err != nil {
			t.Fatalf("release input: %v", err)
		}
		state.EndFrame()
	}

	click()
	state.PressMouse(input.MouseLeft, 24, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("second press: %v", err)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Anchor != 0 || component.Caret != len([]rune(component.Value)) {
		t.Fatalf("double-click selection = anchor %d caret %d, want all %q", component.Anchor, component.Caret, component.Value)
	}
	config := state.TextInputConfig()
	if config.SelectionStart != 0 || config.SelectionEnd != len([]rune(component.Value)) {
		t.Fatalf("platform selection = %d..%d, want all text", config.SelectionStart, config.SelectionEnd)
	}
}

func TestSelectAllKeepsTextareaScrollPosition(t *testing.T) {
	world := newUIWorld(t)
	value := strings.Repeat("line\n", 100)
	control := SpawnTextarea(world, TextOptions{Width: 220, Height: 80, Value: value})
	eventState := &uiEventState{}
	ecs.InsertResource(world, eventState)
	focus(world, eventState, control)
	field := ecs.GetMut[TextField](world, control)
	field.Caret, field.Anchor = 20, 20
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	textarea := ecs.GetMut[Textarea](world, control)
	textarea.ScrollY = 120
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	before := ecs.GetRef[Textarea](world, control).ScrollY
	if before <= 0 {
		t.Fatal("textarea did not retain the requested starting scroll position")
	}
	state, _ := ecs.GetResource[*input.State](world)
	state.PressKey(input.KeyLeftControl)
	state.PressKey(input.KeyA)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	field = ecs.GetMut[TextField](world, control)
	textarea = ecs.GetMut[Textarea](world, control)
	if field.Anchor != 0 || field.Caret != len([]rune(value)) || textarea.ScrollY != before {
		t.Fatalf("select all changed selection or view: selection=%d:%d scroll=%g, want 0:%d at %g", field.Anchor, field.Caret, textarea.ScrollY, len([]rune(value)), before)
	}
}

func TestDoubleClickingNumberInputSelectsCopiesAndDeletesText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{Width: 180, Height: 36, Value: "1234"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	copied := ""
	state.SetClipboard(func(value string) bool { copied = value; return true }, func() (string, bool) { return "", false })
	click := func() {
		t.Helper()
		state.PressMouse(input.MouseLeft, 24, 12)
		if err := runUIUpdate(world); err != nil {
			t.Fatalf("press number input: %v", err)
		}
		state.EndFrame()
		state.ReleaseMouse(input.MouseLeft, 24, 12)
		if err := runUIUpdate(world); err != nil {
			t.Fatalf("release number input: %v", err)
		}
		state.EndFrame()
	}
	click()
	state.PressMouse(input.MouseLeft, 24, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("double-click number input: %v", err)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Anchor != 0 || component.Caret != 4 {
		t.Fatalf("number input selection = %d..%d, want 0..4", component.Anchor, component.Caret)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync number input selection: %v", err)
	}
	selected := false
	for entity, access := range ecs.Query2[Relation[SurfaceStyle], Indexed[SelectionPart]](world).All {
		visual := access.A.Mut()
		access.B.Mut()

		if visual.Owner == field {
			if surface := uiSurfaceSnapshot(world, entity); surface != nil && surface.Width > 0 {
				selected = true
			}
		}
	}
	if !selected {
		t.Fatal("number input selection was not rendered")
	}
	state.EndFrame()
	state.PressKey(input.KeyLeftControl)
	state.PressKey(input.KeyC)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("copy number input: %v", err)
	}
	if copied != "1234" {
		t.Fatalf("copied number input text = %q, want %q", copied, "1234")
	}
	state.EndFrame()
	state.ReleaseKey(input.KeyC)
	state.ReleaseKey(input.KeyLeftControl)
	state.PressKey(input.KeyBackspace)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("delete number input selection: %v", err)
	}
	component, _ = ecs.Get[TextField](world, field)
	if component.Value != "" {
		t.Fatalf("number input value after deleting selection = %q, want empty", component.Value)
	}
}

func TestDraggingNumberInputSelectsText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{Width: 180, Height: 36, Value: "1234"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 12, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press number input: %v", err)
	}
	state.EndFrame()
	state.MoveMouse(100, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("drag number input selection: %v", err)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Anchor != 0 || component.Caret != 4 || !component.Selecting {
		t.Fatalf("dragged number input selection = anchor %d caret %d selecting %v, want 0..4 while selecting", component.Anchor, component.Caret, component.Selecting)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync number input selection: %v", err)
	}
	selected := false
	for entity, access := range ecs.Query2[Relation[SurfaceStyle], Indexed[SelectionPart]](world).All {
		visual := access.A.Mut()
		access.B.Mut()

		if visual.Owner == field {
			if surface := uiSurfaceSnapshot(world, entity); surface != nil && surface.Width > 0 {
				selected = true
			}
		}
	}
	if !selected {
		t.Fatal("dragged number input selection was not rendered")
	}
}

func TestDraggingNumberInputSelectsTextRightToLeft(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{Width: 180, Height: 36, Value: "1234"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 100, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press number input: %v", err)
	}
	state.EndFrame()
	state.MoveMouse(12, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("drag number input selection right to left: %v", err)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Anchor != 4 || component.Caret != 0 || !component.Selecting {
		t.Fatalf("right-to-left number input selection = anchor %d caret %d selecting %v, want 4..0 while selecting", component.Anchor, component.Caret, component.Selecting)
	}
}

func TestDraggingInputIgnoresStalePlatformSelection(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "1234"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 100, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press input: %v", err)
	}
	state.EndFrame()
	// Native text controls can report their previous collapsed selection after
	// the canvas press; the active mouse drag must remain authoritative.
	state.ApplyTextSelection(input.TextSelection{Start: 4, End: 4})
	state.MoveMouse(12, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("drag input with stale platform selection: %v", err)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Anchor != 4 || component.Caret != 0 || !component.Selecting {
		t.Fatalf("dragged input selection = %d..%d selecting=%v, want 4..0 while selecting", component.Anchor, component.Caret, component.Selecting)
	}
	state.EndFrame()
	state.ApplyTextSelection(input.TextSelection{Start: 4, End: 4})
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("hold input with stale platform selection: %v", err)
	}
	component, _ = ecs.Get[TextField](world, field)
	if component.Anchor != 4 || component.Caret != 0 || !component.Selecting {
		t.Fatalf("held input selection = %d..%d selecting=%v, want 4..0 while selecting", component.Anchor, component.Caret, component.Selecting)
	}
	config := state.TextInputConfig()
	if config.SelectionStart != 0 || config.SelectionEnd != 4 {
		t.Fatalf("platform selection = %d..%d, want ordered range 0..4", config.SelectionStart, config.SelectionEnd)
	}
	state.EndFrame()
	state.ApplyTextSelection(input.TextSelection{Start: 4, End: 4})
	state.ReleaseMouse(input.MouseLeft, 12, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release input with stale platform selection: %v", err)
	}
	component, _ = ecs.Get[TextField](world, field)
	if component.Anchor != 4 || component.Caret != 0 || component.Selecting {
		t.Fatalf("released input selection = %d..%d selecting=%v, want retained 4..0", component.Anchor, component.Caret, component.Selecting)
	}
}

func TestClickingAnotherControlBreaksInputDoubleClick(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "select all"})
	SpawnButton(world, ButtonOptions{X: 200, Width: 80, Height: 36, Label: "Other"})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)

	click := func(x float64) {
		t.Helper()
		state.PressMouse(input.MouseLeft, x, 12)
		if err := runUIUpdate(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
		state.ReleaseMouse(input.MouseLeft, x, 12)
		if err := runUIUpdate(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
	}

	click(24)
	click(220)
	state.PressMouse(input.MouseLeft, 24, 12)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Anchor == 0 && component.Caret == len([]rune(component.Value)) {
		t.Fatal("input click after another control was treated as a double-click")
	}
}

func TestNumberInputSubmitClearsFocus(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{X: 0, Y: 0, Width: 160, Height: 36, Value: "12", Step: 1})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	state.PressKey(input.KeyEnter)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("submit: %v", err)
	}

	component, _ := ecs.Get[TextField](world, field)
	input, _ := ecs.GetResource[*uiEventState](world)
	if component.Focused || input.Focused != 0 || state.TextInputConfig().Active {
		t.Fatalf("number input remained focused after submit: field=%#v input=%#v", component, input)
	}
}

func TestInputArrowKeysMoveCaretAndInsertText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{X: 0, Y: 0, Width: 160, Height: 36, Value: "a界c"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 150, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 150, 8)
	state.PressKey(input.KeyLeft)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("move left: %v", err)
	}
	state.EndFrame()
	state.ReleaseKey(input.KeyLeft)
	state.CommitText("你")
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("insert: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	if component.Value != "a界你c" || component.Caret != 3 {
		t.Fatalf("unexpected input caret state: %#v", component)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := valueText(world, field); got != "a界你c" {
		t.Fatalf("caret changed input text: %q", got)
	}
}

func TestAllTextInputsPlaceCaretFromMouseOrTouch(t *testing.T) {
	tests := []struct {
		name  string
		value string
		spawn func(*ecs.World) ecs.Entity
		get   func(*ecs.World, ecs.Entity) (string, int)
	}{
		{name: "input", value: "abcd", spawn: func(world *ecs.World) ecs.Entity {
			return SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "abcd"})
		}, get: textFieldValueAndCaret},
		{name: "number input", value: "1234", spawn: func(world *ecs.World) ecs.Entity {
			return SpawnNumberInput(world, NumberInputOptions{Width: 180, Height: 36, Value: "1234"})
		}, get: textFieldValueAndCaret},
		{name: "textarea", value: "abcd", spawn: func(world *ecs.World) ecs.Entity {
			return SpawnTextarea(world, TextOptions{Width: 180, Height: 80, Value: "abcd"})
		}, get: textFieldValueAndCaret},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := newUIWorld(t)
			field := test.spawn(world)
			syncTransforms(t, world)
			state, _ := ecs.GetResource[*input.State](world)
			if i%2 == 0 {
				state.TouchStart(12, 8)
			} else {
				state.PressMouse(input.MouseLeft, 12, 8)
			}
			if err := runUIUpdate(world); err != nil {
				t.Fatalf("place caret: %v", err)
			}
			value, caret := test.get(world, field)
			if value != test.value || caret != 0 {
				t.Fatalf("unexpected pointer caret: value=%q caret=%d", value, caret)
			}
			state.EndFrame()
			if i%2 == 0 {
				state.TouchEnd(12, 8)
			} else {
				state.ReleaseMouse(input.MouseLeft, 12, 8)
			}
			state.PressKey(input.KeyRight)
			if err := runUIUpdate(world); err != nil {
				t.Fatalf("move caret: %v", err)
			}
			_, caret = test.get(world, field)
			if caret != 1 {
				t.Fatalf("right arrow did not move caret: %d", caret)
			}
		})
	}
}

func TestTextInputSelectionCopiesAndPastesOverRange(t *testing.T) {
	state := input.NewState()
	copied := ""
	state.SetClipboard(func(value string) bool { copied = value; return true }, func() (string, bool) { return "X", true })
	state.PressKey(input.KeyLeftControl)
	state.PressKey(input.KeyC)
	value, caret, anchor, _ := editTextValue("hello", 4, 1, 0, false, nil, state, false, nil)
	if copied != "ell" || value != "hello" || caret != 4 || anchor != 1 {
		t.Fatalf("unexpected copy result: copied=%q value=%q selection=%d..%d", copied, value, anchor, caret)
	}
	state.EndFrame()
	state.PressKey(input.KeyV)
	value, caret, anchor, _ = editTextValue(value, caret, anchor, 0, false, nil, state, false, nil)
	if value != "hXo" || caret != 2 || anchor != 2 {
		t.Fatalf("unexpected paste result: value=%q selection=%d..%d", value, anchor, caret)
	}
}

func TestTextareaInputChoosesLine(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 180, Height: 80, Value: "first\nsecond"})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.TouchStart(12, 38)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("place caret: %v", err)
	}
	component := ecs.GetMut[TextField](world, field)
	if component.Caret != 6 {
		t.Fatalf("expected second-line caret, got %d", component.Caret)
	}
}

func TestFocusedInputCaretBlinks(t *testing.T) {
	world := newUIWorld(t)
	clock := scene.NewTime()
	ecs.InsertResource(world, clock)
	request := &scene.RenderRequest{}
	ecs.InsertResource(world, request)
	field := SpawnInput(world, TextOptions{Width: 160, Height: 36, Value: "stable"})
	component := ecs.GetMut[TextField](world, field)
	component.Focused = true

	for _, test := range []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 0, want: "stable"},
		{elapsed: 499 * time.Millisecond, want: "stable"},
		{elapsed: 500 * time.Millisecond, want: "stable"},
		{elapsed: 999 * time.Millisecond, want: "stable"},
		{elapsed: time.Second, want: "stable"},
	} {
		clock.Set(0, test.elapsed, 0)
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatalf("sync at %s: %v", test.elapsed, err)
		}
		if got := valueText(world, field); got != test.want {
			t.Fatalf("display at %s: got %q, want %q", test.elapsed, got, test.want)
		}
		caret := caretSurface(world, field)
		wantCaret := test.elapsed < 500*time.Millisecond || test.elapsed >= time.Second
		if visible := caretVisibleState(world, field); visible != wantCaret {
			t.Fatalf("caret visibility at %s: got %v, want %v", test.elapsed, visible, wantCaret)
		}
		if caret.Width != 1 || caret.Height <= 1 {
			t.Fatalf("unexpected caret size at %s: %.0fx%.0f", test.elapsed, caret.Width, caret.Height)
		}
		if (test.elapsed == 500*time.Millisecond || test.elapsed == time.Second) && !request.Take() {
			t.Fatal("caret phase change did not request a render")
		}
	}

	component.Focused = false
	clock.Set(0, 1500*time.Millisecond, 0)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync unfocused input: %v", err)
	}
	if request.Take() {
		t.Fatal("unfocused caret phase change requested a render")
	}
	if caretVisibleState(world, field) {
		t.Fatal("unfocused caret remained visible")
	}
}

func TestInputAcceptsIMECommitAndShowsComposition(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{X: 0, Y: 0, Width: 160, Height: 36, Placeholder: "Name"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	if config := state.TextInputConfig(); !config.Active || config.Kind != input.TextInputText || config.Multiline {
		t.Fatalf("unexpected text input config: %#v", config)
	}

	state.ApplyTextComposition(input.TextComposition{Text: "ni", Active: true})
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("compose: %v", err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync compose: %v", err)
	}
	if got := valueText(world, field); got != "ni" {
		t.Fatalf("unexpected composition display: %q", got)
	}

	state.CommitText("你")
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("commit: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	if component.Value != "你" || component.Composition != "" {
		t.Fatalf("unexpected input state: %#v", component)
	}
}

func TestIMECommitReplacesSelectedText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 160, Height: 36, Value: "abc"})
	uiState := &uiEventState{}
	ecs.InsertResource(world, uiState)
	focus(world, uiState, field)

	component := ecs.GetMut[TextField](world, field)
	component.Anchor, component.Caret = 0, 3
	state, _ := ecs.GetResource[*input.State](world)
	state.SetTextInputSelection(0, 3)
	state.EndFrame()

	state.ApplyTextComposition(input.TextComposition{Active: true})
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("start composition: %v", err)
	}
	state.ApplyTextComposition(input.TextComposition{Text: "n", Active: true})
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("update composition: %v", err)
	}
	selected, _ := ecs.Get[TextField](world, field)
	if selected.Anchor != 0 || selected.Caret != 3 {
		t.Fatalf("composition collapsed selection to %d..%d", selected.Anchor, selected.Caret)
	}
	state.CommitText("你")
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("commit composition: %v", err)
	}

	result, _ := ecs.Get[TextField](world, field)
	if result.Value != "你" {
		t.Fatalf("IME commit value = %q, want selected text replaced with %q", result.Value, "你")
	}
}

func TestTextInputConfigCaretTracksRenderedUICaret(t *testing.T) {
	world := newUIWorld(t)
	parent := spawnAnchor(world, 20, 30)
	parentTransform := ecs.GetMut[scene.UITransform](world, parent)
	parentTransform.Scale = mgl32.Vec2{1.5, 1.25}
	field := SpawnTextarea(world, TextOptions{X: 8, Y: 12, Width: 180, Height: 80, Value: "ab\ncd"})
	if err := ecs.Insert(world, field, scene.Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	uiState := &uiEventState{}
	ecs.InsertResource(world, uiState)
	focus(world, uiState, field)

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync UI: %v", err)
	}
	syncTransforms(t, world)
	if err := SyncTextInputSystem("").Run(world); err != nil {
		t.Fatalf("sync text input: %v", err)
	}

	want := input.TextInputRect{}
	found := false
	for entity, access := range ecs.Query3[Relation[SurfaceStyle], scene.UIGlobalTransform, scene.Size2D](world).All {
		visual := access.A.Ref()
		transform := access.B.Ref()
		size := access.C.Ref()

		if visual.Owner != field || !ecs.Has[Part[CaretPart]](world, entity) {
			continue
		}
		minX, minY, maxX, maxY, ok := scene.BoundsInSpace2D(transform.Matrix, scene.AnchorTopLeft, size.Width, size.Height, true)
		if ok {
			want = input.TextInputRect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
			found = true
		}
		break
	}
	if !found {
		t.Fatal("rendered caret was not found")
	}
	uiInputState, _ := ecs.GetResource[*input.State](world)
	config := uiInputState.TextInputConfig()
	if config.Caret != want {
		t.Fatalf("IME caret = %+v, want rendered caret %+v", config.Caret, want)
	}
	if config.Caret.X <= 20 || config.Caret.Y <= 30 || config.Caret.Width <= 1 || config.Caret.Height <= 0 {
		t.Fatalf("IME caret did not include the nested UI transform: %+v", config.Caret)
	}
}

func TestCompositionSuppressesEditingAndClearsWhenFocusMoves(t *testing.T) {
	world := newUIWorld(t)
	first := SpawnInput(world, TextOptions{X: 0, Y: 0, Width: 160, Height: 36, Value: "a"})
	second := SpawnInput(world, TextOptions{X: 0, Y: 48, Width: 160, Height: 36})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus first: %v", err)
	}
	state.EndFrame()
	state.ApplyTextComposition(input.TextComposition{Active: true})
	state.PressKey(input.KeyBackspace)
	state.PushTextCommand(input.TextCommandEnter)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("compose: %v", err)
	}
	component := ecs.GetMut[TextField](world, first)
	if component.Value != "a" || component.Submitted {
		t.Fatalf("composition edited committed value: %#v", component)
	}

	input, _ := ecs.GetResource[*uiEventState](world)
	previousRevision := state.TextInputConfig().Revision
	focus(world, input, second)
	if state.Composing() || state.CompositionText() != "" {
		t.Fatal("expected focus change to clear composition")
	}
	if state.TextInputConfig().Revision <= previousRevision {
		t.Fatal("expected focus change to start a new text input session")
	}
}

func TestTextareaEnterSubmitsWithoutAddingNewline(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{X: 0, Y: 0, Width: 160, Height: 80})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	state.CommitText("\r\nx\b")
	state.PressKey(input.KeyEnter)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("edit: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	if component.Value != "x" || !component.Submitted {
		t.Fatalf("unexpected textarea value: %q", component.Value)
	}
	events := (&ecs.MessageReader[SubmittedEvent[string]]{}).Read(world)
	if len(events) != 1 || events[0].Entity != field || events[0].Value != "x" {
		t.Fatalf("unexpected textarea submit events: %#v", events)
	}
}

func TestTextareaShiftEnterAddsNewlineWithoutSubmitting(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{X: 0, Y: 0, Width: 160, Height: 80, Value: "first"})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	state.PressKey(input.KeyLeftShift)
	state.PressKey(input.KeyEnter)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("edit: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	if component.Value != "\nfirst" || component.Submitted {
		t.Fatalf("unexpected textarea state: %#v", component)
	}
	if events := (&ecs.MessageReader[SubmittedEvent[string]]{}).Read(world); len(events) != 0 {
		t.Fatalf("unexpected textarea submit events: %#v", events)
	}
}

func TestTextareaDisplaysMultipleLinesWithinItsClip(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{X: 4, Y: 6, Width: 160, Height: 80, Value: "first\nsecond"})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	clip := ecs.GetMut[scene.ClipRect2D](world, field)
	ok := clip != nil
	if !ok || clip == nil || clip.X != 0 || clip.Y != 0 || clip.Width != 160 || clip.Height != 80 {
		t.Fatalf("unexpected textarea clip: %#v", clip)
	}

	var value *scene.Text2D
	for entity, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()

		if visual.Owner == field && ecs.Has[Part[ValuePart]](world, entity) {
			value = text
			break
		}
	}
	if value == nil {
		t.Fatal("expected textarea value text")
	}
	_, height, ok := scene.MeasureText2D(value)
	if !ok || height <= 20 {
		t.Fatalf("expected multiline text height, got %.2f", height)
	}
}

func TestTextareaClipTracksResizedControl(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 160, Height: 80, Value: "first\nsecond"})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("initial textarea sync: %v", err)
	}
	target := ecs.GetMut[scene.InputTarget2D](world, field)
	ok := target != nil
	if !ok || target == nil {
		t.Fatal("textarea has no input target")
	}
	target.Width, target.Height = 320, 120
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("resized textarea sync: %v", err)
	}
	clip, ok := ecs.Get[scene.ClipRect2D](world, field)
	if !ok || clip.X != 0 || clip.Y != 0 || clip.Width != 320 || clip.Height != 120 {
		t.Fatalf("resized textarea clip: %#v", clip)
	}
}

func TestTextareaLongLineScrollsToCaret(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 100, Height: 40, Value: "a very long line that exceeds the textarea viewport"})
	component := ecs.GetMut[TextField](world, field)
	component.Focused = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	textarea, _ := ecs.Get[Textarea](world, field)
	if textarea.Wrap || textarea.ScrollX <= 0 {
		t.Fatalf("textarea state = %#v, want default horizontal scrolling", textarea)
	}
	var textTransform *scene.UITransform
	var text *scene.Text2D
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		value := access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner == field && ecs.Has[Part[ValuePart]](world, entity) {
			text, textTransform = value, transform
			break
		}
	}
	if text == nil || textTransform == nil || text.WrapMode != scene.TextWrapNone || textTransform.Translation.X.Value >= textareaTextPaddingX {
		t.Fatalf("textarea text layout = text=%#v transform=%#v", text, textTransform)
	}
	trackVisible := false
	for entity, access := range ecs.Query2[Relation[SurfaceStyle], scene.Size2D](world).All {
		visual := access.A.Mut()
		size := access.B.Mut()

		if visual.Owner == field && ecs.Has[Part[ScrollTrackHorizontalPart]](world, entity) {
			trackVisible = size.Width > 0 && size.Height > 0
			break
		}
	}
	if !trackVisible {
		t.Fatal("expected the textarea horizontal scrollbar track")
	}
}

func TestInputLongLineScrollsToCaretAndClipsText(t *testing.T) {
	world := newUIWorld(t)
	fieldEntity := SpawnInput(world, TextOptions{Width: 100, Height: 36, Value: "resources/generated/background-1786032159368810.png"})
	field := ecs.GetMut[TextField](world, fieldEntity)
	field.Focused = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	clip, ok := ecs.Get[scene.ClipRect2D](world, fieldEntity)
	if !ok || clip.Width != 100 || clip.Height != 36 {
		t.Fatalf("unexpected input clip: %#v", clip)
	}
	field = ecs.GetMut[TextField](world, fieldEntity)
	if field.ScrollX >= 0 {
		t.Fatalf("input scroll = %.1f, want negative offset at end caret", field.ScrollX)
	}
	endScroll := field.ScrollX
	var textTransform *scene.UITransform
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner == fieldEntity && ecs.Has[Part[ValuePart]](world, entity) {
			textTransform = transform
			break
		}
	}
	if textTransform == nil || textTransform.Translation.X.Value >= textareaTextPaddingX {
		t.Fatalf("input text transform = %#v, want horizontal offset", textTransform)
	}

	field.Caret = 0
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync after moving caret: %v", err)
	}
	field = ecs.GetMut[TextField](world, fieldEntity)
	if field.ScrollX != 0 {
		t.Fatalf("input scroll after moving caret to start = %.1f, want 0", field.ScrollX)
	}
	if textTransform.Translation.X.Value != textareaTextPaddingX {
		t.Fatalf("input text x after moving caret to start = %.1f, want %.1f", textTransform.Translation.X.Value, textareaTextPaddingX)
	}
	if endScroll >= field.ScrollX || textTransform.Translation.X.Value <= textareaTextPaddingX+endScroll {
		t.Fatalf("increasing ScrollX did not move input text right: scroll %.1f -> %.1f, x=%0.1f", endScroll, field.ScrollX, textTransform.Translation.X.Value)
	}
}

func TestInputInsertionKeepsTextBeforeCaretAnchored(t *testing.T) {
	world := newUIWorld(t)
	fieldEntity := SpawnInput(world, TextOptions{Width: 90, Height: 36, Value: "12345678901234567890"})
	field := ecs.GetMut[TextField](world, fieldEntity)
	field.Focused, field.Caret, field.Anchor = true, 10, 10
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync before insertion: %v", err)
	}

	var text *scene.Text2D
	var transform *scene.UITransform
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		value := access.B.Mut()
		position := access.C.Mut()

		if visual.Owner == fieldEntity && ecs.Has[Part[ValuePart]](world, entity) {
			text, transform = value, position
			break
		}
	}
	if text == nil || transform == nil {
		t.Fatal("input text visual is missing")
	}
	leftBefore, _, _, _, _ := scene.TextCaretRect2D(text, text.Value, 9)
	rightBefore, _, _, _, _ := scene.TextCaretRect2D(text, text.Value, 11)
	leftBefore += transform.Translation.X.Value
	rightBefore += transform.Translation.X.Value

	field.Value, field.Caret, field.Anchor = "1234567890X1234567890", 11, 11
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync after insertion: %v", err)
	}
	leftAfter, _, _, _, _ := scene.TextCaretRect2D(text, field.Value, 9)
	rightAfter, _, _, _, _ := scene.TextCaretRect2D(text, field.Value, 12)
	leftAfter += transform.Translation.X.Value
	rightAfter += transform.Translation.X.Value
	if diff := absFloat32(leftAfter - leftBefore); diff > 0.001 {
		t.Fatalf("text before insertion caret moved: before=%v after=%v", leftBefore, leftAfter)
	}
	if rightAfter <= rightBefore {
		t.Fatalf("text after insertion caret did not move right: before=%v after=%v", rightBefore, rightAfter)
	}
}

func TestTextareaScrollbarDragDoesNotSelectText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 100, Height: 40, Value: "a very long line that exceeds the textarea viewport"})
	component := ecs.GetMut[TextField](world, field)
	component.Focused = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	syncTransforms(t, world)

	textarea, _ := ecs.Get[Textarea](world, field)
	before := textarea.ScrollX
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 80, 35)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("scrollbar press: %v", err)
	}
	current, _ := ecs.Get[TextField](world, field)
	if current.Selecting {
		t.Fatal("pressing the textarea scrollbar started text selection")
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync pressed scrollbar: %v", err)
	}
	body := findSurfacePart[BodyPart](world, field)
	surface, _ := ecs.Get[scene.UIBackground2D](world, body)
	theme, _ := ecs.GetResource[Theme](world)
	if surface.Shape.Fill == theme.SurfaceActive {
		t.Fatal("textarea body used the pressed color while dragging its scrollbar")
	}
	state.EndFrame()
	state.MoveMouse(20, 35)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("scrollbar drag: %v", err)
	}
	textarea, _ = ecs.Get[Textarea](world, field)
	current, _ = ecs.Get[TextField](world, field)
	if textarea.ScrollX >= before || current.Selecting {
		t.Fatalf("textarea scrollbar drag = scroll %.1f -> %.1f selecting=%v", before, textarea.ScrollX, current.Selecting)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 20, 35)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("scrollbar release: %v", err)
	}
}

func TestTextareaVerticalScrollbarDragDoesNotSelectText(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 100, Height: 80, Value: strings.Repeat("line\n", 20)})
	component := ecs.GetMut[TextField](world, field)
	component.Focused = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	syncTransforms(t, world)

	textarea, _ := ecs.Get[Textarea](world, field)
	before := textarea.ScrollY
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 95, 60)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("vertical scrollbar press: %v", err)
	}
	current, _ := ecs.Get[TextField](world, field)
	if current.Selecting {
		t.Fatal("pressing the vertical textarea scrollbar started text selection")
	}
	state.EndFrame()
	state.MoveMouse(95, 20)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("vertical scrollbar drag: %v", err)
	}
	textarea, _ = ecs.Get[Textarea](world, field)
	current, _ = ecs.Get[TextField](world, field)
	if textarea.ScrollY >= before || current.Selecting {
		t.Fatalf("vertical textarea scrollbar drag = scroll %.1f -> %.1f selecting=%v", before, textarea.ScrollY, current.Selecting)
	}
}

func TestTextareaCanWrapLongLinesByCharacters(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 100, Height: 80, Wrap: true, Value: "resources/generated/background-very-long-name.png"})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var text *scene.Text2D
	for entity, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
		visual := access.A.Mut()
		value := access.B.Mut()

		if visual.Owner == field && ecs.Has[Part[ValuePart]](world, entity) {
			text = value
			break
		}
	}
	if text == nil || text.WrapMode != scene.TextWrapCharacters {
		t.Fatalf("textarea wrap mode = %#v", text)
	}
	width, height, ok := scene.MeasureText2D(text)
	lines, linesOK := scene.TextLineRects2D(text)
	if !ok || !linesOK || len(lines) < 2 || width > 77.5 || height <= 20 {
		t.Fatalf("wrapped textarea layout = width %.2f height %.2f lines %d ok %v/%v", width, height, len(lines), ok, linesOK)
	}
}

func TestTextareaSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	SpawnTextarea(world, TextOptions{Width: 180, Height: 80, Wrap: true, Value: "stable textarea value"})
	schedule := ecs.NewSchedule("textarea_sync_allocations")
	if err := schedule.Add(SyncSystem("")); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := schedule.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("textarea sync allocated %.0f objects, want 0", allocations)
	}
}

func TestPasswordInputMasksDisplayedValue(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "s3cret", Password: true})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := valueText(world, field); got != "******" {
		t.Fatalf("password display = %q, want masked value", got)
	}
	component, _ := ecs.Get[TextField](world, field)
	if component.Value != "s3cret" {
		t.Fatalf("password value changed to %q", component.Value)
	}
}

func TestTextInputSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	for _, password := range []bool{false, true} {
		name := "plain"
		if password {
			name = "password"
		}
		t.Run(name, func(t *testing.T) {
			world := newUIWorld(t)
			SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "s3cret", Password: password})
			schedule := ecs.NewSchedule("text_input_sync_allocations")
			if err := schedule.Add(SyncSystem("")); err != nil {
				t.Fatal(err)
			}
			if err := schedule.Run(world); err != nil {
				t.Fatal(err)
			}

			if allocations := testing.AllocsPerRun(100, func() {
				if err := schedule.Run(world); err != nil {
					t.Fatal(err)
				}
			}); allocations != 0 {
				t.Fatalf("text input sync allocated %.0f objects, want 0", allocations)
			}
		})
	}
}

func TestComposedTextInputSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{Width: 180, Height: 36, Value: "text"})
	textField := ecs.GetMut[TextField](world, field)
	if textField == nil {
		t.Fatal("expected text field")
	}
	textField.Composition = "中"
	textField.Caret = 2
	textField.Focused = true

	schedule := ecs.NewSchedule("composed_text_input_sync_allocations")
	if err := schedule.Add(SyncSystem("")); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := schedule.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("composed text input sync allocated %.0f objects, want 0", allocations)
	}
}

func TestFocusedTextInputUpdateDoesNotAllocateWithoutEdits(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnInput(world, TextOptions{X: 20, Y: 20, Width: 180, Height: 36, Value: "text"})
	input := &uiEventState{}
	ecs.InsertResource(world, input)
	focus(world, input, field)
	schedule := ecs.NewSchedule("focused_text_input_update_allocations")
	if err := schedule.Add(UpdateSystem("")); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatal(err)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		if err := schedule.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("focused input update allocated %.0f objects, want 0", allocations)
	}
}

func TestEditTextValueAppliesRuneLimitInOnePass(t *testing.T) {
	got, _, _ := input.ApplyTextEditsInRange("", 0, 0, 3, false, []input.TextEdit{{Text: "你好世界"}})
	if got != "你好世" {
		t.Fatalf("unexpected limited text: %q", got)
	}
}

func TestNumberInputStepButtonChangesValue(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{X: 0, Y: 0, Width: 160, Height: 36, Value: "1.5", Step: 0.25})
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 150, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 150, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	number := ecs.GetMut[NumberInput](world, field)
	if component.Value != "1.75" || number == nil || !number.Changed {
		t.Fatalf("unexpected number input state: field=%#v number=%#v", component, number)
	}
	events := (&ecs.MessageReader[ChangedEvent[string]]{}).Read(world)
	if len(events) != 1 || events[0].Value != "1.75" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestNumberInputEditingEmitsInputEvent(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{X: 0, Y: 0, Width: 160, Height: 36, Step: 1})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	reader := &ecs.MessageReader[InputEvent[string]]{}

	state.PressMouse(input.MouseLeft, 8, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	reader.Read(world)
	state.EndFrame()
	state.CommitText("7")
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("edit: %v", err)
	}

	events := reader.Read(world)
	if len(events) != 1 || events[0].Entity != field || events[0].Value != "7" {
		t.Fatalf("unexpected input events: %#v", events)
	}
}

func TestNumberInputStepUsesStableDecimalPrecision(t *testing.T) {
	tests := []struct {
		value     string
		step      float64
		direction int
		want      string
	}{
		{value: "-28", step: 0.1, direction: 1, want: "-27.9"},
		{value: "-27.9", step: 0.1, direction: 1, want: "-27.8"},
		{value: "0.3", step: 0.1, direction: -1, want: "0.2"},
		{value: "1.23", step: 0.01, direction: 1, want: "1.24"},
	}
	for _, test := range tests {
		got, changed := StepNumberInputValue(test.value, NumberInput{Step: test.step}, test.direction)
		if !changed || got != test.want {
			t.Errorf("step %q by %v * %d = %q, changed=%v; want %q", test.value, test.step, test.direction, got, changed, test.want)
		}
	}

	got, ok := NormalizeNumberInputValue("-27.999954", NumberInput{Step: 0.1})
	if !ok || got != "-28" {
		t.Fatalf("normalized float artifact = %q, ok=%v; want -28", got, ok)
	}
}

func TestNumberInputStepButtonsKeepTheirJoinedEdgeSquare(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{Width: 160, Height: 36, Radius: 6})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	want := map[int]joinedEdgeSide{1: joinedEdgeBottom, -1: joinedEdgeTop}
	seen := map[int]bool{}
	for stepEntity, access := range ecs.Query2[NumberInputStep, scene.InputTarget2D](world).All {
		step := access.A.Mut()
		target := access.B.Mut()

		if step.Input != field {
			continue
		}
		for entity, access := range ecs.Query2[joinedEdge, scene.UITransform](world).All {
			edge := access.A.Mut()
			transform := access.B.Mut()

			if edge.Owner != stepEntity {
				continue
			}
			seen[step.Direction] = true
			surface := uiSurfaceSnapshot(world, entity)
			if surface == nil || edge.Side != want[step.Direction] || surface.Width != target.Width || surface.Height != 6 {
				t.Fatalf("unexpected joined edge for step %d: edge=%+v surface=%+v", step.Direction, edge, surface)
			}
			wantY := float32(0)
			if step.Direction > 0 {
				wantY = target.Height - 6
			}
			if transform.Translation.X.Value != 0 || transform.Translation.Y.Value != wantY {
				t.Fatalf("unexpected joined edge position for step %d: %+v", step.Direction, transform)
			}
		}
	}
	if !seen[1] || !seen[-1] {
		t.Fatalf("missing number input joined edges: %v", seen)
	}
}

func TestNumberInputBorderRendersAboveStepButtons(t *testing.T) {
	world := newUIWorld(t)
	borderColor := graphics.ColorFromRGBA(40, 50, 60, 255)
	style := DefaultSkin(DefaultTheme()).NumberInput
	style.Field.Body = NewStateStyle(ColorSurface(graphics.White).WithBorder(
		scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, borderColor, 0, 2),
	))
	field := SpawnNumberInput(world, NumberInputOptions{Width: 160, Height: 36, Radius: 6, Style: &style})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	borderEntity := ecs.Entity(0)
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		relation := access.Mut()

		if relation.Owner == field && ecs.Has[Part[NumberInputBorderPart]](world, entity) {
			borderEntity = entity
			break
		}
	}
	if borderEntity == 0 {
		t.Fatal("number input is missing its border overlay")
	}
	background, backgroundOK := ecs.Get[scene.UIBackground2D](world, borderEntity)
	border, borderOK := ecs.Get[scene.UIBorder2D](world, borderEntity)
	size, sizeOK := ecs.Get[scene.Size2D](world, borderEntity)
	borderStack, stackOK := ecs.Get[scene.UIStackIndex](world, borderEntity)
	if !backgroundOK || background.Shape.Kind != render.ShapeRoundedRect || background.Shape.Fill.A != 0 || !borderOK || border.Shape.Border != borderColor || border.Shape.BorderWidth != 2 || border.Shape.Radius != 6 {
		t.Fatalf("number input border overlay = background:%#v border:%#v", background, border)
	}
	if !sizeOK || size.Width != 160 || size.Height != 36 || !stackOK {
		t.Fatalf("number input border geometry = size:%#v stack:%v", size, borderStack)
	}

	steps := 0
	for entity, access := range ecs.Query[NumberInputStep](world).All {
		step := access.Mut()

		if step.Input != field {
			continue
		}
		steps++
		stepStack, ok := ecs.Get[scene.UIStackIndex](world, entity)
		if !ok || borderStack <= stepStack {
			t.Fatalf("border stack %d does not cover step stack %d", borderStack, stepStack)
		}
	}
	if steps != 2 {
		t.Fatalf("number input step count = %d, want 2", steps)
	}
}

func TestFocusedNumberInputWheelChangesValue(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnNumberInput(world, NumberInputOptions{X: 0, Y: 0, Width: 160, Height: 36, Value: "1.5", Step: 0.25})
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 12, 8)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("focus: %v", err)
	}
	state.EndFrame()
	state.ScrollMouse(0, 1)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("wheel: %v", err)
	}

	component := ecs.GetMut[TextField](world, field)
	number := ecs.GetMut[NumberInput](world, field)
	if component.Value != "1.75" || component.Caret != 4 || number == nil || !number.Changed {
		t.Fatalf("unexpected number input wheel state: field=%#v number=%#v", component, number)
	}
}

func TestNumberInputStepButtonsUseCustomSVGIcons(t *testing.T) {
	world := newUIWorld(t)
	upIcon, err := graphicssvg.Decode(
		[]byte(`<svg viewBox="0 0 12 6"><path d="M1 5 6 1l5 4" stroke="red"/></svg>`),
		svg.Options{Width: 12, Height: 6},
		graphics.TextureOptions{Mipmap: true, Usage: graphics.ResourceUsageGPU},
	)
	if err != nil {
		t.Fatalf("load up icon: %v", err)
	}
	downIcon, err := graphicssvg.Decode(
		[]byte(`<svg viewBox="0 0 6 12"><path d="m1 1 4 5-4 5" stroke="blue"/></svg>`),
		svg.Options{Width: 6, Height: 12},
		graphics.TextureOptions{Mipmap: true, Usage: graphics.ResourceUsageGPU},
	)
	if err != nil {
		t.Fatalf("load down icon: %v", err)
	}
	field := SpawnNumberInput(world, NumberInputOptions{
		X: 0, Y: 0, Width: 160, Height: 36,
		UpIcon: TextureIcon(upIcon, 12, 6), DownIcon: TextureIcon(downIcon, 6, 12),
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	assertNumberInputStepIcon(t, world, field, 1, upIcon, 12, 6, 3, 6)
	assertNumberInputStepIcon(t, world, field, -1, downIcon, 6, 12, 6, 3)
}

func assertNumberInputStepIcon(t *testing.T, world *ecs.World, inputEntity ecs.Entity, direction int, wantTexture *graphics.Texture, wantWidth, wantHeight, wantX, wantY float32) {
	t.Helper()
	var stepEntity ecs.Entity
	for entity, access := range ecs.Query[NumberInputStep](world).All {
		step := access.Mut()

		if step.Input == inputEntity && step.Direction == direction {
			stepEntity = entity
			break
		}
	}
	if stepEntity == 0 {
		t.Fatalf("missing number input step %d", direction)
	}

	var iconEntity ecs.Entity
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		visual := access.Mut()

		if visual.Owner == stepEntity && ecs.Has[Part[NumberStepIconPart]](world, entity) {
			iconEntity = entity
			break
		}
	}
	if iconEntity == 0 {
		t.Fatalf("missing number input step icon %d", direction)
	}
	surface := uiSurfaceSnapshot(world, iconEntity)
	if surface == nil || surface.Texture != wantTexture || surface.Width != wantWidth || surface.Height != wantHeight {
		t.Fatalf("unexpected step icon surface %d: %#v", direction, surface)
	}
	transform := ecs.GetMut[scene.UITransform](world, iconEntity)
	if transform.Translation.X.Value != wantX || transform.Translation.Y.Value != wantY {
		t.Fatalf("unexpected step icon position %d: %#v", direction, transform)
	}
	for entity, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()

		if visual.Owner == stepEntity && ecs.Has[Part[IconPart]](world, entity) && text.Value != "" {
			t.Fatalf("expected custom icon to replace step text, got %q", text.Value)
		}
	}
}

func TestSelectorSelectsItem(t *testing.T) {
	world := newUIWorld(t)
	selector := SpawnSelector(world, SelectorOptions{X: 0, Y: 0, Width: 140, Height: 32, Label: "Mode", Options: []string{"A", "B"}})
	syncTransforms(t, world)

	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 10, 70)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 10, 70)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release: %v", err)
	}

	if component.Selected != 1 || component.Open {
		t.Fatalf("unexpected selector state: %#v", component)
	}
}

func TestSelectorClosesWhenPointerPressMissesInputTargets(t *testing.T) {
	world := newUIWorld(t)
	selector := SpawnSelector(world, SelectorOptions{Width: 140, Height: 32, Options: []string{"A", "B"}})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 500, 500)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("outside press: %v", err)
	}
	if component.Open {
		t.Fatal("selector remained open after a pointer press outside input targets")
	}
}

func TestSelectorPopupAlwaysUsesTopmostStackRange(t *testing.T) {
	world := newUIWorld(t)
	selector := SpawnSelector(world, SelectorOptions{
		Width: 140, Height: 32, Options: []string{"A", "B"}, Theme: Theme{StackIndex: 12},
	})
	SpawnButton(world, ButtonOptions{
		Y: 32, Width: 140, Height: 32, Label: "Cover", Theme: Theme{StackIndex: 1_000_000},
	})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	list := ecs.Entity(0)
	for entity, access := range ecs.Query[SelectorList](world).All {
		current := access.Get()

		if current.Selector == selector {
			list = entity
			break
		}
	}
	if list == 0 {
		t.Fatal("selector popup list not found")
	}

	popupCount, popupMin, otherMax := 0, math.MaxInt, math.MinInt
	for entity, access := range ecs.Query[scene.UIStackIndex](world).All {
		stack := access.Get()

		popup := false
		for current := entity; current != 0; {
			if current == list {
				popup = true
				break
			}
			parent, ok := ecs.Get[scene.Parent](world, current)
			if !ok {
				break
			}
			current = parent.Entity
		}
		if popup {
			popupCount++
			popupMin = min(popupMin, int(stack))
		} else {
			otherMax = max(otherMax, int(stack))
		}
	}
	if popupCount == 0 || popupMin <= otherMax {
		t.Fatalf("selector popup stack range starts at %d, other UI reaches %d", popupMin, otherMax)
	}
}

func TestBindSelectorBuildsHiddenRuntimeListOnExistingEntity(t *testing.T) {
	world := newUIWorld(t)
	entity := spawnAnchor(world, 24, 30)
	if err := BindSelector(world, entity, SelectorOptions{
		Width: 140, Height: 32, Options: []string{"A", "B"}, ItemHeight: 28, ListHeight: 56,
	}); err != nil {
		t.Fatal(err)
	}
	transform, _ := ecs.Get[scene.UITransform](world, entity)
	width, height, _ := ControlBounds(world, entity)
	if transform.Translation.X.Value != 24 || transform.Translation.Y.Value != 30 || width != 140 || height != 32 {
		t.Fatalf("bound selector transform/bounds = %+v (%v,%v)", transform.Translation, width, height)
	}
	items := 0
	lists := 0
	for _, access := range ecs.Query2[SelectorItem, scene.Visibility](world).All {
		visible := access.B.Get()

		items++
		if visible.Visible {
			t.Fatal("bound selector option is visible while closed")
		}
	}
	for _, access := range ecs.Query2[SelectorList, scene.Visibility](world).All {
		list := access.A.Get()
		visible := access.B.Get()

		if list.Selector == entity {
			lists++
			if visible.Visible {
				t.Fatal("bound selector list is visible while closed")
			}
		}
	}
	if items != 2 || lists != 1 {
		t.Fatalf("bound selector runtime entities = %d items, %d lists", items, lists)
	}
}

func TestSelectorHighlightsFocusedOptionAndUsesCustomIcon(t *testing.T) {
	world := newUIWorld(t)
	icon := graphics.CreateTextureByColor(graphics.ColorFromRGBA(9, 8, 7, 255))
	selector := SpawnSelector(world, SelectorOptions{X: 0, Y: 0, Width: 140, Height: 32, Options: []string{"A", "B"}, Icon: TextureIcon(icon, 12, 12)})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(10, 42)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("hover: %v", err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync hover: %v", err)
	}
	focused := false
	for _, access := range ecs.Query2[SelectorItem, scene.InputState2D](world).All {
		item := access.A.Mut()
		pointer := access.B.Mut()

		if item.Owner == selector && item.Index == 0 {
			focused = pointer.Hovered
		}
	}
	if !focused {
		t.Fatal("expected focused selector option to highlight")
	}
	foundIcon := false
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		visual := access.Mut()

		if visual.Owner == selector && ecs.Has[Part[SelectorIconPart]](world, entity) {
			surface := uiSurfaceSnapshot(world, entity)
			foundIcon = surface != nil && surface.Texture == icon
		}
	}
	if !foundIcon {
		t.Fatal("expected selector to use the supplied icon texture")
	}
}

func TestSelectorHandlesSameFrameClick(t *testing.T) {
	world := newUIWorld(t)
	selector := SpawnSelector(world, SelectorOptions{Width: 140, Height: 32, Options: []string{"A", "B"}})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 10, 70)
	state.ReleaseMouse(input.MouseLeft, 10, 70)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("click: %v", err)
	}
	if component.Selected != 1 || component.Open {
		t.Fatalf("unexpected selector state: %#v", component)
	}
}

func TestSelectorListHeightScrollAndAncestorVisibility(t *testing.T) {
	world := newUIWorld(t)
	parent := SpawnPanel(world, PanelOptions{Width: 200, Height: 200})
	selector := SpawnSelector(world, SelectorOptions{
		X: 10, Y: 10, Width: 140, Height: 20, ItemHeight: 20, ListHeight: 40,
		Options: []string{"A", "B", "C", "D"},
	})
	_ = ecs.Insert(world, selector, scene.Parent{Entity: parent})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := runUIFrame(world); err != nil {
		t.Fatalf("initial layout: %v", err)
	}

	var clip *scene.ClipRect2D
	for _, access := range ecs.Query2[SelectorList, scene.ClipRect2D](world).All {
		list := access.A.Mut()
		value := access.B.Mut()

		if list.Selector == selector {
			clip = value
			break
		}
	}
	if clip == nil || clip.Height != 40 {
		t.Fatalf("unexpected selector list clip: %#v", clip)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(20, 40)
	state.ScrollMouse(0, -1)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("scroll: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("scroll layout: %v", err)
	}
	var selectorScroll float32
	for _, access := range ecs.Query2[SelectorList, scene.UIScroll](world).All {
		list := access.A.Mut()
		scroll := access.B.Mut()

		if list.Selector == selector {
			selectorScroll = -scroll.Position.Y()
			break
		}
	}
	if selectorScroll != 20 {
		t.Fatalf("expected one-row scroll, got %v", selectorScroll)
	}
	state.EndFrame()
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync scroll: %v", err)
	}
	syncTransforms(t, world)
	state.MoveMouse(20, 40)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("hover scrolled item: %v", err)
	}
	hovered := -1
	for _, access := range ecs.Query2[SelectorItem, scene.InputState2D](world).All {
		item := access.A.Mut()
		pointer := access.B.Mut()

		if item.Owner == selector && pointer.Hovered {
			hovered = item.Index
		}
	}
	if hovered != 1 {
		t.Fatalf("expected item 1 under pointer after scrolling, got %d", hovered)
	}

	parentVisibility := ecs.GetMut[scene.Visibility](world, parent)
	parentVisibility.Visible = false
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync hidden parent: %v", err)
	}
	for _, access := range ecs.Query2[SelectorItem, scene.Visibility](world).All {
		item := access.A.Mut()
		visibility := access.B.Mut()

		if item.Owner == selector && visibility.Visible {
			t.Fatalf("selector item %d remained visible with hidden ancestor", item.Index)
		}
	}
}

func TestSetSelectorScrollOffsetUpdatesUIScroll(t *testing.T) {
	world := newUIWorld(t)
	selector := SpawnSelector(world, SelectorOptions{
		Width: 140, Height: 20, ItemHeight: 20, ListHeight: 40,
		Options: []string{"A", "B", "C", "D"},
	})
	if !SetSelectorScrollOffset(world, selector, 20) {
		t.Fatal("selector list scroll was not updated")
	}
	var position float32
	for _, access := range ecs.Query2[SelectorList, scene.UIScroll](world).All {
		list := access.A.Mut()
		scroll := access.B.Mut()

		if list.Selector == selector {
			position = scroll.Position.Y()
			break
		}
	}
	if position != -20 {
		t.Fatalf("selector layout scroll = %v, want -20", position)
	}
}

func TestSelectorListOpensUpwardWhenViewportSpaceBelowIsInsufficient(t *testing.T) {
	world := newUIWorld(t)
	ecs.InsertResource(world, scene.ViewportSize{Width: 200, Height: 140, ScaleFactor: 1})
	selector := SpawnSelector(world, SelectorOptions{
		X: 20, Y: 100, Width: 140, Height: 20, ItemHeight: 20,
		Options: []string{"A", "B", "C"},
	})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	syncTransforms(t, world)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync selector: %v", err)
	}

	found := false
	for _, access := range ecs.Query3[SelectorList, scene.UITransform, scene.ClipRect2D](world).All {
		list := access.A.Mut()
		transform := access.B.Mut()
		clip := access.C.Mut()

		if list.Selector != selector {
			continue
		}
		found = true
		if transform.Translation.Y.Value != -60 || clip.Height != 60 {
			t.Fatalf("upward selector layout = y:%v height:%v, want y:-60 height:60", transform.Translation.Y.Value, clip.Height)
		}
		break
	}
	if !found {
		t.Fatal("selector list was not found")
	}
}

func TestSelectorListDefaultsToDownward(t *testing.T) {
	world := newUIWorld(t)
	ecs.InsertResource(world, scene.ViewportSize{Width: 200, Height: 200, ScaleFactor: 1})
	selector := SpawnSelector(world, SelectorOptions{
		X: 20, Y: 20, Width: 140, Height: 20, ItemHeight: 20,
		Options: []string{"A", "B", "C"},
	})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	syncTransforms(t, world)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync selector: %v", err)
	}

	found := false
	for _, access := range ecs.Query3[SelectorList, scene.UITransform, scene.ClipRect2D](world).All {
		list := access.A.Mut()
		transform := access.B.Mut()
		clip := access.C.Mut()

		if list.Selector != selector {
			continue
		}
		found = true
		if transform.Translation.Y.Value != 20 || clip.Height != 60 {
			t.Fatalf("default selector layout = y:%v height:%v, want y:20 height:60", transform.Translation.Y.Value, clip.Height)
		}
		break
	}
	if !found {
		t.Fatal("selector list was not found")
	}
}

func TestSelectorPopupHasDefaultBorder(t *testing.T) {
	world := newUIWorld(t)
	selector := SpawnSelector(world, SelectorOptions{
		Width: 140, Height: 20, ItemHeight: 20, ListHeight: 40,
		Options: []string{"A", "B"},
	})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync selector: %v", err)
	}

	panel := findSurfacePart[SelectorPanelPart](world, func() ecs.Entity {
		var listEntity ecs.Entity
		for entity, access := range ecs.Query[SelectorList](world).All {
			list := access.Get()

			if list.Selector == selector {
				listEntity = entity
				break
			}
		}
		return listEntity
	}())
	if panel == 0 {
		t.Fatal("selector popup panel not found")
	}
	border, ok := ecs.Get[scene.UIBorder2D](world, panel)
	if !ok || border.Shape.BorderWidth != 1 {
		t.Fatalf("selector popup surface border = %#v, want 1px", border)
	}
	overlay := findSurfacePart[SelectorPanelBorderPart](world, func() ecs.Entity {
		var listEntity ecs.Entity
		for entity, access := range ecs.Query[SelectorList](world).All {
			list := access.Get()

			if list.Selector == selector {
				listEntity = entity
				break
			}
		}
		return listEntity
	}())
	if overlay == 0 {
		t.Fatal("selector popup border overlay not found")
	}
	overlayBorder, ok := ecs.Get[scene.UIBorder2D](world, overlay)
	if !ok || overlayBorder.Shape.BorderWidth != 1 {
		t.Fatalf("selector popup overlay border = %#v, want 1px", overlayBorder)
	}
}

func TestSelectorListFitsViewportAndShowsScrollbarWhenNeitherDirectionFits(t *testing.T) {
	world := newUIWorld(t)
	ecs.InsertResource(world, scene.ViewportSize{Width: 200, Height: 100, ScaleFactor: 1})
	selector := SpawnSelector(world, SelectorOptions{
		X: 20, Y: 40, Width: 140, Height: 20, ItemHeight: 20,
		Options: []string{"A", "B", "C", "D", "E"},
	})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	syncTransforms(t, world)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync selector: %v", err)
	}

	foundList, foundTrack, foundThumb := false, false, false
	for _, access := range ecs.Query3[SelectorList, scene.UITransform, scene.ClipRect2D](world).All {
		list := access.A.Mut()
		transform := access.B.Mut()
		clip := access.C.Mut()

		if list.Selector != selector {
			continue
		}
		foundList = true
		if transform.Translation.Y.Value != 20 || clip.Height != 40 {
			t.Fatalf("fitted selector layout = y:%v height:%v, want y:20 height:40", transform.Translation.Y.Value, clip.Height)
		}
		break
	}
	for entity, access := range ecs.Query2[Relation[SurfaceStyle], scene.Size2D](world).All {
		visual := access.A.Mut()
		size := access.B.Mut()

		list, ok := ecs.Get[SelectorList](world, visual.Owner)
		if !ok || list.Selector != selector {
			continue
		}
		switch {
		case ecs.Has[Part[SelectorScrollTrackPart]](world, entity):
			foundTrack = size.Width == selectorScrollBarSize && size.Height == 40
		case ecs.Has[Part[SelectorScrollThumbPart]](world, entity):
			foundThumb = size.Width == selectorScrollThumbSize && size.Height == 20
		}
	}
	if !foundList || !foundTrack || !foundThumb {
		t.Fatalf("adaptive selector parts = list:%v track:%v thumb:%v", foundList, foundTrack, foundThumb)
	}
}

func TestSelectorListConsumesWheelBeforeUnderlyingScrollArea(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		X: 0, Y: 20, Width: 160, Height: 80,
		ContentWidth: 280, ContentHeight: 280,
	})
	selector := SpawnSelector(world, SelectorOptions{
		Width: 140, Height: 20, ItemHeight: 20, ListHeight: 40,
		Options: []string{"A", "B", "C", "D"},
	})
	component := ecs.GetMut[Selector](world, selector)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync open selector: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("initial layout: %v", err)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(10, 30)
	state.ScrollMouse(0, -1)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("wheel update: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("wheel layout: %v", err)
	}

	underlying, _ := GetScrollAreaOffset(world, area)
	var selectorScroll float32
	for _, access := range ecs.Query2[SelectorList, scene.UIScroll](world).All {
		list := access.A.Mut()
		scroll := access.B.Mut()

		if list.Selector == selector {
			selectorScroll = -scroll.Position.Y()
			break
		}
	}
	if selectorScroll != 20 {
		t.Fatalf("selector scroll y = %v, want 20", selectorScroll)
	}
	if underlying.Y() != 0 {
		t.Fatalf("underlying scroll area received selector wheel: %v", underlying.Y())
	}
	if pointer := state.Pointer(); pointer.WheelX != 0 || pointer.WheelY != 0 {
		t.Fatalf("selector wheel was not consumed: %+v", pointer)
	}

	state.EndFrame()
	state.ScrollMouse(1, 0)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("horizontal wheel update: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("horizontal wheel layout: %v", err)
	}
	if pointer := state.Pointer(); pointer.WheelX != 0 || pointer.WheelY != 0 {
		t.Fatalf("selector horizontal wheel was not consumed: %+v", pointer)
	}
	underlying, _ = GetScrollAreaOffset(world, area)
	if underlying.X() != 0 {
		t.Fatalf("underlying scroll area received selector horizontal wheel: %v", underlying.X())
	}
}

func TestTabSwitchesPanelVisibility(t *testing.T) {
	world := newUIWorld(t)
	tabs := SpawnTab(world, TabOptions{X: 0, Y: 0, Width: 220, Height: 140, Items: tabItems("Info", "Logs")})
	infoPanel := SpawnTabPanel(world, tabs, TabPanelOptions{Index: 0})
	logPanel := SpawnTabPanel(world, tabs, TabPanelOptions{Index: 1})
	infoLabel := SpawnLabel(world, LabelOptions{X: 12, Y: 12, Value: "Info body"})
	logLabel := SpawnLabel(world, LabelOptions{X: 12, Y: 12, Value: "Log body"})
	_ = ecs.Insert(world, infoLabel, scene.Parent{Entity: infoPanel})
	_ = ecs.Insert(world, logLabel, scene.Parent{Entity: logPanel})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)

	infoVisibility := ecs.GetMut[scene.Visibility](world, infoLabel)
	logVisibility := ecs.GetMut[scene.Visibility](world, logLabel)
	if infoVisibility == nil || !infoVisibility.Visible || logVisibility == nil || logVisibility.Visible {
		t.Fatalf("unexpected initial visibility: info=%#v log=%#v", infoVisibility, logVisibility)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 140, 18)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 140, 18)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync after click: %v", err)
	}

	component := ecs.GetMut[Tab](world, tabs)
	if component.Selected != 1 {
		t.Fatalf("unexpected tab state: %#v", component)
	}
	if infoVisibility.Visible || !logVisibility.Visible {
		t.Fatalf("unexpected tab visibility: info=%#v log=%#v", infoVisibility, logVisibility)
	}
	events := (&ecs.MessageReader[SelectedEvent[string]]{}).Read(world)
	if len(events) != 1 || events[0].Entity != tabs || events[0].Value != "Logs" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestTabTextAndIconAreVerticallyCenteredWithoutOverlap(t *testing.T) {
	world := newUIWorld(t)
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 80, HeaderHeight: 40, Items: []TabItemSpec{{Label: "Info", Icon: TextIcon("#")}}})
	tab := ecs.GetMut[Tab](world, tabs)
	tab.HeaderHeight = 52
	for _, access := range ecs.Query[TabItem](world).All {
		item := access.Mut()

		if item.Tabs == tabs {
			item.Icon = TextIcon("WWW")
		}
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var iconRight float32
	var labelX float32
	foundIcon := false
	foundLabel := false
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()
		transform := access.C.Mut()

		item := ecs.GetMut[TabItem](world, visual.Owner)
		if item == nil || item.Tabs != tabs {
			continue
		}
		width, lineHeight, measured := scene.MeasureTextValue2D(text, text.Value)
		if !measured {
			t.Fatal("expected tab text to be measurable")
		}
		wantY := (float32(52) - lineHeight) * 0.5
		if math.Abs(float64(transform.Translation.Y.Value-wantY)) > 0.001 {
			t.Fatalf("expected vertically centered tab text y=%v, got %v", wantY, transform.Translation.Y.Value)
		}
		switch {
		case ecs.Has[Part[IconPart]](world, entity):
			foundIcon = true
			iconRight = transform.Translation.X.Value + width
		case ecs.Has[Part[ItemPart]](world, entity):
			foundLabel = true
			labelX = transform.Translation.X.Value
		}
	}
	if !foundIcon || !foundLabel {
		t.Fatalf("expected tab icon and label, icon=%v label=%v", foundIcon, foundLabel)
	}
	if math.Abs(float64(labelX-iconRight-textIconGap)) > 0.001 {
		t.Fatalf("expected tab icon gap %v, got %v", textIconGap, labelX-iconRight)
	}
}

func TestTabUsesCustomItemWidths(t *testing.T) {
	world := newUIWorld(t)
	tabs := SpawnTab(world, TabOptions{Width: 100, Height: 40, Items: tabItems("Short", "Long")})
	for _, access := range ecs.Query[TabItem](world).All {
		item := access.Mut()

		if item.Tabs != tabs {
			continue
		}
		item.Width = 30 + float32(item.Index)*40
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	items := make([]ecs.Entity, 2)
	for entity, access := range ecs.Query[TabItem](world).All {
		item := access.Mut()

		if item.Tabs == tabs && item.Index >= 0 && item.Index < len(items) {
			items[item.Index] = entity
		}
	}
	for index, entity := range items {
		width, _, ok := ControlBounds(world, entity)
		if !ok || width != 30+float32(index)*40 {
			t.Fatalf("tab item %d width = %v, want %v", index, width, 30+float32(index)*40)
		}
		transform, ok := ecs.Get[scene.UITransform](world, entity)
		wantX := [...]float32{0, 30}[index]
		if !ok || transform.Translation.X.Value != wantX {
			t.Fatalf("tab item %d x = %v, want %v", index, transform.Translation.X.Value, wantX)
		}
	}
}

func TestTabReservesGapForTextureIcon(t *testing.T) {
	world := newUIWorld(t)
	icon := graphics.CreateTextureByColor(graphics.White)
	tabs := SpawnTab(world, TabOptions{Width: 120, Height: 40, Items: []TabItemSpec{{Label: "Files", Icon: TextureIcon(icon, 16, 16)}}})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	labelX := float32(-1)
	for entity, access := range ecs.Query2[Relation[scene.Text2D], scene.UITransform](world).All {
		visual := access.A.Mut()
		transform := access.B.Mut()

		item := ecs.GetMut[TabItem](world, visual.Owner)
		if item != nil && item.Tabs == tabs && ecs.Has[Part[ItemPart]](world, entity) {
			labelX = transform.Translation.X.Value
		}
	}
	want := textIconX + 16 + textIconGap
	if labelX != want {
		t.Fatalf("expected external tab icon gap at x=%v, got %v", want, labelX)
	}
}

func TestCheckboxTogglesAsStandardBox(t *testing.T) {
	world := newUIWorld(t)
	checkbox := SpawnCheckbox(world, CheckboxOptions{X: 0, Y: 0, Width: 140, Height: 36, Label: "Visible"})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("initial layout: %v", err)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 12, 18)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 12, 18)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync after click: %v", err)
	}

	component := ecs.GetMut[Toggle[bool]](world, checkbox)
	if !component.Value || !component.Changed {
		t.Fatalf("unexpected checkbox state: %#v", component)
	}
	mark := checkboxMarkSurface(world, checkbox)
	if mark == nil || mark.Width == 0 || mark.Height == 0 {
		t.Fatalf("unexpected checkbox mark: %#v", mark)
	}
	if mark.Texture != nil || mark.Shape.Kind != render.ShapeCheckmark {
		t.Fatalf("checkbox mark did not use the GPU shape pipeline: %#v", mark)
	}
	events := (&ecs.MessageReader[ChangedEvent[bool]]{}).Read(world)
	if len(events) != 1 || events[0].Entity != checkbox || !events[0].Value {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func checkboxMarkSurface(world *ecs.World, owner ecs.Entity) *uiSurface {
	var result *uiSurface
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		visual := access.Mut()

		if visual.Owner == owner && ecs.Has[Part[KnobPart]](world, entity) {
			result = uiSurfaceSnapshot(world, entity)
			break
		}
	}
	return result
}

func TestContextMenuSelectsItemAndCloses(t *testing.T) {
	world := newUIWorld(t)
	menu := SpawnContextMenu(world, ContextMenuOptions{X: 0, Y: 0, Width: 160, ItemHeight: 30, Items: []string{"Open", "Delete"}, Open: true})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)

	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 16, 44)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press: %v", err)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 16, 44)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release: %v", err)
	}

	component := ecs.GetMut[ContextMenu](world, menu)
	if component.Open || component.Selected != 1 {
		t.Fatalf("unexpected context menu state: %#v", component)
	}
	events := (&ecs.MessageReader[SelectedEvent[string]]{}).Read(world)
	if len(events) != 1 || events[0].Entity != menu || events[0].Value != "Delete" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestMenuPanelUsesPaddingAutoWidthAndCenteredItems(t *testing.T) {
	world := newUIWorld(t)
	menu := SpawnMenu(world, MenuOptions{
		Width: 40, Height: 30, Label: "File",
		Items: []string{"Open", "A much wider menu item"}, Padding: 8,
	})
	component := ecs.GetMut[Menu](world, menu)
	component.Open = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	panelWidth := float32(0)
	for entity, access := range ecs.Query2[Relation[SurfaceStyle], scene.UITransform](world).All {
		visual := access.A.Mut()
		transform := access.B.Mut()

		if visual.Owner == menu && ecs.Has[Part[MenuPanelPart]](world, entity) {
			surface := uiSurfaceSnapshot(world, entity)
			if surface == nil {
				continue
			}
			panelWidth = surface.Width
			if transform.Translation.Y.Value != 30 {
				t.Fatalf("expected menu panel below trigger at y=30, got %v", transform.Translation.Y.Value)
			}
			if surface.Height != 76 {
				t.Fatalf("expected padded menu panel height 76, got %v", surface.Height)
			}
		}
	}
	if panelWidth <= 40 {
		t.Fatalf("expected menu panel to expand beyond minimum width, got %v", panelWidth)
	}
	assertMenuItemLayout(t, world, menu, 8, 38, 30, panelWidth)
}

func TestContextMenuPanelUsesPaddingAutoWidthAndCenteredItems(t *testing.T) {
	world := newUIWorld(t)
	menu := SpawnContextMenu(world, ContextMenuOptions{
		Width: 40, ItemHeight: 30,
		Items: []string{"Open", "A much wider context item"}, Open: true, Padding: 8,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	panelWidth := float32(0)
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		visual := access.Mut()

		if visual.Owner == menu && ecs.Has[Part[ContextMenuPanelPart]](world, entity) {
			surface := uiSurfaceSnapshot(world, entity)
			if surface == nil {
				continue
			}
			panelWidth = surface.Width
			if surface.Height != 76 {
				t.Fatalf("expected padded context menu height 76, got %v", surface.Height)
			}
		}
	}
	if panelWidth <= 40 {
		t.Fatalf("expected context menu to expand beyond minimum width, got %v", panelWidth)
	}
	assertContextMenuItemLayout(t, world, menu, 8, 8, 30, panelWidth)
}

func assertMenuItemLayout(t *testing.T, world *ecs.World, menu ecs.Entity, wantX, wantY, wantHeight, panelWidth float32) {
	t.Helper()
	found := false
	for entity, access := range ecs.Query2[MenuItem, scene.UITransform](world).All {
		item := access.A.Mut()
		transform := access.B.Mut()

		if item.Owner != menu || item.Index != 0 {
			continue
		}
		found = true
		assertPaddedCenteredMenuItem(t, world, entity, transform, wantX, wantY, wantHeight, panelWidth)
		break
	}
	if !found {
		t.Fatal("expected menu item")
	}
}

func assertContextMenuItemLayout(t *testing.T, world *ecs.World, menu ecs.Entity, wantX, wantY, wantHeight, panelWidth float32) {
	t.Helper()
	found := false
	for entity, access := range ecs.Query2[ContextMenuItem, scene.UITransform](world).All {
		item := access.A.Mut()
		transform := access.B.Mut()

		if item.Owner != menu || item.Index != 0 {
			continue
		}
		found = true
		assertPaddedCenteredMenuItem(t, world, entity, transform, wantX, wantY, wantHeight, panelWidth)
		break
	}
	if !found {
		t.Fatal("expected context menu item")
	}
}

func assertPaddedCenteredMenuItem(t *testing.T, world *ecs.World, item ecs.Entity, transform *scene.UITransform, wantX, wantY, wantHeight, panelWidth float32) {
	t.Helper()
	if transform.Translation.X.Value != wantX || transform.Translation.Y.Value != wantY {
		t.Fatalf("expected padded menu item at (%v,%v), got (%v,%v)", wantX, wantY, transform.Translation.X.Value, transform.Translation.Y.Value)
	}
	width, _, ok := ControlBounds(world, item)
	if !ok || width != panelWidth-wantX*2 {
		t.Fatalf("expected symmetric horizontal padding, panel=%v width=%v", panelWidth, width)
	}
	foundText := false
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()
		textTransform := access.C.Mut()

		if visual.Owner != item || !ecs.Has[Part[ItemPart]](world, entity) {
			continue
		}
		foundText = true
		_, lineHeight, ok := scene.MeasureTextValue2D(text, text.Value)
		if !ok {
			t.Fatal("expected menu item text to be measurable")
		}
		wantTextY := (wantHeight - lineHeight) * 0.5
		if math.Abs(float64(textTransform.Translation.Y.Value-wantTextY)) > 0.001 {
			t.Fatalf("expected centered menu text y=%v, got %v", wantTextY, textTransform.Translation.Y.Value)
		}
		stackIndex, _ := ecs.Get[scene.UIStackIndex](world, entity)
		itemStackIndex, _ := ecs.Get[scene.UIStackIndex](world, item)
		if stackIndex <= itemStackIndex {
			t.Fatalf("expected menu text stackIndex %d above item background stackIndex %d", stackIndex, itemStackIndex)
		}
		break
	}
	if !foundText {
		t.Fatal("expected menu item text")
	}
}

func TestProgressSyncsFillWidth(t *testing.T) {
	world := newUIWorld(t)
	progress := SpawnProgress(world, ProgressOptions{X: 0, Y: 0, Width: 200, Height: 12, Min: 0, Max: 100, Value: 25})
	syncTransforms(t, world)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	component := ecs.GetMut[Progress](world, progress)
	fill, _ := ecs.Get[scene.Size2D](world, component.Fill)
	if fill.Width != 50 {
		t.Fatalf("expected fill width 50, got %v", fill.Width)
	}
}

func TestProgressSliderIsOptionalAndDraggable(t *testing.T) {
	world := newUIWorld(t)
	controlled := SpawnProgress(world, ProgressOptions{Width: 200, Height: 10, Min: 0, Max: 100, Value: 25})
	controlledProgress, _ := ecs.Get[Progress](world, controlled)
	thumbVisibility, _ := ecs.Get[scene.Visibility](world, controlledProgress.Thumb)
	if controlledProgress.Thumb == 0 || thumbVisibility.Visible {
		t.Fatalf("program-controlled progress should hide its slider: progress=%#v visibility=%#v", controlledProgress, thumbVisibility)
	}
	controlledComponent := ecs.GetMut[Progress](world, controlled)
	controlledComponent.Slider = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("enable slider: %v", err)
	}
	thumbVisibility, _ = ecs.Get[scene.Visibility](world, controlledProgress.Thumb)
	if !thumbVisibility.Visible || !ecs.Has[CornerRadius](world, controlledProgress.Thumb) {
		t.Fatalf("enabling slider did not reveal its thumb: %#v", thumbVisibility)
	}

	slider := SpawnProgress(world, ProgressOptions{X: 10, Y: 20, Width: 200, Height: 10, Min: 0, Max: 100, Value: 25, Slider: true})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}
	syncTransforms(t, world)
	state, _ := ecs.GetResource[*input.State](world)
	state.PressMouse(input.MouseLeft, 110, 25)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("update: %v", err)
	}

	progress, _ := ecs.Get[Progress](world, slider)
	if progress.Thumb == 0 || !progress.Changed || math.Abs(float64(progress.Value-50)) > .001 {
		t.Fatalf("unexpected slider state: %#v", progress)
	}
	events := (&ecs.MessageReader[ChangedEvent[float32]]{}).Read(world)
	if len(events) != 1 || events[0].Entity != slider || events[0].Value != 50 {
		t.Fatalf("unexpected slider events: %#v", events)
	}
}

func TestScrollAreaAutoShowsBothScrollbars(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		X: 0, Y: 0, Width: 120, Height: 80,
		ContentWidth: 200, ContentHeight: 160,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	component := ecs.GetMut[ScrollArea](world, area)
	if component == nil {
		t.Fatalf("expected scroll area component")
	}
	metrics, _ := GetScrollAreaMetrics(world, area)
	if !metrics.HorizontalVisible || !metrics.VerticalVisible {
		t.Fatalf("expected both scrollbars to be visible, got %#v", metrics)
	}
	if metrics.ViewportWidth != 120 || metrics.ViewportHeight != 80 {
		t.Fatalf("unexpected viewport size: got %.0fx%.0f", metrics.ViewportWidth, metrics.ViewportHeight)
	}
	if metrics.MaxScrollX != 80 || metrics.MaxScrollY != 80 {
		t.Fatalf("unexpected max scroll: got %.0f/%.0f", metrics.MaxScrollX, metrics.MaxScrollY)
	}
	clip := ecs.GetMut[scene.ClipRect2D](world, area)
	if clip == nil {
		t.Fatalf("expected scroll area clip")
	}
	if clip.X != 0 || clip.Y != 0 || clip.Width != 120 || clip.Height != 80 {
		t.Fatalf("unexpected clip rect: %#v", clip)
	}
	var horizontalTrack, verticalTrack bool
	for entity, access := range ecs.Query3[Relation[SurfaceStyle], scene.Size2D, scene.UITransform](world).All {
		visual := access.A.Mut()
		size := access.B.Mut()
		transform := access.C.Mut()

		if visual.Owner != area {
			continue
		}
		switch {
		case ecs.Has[Part[ScrollTrackHorizontalPart]](world, entity):
			horizontalTrack = size.Width == 120 && size.Height == 10 && transform.Translation.X.Value == 0 && transform.Translation.Y.Value == 70
		case ecs.Has[Part[ScrollTrackVerticalPart]](world, entity):
			verticalTrack = size.Width == 10 && size.Height == 80 && transform.Translation.X.Value == 110 && transform.Translation.Y.Value == 0
		}
	}
	if !horizontalTrack || !verticalTrack {
		t.Fatalf("scrollbar tracks did not use independent full-axis geometry: horizontal=%v vertical=%v", horizontalTrack, verticalTrack)
	}
	exemptVisuals := 0
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		visual := access.Mut()

		if visual.Owner == area && ecs.Has[scene.ClipExempt2D](world, entity) {
			exemptVisuals++
		}
	}
	if exemptVisuals != 4 {
		t.Fatalf("expected 4 clip-exempt scrollbar visuals, got %d", exemptVisuals)
	}
}

func TestScrollAreaMeasuresScrollbarAxesIndependently(t *testing.T) {
	tests := []struct {
		name                          string
		width, height                 float32
		contentWidth, contentHeight   float32
		horizontal, vertical          bool
		viewportWidth, viewportHeight float32
		maxScrollX, maxScrollY        float32
	}{
		{
			name:  "vertical overflow does not reduce horizontal viewport",
			width: 120, height: 80, contentWidth: 120, contentHeight: 160,
			vertical: true, viewportWidth: 120, viewportHeight: 80, maxScrollY: 80,
		},
		{
			name:  "horizontal overflow does not reduce vertical viewport",
			width: 120, height: 80, contentWidth: 200, contentHeight: 80,
			horizontal: true, viewportWidth: 120, viewportHeight: 80, maxScrollX: 80,
		},
		{
			name:  "both axes keep their full viewport",
			width: 120, height: 80, contentWidth: 200, contentHeight: 160,
			horizontal: true, vertical: true, viewportWidth: 120, viewportHeight: 80,
			maxScrollX: 80, maxScrollY: 80,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metrics := MeasureScrollArea(test.width, test.height, test.contentWidth, test.contentHeight)
			if metrics.HorizontalVisible != test.horizontal || metrics.VerticalVisible != test.vertical {
				t.Fatalf("visibility = horizontal:%v vertical:%v, want horizontal:%v vertical:%v", metrics.HorizontalVisible, metrics.VerticalVisible, test.horizontal, test.vertical)
			}
			if metrics.ViewportWidth != test.viewportWidth || metrics.ViewportHeight != test.viewportHeight {
				t.Fatalf("viewport = %.0fx%.0f, want %.0fx%.0f", metrics.ViewportWidth, metrics.ViewportHeight, test.viewportWidth, test.viewportHeight)
			}
			if metrics.MaxScrollX != test.maxScrollX || metrics.MaxScrollY != test.maxScrollY {
				t.Fatalf("max scroll = %.0f/%.0f, want %.0f/%.0f", metrics.MaxScrollX, metrics.MaxScrollY, test.maxScrollX, test.maxScrollY)
			}
		})
	}
}

func TestScrollAreaHidesScrollbarsWhenContentFits(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		X: 0, Y: 0, Width: 160, Height: 90,
		ContentWidth: 120, ContentHeight: 60,
		ScrollX: 40, ScrollY: 20,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync: %v", err)
	}

	component := ecs.GetMut[ScrollArea](world, area)
	if component == nil {
		t.Fatalf("expected scroll area component")
	}
	metrics, _ := GetScrollAreaMetrics(world, area)
	if metrics.HorizontalVisible || metrics.VerticalVisible {
		t.Fatalf("expected scrollbars to stay hidden, got %#v", metrics)
	}
	offset, _ := GetScrollAreaOffset(world, area)
	if offset.X() != 0 || offset.Y() != 0 {
		t.Fatalf("expected scroll offsets to clamp to zero, got %.0f/%.0f", offset.X(), offset.Y())
	}
}

func TestScrollAreaWheelMovesContent(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		X: 10, Y: 20, Width: 120, Height: 80,
		ContentWidth: 120, ContentHeight: 280,
	})
	content := SpawnScrollContent(world, area, 4, 6)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("initial layout: %v", err)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(30, 40)
	state.ScrollMouse(0, -1)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("wheel update: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("content layout: %v", err)
	}

	areaOffset, _ := GetScrollAreaOffset(world, area)
	if areaOffset.Y() != 48 {
		t.Fatalf("scroll y = %v, want 48", areaOffset.Y())
	}
	transform, _ := ecs.Get[scene.UITransform](world, content)
	if transform.Translation.X.Value != 4 || transform.Translation.Y.Value != -42 {
		t.Fatalf("content transform = (%v, %v), want (4, -42)", transform.Translation.X.Value, transform.Translation.Y.Value)
	}
}

func TestControlledScrollAreaLeavesWheelForOwner(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		X: 10, Y: 20, Width: 120, Height: 80,
		ContentWidth: 120, ContentHeight: 280,
		Controlled: true,
	})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("initial layout: %v", err)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(30, 40)
	state.ScrollMouse(0, -1)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("wheel update: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("controlled wheel layout: %v", err)
	}

	component, _ := GetScrollAreaOffset(world, area)
	if component.Y() != 0 {
		t.Fatalf("controlled scroll y = %v, want 0", component.Y())
	}
	if state.Pointer().WheelY != -1 {
		t.Fatalf("wheel y = %v, want -1 for the owner", state.Pointer().WheelY)
	}
}

func TestScrollAreaStableUIScrollFrameDoesNotAllocate(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{
		Width: 120, Height: 80, ContentWidth: 120, ContentHeight: 280,
	})
	SpawnScrollContent(world, area, 4, 6)
	if err := runUIFrame(world); err != nil {
		t.Fatalf("warm layout frame: %v", err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		if err := runUIFrame(world); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("stable UI layout frame allocations = %v, want 0", allocations)
	}
}

func TestTreeTapSelectsNode(t *testing.T) {
	world := newUIWorld(t)
	tree := SpawnTree[string](world, TreeOptions[string]{
		X: 0, Y: 0, Width: 220, RowHeight: 30,
		Nodes: []TreeNodeSpec[string]{
			{ID: "root", Label: "Root", Expanded: true},
			{ID: "child", Parent: "root", HasParent: true, Label: "Child", Leaf: true},
		},
	})
	syncTransforms(t, world)

	var child ecs.Entity
	for entity, access := range ecs.Query[TreeNode[string]](world).All {
		node := access.Mut()

		if node.ID == "child" {
			child = entity
			break
		}
	}
	ecs.WriteMessage(world, scene.InputEvent{Entity: child, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatalf("update tree: %v", err)
	}

	component := ecs.GetMut[Tree[string]](world, tree)
	if !component.HasSelection || component.Selected != "child" {
		t.Fatalf("unexpected tree selection: %#v", component)
	}
}

func TestTreeCollapsedRowsReflow(t *testing.T) {
	world := newUIWorld(t)
	tree := SpawnTree[string](world, TreeOptions[string]{
		X: 0, Y: 0, Width: 220, RowHeight: 30,
		Nodes: []TreeNodeSpec[string]{
			{ID: "root", Label: "Root"},
			{ID: "child", Parent: "root", HasParent: true, Label: "Child", Leaf: true},
			{ID: "sibling", Label: "Sibling", Leaf: true},
		},
	})
	if err := SyncTreeSystem[string]("").Run(world); err != nil {
		t.Fatalf("sync tree: %v", err)
	}

	var childEntity ecs.Entity
	var siblingEntity ecs.Entity
	for entity, access := range ecs.Query[TreeNode[string]](world).All {
		node := access.Mut()

		switch node.ID {
		case "child":
			childEntity = entity
		case "sibling":
			siblingEntity = entity
		}
	}

	childVisibility := ecs.GetMut[scene.Visibility](world, childEntity)
	if childVisibility == nil || childVisibility.Visible {
		t.Fatalf("expected collapsed child to be hidden, got %#v", childVisibility)
	}
	siblingTransform := ecs.GetMut[scene.UITransform](world, siblingEntity)
	if siblingTransform == nil || siblingTransform.Translation.Y.Value != 32 {
		t.Fatalf("expected sibling row to reflow to y=32, got %#v", siblingTransform)
	}
	component := ecs.GetMut[Tree[string]](world, tree)
	if component.RowHeight != 30 {
		t.Fatalf("unexpected row height: %#v", component)
	}
}

func TestTreeSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	nodes := make([]TreeNodeSpec[int], 100)
	nodes[0] = TreeNodeSpec[int]{ID: 0, Label: "Root", Expanded: true}
	for index := 1; index < len(nodes); index++ {
		nodes[index] = TreeNodeSpec[int]{ID: index, Parent: 0, HasParent: true, Label: "Node", Leaf: true}
	}
	SpawnTree[int](world, TreeOptions[int]{Width: 220, RowHeight: 30, Nodes: nodes})
	schedule := ecs.NewSchedule("tree_sync_allocations")
	if err := schedule.Add(SyncTreeSystem[int]("")); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatal(err)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		if err := schedule.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("tree sync allocated %.0f objects, want 0", allocations)
	}
}

func TestTreeSyncRejectsParentCycles(t *testing.T) {
	world := newUIWorld(t)
	SpawnTree[string](world, TreeOptions[string]{
		Width: 220, RowHeight: 30,
		Nodes: []TreeNodeSpec[string]{
			{ID: "first", Parent: "second", HasParent: true, Expanded: true},
			{ID: "second", Parent: "first", HasParent: true, Expanded: true},
		},
	})

	err := SyncTreeSystem[string]("").Run(world)
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Fatalf("expected tree parent cycle error, got %v", err)
	}
}

func valueText(world *ecs.World, owner ecs.Entity) string {
	value := ""
	for entity, access := range ecs.Query2[Relation[scene.Text2D], scene.Text2D](world).All {
		visual := access.A.Mut()
		text := access.B.Mut()

		if visual.Owner == owner && ecs.Has[Part[ValuePart]](world, entity) {
			value = text.Value
			break
		}
	}
	return value
}

func textFieldValueAndCaret(world *ecs.World, entity ecs.Entity) (string, int) {
	field, _ := ecs.Get[TextField](world, entity)
	return field.Value, field.Caret
}

func caretSurface(world *ecs.World, owner ecs.Entity) *uiSurface {
	var caret *uiSurface
	for entity, access := range ecs.Query[Relation[SurfaceStyle]](world).All {
		visual := access.Mut()

		if visual.Owner == owner && ecs.Has[Part[CaretPart]](world, entity) {
			caret = uiSurfaceSnapshot(world, entity)
			break
		}
	}
	return caret
}

func caretVisibleState(world *ecs.World, owner ecs.Entity) bool {
	visible := false
	for entity, access := range ecs.Query2[Relation[SurfaceStyle], scene.Visibility](world).All {
		visual := access.A.Mut()
		visibility := access.B.Mut()

		if visual.Owner == owner && ecs.Has[Part[CaretPart]](world, entity) {
			visible = visibility.Visible
			break
		}
	}
	return visible
}

type uiSurface struct {
	Width   float32
	Height  float32
	Shape   scene.Shape2D
	Texture *graphics.Texture
}

func uiSurfaceSnapshot(world *ecs.World, entity ecs.Entity) *uiSurface {
	if !ecs.Has[scene.UIBackground2D](world, entity) && !ecs.Has[scene.UIImage2D](world, entity) && !ecs.Has[scene.UIBorder2D](world, entity) {
		entity = findSurfacePart[BodyPart](world, entity)
		if entity == 0 {
			return nil
		}
	}
	size, sizeOK := ecs.Get[scene.Size2D](world, entity)
	if !sizeOK {
		return nil
	}
	surface := &uiSurface{Width: size.Width, Height: size.Height}
	if background, ok := ecs.Get[scene.UIBackground2D](world, entity); ok {
		surface.Shape = background.Shape
	}
	if image, ok := ecs.Get[scene.UIImage2D](world, entity); ok {
		surface.Texture = image.Texture
	}
	return surface
}

func TestInteractiveRootOwnsExplicitBodyElement(t *testing.T) {
	world := newUIWorld(t)
	button := SpawnButton(world, ButtonOptions{Width: 80, Height: 32, Label: "Run"})
	if ecs.Has[scene.UIBackground2D](world, button) || ecs.Has[scene.UIImage2D](world, button) || ecs.Has[scene.UIBorder2D](world, button) {
		t.Fatal("interactive root still owns implicit render components")
	}
	body := findSurfacePart[BodyPart](world, button)
	if body == 0 {
		t.Fatal("button is missing its explicit body child")
	}
	parent, ok := ecs.Get[scene.Parent](world, body)
	if !ok || parent.Entity != button {
		t.Fatalf("body parent = %#v, want button %d", parent, button)
	}
}

func tabItems(labels ...string) []TabItemSpec {
	items := make([]TabItemSpec, len(labels))
	for index, label := range labels {
		items[index].Label = label
	}
	return items
}

func newUIWorld(t *testing.T) *ecs.World {
	t.Helper()
	world := ecs.NewWorld()
	RegisterRuntimeFeature[FullRuntime](world)
	ecs.InsertResource(world, input.NewState())
	camera := scene.SpawnCamera2D(world)
	component := ecs.GetMut[scene.Camera](world, camera)
	component.UpdateViewport(1920, 1080)
	if err := scene.SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	theme := DefaultTheme()
	theme.Font = notosanssc.NotoSansSCRegular
	ecs.InsertResource(world, theme)
	return world
}

func runUIUpdate(world *ecs.World) error {
	if err := scene.DispatchInputEvents2DSystem("").Run(world); err != nil {
		return err
	}
	return UpdateSystem("").Run(world)
}

func runUIFrame(world *ecs.World) error {
	if err := SyncSystem("").Run(world); err != nil {
		return err
	}
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		return err
	}
	if err := SyncAfterLayoutSystem("").Run(world); err != nil {
		return err
	}
	return scene.SyncUIGlobalTransformSystem("").Run(world)
}

func TestThemeForWorldKeepsConfiguredFontForPartialOverrides(t *testing.T) {
	world := newUIWorld(t)
	theme := themeForWorld(world, Theme{StackIndex: 7})
	if theme.Font != notosanssc.NotoSansSCRegular || theme.StackIndex != 7 {
		t.Fatalf("resolved theme = %+v", theme)
	}
}

func syncTransforms(t *testing.T, world *ecs.World) {
	t.Helper()
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatalf("sync transform: %v", err)
	}
}
