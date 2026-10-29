package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

// ScriptContext is the per-call context passed to a script component.
// Entity is set to the entity currently being processed by the generated
// script system. The context value must not be retained across callbacks.
type ScriptContext struct {
	Scene        *SceneInstance
	World        *ecs.World
	Commands     *ecs.Commands
	Entity       ecs.Entity
	bindingName  string
	inputHandler func(*ScriptContext, *scene.InputEventContext) error
	inputMask    scene.InputMask
	entities     []ecs.Entity
}

// Hooks supplies statically typed lifecycle methods. Only registration creates
// closures; update traversal reuses scene-owned contexts and entity lists.
type Hooks[T any] struct {
	Install   func(*T, *ScriptContext) error
	Update    func(*T, *ScriptContext) error
	Input     func(*T, *ScriptContext, *scene.InputEventContext) error
	Destroy   func(*T, *ScriptContext) error
	InputMask scene.InputMask
}

// Script creates the lifecycle bridge for a concrete component type. Install
// runs after insertion, so its changes affect the component stored in the ECS.
func Script[T any](name string, hooks Hooks[T]) ScriptBinding {
	binding := ScriptBinding{
		Name: name,
		Bind: func(ctx *ScriptContext, spec ScriptComponentSpec) error {
			var value T
			if err := spec.DecodeScriptValues(&value); err != nil {
				return err
			}
			if err := ctx.World.Insert(ctx.Entity, value); err != nil {
				return err
			}
			if hooks.Install != nil {
				if err := hooks.Install(ctx.World.MustMut[T](ctx.Entity), ctx); err != nil {
					ctx.World.Remove[T](ctx.Entity)
					return err
				}
			}
			return nil
		},
		Remove: func(ctx *ScriptContext) error {
			ctx.World.Remove[T](ctx.Entity)
			return nil
		},
		InputMask: hooks.InputMask,
	}
	if hooks.Update != nil {
		binding.Update = func(ctx *ScriptContext) error {
			for entity, access := range ctx.World.Query[T]().In(ctx.entities).All {
				ctx.Entity = entity
				if err := hooks.Update(access.Mut(), ctx); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if hooks.Input != nil {
		binding.Input = func(ctx *ScriptContext, event *scene.InputEventContext) error {
			if value := ctx.World.GetMut[T](ctx.Entity); value != nil {
				return hooks.Input(value, ctx, event)
			}
			return nil
		}
	}
	if hooks.Destroy != nil {
		binding.Destroy = func(ctx *ScriptContext) error {
			if value := ctx.World.GetMut[T](ctx.Entity); value != nil {
				return hooks.Destroy(value, ctx)
			}
			return nil
		}
	}
	return binding
}

// ListenInput routes input events from target to the script currently
// being installed. Registrations are removed with the owning script.
func (context *ScriptContext) ListenInput(target ecs.Entity) error {
	if context == nil || context.Scene == nil || context.World == nil {
		return fmt.Errorf("script context is nil")
	}
	if context.bindingName == "" || context.inputHandler == nil {
		return fmt.Errorf("script does not implement OnInput")
	}
	if !context.inputMask.Intersects(scene.InputMaskPointer) {
		return fmt.Errorf("script input mask does not include pointer events")
	}
	if target == 0 || !context.World.IsAlive(target) {
		return fmt.Errorf("input target %d is unavailable", target)
	}
	context.Scene.addScriptInputBinding(target, context.Entity, context.bindingName, context.inputHandler)
	return nil
}

// ScriptBinding is the lifecycle bridge produced by Script for a component type.
// Bind installs the typed component and invokes its install hook. Update,
// Input and Destroy are optional. Remove clears the typed component when a
// binding is replaced while a scene remains alive.
type ScriptBinding struct {
	Name   string
	Bind   func(*ScriptContext, ScriptComponentSpec) error
	Update func(*ScriptContext) error
	Input  func(*ScriptContext, *scene.InputEventContext) error
	// InputMask limits which input categories reach Input. Zero subscribes to all categories.
	InputMask scene.InputMask
	Destroy   func(*ScriptContext) error
	Remove    func(*ScriptContext) error
}

type scriptSystem struct {
	binding  ScriptBinding
	entities []ecs.Entity
}

type scriptInputBinding struct {
	owner  ecs.Entity
	name   string
	handle func(*ScriptContext, *scene.InputEventContext) error
}

type scriptGlobalInputBindings struct {
	keyboard []scriptInputBinding
	text     []scriptInputBinding
	files    []scriptInputBinding
}

func (bindings *scriptGlobalInputBindings) add(mask scene.InputMask, binding scriptInputBinding) {
	if mask.Intersects(scene.InputMaskKeyboard) {
		bindings.keyboard = insertScriptInputBinding(bindings.keyboard, binding)
	}
	if mask.Intersects(scene.InputMaskText) {
		bindings.text = insertScriptInputBinding(bindings.text, binding)
	}
	if mask.Intersects(scene.InputMaskFiles) {
		bindings.files = insertScriptInputBinding(bindings.files, binding)
	}
}

func (bindings *scriptGlobalInputBindings) remove(owner ecs.Entity, name string) {
	bindings.keyboard = removeScriptInputBindings(bindings.keyboard, owner, name)
	bindings.text = removeScriptInputBindings(bindings.text, owner, name)
	bindings.files = removeScriptInputBindings(bindings.files, owner, name)
}

func (bindings *scriptGlobalInputBindings) forKind(kind scene.InputEventKind) []scriptInputBinding {
	switch kind.Mask() {
	case scene.InputMaskKeyboard:
		return bindings.keyboard
	case scene.InputMaskText:
		return bindings.text
	case scene.InputMaskFiles:
		return bindings.files
	default:
		return nil
	}
}

func (bindings *scriptGlobalInputBindings) clear() {
	clear(bindings.keyboard)
	clear(bindings.text)
	clear(bindings.files)
	bindings.keyboard = bindings.keyboard[:0]
	bindings.text = bindings.text[:0]
	bindings.files = bindings.files[:0]
}

// ScriptRegistry contains generated script bindings in deterministic order.
type ScriptRegistry struct {
	bindings []ScriptBinding
	byName   map[string]int
}

// NewScriptRegistry creates a registry from generated bindings.
func NewScriptRegistry(bindings ...ScriptBinding) ScriptRegistry {
	registry := ScriptRegistry{
		bindings: make([]ScriptBinding, 0, len(bindings)),
		byName:   make(map[string]int, len(bindings)),
	}
	for _, binding := range bindings {
		name := strings.TrimSpace(binding.Name)
		if name == "" {
			panic("project: script binding name is required")
		}
		if binding.Bind == nil {
			panic(fmt.Sprintf("project: script binding %q has nil bind function", name))
		}
		if _, exists := registry.byName[name]; exists {
			panic(fmt.Sprintf("project: duplicate script binding %q", name))
		}
		binding.Name = name
		if binding.Input == nil {
			binding.InputMask = 0
		} else if binding.InputMask == 0 {
			binding.InputMask = scene.InputMaskAll
		} else if binding.InputMask&^scene.InputMaskAll != 0 {
			panic(fmt.Sprintf("project: script binding %q has invalid input mask %d", name, binding.InputMask))
		}
		registry.byName[name] = len(registry.bindings)
		registry.bindings = append(registry.bindings, binding)
	}
	return registry
}

func (r ScriptRegistry) empty() bool {
	return len(r.bindings) == 0
}

// Empty reports whether the registry contains no generated script bindings.
func (r ScriptRegistry) Empty() bool {
	return r.empty()
}

// InputMask reports the input categories handled by registered scripts.
func (r ScriptRegistry) InputMask() scene.InputMask {
	var mask scene.InputMask
	for _, binding := range r.bindings {
		mask |= binding.InputMask
	}
	return mask
}

func (r ScriptRegistry) binding(name string) (ScriptBinding, bool) {
	index, ok := r.byName[strings.TrimSpace(name)]
	if !ok {
		return ScriptBinding{}, false
	}
	return r.bindings[index], true
}

// DecodeScriptValues applies editor-authored values to a typed script value.
// This is only used while a scene is instantiated; generated update paths do
// not use JSON or reflection.
func (spec ScriptComponentSpec) DecodeScriptValues[T any](target *T) error {
	if target == nil {
		return fmt.Errorf("script %q target is nil", spec.Name)
	}
	if len(spec.Values) == 0 {
		return nil
	}
	var data bytes.Buffer
	data.WriteByte('{')
	first := true
	for name, raw := range spec.Values {
		if strings.TrimSpace(name) == "" || len(raw) == 0 {
			continue
		}
		if !first {
			data.WriteByte(',')
		}
		first = false
		key, _ := json.Marshal(name)
		data.Write(key)
		data.WriteByte(':')
		data.Write(raw)
	}
	data.WriteByte('}')
	if first {
		return nil
	}
	if err := json.Unmarshal(data.Bytes(), target); err != nil {
		return fmt.Errorf("decode script %q values: %w", spec.Name, err)
	}
	return nil
}

func scriptComponentsEqual(left, right []ScriptComponentSpec) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Name != right[index].Name || len(left[index].Values) != len(right[index].Values) {
			return false
		}
		for name, value := range left[index].Values {
			if !bytes.Equal(value, right[index].Values[name]) {
				return false
			}
		}
	}
	return true
}

// SetScriptRegistry installs the registry used by this scene and binds all
// currently authored script components. Calling it again replaces bindings
// using the new registry. A binding error leaves the scene unbound.
func (s *SceneInstance) SetScriptRegistry(registry ScriptRegistry) error {
	if s == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	if s.scriptsBound {
		s.unbindAllScripts()
		s.scriptsBound = false
		s.scriptSystems = nil
	}
	s.scriptRegistry = registry
	return s.BindScripts()
}

// BindScripts binds the scene's authored script components. It is idempotent
// for one scene generation and is also used after a scene hierarchy rebuild.
func (s *SceneInstance) BindScripts() error {
	if s == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	if s.scriptRegistry.empty() || s.scriptsBound {
		return nil
	}
	if s.scriptBindings == nil {
		s.scriptBindings = make(map[ecs.Entity][]ScriptBinding)
	}
	if s.inputBindings == nil {
		s.inputBindings = make(map[ecs.Entity][]scriptInputBinding)
	}
	// Order by entity handle; map iteration must not decide lifecycle order.
	entities := make([]ecs.Entity, 0, len(s.nodes))
	for entity := range s.nodes {
		entities = append(entities, entity)
	}
	slices.Sort(entities)
	for _, entity := range entities {
		node := s.nodes[entity]
		if err := s.bindNodeScripts(entity, node.Spec); err != nil {
			s.unbindAllScripts()
			s.scriptSystems = nil
			return err
		}
	}
	s.rebuildScriptSystems()
	s.scriptsBound = true
	return nil
}

func (s *SceneInstance) bindNodeScripts(entity ecs.Entity, spec SceneNodeSpec) error {
	if len(spec.Scripts) == 0 || s.scriptRegistry.empty() {
		return nil
	}
	ctx := &ScriptContext{Scene: s, World: s.World, Commands: s.World.Commands(), Entity: entity}
	for _, script := range spec.Scripts {
		binding, ok := s.scriptRegistry.binding(script.Name)
		if !ok {
			return fmt.Errorf("scene node %q references unregistered script %q", spec.ID, script.Name)
		}
		ctx.bindingName = binding.Name
		ctx.inputHandler = binding.Input
		ctx.inputMask = binding.InputMask
		if err := binding.Bind(ctx, script); err != nil {
			s.removeScriptInputBinding(entity, binding.Name)
			ctx.bindingName = ""
			ctx.inputHandler = nil
			ctx.inputMask = 0
			return fmt.Errorf("bind script %q on node %q: %w", binding.Name, spec.ID, err)
		}
		ctx.bindingName = ""
		ctx.inputHandler = nil
		ctx.inputMask = 0
		if binding.Input != nil {
			if binding.InputMask.Intersects(scene.InputMaskPointer) {
				s.addScriptInputBinding(entity, entity, binding.Name, binding.Input)
			}
			s.addGlobalScriptInputBinding(entity, binding.Name, binding.Input, binding.InputMask)
		}
		s.scriptBindings[entity] = append(s.scriptBindings[entity], binding)
		s.scriptSystemsDirty = true
	}
	return nil
}

func (s *SceneInstance) addScriptInputBinding(
	target, owner ecs.Entity,
	name string,
	handle func(*ScriptContext, *scene.InputEventContext) error,
) {
	if s == nil || target == 0 || owner == 0 || name == "" || handle == nil {
		return
	}
	if s.inputBindings == nil {
		s.inputBindings = make(map[ecs.Entity][]scriptInputBinding)
	}
	bindings := s.inputBindings[target]
	for _, current := range bindings {
		if current.owner == owner && current.name == name {
			return
		}
	}
	s.inputBindings[target] = append(bindings, scriptInputBinding{owner: owner, name: name, handle: handle})
}

func insertScriptInputBinding(bindings []scriptInputBinding, binding scriptInputBinding) []scriptInputBinding {
	index := len(bindings)
	for currentIndex, current := range bindings {
		if current.owner == binding.owner && current.name == binding.name {
			return bindings
		}
		if binding.owner < current.owner || binding.owner == current.owner && binding.name < current.name {
			index = currentIndex
			break
		}
	}
	bindings = append(bindings, scriptInputBinding{})
	copy(bindings[index+1:], bindings[index:])
	bindings[index] = binding
	return bindings
}

func removeScriptInputBindings(bindings []scriptInputBinding, owner ecs.Entity, name string) []scriptInputBinding {
	kept := bindings[:0]
	for _, binding := range bindings {
		if binding.owner != owner || binding.name != name {
			kept = append(kept, binding)
		}
	}
	clear(bindings[len(kept):])
	return kept
}

func (s *SceneInstance) addGlobalScriptInputBinding(owner ecs.Entity, name string, handle func(*ScriptContext, *scene.InputEventContext) error, mask scene.InputMask) {
	if s == nil || owner == 0 || name == "" || handle == nil {
		return
	}
	binding := scriptInputBinding{owner: owner, name: name, handle: handle}
	s.globalInputBindings.add(mask, binding)
}

func (s *SceneInstance) removeScriptInputBinding(owner ecs.Entity, name string) {
	if s == nil {
		return
	}
	for target, bindings := range s.inputBindings {
		kept := bindings[:0]
		for _, binding := range bindings {
			if binding.owner != owner || binding.name != name {
				kept = append(kept, binding)
			}
		}
		if len(kept) == 0 {
			delete(s.inputBindings, target)
			continue
		}
		s.inputBindings[target] = kept
	}
	s.globalInputBindings.remove(owner, name)
}

func (s *SceneInstance) removeScriptInputTarget(target ecs.Entity) {
	if s != nil && s.inputBindings != nil {
		delete(s.inputBindings, target)
	}
}

// DispatchInputScripts delivers global input to every bound input script and
// routes pointer input only to the event target or its bubbling path.
func (s *SceneInstance) DispatchInputScripts(context *ecs.SystemContext, reader *ecs.MessageReader[scene.InputEvent]) error {
	if s == nil || context == nil || context.World == nil || reader == nil {
		return nil
	}
	for event := range reader.All(context.World) {
		if event.Handled {
			continue
		}
		if event.Entity == 0 {
			_, err := s.dispatchGlobalInput(context, event)
			if event.Handled && event.Seq != 0 {
				if state, ok := context.World.GetResource[*input.State](); ok && state != nil {
					state.ConsumeEvent(event.Seq)
				}
			}
			if err != nil {
				return err
			}
			continue
		}
		targets := event.Path
		if len(targets) == 0 {
			targets = []ecs.Entity{event.Entity}
		}
		for _, target := range targets {
			handled, err := s.dispatchInputAt(context, event, target)
			if err != nil {
				return err
			}
			if handled {
				break
			}
		}
	}
	return nil
}

func (s *SceneInstance) dispatchGlobalInput(context *ecs.SystemContext, event *scene.InputEvent) (bool, error) {
	scriptContext := &s.inputScriptContext
	*scriptContext = ScriptContext{Scene: s, World: context.World, Commands: context.Commands}
	inputContext := &s.inputEventContext
	*inputContext = scene.InputEventContext{Event: event}
	for _, route := range s.globalInputBindings.forKind(event.Kind) {
		if !context.World.IsAlive(route.owner) {
			continue
		}
		scriptContext.Entity = route.owner
		inputContext.Target, inputContext.Current = route.owner, route.owner
		if err := route.handle(scriptContext, inputContext); err != nil {
			return false, fmt.Errorf("input script %q: %w", route.name, err)
		}
		if inputContext.Handled || event.Handled {
			event.Handled = true
			return true, nil
		}
	}
	return false, nil
}

func (s *SceneInstance) dispatchInputAt(context *ecs.SystemContext, event *scene.InputEvent, target ecs.Entity) (bool, error) {
	filter := scene.InputFilterPass
	if pointerTarget, ok := context.World.Get[scene.InputTarget2D](target); ok {
		filter = pointerTarget.InputFilter
	}
	if filter == scene.InputFilterIgnore {
		return false, nil
	}
	bindings := s.inputBindings[target]
	if len(bindings) == 0 {
		return filter == scene.InputFilterStop, nil
	}
	scriptContext := &s.inputScriptContext
	*scriptContext = ScriptContext{Scene: s, World: context.World, Commands: context.Commands}
	inputContext := &s.inputEventContext
	*inputContext = scene.InputEventContext{Event: event, Target: event.Entity, Current: target}
	for _, route := range bindings {
		if !context.World.IsAlive(route.owner) {
			continue
		}
		scriptContext.Entity = route.owner
		if err := route.handle(scriptContext, inputContext); err != nil {
			return false, fmt.Errorf("input script %q: %w", route.name, err)
		}
		if inputContext.Handled || event.Handled {
			event.Handled = true
			return true, nil
		}
	}
	return filter == scene.InputFilterStop, nil
}

// UpdateScripts runs script types in registry order and their bound entities in
// ascending handle order. Typed queries visit only this scene's entity lists;
// the hot loop uses no string lookup and reuses contexts at each call depth.
func (s *SceneInstance) UpdateScripts(context *ecs.SystemContext) error {
	if s == nil || context == nil || context.World == nil {
		return nil
	}
	if context.World != s.World {
		return fmt.Errorf("script update context belongs to a different world")
	}
	if s.scriptSystemsDirty {
		s.rebuildScriptSystems()
	}
	if len(s.scriptSystems) == 0 {
		return nil
	}
	// Keep stable context addresses when nested updates grow the arena.
	depth := s.scriptUpdateDepth
	if depth == len(s.scriptUpdateContexts) {
		s.scriptUpdateContexts = append(s.scriptUpdateContexts, new(ScriptContext))
	}
	base := s.scriptUpdateContexts[depth]
	*base = ScriptContext{Scene: s, World: s.World, Commands: context.Commands}
	s.scriptUpdateDepth++
	defer func() { s.scriptUpdateDepth--; *base = ScriptContext{} }()
	for _, system := range s.scriptSystems {
		binding := system.binding
		if binding.Update == nil || len(system.entities) == 0 {
			continue
		}
		base.entities = system.entities
		if err := binding.Update(base); err != nil {
			return fmt.Errorf("update script %q: %w", binding.Name, err)
		}
	}
	return nil
}

func (s *SceneInstance) destroyScripts(entity ecs.Entity) {
	s.removeScriptInputTarget(entity)
	s.unbindScripts(entity, false)
}

func (s *SceneInstance) unbindAllScripts() {
	entities := make([]ecs.Entity, 0, len(s.scriptBindings))
	for entity := range s.scriptBindings {
		entities = append(entities, entity)
	}
	slices.Sort(entities)
	for _, entity := range entities {
		s.unbindScripts(entity, true)
	}
}

func (s *SceneInstance) rebuildScriptSystems() {
	count := len(s.scriptRegistry.bindings)
	if cap(s.scriptSystems) < count {
		s.scriptSystems = make([]scriptSystem, count)
	} else {
		s.scriptSystems = s.scriptSystems[:count]
	}
	for index, binding := range s.scriptRegistry.bindings {
		s.scriptSystems[index].binding = binding
		s.scriptSystems[index].entities = s.scriptSystems[index].entities[:0]
	}
	for entity, bindings := range s.scriptBindings {
		for _, binding := range bindings {
			system := &s.scriptSystems[s.scriptRegistry.byName[binding.Name]]
			system.entities = append(system.entities, entity)
		}
	}
	for index := range s.scriptSystems {
		slices.Sort(s.scriptSystems[index].entities)
	}
	s.scriptSystemsDirty = false
}

func (s *SceneInstance) unbindScripts(entity ecs.Entity, remove bool) {
	bindings := s.scriptBindings[entity]
	if len(bindings) == 0 {
		return
	}
	ctx := &ScriptContext{Scene: s, World: s.World, Commands: s.World.Commands(), Entity: entity}
	for _, binding := range bindings {
		if binding.Destroy != nil {
			_ = binding.Destroy(ctx)
		}
		if remove && binding.Remove != nil {
			_ = binding.Remove(ctx)
		}
		s.removeScriptInputBinding(entity, binding.Name)
	}
	delete(s.scriptBindings, entity)
	s.scriptSystemsDirty = true
}
