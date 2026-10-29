package scene

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/entitycache"
	"github.com/go-gl/mathgl/mgl32"
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
	entity    ecs.Entity
	parent    ecs.Entity
	transform Transform
	local     mgl32.Mat4
	matrix    mgl32.Mat4
	dirty     bool
	state     uint8
}

type transformWorkspace struct {
	global          ecs.ComponentBinding[GlobalTransform]
	bound           bool
	scope           *ecs.QueryScope
	scopeVersion    uint64
	nodes           entitycache.Store[transformNode]
	active          []ecs.Entity
	queue           []ecs.Entity
	structures      [2]uint64
	versions        [3]uint64
	lastRun         ecs.Tick
	changes         ecs.SystemContext
	parentStructure uint64
	initialized     bool
	recomputed      int
}

func transformRevisions(world *ecs.World) (versions [3]uint64, structures [2]uint64) {
	versions[0], structures[0] = world.ComponentValueRevision[Transform]()
	versions[1], _ = world.ComponentValueRevision[Parent]()
	versions[2], structures[1] = world.ComponentValueRevision[GlobalTransform]()
	return
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
	if !workspace.bound {
		workspace.global = world.BindComponent[GlobalTransform]()
		workspace.bound = true
	}
	scope, version := world.QueryScopeKey()
	if workspace.scope != scope || workspace.scopeVersion != version {
		workspace.initialized = false
		workspace.scope, workspace.scopeVersion = scope, version
	}
	workspace.recomputed = 0
	versions, structures := transformRevisions(world)
	if workspace.initialized && workspace.structures == structures && workspace.versions == versions {
		return nil
	}
	workspace.queue = workspace.queue[:0]
	hierarchy := HierarchyFor(world)
	if !workspace.initialized || workspace.structures != structures {
		workspace.nodes.Reset()
		workspace.active = workspace.active[:0]
		for entity, access := range world.Query[Transform]().All {
			transform := access.Get()

			node := workspace.nodes.Ensure(entity)
			*node = transformNode{entity: entity, transform: transform, local: transform.LocalMatrix(), dirty: true}
			if parent, ok := world.Get[Parent](entity); ok {
				node.parent = parent.Entity
			}
			workspace.active = append(workspace.active, entity)
			workspace.queue = append(workspace.queue, entity)
		}
	} else {
		changes := &workspace.changes
		since := workspace.lastRun
		if since == world.ChangeTick() {
			since--
		}
		*changes = world.ChangesSince(since)
		if workspace.versions[1] != versions[1] {
			refreshParent := func(entity ecs.Entity) {
				node := workspace.node(entity)
				if node == nil {
					return
				}
				parent, _ := world.Get[Parent](entity)
				if node.parent != parent.Entity {
					node.parent = parent.Entity
					workspace.mark(entity)
				}
			}
			_, parentStructure := world.ComponentValueRevision[Parent]()
			if workspace.parentStructure != parentStructure {
				for _, entity := range workspace.active {
					refreshParent(entity)
				}
			} else {
				for entity, _ := range world.Query[Parent](changes.Changed[Parent](), ecs.With[Transform]()).All {
					refreshParent(entity)
				}
			}
		}
		if workspace.versions[0] != versions[0] {
			for entity, access := range world.Query[Transform](changes.Changed[Transform]()).All {
				transform := access.Get()

				if node := workspace.node(entity); node != nil && node.transform != transform {
					node.transform, node.local = transform, transform.LocalMatrix()
					workspace.mark(entity)
				}
			}
		}
		if workspace.versions[2] != versions[2] {
			for entity, access := range world.Query[GlobalTransform](changes.Changed[GlobalTransform]()).All {
				transform := access.Get()

				if node := workspace.node(entity); node != nil && node.matrix != transform.Matrix {
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
		matrix, err := workspace.resolve(entity)
		if err != nil {
			workspace.initialized = false
			return err
		}
		access := workspace.global.Access(world.Row(entity))
		if global := access.Ref(); global != nil {
			if global.Matrix != matrix {
				ecs.CommitComponent(access, GlobalTransform{Matrix: matrix})
			}
			continue
		}
		if err := world.Insert(entity, GlobalTransform{Matrix: matrix}); err != nil {
			return err
		}
	}
	_, workspace.parentStructure = world.ComponentValueRevision[Parent]()
	workspace.initialized = true
	workspace.versions, workspace.structures = transformRevisions(world)
	workspace.lastRun = world.ChangeTick()
	return nil
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

func (workspace *transformWorkspace) resolve(entity ecs.Entity) (mgl32.Mat4, error) {
	node := workspace.node(entity)
	if node == nil {
		return mgl32.Ident4(), nil
	}
	if !node.dirty {
		return node.matrix, nil
	}
	if node.state == 1 {
		return mgl32.Mat4{}, fmt.Errorf("ecs: cycle detected in Parent hierarchy at entity %d", entity)
	}
	node.state = 1
	parent, err := workspace.resolve(node.parent)
	if err != nil {
		return mgl32.Mat4{}, err
	}
	node.matrix, node.dirty, node.state = parent.Mul4(node.local), false, 0
	workspace.recomputed++
	return node.matrix, nil
}
