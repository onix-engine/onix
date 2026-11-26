package scene

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/internal/entitycache"
	"github.com/onix-engine/onix/internal/reuse"
)

type Name struct {
	Value string
}

type Parent struct {
	Entity ecs.Entity
}

type Visibility struct {
	Visible bool
}

// Transform is the local spatial transform shared by 2D and 3D world entities.
// Two-dimensional entities use the Z translation for depth ordering and normally
// rotate around Z with a unit Z scale.
type Transform struct {
	Translation mgl32.Vec3
	Rotation    mgl32.Quat
	Scale       mgl32.Vec3
}

type GlobalTransform struct {
	Matrix mgl32.Mat4
}

func (t GlobalTransform) renderMatrix2D() mgl32.Mat4 { return t.Matrix }
func (GlobalTransform) renderClip2D() clipRect2D     { return clipRect2D{} }

func NewTransform() Transform {
	return Transform{Rotation: mgl32.QuatIdent(), Scale: mgl32.Vec3{1, 1, 1}}
}

func TransformFromTranslation(translation mgl32.Vec3) Transform {
	transform := NewTransform()
	transform.Translation = translation
	return transform
}

func TransformFromXYZ(x, y, z float32) Transform {
	return TransformFromTranslation(mgl32.Vec3{x, y, z})
}

func (t Transform) LocalMatrix() mgl32.Mat4 {
	scale := t.Scale
	if scale == (mgl32.Vec3{}) {
		scale = mgl32.Vec3{1, 1, 1}
	}
	rotation := t.Rotation
	if rotation.Len() == 0 {
		rotation = mgl32.QuatIdent()
	}
	return mgl32.Translate3D(t.Translation.X(), t.Translation.Y(), t.Translation.Z()).
		Mul4(rotation.Normalize().Mat4()).
		Mul4(mgl32.Scale3D(scale.X(), scale.Y(), scale.Z()))
}

func (t Transform) LookingAt(target, up mgl32.Vec3) Transform {
	if target == t.Translation || up.Len() == 0 {
		return t
	}
	t.Rotation = mgl32.Mat4ToQuat(mgl32.LookAtV(t.Translation, target, up).Inv()).Normalize()
	return t
}

type transformWorkspaceTag struct{}

type transformNode struct {
	parent    ecs.Entity
	transform Transform
	local     mgl32.Mat4
	dirty     bool
	state     uint8
}

type transformWorkspace struct {
	changes            ecs.SystemContext
	global             ecs.ComponentBinding[GlobalTransform]
	queue              []ecs.Entity
	nodes              entitycache.Store[transformNode]
	versions, detached [3]uint64
	entityVersion      uint64
	retention          reuse.Retention
	scopeVersion       uint64
	recomputed         int
	lastRun            ecs.Tick
	initialized        bool
}

func SyncGlobalTransformSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_global_transform"
	}
	return ecs.SystemConfig{Name: name, System: syncGlobalTransforms}
}

func syncGlobalTransforms(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	workspace := world.ScopeValue[transformWorkspaceTag, transformWorkspace]()
	_, scopeVersion := world.QueryScopeKey()
	workspace.nodes.Trim()
	workspace.queue = workspace.retention.Trim(workspace.queue[:0], 64)
	workspace.recomputed = 0
	versions := [3]uint64{componentValueVersion[Transform](world), componentValueVersion[Parent](world), componentValueVersion[GlobalTransform](world)}
	entityVersion := world.EntityVersion()
	rebuild := !workspace.initialized || workspace.scopeVersion != scopeVersion
	if !rebuild && workspace.versions == versions {
		workspace.entityVersion, workspace.lastRun = entityVersion, world.ChangeTick()
		return nil
	}
	if !workspace.initialized {
		workspace.global = world.BindComponent[GlobalTransform]()
	}
	hierarchy := HierarchyFor(world)
	detached := [3]uint64{world.ComponentDetachRevision[Transform](), world.ComponentDetachRevision[Parent](), world.ComponentDetachRevision[GlobalTransform]()}
	if rebuild || detached[0] != workspace.detached[0] || detached[2] != workspace.detached[2] ||
		(entityVersion != workspace.entityVersion && !workspace.retireEntities(world, hierarchy)) {
		workspace.nodes.Reset()
		for entity, access := range world.Query[Transform]().All {
			workspace.add(entity, access.Get(), hierarchy.Parent(entity))
		}
	} else {
		// Other systems can write after this cache was read in the same tick.
		changes := &workspace.changes
		*changes = world.ChangesSince(workspace.lastRun - 1)
		if workspace.versions[1] != versions[1] {
			refreshParent := func(entity ecs.Entity, node *transformNode) {
				if parent := hierarchy.Parent(entity); node != nil && node.parent != parent {
					node.parent = parent
					workspace.mark(entity)
				}
			}
			if detached[1] != workspace.detached[1] {
				for entity, node := range workspace.nodes.All {
					refreshParent(entity, node)
				}
			} else {
				for entity, _ := range world.Query[Parent](changes.Changed[Parent](), ecs.With[Transform]()).All {
					refreshParent(entity, workspace.node(entity))
				}
			}
		}
		if workspace.versions[0] != versions[0] {
			for entity, access := range world.Query[Transform](changes.Changed[Transform]()).All {
				transform := access.Get()
				if node := workspace.node(entity); node == nil {
					workspace.add(entity, transform, hierarchy.Parent(entity))
				} else if node.transform != transform {
					node.transform, node.local = transform, transform.LocalMatrix()
					workspace.mark(entity)
				}
			}
		}
		if workspace.versions[2] != versions[2] {
			for entity, access := range world.Query[GlobalTransform](changes.Changed[GlobalTransform]()).All {
				if node := workspace.node(entity); node != nil && !node.dirty && workspace.expectedMatrix(world, node) != access.Ref().Matrix {
					workspace.mark(entity)
				}
			}
		}
		for index := 0; index < len(workspace.queue); index++ {
			for child := hierarchy.FirstChild(workspace.queue[index]); child != 0; child = hierarchy.NextSibling(child) {
				workspace.mark(child)
			}
		}
	}
	for _, entity := range workspace.queue {
		if _, err := workspace.resolve(world, entity); err != nil {
			workspace.initialized = false
			return err
		}
	}
	versions[2] = componentValueVersion[GlobalTransform](world)
	workspace.versions, workspace.detached = versions, detached
	workspace.initialized, workspace.scopeVersion, workspace.entityVersion = true, scopeVersion, entityVersion
	workspace.retention.Record(len(workspace.queue))
	workspace.lastRun = world.ChangeTick()
	return nil
}

func (workspace *transformWorkspace) add(entity ecs.Entity, transform Transform, parent ecs.Entity) {
	*workspace.nodes.Ensure(entity) = transformNode{parent: parent, transform: transform, local: transform.LocalMatrix(), dirty: true}
	workspace.queue = append(workspace.queue, entity)
}

func (workspace *transformWorkspace) retireEntities(world *ecs.World, hierarchy *Hierarchy) bool {
	// The work queue is empty until the dirty transforms are collected.
	changes, complete := world.EntityChangesSince(workspace.entityVersion, workspace.queue[:0])
	workspace.queue = changes[:0]
	workspace.retention.Record(len(changes))
	if !complete {
		return false
	}
	for _, entity := range changes {
		if !world.IsAlive(entity) && workspace.node(entity) != nil {
			// Surviving children of a despawned parent have lost their inherited matrix.
			if hierarchy.missingParents[entity] != 0 {
				return false
			}
			workspace.nodes.Delete(entity)
		}
	}
	return true
}

func (workspace *transformWorkspace) node(entity ecs.Entity) *transformNode {
	return workspace.nodes.Get(entity)
}

func (workspace *transformWorkspace) mark(entity ecs.Entity) {
	node := workspace.node(entity)
	if node != nil && !node.dirty {
		node.dirty, node.state = true, 0
		workspace.queue = append(workspace.queue, entity)
	}
}

func (workspace *transformWorkspace) resolve(world *ecs.World, entity ecs.Entity) (mgl32.Mat4, error) {
	node := workspace.node(entity)
	if node == nil {
		return mgl32.Ident4(), nil
	}
	if !node.dirty {
		if value, ok := workspace.global.Get(world.Row(entity)); ok {
			return value.Matrix, nil
		}
	}
	if node.state == 1 {
		return mgl32.Mat4{}, fmt.Errorf("ecs: cycle detected in Parent hierarchy at entity %d", entity)
	}
	node.state = 1
	parent, err := workspace.resolve(world, node.parent)
	if err != nil {
		return mgl32.Mat4{}, err
	}
	matrix := parent.Mul4(node.local)
	access := workspace.global.Access(world.Row(entity))
	if access.Ref() != nil {
		ecs.CommitComponent(access, GlobalTransform{Matrix: matrix})
	} else if err := world.Insert(entity, GlobalTransform{Matrix: matrix}); err != nil {
		return mgl32.Mat4{}, err
	}
	node.dirty, node.state = false, 0
	workspace.recomputed++
	return matrix, nil
}

func (workspace *transformWorkspace) expectedMatrix(world *ecs.World, node *transformNode) mgl32.Mat4 {
	parent := mgl32.Ident4()
	if workspace.node(node.parent) != nil {
		if global, ok := workspace.global.Get(world.Row(node.parent)); ok {
			parent = global.Matrix
		}
	}
	return parent.Mul4(node.local)
}
