package project

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	enginephysics "github.com/diiyw/onix/physics"
	sceneapi "github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/diiyw/onix/spine"
	engineui "github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

type SceneInstance struct {
	World                *ecs.World
	project              *Bundle
	sceneID              SceneID
	nodes                map[ecs.Entity]sceneSpawnNode
	scriptRegistry       ScriptRegistry
	scriptBindings       map[ecs.Entity][]ScriptBinding
	inputBindings        map[ecs.Entity][]scriptInputBinding
	globalInputBindings  scriptGlobalInputBindings
	inputScriptContext   ScriptContext
	inputEventContext    sceneapi.InputEventContext
	scriptSystems        []scriptSystem
	scriptSystemsDirty   bool
	scriptUpdateContexts []*ScriptContext
	scriptUpdateDepth    int
	scriptsBound         bool
	Entities             map[string]ecs.Entity
	EntityIDs            map[ecs.Entity]string
	Roots                []ecs.Entity
}

type SceneRenderItem struct {
	Entity    ecs.Entity
	UI        bool
	SortIndex int
	Depth     float32
	Sequence  uint64
	Material  uint64
}

// sceneUISync keeps optional UI operations behind the selected load profile.
// The render-only profiles do not link layout or control composition code.
type sceneUISync struct {
	beforeSync   func(*ecs.World) error
	afterSync    func(*ecs.World) error
	layout       func(*ecs.World) error
	unbindPart   func(*ecs.World, ecs.Entity)
	widgetTarget func(*ecs.World, ecs.Entity, ecs.Entity) error
}

func fullSceneUISync() sceneUISync {
	return sceneUISync{
		layout:       engineui.UpdateLayout,
		unbindPart:   engineui.UnbindControlCompositionPart,
		widgetTarget: resolveSceneWidgetTarget,
	}
}

func resolveSceneWidgetTarget(world *ecs.World, entity, target ecs.Entity) error {
	widget := ecs.GetMut[engineui.Widget](world, entity)
	if widget == nil {
		return fmt.Errorf("widget target requires a Widget component")
	}
	widget.Target = target
	return nil
}

func (s *SceneInstance) unbindControlPart(entity ecs.Entity) {
	if s.project != nil && s.project.uiSync.unbindPart != nil {
		s.project.uiSync.unbindPart(s.World, entity)
	}
}

func (b *Bundle) InstantiateEntryScene(world *ecs.World) (*SceneInstance, error) {
	if b == nil {
		return nil, fmt.Errorf("project bundle is nil")
	}
	return b.InstantiateScene(world, b.entrySceneID)
}

// PreloadScene builds one registered scene in an isolated world so
// its decoded resources, compiled animations, and static glyphs are cached.
func (b *Bundle) PreloadScene(id SceneID) error {
	scene, err := b.InstantiateScene(ecs.NewWorld(), id)
	if err != nil {
		return fmt.Errorf("preload scene %q: %w", id, err)
	}
	defer scene.dispose(false)

	for _, access := range ecs.Query[sceneapi.Text2D](scene.World).All {
		text := access.Mut()

		sceneapi.MeasureText2D(text)
	}
	return nil
}

// PreloadScenes preloads registered scenes in order.
func (b *Bundle) PreloadScenes(ids ...SceneID) error {
	for _, id := range ids {
		if err := b.PreloadScene(id); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bundle) InstantiateScene(world *ecs.World, id SceneID) (*SceneInstance, error) {
	scene, plan, err := b.newSceneInstance(world, id)
	if err != nil {
		return nil, err
	}
	entities := make([]ecs.Entity, len(plan.Nodes))
	for index := range plan.Nodes {
		if err := b.spawnScenePlanNode(scene, plan, entities, index); err != nil {
			scene.Dispose()
			return nil, err
		}
	}
	for index := range plan.Nodes {
		if err := b.resolveScenePlanNode(scene, plan, entities, index); err != nil {
			scene.Dispose()
			return nil, err
		}
	}
	if err := bindSceneAnimationTargets(scene.World, plan, entities); err != nil {
		scene.Dispose()
		return nil, err
	}
	if err := scene.RefreshTransforms(); err != nil {
		scene.Dispose()
		return nil, err
	}
	return scene, nil
}

func (b *Bundle) newSceneInstance(world *ecs.World, id SceneID) (*SceneInstance, sceneSpawnPlan, error) {
	if world == nil {
		return nil, sceneSpawnPlan{}, fmt.Errorf("ecs world is nil")
	}
	if b == nil {
		return nil, sceneSpawnPlan{}, fmt.Errorf("project bundle is nil")
	}
	id = SceneID(strings.TrimSpace(string(id)))
	plan, ok := b.scenePlansByID[id]
	if !ok {
		return nil, sceneSpawnPlan{}, fmt.Errorf("scene %q is not registered by %s", id, sceneCatalogFileName)
	}
	if clr, ok := b.ClearColor(); ok {
		ecs.InsertResource(world, sceneapi.RenderSettings2D{ClearColor: clr})
	}

	scene := &SceneInstance{
		World:          world,
		project:        b,
		sceneID:        id,
		nodes:          make(map[ecs.Entity]sceneSpawnNode, len(plan.Nodes)),
		scriptBindings: make(map[ecs.Entity][]ScriptBinding),
		inputBindings:  make(map[ecs.Entity][]scriptInputBinding),
		Entities:       make(map[string]ecs.Entity, len(plan.Nodes)),
		EntityIDs:      make(map[ecs.Entity]string, len(plan.Nodes)),
		Roots:          make([]ecs.Entity, 0, len(plan.Nodes)),
	}
	return scene, plan, nil
}

func (b *Bundle) spawnScenePlanNode(scene *SceneInstance, plan sceneSpawnPlan, entities []ecs.Entity, index int) error {
	node := plan.Nodes[index]
	var parent ecs.Entity
	if node.Parent > 0 {
		parent = entities[node.Parent-1]
	}
	entity, err := b.spawnECSPlanNode(scene.World, node, parent, scene.Entities, scene.EntityIDs)
	if err != nil {
		return err
	}
	entities[index] = entity
	scene.nodes[entity] = node
	if node.Parent == 0 {
		scene.Roots = append(scene.Roots, entity)
	}
	return nil
}

func (b *Bundle) resolveScenePlanNode(scene *SceneInstance, plan sceneSpawnPlan, entities []ecs.Entity, index int) error {
	node := plan.Nodes[index]
	if node.TargetCamera != 0 {
		if err := ecs.Insert(scene.World, entities[index], sceneapi.UITargetCamera{Entity: entities[node.TargetCamera-1]}); err != nil {
			return fmt.Errorf("game object %q targetCamera: %w", node.Spec.ID, err)
		}
	}
	if node.WidgetTarget != 0 {
		if b.uiSync.widgetTarget == nil {
			return fmt.Errorf("game object %q widget target is unavailable in the selected profile", node.Spec.ID)
		}
		if err := b.uiSync.widgetTarget(scene.World, entities[index], entities[node.WidgetTarget-1]); err != nil {
			return fmt.Errorf("game object %q: %w", node.Spec.ID, err)
		}
	}
	return nil
}

func (s *SceneInstance) SceneID() SceneID {
	if s == nil {
		return ""
	}
	return s.sceneID
}

// Resources returns the resources owned by the scene's project bundle.
func (s *SceneInstance) Resources() *Resources {
	if s == nil || s.project == nil {
		return nil
	}
	return s.project.Resources()
}

// CloneChildrenUnder clones the authored children of a scene node under an
// existing runtime entity. The cloned nodes get a unique ID prefix and remain
// part of the scene so their authored UI composition can be bound normally.
func (s *SceneInstance) CloneChildrenUnder(parent ecs.Entity, templateID, prefix string) (map[string]ecs.Entity, error) {
	if s == nil || s.World == nil || s.project == nil {
		return nil, fmt.Errorf("scene instance is nil")
	}
	if !s.World.IsAlive(parent) {
		return nil, fmt.Errorf("clone parent entity %d does not exist", parent)
	}
	template, ok := s.Entities[strings.TrimSpace(templateID)]
	if !ok {
		return nil, fmt.Errorf("scene template %q does not exist", templateID)
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "-")
	if prefix == "" {
		return nil, fmt.Errorf("scene template prefix is required")
	}
	clones := make(map[string]ecs.Entity)
	rollback := func(err error) (map[string]ecs.Entity, error) {
		s.rollbackClones(clones)
		return nil, err
	}
	children := sceneChildIndex(s.World)
	for _, child := range children[template] {
		if err := s.cloneTemplateNode(parent, child, prefix, clones, children); err != nil {
			return rollback(err)
		}
	}
	if err := s.resolveCloneWidgetTargets(clones); err != nil {
		return rollback(err)
	}
	if err := s.bindRuntimeAnimationTargets(cloneEntities(clones)...); err != nil {
		return rollback(err)
	}
	if err := s.RefreshTransforms(); err != nil {
		return rollback(err)
	}
	return clones, nil
}

// CloneNodeUnder clones one authored scene node and its descendants under an
// existing entity. It is useful for data-driven UI that keeps one editable
// item template in the scene and creates the remaining items at runtime.
func (s *SceneInstance) CloneNodeUnder(parent ecs.Entity, templateID, prefix string) (map[string]ecs.Entity, error) {
	if s == nil || s.World == nil || s.project == nil {
		return nil, fmt.Errorf("scene instance is nil")
	}
	if !s.World.IsAlive(parent) {
		return nil, fmt.Errorf("clone parent entity %d does not exist", parent)
	}
	template, ok := s.Entities[strings.TrimSpace(templateID)]
	if !ok {
		return nil, fmt.Errorf("scene template %q does not exist", templateID)
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "-")
	if prefix == "" {
		return nil, fmt.Errorf("scene template prefix is required")
	}
	clones := make(map[string]ecs.Entity)
	rollback := func(err error) (map[string]ecs.Entity, error) {
		s.rollbackClones(clones)
		return nil, err
	}
	if err := s.cloneTemplateNode(parent, template, prefix, clones, sceneChildIndex(s.World)); err != nil {
		return rollback(err)
	}
	if err := s.resolveCloneWidgetTargets(clones); err != nil {
		return rollback(err)
	}
	if err := s.bindRuntimeAnimationTargets(cloneEntities(clones)...); err != nil {
		return rollback(err)
	}
	if err := s.RefreshTransforms(); err != nil {
		return rollback(err)
	}
	return clones, nil
}

// CloneRootsUnder clones all authored root nodes under an existing runtime
// entity. The cloned nodes get a unique ID prefix and remain part of the
// scene so their authored UI composition can be bound normally.
func (s *SceneInstance) CloneRootsUnder(parent ecs.Entity, prefix string) (map[string]ecs.Entity, error) {
	if s == nil || s.World == nil || s.project == nil {
		return nil, fmt.Errorf("scene instance is nil")
	}
	if !s.World.IsAlive(parent) {
		return nil, fmt.Errorf("clone parent entity %d does not exist", parent)
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "-")
	if prefix == "" {
		return nil, fmt.Errorf("scene template prefix is required")
	}
	clones := make(map[string]ecs.Entity)
	rollback := func(err error) (map[string]ecs.Entity, error) {
		s.rollbackClones(clones)
		return nil, err
	}
	children := sceneChildIndex(s.World)
	for _, root := range s.Roots {
		if err := s.cloneTemplateNode(parent, root, prefix, clones, children); err != nil {
			return rollback(err)
		}
	}
	if err := s.resolveCloneWidgetTargets(clones); err != nil {
		return rollback(err)
	}
	if err := s.bindRuntimeAnimationTargets(cloneEntities(clones)...); err != nil {
		return rollback(err)
	}
	if err := s.RefreshTransforms(); err != nil {
		return rollback(err)
	}
	return clones, nil
}

func cloneEntities(clones map[string]ecs.Entity) []ecs.Entity {
	entities := make([]ecs.Entity, 0, len(clones))
	for _, entity := range clones {
		entities = append(entities, entity)
	}
	return entities
}

func (s *SceneInstance) rollbackClones(clones map[string]ecs.Entity) {
	if s == nil || len(clones) == 0 {
		return
	}
	ids := make([]string, 0, len(clones))
	for id := range clones {
		ids = append(ids, id)
	}
	s.removeNodes(false, ids...)
}

func sceneChildIndex(world *ecs.World) map[ecs.Entity][]ecs.Entity {
	children := make(map[ecs.Entity][]ecs.Entity)
	for entity, access := range ecs.Query[sceneapi.Parent](world).All {
		parent := access.Ref().Entity
		children[parent] = append(children[parent], entity)
	}
	for _, siblings := range children {
		slices.Sort(siblings)
	}
	return children
}

func sceneChildCount(world *ecs.World, parent ecs.Entity) int {
	count := 0
	for _, access := range ecs.Query[sceneapi.Parent](world).All {
		if access.Ref().Entity == parent {
			count++
		}
	}
	return count
}

func (s *SceneInstance) cloneTemplateNode(parent, source ecs.Entity, prefix string, clones map[string]ecs.Entity, children map[ecs.Entity][]ecs.Entity) error {
	node, ok := s.nodes[source]
	if !ok {
		return fmt.Errorf("scene template entity %d is not authored", source)
	}
	clone := node
	clone.Spec.ID = prefix + "-" + s.ID(source)
	clone.Spec.Children = nil
	if clone.Spec.Widget != nil {
		widget := *clone.Spec.Widget
		clone.Spec.Widget = &widget
		if targetID := strings.TrimSpace(clone.Spec.Widget.TargetID); targetID != "" {
			if target, exists := s.Entities[targetID]; exists && sceneEntityIsUnder(s.World, target, source) {
				clone.Spec.Widget.TargetID = prefix + "-" + targetID
			}
		}
	}
	components, err := s.project.ecsComponentsForPlanNode(clone)
	if err != nil {
		return err
	}
	ecs.Put(&components, sceneapi.Parent{Entity: parent})
	entity := s.World.Spawn(components)
	cleanup := func() {
		s.destroyScripts(entity)
		s.rebuildScriptSystems()
		s.unbindControlPart(entity)
		id := s.EntityIDs[entity]
		delete(s.nodes, entity)
		delete(s.EntityIDs, entity)
		if id != "" {
			delete(s.Entities, id)
		}
		s.World.Despawn(entity)
	}
	if clone.Spec.BlockInput && (clone.Spec.Components.Has2D() || clone.Spec.Control != nil) {
		_ = ecs.Insert(s.World, entity, sceneapi.InputBlocker2D{
			Width: float32(clone.Spec.Width), Height: float32(clone.Spec.Height), PassThrough: clone.Spec.PassThrough,
		})
	}
	if err := indexSceneEntity(clone.Spec, entity, s.Entities, s.EntityIDs); err != nil {
		cleanup()
		return err
	}
	s.nodes[entity] = clone
	if err := s.bindNodeScripts(entity, clone.Spec); err != nil {
		cleanup()
		return err
	}
	clones[clone.Spec.ID] = entity
	for _, child := range children[source] {
		if err := s.cloneTemplateNode(entity, child, prefix, clones, children); err != nil {
			return err
		}
	}
	return nil
}

func (s *SceneInstance) resolveCloneWidgetTargets(clones map[string]ecs.Entity) error {
	for _, entity := range clones {
		node, ok := s.nodes[entity]
		if !ok {
			return fmt.Errorf("cloned entity %d has no authored node", entity)
		}
		if err := resolveWidgetTarget(s.World, entity, node.Spec, s.Entities); err != nil {
			return err
		}
	}
	return nil
}

func sceneEntityIsUnder(world *ecs.World, entity, root ecs.Entity) bool {
	for remaining := world.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if entity == root {
			return true
		}
		parent, ok := ecs.Get[sceneapi.Parent](world, entity)
		if !ok {
			return false
		}
		entity = parent.Entity
	}
	return false
}

func (s *SceneInstance) Switch(id SceneID) error {
	if s == nil || s.project == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	next, err := s.project.InstantiateScene(s.World, id)
	if err != nil {
		return err
	}
	next.scriptRegistry = s.scriptRegistry
	if err := next.BindScripts(); err != nil {
		next.Dispose()
		return err
	}
	s.dispose(false)
	*s = *next
	return nil
}

// ReplaceNodes replaces the authored scene hierarchy while retaining the
// scene world and loaded project resources.
func (s *SceneInstance) ReplaceNodes(specs []SceneNodeSpec) error {
	if s == nil || s.project == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	plan, err := compileSceneSpawnPlan(specs)
	if err != nil {
		return err
	}

	s.dispose(false)
	s.nodes = make(map[ecs.Entity]sceneSpawnNode, len(plan.Nodes))
	s.scriptBindings = make(map[ecs.Entity][]ScriptBinding)
	s.inputBindings = make(map[ecs.Entity][]scriptInputBinding)
	s.globalInputBindings.clear()
	s.scriptSystems = nil
	s.scriptsBound = false
	s.Entities = make(map[string]ecs.Entity, len(plan.Nodes))
	s.EntityIDs = make(map[ecs.Entity]string, len(plan.Nodes))
	s.Roots = make([]ecs.Entity, 0, len(plan.Nodes))
	entities := make([]ecs.Entity, len(plan.Nodes))
	for index, node := range plan.Nodes {
		var parent ecs.Entity
		if node.Parent > 0 {
			parent = entities[node.Parent-1]
		}
		entity, err := s.project.spawnECSPlanNode(s.World, node, parent, s.Entities, s.EntityIDs)
		if err != nil {
			s.Dispose()
			return err
		}
		entities[index] = entity
		s.nodes[entity] = node
		if node.Parent == 0 {
			s.Roots = append(s.Roots, entity)
		}
	}
	for index, node := range plan.Nodes {
		if node.TargetCamera != 0 {
			if err := ecs.Insert(s.World, entities[index], sceneapi.UITargetCamera{Entity: entities[node.TargetCamera-1]}); err != nil {
				s.Dispose()
				return fmt.Errorf("game object %q targetCamera: %w", node.Spec.ID, err)
			}
		}
	}
	if err := bindSceneAnimationTargets(s.World, plan, entities); err != nil {
		s.Dispose()
		return err
	}
	if err := s.BindScripts(); err != nil {
		return err
	}
	return s.RefreshTransforms()
}

// Dispose despawns every entity owned by the scene.
func (s *SceneInstance) Dispose() {
	s.dispose(true)
}

func (s *SceneInstance) dispose(refresh bool) {
	if s == nil || s.World == nil {
		return
	}
	rootIDs := make([]string, 0, len(s.Roots))
	for _, entity := range s.Roots {
		if id := s.EntityIDs[entity]; id != "" {
			rootIDs = append(rootIDs, id)
		}
	}
	s.removeNodes(refresh, rootIDs...)
	clear(s.Entities)
	clear(s.EntityIDs)
	clear(s.nodes)
	clear(s.scriptBindings)
	clear(s.inputBindings)
	s.globalInputBindings.clear()
	s.scriptSystems = nil
	s.scriptsBound = false
	s.Roots = nil
}

// RemoveNodes despawns the requested game objects and their ECS descendants.
func (s *SceneInstance) RemoveNodes(ids ...string) int {
	return s.removeNodes(true, ids...)
}

func (s *SceneInstance) removeNodes(refresh bool, ids ...string) int {
	if s == nil || s.World == nil || len(ids) == 0 {
		return 0
	}
	roots := make(map[ecs.Entity]struct{}, len(ids))
	for _, id := range ids {
		if entity, ok := s.Entities[strings.TrimSpace(id)]; ok {
			roots[entity] = struct{}{}
		}
	}
	if len(roots) == 0 {
		return 0
	}

	children := make(map[ecs.Entity][]ecs.Entity)
	for entity, access := range ecs.Query[sceneapi.Parent](s.World).All {
		parent := access.Ref()

		children[parent.Entity] = append(children[parent.Entity], entity)
	}
	removedEntities := make(map[ecs.Entity]struct{})
	var collect func(ecs.Entity)
	collect = func(entity ecs.Entity) {
		if _, exists := removedEntities[entity]; exists {
			return
		}
		removedEntities[entity] = struct{}{}
		for _, child := range children[entity] {
			collect(child)
		}
	}
	for entity := range roots {
		collect(entity)
	}
	for entity := range removedEntities {
		s.destroyScripts(entity)
		s.unbindControlPart(entity)
	}
	s.rebuildScriptSystems()

	removed := 0
	for id, entity := range s.Entities {
		if _, exists := removedEntities[entity]; !exists {
			continue
		}
		delete(s.Entities, id)
		delete(s.EntityIDs, entity)
		delete(s.nodes, entity)
		removed++
	}
	remainingRoots := s.Roots[:0]
	for _, entity := range s.Roots {
		if _, removed := removedEntities[entity]; !removed {
			remainingRoots = append(remainingRoots, entity)
		}
	}
	s.Roots = remainingRoots

	entities := make([]ecs.Entity, 0, len(removedEntities))
	for entity := range removedEntities {
		entities = append(entities, entity)
	}
	// Releasing highest entity slots first makes subsequent scene rebuilds reuse
	// slots in authored spawn order rather than randomized map iteration order.
	slices.SortFunc(entities, func(a, b ecs.Entity) int {
		return cmp.Compare(uint32(b), uint32(a))
	})
	for _, entity := range entities {
		s.World.Despawn(entity)
	}
	if refresh {
		_ = s.RefreshTransforms()
	}
	return removed
}

func (s *SceneInstance) Entity(id string) (ecs.Entity, error) {
	if s == nil {
		return 0, fmt.Errorf("scene instance is nil")
	}
	entity, ok := s.Entities[strings.TrimSpace(id)]
	if !ok {
		return 0, fmt.Errorf("ecs entity %q not found", id)
	}
	return entity, nil
}

// Slot resolves an authored or runtime entity name once into a generation
// checked lookup handle. Reuse the returned slot in frame code instead of
// rebuilding string IDs for every access.
func (s *SceneInstance) Slot(id string) (ecs.EntitySlot, error) {
	if s == nil || s.World == nil {
		return ecs.EntitySlot{}, fmt.Errorf("scene instance is nil")
	}
	entity, err := s.Entity(id)
	if err != nil {
		return ecs.EntitySlot{}, err
	}
	slot := s.World.Slot(entity)
	if !slot.Valid() {
		return ecs.EntitySlot{}, fmt.Errorf("ecs entity %q is not alive", id)
	}
	return slot, nil
}

// MustSlot is the panic-on-error form of Slot for setup code.
func (s *SceneInstance) MustSlot(id string) ecs.EntitySlot {
	slot, err := s.Slot(id)
	if err != nil {
		panic(err)
	}
	return slot
}

// EntityBySlot resolves a slot created from this scene's world without a
// string lookup. It returns false for stale entities and slots from another
// world.
func (s *SceneInstance) EntityBySlot(slot ecs.EntitySlot) (ecs.Entity, bool) {
	if s == nil || s.World == nil {
		return 0, false
	}
	return s.World.EntityBySlot(slot)
}

func (s *SceneInstance) MustEntity(id string) ecs.Entity {
	entity, err := s.Entity(id)
	if err != nil {
		panic(err)
	}
	return entity
}

func (s *SceneInstance) EntityAt(path string) (ecs.Entity, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return 0, fmt.Errorf("ecs entity path is required")
	}
	parts := strings.Split(path, "/")
	entity, err := s.Entity(parts[len(parts)-1])
	if err != nil {
		return 0, err
	}
	if actual := s.Path(parts[len(parts)-1]); actual != path {
		return 0, fmt.Errorf("ecs entity path %q not found", path)
	}
	return entity, nil
}

func (s *SceneInstance) MustEntityAt(path string) ecs.Entity {
	entity, err := s.EntityAt(path)
	if err != nil {
		panic(err)
	}
	return entity
}

func (s *SceneInstance) Child(parent ecs.Entity, id string) (ecs.Entity, error) {
	entity, err := s.Entity(id)
	if err != nil {
		return 0, err
	}
	relation, ok := ecs.Get[sceneapi.Parent](s.World, entity)
	if !ok || relation.Entity != parent {
		return 0, fmt.Errorf("ecs entity %q is not a child of %q", id, s.ID(parent))
	}
	return entity, nil
}

func (s *SceneInstance) MustChild(parent ecs.Entity, id string) ecs.Entity {
	entity, err := s.Child(parent, id)
	if err != nil {
		panic(err)
	}
	return entity
}

func (s *SceneInstance) ID(entity ecs.Entity) string {
	if s == nil {
		return ""
	}
	return s.EntityIDs[entity]
}

func (s *SceneInstance) Path(id string) string {
	entity, err := s.Entity(id)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, 4)
	for current := entity; current != 0; {
		name := strings.TrimSpace(s.ID(current))
		if name != "" {
			parts = append(parts, name)
		}
		parent, ok := ecs.Get[sceneapi.Parent](s.World, current)
		if !ok {
			break
		}
		current = parent.Entity
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

func (s *SceneInstance) RefreshTransforms() error {
	if s == nil || s.World == nil {
		return nil
	}
	if err := sceneapi.UpdateWorldAnimatorSystem("").Run(s.World); err != nil {
		return err
	}
	if err := sceneapi.SyncGlobalTransformSystem("").Run(s.World); err != nil {
		return err
	}
	if s.project != nil && s.project.uiSync.beforeSync != nil {
		if err := s.project.uiSync.beforeSync(s.World); err != nil {
			return err
		}
	}
	if s.project != nil && s.project.uiSync.afterSync != nil {
		if err := s.project.uiSync.afterSync(s.World); err != nil {
			return err
		}
	}
	if err := sceneapi.UpdateUIAnimatorSystem("").Run(s.World); err != nil {
		return err
	}
	// Profiles with Layout/Widget support apply constraints after animations.
	if s.project != nil && s.project.uiSync.layout != nil {
		if err := s.project.uiSync.layout(s.World); err != nil {
			return err
		}
	}
	return sceneapi.SyncUIGlobalTransformSystem("").Run(s.World)
}

func (s *SceneInstance) RenderItems() []SceneRenderItem {
	if s == nil || s.World == nil {
		return nil
	}
	items := make([]SceneRenderItem, 0, s.World.EntityCount())
	items = appendSceneRenderItemsForTransform(s.World, items, false, func(transform sceneapi.GlobalTransform) mgl32.Mat4 {
		return transform.Matrix
	})
	items = appendSceneRenderItemsForTransform(s.World, items, true, func(transform sceneapi.UIGlobalTransform) mgl32.Mat4 {
		return transform.Matrix
	})
	sortSceneRenderItems(items)
	return items
}

func appendSceneRenderItemsForTransform[T any](world *ecs.World, items []SceneRenderItem, ui bool, matrix func(T) mgl32.Mat4) []SceneRenderItem {
	for entity, access := range ecs.Query3[sceneapi.Sprite2D, T, sceneapi.Visibility](world).All {
		sprite := access.A.Get()
		transform := access.B.Get()
		visibility := access.C.Get()

		if !visibility.Visible || sprite.Texture == nil || !sprite.Texture.Valid() {
			continue
		}
		items = append(items, SceneRenderItem{
			Entity:    entity,
			UI:        ui,
			SortIndex: sceneSortIndex(world, entity, ui, 0),
			Depth:     matrix(transform)[14],
			Sequence:  uint64(entity),
			Material:  sprite.Texture.BatchKey(),
		})
	}
	for entity, access := range ecs.Query3[sceneapi.ParticleEmitter2D, T, sceneapi.Visibility](world).All {
		emitter := access.A.Get()
		transform := access.B.Get()
		visibility := access.C.Get()

		if !visibility.Visible {
			continue
		}
		texture := emitter.Texture
		if texture == nil {
			texture = graphics.CreateTextureByColor(graphics.White)
		}
		items = append(items, SceneRenderItem{
			Entity:    entity,
			UI:        ui,
			SortIndex: sceneSortIndex(world, entity, ui, int(sceneapi.DefaultParticleRenderOrder)),
			Depth:     matrix(transform)[14],
			Sequence:  uint64(entity),
			Material:  texture.BatchKey(),
		})
	}
	for entity, access := range ecs.Query3[sceneapi.Tilemap2D, T, sceneapi.Visibility](world).All {
		tilemap := access.A.Get()
		transform := access.B.Get()
		visibility := access.C.Get()

		if !visibility.Visible || !tilemap.Valid() {
			continue
		}
		items = append(items, SceneRenderItem{
			Entity: entity, UI: ui, SortIndex: sceneSortIndex(world, entity, ui, 0), Depth: matrix(transform)[14], Sequence: uint64(entity), Material: tilemap.Texture.BatchKey(),
		})
	}
	for entity, access := range ecs.Query3[sceneapi.Text2D, T, sceneapi.Visibility](world).All {
		text := access.A.Get()
		transform := access.B.Get()
		visibility := access.C.Get()

		if !visibility.Visible || strings.TrimSpace(text.Value) == "" || text.Font == nil {
			continue
		}
		items = append(items, SceneRenderItem{
			Entity:    entity,
			UI:        ui,
			SortIndex: sceneSortIndex(world, entity, ui, 0),
			Depth:     matrix(transform)[14],
			Sequence:  uint64(entity),
		})
	}
	for entity, access := range ecs.Query3[sceneapi.Spine2D, T, sceneapi.Visibility](world).All {
		component := access.A.Get()
		transform := access.B.Get()
		visibility := access.C.Get()

		if !visibility.Visible || component.Player == nil {
			continue
		}
		items = append(items, SceneRenderItem{Entity: entity, UI: ui, SortIndex: sceneSortIndex(world, entity, ui, 0), Depth: matrix(transform)[14], Sequence: uint64(entity), Material: component.Player.Resource.Texture.BatchKey()})
	}
	return items
}

func (s *SceneInstance) Bounds(entity ecs.Entity) (float32, float32, float32, float32, bool) {
	if s == nil || s.World == nil {
		return 0, 0, 0, 0, false
	}
	transform, ok := sceneTransformMatrix(s.World, entity)
	if !ok {
		return 0, 0, 0, 0, false
	}
	anchor, _ := ecs.Get[sceneapi.Anchor](s.World, entity)
	ui := ecs.Has[sceneapi.UITransform](s.World, entity)
	var minX, minY, maxX, maxY float32
	found := false
	include := func(x0, y0, x1, y1 float32, ok bool) {
		if !ok {
			return
		}
		if !found {
			minX, minY, maxX, maxY, found = x0, y0, x1, y1, true
			return
		}
		minX, minY = min(minX, x0), min(minY, y0)
		maxX, maxY = max(maxX, x1), max(maxY, y1)
	}
	if size, ok := ecs.Get[sceneapi.Size2D](s.World, entity); ok {
		include(sceneapi.BoundsInSpace2D(transform, anchor, size.Width, size.Height, ui))
	}
	if sprite, ok := ecs.Get[sceneapi.Sprite2D](s.World, entity); ok {
		include(sceneapi.SpriteBounds2D(&sprite, anchor, transform, ui))
	}
	if text := ecs.GetMut[sceneapi.Text2D](s.World, entity); text != nil {
		include(sceneapi.TextBounds2D(text, anchor, transform, ui))
	}
	if emitter, ok := ecs.Get[sceneapi.ParticleEmitter2D](s.World, entity); ok {
		include(sceneapi.ParticleEmitterBounds2D(&emitter, transform, ui))
	}
	if tilemap, ok := ecs.Get[sceneapi.Tilemap2D](s.World, entity); ok {
		include(sceneapi.BoundsInSpace2D(transform, anchor, tilemap.PixelWidth(), tilemap.PixelHeight(), ui))
	}
	if component, ok := ecs.Get[sceneapi.Spine2D](s.World, entity); ok && component.Player != nil {
		x0, y0, x1, y1, valid := component.Player.AnimationBounds()
		if valid {
			spineTransform := sceneapi.SpineModelMatrix2D(transform, ui)
			points := [4]mgl32.Vec4{{x0, y0, 0, 1}, {x1, y0, 0, 1}, {x0, y1, 0, 1}, {x1, y1, 0, 1}}
			first := spineTransform.Mul4x1(points[0])
			spineMinX, spineMinY, spineMaxX, spineMaxY := first.X(), first.Y(), first.X(), first.Y()
			for _, point := range points[1:] {
				value := spineTransform.Mul4x1(point)
				spineMinX, spineMinY = min(spineMinX, value.X()), min(spineMinY, value.Y())
				spineMaxX, spineMaxY = max(spineMaxX, value.X()), max(spineMaxY, value.Y())
			}
			include(spineMinX, spineMinY, spineMaxX, spineMaxY, true)
		}
	}
	if size, ok := ecs.Get[sceneapi.Size2D](s.World, entity); ok {
		include(sceneapi.BoundsInSpace2D(transform, anchor, size.Width, size.Height, ui))
	}
	return minX, minY, maxX, maxY, found
}

// TransformMatrix2D returns the resolved matrix for a world or UI entity.
func (s *SceneInstance) TransformMatrix2D(entity ecs.Entity) (mgl32.Mat4, bool) {
	if s == nil || s.World == nil {
		return mgl32.Mat4{}, false
	}
	return sceneTransformMatrix(s.World, entity)
}

func sceneTransformMatrix(world *ecs.World, entity ecs.Entity) (mgl32.Mat4, bool) {
	if transform, ok := ecs.Get[sceneapi.UIGlobalTransform](world, entity); ok {
		return transform.Matrix, true
	}
	if transform, ok := ecs.Get[sceneapi.GlobalTransform](world, entity); ok {
		return transform.Matrix, true
	}
	return mgl32.Mat4{}, false
}

func (s *SceneInstance) BoundsByID(id string) (float32, float32, float32, float32, bool) {
	entity, err := s.Entity(id)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	return s.Bounds(entity)
}

func (s *SceneInstance) IsUI(entity ecs.Entity) bool {
	return s != nil && s.World != nil && ecs.Has[sceneapi.UITransform](s.World, entity)
}

func (s *SceneInstance) Pick(worldPosition, viewportPosition mgl32.Vec2) (string, bool) {
	items := s.RenderItems()
	for i := len(items) - 1; i >= 0; i-- {
		if !ecs.Has[sceneapi.Sprite2D](s.World, items[i].Entity) && !ecs.Has[sceneapi.Text2D](s.World, items[i].Entity) && !ecs.Has[sceneapi.Spine2D](s.World, items[i].Entity) && !ecs.Has[sceneapi.ParticleEmitter2D](s.World, items[i].Entity) {
			continue
		}
		x0, y0, x1, y1, ok := s.Bounds(items[i].Entity)
		if !ok {
			continue
		}
		point := worldPosition
		if ecs.Has[sceneapi.UITransform](s.World, items[i].Entity) {
			point = viewportPosition
		}
		x, y := point.X(), point.Y()
		if x >= x0 && x <= x1 && y >= y0 && y <= y1 {
			id := s.ID(items[i].Entity)
			return id, id != ""
		}
	}
	return "", false
}

func (s *SceneInstance) ApplyNode(spec SceneNodeSpec) error {
	return s.applyNodeUnder("", spec)
}

// ApplyNodeUnder creates a missing node under an existing parent. Updating an
// existing node preserves its current hierarchy.
func (s *SceneInstance) ApplyNodeUnder(parentID string, spec SceneNodeSpec) error {
	return s.applyNodeUnder(strings.TrimSpace(parentID), spec)
}

func (s *SceneInstance) applyNodeUnder(parentID string, spec SceneNodeSpec) error {
	if s == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		return fmt.Errorf("game object id is required")
	}
	if err := validateSceneNodes("apply", []SceneNodeSpec{spec}, make(map[string]struct{})); err != nil {
		return err
	}
	usesUI := specUsesUI(&spec)
	if err := validateGameObjectCoordinateSpaces("apply", []SceneNodeSpec{spec}, nil); err != nil {
		return err
	}
	entity, exists := s.Entities[id]
	if !exists {
		if s.project == nil {
			return fmt.Errorf("scene project is nil")
		}
		parent, hasParent := s.Entities[parentID]
		if parentID != "" && !hasParent {
			return fmt.Errorf("game object %q references unknown parent %q", spec.ID, parentID)
		}
		if hasParent && ecs.Has[sceneapi.UITransform](s.World, parent) != usesUI {
			return fmt.Errorf("game object %q cannot use %s coordinates under parent %q", spec.ID, coordinateSpaceName(usesUI), parentID)
		}
		siblingIndex := len(s.Roots)
		if hasParent {
			siblingIndex = sceneChildCount(s.World, parent)
		}
		entity, err := s.project.spawnECSEntity(s.World, spec, parent, hasParent, siblingIndex, s.Entities, s.EntityIDs)
		if err != nil {
			s.removeNodes(false, id)
			return err
		}
		if err := resolveWidgetTarget(s.World, entity, spec, s.Entities); err != nil {
			s.removeNodes(false, id)
			return err
		}
		rollback := func(err error) error {
			s.removeNodes(false, id)
			return err
		}
		if !hasParent {
			s.Roots = append(s.Roots, entity)
		}
		if err := applyUITargetCameraSpec(s.World, entity, spec.TargetCamera, s.Entities); err != nil {
			return rollback(fmt.Errorf("game object %q: %w", spec.ID, err))
		}
		s.recordSceneNodes(spec)
		if err := s.bindNodeScripts(entity, spec); err != nil {
			return rollback(err)
		}
		if err := s.bindRuntimeAnimationTargetsForSpec(spec); err != nil {
			return rollback(err)
		}
		if err := s.RefreshTransforms(); err != nil {
			return rollback(err)
		}
		return nil
	}
	previousNode := s.nodes[entity]
	previousAnimator, hadAnimator := ecs.Get[sceneapi.Animator](s.World, entity)
	scriptsChanged := !scriptComponentsEqual(previousNode.Spec.Scripts, spec.Scripts)
	if parent, ok := ecs.Get[sceneapi.Parent](s.World, entity); ok {
		if ecs.Has[sceneapi.UITransform](s.World, parent.Entity) != usesUI {
			return fmt.Errorf("entity %q cannot change coordinate space independently of its parent", spec.ID)
		}
	}
	if scriptsChanged {
		s.unbindScripts(entity, true)
		s.rebuildScriptSystems()
	}
	rollback := func(err error) error {
		if restoreErr := s.restoreAppliedNode(entity, previousNode, previousAnimator, hadAnimator, scriptsChanged); restoreErr != nil {
			return fmt.Errorf("%w (failed to restore game object %q: %v)", err, spec.ID, restoreErr)
		}
		return err
	}
	if err := s.applyAuthoredNodeSpec(entity, spec); err != nil {
		return rollback(err)
	}
	if scriptsChanged {
		if err := s.bindNodeScripts(entity, spec); err != nil {
			s.rebuildScriptSystems()
			return rollback(err)
		}
		s.rebuildScriptSystems()
	}
	if err := s.RefreshTransforms(); err != nil {
		return rollback(err)
	}
	return nil
}

// restoreAppliedNode puts an existing entity back into its authored state after
// a replacement failed. The replacement path intentionally mutates one entity
// in place, so restoring through the same compiler keeps component defaults and
// animation bindings consistent without duplicating the ECS composition logic.
func (s *SceneInstance) restoreAppliedNode(entity ecs.Entity, previous sceneSpawnNode, previousAnimator sceneapi.Animator, hadAnimator, scriptsChanged bool) error {
	if s == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	if scriptsChanged {
		s.unbindScripts(entity, true)
		s.rebuildScriptSystems()
	}
	// Make the old authored node visible while its animation targets are being
	// resolved. This matters when the failed replacement changed the clip.
	s.nodes[entity] = previous
	if err := s.applyAuthoredNodeSpec(entity, previous.Spec); err != nil {
		return err
	}
	if hadAnimator {
		if err := ecs.Insert(s.World, entity, previousAnimator); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.Animator](s.World, entity)
	}
	// Dynamic recompilation does not have the static plan's cache key and
	// pre-resolved target arrays. Restore the complete plan node metadata after
	// the ECS composition has been rebuilt.
	s.nodes[entity] = previous
	if scriptsChanged {
		if err := s.bindNodeScripts(entity, previous.Spec); err != nil {
			s.rebuildScriptSystems()
			return err
		}
		s.rebuildScriptSystems()
	}
	return s.RefreshTransforms()
}

// applyAuthoredNodeSpec replaces the authored ECS composition while retaining
// the entity identity and all runtime/script bindings. A node's coordinate
// space selects its transform family; every authored component is installed
// independently on that transform.
func (s *SceneInstance) applyAuthoredNodeSpec(entity ecs.Entity, spec SceneNodeSpec) error {
	node, err := compileSceneSpawnNode(spec)
	if err != nil {
		return err
	}
	// Keep an existing runtime control alive until every fallible authored and
	// animation operation has completed. This makes an unsuccessful replacement
	// reversible even when the new spec removes the control declaration.
	removeControl := spec.Control == nil && ecs.Has[engineui.Control](s.World, entity)
	s.project.clearComponents(s.World, entity)
	if err := ecs.Insert(s.World, entity, sceneapi.Name{Value: strings.TrimSpace(spec.ID)}); err != nil {
		return err
	}
	if node.UI {
		if err := ecs.Insert(s.World, entity, node.UITransform.Component()); err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, sceneapi.UIGlobalTransform{}); err != nil {
			return err
		}
		ecs.Remove[sceneapi.Transform](s.World, entity)
		ecs.Remove[sceneapi.GlobalTransform](s.World, entity)
		ecs.Remove[sceneapi.RenderOrder2D](s.World, entity)
		if err := ecs.Insert(s.World, entity, sceneapi.UIStackIndex(spec.Order)); err != nil {
			return err
		}
	} else {
		if err := ecs.Insert(s.World, entity, node.Transform); err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, sceneapi.GlobalTransform{}); err != nil {
			return err
		}
		ecs.Remove[sceneapi.UITransform](s.World, entity)
		ecs.Remove[sceneapi.UIGlobalTransform](s.World, entity)
		ecs.Remove[sceneapi.UIStackIndex](s.World, entity)
		applyRenderOrderSpec(s.World, entity, spec)
	}
	if err := ecs.Insert(s.World, entity, sceneapi.Visibility{Visible: node.Visible}); err != nil {
		return err
	}
	if err := ecs.Insert(s.World, entity, spec.Anchor); err != nil {
		return err
	}
	if node.UI {
		layoutOrder := 0
		if authored, exists := s.nodes[entity]; exists {
			layoutOrder = authored.LayoutOrder
		}
		if err := ecs.Insert(s.World, entity, sceneapi.SiblingOrder(layoutOrder)); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.SiblingOrder](s.World, entity)
	}
	if spec.Layout != nil {
		if err := ecs.Insert(s.World, entity, *spec.Layout); err != nil {
			return err
		}
	} else {
		ecs.Remove[engineui.Layout](s.World, entity)
	}
	if spec.Widget != nil {
		if err := ecs.Insert(s.World, entity, *spec.Widget); err != nil {
			return err
		}
	} else {
		ecs.Remove[engineui.Widget](s.World, entity)
	}
	if err := resolveWidgetTarget(s.World, entity, spec, s.Entities); err != nil {
		return err
	}
	applyRenderLayersSpec(s.World, entity, spec.RenderLayers)
	if spec.Control != nil || spec.Components.VisualsEmpty() || (ecs.Has[engineui.Control](s.World, entity) && !removeControl) {
		if err := ecs.Insert(s.World, entity, sceneapi.Size2D{Width: float32(spec.Width), Height: float32(spec.Height)}); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.Size2D](s.World, entity)
	}
	if spec.BlockInput && (spec.Components.Has2D() || spec.Control != nil) {
		if err := ecs.Insert(s.World, entity, sceneapi.InputBlocker2D{
			Width: float32(spec.Width), Height: float32(spec.Height), PassThrough: spec.PassThrough,
		}); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.InputBlocker2D](s.World, entity)
	}
	if err := s.applyAuthoredVisualComponents(entity, node); err != nil {
		return err
	}
	if err := applyUITargetCameraSpec(s.World, entity, spec.TargetCamera, s.Entities); err != nil {
		return fmt.Errorf("game object %q: %w", spec.ID, err)
	}
	if ecs.Has[engineui.Control](s.World, entity) && !removeControl {
		if err := s.project.applyBoundControlSpec(s.World, entity, spec); err != nil {
			return err
		}
	}
	s.nodes[entity] = node
	if err := s.bindRuntimeAnimationTargetsForEntity(entity); err != nil {
		return err
	}
	if err := s.recomposeAuthoredNode(entity); err != nil {
		return err
	}
	if removeControl {
		engineui.UnbindControl(s.World, entity)
	}
	return nil
}

func (s *SceneInstance) applyAuthoredVisualComponents(entity ecs.Entity, node sceneSpawnNode) error {
	spec := node.Spec
	var animator sceneapi.Animator
	requiresSprite := false
	if node.HasAnimation {
		var err error
		animator, requiresSprite, err = s.project.ecsAnimationForSpec(spec, &node)
		if err != nil {
			return err
		}
	}
	if node.HasSprite {
		sprite, err := s.project.ecsSpriteForSpec(spec)
		if err != nil {
			return err
		}
		if requiresSprite {
			if texture := animator.CurrentTexture(); texture != nil {
				sprite.Texture = texture
				sprite.Slice = nil
				if spec.Width <= 0 {
					sprite.Width = float32(texture.LogicalWidth())
				}
				if spec.Height <= 0 {
					sprite.Height = float32(texture.LogicalHeight())
				}
			}
		}
		if err := ecs.Insert(s.World, entity, sprite); err != nil {
			return err
		}
		if trail := spec.Components.Sprite.Trail; trail != nil {
			if err := ecs.Insert(s.World, entity, trail.component()); err != nil {
				return err
			}
		} else {
			ecs.Remove[sceneapi.SpriteTrail2D](s.World, entity)
		}
	} else {
		ecs.Remove[sceneapi.Sprite2D](s.World, entity)
		ecs.Remove[sceneapi.SpriteTrail2D](s.World, entity)
		ecs.Remove[sceneapi.Animator](s.World, entity)
	}
	if node.HasAnimation {
		if requiresSprite && !node.HasSprite {
			sprite := sceneapi.Sprite2DFromTexture(animator.CurrentTexture())
			sprite.Alpha = spec.Alpha
			if spec.Width > 0 {
				sprite.Width = float32(spec.Width)
			}
			if spec.Height > 0 {
				sprite.Height = float32(spec.Height)
			}
			if err := ecs.Insert(s.World, entity, sprite); err != nil {
				return err
			}
		}
		if err := ecs.Insert(s.World, entity, animator); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.Animator](s.World, entity)
		if !node.HasSprite {
			ecs.Remove[sceneapi.Sprite2D](s.World, entity)
		}
	}
	if node.HasText {
		text, err := s.project.ecsTextForPlanNode(node)
		if err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, text); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.Text2D](s.World, entity)
	}
	if node.HasParticle {
		emitter, err := s.project.ecsParticleEmitterForGameObject(spec)
		if err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, emitter); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.ParticleEmitter2D](s.World, entity)
	}
	if node.HasTilemap {
		tilemap, err := s.project.ecsTilemapForGameObject(spec)
		if err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, tilemap); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.Tilemap2D](s.World, entity)
	}
	if node.HasSpine {
		component, err := s.project.ecsSpineForGameObject(spec)
		if err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, component); err != nil {
			return err
		}
	} else {
		ecs.Remove[sceneapi.Spine2D](s.World, entity)
	}
	if node.HasModel {
		mesh, material, supported, err := s.project.modelForSpec(spec)
		if err != nil {
			return err
		}
		if supported {
			if err := ecs.Insert(s.World, entity, render3d.Mesh{Mesh: mesh}); err != nil {
				return err
			}
			if err := ecs.Insert(s.World, entity, material); err != nil {
				return err
			}
		} else {
			ecs.Remove[render3d.Mesh](s.World, entity)
			ecs.Remove[render3d.Material[graphics.StandardMaterial]](s.World, entity)
		}
	}
	if node.HasCamera {
		camera := sceneapi.Camera{
			Viewport:   node.Camera.Viewport,
			Order:      node.Camera.Order,
			Active:     node.Camera.Active,
			Clear:      true,
			ClearColor: node.Camera.ClearColor,
		}
		if err := ecs.Insert(s.World, entity, camera); err != nil {
			return err
		}
		renderSettings, err := spec.Components.Camera.renderSettings()
		if err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, renderSettings); err != nil {
			return err
		}
		environment, err := s.project.cameraEnvironment(spec.Components.Camera)
		if err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, environment); err != nil {
			return err
		}
		if err := ecs.Insert(s.World, entity, node.Camera.Projection); err != nil {
			return err
		}
	}
	if node.HasLight {
		if err := ecs.Insert(s.World, entity, node.Light); err != nil {
			return err
		}
	}
	if node.HasAmbientLight {
		if err := ecs.Insert(s.World, entity, node.AmbientLight); err != nil {
			return err
		}
	}
	if node.HasPointLight {
		if err := ecs.Insert(s.World, entity, node.PointLight); err != nil {
			return err
		}
	}
	if node.HasSpotLight {
		if err := ecs.Insert(s.World, entity, node.SpotLight); err != nil {
			return err
		}
	}
	if node.HasAreaLight {
		if err := ecs.Insert(s.World, entity, node.AreaLight); err != nil {
			return err
		}
	}
	if err := applyPhysics3DComponents(s.project, s.World, entity, spec); err != nil {
		return err
	}
	return applyPhysics2DComponents(s.World, entity, spec)
}

func (s *SceneInstance) recomposeAuthoredNode(entity ecs.Entity) error {
	root := entity
	if !ecs.Has[engineui.Control](s.World, root) {
		if relation, ok := ecs.Get[engineui.Relation[engineui.Control]](s.World, entity); ok {
			root = relation.Owner
		} else if parent, ok := ecs.Get[sceneapi.Parent](s.World, entity); ok {
			root = parent.Entity
		} else {
			root = 0
		}
	}
	for root != 0 && !ecs.Has[engineui.Control](s.World, root) {
		parent, ok := ecs.Get[sceneapi.Parent](s.World, root)
		if !ok {
			root = 0
			break
		}
		root = parent.Entity
	}
	if root != 0 {
		return s.recomposeControlsUnder(map[ecs.Entity]struct{}{root: {}})
	}
	return nil
}

func (s *SceneInstance) recordSceneNodes(spec SceneNodeSpec) {
	entity := s.Entities[strings.TrimSpace(spec.ID)]
	if entity != 0 {
		if node, err := compileSceneSpawnNode(spec); err == nil {
			if order, ok := ecs.Get[sceneapi.SiblingOrder](s.World, entity); ok {
				node.LayoutOrder = int(order)
			}
			s.nodes[entity] = node
		}
	}
	for _, child := range spec.Children {
		s.recordSceneNodes(child)
	}
}

func (s *SceneInstance) SetParticleResource(spec ParticleSpec) error {
	if s == nil || s.project == nil {
		return fmt.Errorf("scene project is nil")
	}
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		return fmt.Errorf("particle resource id is required")
	}
	if err := spec.Texture.validate(false); err != nil {
		return fmt.Errorf("particle %q texture %w", id, err)
	}
	s.project.resourceSpecMu.Lock()
	s.project.particleSpecs[id] = spec
	s.project.resourceSpecMu.Unlock()
	return nil
}

func (s *SceneInstance) SetTilemapResource(spec TilemapSpec) error {
	if s == nil || s.project == nil {
		return fmt.Errorf("scene project is nil")
	}
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		return fmt.Errorf("tilemap resource id is required")
	}
	if err := spec.validate(); err != nil {
		return err
	}
	spec.Normalize()
	s.project.resourceSpecMu.Lock()
	s.project.tilemapSpecs[id] = spec
	s.project.resourceSpecMu.Unlock()
	return nil
}

func (s *SceneInstance) SetModelResource(spec ModelSpec) error {
	if s == nil || s.project == nil {
		return fmt.Errorf("scene project is nil")
	}
	if err := spec.validate(); err != nil {
		return err
	}
	id := strings.TrimSpace(spec.ID)
	s.project.resourceSpecMu.Lock()
	defer s.project.resourceSpecMu.Unlock()
	if current, ok := s.project.modelSpecs[id]; ok && equalModelSpec(current, spec) {
		return nil
	}
	modelResources, ok := s.project.modelResources.(*resourceCache[string, modelResource])
	if !ok {
		return fmt.Errorf("model resources are not configured")
	}
	if err := modelResources.remove(id); err != nil {
		return fmt.Errorf("dispose model %q: %w", id, err)
	}
	s.project.modelSpecs[id] = spec
	return nil
}

func (s *SceneInstance) SetSpineResource(spec SpineSpec) error {
	if s == nil || s.project == nil || s.project.resources == nil {
		return fmt.Errorf("scene project is nil")
	}
	if err := spec.validate(); err != nil {
		return err
	}
	id := strings.TrimSpace(spec.ID)
	spec.ID = id
	s.project.resourceSpecMu.Lock()
	defer s.project.resourceSpecMu.Unlock()
	if current, ok := s.project.spineSpecs[id]; ok && current == spec {
		return nil
	}
	resource, err := s.project.resources.loadSpine(spec)
	if err != nil {
		return fmt.Errorf("load spine %q: %w", id, err)
	}
	spines, ok := s.project.resources.spines.(*resourceCache[string, *spine.Resource])
	if !ok {
		return fmt.Errorf("spine resources are not configured")
	}
	if err := spines.replace(id, resource, nil); err != nil {
		return fmt.Errorf("replace spine %q: %w", id, err)
	}
	s.project.spineSpecs[id] = spec
	return nil
}

func (b *Bundle) spawnECSEntity(world *ecs.World, spec SceneNodeSpec, parent ecs.Entity, hasParent bool, siblingIndex int, index map[string]ecs.Entity, reverse map[ecs.Entity]string) (ecs.Entity, error) {
	components, err := b.ecsComponentsForSpec(spec)
	if err != nil {
		return 0, err
	}
	if hasParent {
		ecs.Put(&components, sceneapi.Parent{Entity: parent})
	}
	entity := world.Spawn(components)
	if specUsesUI(&spec) {
		if err := ecs.Insert(world, entity, sceneapi.SiblingOrder(siblingIndex)); err != nil {
			return 0, err
		}
	}
	if spec.BlockInput && (spec.Components.Has2D() || spec.Control != nil) {
		_ = ecs.Insert(world, entity, sceneapi.InputBlocker2D{
			Width: float32(spec.Width), Height: float32(spec.Height), PassThrough: spec.PassThrough,
		})
	}

	if err := indexSceneEntity(spec, entity, index, reverse); err != nil {
		return 0, err
	}
	for childIndex, child := range spec.Children {
		if _, err := b.spawnECSEntity(world, child, entity, true, childIndex, index, reverse); err != nil {
			return 0, err
		}
	}
	return entity, nil
}

func resolveWidgetTarget(world *ecs.World, entity ecs.Entity, spec SceneNodeSpec, entities map[string]ecs.Entity) error {
	if spec.Widget != nil {
		widget := ecs.GetMut[engineui.Widget](world, entity)
		if widget == nil {
			return fmt.Errorf("game object %q widget target requires a Widget component", spec.ID)
		}
		if targetID := strings.TrimSpace(spec.Widget.TargetID); targetID != "" {
			target, exists := entities[targetID]
			if !exists {
				return fmt.Errorf("game object %q widget target %q does not exist", spec.ID, targetID)
			}
			widget.Target = target
		}
	}
	for _, child := range spec.Children {
		childEntity, exists := entities[strings.TrimSpace(child.ID)]
		if !exists {
			return fmt.Errorf("game object %q child %q was not spawned", spec.ID, child.ID)
		}
		if err := resolveWidgetTarget(world, childEntity, child, entities); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bundle) spawnECSPlanNode(world *ecs.World, node sceneSpawnNode, parent ecs.Entity, index map[string]ecs.Entity, reverse map[ecs.Entity]string) (ecs.Entity, error) {
	hasParent := node.Parent > 0
	components, err := b.ecsComponentsForPlanNode(node)
	if err != nil {
		return 0, err
	}
	if hasParent {
		ecs.Put(&components, sceneapi.Parent{Entity: parent})
	}
	entity := world.Spawn(components)
	if node.Spec.BlockInput && (node.Spec.Components.Has2D() || node.Spec.Control != nil) {
		_ = ecs.Insert(world, entity, sceneapi.InputBlocker2D{
			Width: float32(node.Spec.Width), Height: float32(node.Spec.Height), PassThrough: node.Spec.PassThrough,
		})
	}
	if err := indexSceneEntity(node.Spec, entity, index, reverse); err != nil {
		return 0, err
	}
	return entity, nil
}

func indexSceneEntity(spec SceneNodeSpec, entity ecs.Entity, index map[string]ecs.Entity, reverse map[ecs.Entity]string) error {
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		return nil
	}
	if _, exists := index[id]; exists {
		return fmt.Errorf("scene contains duplicate node id %q", id)
	}
	index[id] = entity
	reverse[entity] = id
	return nil
}

func (s *SceneInstance) recomposeControlsUnder(roots map[ecs.Entity]struct{}) error {
	entities := make([]ecs.Entity, 0, len(s.nodes))
	for entity := range s.nodes {
		if sceneEntityUnder(s.World, entity, roots) {
			entities = append(entities, entity)
		}
	}
	sort.Slice(entities, func(i, j int) bool { return entities[i] < entities[j] })
	for _, entity := range entities {
		s.unbindControlPart(entity)
	}
	for _, entity := range entities {
		if ecs.Has[engineui.Control](s.World, entity) {
			ecs.Remove[engineui.Variants[engineui.ControlVariant]](s.World, entity)
		}
	}
	for _, entity := range entities {
		if !ecs.Has[engineui.Control](s.World, entity) {
			continue
		}
		if err := s.bindControlComposition(entity); err != nil {
			return err
		}
	}
	return s.RefreshTransforms()
}

func sceneEntityUnder(world *ecs.World, entity ecs.Entity, roots map[ecs.Entity]struct{}) bool {
	for remaining := world.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if _, ok := roots[entity]; ok {
			return true
		}
		parent, ok := ecs.Get[sceneapi.Parent](world, entity)
		if !ok {
			return false
		}
		entity = parent.Entity
	}
	return false
}

// BindControl installs an explicitly selected runtime behavior on an authored
// control root.
func BindControl[T any](scene *SceneInstance, id string, component T) error {
	if scene == nil || scene.World == nil || scene.project == nil {
		return fmt.Errorf("scene instance is nil")
	}
	entity, err := scene.Entity(id)
	if err != nil {
		return err
	}
	if scene.nodes[entity].Spec.Control == nil {
		return fmt.Errorf("game object %q must declare a control component before binding a control", id)
	}
	if !scene.nodes[entity].UI {
		return fmt.Errorf("game object %q requires coordinate space ui before binding a control", id)
	}
	if err := bindSceneControl(scene, entity, component); err != nil {
		return err
	}
	return scene.RefreshTransforms()
}

// BindControlEntity installs behavior on a runtime entity created from a
// scene template. It is the entity-based counterpart to BindControl.
func BindControlEntity[T any](scene *SceneInstance, entity ecs.Entity, component T) error {
	if scene == nil || scene.World == nil || scene.project == nil {
		return fmt.Errorf("scene instance is nil")
	}
	node, ok := scene.authoredNode(entity)
	if !ok || node.Spec.Control == nil || !node.UI {
		return fmt.Errorf("entity %d is not an authored UI control", entity)
	}
	return bindSceneControl(scene, entity, component)
}

func bindSceneControl[T any](scene *SceneInstance, entity ecs.Entity, component T) error {
	node, ok := scene.authoredNode(entity)
	if !ok {
		return fmt.Errorf("entity %d is not an authored node", entity)
	}
	theme, err := scene.project.uiThemeForSpec(scene.World, node.Spec)
	if err != nil {
		return err
	}
	width, height := float32(node.Spec.Width), float32(node.Spec.Height)
	switch value := any(&component).(type) {
	case *engineui.Selector:
		value.StackIndex = theme.StackIndex
	case *engineui.ScrollArea:
		value.Width = width
		value.Height = height
		value.StackIndex = theme.StackIndex
	}
	opts := engineui.ControlOptions{
		X: node.UITransform.Translation.X.Value, Y: node.UITransform.Translation.Y.Value,
		Width: width, Height: height, StackIndex: theme.StackIndex,
		Disabled: node.Spec.Disabled, PassThrough: node.Spec.PassThrough,
	}
	if err := engineui.BindControl(scene.World, entity, opts, component); err != nil {
		return err
	}
	switch any(component).(type) {
	case engineui.Button, engineui.Toggle[bool]:
		if err := ecs.Insert(scene.World, entity, engineui.Content[string]{Value: sceneNodeControlText(node.Spec)}); err != nil {
			return err
		}
		if err := ecs.Insert(scene.World, entity, engineui.Typography{FontSize: theme.FontSize, DPI: theme.DPI}); err != nil {
			return err
		}
	case engineui.ScrollArea:
		if err := ecs.Insert(scene.World, entity, sceneapi.ClipRect2D{}); err != nil {
			return err
		}
	}
	roots := map[ecs.Entity]struct{}{entity: {}}
	scene.visitAuthoredComposition(entity, func(child ecs.Entity, _ sceneSpawnNode) {
		if child != entity && sceneEntityUnder(scene.World, child, roots) {
			engineui.UnbindControlCompositionPart(scene.World, child)
		}
	})
	ecs.Remove[engineui.Variants[engineui.ControlVariant]](scene.World, entity)
	return scene.bindControlComposition(entity)
}

func (s *SceneInstance) bindControlComposition(owner ecs.Entity) error {
	composition := make(engineui.ControlComposition, s.authoredCompositionSize(owner))
	s.visitAuthoredComposition(owner, func(entity ecs.Entity, node sceneSpawnNode) {
		var kind engineui.CompositionNodeKind
		switch {
		case node.Spec.Control != nil:
			kind = engineui.CompositionGroup
		case node.HasSprite:
			kind = engineui.CompositionSprite
		case node.HasText:
			kind = engineui.CompositionText
		case node.HasAnimation || (!node.HasParticle && !node.HasTilemap && !node.HasSpine):
			kind = engineui.CompositionGroup
		default:
			return
		}
		composition[entity] = engineui.CompositionNode{
			Kind: kind,
			X:    node.UITransform.Translation.X.Value, Y: node.UITransform.Translation.Y.Value,
			Width: float32(node.Spec.Width), Height: float32(node.Spec.Height),
			Text: sceneNodeControlText(node.Spec), Visible: node.Visible,
		}
	})
	return engineui.BindControlComposition(s.World, composition, owner, func(entity ecs.Entity) error {
		return bindSceneControl(s, entity, engineui.Button{})
	})
}

func (s *SceneInstance) authoredNode(entity ecs.Entity) (sceneSpawnNode, bool) {
	node, ok := s.nodes[entity]
	return node, ok
}

func (s *SceneInstance) authoredCompositionSize(owner ecs.Entity) int {
	if s == nil || s.World == nil {
		return 0
	}
	count := 0
	for entity := range s.nodes {
		if s.authoredNodeUnder(owner, entity) {
			count++
		}
	}
	return count
}

func (s *SceneInstance) visitAuthoredComposition(owner ecs.Entity, visit func(ecs.Entity, sceneSpawnNode)) {
	if s == nil || s.World == nil || visit == nil {
		return
	}
	for entity, node := range s.nodes {
		if !s.authoredNodeUnder(owner, entity) {
			continue
		}
		visit(entity, node)
	}
}

func (s *SceneInstance) authoredNodeUnder(owner, entity ecs.Entity) bool {
	if owner == 0 || entity == 0 {
		return false
	}
	for steps := s.World.EntityCount() + 1; entity != 0 && steps > 0; steps-- {
		if entity == owner {
			return true
		}
		parent, ok := ecs.Get[sceneapi.Parent](s.World, entity)
		if !ok {
			return false
		}
		entity = parent.Entity
	}
	return false
}

func (b *Bundle) uiThemeForSpec(world *ecs.World, spec SceneNodeSpec) (engineui.Theme, error) {
	theme := engineui.DefaultTheme()
	// A project can install one shared UI theme before scene instantiation.
	if configured, ok := ecs.GetResource[engineui.Theme](world); ok {
		theme = configured
	}
	theme.StackIndex = spec.Order
	control := spec.Control
	font, fontSize := "", float64(0)
	if control != nil {
		font, fontSize = control.Font, control.FontSize
	} else if text := spec.Components.Text; text != nil {
		font, fontSize = text.Font, text.FontSize
	}
	if fontSize > 0 {
		theme.FontSize = fontSize
	}
	if font != "" {
		fontResource, err := b.resources.Font(font)
		if err != nil {
			return engineui.Theme{}, fmt.Errorf("game object %q font: %w", spec.ID, err)
		}
		theme.Font = fontResource
	}
	return theme, nil
}

func sceneNodeControlText(spec SceneNodeSpec) string {
	if spec.Control != nil {
		return spec.Control.Text
	}
	if spec.Components.Text != nil {
		return spec.Components.Text.Value
	}
	return ""
}

func (b *Bundle) applyBoundControlSpec(world *ecs.World, entity ecs.Entity, spec SceneNodeSpec) error {
	theme, err := b.uiThemeForSpec(world, spec)
	if err != nil {
		return err
	}
	width, height := float32(spec.Width), float32(spec.Height)
	engineui.SetControlBounds(world, entity, width, height)
	engineui.SetControlDisabled(world, entity, spec.Disabled)
	engineui.SetControlPassThrough(world, entity, spec.PassThrough)
	setUIStackIndex(world, entity, theme.StackIndex)
	if content := ecs.GetMut[engineui.Content[string]](world, entity); content != nil {
		content.Value = sceneNodeControlText(spec)
	}
	if typography := ecs.GetMut[engineui.Typography](world, entity); typography != nil {
		typography.FontSize, typography.DPI = theme.FontSize, theme.DPI
	}
	if blocker := ecs.GetMut[sceneapi.InputBlocker2D](world, entity); blocker != nil {
		blocker.Width, blocker.Height = width, height
	}
	return err
}

func (b *Bundle) ecsComponentsForSpec(spec SceneNodeSpec) (ecs.Bundle, error) {
	node, err := compileSceneSpawnNode(spec)
	if err != nil {
		return ecs.Bundle{}, err
	}
	return b.ecsComponentsForPlanNode(node)
}

func (b *Bundle) ecsComponentsForPlanNode(node sceneSpawnNode) (ecs.Bundle, error) {
	if b == nil || b.components == nil {
		return ecs.Bundle{}, fmt.Errorf("project scene component feature set is not configured")
	}
	return b.components(b, node)
}

type sceneComponentBuilder func(*Bundle, sceneSpawnNode) (ecs.Bundle, error)

func buildAllSceneComponents(bundle *Bundle, node sceneSpawnNode) (ecs.Bundle, error) {
	return bundle.buildSceneComponents(node, true, true)
}

func buildConfiguredSceneComponents(bundle *Bundle, node sceneSpawnNode) (ecs.Bundle, error) {
	config := bundle.engineConfig
	if node.Has3DComponents && !config.HasFeature(FeatureRender3D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Render 3D system", node.Spec.ID)
	}
	if (node.HasSprite || node.HasText || node.HasHTML || node.HasParticle || node.HasTilemap || node.HasSpine || node.Spec.Control != nil) && !config.HasFeature(FeatureRender2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Render 2D system", node.Spec.ID)
	}
	if node.HasHTML && !config.HasFeature(FeatureUIHTML) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the UI HTML system", node.Spec.ID)
	}
	if node.HasAnimation && !config.HasFeature(FeatureAnimations2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Animations 2D system", node.Spec.ID)
	}
	if node.Spec.Components.Sprite != nil && node.Spec.Components.Sprite.Trail != nil && !config.HasFeature(FeatureSpriteTrails2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Sprite Trails 2D system", node.Spec.ID)
	}
	if node.HasParticle && !config.HasFeature(FeatureParticles2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Particles 2D system", node.Spec.ID)
	}
	if node.HasTilemap && !config.HasFeature(FeatureTilemaps2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Tilemaps 2D system", node.Spec.ID)
	}
	if node.HasSpine && !config.HasFeature(FeatureSpine2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Spine 2D system", node.Spec.ID)
	}
	if node.HasPhysics2D && !config.HasFeature(FeaturePhysics2D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Physics 2D system", node.Spec.ID)
	}
	if node.HasPhysics3D && !config.HasFeature(FeaturePhysics3D) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the Physics 3D system", node.Spec.ID)
	}
	if node.Spec.Control != nil && !config.HasFeature(FeatureUIControls) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the UI Controls system", node.Spec.ID)
	}
	if (node.Spec.Layout != nil || node.Spec.Widget != nil) && !config.HasFeature(FeatureUIWidgets) && !config.HasFeature(FeatureUIControls) {
		return ecs.Bundle{}, fmt.Errorf("scene node %q requires the UI Widgets system", node.Spec.ID)
	}
	return bundle.buildSceneComponents(node, true, true)
}

// buildSceneComponents materializes a node from its independent authored
// components. The transform mode is selected by coordinate space; attached
// render, animation, camera, and light components are all installed together.
func (b *Bundle) buildSceneComponents(node sceneSpawnNode, named, include3D bool) (ecs.Bundle, error) {
	spec := node.Spec
	components := ecs.NewBundle(28)
	if named {
		ecs.Put(&components, sceneapi.Name{Value: spec.ID})
	}
	if node.UI {
		ecs.Put(&components, node.UITransform.Component())
		ecs.Put(&components, sceneapi.UIGlobalTransform{})
		ecs.Put(&components, sceneapi.UIStackIndex(spec.Order))
		ecs.Put(&components, sceneapi.SiblingOrder(node.LayoutOrder))
	} else {
		ecs.Put(&components, node.Transform)
		ecs.Put(&components, sceneapi.GlobalTransform{})
		order := spec.Order
		if node.HasParticle && order == 0 {
			order = int(sceneapi.DefaultParticleRenderOrder)
		}
		ecs.Put(&components, sceneapi.RenderOrder2D(order))
	}
	if spec.Layout != nil {
		ecs.Put(&components, *spec.Layout)
	}
	if spec.Widget != nil {
		ecs.Put(&components, *spec.Widget)
	}
	ecs.Put(&components, sceneapi.Visibility{Visible: node.Visible})
	ecs.Put(&components, spec.Anchor)
	if layers, ok := renderLayersFromSpec(spec.RenderLayers); ok {
		ecs.Put(&components, layers)
	}
	var animator sceneapi.Animator
	requiresSprite := false
	if node.HasAnimation {
		var err error
		animator, requiresSprite, err = b.ecsAnimationForSpec(spec, &node)
		if err != nil {
			return ecs.Bundle{}, err
		}
	}
	if spec.Control != nil || spec.Components.VisualsEmpty() {
		ecs.Put(&components, sceneapi.Size2D{Width: float32(spec.Width), Height: float32(spec.Height)})
	}
	if node.HasSprite {
		sprite, err := b.ecsSpriteForSpec(spec)
		if err != nil {
			return ecs.Bundle{}, err
		}
		if requiresSprite {
			if texture := animator.CurrentTexture(); texture != nil {
				sprite.Texture = texture
				sprite.Slice = nil
				if spec.Width <= 0 {
					sprite.Width = float32(texture.LogicalWidth())
				}
				if spec.Height <= 0 {
					sprite.Height = float32(texture.LogicalHeight())
				}
			}
		}
		ecs.Put(&components, sprite)
		if trail := spec.Components.Sprite.Trail; trail != nil {
			if err := trail.validate(); err != nil {
				return ecs.Bundle{}, fmt.Errorf("game object %q trail %w", spec.ID, err)
			}
			ecs.Put(&components, trail.component())
		}
	}
	if node.HasText {
		text, err := b.ecsTextForPlanNode(node)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&components, text)
	}
	if node.HasHTML {
		value, err := b.resources.HTML(spec.Components.HTML.Resource)
		if err != nil {
			return ecs.Bundle{}, fmt.Errorf("game object %q html: %w", spec.ID, err)
		}
		ecs.Put(&components, html.Surface{Value: value, Width: float32(spec.Width), Height: float32(spec.Height), PassThrough: spec.PassThrough, StackIndex: spec.Order})
	}
	if node.HasParticle {
		emitter, err := b.ecsParticleEmitterForGameObject(spec)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&components, emitter)
	}
	if node.HasTilemap {
		tilemap, err := b.ecsTilemapForGameObject(spec)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&components, tilemap)
	}
	if node.HasSpine {
		component, err := b.ecsSpineForGameObject(spec)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&components, component)
	}
	if node.HasAnimation {
		if requiresSprite && !node.HasSprite {
			sprite := sceneapi.Sprite2DFromTexture(animator.CurrentTexture())
			sprite.Alpha = spec.Alpha
			if spec.Width > 0 {
				sprite.Width = float32(spec.Width)
			}
			if spec.Height > 0 {
				sprite.Height = float32(spec.Height)
			}
			ecs.Put(&components, sprite)
		}
		ecs.Put(&components, animator)
	}
	if include3D {
		if node.HasModel {
			mesh, material, supported, err := b.modelForSpec(spec)
			if err != nil {
				return ecs.Bundle{}, err
			}
			if supported {
				ecs.Put(&components, render3d.Mesh{Mesh: mesh})
				ecs.Put(&components, material)
			}
		}
		if node.HasCamera {
			ecs.Put(&components, sceneapi.Camera{
				Viewport: node.Camera.Viewport, Order: node.Camera.Order, Active: node.Camera.Active,
				Clear: true, ClearColor: node.Camera.ClearColor,
			})
			renderSettings, err := spec.Components.Camera.renderSettings()
			if err != nil {
				return ecs.Bundle{}, err
			}
			ecs.Put(&components, renderSettings)
			ecs.Put(&components, node.Camera.Projection)
			environment, err := b.cameraEnvironment(spec.Components.Camera)
			if err != nil {
				return ecs.Bundle{}, err
			}
			ecs.Put(&components, environment)
		}
		if node.HasLight {
			ecs.Put(&components, node.Light)
		}
		if node.HasAmbientLight {
			ecs.Put(&components, node.AmbientLight)
		}
		if node.HasPointLight {
			ecs.Put(&components, node.PointLight)
		}
		if node.HasSpotLight {
			ecs.Put(&components, node.SpotLight)
		}
		if node.HasAreaLight {
			ecs.Put(&components, node.AreaLight)
		}
	}
	if err := putPhysics3DComponents(b, &components, spec); err != nil {
		return ecs.Bundle{}, err
	}
	putPhysics2DComponents(&components, spec)
	return components, nil
}

func (b *Bundle) modelForSpec(gameObject SceneNodeSpec) (*graphics.Mesh, render3d.Material[graphics.StandardMaterial], bool, error) {
	value, supported, err := b.modelResourceForSpec(gameObject)
	if err != nil {
		return nil, render3d.Material[graphics.StandardMaterial]{}, false, err
	}
	if !supported {
		return nil, render3d.Material[graphics.StandardMaterial]{}, false, nil
	}
	return value.mesh, render3d.Material[graphics.StandardMaterial]{Material: value.material, Materials: value.materials}, supported, nil
}

func (b *Bundle) collisionForModelID(id string) (enginephysics.CookedCollision, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return enginephysics.CookedCollision{}, fmt.Errorf("3D collision requires a model resource")
	}
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: id, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: id}}})
	if err != nil {
		return enginephysics.CookedCollision{}, err
	}
	if !value.hasCollision {
		return enginephysics.CookedCollision{}, fmt.Errorf("model %q has no cooked collision; set its collision mode or collisionPath", id)
	}
	return value.collision, nil
}

func (b *Bundle) modelResourceForSpec(gameObject SceneNodeSpec) (modelResource, bool, error) {
	id := strings.TrimSpace(gameObject.Components.Model.Resource)
	modelResources, ok := b.modelResources.(*resourceCache[string, modelResource])
	if !ok {
		return modelResource{}, false, nil
	}
	if cached, found, cacheErr := modelResources.readyValue(id); found {
		if cacheErr != nil {
			return modelResource{}, false, cacheErr
		}
		return cached, cached.mesh != nil, nil
	}
	b.resourceSpecMu.RLock()
	model, ok := b.modelSpecs[id]
	b.resourceSpecMu.RUnlock()
	if !ok {
		return modelResource{}, false, fmt.Errorf("game object %q model resource %q not found", gameObject.ID, id)
	}
	value, err := modelResources.load(id, func() (modelResource, error) {
		return b.resources.loadModModel(model)
	}, modelResource.dispose)
	if err != nil {
		return modelResource{}, false, fmt.Errorf("game object %q model %q: %w", gameObject.ID, id, err)
	}
	return value, value.mesh != nil, nil
}

func transform3DForSpec(spec SceneNodeSpec) sceneapi.Transform {
	transform := sceneapi.NewTransform()
	transform.Translation = mgl32.Vec3{spec.X, spec.Y, spec.Z}
	transform.Scale = mgl32.Vec3{spec.ScaleX, spec.ScaleY, spec.ScaleZ}
	rx := mgl32.QuatRotate(spec.RotationX, mgl32.Vec3{1, 0, 0})
	ry := mgl32.QuatRotate(spec.RotationY, mgl32.Vec3{0, 1, 0})
	rz := mgl32.QuatRotate(spec.RotationZ, mgl32.Vec3{0, 0, 1})
	transform.Rotation = rz.Mul(ry).Mul(rx).Normalize()
	return transform
}

func transform2DForSpec(spec SceneNodeSpec) sceneapi.Transform {
	transform := sceneapi.NewTransform()
	transform.Translation = mgl32.Vec3{spec.X, spec.Y, spec.Z}
	transform.Scale = mgl32.Vec3{spec.ScaleX, spec.ScaleY, 1}
	transform.Rotation = mgl32.QuatRotate(spec.Rotation, mgl32.Vec3{0, 0, 1})
	return transform
}

func uiTransformForSpec(spec SceneNodeSpec) sceneapi.UITransform {
	transform := sceneapi.NewUITransform()
	transform.Translation = sceneapi.Px2(spec.X, spec.Y)
	transform.Scale = mgl32.Vec2{spec.ScaleX, spec.ScaleY}
	transform.Rotation = spec.Rotation
	return transform
}

func (b *Bundle) ecsSpriteForSpec(spec SceneNodeSpec) (sceneapi.Sprite2D, error) {
	component := spec.Components.Sprite
	var texture *graphics.Texture
	tint := graphics.White
	if component.Texture != nil && strings.TrimSpace(component.Texture.Color) != "" {
		var err error
		tint, err = b.parseColor(component.Texture.Color, 1)
		if err != nil {
			return sceneapi.Sprite2D{}, fmt.Errorf("game object %q texture: %w", spec.ID, err)
		}
		texture = graphics.CreateTextureByColor(graphics.White)
	} else {
		loaded, err := b.textureFor(spec)
		if err != nil {
			return sceneapi.Sprite2D{}, err
		}
		texture = loaded
	}
	sprite, err := sprite2DForSpec(spec, texture, tint)
	return sprite, err
}

func (b *Bundle) ecsAnimationForSpec(spec SceneNodeSpec, plan *sceneSpawnNode) (sceneapi.Animator, bool, error) {
	component := spec.Components.Animation
	if component == nil {
		return sceneapi.Animator{}, false, nil
	}
	if component.Clip.HasContent() {
		var clip *sceneapi.AnimationClip
		if plan != nil && plan.AnimationKey != "" {
			var exists bool
			var err error
			clip, exists, err = b.inlineAnimationClips.get(plan.AnimationKey)
			if err != nil {
				return sceneapi.Animator{}, false, fmt.Errorf("game object %q animation: %w", spec.ID, err)
			}
			if !exists {
				return sceneapi.Animator{}, false, fmt.Errorf("game object %q animation cache %q not found", spec.ID, plan.AnimationKey)
			}
		}
		var err error
		if clip == nil {
			clip, err = b.resources.compileAnimationClipSpec(component.Clip, spec.ID)
			if err != nil {
				return sceneapi.Animator{}, false, fmt.Errorf("game object %q animation: %w", spec.ID, err)
			}
		}
		animator := sceneapi.NewAnimator(clip, component.Clip.Loop)
		if component.Speed > 0 {
			animator.Speed = component.Speed
		}
		requiresSprite, err := clip.RequiresSprite2D()
		if err != nil {
			return sceneapi.Animator{}, false, fmt.Errorf("game object %q animation: %w", spec.ID, err)
		}
		return animator, requiresSprite, nil
	}
	return sceneapi.Animator{}, false, nil
}

func bindSceneAnimationTargets(world *ecs.World, plan sceneSpawnPlan, entities []ecs.Entity) error {
	maxTargetCount := 0
	for index := range plan.Nodes {
		node := &plan.Nodes[index]
		if node.Spec.Components.Animation == nil || !node.Spec.Components.Animation.Clip.HasContent() {
			continue
		}
		maxTargetCount = max(maxTargetCount, len(node.AnimationFloatTargets)+len(node.AnimationTextureTargets)+len(node.AnimationBoolTargets))
	}
	targetScratch := make([]ecs.Entity, maxTargetCount)
	for index := range plan.Nodes {
		node := &plan.Nodes[index]
		if index >= len(entities) || node.Spec.Components.Animation == nil || !node.Spec.Components.Animation.Clip.HasContent() {
			continue
		}
		animator := ecs.GetMut[sceneapi.Animator](world, entities[index])
		if animator == nil {
			return fmt.Errorf("game object %q animation component is missing", node.Spec.ID)
		}
		targetCount := len(node.AnimationFloatTargets) + len(node.AnimationTextureTargets) + len(node.AnimationBoolTargets)
		targets := targetScratch[:targetCount]
		floatEnd := len(node.AnimationFloatTargets)
		textureEnd := floatEnd + len(node.AnimationTextureTargets)
		floatTargets := targets[:floatEnd]
		textureTargets := targets[floatEnd:textureEnd]
		boolTargets := targets[textureEnd:]
		if err := resolveAnimationEntities(node.AnimationFloatTargets, entities, floatTargets); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		if err := resolveAnimationEntities(node.AnimationTextureTargets, entities, textureTargets); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		if err := resolveAnimationEntities(node.AnimationBoolTargets, entities, boolTargets); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		animator.BindTargets(entities[index], floatTargets, textureTargets, boolTargets)
	}
	return nil
}

// bindRuntimeAnimationTargets resolves inline clip paths once for entities
// created outside a precompiled scene/prefab plan (editor ApplyNode and
// runtime clones). Playback receives only these concrete entities; no path or
// name lookup is performed by the animator update system.
func (s *SceneInstance) bindRuntimeAnimationTargets(owners ...ecs.Entity) error {
	if s == nil || s.World == nil {
		return fmt.Errorf("scene instance is nil")
	}
	requested := make(map[ecs.Entity]struct{}, len(owners))
	requestedOwners := make([]ecs.Entity, 0, len(owners))
	targetCount := 0
	for _, owner := range owners {
		if owner == 0 {
			continue
		}
		node, exists := s.nodes[owner]
		if !exists || node.Spec.Components.Animation == nil {
			continue
		}
		if _, duplicate := requested[owner]; duplicate {
			continue
		}
		animator, ok := ecs.Get[sceneapi.Animator](s.World, owner)
		if !ok || animator.Clip == nil {
			return fmt.Errorf("game object %q animation component is missing", node.Spec.ID)
		}
		requested[owner] = struct{}{}
		requestedOwners = append(requestedOwners, owner)
		targetCount += len(animator.Clip.FloatTracks) + len(animator.Clip.TextureTracks) + len(animator.Clip.BoolTracks)
	}
	if len(requestedOwners) == 0 {
		return nil
	}
	children := s.runtimeAnimationChildIndex()
	claims := make(map[runtimeAnimationClaim]ecs.Entity, targetCount)
	seen := make(map[runtimeAnimationSeen]struct{}, targetCount)
	if err := s.claimExistingRuntimeAnimationTargets(children, requested, claims, seen); err != nil {
		return err
	}
	targets := make([]ecs.Entity, targetCount)
	nextTarget := 0
	for _, owner := range requestedOwners {
		node := s.nodes[owner]
		animator, ok := ecs.Get[sceneapi.Animator](s.World, owner)
		if !ok || animator.Clip == nil {
			return fmt.Errorf("game object %q animation component is missing", node.Spec.ID)
		}
		floatEnd := nextTarget + len(animator.Clip.FloatTracks)
		textureEnd := floatEnd + len(animator.Clip.TextureTracks)
		boolEnd := textureEnd + len(animator.Clip.BoolTracks)
		floatTargets := targets[nextTarget:floatEnd]
		textureTargets := targets[floatEnd:textureEnd]
		boolTargets := targets[textureEnd:boolEnd]
		nextTarget = boolEnd
		if err := resolveRuntimeAnimationTrackTargets(s, children, owner, animator.Clip.FloatTracks, animationTrackFloatTarget, claims, seen, floatTargets); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		if err := resolveRuntimeAnimationTrackTargets(s, children, owner, animator.Clip.TextureTracks, animationTrackTextureTarget, claims, seen, textureTargets); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		if err := resolveRuntimeAnimationTrackTargets(s, children, owner, animator.Clip.BoolTracks, animationTrackBoolTarget, claims, seen, boolTargets); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
	}
	nextTarget = 0
	for _, owner := range requestedOwners {
		node := s.nodes[owner]
		animator := ecs.GetMut[sceneapi.Animator](s.World, owner)
		if animator == nil || animator.Clip == nil {
			return fmt.Errorf("game object %q animation component is missing", node.Spec.ID)
		}
		floatEnd := nextTarget + len(animator.Clip.FloatTracks)
		textureEnd := floatEnd + len(animator.Clip.TextureTracks)
		boolEnd := textureEnd + len(animator.Clip.BoolTracks)
		animator.BindTargets(owner, targets[nextTarget:floatEnd], targets[floatEnd:textureEnd], targets[textureEnd:boolEnd])
		nextTarget = boolEnd
	}
	return nil
}

func (s *SceneInstance) bindRuntimeAnimationTargetsForSpec(spec SceneNodeSpec) error {
	capacity := countAnimationNodes(&spec)
	if capacity == 0 {
		return nil
	}
	owners := make([]ecs.Entity, 0, capacity)
	var collect func(*SceneNodeSpec)
	collect = func(value *SceneNodeSpec) {
		if value == nil {
			return
		}
		if value.Components.Animation != nil {
			entity, ok := s.Entities[strings.TrimSpace(value.ID)]
			if ok {
				owners = append(owners, entity)
			}
		}
		for index := range value.Children {
			collect(&value.Children[index])
		}
	}
	collect(&spec)
	return s.bindRuntimeAnimationTargets(owners...)
}

func (s *SceneInstance) bindRuntimeAnimationTargetsForEntity(entity ecs.Entity) error {
	if s == nil || s.World == nil || entity == 0 {
		return nil
	}
	var owners []ecs.Entity
	for remaining := s.World.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if node, exists := s.nodes[entity]; exists && node.Spec.Components.Animation != nil {
			owners = append(owners, entity)
		}
		parent, ok := ecs.Get[sceneapi.Parent](s.World, entity)
		if !ok {
			break
		}
		entity = parent.Entity
	}
	return s.bindRuntimeAnimationTargets(owners...)
}

func countAnimationNodes(spec *SceneNodeSpec) int {
	if spec == nil {
		return 0
	}
	count := 0
	if spec.Components.Animation != nil {
		count++
	}
	for index := range spec.Children {
		count += countAnimationNodes(&spec.Children[index])
	}
	return count
}

type animationTrackTargetKind uint8

const (
	animationTrackFloatTarget animationTrackTargetKind = iota
	animationTrackTextureTarget
	animationTrackBoolTarget
)

type runtimeAnimationChildKey struct {
	parent ecs.Entity
	id     string
}

type runtimeAnimationChild struct {
	entity    ecs.Entity
	ambiguous bool
}

type runtimeAnimationClaim struct {
	entity ecs.Entity
	target sceneapi.AnimationTarget
}

type runtimeAnimationSeen struct {
	owner  ecs.Entity
	entity ecs.Entity
	target sceneapi.AnimationTarget
}

func (s *SceneInstance) runtimeAnimationChildIndex() map[runtimeAnimationChildKey]runtimeAnimationChild {
	if s == nil || s.World == nil || len(s.nodes) == 0 {
		return nil
	}
	children := make(map[runtimeAnimationChildKey]runtimeAnimationChild, len(s.nodes))
	for entity, node := range s.nodes {
		parent, ok := ecs.Get[sceneapi.Parent](s.World, entity)
		if !ok {
			continue
		}
		localID := node.LocalID
		if localID == "" {
			localID = node.Spec.ID
		}
		key := runtimeAnimationChildKey{parent: parent.Entity, id: localID}
		if previous, exists := children[key]; exists {
			previous.entity = 0
			previous.ambiguous = true
			children[key] = previous
			continue
		}
		children[key] = runtimeAnimationChild{entity: entity}
	}
	return children
}

func (s *SceneInstance) resolveRuntimeAnimationPathIndexed(children map[runtimeAnimationChildKey]runtimeAnimationChild, owner ecs.Entity, path string) (ecs.Entity, error) {
	path = strings.TrimSpace(path)
	if err := validateAnimationBindingPath(path); err != nil {
		return 0, err
	}
	current := owner
	if path == "" {
		return current, nil
	}
	for remaining := path; remaining != ""; {
		segment, next, foundSeparator := strings.Cut(remaining, "/")
		if !foundSeparator {
			next = ""
		}
		child, exists := children[runtimeAnimationChildKey{parent: current, id: segment}]
		if !exists || child.entity == 0 {
			if child.ambiguous {
				return 0, fmt.Errorf("path %q is ambiguous at %q", path, segment)
			}
			return 0, fmt.Errorf("path %q does not name a descendant", path)
		}
		current = child.entity
		remaining = next
	}
	return current, nil
}

func (s *SceneInstance) claimExistingRuntimeAnimationTargets(children map[runtimeAnimationChildKey]runtimeAnimationChild, requested map[ecs.Entity]struct{}, claims map[runtimeAnimationClaim]ecs.Entity, seen map[runtimeAnimationSeen]struct{}) error {
	for owner, node := range s.nodes {
		if _, rebind := requested[owner]; rebind || node.Spec.Components.Animation == nil {
			continue
		}
		animator, ok := ecs.Get[sceneapi.Animator](s.World, owner)
		if !ok || animator.Clip == nil {
			continue
		}
		if err := resolveRuntimeAnimationTrackTargets(s, children, owner, animator.Clip.FloatTracks, animationTrackFloatTarget, claims, seen, nil); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		if err := resolveRuntimeAnimationTrackTargets(s, children, owner, animator.Clip.TextureTracks, animationTrackTextureTarget, claims, seen, nil); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
		if err := resolveRuntimeAnimationTrackTargets(s, children, owner, animator.Clip.BoolTracks, animationTrackBoolTarget, claims, seen, nil); err != nil {
			return fmt.Errorf("game object %q animation: %w", node.Spec.ID, err)
		}
	}
	return nil
}

func resolveRuntimeAnimationTrackTargets[T any](s *SceneInstance, children map[runtimeAnimationChildKey]runtimeAnimationChild, owner ecs.Entity, tracks []sceneapi.AnimationTrack[T], kind animationTrackTargetKind, claims map[runtimeAnimationClaim]ecs.Entity, seen map[runtimeAnimationSeen]struct{}, result []ecs.Entity) error {
	for index, track := range tracks {
		entity, err := s.resolveRuntimeAnimationPathIndexed(children, owner, track.Binding.Path)
		if err != nil {
			return fmt.Errorf("track %d path %q: %w", index, track.Binding.Path, err)
		}
		if err := validateRuntimeAnimationTarget(s.World, entity, track.Binding.Target, kind); err != nil {
			return fmt.Errorf("track %d: %w", index, err)
		}
		if claims != nil {
			if err := claimRuntimeAnimationBinding(s, claims, seen, owner, entity, track.Binding.Target); err != nil {
				return fmt.Errorf("track %d: %w", index, err)
			}
		}
		if index < len(result) {
			result[index] = entity
		}
	}
	return nil
}

func claimRuntimeAnimationTarget(s *SceneInstance, claims map[runtimeAnimationClaim]ecs.Entity, owner, entity ecs.Entity, target sceneapi.AnimationTarget) error {
	key := runtimeAnimationClaim{entity: entity, target: animationOwnershipTarget(target)}
	if previous, exists := claims[key]; exists {
		if previous != owner {
			return fmt.Errorf("target node %q property %q is already animated by node %q", runtimeAnimationNodeName(s, entity), target, runtimeAnimationNodeName(s, previous))
		}
		return nil
	}
	claims[key] = owner
	return nil
}

func claimRuntimeAnimationBinding(s *SceneInstance, claims map[runtimeAnimationClaim]ecs.Entity, seen map[runtimeAnimationSeen]struct{}, owner, entity ecs.Entity, target sceneapi.AnimationTarget) error {
	if seen != nil {
		key := runtimeAnimationSeen{owner: owner, entity: entity, target: target}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("target node %q property %q is bound more than once by node %q", runtimeAnimationNodeName(s, entity), target, runtimeAnimationNodeName(s, owner))
		}
		seen[key] = struct{}{}
	}
	return claimRuntimeAnimationTarget(s, claims, owner, entity, target)
}

func runtimeAnimationNodeName(s *SceneInstance, entity ecs.Entity) string {
	if s != nil {
		if id := s.ID(entity); id != "" {
			return id
		}
	}
	return fmt.Sprintf("%d", entity)
}

func validateRuntimeAnimationTarget(world *ecs.World, entity ecs.Entity, target sceneapi.AnimationTarget, kind animationTrackTargetKind) error {
	if entity == 0 {
		return fmt.Errorf("animation target entity is missing")
	}
	switch kind {
	case animationTrackTextureTarget:
		if target != sceneapi.AnimationTargetSpriteTexture || !ecs.Has[sceneapi.Sprite2D](world, entity) {
			return fmt.Errorf("target requires Sprite2D texture")
		}
	case animationTrackBoolTarget:
		if target != sceneapi.AnimationTargetVisibility || !ecs.Has[sceneapi.Visibility](world, entity) {
			return fmt.Errorf("target requires Visibility")
		}
	case animationTrackFloatTarget:
		switch target {
		case sceneapi.AnimationTargetSpriteAlpha:
			if !ecs.Has[sceneapi.Sprite2D](world, entity) {
				return fmt.Errorf("target requires Sprite2D")
			}
		case sceneapi.AnimationTargetTransformRotationX, sceneapi.AnimationTargetTransformRotationY:
			if !ecs.Has[sceneapi.Transform](world, entity) {
				return fmt.Errorf("X/Y rotation requires Transform")
			}
		case sceneapi.AnimationTargetTransformPositionZ, sceneapi.AnimationTargetTransformScaleZ:
			if !ecs.Has[sceneapi.Transform](world, entity) {
				return fmt.Errorf("Z transform animation requires Transform")
			}
		case sceneapi.AnimationTargetTransformPositionX, sceneapi.AnimationTargetTransformPositionY,
			sceneapi.AnimationTargetTransformScaleX, sceneapi.AnimationTargetTransformScaleY,
			sceneapi.AnimationTargetTransformRotationZ:
			if !ecs.Has[sceneapi.Transform](world, entity) && !ecs.Has[sceneapi.UITransform](world, entity) {
				return fmt.Errorf("target requires Transform or UITransform")
			}
		default:
			return fmt.Errorf("unsupported float animation target %q", target)
		}
	default:
		return fmt.Errorf("unsupported animation track kind")
	}
	return nil
}

func resolveAnimationEntities(indices []uint32, entities, result []ecs.Entity) error {
	if len(result) != len(indices) {
		return fmt.Errorf("animation target result length %d does not match plan length %d", len(result), len(indices))
	}
	for index, value := range indices {
		if value == 0 || int(value) > len(entities) || entities[value-1] == 0 {
			return fmt.Errorf("animation target plan index %d is invalid", value)
		}
		result[index] = entities[value-1]
	}
	return nil
}

func sprite2DForSpec(spec SceneNodeSpec, texture *graphics.Texture, tint graphics.ColorRGBA) (sceneapi.Sprite2D, error) {
	component := spec.Components.Sprite
	sprite := sceneapi.Sprite2DFromTexture(texture)
	sprite.Tint = tint
	sprite.Alpha = spec.Alpha
	if component.Texture != nil && component.Texture.Slice != nil {
		if err := component.Texture.Slice.Validate(texture.LogicalWidth(), texture.LogicalHeight()); err != nil {
			return sceneapi.Sprite2D{}, fmt.Errorf("game object %q texture slice: %w", spec.ID, err)
		}
		slice := component.Texture.Slice
		sprite.Slice = &sceneapi.NineSlice2D{
			Left: float32(slice.Left), Right: float32(slice.Right),
			Top: float32(slice.Top), Bottom: float32(slice.Bottom),
		}
	}
	if spec.Width > 0 {
		sprite.Width = float32(spec.Width)
	}
	if spec.Height > 0 {
		sprite.Height = float32(spec.Height)
	}
	return sprite, nil
}

func (b *Bundle) ecsTextForPlanNode(node sceneSpawnNode) (sceneapi.Text2D, error) {
	spec := node.Spec
	component := spec.Components.Text
	text := sceneapi.NewText2D(component.Value)
	text.Alpha = spec.Alpha
	if component.Font != "" {
		fontResource, err := b.resources.Font(component.Font)
		if err != nil {
			return sceneapi.Text2D{}, err
		}
		text.Font = fontResource
	}
	if component.FontSize > 0 {
		text.FontSize = component.FontSize
	}
	if err := configureTextLayout(&text, component, spec.ID); err != nil {
		return sceneapi.Text2D{}, err
	}
	text.OutlineWidth = component.OutlineWidth
	text.MaxWidth = component.MaxWidth
	if node.TextStyle.HasColor {
		text.Color = node.TextStyle.Color
	}
	if node.TextStyle.HasOutlineColor {
		text.OutlineColor = node.TextStyle.OutlineColor
	}
	if node.TextStyle.HasShadowColor {
		text.ShadowX = node.TextStyle.ShadowX
		text.ShadowY = node.TextStyle.ShadowY
		text.ShadowColor = node.TextStyle.ShadowColor
	}
	return text, nil
}

func configureTextLayout(text *sceneapi.Text2D, component *TextComponentSpec, id string) error {
	if component.WrapWidth < 0 || math.IsNaN(float64(component.WrapWidth)) || math.IsInf(float64(component.WrapWidth), 0) {
		return fmt.Errorf("game object %q wrap width must be finite and non-negative", id)
	}
	if component.LetterSpacing < 0 || math.IsNaN(float64(component.LetterSpacing)) || math.IsInf(float64(component.LetterSpacing), 0) {
		return fmt.Errorf("game object %q letter spacing must be finite and non-negative", id)
	}
	if component.LineHeight < 0 || math.IsNaN(float64(component.LineHeight)) || math.IsInf(float64(component.LineHeight), 0) {
		return fmt.Errorf("game object %q line height must be finite and non-negative", id)
	}
	text.WrapWidth = component.WrapWidth
	text.LetterSpacing = component.LetterSpacing
	text.LineHeight = component.LineHeight
	switch strings.ToLower(strings.TrimSpace(component.WrapMode)) {
	case "", "words":
		text.WrapMode = sceneapi.TextWrapWords
	case "newlines":
		text.WrapMode = sceneapi.TextWrapNewlines
	case "none":
		text.WrapMode = sceneapi.TextWrapNone
	default:
		return fmt.Errorf("game object %q unsupported text wrap mode %q", id, component.WrapMode)
	}
	switch strings.ToLower(strings.TrimSpace(component.TextAlignment)) {
	case "", "left":
		text.TextAlignment = sceneapi.TextAlignLeft
	case "center":
		text.TextAlignment = sceneapi.TextAlignCenter
	case "right":
		text.TextAlignment = sceneapi.TextAlignRight
	default:
		return fmt.Errorf("game object %q unsupported text alignment %q", id, component.TextAlignment)
	}
	return nil
}

func (b *Bundle) ecsSpineForGameObject(spec SceneNodeSpec) (sceneapi.Spine2D, error) {
	component := spec.Components.Spine
	resource, err := b.resources.Spine(component.Resource)
	if err != nil {
		return sceneapi.Spine2D{}, fmt.Errorf("game object %q: %w", spec.ID, err)
	}
	player := spine.NewPlayer(resource)
	player.Alpha = spec.Alpha
	player.Speed = component.Speed
	if component.Skin != "" {
		if err := player.SetSkin(component.Skin); err != nil {
			return sceneapi.Spine2D{}, fmt.Errorf("game object %q: %w", spec.ID, err)
		}
	}
	if component.Animation != "" {
		if err := player.SetAnimation(component.Animation, component.Loop); err != nil {
			return sceneapi.Spine2D{}, fmt.Errorf("game object %q: %w", spec.ID, err)
		}
	}
	return sceneapi.Spine2D{Player: player}, nil
}

func renderLayersFromSpec(values []uint) (sceneapi.RenderLayers, bool) {
	if len(values) == 0 {
		return sceneapi.RenderLayers{}, false
	}
	layers := sceneapi.RenderLayersNone()
	for _, layer := range values {
		layers = layers.With(layer)
	}
	return layers, true
}

func sceneSortIndex(world *ecs.World, entity ecs.Entity, ui bool, fallback int) int {
	if !ui {
		if order, ok := ecs.Get[sceneapi.RenderOrder2D](world, entity); ok {
			return int(order)
		}
		return fallback
	}
	for steps := 0; entity != 0 && steps <= world.EntityCount(); steps++ {
		if stackIndex, ok := ecs.Get[sceneapi.UIStackIndex](world, entity); ok {
			return int(stackIndex)
		}
		parent, ok := ecs.Get[sceneapi.Parent](world, entity)
		if !ok {
			break
		}
		entity = parent.Entity
	}
	return fallback
}

func applyRenderOrderSpec(world *ecs.World, entity ecs.Entity, spec SceneNodeSpec) {
	if specUsesUI(&spec) {
		ecs.Remove[sceneapi.RenderOrder2D](world, entity)
		setUIStackIndex(world, entity, spec.Order)
		return
	}
	ecs.Remove[sceneapi.UIStackIndex](world, entity)
	order := spec.Order
	if spec.Components.Particle != nil && order == 0 {
		order = int(sceneapi.DefaultParticleRenderOrder)
	}
	if current := ecs.GetMut[sceneapi.RenderOrder2D](world, entity); current != nil {
		*current = sceneapi.RenderOrder2D(order)
		return
	}
	_ = ecs.Insert(world, entity, sceneapi.RenderOrder2D(order))
}

func setUIStackIndex(world *ecs.World, entity ecs.Entity, stackIndex int) {
	if current := ecs.GetMut[sceneapi.UIStackIndex](world, entity); current != nil {
		*current = sceneapi.UIStackIndex(stackIndex)
		return
	}
	_ = ecs.Insert(world, entity, sceneapi.UIStackIndex(stackIndex))
}

func applyRenderLayersSpec(world *ecs.World, entity ecs.Entity, values []uint) {
	layers, ok := renderLayersFromSpec(values)
	if !ok {
		ecs.Remove[sceneapi.RenderLayers](world, entity)
		return
	}
	if current := ecs.GetMut[sceneapi.RenderLayers](world, entity); current != nil {
		*current = layers
		return
	}
	_ = ecs.Insert(world, entity, layers)
}

func applyUITargetCameraSpec(world *ecs.World, entity ecs.Entity, targetID string, entities map[string]ecs.Entity) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		ecs.Remove[sceneapi.UITargetCamera](world, entity)
		return nil
	}
	target, ok := entities[targetID]
	if !ok {
		return fmt.Errorf("targetCamera %q does not exist", targetID)
	}
	if !ecs.Has[sceneapi.Camera](world, target) {
		return fmt.Errorf("targetCamera %q is not a camera", targetID)
	}
	component := sceneapi.UITargetCamera{Entity: target}
	if current := ecs.GetMut[sceneapi.UITargetCamera](world, entity); current != nil {
		*current = component
		return nil
	}
	return ecs.Insert(world, entity, component)
}

func (b *Bundle) ecsParticleEmitterForSpec(spec ParticleSpec) (sceneapi.ParticleEmitter2D, error) {
	emitter := sceneapi.NewParticleEmitter2D()
	emitter.Width = spec.Width
	emitter.Height = spec.Height
	emitter.Count = spec.Count
	emitter.Rate = spec.Rate
	emitter.Lifetime = spec.Lifetime
	emitter.SpeedMin = spec.SpeedMin
	emitter.SpeedMax = spec.SpeedMax
	emitter.SizeMin = spec.SizeMin
	emitter.SizeMax = spec.SizeMax
	emitter.Direction = spec.Direction
	emitter.Spread = spec.Spread
	emitter.GravityX = spec.GravityX
	emitter.GravityY = spec.GravityY
	emitter.AlphaStart = spec.AlphaStart
	emitter.AlphaEnd = spec.AlphaEnd
	emitter.Loop = spec.Loop
	texture, err := b.resources.texture(spec.Texture)
	if err != nil {
		return sceneapi.ParticleEmitter2D{}, fmt.Errorf("particle %q texture: %w", spec.ID, err)
	}
	emitter.Texture = texture

	emitter.Colors = emitter.Colors[:0]
	for _, value := range spec.Colors {
		clr, err := b.resources.ParseColor(value, 1)
		if err != nil {
			return sceneapi.ParticleEmitter2D{}, err
		}
		emitter.Colors = append(emitter.Colors, clr)
	}
	emitter.Normalize()
	return emitter, nil
}

func (b *Bundle) ecsParticleEmitterForGameObject(gameObject SceneNodeSpec) (sceneapi.ParticleEmitter2D, error) {
	id := strings.TrimSpace(gameObject.Components.Particle.Resource)
	b.resourceSpecMu.RLock()
	spec, ok := b.particleSpecs[id]
	b.resourceSpecMu.RUnlock()
	if !ok {
		return sceneapi.ParticleEmitter2D{}, fmt.Errorf("game object %q references unknown particle %q", gameObject.ID, id)
	}
	emitter, err := b.ecsParticleEmitterForSpec(spec)
	if err != nil {
		return sceneapi.ParticleEmitter2D{}, err
	}
	return emitter, nil
}

func (b *Bundle) ecsTilemapForGameObject(gameObject SceneNodeSpec) (sceneapi.Tilemap2D, error) {
	id := strings.TrimSpace(gameObject.Components.Tilemap.Resource)
	b.resourceSpecMu.RLock()
	spec, ok := b.tilemapSpecs[id]
	b.resourceSpecMu.RUnlock()
	if !ok {
		return sceneapi.Tilemap2D{}, fmt.Errorf("game object %q references unknown tilemap %q", gameObject.ID, id)
	}
	var texture *graphics.Texture
	var err error
	if strings.TrimSpace(spec.Tileset) != "" {
		texture, err = b.resources.texture(&TextureSpec{Image: spec.Tileset})
		if err != nil {
			return sceneapi.Tilemap2D{}, fmt.Errorf("tilemap %q tileset: %w", id, err)
		}
		if texture.Width%spec.TileWidth != 0 || texture.Height%spec.TileHeight != 0 {
			return sceneapi.Tilemap2D{}, fmt.Errorf("tilemap %q tileset size must be divisible by %dx%d", id, spec.TileWidth, spec.TileHeight)
		}
	}
	return sceneapi.Tilemap2D{
		Texture: texture, Tiles: append([]int(nil), spec.Tiles...), Width: spec.Width, Height: spec.Height,
		TileWidth: spec.TileWidth, TileHeight: spec.TileHeight, Columns: func() int {
			if texture == nil {
				return 0
			}
			return texture.Width / spec.TileWidth
		}(), Alpha: gameObject.Alpha,
	}, nil
}

func sortSceneRenderItems(items []SceneRenderItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UI != items[j].UI {
			return !items[i].UI
		}
		if items[i].SortIndex != items[j].SortIndex {
			return items[i].SortIndex < items[j].SortIndex
		}
		if items[i].Depth != items[j].Depth {
			return items[i].Depth < items[j].Depth
		}
		return items[i].Sequence < items[j].Sequence
	})
}
