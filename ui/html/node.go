package html

import (
	"fmt"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/scene"
)

// Node is a mutable node owned by an HTML surface. Mutations are rendered
// by the next HTML synchronization pass.
type Node struct {
	world *ecs.World
	owner ecs.Entity
	node  *dom.Node
}

// Root returns the retained document root for an HTML surface.
func Root(world *ecs.World, owner ecs.Entity) (Node, error) {
	if world == nil {
		return Node{}, fmt.Errorf("ecs: html world is nil")
	}
	value, ok := ecs.Get[Surface](world, owner)
	if !ok {
		return Node{}, fmt.Errorf("ecs: entity %d has no HTML component", owner)
	}
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil {
		if err := ecs.Insert(world, owner, htmlRenderState{}); err != nil {
			return Node{}, err
		}
		state = ecs.GetMut[htmlRenderState](world, owner)
	}
	if state.Root == nil || state.Source != value.Value {
		root, embeddedCSS, err := parseHTMLDocument(value.Value)
		if err != nil {
			return Node{}, fmt.Errorf("ecs: parse html: %w", err)
		}
		state.Source = value.Value
		state.EmbeddedCSS = embeddedCSS
		state.Root = root
		state.StyleCache.InvalidateDefinitions()
		syncHTMLDocumentIndex(state)
		state.Input = css.State{}
		clear(state.StateRoots[:])
		clear(state.NodeOffsets)
		state.NodeOffsets = state.NodeOffsets[:0]
		clear(state.NodeScrolls)
		state.NodeScrolls = state.NodeScrolls[:0]
		prepareHTMLNodeBindings(world, state)
		clear(state.NodeEvents)
		clear(state.ComponentEvents)
		clear(state.ControlSnapshots)
		state.Revision++
	}
	return Node{world: world, owner: owner, node: state.Root}, nil
}

// NodeByID finds the first element with id within an HTML surface.
func NodeByID(world *ecs.World, owner ecs.Entity, id string) (Node, bool) {
	root, err := Root(world, owner)
	if err != nil {
		return Node{}, false
	}
	if state := ecs.GetMut[htmlRenderState](world, owner); state != nil {
		if found := lookupHTMLNode(state, id); found != nil {
			return Node{world: world, owner: owner, node: found}, true
		}
		return Node{}, false
	}
	return root.FindByID(id)
}

// FindByID finds the first descendant element with id.
func (n Node) FindByID(id string) (Node, bool) {
	var found *dom.Node
	if n.world != nil {
		if state := ecs.GetMut[htmlRenderState](n.world, n.owner); state != nil && state.Root == n.node {
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
	state := ecs.GetMut[htmlRenderState](n.world, n.owner)
	if state == nil {
		return false
	}
	if state.NodeEvents == nil {
		state.NodeEvents = make(map[*dom.Node]htmlNodeEventBinding)
	}
	binding := state.NodeEvents[n.node]
	binding.Pointer = handler
	if binding.Pointer == nil && binding.Click == nil {
		delete(state.NodeEvents, n.node)
	} else {
		state.NodeEvents[n.node] = binding
	}
	state.EventRevision++
	n.markEventMutation(state)
	return true
}

// OnClick binds a native click callback to this HTML node. It runs for tap
// events after the pointer callback, when one is installed.
// Passing nil removes the click callback.
func (n Node) OnClick(handler ClickHandler) bool {
	if !n.valid() {
		return false
	}
	state := ecs.GetMut[htmlRenderState](n.world, n.owner)
	if state == nil {
		return false
	}
	if state.NodeEvents == nil {
		state.NodeEvents = make(map[*dom.Node]htmlNodeEventBinding)
	}
	binding := state.NodeEvents[n.node]
	binding.Click = handler
	if binding.Pointer == nil && binding.Click == nil {
		delete(state.NodeEvents, n.node)
	} else {
		state.NodeEvents[n.node] = binding
	}
	state.EventRevision++
	n.markEventMutation(state)
	return true
}

// Clone returns a detached deep copy owned by the same HTML surface.
func (n Node) Clone() Node {
	return Node{world: n.world, owner: n.owner, node: cloneHTMLNode(n.node, nil)}
}

// Append appends a detached node as the last child.
func (n Node) Append(child Node) bool {
	if !n.valid() || !child.valid() || n.world != child.world || n.owner != child.owner || child.node.Parent != nil {
		return false
	}
	kind := n.structureMutationKind(child.node)
	n.node.AppendChild(child.node)
	dom.SynchronizeTextContent(n.node)
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
	dom.SynchronizeTextContent(n.node)
	n.markMutation(kind)
	return true
}

// Remove detaches the node from its parent.
func (n Node) Remove() bool {
	if !n.valid() || n.node.Parent == nil {
		return false
	}
	parent := n.node.Parent
	for index, child := range parent.Children {
		if child != n.node {
			continue
		}
		parentNode := Node{world: n.world, owner: n.owner, node: parent}
		kind := parentNode.structureMutationKind(n.node)
		copy(parent.Children[index:], parent.Children[index+1:])
		parent.Children[len(parent.Children)-1] = nil
		parent.Children = parent.Children[:len(parent.Children)-1]
		n.node.Parent = nil
		dom.SynchronizeTextContent(parent)
		parentNode.markMutation(kind)
		return true
	}
	return false
}

// SetText replaces the node contents with one text node.
func (n Node) SetText(value string) bool {
	if !n.valid() {
		return false
	}
	if len(n.node.Children) == 1 {
		child := n.node.Children[0]
		if child != nil && child.Type == dom.TextNode {
			if child.Data != value {
				child.Data = value
				dom.SynchronizeTextContent(n.node)
				n.markMutation(htmlMutationText)
			}
			return true
		}
	}
	kind := n.detachChildrenMutationKind()
	n.node.AppendChild(dom.NewText(value))
	dom.SynchronizeTextContent(n.node)
	n.markMutation(kind)
	return true
}

// SetAttribute sets an element attribute.
func (n Node) SetAttribute(name, value string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !n.valid() || n.node.Type != dom.ElementNode || name == "" {
		return false
	}
	if n.node.Attributes == nil {
		n.node.Attributes = make(map[string]string)
	} else if current, ok := n.node.Attributes[name]; ok && current == value {
		return true
	}
	n.node.Attributes[name] = value
	n.syncAttributeState(name, true)
	n.markAttributeDirty(name)
	return true
}

// RemoveAttribute removes an element attribute.
func (n Node) RemoveAttribute(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !n.valid() || n.node.Type != dom.ElementNode {
		return false
	}
	if _, ok := n.node.Attributes[name]; !ok {
		return false
	}
	delete(n.node.Attributes, name)
	n.syncAttributeState(name, false)
	n.markAttributeDirty(name)
	return true
}

func (n Node) syncAttributeState(name string, present bool) {
	switch name {
	case "checked":
		dom.SynchronizeCheckedAttribute(n.node, present)
	case "selected":
		dom.SynchronizeSelectedAttribute(n.node, present)
	case "value":
		dom.SynchronizeValueAttribute(n.node)
	case "placeholder":
		dom.SynchronizePlaceholderAttribute(n.node)
	case "type":
		dom.SynchronizeValueAttribute(n.node)
		dom.SynchronizePlaceholderAttribute(n.node)
	case "multiple", "size":
		dom.SynchronizeSelectAttribute(n.node, name)
	}
}

// SetHidden controls the standard hidden attribute.
func (n Node) SetHidden(hidden bool) bool {
	if hidden {
		return n.SetAttribute("hidden", "")
	}
	if !n.valid() || n.node.Type != dom.ElementNode {
		return false
	}
	if _, ok := n.node.Attributes["hidden"]; ok {
		delete(n.node.Attributes, "hidden")
		n.markAttributeDirty("hidden")
	}
	return true
}

// SetSelectedIndex writes the live selection, preserving authored defaults.
// It also works before the first render or while option text is being updated.
func (n Node) SetSelectedIndex(selected int) bool {
	if !n.valid() || n.node.Type != dom.ElementNode || n.node.Data != "select" || selected < -1 {
		return false
	}
	state := ecs.GetMut[htmlRenderState](n.world, n.owner)
	if state == nil {
		return false
	}
	dom.SetSelectedIndex(n.node, selected)
	key := htmlRenderIdentity{Node: n.node, Role: htmlRoleControl, Variant: inputType(n.node)}
	if group := state.EntityGroups[key]; group != nil {
		for _, binding := range group.Bindings {
			if control := ecs.GetMut[Control](n.world, binding.Entity); control != nil && control.Owner == n.owner {
				syncHTMLSelectOptionStates(n.node, control)
			}
		}
	}
	n.markMutation(htmlMutationContent)
	return true
}

func (n Node) valid() bool {
	return n.world != nil && n.owner != 0 && n.node != nil
}

func findHTMLNodeByID(node *dom.Node, id string) *dom.Node {
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

func cloneHTMLNode(node, parent *dom.Node) *dom.Node {
	if node == nil {
		return nil
	}
	clone := &dom.Node{Type: node.Type, Data: node.Data, Parent: parent, States: node.States, Order: node.Order}
	if len(node.Attributes) > 0 {
		clone.Attributes = make(map[string]string, len(node.Attributes))
		for name, value := range node.Attributes {
			clone.Attributes[name] = value
		}
	}
	if len(node.Children) > 0 {
		clone.Children = make([]*dom.Node, len(node.Children))
		for index, child := range node.Children {
			clone.Children[index] = cloneHTMLNode(child, clone)
		}
	}
	return clone
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
	state, ok := ecs.Get[htmlRenderState](n.world, n.owner)
	if !ok || !state.Rendered {
		return Rect{}, false
	}
	return htmlNodeBounds(state.Boxes, n.node)
}

// ScrollContainer returns the node's synchronized overflow scroll entity,
// including when its content is outside the visible paint area.
func (n Node) ScrollContainer() (ecs.Entity, bool) {
	if !n.valid() {
		return 0, false
	}
	state := ecs.GetRef[htmlRenderState](n.world, n.owner)
	if state == nil {
		return 0, false
	}
	entity := state.OverflowBindings[n.node].entity
	_, ok := ecs.Get[scene.UIScroll](n.world, entity)
	return entity, ok
}

func htmlNodeBounds(box *layout.Box, node *dom.Node) (Rect, bool) {
	if box == nil {
		return Rect{}, false
	}
	if box.Node == node {
		r := box.BorderBox()
		return Rect{float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height)}, r.Width > 0 && r.Height > 0
	}
	for _, child := range box.Children {
		if r, ok := htmlNodeBounds(child, node); ok {
			return r, true
		}
	}
	return Rect{}, false
}
