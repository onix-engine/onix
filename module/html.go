package module

import (
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

// Rect is an element's border box in its HTML surface's coordinates.
type Rect struct{ X, Y, Width, Height float32 }

// HandleHTMLEvent gives semantic form/control input to the owning scene.
// Surface and generated target identities never enter the behavior callback.
func (r *Runtime) HandleHTMLEvent(event htmlui.Event) error {
	if r.closed {
		return ErrDisposed
	}
	if r.busy {
		return ErrBusy
	}
	owner, ok := r.world.Get[entityOwner](event.Entity)
	if !ok || !owner.context.running() || !owner.context.enabled || owner.context.scene.state != SceneActive || !scene.IsInteractive(r.world, event.Entity) {
		return nil
	}
	c := owner.context
	return r.deliverInput(c, Input{Target: Entity{c, event.Entity}, HTMLID: event.ID, HTMLKind: event.Kind, Value: event.Value})
}

// HTMLNode permits document edits inside an owned surface without exposing a
// World, ECS identity or callbacks with access to foreign input targets.
type HTMLNode struct {
	context  *Context
	surface  Entity
	node     htmlui.Node
	document htmlui.Node
	source   string
}

func HTML(c *Context, surface Entity) (HTMLNode, error) {
	if !c.usable() {
		return HTMLNode{}, ErrDisposed
	}
	if !c.owns(surface) {
		return HTMLNode{}, ErrOwnership
	}
	node, err := htmlui.Root(c.runtime.world, surface.entity)
	value, _ := c.Get[htmlui.Surface](surface)
	return HTMLNode{context: c, surface: surface, node: node, document: node, source: value.Value}, err
}

func (n HTMLNode) valid() bool {
	if !n.context.usable() {
		return false
	}
	value, ok := n.context.Get[htmlui.Surface](n.surface)
	if !ok || value.Value != n.source {
		return false
	}
	root, err := htmlui.Root(n.context.runtime.world, n.surface.entity)
	return err == nil && root == n.document
}

func (n HTMLNode) FindByID(id string) (HTMLNode, bool) {
	if !n.valid() {
		return HTMLNode{}, false
	}
	node, ok := n.node.FindByID(id)
	return HTMLNode{context: n.context, surface: n.surface, node: node, document: n.document, source: n.source}, ok
}

func (n HTMLNode) SetText(value string) bool { return n.valid() && n.node.SetText(value) }
func (n HTMLNode) SetAttribute(name, value string) bool {
	return n.valid() && n.node.SetAttribute(name, value)
}
func (n HTMLNode) RemoveAttribute(name string) bool { return n.valid() && n.node.RemoveAttribute(name) }
func (n HTMLNode) SetHidden(hidden bool) bool       { return n.valid() && n.node.SetHidden(hidden) }
func (n HTMLNode) SetSelectedIndex(selected int) bool {
	return n.valid() && n.node.SetSelectedIndex(selected)
}
func (n HTMLNode) Clear() bool  { return n.valid() && n.node.Clear() }
func (n HTMLNode) Remove() bool { return n.valid() && n.node.Remove() }

// Bounds returns the node's last synchronized layout border box in the HTML
// surface's coordinates. Hidden or detached nodes have no box; runtime scroll
// and animation offsets are not included.
func (n HTMLNode) Bounds() (Rect, bool) {
	if !n.valid() {
		return Rect{}, false
	}
	rect, ok := n.node.Bounds()
	return Rect{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height}, ok
}
func (n HTMLNode) Clone() HTMLNode {
	if !n.valid() {
		return HTMLNode{}
	}
	return HTMLNode{context: n.context, surface: n.surface, node: n.node.Clone(), document: n.document, source: n.source}
}
func (n HTMLNode) Append(child HTMLNode) bool {
	return n.valid() && child.valid() && n.context == child.context && n.surface == child.surface && n.node.Append(child.node)
}
