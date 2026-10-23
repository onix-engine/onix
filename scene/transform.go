package scene

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
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
	entity      ecs.Entity
	parent      ecs.Entity
	firstChild  ecs.Entity
	nextSibling ecs.Entity
	transform   Transform
	local       mgl32.Mat4
	matrix      mgl32.Mat4
	dirty       bool
	state       uint8
}

type transformWorkspace struct {
	scope           *ecs.QueryScope
	scopeVersion    uint64
	nodes           []transformNode
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
	versions[0], structures[0] = ecs.ComponentRevision[Transform](world)
	versions[1], _ = ecs.ComponentRevision[Parent](world)
	versions[2], structures[1] = ecs.ComponentRevision[GlobalTransform](world)
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
	workspace := ecs.ScopeValue[transformWorkspaceTag, transformWorkspace](world)
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
	if !workspace.initialized || workspace.structures != structures {
		for _, entity := range workspace.active {
			index, _ := entity.Index()
			workspace.nodes[index].entity = 0
		}
		workspace.active = workspace.active[:0]
		if len(workspace.nodes) < world.EntitySlots() {
			workspace.nodes = append(workspace.nodes, make([]transformNode, world.EntitySlots()-len(workspace.nodes))...)
		}
		for entity, access := range ecs.Query[Transform](world).All {
			transform := access.Get()

			index, _ := entity.Index()
			node := &workspace.nodes[index]
			*node = transformNode{entity: entity, transform: transform, local: transform.LocalMatrix(), dirty: true}
			if parent, ok := ecs.Get[Parent](world, entity); ok {
				node.parent = parent.Entity
			}
			workspace.active = append(workspace.active, entity)
			workspace.queue = append(workspace.queue, entity)
		}
		for _, entity := range workspace.active {
			node := workspace.node(entity)
			if parent := workspace.node(node.parent); parent != nil {
				node.nextSibling, parent.firstChild = parent.firstChild, entity
			}
		}
	} else {
		changes := &workspace.changes
		since := workspace.lastRun
		if since == world.ChangeTick() {
			since--
		}
		*changes = world.ChangesSince(since)
		if workspace.versions[1] != versions[1] {
			topologyChanged := false
			refreshParent := func(entity ecs.Entity) {
				node := workspace.node(entity)
				if node == nil {
					return
				}
				parent, _ := ecs.Get[Parent](world, entity)
				if node.parent != parent.Entity {
					node.parent = parent.Entity
					workspace.mark(entity)
					topologyChanged = true
				}
			}
			if workspace.parentStructure != ecs.ComponentStructureVersion[Parent](world) {
				for _, entity := range workspace.active {
					refreshParent(entity)
				}
			} else {
				for entity, _ := range ecs.Query[Parent](world, ecs.Changed[Parent](changes), ecs.With[Transform]()).All {
					refreshParent(entity)
				}
			}
			if topologyChanged {
				for _, entity := range workspace.active {
					node := workspace.node(entity)
					node.firstChild, node.nextSibling = 0, 0
				}
				for _, entity := range workspace.active {
					node := workspace.node(entity)
					if parent := workspace.node(node.parent); parent != nil {
						node.nextSibling, parent.firstChild = parent.firstChild, entity
					}
				}
			}
		}
		if workspace.versions[0] != versions[0] {
			for entity, access := range ecs.Query[Transform](world, ecs.Changed[Transform](changes)).All {
				transform := access.Get()

				if node := workspace.node(entity); node != nil && node.transform != transform {
					node.transform, node.local = transform, transform.LocalMatrix()
					workspace.mark(entity)
				}
			}
		}
		if workspace.versions[2] != versions[2] {
			for entity, access := range ecs.Query[GlobalTransform](world, ecs.Changed[GlobalTransform](changes)).All {
				transform := access.Get()

				if node := workspace.node(entity); node != nil && node.matrix != transform.Matrix {
					workspace.mark(entity)
				}
			}
		}
		for index := 0; index < len(workspace.queue); index++ {
			for child := workspace.node(workspace.queue[index]).firstChild; child != 0; child = workspace.node(child).nextSibling {
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
		if global := ecs.GetRef[GlobalTransform](world, entity); global != nil {
			if global.Matrix != matrix {
				updated := ecs.GetMut[GlobalTransform](world, entity)
				updated.Matrix = matrix
			}
			continue
		}
		if err := ecs.Insert(world, entity, GlobalTransform{Matrix: matrix}); err != nil {
			return err
		}
	}
	workspace.parentStructure = ecs.ComponentStructureVersion[Parent](world)
	workspace.initialized = true
	workspace.versions, workspace.structures = transformRevisions(world)
	workspace.lastRun = world.ChangeTick()
	return nil
}

func (workspace *transformWorkspace) node(entity ecs.Entity) *transformNode {
	index, ok := entity.Index()
	if !ok || int(index) >= len(workspace.nodes) || workspace.nodes[index].entity != entity {
		return nil
	}
	return &workspace.nodes[index]
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
