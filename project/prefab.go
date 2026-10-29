package project

import (
	"fmt"
	iofs "io/fs"
	"strconv"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/sourcejson"
	sceneapi "github.com/diiyw/onix/scene"
)

// PrefabID identifies one reusable node hierarchy registered by resources.json.
type PrefabID string

// PrefabResourceSpec registers one prefab source file in resources.json.
type PrefabResourceSpec struct {
	ID   PrefabID `json:"id"`
	Path string   `json:"path"`
}

// prefabManifest intentionally contains one root. A prefab is a mountable
// scene fragment, rather than an independently switchable scene.
type prefabManifest struct {
	Prepared  bool           `json:"-"`
	SpawnPlan sceneSpawnPlan `json:"-"`
	Root      SceneNodeSpec  `json:"root"`
}

type prefabPlan struct {
	plan        sceneSpawnPlan
	nodeIndices map[string]uint32
}

// PrefabInstance owns one mounted prefab hierarchy. Entity resolves prefab
// local IDs without rebuilding the instance's namespaced scene IDs.
type PrefabInstance struct {
	scene    *SceneInstance
	prefab   *prefabPlan
	entities []ecs.Entity
	rootID   string
}

// NormalizePrefabPath validates and canonicalizes a prefab resource path.
func NormalizePrefabPath(value string) (string, error) {
	return normalizeResourceFilePath[prefabResourceFile](value)
}

func normalizePrefabResourceSpecs(specs []PrefabResourceSpec) ([]PrefabResourceSpec, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	normalized := make([]PrefabResourceSpec, len(specs))
	seen := make(map[PrefabID]struct{}, len(specs))
	for index := range specs {
		spec := specs[index]
		spec.ID = PrefabID(strings.TrimSpace(string(spec.ID)))
		if spec.ID == "" {
			return nil, fmt.Errorf("prefab with empty id")
		}
		if _, exists := seen[spec.ID]; exists {
			return nil, fmt.Errorf("duplicate prefab id %q", spec.ID)
		}
		path, err := NormalizePrefabPath(spec.Path)
		if err != nil {
			return nil, fmt.Errorf("prefab %q path %q must be a relative *.prefab.json path", spec.ID, spec.Path)
		}
		spec.Path = path
		seen[spec.ID] = struct{}{}
		normalized[index] = spec
	}
	return normalized, nil
}

func loadPrefabResources(source iofs.FS, projectDir string, specs []PrefabResourceSpec) (map[PrefabID]*prefabPlan, []PrefabID, error) {
	normalized, err := normalizePrefabResourceSpecs(specs)
	if err != nil {
		return nil, nil, err
	}
	plans := make(map[PrefabID]*prefabPlan, len(normalized))
	ids := make([]PrefabID, 0, len(normalized))
	for _, spec := range normalized {
		prefabPath, err := ResolveResourcePath(projectDir, spec.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve prefab %q: %w", spec.ID, err)
		}
		manifest, err := readJSON[prefabManifest](source, prefabPath)
		if err != nil {
			return nil, nil, fmt.Errorf("load prefab %q: %w", spec.ID, err)
		}
		if !manifest.Prepared {
			if !sourcejson.Enabled {
				return nil, nil, fmt.Errorf("prefab %q: release builds require a preprocessed spawn plan", spec.ID)
			}
			if err := compilePrefabManifest(string(spec.ID), &manifest); err != nil {
				return nil, nil, err
			}
		}
		plan, err := newPrefabPlan(spec.ID, manifest.SpawnPlan)
		if err != nil {
			return nil, nil, err
		}
		plans[spec.ID] = plan
		ids = append(ids, spec.ID)
	}
	return plans, ids, nil
}

func compilePrefabManifest(name string, manifest *prefabManifest) error {
	if manifest == nil {
		return fmt.Errorf("prefab %q is nil", name)
	}
	scene := sceneManifest{Nodes: []SceneNodeSpec{manifest.Root}}
	if err := prepareSceneManifest(name, &scene); err != nil {
		return err
	}
	plan, err := compileSceneSpawnPlan(scene.Nodes)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	manifest.SpawnPlan = plan
	manifest.Root = SceneNodeSpec{}
	manifest.Prepared = true
	return nil
}

func newPrefabPlan(id PrefabID, plan sceneSpawnPlan) (*prefabPlan, error) {
	if len(plan.Nodes) == 0 || plan.Nodes[0].Parent != 0 {
		return nil, fmt.Errorf("prefab %q requires one root node", id)
	}
	indices := make(map[string]uint32, len(plan.Nodes))
	for index, node := range plan.Nodes {
		if node.Parent == 0 && index != 0 {
			return nil, fmt.Errorf("prefab %q requires exactly one root node", id)
		}
		if _, exists := indices[node.Spec.ID]; exists {
			return nil, fmt.Errorf("prefab %q contains duplicate node id %q", id, node.Spec.ID)
		}
		indices[node.Spec.ID] = uint32(index)
	}
	return &prefabPlan{plan: plan, nodeIndices: indices}, nil
}

// MountPrefab instantiates a registered prefab under parent. instanceID is
// used only to namespace scene entity IDs; prefab-local IDs remain available
// through the returned PrefabInstance.
func (s *SceneInstance) MountPrefab(parent ecs.Entity, id PrefabID, instanceID string) (*PrefabInstance, error) {
	return s.mountPrefab(parent, id, instanceID, true)
}

func (s *SceneInstance) mountPrefab(parent ecs.Entity, id PrefabID, instanceID string, refresh bool) (*PrefabInstance, error) {
	if s == nil || s.project == nil || s.World == nil {
		return nil, fmt.Errorf("scene instance is nil")
	}
	if !s.World.IsAlive(parent) {
		return nil, fmt.Errorf("prefab parent entity %d does not exist", parent)
	}
	id = PrefabID(strings.TrimSpace(string(id)))
	plan, ok := s.project.prefabPlansByID[id]
	if !ok {
		return nil, fmt.Errorf("prefab %q is not registered by %s", id, resourceFileName)
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("prefab instance id is required")
	}
	if plan.plan.Nodes[0].UI != s.World.Has[sceneapi.UITransform](parent) {
		return nil, fmt.Errorf("prefab %q coordinate space does not match parent entity %d", id, parent)
	}
	if s.nodes == nil {
		s.nodes = make(map[ecs.Entity]sceneSpawnNode)
	}
	if s.Entities == nil {
		s.Entities = make(map[string]ecs.Entity)
	}
	if s.EntityIDs == nil {
		s.EntityIDs = make(map[ecs.Entity]string)
	}
	if s.scriptBindings == nil {
		s.scriptBindings = make(map[ecs.Entity][]ScriptBinding)
	}
	if s.inputBindings == nil {
		s.inputBindings = make(map[ecs.Entity][]scriptInputBinding)
	}

	entityIDs := make([]string, len(plan.plan.Nodes))
	for index, node := range plan.plan.Nodes {
		entityIDs[index] = prefabEntityID(instanceID, node.Spec.ID)
		if _, exists := s.Entities[entityIDs[index]]; exists {
			return nil, fmt.Errorf("prefab instance %q conflicts with scene entity %q", instanceID, entityIDs[index])
		}
	}
	entities := make([]ecs.Entity, len(plan.plan.Nodes))
	rollback := func() {
		s.removeNodes(false, entityIDs[0])
	}
	for index, sourceNode := range plan.plan.Nodes {
		node := sourceNode
		node.Spec.ID = entityIDs[index]
		parentEntity := parent
		if node.Parent > 0 {
			parentEntity = entities[node.Parent-1]
		}
		components, err := s.project.ecsComponentsForPlanNode(node)
		if err != nil {
			rollback()
			return nil, err
		}
		components.Put(sceneapi.Parent{Entity: parentEntity})
		entity := s.World.Spawn(components)
		if node.Spec.BlockInput && (node.Spec.Components.Has2D() || node.Spec.Control != nil) {
			_ = s.World.Insert(entity, sceneapi.InputBlocker2D{
				Width: float32(node.Spec.Width), Height: float32(node.Spec.Height), PassThrough: node.Spec.PassThrough,
			})
		}
		if err := indexSceneEntity(node.Spec, entity, s.Entities, s.EntityIDs); err != nil {
			rollback()
			return nil, err
		}
		s.nodes[entity] = node
		if s.scriptsBound {
			if err := s.bindNodeScripts(entity, node.Spec); err != nil {
				rollback()
				return nil, err
			}
		}
		entities[index] = entity
	}
	for index, node := range plan.plan.Nodes {
		if node.TargetCamera != 0 {
			if err := s.World.Insert(entities[index], sceneapi.UITargetCamera{Entity: entities[node.TargetCamera-1]}); err != nil {
				rollback()
				return nil, fmt.Errorf("prefab %q node %q targetCamera: %w", id, node.Spec.ID, err)
			}
		}
	}
	if err := bindSceneAnimationTargets(s.World, plan.plan, entities); err != nil {
		rollback()
		return nil, err
	}
	if refresh {
		if err := s.RefreshTransforms(); err != nil {
			rollback()
			return nil, err
		}
	}
	return &PrefabInstance{scene: s, prefab: plan, entities: entities, rootID: entityIDs[0]}, nil
}

func prefabEntityID(instanceID, localID string) string {
	return instanceID + "." + localID
}

// Root returns the prefab root entity while it remains alive.
func (p *PrefabInstance) Root() ecs.Entity {
	if p == nil || p.scene == nil || p.scene.World == nil || len(p.entities) == 0 || !p.scene.World.IsAlive(p.entities[0]) {
		return 0
	}
	return p.entities[0]
}

// Entity resolves one prefab-local node ID while the instance remains alive.
func (p *PrefabInstance) Entity(id string) (ecs.Entity, bool) {
	if p == nil || p.scene == nil || p.scene.World == nil || p.prefab == nil {
		return 0, false
	}
	index, ok := p.prefab.nodeIndices[strings.TrimSpace(id)]
	if !ok || int(index) >= len(p.entities) {
		return 0, false
	}
	entity := p.entities[index]
	return entity, p.scene.World.IsAlive(entity)
}

// SetVisible changes visibility for every node in the prefab hierarchy.
func (p *PrefabInstance) SetVisible(visible bool) {
	if p == nil || p.scene == nil || p.scene.World == nil {
		return
	}
	for _, entity := range p.entities {
		if value := p.scene.World.GetMut[sceneapi.Visibility](entity); value != nil {
			value.Visible = visible
		}
	}
}

// Reset restores authored transforms, visibility, ordering, and animation
// playback. Component-specific runtime state remains under the caller's
// control, which keeps pooled effect activation allocation-free.
func (p *PrefabInstance) Reset() bool {
	if p == nil || p.scene == nil || p.scene.World == nil || p.prefab == nil || len(p.entities) != len(p.prefab.plan.Nodes) {
		return false
	}
	for index, entity := range p.entities {
		if !p.scene.World.IsAlive(entity) {
			return false
		}
		node := p.prefab.plan.Nodes[index]
		if node.UI {
			if transform := p.scene.World.GetMut[sceneapi.UITransform](entity); transform != nil {
				*transform = node.UITransform.Component()
			}
			if stack := p.scene.World.GetMut[sceneapi.UIStackIndex](entity); stack != nil {
				*stack = sceneapi.UIStackIndex(node.Spec.Order)
			}
		} else {
			if transform := p.scene.World.GetMut[sceneapi.Transform](entity); transform != nil {
				*transform = node.Transform
			}
			if order := p.scene.World.GetMut[sceneapi.RenderOrder2D](entity); order != nil {
				*order = sceneapi.RenderOrder2D(node.Spec.Order)
			}
		}
		if anchor := p.scene.World.GetMut[sceneapi.Anchor](entity); anchor != nil {
			*anchor = node.Spec.Anchor
		}
		if size := p.scene.World.GetMut[sceneapi.Size2D](entity); size != nil {
			size.Width, size.Height = float32(node.Spec.Width), float32(node.Spec.Height)
		}
		if visibility := p.scene.World.GetMut[sceneapi.Visibility](entity); visibility != nil {
			visibility.Visible = node.Visible
		}
		if animator := p.scene.World.GetMut[sceneapi.Animator](entity); animator != nil {
			animator.Restart()
		}
		if trail := p.scene.World.GetMut[sceneapi.SpriteTrail2D](entity); trail != nil {
			trail.Clear()
		}
	}
	return true
}

// Remove despawns this prefab hierarchy from its owning scene.
func (p *PrefabInstance) Remove() int {
	if p == nil || p.scene == nil || p.rootID == "" {
		return 0
	}
	return p.scene.RemoveNodes(p.rootID)
}

// PrefabPool eagerly mounts a fixed number of prefab instances. Acquire
// cycles through slots, reclaiming the oldest slot when the pool is full.
type PrefabPool struct {
	instances []PrefabInstance
	next      int
}

// NewPrefabPool mounts capacity instances under parent. It performs all
// entity allocation during setup; Acquire and Release allocate nothing.
func (s *SceneInstance) NewPrefabPool(parent ecs.Entity, id PrefabID, prefix string, capacity int) (*PrefabPool, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("prefab pool capacity must be positive")
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "-")
	if prefix == "" {
		return nil, fmt.Errorf("prefab pool prefix is required")
	}
	pool := &PrefabPool{instances: make([]PrefabInstance, capacity)}
	for index := range pool.instances {
		instance, err := s.mountPrefab(parent, id, prefix+"-"+strconv.Itoa(index), false)
		if err != nil {
			pool.Dispose()
			return nil, err
		}
		pool.instances[index] = *instance
	}
	if err := s.RefreshTransforms(); err != nil {
		pool.Dispose()
		return nil, err
	}
	return pool, nil
}

// Len returns the fixed number of mounted prefab instances.
func (p *PrefabPool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.instances)
}

// Instance returns a stable pool slot by index.
func (p *PrefabPool) Instance(index int) *PrefabInstance {
	if p == nil || index < 0 || index >= len(p.instances) {
		return nil
	}
	return &p.instances[index]
}

// Acquire returns the next fixed slot and restores its authored base state.
func (p *PrefabPool) Acquire() *PrefabInstance {
	_, instance := p.AcquireSlot()
	return instance
}

// AcquireSlot returns the next fixed slot index and restores its authored
// base state. The index lets callers keep parallel value-type runtime state
// without a per-acquire lookup or allocation.
func (p *PrefabPool) AcquireSlot() (int, *PrefabInstance) {
	if p == nil || len(p.instances) == 0 {
		return -1, nil
	}
	index := p.next
	instance := &p.instances[index]
	p.next = (index + 1) % len(p.instances)
	if !instance.Reset() {
		return index, nil
	}
	return index, instance
}

// Release hides an instance owned by this pool.
func (p *PrefabPool) Release(instance *PrefabInstance) bool {
	if p == nil || instance == nil {
		return false
	}
	for index := range p.instances {
		if &p.instances[index] != instance {
			continue
		}
		instance.SetVisible(false)
		return true
	}
	return false
}

// Dispose removes every mounted pool instance.
func (p *PrefabPool) Dispose() {
	if p == nil {
		return
	}
	for index := range p.instances {
		p.instances[index].Remove()
	}
	p.instances = nil
	p.next = 0
}
