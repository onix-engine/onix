package html

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/scene"
)

// Template is the immutable source of a component view. It is parsed once
// by Define and reused by every component instance.
type Template struct {
	Source string
}

// NewTemplate creates a component template from an HTML fragment.
func NewTemplate(source string) Template {
	return Template{Source: source}
}

// Component is the typed state and props of one HTML component instance.
// Instances must be created with MountComponent so the page runtime can
// attach the type-erased render record used by the HTML component system.
type Component[P, S any] struct {
	Definition *Definition[P, S]
	Props      P
	State      S
	Page       ecs.Entity
	Parent     ecs.Entity
	Key        uint64
}

// Behavior describes the render and event logic of a component.
type Behavior[P, S any] struct {
	Render      func(RenderContext[P, S], *View)
	Mount       func(*ComponentContext[P, S])
	Update      func(*ComponentContext[P, S])
	Unmount     func(*ComponentContext[P, S])
	HandleEvent func(*ComponentEventContext[P, S])
}

// Definition is a compiled, reusable component definition.
type Definition[P, S any] struct {
	Template Template
	CSS      string
	// ScopedCSS isolates this definition's selectors and template elements.
	ScopedCSS bool
	Behavior  Behavior[P, S]

	runtime *htmlDefinitionRuntime
}

// RenderContext is passed by value to Render. Props and State point to the
// component instance; copying the context does not copy its props or state.
type RenderContext[P, S any] struct {
	World     *ecs.World
	Component ecs.Entity
	Props     *P
	State     *S
}

// ComponentContext is passed to component lifecycle callbacks.
type ComponentContext[P, S any] struct {
	World     *ecs.World
	Component ecs.Entity
	Props     *P
	State     *S
}

// ComponentEventContext is passed to a typed component event handler.
type ComponentEventContext[P, S any] struct {
	World     *ecs.World
	Component ecs.Entity
	Ref       string
	Event     Event
	Pointer   *PointerEvent
	Props     *P
	State     *S

	defaultPrevented bool
	stopPropagation  bool
}

// UpdateState mutates state and schedules one component render for the current
// synchronization pass. Mutating State directly is also supported; this method
// is useful when the update should be explicit at the call site.
func (c *ComponentEventContext[P, S]) UpdateState(update func(*S)) bool {
	if c == nil || c.World == nil || update == nil {
		return false
	}
	component := ecs.GetMut[Component[P, S]](c.World, c.Component)
	if component == nil {
		return false
	}
	update(&component.State)
	markHTMLComponentDirty(c.World, c.Component)
	c.State = &component.State
	return true
}

// PreventDefault cancels the component event's default action.
func (c *ComponentEventContext[P, S]) PreventDefault() {
	if c == nil || !c.Event.Cancelable {
		return
	}
	c.defaultPrevented = true
	c.Event.DefaultPrevented = true
}

// StopPropagation consumes the pointer event so it does not reach an HTML
// ancestor callback.
func (c *ComponentEventContext[P, S]) StopPropagation() {
	if c == nil {
		return
	}
	c.stopPropagation = true
	if c.Pointer != nil {
		c.Pointer.Consume()
	}
}

// DefaultPrevented reports whether PreventDefault was called.
func (c *ComponentEventContext[P, S]) DefaultPrevented() bool {
	return c != nil && c.defaultPrevented
}

// Define parses and compiles an HTML component definition.
func Define[P, S any](template Template, cssText string, behavior Behavior[P, S]) (*Definition[P, S], error) {
	compiled, err := compileHTMLComponentTemplate(template.Source)
	if err != nil {
		return nil, err
	}
	definition := &Definition[P, S]{Template: template, CSS: cssText, Behavior: behavior}
	definition.CSS = joinHTMLComponentCSS(cssText, compiled.EmbeddedCSS)
	definition.runtime = &htmlDefinitionRuntime{template: compiled}
	definition.runtime.mountChild = func(world *ecs.World, parent ecs.Entity, slot string, key uint64, values any) (ecs.Entity, error) {
		child := values.(*htmlComponentChildValues[P, S])
		return MountComponentInto(world, parent, slot, definition, child.Props, child.State, key)
	}
	definition.runtime.updateChild = func(world *ecs.World, entity ecs.Entity, values any) {
		// Initial state is used only on mount; keyed updates preserve local state.
		UpdateComponentProps[P, S](world, entity, values.(*htmlComponentChildValues[P, S]).Props)
	}
	definition.runtime.clearChild = func(values any) {
		*values.(*htmlComponentChildValues[P, S]) = htmlComponentChildValues[P, S]{}
	}
	definition.runtime.render = func(world *ecs.World, entity ecs.Entity, instance *htmlComponentInstance) {
		component := ecs.GetMut[Component[P, S]](world, entity)
		if component == nil {
			return
		}
		view := &instance.renderView
		*view = View{instance: instance, template: compiled}
		view.reset()
		if behavior.Render != nil {
			behavior.Render(RenderContext[P, S]{
				World: world, Component: entity, Props: &component.Props, State: &component.State,
			}, view)
		}
		reconcileHTMLComponentView(world, entity, instance, view)
	}
	definition.runtime.event = func(world *ecs.World, entity ecs.Entity, ref string, event Event, pointer *PointerEvent, emit bool) (Event, bool) {
		component := ecs.GetMut[Component[P, S]](world, entity)
		if component == nil {
			return event, false
		}
		instance := ecs.GetMut[htmlComponentInstance](world, entity)
		if instance == nil {
			return event, false
		}
		if event.Entity == 0 {
			event.Entity = component.Page
		}
		if event.Target == 0 {
			event.Target = entity
		}
		if event.ID == "" {
			event.ID = ref
		}
		context := &ComponentEventContext[P, S]{
			World: world, Component: entity, Ref: ref, Event: event, Pointer: pointer,
			Props: &component.Props, State: &component.State,
		}
		if behavior.HandleEvent != nil {
			behavior.HandleEvent(context)
		}
		instance.Dirty = true
		if context.defaultPrevented && pointer != nil {
			pointer.Consume()
		}
		if emit {
			ecs.WriteMessage(world, context.Event)
		}
		return context.Event, context.stopPropagation
	}
	definition.runtime.mount = func(world *ecs.World, entity ecs.Entity) {
		if behavior.Mount == nil {
			return
		}
		component := ecs.GetMut[Component[P, S]](world, entity)
		if component == nil {
			return
		}
		behavior.Mount(&ComponentContext[P, S]{World: world, Component: entity, Props: &component.Props, State: &component.State})
	}
	definition.runtime.update = func(world *ecs.World, entity ecs.Entity) {
		if behavior.Update == nil {
			return
		}
		component := ecs.GetMut[Component[P, S]](world, entity)
		if component == nil {
			return
		}
		behavior.Update(&ComponentContext[P, S]{World: world, Component: entity, Props: &component.Props, State: &component.State})
	}
	definition.runtime.unmount = func(world *ecs.World, entity ecs.Entity) {
		if behavior.Unmount == nil {
			return
		}
		component := ecs.GetMut[Component[P, S]](world, entity)
		if component == nil {
			return
		}
		behavior.Unmount(&ComponentContext[P, S]{World: world, Component: entity, Props: &component.Props, State: &component.State})
	}
	return definition, nil
}

// DefineScoped creates a component whose template elements and stylesheet
// are isolated by a stable, definition-level scope class.
func DefineScoped[P, S any](template Template, cssText string, behavior Behavior[P, S]) (*Definition[P, S], error) {
	definition, err := Define(template, cssText, behavior)
	if err != nil {
		return nil, err
	}
	scopeClass := htmlComponentScopeClass(template.Source, definition.CSS)
	if definition.CSS != "" {
		definition.CSS, err = css.ScopeStylesheet(definition.CSS, scopeClass)
		if err != nil {
			return nil, fmt.Errorf("ecs: scope HTML component CSS: %w", err)
		}
	}
	definition.ScopedCSS = true
	definition.runtime.scopeClass = scopeClass
	return definition, nil
}

func htmlComponentScopeClass(template, cssText string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(template))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(cssText))
	return fmt.Sprintf("onix-scope-%08x", hash.Sum32())
}

// View writes dynamic values into the component's next VDOM buffer.
// References come from data-ref attributes in the compiled template.
type View struct {
	instance *htmlComponentInstance
	template *htmlComponentTemplate
	patch    bool
}

// PatchBindings updates only the Text, Attr and RemoveAttr bindings written by
// this render. Other bindings, events and child lists retain their ownership.
// This render may only write scalar bindings; structural updates require an ordinary render.
func (v *View) PatchBindings() { v.patch = true }

// VNodeAttribute is one attribute on a keyed VNode child.
type VNodeAttribute struct {
	Name  string
	Value string
}

// VNode describes one keyed element child. Children may contain nested
// keyed elements and are reconciled in place with the same DOM identity rules.
type VNode struct {
	Key        uint64
	Tag        string
	Text       string
	Attributes []VNodeAttribute
	Children   []VNode
}

// ComponentVNode describes one typed child component in a component VDOM.
// Values are snapshots; mount/update operations are shared by the definition.
type ComponentVNode struct {
	Key        uint64
	definition *htmlDefinitionRuntime
	values     any
	owned      bool // Only the View's private buffers may reuse this payload.
}

type htmlComponentChildValues[P, S any] struct {
	Props P
	State S
}

// ComponentChild creates a keyed child component VNode for a slot.
func ComponentChild[P, S any](key uint64, definition *Definition[P, S], props P, state S) ComponentVNode {
	child := ComponentVNode{Key: key}
	if definition != nil && definition.runtime != nil {
		child.definition = definition.runtime
		child.values = &htmlComponentChildValues[P, S]{Props: props, State: state}
	}
	return child
}

// AppendComponentChild appends a typed child directly to a View slot for this
// render. The slot owns its parameter buffers and reuses them on later renders.
// Call order determines child order; omitting a child on a render unmounts it.
func AppendComponentChild[P, S any](v *View, slot string, key uint64, definition *Definition[P, S], props P, state S) bool {
	if definition == nil || definition.runtime == nil || !v.componentSlot(slot) {
		return false
	}
	nodes := v.instance.Next.ComponentChildren[slot]
	index := len(nodes)
	if index == cap(nodes) {
		nodes = append(nodes, ComponentVNode{})
	} else {
		nodes = nodes[:index+1]
	}
	node := &nodes[index]
	var values *htmlComponentChildValues[P, S]
	if node.owned {
		values, _ = node.values.(*htmlComponentChildValues[P, S])
	}
	if values == nil {
		values = &htmlComponentChildValues[P, S]{}
	}
	*values = htmlComponentChildValues[P, S]{Props: props, State: state}
	*node = ComponentVNode{Key: key, definition: definition.runtime, values: values, owned: true}
	v.instance.Next.ComponentChildren[slot] = nodes
	return true
}

func (n *ComponentVNode) reset() {
	if !n.owned {
		*n = ComponentVNode{}
		return
	}
	if n.definition != nil {
		n.definition.clearChild(n.values)
	}
	n.Key, n.definition = 0, nil
}

// TextVNode creates a keyed text-bearing element node.
func TextVNode(key uint64, tag, text string) VNode {
	return VNode{Key: key, Tag: tag, Text: text}
}

// VNodeElement creates a keyed element with nested VNode children.
func VNodeElement(key uint64, tag string, attributes []VNodeAttribute, children []VNode) VNode {
	return VNode{Key: key, Tag: tag, Attributes: attributes, Children: children}
}

// Text updates a text binding identified by data-ref.
func (v *View) Text(ref, value string) bool {
	if v == nil || v.instance == nil || !v.template.hasRef(ref) {
		return false
	}
	v.instance.Next.Texts[ref] = value
	return true
}

// Attr updates an element attribute binding identified by data-ref.
func (v *View) Attr(ref, name, value string) bool {
	if v == nil || v.instance == nil || !v.template.hasRef(ref) || strings.TrimSpace(name) == "" {
		return false
	}
	key := htmlComponentAttrKey{Ref: ref, Name: strings.ToLower(strings.TrimSpace(name))}
	delete(v.instance.Next.RemovedAttrs, key)
	v.instance.Next.Attrs[key] = value
	return true
}

// RemoveAttr removes an element attribute from a dynamic binding.
func (v *View) RemoveAttr(ref, name string) bool {
	if v == nil || v.instance == nil || !v.template.hasRef(ref) || strings.TrimSpace(name) == "" {
		return false
	}
	key := htmlComponentAttrKey{Ref: ref, Name: strings.ToLower(strings.TrimSpace(name))}
	delete(v.instance.Next.Attrs, key)
	v.instance.Next.RemovedAttrs[key] = true
	return true
}

// Event enables a browser-style event binding identified by data-ref. Pointer
// clicks are delivered during input dispatch; native control events are
// delivered from the retained HTML event stream before the next render pass.
func (v *View) Event(ref string, kind EventKind) bool {
	if v == nil || v.instance == nil || !v.template.hasRef(ref) || !htmlComponentEventKindSupported(kind) {
		return false
	}
	v.instance.Next.Events[htmlComponentEventKey{Ref: ref, Kind: kind}] = struct{}{}
	return true
}

func htmlComponentEventKindSupported(kind EventKind) bool {
	switch kind {
	case EventClick, EventBeforeInput, EventInput, EventChange,
		EventSubmit, EventReset, EventFocus, EventBlur:
		return true
	default:
		return false
	}
}

// Keyed reconciles element children under a data-ref container. Existing DOM
// nodes are matched by Key and moved in place, preserving their DOM identity.
func (v *View) Keyed(ref string, nodes []VNode) bool {
	if v == nil || v.instance == nil || !v.template.hasRef(ref) {
		return false
	}
	if v.instance.Keyed[ref] == nil {
		v.instance.Keyed[ref] = &htmlComponentKeyedList{}
	}
	values := v.instance.Next.Keyed[ref][:0]
	values = append(values, nodes...)
	v.instance.Next.Keyed[ref] = values
	v.instance.Next.KeyedActive[ref] = true
	return true
}

// Components declares keyed child component instances rendered into a slot.
// Omitting a slot on a later render unmounts its previous children.
func (v *View) Components(slot string, nodes []ComponentVNode) bool {
	if !v.componentSlot(slot) {
		return false
	}
	values := v.instance.Next.ComponentChildren[slot][:0]
	values = append(values, nodes...)
	// A replacement may shorten a slot already populated by AppendComponentChild.
	for index := len(values); index < cap(values); index++ {
		values[:cap(values)][index].reset()
	}
	v.instance.Next.ComponentChildren[slot] = values
	return true
}

func (v *View) componentSlot(slot string) bool {
	if v == nil || v.instance == nil {
		return false
	}
	if v.instance.Next.ComponentChildrenActive[slot] {
		return true
	}
	if !v.template.hasRef(slot) {
		return false
	}
	if v.instance.ComponentChildren == nil {
		v.instance.ComponentChildren = make(map[string]*htmlComponentChildList)
	}
	if v.instance.ComponentChildren[slot] == nil {
		v.instance.ComponentChildren[slot] = &htmlComponentChildList{}
	}
	if v.instance.Next.ComponentChildren == nil {
		v.instance.Next.ComponentChildren = make(map[string][]ComponentVNode)
	}
	v.instance.Next.ComponentChildrenActive[slot] = true
	return true
}

type htmlDefinitionRuntime struct {
	mountChild  func(*ecs.World, ecs.Entity, string, uint64, any) (ecs.Entity, error)
	updateChild func(*ecs.World, ecs.Entity, any)
	clearChild  func(any)
	template    *htmlComponentTemplate
	scopeClass  string
	render      func(*ecs.World, ecs.Entity, *htmlComponentInstance)
	event       func(*ecs.World, ecs.Entity, string, Event, *PointerEvent, bool) (Event, bool)
	mount       func(*ecs.World, ecs.Entity)
	update      func(*ecs.World, ecs.Entity)
	unmount     func(*ecs.World, ecs.Entity)
}

type htmlComponentInstance struct {
	renderView        View
	Page              ecs.Entity
	Parent            ecs.Entity
	Key               uint64
	Definition        *htmlDefinitionRuntime
	Roots             []*dom.Node
	Refs              map[string]*dom.Node
	Handlers          map[string]ClickHandler
	EventNodes        map[htmlComponentEventKey]*dom.Node
	Children          []ecs.Entity
	Keyed             map[string]*htmlComponentKeyedList
	ComponentChildren map[string]*htmlComponentChildList
	Current           htmlComponentVNodeState
	Next              htmlComponentVNodeState
	Dirty             bool
	Mounted           bool
	ChildrenPending   bool
	Rendering         bool
	Unmounting        bool
	UnmountPending    bool
}

type htmlComponentVNodeState struct {
	Texts                   map[string]string
	Attrs                   map[htmlComponentAttrKey]string
	RemovedAttrs            map[htmlComponentAttrKey]bool
	Events                  map[htmlComponentEventKey]struct{}
	Keyed                   map[string][]VNode
	KeyedActive             map[string]bool
	ComponentChildren       map[string][]ComponentVNode
	ComponentChildrenActive map[string]bool
}

type htmlComponentAttrKey struct {
	Ref  string
	Name string
}

type htmlComponentAttributeNamesScratch struct{}
type htmlComponentRemovedRootsScratch struct{}
type htmlComponentRemovedParentsScratch struct{}

type htmlComponentEventKey struct {
	Ref  string
	Kind EventKind
}

type htmlComponentEventBinding struct {
	Component ecs.Entity
	Ref       string
	Kind      EventKind
}

type htmlComponentKeyedItem struct {
	Key  uint64
	Node *dom.Node
}

type htmlComponentKeyedList struct {
	htmlComponentVNodeChildren
	Children map[*dom.Node]*htmlComponentVNodeChildren
	Nodes    map[*dom.Node]bool
	Anchor   *dom.Node
}

type htmlComponentChildItem struct {
	Key        uint64
	Definition *htmlDefinitionRuntime
	Entity     ecs.Entity
}

type htmlComponentChildList struct {
	Items    []htmlComponentChildItem
	Scratch  []htmlComponentChildItem
	OldIndex map[uint64]int
	Seen     map[uint64]bool
	Nodes    map[*dom.Node]bool
}

type htmlComponentVNodeChildren struct {
	Items    []htmlComponentKeyedItem
	Scratch  []htmlComponentKeyedItem
	OldIndex map[uint64]int
	Seen     map[uint64]bool
	Used     map[uint64]bool
	Removed  map[*dom.Node]struct{}
}

type htmlComponentTemplate struct {
	Root        *dom.Node
	EmbeddedCSS string
	Refs        map[string]struct{}
}

func compileHTMLComponentTemplate(source string) (*htmlComponentTemplate, error) {
	root, embeddedCSS, err := parseHTMLDocument(source)
	if err != nil {
		return nil, fmt.Errorf("ecs: parse HTML component template: %w", err)
	}
	result := &htmlComponentTemplate{Root: root, EmbeddedCSS: embeddedCSS, Refs: make(map[string]struct{})}
	var duplicate string
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil || duplicate != "" {
			return
		}
		if ref, ok := node.Attr("data-ref"); ok && ref != "" {
			if _, exists := result.Refs[ref]; exists {
				duplicate = ref
			} else {
				result.Refs[ref] = struct{}{}
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	if duplicate != "" {
		return nil, fmt.Errorf("ecs: duplicate HTML component data-ref %q", duplicate)
	}
	return result, nil
}

func (t *htmlComponentTemplate) hasRef(ref string) bool {
	if t == nil {
		return false
	}
	_, ok := t.Refs[ref]
	return ok
}

func joinHTMLComponentCSS(cssText, embedded string) string {
	if cssText == "" {
		return embedded
	}
	if embedded == "" {
		return cssText
	}
	return cssText + "\n" + embedded
}

// MountComponent appends a component's template roots to a page HTML
// document and creates the typed ECS component instance.
func MountComponent[P, S any](world *ecs.World, page ecs.Entity, definition *Definition[P, S], props P, state S, key uint64) (ecs.Entity, error) {
	return mountHTMLComponentAt(world, page, 0, nil, definition, props, state, key, nil)
}

// HydrateComponent adopts one matching component root sequence already
// retained by the page DOM. The sequence must be unique among the page's
// direct children; dynamic values are reconciled on the first Sync pass.
func HydrateComponent[P, S any](world *ecs.World, page ecs.Entity, definition *Definition[P, S], props P, state S, key uint64) (ecs.Entity, error) {
	return hydrateHTMLComponentAt(world, page, 0, nil, definition, props, state, key)
}

// MountComponentInto mounts a child component into a data-ref element of
// an existing component. The child remains an independent ECS entity while
// its roots participate in the parent's DOM subtree.
func MountComponentInto[P, S any](world *ecs.World, parent ecs.Entity, slot string, definition *Definition[P, S], props P, state S, key uint64) (ecs.Entity, error) {
	if world == nil || parent == 0 {
		return 0, fmt.Errorf("ecs: HTML component parent is invalid")
	}
	parentInstance, ok := ecs.Get[htmlComponentInstance](world, parent)
	if !ok || parentInstance.Page == 0 {
		return 0, fmt.Errorf("ecs: entity %d is not an HTML component", parent)
	}
	slotNode := parentInstance.Refs[slot]
	if slotNode == nil || slotNode.Type != dom.ElementNode {
		return 0, fmt.Errorf("ecs: HTML component slot %q is missing", slot)
	}
	return mountHTMLComponentAt(world, parentInstance.Page, parent, slotNode, definition, props, state, key, nil)
}

// HydrateComponentInto adopts one matching child root sequence already
// present in a component slot. It preserves the existing DOM node identity.
func HydrateComponentInto[P, S any](world *ecs.World, parent ecs.Entity, slot string, definition *Definition[P, S], props P, state S, key uint64) (ecs.Entity, error) {
	if world == nil || parent == 0 {
		return 0, fmt.Errorf("ecs: HTML component parent is invalid")
	}
	parentInstance, ok := ecs.Get[htmlComponentInstance](world, parent)
	if !ok || parentInstance.Page == 0 {
		return 0, fmt.Errorf("ecs: entity %d is not an HTML component", parent)
	}
	slotNode := parentInstance.Refs[slot]
	if slotNode == nil || slotNode.Type != dom.ElementNode {
		return 0, fmt.Errorf("ecs: HTML component slot %q is missing", slot)
	}
	return hydrateHTMLComponentAt(world, parentInstance.Page, parent, slotNode, definition, props, state, key)
}

func hydrateHTMLComponentAt[P, S any](world *ecs.World, page, parent ecs.Entity, slotNode *dom.Node, definition *Definition[P, S], props P, state S, key uint64) (ecs.Entity, error) {
	if world == nil {
		return 0, fmt.Errorf("ecs: HTML component world is nil")
	}
	if definition == nil || definition.runtime == nil || definition.runtime.template == nil {
		return 0, fmt.Errorf("ecs: HTML component definition is nil")
	}
	if err := ensureHTMLDefinitionScope(definition); err != nil {
		return 0, err
	}
	root, err := Root(world, page)
	if err != nil {
		return 0, err
	}
	container := root.node
	if parent != 0 {
		if slotNode == nil || !htmlNodeWithin(slotNode, root.node) {
			return 0, fmt.Errorf("ecs: HTML component slot is detached")
		}
		container = slotNode
	}
	roots, err := findHTMLComponentHydrationRoots(container, definition.runtime.template.Root.Children)
	if err != nil {
		return 0, err
	}
	return mountHTMLComponentAt(world, page, parent, container, definition, props, state, key, roots)
}

func mountHTMLComponentAt[P, S any](world *ecs.World, page, parent ecs.Entity, slotNode *dom.Node, definition *Definition[P, S], props P, state S, key uint64, adoptedRoots []*dom.Node) (ecs.Entity, error) {
	if world == nil {
		return 0, fmt.Errorf("ecs: HTML component world is nil")
	}
	if definition == nil || definition.runtime == nil || definition.runtime.template == nil {
		return 0, fmt.Errorf("ecs: HTML component definition is nil")
	}
	if err := ensureHTMLDefinitionScope(definition); err != nil {
		return 0, err
	}
	root, err := Root(world, page)
	if err != nil {
		return 0, err
	}
	mountParent := root.node
	if parent != 0 {
		if slotNode == nil || !htmlNodeWithin(slotNode, root.node) {
			return 0, fmt.Errorf("ecs: HTML component slot is detached")
		}
		mountParent = slotNode
	}
	instance := htmlComponentInstance{
		Page: page, Parent: parent, Key: key, Definition: definition.runtime,
		Refs: make(map[string]*dom.Node), Handlers: make(map[string]ClickHandler), EventNodes: make(map[htmlComponentEventKey]*dom.Node),
		Keyed:             make(map[string]*htmlComponentKeyedList),
		ComponentChildren: make(map[string]*htmlComponentChildList),
		Current:           newHTMLComponentVNodeState(), Next: newHTMLComponentVNodeState(), Dirty: true,
	}
	if adoptedRoots != nil {
		for _, root := range adoptedRoots {
			instance.Roots = append(instance.Roots, root)
			prepareHTMLComponentSubtree(root, definition.runtime.scopeClass, instance.Refs)
		}
	} else {
		for _, child := range definition.runtime.template.Root.Children {
			clone := cloneHTMLNode(child, nil)
			mountParent.AppendChild(clone)
			instance.Roots = append(instance.Roots, clone)
			prepareHTMLComponentSubtree(clone, definition.runtime.scopeClass, instance.Refs)
		}
	}
	bundle := ecs.NewBundle(3)
	ancestor := parent
	if ancestor == 0 {
		ancestor = page
	}
	ecs.Put(&bundle, scene.Parent{Entity: ancestor})
	ecs.Put(&bundle, Component[P, S]{Definition: definition, Props: props, State: state, Page: page, Parent: parent, Key: key})
	ecs.Put(&bundle, instance)
	entity := world.Spawn(bundle)
	if err := attachHTMLComponentCSS(world, page, definition.runtime, definition.CSS); err != nil {
		world.Despawn(entity)
		return 0, err
	}
	if pageState := ecs.GetMut[htmlRenderState](world, page); pageState != nil {
		pageState.IndexDirty = true
		pageState.Revision++
	}
	if parent != 0 {
		if parentInstance := ecs.GetMut[htmlComponentInstance](world, parent); parentInstance != nil {
			parentInstance.Children = append(parentInstance.Children, entity)
		}
	}
	definition.runtime.mount(world, entity)
	return entity, nil
}

func findHTMLComponentHydrationRoots(container *dom.Node, templates []*dom.Node) ([]*dom.Node, error) {
	if container == nil || container.Type != dom.ElementNode && container.Type != dom.DocumentNode {
		return nil, fmt.Errorf("ecs: HTML hydration container is invalid")
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("ecs: HTML hydration template has no root nodes")
	}
	matchStart := -1
	for start := 0; start+len(templates) <= len(container.Children); start++ {
		matched := true
		for index, template := range templates {
			if !htmlComponentHydrationRootMatches(container.Children[start+index], template) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if matchStart >= 0 {
			return nil, fmt.Errorf("ecs: HTML hydration template matches multiple root sequences")
		}
		matchStart = start
	}
	if matchStart < 0 {
		return nil, fmt.Errorf("ecs: HTML hydration template does not match retained DOM")
	}
	return container.Children[matchStart : matchStart+len(templates)], nil
}

func htmlComponentHydrationRootMatches(actual, template *dom.Node) bool {
	if actual == nil || template == nil || actual.Type != template.Type {
		return false
	}
	if actual.Type != dom.ElementNode {
		return actual.Data == template.Data
	}
	if !strings.EqualFold(actual.Data, template.Data) {
		return false
	}
	templateRef, templateHasRef := template.Attr("data-ref")
	if !templateHasRef {
		return true
	}
	actualRef, actualHasRef := actual.Attr("data-ref")
	return actualHasRef && actualRef == templateRef
}

func ensureHTMLDefinitionScope[P, S any](definition *Definition[P, S]) error {
	if definition == nil || !definition.ScopedCSS || definition.runtime == nil {
		return nil
	}
	if definition.runtime.scopeClass != "" {
		return nil
	}
	scopeClass := htmlComponentScopeClass(definition.Template.Source, definition.CSS)
	if definition.CSS != "" {
		cssText, err := css.ScopeStylesheet(definition.CSS, scopeClass)
		if err != nil {
			return fmt.Errorf("ecs: scope HTML component CSS: %w", err)
		}
		definition.CSS = cssText
	}
	definition.runtime.scopeClass = scopeClass
	return nil
}

// UpdateComponentProps replaces typed props and schedules a render.
func UpdateComponentProps[P, S any](world *ecs.World, entity ecs.Entity, props P) bool {
	component := ecs.GetMut[Component[P, S]](world, entity)
	if component == nil {
		return false
	}
	component.Props = props
	markHTMLComponentDirty(world, entity)
	return true
}

// UpdateComponentState applies a typed state update and schedules a render.
func UpdateComponentState[P, S any](world *ecs.World, entity ecs.Entity, update func(*S)) bool {
	if update == nil {
		return false
	}
	component := ecs.GetMut[Component[P, S]](world, entity)
	if component == nil {
		return false
	}
	update(&component.State)
	markHTMLComponentDirty(world, entity)
	return true
}

// UnmountComponent removes the component roots, bindings, and ECS entity.
func UnmountComponent(world *ecs.World, entity ecs.Entity) bool {
	if world == nil || entity == 0 {
		return false
	}
	instance := ecs.GetMut[htmlComponentInstance](world, entity)
	if instance == nil {
		return false
	}
	if instance.Unmounting {
		return false
	}
	if instance.Rendering {
		instance.UnmountPending = true
		return true
	}
	instance.Unmounting = true
	if instance.Definition != nil && instance.Definition.unmount != nil {
		instance.Definition.unmount(world, entity)
	}
	for len(instance.Children) > 0 {
		index := len(instance.Children) - 1
		child := instance.Children[index]
		instance.Children = instance.Children[:index]
		UnmountComponent(world, child)
	}
	for ref, _ := range instance.Handlers {
		if node := instance.Refs[ref]; node != nil {
			Node{world: world, owner: instance.Page, node: node}.OnClick(nil)
		}
	}
	for key := range instance.Current.Events {
		removeHTMLComponentEventBinding(world, instance.Page, instance.EventNodes[key], key, entity)
	}
	removeHTMLComponentRoots(world, instance.Page, instance.Roots)
	_ = detachHTMLComponentCSS(world, instance.Page, instance.Definition)
	if instance.Parent != 0 {
		if parent := ecs.GetMut[htmlComponentInstance](world, instance.Parent); parent != nil {
			for index, child := range parent.Children {
				if child != entity {
					continue
				}
				copy(parent.Children[index:], parent.Children[index+1:])
				parent.Children[len(parent.Children)-1] = 0
				parent.Children = parent.Children[:len(parent.Children)-1]
				break
			}
		}
	}
	return world.Despawn(entity)
}

// Root removal follows lifecycle callbacks. Filter each affected sibling list
// once and synchronize its text and resource mutations after all removals.
func removeHTMLComponentRoots(world *ecs.World, owner ecs.Entity, roots []*dom.Node) {
	removed := ecs.FrameScratchMap[htmlComponentRemovedRootsScratch, *dom.Node, struct{}](world)
	parents := ecs.FrameScratchMap[htmlComponentRemovedParentsScratch, *dom.Node, struct{}](world)
	defer clear(removed)
	defer clear(parents)
	for _, node := range roots {
		if node != nil && node.Parent != nil {
			removed[node] = struct{}{}
			parents[node.Parent] = struct{}{}
		}
	}
	for parent := range parents {
		parentNode := Node{world: world, owner: owner, node: parent}
		kind := parentNode.structureMutationKind(nil)
		children := parent.Children[:0]
		for _, child := range parent.Children {
			if _, remove := removed[child]; !remove {
				children = append(children, child)
				continue
			}
			if kind&htmlMutationDefinitions == 0 && htmlSubtreeDefinesFilter(child) {
				kind |= htmlMutationDefinitions
			}
			child.Parent = nil
		}
		if len(children) != len(parent.Children) {
			clear(parent.Children[len(children):])
			parent.Children = children
			dom.SynchronizeTextContent(parent)
			parentNode.markMutation(kind)
		}
	}
}

func addHTMLComponentEventBinding(world *ecs.World, page ecs.Entity, binding htmlComponentEventBinding, node *dom.Node) {
	if world == nil || page == 0 || node == nil {
		return
	}
	state := ecs.GetMut[htmlRenderState](world, page)
	if state == nil {
		return
	}
	if state.ComponentEvents == nil {
		state.ComponentEvents = make(map[*dom.Node][]htmlComponentEventBinding)
	}
	state.ComponentEvents[node] = append(state.ComponentEvents[node], binding)
}

func removeHTMLComponentEventBinding(world *ecs.World, page ecs.Entity, node *dom.Node, key htmlComponentEventKey, component ecs.Entity) {
	if world == nil || page == 0 || node == nil {
		return
	}
	state := ecs.GetMut[htmlRenderState](world, page)
	if state == nil {
		return
	}
	bindings := state.ComponentEvents[node]
	for index, binding := range bindings {
		if binding.Component != component || binding.Ref != key.Ref || binding.Kind != key.Kind {
			continue
		}
		copy(bindings[index:], bindings[index+1:])
		bindings[len(bindings)-1] = htmlComponentEventBinding{}
		bindings = bindings[:len(bindings)-1]
		if len(bindings) == 0 {
			delete(state.ComponentEvents, node)
		} else {
			state.ComponentEvents[node] = bindings
		}
		return
	}
}

// SyncComponentsSystem renders dirty components before the HTML layout
// system runs. SyncSystem calls it internally, so applications normally
// only need to register the existing HTML system.
func SyncComponentsSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_html_components"
	}
	return ecs.SystemConfig{Name: name, System: syncHTMLComponents}
}

type htmlComponentEventReaderTag struct{}
type htmlComponentPendingTag struct{}

type htmlComponentPending struct {
	Entities []ecs.Entity
}

func syncHTMLComponents(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	world := context.World
	reader := ecs.Local[htmlComponentEventReaderTag, ecs.MessageReader[Event]](context)
	for event := range reader.All(world) {
		dispatchHTMLComponentEvent(world, event)
	}
	pending := ecs.Local[htmlComponentPendingTag, htmlComponentPending](context)
	pending.Entities = pending.Entities[:0]
	for entity, access := range ecs.Query[htmlComponentInstance](world).All {
		instance := access.Ref()

		if instance == nil || !instance.Dirty || instance.Definition == nil || !world.IsAlive(entity) || !scene.IsActive(world, entity) {
			continue
		}
		pending.Entities = append(pending.Entities, entity)
	}
	for _, entity := range pending.Entities {
		renderDirtyHTMLComponent(world, entity)
	}
	return nil
}

func renderDirtyHTMLComponent(world *ecs.World, entity ecs.Entity) {
	instance := ecs.GetMut[htmlComponentInstance](world, entity)
	if instance == nil || !instance.Dirty || instance.Definition == nil || !world.IsAlive(entity) || !scene.IsActive(world, entity) {
		return
	}
	renderHTMLComponent(world, entity, instance)
}

func renderHTMLComponent(world *ecs.World, entity ecs.Entity, instance *htmlComponentInstance) {
	if world == nil || instance == nil || instance.Definition == nil || instance.Rendering || !world.IsAlive(entity) {
		return
	}
	instance.Rendering = true
	instance.Dirty = false
	if instance.Mounted && instance.Definition.update != nil {
		instance.Definition.update(world, entity)
	}
	if instance.UnmountPending {
		instance.Rendering = false
		instance.UnmountPending = false
		UnmountComponent(world, entity)
		return
	}
	if world.IsAlive(entity) && instance.Definition.render != nil {
		instance.Definition.render(world, entity, instance)
	}
	if !world.IsAlive(entity) {
		return
	}
	if instance.UnmountPending {
		instance.Rendering = false
		instance.UnmountPending = false
		UnmountComponent(world, entity)
		return
	}
	instance.Rendering = false
	instance.Mounted = true
	if instance.ChildrenPending {
		reconcileHTMLComponentChildrenVNodes(world, entity, instance)
		instance.ChildrenPending = false
	}
}

func dispatchHTMLComponentEvent(world *ecs.World, event *Event) {
	if world == nil || event == nil || event.Entity == 0 || !scene.IsActive(world, event.Entity) || !htmlComponentMessageEvent(event.Kind) {
		return
	}
	node := htmlComponentEventNode(world, event)
	if node == nil {
		return
	}
	state, ok := ecs.Get[htmlRenderState](world, event.Entity)
	if !ok || state.ComponentEvents == nil {
		return
	}
	for current := node; current != nil; current = current.Parent {
		stopped := false
		for _, binding := range state.ComponentEvents[current] {
			if binding.Kind != event.Kind || !world.IsAlive(binding.Component) {
				continue
			}
			instance, instanceOK := ecs.Get[htmlComponentInstance](world, binding.Component)
			if !instanceOK || instance.Definition == nil || instance.Definition.event == nil {
				continue
			}
			updated, stop := instance.Definition.event(world, binding.Component, binding.Ref, *event, nil, false)
			*event = updated
			stopped = stop
			if stopped {
				break
			}
		}
		if stopped || !event.Bubbles {
			return
		}
	}
}

func htmlComponentMessageEvent(kind EventKind) bool {
	switch kind {
	case EventClick, EventBeforeInput, EventSubmit, EventReset:
		return false
	default:
		return true
	}
}

func dispatchHTMLComponentAction(world *ecs.World, event Event) Event {
	if world == nil || event.Entity == 0 || !scene.IsActive(world, event.Entity) || !htmlComponentEventKindSupported(event.Kind) {
		return event
	}
	node := htmlComponentEventNode(world, &event)
	if node == nil {
		return event
	}
	state, ok := ecs.Get[htmlRenderState](world, event.Entity)
	if !ok || state.ComponentEvents == nil {
		return event
	}
	for current := node; current != nil; current = current.Parent {
		stopped := false
		for _, binding := range state.ComponentEvents[current] {
			if binding.Kind != event.Kind || !world.IsAlive(binding.Component) {
				continue
			}
			instance, instanceOK := ecs.Get[htmlComponentInstance](world, binding.Component)
			if !instanceOK || instance.Definition == nil || instance.Definition.event == nil {
				continue
			}
			updated, stop := instance.Definition.event(world, binding.Component, binding.Ref, event, nil, false)
			event, stopped = updated, stop
			if stopped {
				break
			}
		}
		if stopped || !event.Bubbles {
			break
		}
	}
	return event
}

func htmlComponentEventNode(world *ecs.World, event *Event) *dom.Node {
	var node *dom.Node
	if event.Target != 0 {
		if child, ok := ecs.Get[htmlChild](world, event.Target); ok {
			node = child.Node
		}
	}
	if node == nil && event.Entity != 0 && event.ID != "" {
		if state := ecs.GetMut[htmlRenderState](world, event.Entity); state != nil {
			node = lookupHTMLNode(state, event.ID)
		}
	}
	// Submit is dispatched on the form owner, including external submitters.
	if node != nil && event.Kind == EventSubmit && node.Data != "form" {
		return dom.FormOwner(node)
	}
	return node
}

func markHTMLComponentDirty(world *ecs.World, entity ecs.Entity) {
	if instance := ecs.GetMut[htmlComponentInstance](world, entity); instance != nil {
		instance.Dirty = true
	}
}

func newHTMLComponentVNodeState() htmlComponentVNodeState {
	return htmlComponentVNodeState{
		Texts: make(map[string]string), Attrs: make(map[htmlComponentAttrKey]string),
		RemovedAttrs: make(map[htmlComponentAttrKey]bool), Events: make(map[htmlComponentEventKey]struct{}),
		Keyed: make(map[string][]VNode), KeyedActive: make(map[string]bool),
		ComponentChildren: make(map[string][]ComponentVNode), ComponentChildrenActive: make(map[string]bool),
	}
}

func (s *htmlComponentVNodeState) reset() {
	clear(s.Texts)
	clear(s.Attrs)
	clear(s.RemovedAttrs)
	clear(s.Events)
	for ref, values := range s.Keyed {
		clear(values[:cap(values)])
		s.Keyed[ref] = values[:0]
		s.KeyedActive[ref] = false
	}
	for slot, values := range s.ComponentChildren {
		for index := range cap(values) {
			values[:cap(values)][index].reset()
		}
		s.ComponentChildren[slot] = values[:0]
		s.ComponentChildrenActive[slot] = false
	}
}

func (v *View) reset() {
	if v == nil || v.instance == nil {
		return
	}
	v.instance.Next.reset()
}

func reconcileHTMLComponentView(world *ecs.World, entity ecs.Entity, instance *htmlComponentInstance, view *View) {
	if world == nil || instance == nil || view == nil {
		return
	}
	if view.patch {
		if len(instance.Next.Events) != 0 {
			panic("html: PatchBindings render may only write scalar bindings")
		}
		for _, active := range instance.Next.KeyedActive {
			if active {
				panic("html: PatchBindings render cannot reconcile keyed children")
			}
		}
		for _, active := range instance.Next.ComponentChildrenActive {
			if active {
				panic("html: PatchBindings render cannot reconcile components")
			}
		}
		for ref, value := range instance.Next.Texts {
			if old, ok := instance.Current.Texts[ref]; !ok || old != value {
				if node := instance.Refs[ref]; node != nil {
					Node{world: world, owner: instance.Page, node: node}.SetText(value)
				}
				instance.Current.Texts[ref] = value
			}
		}
		for key, value := range instance.Next.Attrs {
			if old, ok := instance.Current.Attrs[key]; !ok || old != value {
				if node := instance.Refs[key.Ref]; node != nil {
					setHTMLComponentAttribute(world, instance, node, key.Name, value)
				}
				instance.Current.Attrs[key] = value
				delete(instance.Current.RemovedAttrs, key)
			}
		}
		for key := range instance.Next.RemovedAttrs {
			if node := instance.Refs[key.Ref]; node != nil {
				removeHTMLComponentAttribute(world, instance, node, key.Name)
			}
			delete(instance.Current.Attrs, key)
			instance.Current.RemovedAttrs[key] = true
		}
		instance.Next.reset()
		return
	}
	for ref, oldValue := range instance.Current.Texts {
		if _, present := instance.Next.Texts[ref]; !present {
			if node := instance.Refs[ref]; node != nil {
				Node{world: world, owner: instance.Page, node: node}.SetText("")
			}
			delete(instance.Current.Texts, ref)
			_ = oldValue
		}
	}
	for ref, value := range instance.Next.Texts {
		if old, present := instance.Current.Texts[ref]; present && old == value {
			continue
		}
		if node := instance.Refs[ref]; node != nil {
			Node{world: world, owner: instance.Page, node: node}.SetText(value)
		}
		instance.Current.Texts[ref] = value
	}
	for key := range instance.Current.Attrs {
		if _, present := instance.Next.Attrs[key]; present {
			continue
		}
		if instance.Next.RemovedAttrs[key] {
			continue
		}
		if node := instance.Refs[key.Ref]; node != nil {
			removeHTMLComponentAttribute(world, instance, node, key.Name)
		}
		delete(instance.Current.Attrs, key)
	}
	for key, value := range instance.Next.Attrs {
		if old, present := instance.Current.Attrs[key]; present && old == value {
			continue
		}
		if node := instance.Refs[key.Ref]; node != nil {
			setHTMLComponentAttribute(world, instance, node, key.Name, value)
		}
		instance.Current.Attrs[key] = value
		delete(instance.Current.RemovedAttrs, key)
	}
	for key := range instance.Next.RemovedAttrs {
		if node := instance.Refs[key.Ref]; node != nil {
			removeHTMLComponentAttribute(world, instance, node, key.Name)
		}
		delete(instance.Current.Attrs, key)
	}
	for key := range instance.Current.Events {
		if _, present := instance.Next.Events[key]; present {
			continue
		}
		if key.Kind == EventClick {
			if node := instance.Refs[key.Ref]; node != nil {
				Node{world: world, owner: instance.Page, node: node}.OnClick(nil)
			}
		}
		removeHTMLComponentEventBinding(world, instance.Page, instance.EventNodes[key], key, entity)
		delete(instance.EventNodes, key)
		delete(instance.Current.Events, key)
	}
	for key := range instance.Next.Events {
		if _, present := instance.Current.Events[key]; present {
			continue
		}
		if key.Kind == EventClick {
			if node := instance.Refs[key.Ref]; node != nil {
				bindHTMLComponentClick(world, entity, instance, key.Ref, node)
			}
		}
		instance.EventNodes[key] = instance.Refs[key.Ref]
		addHTMLComponentEventBinding(world, instance.Page, htmlComponentEventBinding{Component: entity, Ref: key.Ref, Kind: key.Kind}, instance.Refs[key.Ref])
		instance.Current.Events[key] = struct{}{}
	}
	reconcileHTMLComponentKeyed(world, instance)
	instance.ChildrenPending = len(instance.ComponentChildren) != 0
	instance.Current, instance.Next = instance.Next, instance.Current
	instance.Next.reset()
}

func reconcileHTMLComponentChildrenVNodes(world *ecs.World, parentEntity ecs.Entity, instance *htmlComponentInstance) {
	if world == nil || instance == nil {
		return
	}
	for slot, list := range instance.ComponentChildren {
		if list == nil {
			continue
		}
		slotNode := instance.Refs[slot]
		next := instance.Current.ComponentChildren[slot]
		if !instance.Current.ComponentChildrenActive[slot] {
			next = nil
		}
		reconcileHTMLComponentChildList(world, parentEntity, instance, slot, slotNode, list, next)
		// Keep workspaces only for the currently declared child count. Collapsing
		// a large list releases its payloads instead of retaining a high-water pool.
		values := instance.Current.ComponentChildren[slot]
		clear(values[:cap(values)][len(next):])
		spare := instance.Next.ComponentChildren[slot]
		if len(next) < cap(spare) {
			clear(spare[:cap(spare)][len(next):])
		}
	}
}

func reconcileHTMLComponentChildList(world *ecs.World, parentEntity ecs.Entity, parent *htmlComponentInstance, slot string, slotNode *dom.Node, list *htmlComponentChildList, next []ComponentVNode) {
	if parent == nil || list == nil || slotNode == nil || slotNode.Type != dom.ElementNode {
		return
	}
	if list.Seen == nil {
		list.Seen = make(map[uint64]bool)
	}
	if list.OldIndex == nil {
		list.OldIndex = make(map[uint64]int)
	}
	if list.Nodes == nil {
		list.Nodes = make(map[*dom.Node]bool)
	}
	clear(list.Seen)
	clear(list.OldIndex)
	old := list.Items
	for index, item := range old {
		list.OldIndex[item.Key] = index
	}
	clear(list.Scratch[:cap(list.Scratch)])
	items := list.Scratch[:0]
	for _, vnode := range next {
		if vnode.definition == nil || vnode.values == nil || list.Seen[vnode.Key] {
			continue
		}
		list.Seen[vnode.Key] = true
		var item htmlComponentChildItem
		if index, ok := list.OldIndex[vnode.Key]; ok {
			item = old[index]
		}
		if item.Entity != 0 && item.Definition != vnode.definition {
			UnmountComponent(world, item.Entity)
			item = htmlComponentChildItem{}
		}
		if item.Entity == 0 || !world.IsAlive(item.Entity) {
			entity, err := vnode.definition.mountChild(world, parentEntity, slot, vnode.Key, vnode.values)
			if err != nil {
				continue
			}
			item = htmlComponentChildItem{Key: vnode.Key, Definition: vnode.definition, Entity: entity}
			renderMountedHTMLComponent(world, entity)
		} else {
			vnode.definition.updateChild(world, item.Entity, vnode.values)
			item.Definition = vnode.definition
			renderDirtyHTMLComponent(world, item.Entity)
		}
		items = append(items, item)
	}
	for _, oldItem := range old {
		if !list.Seen[oldItem.Key] && oldItem.Entity != 0 && world.IsAlive(oldItem.Entity) {
			UnmountComponent(world, oldItem.Entity)
		}
	}
	changed := len(old) != len(items)
	if !changed {
		for index := range old {
			if old[index].Entity != items[index].Entity {
				changed = true
				break
			}
		}
	}
	if changed {
		reorderHTMLComponentChildRoots(world, parent.Page, slotNode, list, items)
	}
	clear(old)
	list.Items, list.Scratch = items, old[:0]
}

func renderMountedHTMLComponent(world *ecs.World, entity ecs.Entity) {
	instance := ecs.GetMut[htmlComponentInstance](world, entity)
	if instance == nil || instance.Definition == nil {
		return
	}
	renderHTMLComponent(world, entity, instance)
}

func reorderHTMLComponentChildRoots(world *ecs.World, owner ecs.Entity, slotNode *dom.Node, list *htmlComponentChildList, items []htmlComponentChildItem) {
	if slotNode == nil || list == nil {
		return
	}
	if list.Nodes == nil {
		list.Nodes = make(map[*dom.Node]bool)
	}
	clear(list.Nodes)
	for _, item := range list.Items {
		child, ok := ecs.Get[htmlComponentInstance](world, item.Entity)
		if !ok {
			continue
		}
		for _, root := range child.Roots {
			if root != nil {
				list.Nodes[root] = true
			}
		}
	}
	for _, item := range items {
		child, ok := ecs.Get[htmlComponentInstance](world, item.Entity)
		if !ok {
			continue
		}
		for _, root := range child.Roots {
			if root != nil {
				list.Nodes[root] = true
			}
		}
	}
	children := slotNode.Children
	remaining := children[:0]
	for _, child := range children {
		if !list.Nodes[child] {
			remaining = append(remaining, child)
		}
	}
	baseLength := len(remaining)
	rootCount := 0
	for _, item := range items {
		if child, ok := ecs.Get[htmlComponentInstance](world, item.Entity); ok {
			rootCount += len(child.Roots)
		}
	}
	needed := baseLength + rootCount
	if cap(remaining) < needed {
		grown := make([]*dom.Node, baseLength, needed)
		copy(grown, remaining)
		remaining = grown
	}
	remaining = remaining[:needed]
	clear(list.Nodes)
	write := baseLength
	for _, item := range items {
		child, ok := ecs.Get[htmlComponentInstance](world, item.Entity)
		if !ok {
			continue
		}
		for _, root := range child.Roots {
			if root == nil {
				continue
			}
			remaining[write] = root
			list.Nodes[root] = true
			root.Parent = slotNode
			write++
		}
	}
	clear(remaining[write:cap(remaining)])
	slotNode.Children = remaining[:write]
	dom.SynchronizeTextContent(slotNode)
	Node{world: world, owner: owner, node: slotNode}.markMutation(htmlMutationStructure)
}

func reconcileHTMLComponentKeyed(world *ecs.World, instance *htmlComponentInstance) {
	for ref, list := range instance.Keyed {
		container := instance.Refs[ref]
		if container == nil || container.Type != dom.ElementNode || list == nil {
			continue
		}
		next := instance.Next.Keyed[ref]
		if !instance.Next.KeyedActive[ref] {
			next = nil
		}
		reconcileHTMLComponentKeyedList(world, instance.Page, container, list, next, instance.Definition.scopeClass)
	}
}

func reconcileHTMLComponentKeyedList(world *ecs.World, owner ecs.Entity, parent *dom.Node, list *htmlComponentKeyedList, next []VNode, scopeClass string) {
	if list.Children == nil {
		list.Children = make(map[*dom.Node]*htmlComponentVNodeChildren)
	}
	if list.Nodes == nil {
		list.Nodes = make(map[*dom.Node]bool)
	}
	// Nodes already contains the managed nodes from the previous commit.
	if list.Anchor == nil {
		for _, child := range parent.Children {
			if !list.Nodes[child] {
				list.Anchor = child
				break
			}
		}
	}
	items, changed := reconcileHTMLComponentVNodeList(world, owner, parent, &list.htmlComponentVNodeChildren, next, list.Children, scopeClass)
	if reconcileHTMLComponentChildren(parent, list, items) {
		changed = true
	}
	old := list.Items
	clear(old)
	list.Items, list.Scratch = items, old[:0]
	if changed {
		dom.SynchronizeTextContent(parent)
		Node{world: world, owner: owner, node: parent}.markMutation(htmlMutationStructure)
	}
}

// reconcileHTMLComponentVNodeList shares key matching, node reuse and removal.
func reconcileHTMLComponentVNodeList(world *ecs.World, owner ecs.Entity, parent *dom.Node, list *htmlComponentVNodeChildren, next []VNode, children map[*dom.Node]*htmlComponentVNodeChildren, scopeClass string) ([]htmlComponentKeyedItem, bool) {
	if list.OldIndex == nil {
		list.OldIndex = make(map[uint64]int)
	}
	if list.Seen == nil {
		list.Seen = make(map[uint64]bool)
	}
	if list.Used == nil {
		list.Used = make(map[uint64]bool)
	}
	clear(list.OldIndex)
	clear(list.Seen)
	clear(list.Used)
	for index, item := range list.Items {
		list.OldIndex[item.Key] = index
	}

	clear(list.Scratch[:cap(list.Scratch)])
	items := list.Scratch[:0]
	for _, vnode := range next {
		tag := strings.ToLower(strings.TrimSpace(vnode.Tag))
		if tag == "" || list.Seen[vnode.Key] {
			continue
		}
		list.Seen[vnode.Key] = true
		var node *dom.Node
		if oldIndex, ok := list.OldIndex[vnode.Key]; ok && oldIndex >= 0 && oldIndex < len(list.Items) {
			candidate := list.Items[oldIndex].Node
			if candidate != nil && candidate.Data == tag {
				node = candidate
				list.Used[vnode.Key] = true
			}
		}
		if node == nil {
			node = dom.NewElement(tag, nil)
		}
		syncHTMLComponentVNodeNode(world, owner, node, vnode, children, scopeClass)
		items = append(items, htmlComponentKeyedItem{Key: vnode.Key, Node: node})
	}

	changed := len(items) != len(list.Items)
	for index, item := range list.Items {
		if !list.Used[item.Key] {
			if item.Node != nil && item.Node.Parent == parent {
				if list.Removed == nil {
					list.Removed = make(map[*dom.Node]struct{})
				}
				list.Removed[item.Node] = struct{}{}
			}
			clearHTMLComponentVNodeChildren(children, item.Node)
			changed = true
			continue
		}
		if index >= len(items) || items[index].Node != item.Node {
			changed = true
		}
	}
	if len(list.Removed) != 0 {
		remaining := parent.Children[:0]
		for _, node := range parent.Children {
			if _, removed := list.Removed[node]; removed {
				node.Parent = nil
			} else {
				remaining = append(remaining, node)
			}
		}
		clear(parent.Children[len(remaining):])
		parent.Children = remaining
		clear(list.Removed)
	}
	return items, changed
}

func syncHTMLComponentVNodeNode(world *ecs.World, owner ecs.Entity, node *dom.Node, vnode VNode, children map[*dom.Node]*htmlComponentVNodeChildren, scopeClass string) {
	if node == nil {
		return
	}
	addHTMLComponentScopeClass(node, scopeClass)
	clearHTMLComponentAttributes(world, owner, node, vnode.Attributes, scopeClass)
	for _, attribute := range vnode.Attributes {
		name := strings.ToLower(strings.TrimSpace(attribute.Name))
		if name == "" {
			continue
		}
		setHTMLComponentVNodeAttribute(world, owner, node, name, attribute.Value, scopeClass)
	}
	if htmlComponentVoidTag(node.Data) {
		if len(node.Children) != 0 {
			Node{world: world, owner: owner, node: node}.Clear()
		}
		clearHTMLComponentVNodeChildren(children, node)
		return
	}
	if len(vnode.Children) == 0 {
		clearHTMLComponentVNodeChildren(children, node)
		Node{world: world, owner: owner, node: node}.SetText(vnode.Text)
		return
	}
	if children[node] == nil {
		children[node] = &htmlComponentVNodeChildren{}
	}
	reconcileHTMLComponentVNodeChildren(world, owner, node, children[node], vnode.Children, children, scopeClass)
}

func reconcileHTMLComponentVNodeChildren(world *ecs.World, owner ecs.Entity, parent *dom.Node, list *htmlComponentVNodeChildren, next []VNode, children map[*dom.Node]*htmlComponentVNodeChildren, scopeClass string) {
	if parent == nil || list == nil {
		return
	}
	items, changed := reconcileHTMLComponentVNodeList(world, owner, parent, list, next, children, scopeClass)
	if len(parent.Children) != len(items) {
		changed = true
	} else {
		for index, item := range items {
			if parent.Children[index] != item.Node {
				changed = true
				break
			}
		}
	}
	if changed {
		children := parent.Children
		clear(children[:cap(children)])
		parent.Children = children[:0]
		for _, item := range items {
			item.Node.Parent = parent
			parent.Children = append(parent.Children, item.Node)
		}
		dom.SynchronizeTextContent(parent)
		Node{world: world, owner: owner, node: parent}.markMutation(htmlMutationStructure)
	}
	old := list.Items
	clear(old)
	list.Items, list.Scratch = items, old[:0]
}

func clearHTMLComponentVNodeChildren(children map[*dom.Node]*htmlComponentVNodeChildren, node *dom.Node) {
	if node == nil || children == nil {
		return
	}
	delete(children, node)
	for _, child := range node.Children {
		clearHTMLComponentVNodeChildren(children, child)
	}
}

func htmlComponentVoidTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func clearHTMLComponentAttributes(world *ecs.World, owner ecs.Entity, node *dom.Node, attributes []VNodeAttribute, scopeClass string) {
	if node == nil || len(node.Attributes) == 0 {
		return
	}
	if len(attributes) == 0 {
		for name := range node.Attributes {
			removeHTMLComponentAttributeWithScope(world, owner, node, name, scopeClass)
		}
		return
	}
	names := ecs.FrameScratchMap[htmlComponentAttributeNamesScratch, string, struct{}](world)
	for _, attribute := range attributes {
		names[strings.ToLower(strings.TrimSpace(attribute.Name))] = struct{}{}
	}
	for name := range node.Attributes {
		if _, found := names[name]; !found {
			removeHTMLComponentAttributeWithScope(world, owner, node, name, scopeClass)
		}
	}
}

func addHTMLComponentScopeClass(node *dom.Node, scopeClass string) {
	prepareHTMLComponentSubtree(node, scopeClass, nil)
}

func prepareHTMLComponentSubtree(node *dom.Node, scopeClass string, refs map[string]*dom.Node) {
	if node == nil {
		return
	}
	if node.Type == dom.ElementNode && scopeClass != "" {
		classValue := node.Attributes["class"]
		if !htmlClassContains(classValue, scopeClass) {
			if classValue == "" {
				classValue = scopeClass
			} else {
				classValue += " " + scopeClass
			}
			if node.Attributes == nil {
				node.Attributes = make(map[string]string, 1)
			}
			node.Attributes["class"] = classValue
		}
	}
	if refs != nil {
		if ref, ok := node.Attr("data-ref"); ok && ref != "" {
			refs[ref] = node
		}
	}
	for _, child := range node.Children {
		prepareHTMLComponentSubtree(child, scopeClass, refs)
	}
}

func setHTMLComponentAttribute(world *ecs.World, instance *htmlComponentInstance, node *dom.Node, name, value string) {
	if instance == nil {
		return
	}
	scopeClass := ""
	if instance.Definition != nil {
		scopeClass = instance.Definition.scopeClass
	}
	setHTMLComponentAttributeWithScope(world, instance.Page, node, name, value, scopeClass)
}

func setHTMLComponentVNodeAttribute(world *ecs.World, owner ecs.Entity, node *dom.Node, name, value, scopeClass string) {
	setHTMLComponentAttributeWithScope(world, owner, node, name, value, scopeClass)
}

func setHTMLComponentAttributeWithScope(world *ecs.World, owner ecs.Entity, node *dom.Node, name, value, scopeClass string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "class" && scopeClass != "" && !htmlClassContains(value, scopeClass) {
		if value == "" {
			value = scopeClass
		} else {
			value += " " + scopeClass
		}
	}
	Node{world: world, owner: owner, node: node}.SetAttribute(name, value)
}

func removeHTMLComponentAttribute(world *ecs.World, instance *htmlComponentInstance, node *dom.Node, name string) {
	if instance == nil {
		return
	}
	scopeClass := ""
	if instance.Definition != nil {
		scopeClass = instance.Definition.scopeClass
	}
	removeHTMLComponentAttributeWithScope(world, instance.Page, node, name, scopeClass)
}

func removeHTMLComponentAttributeWithScope(world *ecs.World, owner ecs.Entity, node *dom.Node, name, scopeClass string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "class" && scopeClass != "" {
		Node{world: world, owner: owner, node: node}.SetAttribute("class", scopeClass)
		return
	}
	Node{world: world, owner: owner, node: node}.RemoveAttribute(name)
}

func htmlClassContains(value, target string) bool {
	for start := 0; start < len(value); {
		for start < len(value) && value[start] <= ' ' {
			start++
		}
		end := start
		for end < len(value) && value[end] > ' ' {
			end++
		}
		if end > start && value[start:end] == target {
			return true
		}
		start = end
	}
	return false
}

func reconcileHTMLComponentChildren(parent *dom.Node, list *htmlComponentKeyedList, items []htmlComponentKeyedItem) bool {
	if parent == nil {
		return false
	}
	children := parent.Children
	remaining := children[:0]
	for _, child := range children {
		if !list.Nodes[child] {
			remaining = append(remaining, child)
		}
	}
	insertAt := len(remaining)
	if list.Anchor != nil {
		for index, child := range remaining {
			if child == list.Anchor {
				insertAt = index
				break
			}
		}
	}
	baseLength := len(remaining)
	needed := baseLength + len(items)
	if cap(remaining) < needed {
		grown := make([]*dom.Node, baseLength, needed)
		copy(grown, remaining)
		remaining = grown
	}
	remaining = remaining[:needed]
	copy(remaining[insertAt+len(items):], remaining[insertAt:baseLength])
	clear(list.Nodes)
	for index, item := range items {
		remaining[insertAt+index] = item.Node
		list.Nodes[item.Node] = true
		item.Node.Parent = parent
	}
	changed := len(children) != len(remaining)
	if !changed {
		for index := range children {
			if children[index] != remaining[index] {
				changed = true
				break
			}
		}
	}
	parent.Children = remaining
	return changed
}

func bindHTMLComponentClick(world *ecs.World, entity ecs.Entity, instance *htmlComponentInstance, ref string, node *dom.Node) {
	if instance.Handlers == nil {
		instance.Handlers = make(map[string]ClickHandler)
	}
	handler := instance.Handlers[ref]
	if handler == nil {
		handler = func(event *PointerEvent) {
			runtime, ok := ecs.Get[htmlComponentInstance](world, entity)
			if !ok || runtime.Definition == nil || runtime.Definition.event == nil {
				return
			}
			runtime.Definition.event(world, entity, ref, Event{
				Entity: runtime.Page, Target: entity, ID: ref, Kind: EventClick,
				Bubbles: true, Cancelable: true,
			}, event, true)
		}
		instance.Handlers[ref] = handler
	}
	Node{world: world, owner: instance.Page, node: node}.OnClick(handler)
}

type htmlComponentPageCSS struct {
	Base   string
	Order  []*htmlDefinitionRuntime
	Values map[*htmlDefinitionRuntime]string
	Counts map[*htmlDefinitionRuntime]int
}

// SetBaseCSS replaces the page stylesheet while preserving mounted component
// styles. Unchanged values reuse the retained stylesheet without allocation.
func SetBaseCSS(world *ecs.World, page ecs.Entity, cssText string) bool {
	surface := ecs.GetMut[Surface](world, page)
	if surface == nil {
		return false
	}
	if state := ecs.GetMut[htmlComponentPageCSS](world, page); state != nil {
		if state.Base == cssText {
			return false
		}
		state.Base = cssText
		_ = rebuildHTMLComponentCSS(world, page, state)
		return true
	}
	if surface.CSS == cssText {
		return false
	}
	surface.CSS = cssText
	return true
}

func attachHTMLComponentCSS(world *ecs.World, page ecs.Entity, definition *htmlDefinitionRuntime, cssText string) error {
	if cssText == "" {
		return nil
	}
	html, ok := ecs.Get[Surface](world, page)
	if !ok {
		return fmt.Errorf("ecs: HTML page %d is missing", page)
	}
	state := ecs.GetMut[htmlComponentPageCSS](world, page)
	if state == nil {
		state = &htmlComponentPageCSS{Base: html.CSS, Values: make(map[*htmlDefinitionRuntime]string), Counts: make(map[*htmlDefinitionRuntime]int)}
		if err := ecs.Insert(world, page, *state); err != nil {
			return err
		}
		state = ecs.GetMut[htmlComponentPageCSS](world, page)
	}
	if state.Counts[definition] == 0 {
		state.Order = append(state.Order, definition)
		state.Values[definition] = cssText
	}
	state.Counts[definition]++
	return rebuildHTMLComponentCSS(world, page, state)
}

func detachHTMLComponentCSS(world *ecs.World, page ecs.Entity, definition *htmlDefinitionRuntime) error {
	state := ecs.GetMut[htmlComponentPageCSS](world, page)
	if state == nil || definition == nil {
		return nil
	}
	if state.Counts[definition] > 1 {
		state.Counts[definition]--
	} else {
		delete(state.Counts, definition)
		delete(state.Values, definition)
		for index, candidate := range state.Order {
			if candidate == definition {
				copy(state.Order[index:], state.Order[index+1:])
				state.Order[len(state.Order)-1] = nil
				state.Order = state.Order[:len(state.Order)-1]
				break
			}
		}
	}
	return rebuildHTMLComponentCSS(world, page, state)
}

func rebuildHTMLComponentCSS(world *ecs.World, page ecs.Entity, state *htmlComponentPageCSS) error {
	if state == nil {
		return nil
	}
	var builder strings.Builder
	builder.Grow(len(state.Base) + 64)
	builder.WriteString(state.Base)
	for _, definition := range state.Order {
		if cssText := state.Values[definition]; cssText != "" {
			if builder.Len() != 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(cssText)
		}
	}
	html := ecs.GetMut[Surface](world, page)
	if html == nil {
		return fmt.Errorf("ecs: HTML page %d is missing", page)
	}
	html.CSS = builder.String()
	return nil
}
