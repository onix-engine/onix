package html

import (
	"fmt"
	"strings"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/element"
	"github.com/onix-engine/onix/scene"
	uinode "github.com/onix-engine/onix/scene/node"
)

// Node is a mutable node owned by an HTML surface. Mutations are rendered
// by the next HTML synchronization pass.
type Node struct {
	world *ecs.World
	node  *uinode.Node
	owner ecs.Entity
}

// Root returns the retained document root for an HTML surface.
func Root(world *ecs.World, owner ecs.Entity) (Node, error) {
	if world == nil {
		return Node{}, fmt.Errorf("ecs: html world is nil")
	}
	value, ok := world.Get[Surface](owner)
	if !ok {
		return Node{}, fmt.Errorf("ecs: entity %d has no HTML component", owner)
	}
	state := world.GetMut[htmlRenderState](owner)
	if state == nil {
		if err := world.Insert(owner, htmlRenderState{}); err != nil {
			return Node{}, err
		}
		state = world.GetMut[htmlRenderState](owner)
	}
	if !state.matchesSource(value) {
		if err := loadHTMLDocument(world, owner, state, &value); err != nil {
			return Node{}, fmt.Errorf("ecs: parse html: %w", err)
		}
	}
	return Node{world: world, owner: owner, node: state.Root}, nil
}

// NodeByID finds the first element with id within an HTML surface.
func NodeByID(world *ecs.World, owner ecs.Entity, id string) (Node, bool) {
	root, err := Root(world, owner)
	if err != nil {
		return Node{}, false
	}
	if state := world.GetMut[htmlRenderState](owner); state != nil {
		if found := lookupHTMLNode(state, id); found != nil {
			return Node{world: world, owner: owner, node: found}, true
		}
		return Node{}, false
	}
	return root.FindByID(id)
}

// FindByID finds the first descendant element with id.
func (n Node) FindByID(id string) (Node, bool) {
	var found *uinode.Node
	if n.world != nil {
		if state := n.world.GetMut[htmlRenderState](n.owner); state != nil && state.Root == n.node {
			found = lookupHTMLNode(state, id)
		} else {
			found = findHTMLNodeByID(n.node, id)
		}
	} else {
		found = findHTMLNodeByID(n.node, id)
	}
	return Node{world: n.world, owner: n.owner, node: found}, found != nil
}

// OnPointer binds a native pointer callback to this HTML node. The callback
// receives enter/leave/press/move/release/wheel/cancel and tap events.
// Passing nil removes the pointer callback.
func (n Node) OnPointer(handler PointerHandler) bool {
	if !n.valid() {
		return false
	}
	state := n.world.GetMut[htmlRenderState](n.owner)
	if state == nil {
		return false
	}
	binding := state.NodeEvents[n.node]
	binding.Pointer = handler
	setHTMLNodeEventBinding(n.world, n.owner, state, n.node, binding, true)
	return true
}

// OnClick binds a native click callback to this HTML node. It runs for tap
// events after the pointer callback, when one is installed.
// Passing nil removes the click callback.
func (n Node) OnClick(handler ClickHandler) bool {
	if !n.valid() {
		return false
	}
	state := n.world.GetMut[htmlRenderState](n.owner)
	if state == nil {
		return false
	}
	binding := state.NodeEvents[n.node]
	binding.Click = handler
	setHTMLNodeEventBinding(n.world, n.owner, state, n.node, binding, true)
	return true
}

// Clone returns a detached deep copy owned by the same HTML surface.
func (n Node) Clone() Node {
	var arena uinode.Arena
	return Node{world: n.world, owner: n.owner, node: arena.Clone(n.node)}
}

// Append appends a detached node as the last child.
func (n Node) Append(child Node) bool {
	if !n.valid() || !child.valid() || n.world != child.world || n.owner != child.owner || child.node.Parent != nil || n.node.Within(child.node) {
		return false
	}
	kind := n.structureMutationKind(child.node)
	n.node.AppendChild(child.node)
	element.SynchronizeTextContent(n.node)
	n.markMutation(kind)
	return true
}

// Clear removes all child nodes.
func (n Node) Clear() bool {
	if !n.valid() {
		return false
	}
	if len(n.node.Children) == 0 {
		return true
	}
	kind := n.detachChildrenMutationKind()
	element.SynchronizeTextContent(n.node)
	n.markMutation(kind)
	return true
}

// Remove detaches the node from its parent.
func (n Node) Remove() bool {
	if !n.valid() || n.node.Parent == nil {
		return false
	}
	parent := n.node.Parent
	parentNode := Node{world: n.world, owner: n.owner, node: parent}
	kind := parentNode.structureMutationKind(n.node)
	if !parent.RemoveChild(n.node) {
		return false
	}
	element.SynchronizeTextContent(parent)
	parentNode.markMutation(kind)
	return true
}

// SetText replaces the node contents with one text node.
func (n Node) SetText(value string) bool {
	if !n.valid() {
		return false
	}
	if len(n.node.Children) == 1 {
		child := n.node.Children[0]
		if child != nil && child.Type == uinode.TextNode {
			if child.Data != value {
				child.Data = value
				element.SynchronizeTextContent(n.node)
				n.markMutation(htmlMutationText)
			}
			return true
		}
	}
	kind := n.detachChildrenMutationKind()
	n.node.AppendChild(uinode.NewText(value))
	element.SynchronizeTextContent(n.node)
	n.markMutation(kind)
	return true
}

// Attribute returns an element's current authored attribute without serializing it.
func (n Node) Attribute(name string) (string, bool) {
	if !n.valid() || n.node.Type != uinode.ElementNode {
		return "", false
	}
	return n.node.Attr(name)
}

// SetAttribute sets an element attribute.
func (n Node) SetAttribute(name, value string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !n.valid() || n.node.Type != uinode.ElementNode || name == "" {
		return false
	}
	if !n.node.SetAttr(name, value) {
		return true
	}
	n.syncAttributeState(name, true)
	n.markAttributeDirty(name)
	return true
}

// RemoveAttribute removes an element attribute.
func (n Node) RemoveAttribute(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !n.valid() || n.node.Type != uinode.ElementNode {
		return false
	}
	if !n.node.RemoveAttr(name) {
		return false
	}
	n.syncAttributeState(name, false)
	n.markAttributeDirty(name)
	return true
}

func (n Node) syncAttributeState(name string, present bool) {
	switch name {
	case "checked":
		element.SynchronizeCheckedAttribute(n.node, present)
	case "selected":
		element.SynchronizeSelectedAttribute(n.node, present)
	case "value":
		element.SynchronizeValueAttribute(n.node)
	case "placeholder":
		element.SynchronizePlaceholderAttribute(n.node)
	case "type":
		element.SynchronizeValueAttribute(n.node)
		element.SynchronizePlaceholderAttribute(n.node)
	case "multiple", "size":
		element.SynchronizeSelectAttribute(n.node, name)
	}
}

// SetHidden controls the standard hidden attribute.
func (n Node) SetHidden(hidden bool) bool {
	if hidden {
		return n.SetAttribute("hidden", "")
	}
	if !n.valid() || n.node.Type != uinode.ElementNode {
		return false
	}
	if n.node.RemoveAttr("hidden") {
		n.markAttributeDirty("hidden")
	}
	return true
}

// SetSelectedIndex writes the live selection, preserving authored defaults.
// It also works before the first render or while option text is being updated.
func (n Node) SetSelectedIndex(selected int) bool {
	if !n.valid() || n.node.Type != uinode.ElementNode || n.node.Data != "select" || selected < -1 {
		return false
	}
	state := n.world.GetMut[htmlRenderState](n.owner)
	if state == nil {
		return false
	}
	element.SetSelectedIndex(n.node, selected)
	key := htmlRenderIdentity{Node: n.node, Role: htmlRoleControl, Variant: inputType(n.node)}
	if group := state.EntityGroups[key]; group != nil {
		for _, binding := range group.Bindings {
			if control := n.world.GetMut[Control](binding.Entity); control != nil && control.Owner == n.owner {
				syncHTMLSelectOptionStates(control)
			}
		}
	}
	n.markMutation(htmlMutationContent)
	return true
}

func (n Node) valid() bool {
	return n.world != nil && n.owner != 0 && n.node != nil
}

func findHTMLNodeByID(node *uinode.Node, id string) *uinode.Node {
	if node == nil {
		return nil
	}
	if value, ok := node.Attr("id"); ok && value == id {
		return node
	}
	for _, child := range node.Children {
		if found := findHTMLNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// Rect is an element's CSS border box in its HTML surface's coordinates.
type Rect struct{ X, Y, Width, Height float32 }

// Bounds returns the last synchronized layout border box. Hidden or detached
// nodes have no box. Native window hit testing can use this without duplicating
// CSS layout. Surface transforms and runtime scroll/animation offsets are not
// included.
func (n Node) Bounds() (Rect, bool) {
	if !n.valid() {
		return Rect{}, false
	}
	state := n.world.GetRef[htmlRenderState](n.owner)
	if state == nil || !state.Rendered {
		return Rect{}, false
	}
	box := state.LayoutCache.BoxForNode(n.node)
	if box == nil {
		return Rect{}, false
	}
	r := box.BorderBox()
	return Rect{float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height)}, r.Width > 0 && r.Height > 0
}

// ScrollContainer returns the node's synchronized overflow scroll entity,
// including when its content is outside the visible paint area.
func (n Node) ScrollContainer() (ecs.Entity, bool) {
	if !n.valid() {
		return 0, false
	}
	state := n.world.GetRef[htmlRenderState](n.owner)
	if state == nil {
		return 0, false
	}
	binding := state.OverflowBindings[n.node]
	if binding == nil {
		return 0, false
	}
	entity := binding.entity
	_, ok := n.world.Get[scene.UIScroll](entity)
	return entity, ok
}
