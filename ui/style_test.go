package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestStateStyleResolutionAndFallback(t *testing.T) {
	style := NewStateStyle("normal").
		WithHovered("hovered").
		WithPressed("pressed").
		WithSelected("selected").
		WithFocused("focused").
		WithDisabled("disabled")
	tests := []struct {
		name  string
		state ControlState
		want  string
	}{
		{name: "normal", want: "normal"},
		{name: "focused", state: ControlState{Focused: true}, want: "focused"},
		{name: "hovered", state: ControlState{Focused: true, Hovered: true}, want: "hovered"},
		{name: "selected", state: ControlState{Focused: true, Hovered: true, Selected: true}, want: "selected"},
		{name: "pressed", state: ControlState{Focused: true, Hovered: true, Pressed: true}, want: "pressed"},
		{name: "disabled", state: ControlState{Focused: true, Hovered: true, Pressed: true, Disabled: true}, want: "disabled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := style.Resolve(test.state); got != test.want {
				t.Fatalf("resolved style = %q, want %q", got, test.want)
			}
		})
	}
	if state := controlState(&controlView{Hovered: true, Selected: true}, false); !state.Hovered || !state.Selected {
		t.Fatalf("control hover and selection states were not preserved independently: %+v", state)
	}

	fallback := NewStateStyle("normal").WithHovered("hovered")
	if got := fallback.Resolve(ControlState{Hovered: true, Disabled: true}); got != "normal" {
		t.Fatalf("missing disabled style fell through to %q, want normal", got)
	}
}

func TestToggleStyleResolution(t *testing.T) {
	style := ToggleStyle[string]{
		Off: NewStateStyle("off").WithHovered("off-hovered"),
		On:  NewStateStyle("on").WithHovered("on-hovered"),
	}
	if got := style.Resolve(false, ControlState{Hovered: true}); got != "off-hovered" {
		t.Fatalf("off toggle style = %q", got)
	}
	if got := style.Resolve(true, ControlState{Hovered: true}); got != "on-hovered" {
		t.Fatalf("on toggle style = %q", got)
	}
}

func TestAllWidgetStylesAcceptNineSliceSurfaces(t *testing.T) {
	texture := graphics.CreateTextureByColor(graphics.White)
	slice := scene.NineSlice2D{Left: 1, Right: 1, Top: 1, Bottom: 1}
	surface := NineSliceSurface(texture, slice)
	state := NewStateStyle(surface)
	button := ButtonStyle{Body: state, JoinedEdge: state}

	tests := []struct {
		name   string
		spawn  func(*ecs.World) ecs.Entity
		target func(*ecs.World, ecs.Entity) ecs.Entity
		tree   bool
	}{
		{name: "panel", spawn: func(world *ecs.World) ecs.Entity {
			return SpawnPanel(world, PanelOptions{Width: 100, Height: 40, Style: &PanelStyle{Body: surface}})
		}},
		{name: "button", spawn: func(world *ecs.World) ecs.Entity {
			return SpawnButton(world, ButtonOptions{Width: 100, Height: 32, Style: &button})
		}},
		{name: "button group", spawn: func(world *ecs.World) ecs.Entity {
			style := ButtonGroupStyle{Button: button, Divider: surface}
			return SpawnButtonGroup(world, ButtonGroupOptions[string]{Height: 32, Items: []ButtonGroupItem[string]{{Value: "a", Width: 50}}, Style: &style})
		}, target: func(world *ecs.World, entity ecs.Entity) ecs.Entity {
			group, _ := world.Get[ButtonGroup](entity)
			return group.Buttons[0]
		}},
		{name: "input", spawn: func(world *ecs.World) ecs.Entity {
			style := TextFieldStyle{Body: state, Caret: surface, Selection: surface}
			return SpawnInput(world, TextOptions{Width: 100, Height: 32, Style: &style})
		}},
		{name: "textarea", spawn: func(world *ecs.World) ecs.Entity {
			style := TextFieldStyle{Body: state, Caret: surface, Selection: surface}
			return SpawnTextarea(world, TextOptions{Width: 100, Height: 64, Style: &style})
		}},
		{name: "number input", spawn: func(world *ecs.World) ecs.Entity {
			style := NumberInputStyle{Field: TextFieldStyle{Body: state}, Step: state}
			return SpawnNumberInput(world, NumberInputOptions{Width: 100, Height: 32, Style: &style})
		}},
		{name: "menu", spawn: func(world *ecs.World) ecs.Entity {
			style := MenuStyle{Trigger: button, Popup: surface, Item: state, Indicator: TextIcon("v")}
			return SpawnMenu(world, MenuOptions{Width: 100, Height: 32, Items: []string{"A"}, Style: &style})
		}},
		{name: "context menu", spawn: func(world *ecs.World) ecs.Entity {
			style := MenuStyle{Popup: surface, Item: state}
			return SpawnContextMenu(world, ContextMenuOptions{Width: 100, ItemHeight: 32, Items: []string{"A"}, Open: true, Style: &style})
		}, target: func(world *ecs.World, entity ecs.Entity) ecs.Entity {
			return findSurfacePart[ContextMenuPanelPart](world, entity)
		}},
		{name: "checkbox", spawn: func(world *ecs.World) ecs.Entity {
			style := CheckboxStyle{Track: ToggleStyle[SurfaceStyle]{Off: state, On: state}, Mark: state}
			return SpawnCheckbox(world, CheckboxOptions{Width: 100, Height: 32, Style: &style})
		}, target: func(world *ecs.World, entity ecs.Entity) ecs.Entity {
			return findSurfacePart[TrackPart](world, entity)
		}},
		{name: "selector", spawn: func(world *ecs.World) ecs.Entity {
			style := SelectorStyle{Body: state, Popup: surface, Item: state}
			return SpawnSelector(world, SelectorOptions{Width: 100, Height: 32, Options: []string{"A"}, ListHeight: 32, Style: &style})
		}},
		{name: "tab", spawn: func(world *ecs.World) ecs.Entity {
			style := TabStyle{Frame: surface, Item: ToggleStyle[SurfaceStyle]{Off: state, On: state}, Panel: surface}
			return SpawnTab(world, TabOptions{Width: 100, Height: 64, Items: tabItems("A"), Style: &style})
		}},
		{name: "progress", spawn: func(world *ecs.World) ecs.Entity {
			style := ProgressStyle{Track: state, Fill: surface, Thumb: state}
			return SpawnProgress(world, ProgressOptions{Width: 100, Height: 12, Max: 1, Style: &style})
		}},
		{name: "scroll", spawn: func(world *ecs.World) ecs.Entity {
			style := ScrollStyle{Track: surface, Thumb: state}
			return SpawnScrollArea(world, ScrollAreaOptions{Width: 100, Height: 60, ContentWidth: 200, ContentHeight: 120, Style: &style})
		}, target: func(world *ecs.World, entity ecs.Entity) ecs.Entity {
			return findSurfacePart[ScrollTrackHorizontalPart](world, entity)
		}},
		{name: "tree", tree: true, spawn: func(world *ecs.World) ecs.Entity {
			style := TreeStyle{Background: surface, Row: ToggleStyle[SurfaceStyle]{Off: state, On: state}, Collapsed: TextIcon(">"), Expanded: TextIcon("v"), Leaf: TextIcon("-")}
			return SpawnTree(world, TreeOptions[string]{Width: 100, RowHeight: 32, Nodes: []TreeNodeSpec[string]{{ID: "root", Label: "Root"}}, Style: &style})
		}},
		{name: "image grid", spawn: func(world *ecs.World) ecs.Entity {
			style := ImageGridStyle{Cell: surface, Remove: state}
			return SpawnImageGrid(world, ImageGridOptions{Width: 100, Columns: 1, Items: []ImageGridItem{{Label: "A"}}, Style: &style})
		}, target: func(world *ecs.World, entity ecs.Entity) ecs.Entity {
			var cellEntity ecs.Entity
			for candidate, access := range world.Query[imageGridCell]().All {
				cell := access.Get()

				if cell.Grid == entity {
					cellEntity = candidate
					break
				}
			}
			return cellEntity
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := newUIWorld(t)
			entity := test.spawn(world)
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if test.tree {
				if err := SyncTreeSystem[string]("").Run(world); err != nil {
					t.Fatal(err)
				}
			}
			if test.target != nil {
				entity = test.target(world, entity)
			}
			assertNineSliceSurface(t, world, entity, texture, slice)
		})
	}
}

func TestWidgetStateTexturesSurviveRepeatedSync(t *testing.T) {
	world := newUIWorld(t)
	normal := graphics.CreateTextureByColor(graphics.ColorFromRGBA(1, 0, 0, 255))
	hovered := graphics.CreateTextureByColor(graphics.ColorFromRGBA(2, 0, 0, 255))
	pressed := graphics.CreateTextureByColor(graphics.ColorFromRGBA(3, 0, 0, 255))
	disabled := graphics.CreateTextureByColor(graphics.ColorFromRGBA(4, 0, 0, 255))
	state := NewStateStyle(TextureSurface(normal)).
		WithHovered(TextureSurface(hovered)).
		WithPressed(TextureSurface(pressed)).
		WithDisabled(TextureSurface(disabled))
	style := ButtonStyle{Body: state, JoinedEdge: state}
	button := SpawnButton(world, ButtonOptions{Width: 100, Height: 32, Style: &style})
	pointer := world.GetMut[scene.InputState2D](button)

	assertSyncTexture := func(want *graphics.Texture) {
		t.Helper()
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
		}
		image, _ := world.Get[scene.UIImage2D](styledSurfaceEntity(world, button))
		if image.Texture != want {
			t.Fatalf("button texture = %p, want %p", image.Texture, want)
		}
	}

	assertSyncTexture(normal)
	pointer.Hovered = true
	assertSyncTexture(hovered)
	pointer.Pressed = true
	assertSyncTexture(pressed)
	SetControlDisabled(world, button, true)
	assertSyncTexture(disabled)
}

func TestToggleTexturesCoverSelectedCheckedAndFocusedStates(t *testing.T) {
	world := newUIWorld(t)
	off := graphics.CreateTextureByColor(graphics.ColorFromRGBA(10, 0, 0, 255))
	on := graphics.CreateTextureByColor(graphics.ColorFromRGBA(11, 0, 0, 255))
	focused := graphics.CreateTextureByColor(graphics.ColorFromRGBA(12, 0, 0, 255))
	offState, onState := NewStateStyle(TextureSurface(off)), NewStateStyle(TextureSurface(on))
	tabStyle := TabStyle{Frame: ColorSurface(graphics.ColorRGBA{}), Item: ToggleStyle[SurfaceStyle]{Off: offState, On: onState}}
	tabs := SpawnTab(world, TabOptions{Width: 100, Height: 40, Items: tabItems("A", "B"), Style: &tabStyle})
	checkboxStyle := CheckboxStyle{Track: ToggleStyle[SurfaceStyle]{Off: offState, On: onState}, Mark: onState}
	checkbox := SpawnCheckbox(world, CheckboxOptions{Width: 100, Height: 32, Style: &checkboxStyle})
	fieldStyle := TextFieldStyle{Body: NewStateStyle(TextureSurface(off)).WithFocused(TextureSurface(focused))}
	field := SpawnInput(world, TextOptions{Width: 100, Height: 32, Style: &fieldStyle})
	fieldComponent := world.GetMut[TextField](field)
	fieldComponent.Focused = true
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	var selectedItem ecs.Entity
	for entity, access := range world.Query[TabItem]().All {
		item := access.Get()

		if item.Tabs == tabs && item.Index == 0 {
			selectedItem = entity
			break
		}
	}
	assertImageTexture(t, world, selectedItem, on)
	assertImageTexture(t, world, findSurfacePart[TrackPart](world, checkbox), off)
	assertImageTexture(t, world, field, focused)

	checkboxComponent := world.GetMut[Toggle[bool]](checkbox)
	checkboxComponent.Value = true
	tab := world.GetMut[Tab](tabs)
	tab.Selected = 1
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	assertImageTexture(t, world, selectedItem, off)
	assertImageTexture(t, world, findSurfacePart[TrackPart](world, checkbox), on)
}

func TestGlobalSkinAndPerEntityStyleOverride(t *testing.T) {
	world := newUIWorld(t)
	theme := DefaultTheme()
	globalTexture := graphics.CreateTextureByColor(graphics.ColorFromRGBA(20, 0, 0, 255))
	overrideTexture := graphics.CreateTextureByColor(graphics.ColorFromRGBA(21, 0, 0, 255))
	skin := DefaultSkin(theme)
	globalState := NewStateStyle(TextureSurface(globalTexture))
	skin.Button.Primary = ButtonStyle{Body: globalState, JoinedEdge: globalState}
	skin.ButtonGroup.Button = ButtonStyle{Body: globalState, JoinedEdge: globalState}
	skin.Menu.Trigger = ButtonStyle{Body: globalState, JoinedEdge: globalState}
	world.InsertResource(skin)

	globalButton := SpawnButton(world, ButtonOptions{Width: 100, Height: 32})
	groupEntity := SpawnButtonGroup(world, ButtonGroupOptions[string]{Height: 32, Items: []ButtonGroupItem[string]{{Value: "a", Width: 50}}})
	group, _ := world.Get[ButtonGroup](groupEntity)
	menu := SpawnMenu(world, MenuOptions{Width: 100, Height: 32, Items: []string{"A"}})
	overrideState := NewStateStyle(TextureSurface(overrideTexture))
	overrideStyle := ButtonStyle{Body: overrideState, JoinedEdge: overrideState}
	overrideButton := SpawnButton(world, ButtonOptions{Width: 100, Height: 32, Style: &overrideStyle})
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	assertImageTexture(t, world, globalButton, globalTexture)
	assertImageTexture(t, world, group.Buttons[0], globalTexture)
	assertImageTexture(t, world, menu, globalTexture)
	assertImageTexture(t, world, overrideButton, overrideTexture)
}

func TestIconSwitchesBetweenFallbackAndTextureAtRuntime(t *testing.T) {
	world := newUIWorld(t)
	texture := graphics.CreateTextureByColor(graphics.White)
	button := SpawnButton(world, ButtonOptions{
		Width: 100, Height: 32, Label: "Run",
		Icon: IconStyle{Fallback: ">", Width: 16, Height: 16},
	})
	assertIcon := func(wantText string, wantTexture *graphics.Texture) {
		t.Helper()
		for range 2 {
			if err := SyncSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
		}
		if got := iconTextValue(world, button); got != wantText {
			t.Fatalf("icon fallback = %q, want %q", got, wantText)
		}
		iconEntity := findSurfacePart[IconPart](world, button)
		assertImageTexture(t, world, iconEntity, wantTexture)
	}

	assertIcon(">", nil)
	component := world.GetMut[IconContent](button)
	component.Value.Texture = texture
	assertIcon("", texture)
	component.Value.Texture = nil
	assertIcon(">", nil)
}

func TestTreeIndicatorTexturesFollowNodeState(t *testing.T) {
	world := newUIWorld(t)
	collapsed := graphics.CreateTextureByColor(graphics.ColorFromRGBA(30, 0, 0, 255))
	expanded := graphics.CreateTextureByColor(graphics.ColorFromRGBA(31, 0, 0, 255))
	leaf := graphics.CreateTextureByColor(graphics.ColorFromRGBA(32, 0, 0, 255))
	style := DefaultSkin(DefaultTheme()).Tree
	style.Collapsed = TextureIcon(collapsed, 12, 12)
	style.Expanded = TextureIcon(expanded, 12, 12)
	style.Leaf = TextureIcon(leaf, 12, 12)
	tree := SpawnTree(world, TreeOptions[string]{
		Width: 120, RowHeight: 32, Style: &style,
		Nodes: []TreeNodeSpec[string]{{ID: "root", Label: "Root"}, {ID: "leaf", Label: "Leaf", Leaf: true}},
	})
	if err := SyncTreeSystem[string]("").Run(world); err != nil {
		t.Fatal(err)
	}
	rootNode := findTreeNode(world, tree, "root")
	leafNode := findTreeNode(world, tree, "leaf")
	assertTreeIndicatorTexture(t, world, rootNode, collapsed)
	assertTreeIndicatorTexture(t, world, leafNode, leaf)

	node := world.GetMut[TreeNode[string]](rootNode)
	node.Expanded = true
	for range 2 {
		if err := SyncTreeSystem[string]("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	assertTreeIndicatorTexture(t, world, rootNode, expanded)
}

func assertNineSliceSurface(t *testing.T, world *ecs.World, entity ecs.Entity, texture *graphics.Texture, slice scene.NineSlice2D) {
	t.Helper()
	entity = styledSurfaceEntity(world, entity)
	if entity == 0 {
		t.Fatal("missing styled surface entity")
	}
	image, ok := world.Get[scene.UIImage2D](entity)
	if !ok || image.Texture != texture || image.Mode != scene.UIImageSliced || image.Slice == nil || *image.Slice != slice {
		t.Fatalf("nine-slice surface = %#v", image)
	}
}

func assertImageTexture(t *testing.T, world *ecs.World, entity ecs.Entity, texture *graphics.Texture) {
	t.Helper()
	entity = styledSurfaceEntity(world, entity)
	image, ok := world.Get[scene.UIImage2D](entity)
	if !ok || image.Texture != texture {
		t.Fatalf("entity %d image = %#v, want texture %p", entity, image, texture)
	}
}

func styledSurfaceEntity(world *ecs.World, entity ecs.Entity) ecs.Entity {
	if world.Has[scene.UIImage2D](entity) {
		return entity
	}
	return findSurfacePart[BodyPart](world, entity)
}

func findSurfacePart[P any](world *ecs.World, owner ecs.Entity) ecs.Entity {
	entity, _ := FindPart[P, SurfaceStyle](world, owner)
	return entity
}

func iconTextValue(world *ecs.World, owner ecs.Entity) string {
	value := ""
	for entity, access := range world.Query2[Relation[scene.Text2D], scene.Text2D]().All {
		visual := access.A.Ref()
		text := access.B.Ref()

		if visual.Owner == owner && world.Has[Part[IconPart]](entity) {
			value = text.Value
			break
		}
	}
	return value
}

func findTreeNode(world *ecs.World, tree ecs.Entity, id string) ecs.Entity {
	var result ecs.Entity
	for entity, access := range world.Query[TreeNode[string]]().All {
		node := access.Get()

		if node.Tree == tree && node.ID == id {
			result = entity
			break
		}
	}
	return result
}

func assertTreeIndicatorTexture(t *testing.T, world *ecs.World, owner ecs.Entity, texture *graphics.Texture) {
	t.Helper()
	found := false
	for entity, access := range world.Query[Relation[TreeNode[string]]]().All {
		element := access.Get()

		if element.Owner == owner && world.Has[Part[TreeIndicatorImagePart]](entity) {
			assertImageTexture(t, world, entity, texture)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing tree indicator image")
	}
}
