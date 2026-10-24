package module

import (
	"iter"
	"reflect"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/text"
	ui "github.com/diiyw/onix/ui"
	html "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

// ContentMessages filters engine control messages to their owning module.
type ContentMessages[T any] struct{ reader ecs.MessageReader[T] }

func (r *ContentMessages[T]) All(c *Content) iter.Seq[*T] {
	return func(yield func(*T) bool) {
		for value := range r.reader.All(c.context.runtime.world) {
			var e ecs.Entity
			switch v := any(*value).(type) {
			case scene.InputEvent:
				e = v.Entity
			case html.Event:
				e = v.Entity
			case ui.InputEvent[string]:
				e = v.Entity
			case ui.WindowTitleBarEvent:
				e = v.Bar
			}
			if c.owns(e) && !yield(value) {
				return
			}
		}
	}
}

type ContentSystemContext struct{ World *Content }

func ContentHas[T any](c *Content, e ecs.Entity) bool { _, ok := ContentGet[T](c, e); return ok }
func ContentRef[T any](c *Content, e ecs.Entity) *T {
	if !c.owns(e) {
		return nil
	}
	return ecs.GetRef[T](c.context.runtime.world, e)
}
func ContentSetIfChanged[T comparable](c *Content, e ecs.Entity, v T) bool {
	if !c.owns(e) || contentProtected[T]() {
		return false
	}
	return ecs.SetIfChanged(c.context.runtime.world, e, v)
}
func ContentQuery2[A, B any](c *Content) iter.Seq2[ecs.Entity, ecs.Components2[A, B]] {
	return func(yield func(ecs.Entity, ecs.Components2[A, B]) bool) {
		if c == nil || !c.context.live || contentProtected[A]() || contentProtected[B]() || !c.queryAvailable() {
			return
		}
		boundary := c.beginQuery()
		defer c.endQuery(boundary)
		for e, a := range ecs.Query2[A, B](c.context.runtime.world).In(c.context.ownedEntities).All {
			if !yield(e, a) {
				return
			}
		}
	}
}

func (c *Content) ReleaseHTML(e ecs.Entity) {
	if c.owns(e) {
		html.Release(c.context.runtime.world, e)
	}
}
func (c *Content) SyncHTML() error {
	c.context.runtime.SyncScopes()
	return ui.SyncHTML(c.context.runtime.world)
}
func (c *Content) SyncUI() error {
	c.context.runtime.SyncScopes()
	return ui.SyncSystem("").Run(c.context.runtime.world)
}
func (c *Content) HTMLRoot(e ecs.Entity) (html.Node, error) {
	if !c.owns(e) {
		return html.Node{}, ErrOwnership
	}
	return html.Root(c.context.runtime.world, e)
}
func (c *Content) HTMLNode(e ecs.Entity, id string) (html.Node, bool) {
	if !c.owns(e) {
		return html.Node{}, false
	}
	return html.NodeByID(c.context.runtime.world, e, id)
}
func (c *Content) HTMLControl(e ecs.Entity, id string) (ecs.Entity, bool) {
	if !c.owns(e) {
		return 0, false
	}
	return html.ControlByID(c.context.runtime.world, e, id)
}
func (c *Content) HTMLSet(e ecs.Entity, source string) {
	if c.owns(e) {
		html.Set(c.context.runtime.world, e, source)
	}
}

func (c *Content) HTMLSetCSS(e ecs.Entity, cssText string) bool {
	return c.owns(e) && html.SetBaseCSS(c.context.runtime.world, e, cssText)
}
func (c *Content) HTMLSetOffset(e ecs.Entity, id string, x, y float32) bool {
	return c.owns(e) && html.SetNodeOffset(c.context.runtime.world, e, id, x, y)
}
func (c *Content) HTMLSetScroll(e ecs.Entity, id string, area ecs.Entity, x, y float32) bool {
	return c.owns(e) && c.owns(area) && html.SetNodeScroll(c.context.runtime.world, e, id, area, x, y)
}
func (c *Content) HTMLScrollStyle(e ecs.Entity, id string) (html.ScrollStyle, bool) {
	if !c.owns(e) {
		return html.ScrollStyle{}, false
	}
	return html.NodeScrollStyle(c.context.runtime.world, e, id)
}
func (c *Content) Focus(e ecs.Entity) bool {
	return c.owns(e) && ui.FocusControl(c.context.runtime.world, e)
}

// FocusedControl returns this module's keyboard focus, or zero if focus is outside.
func (c *Content) FocusedControl() ecs.Entity {
	if c == nil || !c.context.live {
		return 0
	}
	focus, ok := ecs.GetResource[*scene.InputFocus](c.context.runtime.world)
	if !ok || focus == nil || !c.owns(focus.Keyboard) {
		return 0
	}
	return focus.Keyboard
}

func (c *Content) SetControlValue(e ecs.Entity, v string) bool {
	return c.owns(e) && ui.SetHTMLControlValue(c.context.runtime.world, e, v)
}
func (c *Content) SetDisabled(e ecs.Entity, v bool) {
	if c.owns(e) {
		ui.SetControlDisabled(c.context.runtime.world, e, v)
	}
}
func (c *Content) SetSelected(e ecs.Entity, v bool) {
	if c.owns(e) {
		ui.SetControlSelected(c.context.runtime.world, e, v)
	}
}
func (c *Content) ScrollOffset(e ecs.Entity) (mgl32.Vec2, bool) {
	if !c.owns(e) {
		return mgl32.Vec2{}, false
	}
	return ui.GetScrollAreaOffset(c.context.runtime.world, e)
}
func (c *Content) SetScrollOffset(e ecs.Entity, v mgl32.Vec2) bool {
	return c.owns(e) && ui.SetScrollAreaOffset(c.context.runtime.world, e, v)
}
func ContentFindPart[P, T any](c *Content, e ecs.Entity) (ecs.Entity, bool) {
	if !c.owns(e) {
		return 0, false
	}
	return ui.FindPart[P, T](c.context.runtime.world, e)
}
func ContentSpawnNode[T any](c *Content, o ui.NodeOptions, v T) ecs.Entity {
	checkNodeComponent[T]()
	if !c.context.usable() {
		panic(ErrDisposed)
	}
	if o.Parent == 0 {
		o.Parent = c.Root()
	}
	if !c.owns(o.Parent) {
		panic(ErrOwnership)
	}
	if c.context.runtime.busy {
		entity := c.queueNode(o, 1)
		ecs.CommandInsert(c.context.runtime.world.Commands(), entity, v)
		return entity
	}
	return c.adopt(ui.SpawnNode(c.context.runtime.world, o, v))
}
func ContentSpawnNode2[A, B any](c *Content, o ui.NodeOptions, a A, b B) ecs.Entity {
	checkNodeComponent[A]()
	checkNodeComponent[B]()
	if reflect.TypeFor[A]() == reflect.TypeFor[B]() {
		panic(ErrInvalid)
	}
	if !c.context.usable() {
		panic(ErrDisposed)
	}
	if o.Parent == 0 {
		o.Parent = c.Root()
	}
	if !c.owns(o.Parent) {
		panic(ErrOwnership)
	}
	if c.context.runtime.busy {
		entity := c.queueNode(o, 2)
		ecs.CommandInsert(c.context.runtime.world.Commands(), entity, a)
		ecs.CommandInsert(c.context.runtime.world.Commands(), entity, b)
		return entity
	}
	return c.adopt(ui.SpawnNode2(c.context.runtime.world, o, a, b))
}

func checkNodeComponent[T any]() {
	if contentProtected[T]() {
		panic(ErrOwnership)
	}
	switch reflect.TypeFor[T]() {
	case reflect.TypeFor[scene.UITransform](), reflect.TypeFor[scene.UIGlobalTransform](), reflect.TypeFor[scene.Anchor](), reflect.TypeFor[scene.Visibility](), reflect.TypeFor[scene.UIStackIndex](), reflect.TypeFor[scene.Activity]():
		panic(ErrInvalid)
	}
}

// One prepared base and typed command streams avoid rebuilding boxed bundles
// for nodes created by Update/message/query callbacks.
func (c *Content) queueNode(o ui.NodeOptions, additional int) ecs.Entity {
	r := c.context.runtime
	count := 6 + additional
	if len(r.commands)+r.privateCommands+count > r.options.CommandCapacity {
		panic(ErrCommandFull)
	}
	if !c.context.nodePrepared {
		bundle := ecs.NewBundle(8)
		ecs.Put(&bundle, scene.UITransformFromXY(0, 0))
		ecs.Put(&bundle, scene.UIGlobalTransform{})
		ecs.Put(&bundle, scene.Parent{Entity: c.context.root})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Visibility{Visible: true})
		ecs.Put(&bundle, scene.UIStackIndex(0))
		ecs.Put(&bundle, explicitPainterOrder{})
		ecs.Put(&bundle, entityOwner{context: c.context})
		ecs.Put(&bundle, scene.Activity{Active: c.context.enabled})
		c.context.nodeBundle = r.world.PrepareBundle(bundle)
		c.context.nodePrepared = true
	}
	r.privateCommands += count
	commands := r.world.Commands()
	entity := ecs.CommandSpawnPrepared(commands, c.context.nodeBundle)
	anchor := scene.AnchorTopLeft
	if o.Anchor != nil {
		anchor = *o.Anchor
	}
	ecs.CommandInsert(commands, entity, scene.UITransformFromXY(o.X, o.Y))
	ecs.CommandInsert(commands, entity, scene.Parent{Entity: o.Parent})
	ecs.CommandInsert(commands, entity, anchor)
	ecs.CommandInsert(commands, entity, scene.Visibility{Visible: !o.Hidden})
	ecs.CommandInsert(commands, entity, scene.UIStackIndex(o.StackIndex))
	r.privateSpawns[entity] = privateSpawn{owner: c.context, parent: o.Parent}
	c.context.entities = append(c.context.entities, entity)
	c.context.dirtyDrawOrder()
	return entity
}
func ContentBindControl[T any](c *Content, id string, v T) error {
	e, err := c.Element(id)
	if err != nil {
		return err
	}
	size, _ := ContentGet[scene.Size2D](c, e)
	t, _ := ContentGet[scene.UITransform](c, e)
	o := ui.ControlOptions{X: t.Translation.X.Value, Y: t.Translation.Y.Value, Width: size.Width, Height: size.Height}
	if err := ui.BindControl(c.context.runtime.world, e, o, v); err != nil {
		return err
	}
	composition := ui.ControlComposition{}
	for _, child := range c.context.entities {
		inside := false
		for ancestor := child; ancestor != 0; {
			if ancestor == e {
				inside = true
				break
			}
			p, ok := ecs.Get[scene.Parent](c.context.runtime.world, ancestor)
			if !ok {
				break
			}
			ancestor = p.Entity
		}
		if !inside {
			continue
		}
		kind := ui.CompositionGroup
		size, _ := ContentGet[scene.Size2D](c, child)
		t, _ := ContentGet[scene.UITransform](c, child)
		label := ""
		if sprite, ok := ContentGet[scene.Sprite2D](c, child); ok {
			kind = ui.CompositionSprite
			size.Width, size.Height = sprite.Width, sprite.Height
		}
		if value, ok := ContentGet[scene.Text2D](c, child); ok {
			kind = ui.CompositionText
			label = value.Value
		}
		composition[child] = ui.CompositionNode{Kind: kind, X: t.Translation.X.Value, Y: t.Translation.Y.Value, Width: size.Width, Height: size.Height, Text: label, Visible: true}
	}
	return ui.BindControlComposition(c.context.runtime.world, composition, e, func(child ecs.Entity) error {
		return ui.BindControl(c.context.runtime.world, child, o, ui.Button{})
	})
}

func ContentMount[P, S any](c *Content, e ecs.Entity, d *html.Definition[P, S], p P, s S, key uint64) (ecs.Entity, error) {
	if !c.owns(e) {
		return 0, ErrOwnership
	}
	root, err := html.MountComponent(c.context.runtime.world, e, d, p, s, key)
	if err == nil {
		c.adopt(root)
	}
	return root, err
}
func ContentUpdateState[P, S any](c *Content, e ecs.Entity, update func(*S)) bool {
	return c.owns(e) && html.UpdateComponentState[P, S](c.context.runtime.world, e, update)
}
func (c *Content) Unmount(e ecs.Entity) bool {
	return c.owns(e) && html.UnmountComponent(c.context.runtime.world, e)
}

// ImageProvider and fonts are installed on owned surfaces rather than globally.
func (c *Content) SurfaceFonts(e ecs.Entity, font, bold *text.Font) {
	if surface := ContentMut[html.Surface](c, e); surface != nil {
		surface.Font, surface.BoldFont = font, bold
	}
}

func (c *Content) SpawnScrollArea(o ui.ScrollAreaOptions) ecs.Entity {
	if !c.context.usable() {
		panic(ErrDisposed)
	}
	return c.adopt(ui.SpawnScrollArea(c.context.runtime.world, o))
}

func (c *Content) ControlTextTabWidth(e ecs.Entity, text *scene.Text2D) (float32, bool) {
	if !c.owns(e) {
		return 0, false
	}
	return html.ControlTextTabWidth(c.context.runtime.world, e, text)
}
