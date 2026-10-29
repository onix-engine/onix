package scene

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/entitycache"
	"github.com/go-gl/mathgl/mgl32"
)

type UITransform struct {
	Translation Val2
	Scale       mgl32.Vec2
	Rotation    float32
	Skew        mgl32.Vec2
	// Pivot is the local point around which rotation, skew, and scale are applied.
	Pivot mgl32.Vec2
	// LinearMatrix overrides rotation, skew, and scale when nonzero. It preserves
	// exact composition, including singular scales; pivot and translation still apply.
	LinearMatrix mgl32.Mat4
}

type UIGlobalTransform struct {
	Matrix mgl32.Mat4
	clip   clipRect2D
}

func (t UIGlobalTransform) renderMatrix2D() mgl32.Mat4 { return t.Matrix }
func (t UIGlobalTransform) renderClip2D() clipRect2D   { return t.clip }

// ClipRect returns the inherited clipping bounds in world UI coordinates.
// The boolean distinguishes an empty clipping rectangle from no clipping.
func (t UIGlobalTransform) ClipRect() (ClipRect2D, bool) {
	if !t.clip.valid {
		return ClipRect2D{}, false
	}
	return ClipRect2D{X: t.clip.minX, Y: t.clip.minY, Width: t.clip.maxX - t.clip.minX, Height: t.clip.maxY - t.clip.minY}, true
}

func NewUITransform() UITransform {
	return UITransform{Translation: Px2(0, 0), Scale: mgl32.Vec2{1, 1}}
}

func UITransformFromXY(x, y float32) UITransform {
	transform := NewUITransform()
	transform.Translation = Px2(x, y)
	return transform
}

func (t UITransform) LocalMatrix(context UIResolveContext) mgl32.Mat4 {
	translation := t.Translation.Resolve(context)
	transformed := t.LinearMatrix
	if transformed == (mgl32.Mat4{}) {
		transformed = mgl32.HomogRotate3DZ(t.Rotation).
			Mul4(mgl32.Mat4{
				1, float32(math.Tan(float64(t.Skew.Y()))), 0, 0,
				float32(math.Tan(float64(t.Skew.X()))), 1, 0, 0,
				0, 0, 1, 0,
				0, 0, 0, 1,
			}).
			Mul4(mgl32.Scale3D(t.Scale.X(), t.Scale.Y(), 1))
	}
	if t.Pivot.X() != 0 || t.Pivot.Y() != 0 {
		transformed = mgl32.Translate3D(t.Pivot.X(), t.Pivot.Y(), 0).
			Mul4(transformed).
			Mul4(mgl32.Translate3D(-t.Pivot.X(), -t.Pivot.Y(), 0))
	}
	return mgl32.Translate3D(translation.X(), translation.Y(), 0).Mul4(transformed)
}

type resolvedUITransform struct {
	matrix   mgl32.Mat4
	clip     clipRect2D
	revision uint64
}

type uiTransformNode struct {
	entity           ecs.Entity
	transform        UITransform
	parent           ecs.Entity
	localClip        ClipRect2D
	hasClip          bool
	localRoundedClip RoundedClip2D
	hasRoundedClip   bool
	clipExempt       bool
	state            uint8
	resolved         resolvedUITransform
	local            mgl32.Mat4
	baseSize         mgl32.Vec2
	parentRevision   uint64
	seen             uint64
	dirty            bool
}

type uiTransformBindings struct {
	world     *ecs.World
	transform ecs.ComponentBinding[UITransform]
	parent    ecs.ComponentBinding[Parent]
	clip      ecs.ComponentBinding[ClipRect2D]
	rounded   ecs.ComponentBinding[RoundedClip2D]
	exempt    ecs.ComponentBinding[ClipExempt2D]
	size      ecs.ComponentBinding[Size2D]
	sprite    ecs.ComponentBinding[Sprite2D]
	text      ecs.ComponentBinding[Text2D]
	global    ecs.ComponentBinding[UIGlobalTransform]
}

func (b *uiTransformBindings) bind(world *ecs.World) {
	if b.world == world {
		return
	}
	b.world = world
	b.transform = world.BindComponent[UITransform]()
	b.parent = world.BindComponent[Parent]()
	b.clip = world.BindComponent[ClipRect2D]()
	b.rounded = world.BindComponent[RoundedClip2D]()
	b.exempt = world.BindComponent[ClipExempt2D]()
	b.size = world.BindComponent[Size2D]()
	b.sprite = world.BindComponent[Sprite2D]()
	b.text = world.BindComponent[Text2D]()
	b.global = world.BindComponent[UIGlobalTransform]()
}

type uiTransformWorkspace struct {
	bindings     uiTransformBindings
	scope        *ecs.QueryScope
	scopeVersion uint64
	nodes        entitycache.Store[uiTransformNode]
	active       []ecs.Entity
	versions     [9]uint64
	structures   [9]uint64
	queue        []ecs.Entity
	changes      ecs.SystemContext
	lastRun      ecs.Tick
	viewport     mgl32.Vec2
	scaleFactor  float32
	generation   uint64
	initialized  bool
	recomputed   int
}

func uiTransformRevisions(world *ecs.World) (versions, structures [9]uint64) {
	versions[0], structures[0] = world.ComponentValueRevision[UITransform]()
	versions[1], structures[1] = world.ComponentValueRevision[Parent]()
	versions[2], structures[2] = world.ComponentValueRevision[ClipRect2D]()
	versions[3], structures[3] = world.ComponentValueRevision[RoundedClip2D]()
	versions[4], structures[4] = world.ComponentValueRevision[ClipExempt2D]()
	versions[5], structures[5] = world.ComponentValueRevision[Size2D]()
	versions[6], structures[6] = world.ComponentValueRevision[Sprite2D]()
	versions[7], structures[7] = world.ComponentValueRevision[Text2D]()
	versions[8], structures[8] = world.ComponentValueRevision[UIGlobalTransform]()
	return
}

type uiTransformWorkspaceScratch struct{}

const (
	uiTransformUnvisited uint8 = iota
	uiTransformVisiting
	uiTransformResolved
)

func SyncUIGlobalTransformSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_ui_global_transform"
	}
	return ecs.SystemConfig{Name: name, System: syncUIGlobalTransforms}
}

func syncUIGlobalTransforms(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}

	workspace := world.ScopeValue[uiTransformWorkspaceScratch, uiTransformWorkspace]()
	workspace.bindings.bind(world)
	scope, version := world.QueryScopeKey()
	if workspace.scope != scope || workspace.scopeVersion != version {
		workspace.nodes.Reset()
		workspace.active = workspace.active[:0]
		workspace.initialized = false
		workspace.scope, workspace.scopeVersion = scope, version
	}
	workspace.recomputed = 0
	viewport := mgl32.Vec2{}
	scaleFactor := float32(1)
	if size, ok := world.GetResource[ViewportSize](); ok {
		viewport = mgl32.Vec2{size.Width, size.Height}
		if size.ScaleFactor > 0 {
			scaleFactor = size.ScaleFactor
		}
	}
	versions, structures := uiTransformRevisions(world)
	viewportChanged := workspace.viewport != viewport || workspace.scaleFactor != scaleFactor
	if workspace.initialized && workspace.structures == structures && workspace.versions == versions && !viewportChanged {
		return nil
	}
	workspace.generation++
	workspace.queue = workspace.queue[:0]
	if !workspace.initialized {
		for _, entity := range workspace.active {
			node := workspace.node(entity)
			node.dirty, node.state = false, uiTransformUnvisited
		}
	}
	hierarchy := HierarchyFor(world)
	if !workspace.initialized || workspace.structures != structures {
		for _, entity := range workspace.active {
			if parent := workspace.node(workspace.node(entity).parent); parent != nil && !world.Has[UITransform](parent.entity) {
				workspace.mark(entity)
			}
		}
		for _, entity := range workspace.active {
			if !world.Has[UITransform](entity) {
				workspace.nodes.Delete(entity)
			}
		}
		workspace.active = workspace.active[:0]
		for entity, _ := range world.Query[UITransform]().All {
			workspace.refresh(world, entity, viewport, scaleFactor, !workspace.initialized || viewportChanged)
			workspace.active = append(workspace.active, entity)
		}
	} else if viewportChanged {
		for _, entity := range workspace.active {
			workspace.refresh(world, entity, viewport, scaleFactor, true)
		}
	} else {
		changes := &workspace.changes
		since := workspace.lastRun
		if since == world.ChangeTick() {
			since--
		}
		*changes = world.ChangesSince(since)
		refresh := func(entity ecs.Entity) {
			workspace.refresh(world, entity, viewport, scaleFactor, false)
		}
		if workspace.versions[0] != versions[0] {
			collectUITransformChanges[UITransform](world, changes, refresh)
		}
		if workspace.versions[1] != versions[1] {
			collectUITransformChanges[Parent](world, changes, refresh)
		}
		if workspace.versions[2] != versions[2] {
			collectUITransformChanges[ClipRect2D](world, changes, refresh)
		}
		if workspace.versions[3] != versions[3] {
			collectUITransformChanges[RoundedClip2D](world, changes, refresh)
		}
		if workspace.versions[4] != versions[4] {
			collectUITransformChanges[ClipExempt2D](world, changes, refresh)
		}
		if workspace.versions[5] != versions[5] {
			collectUITransformChanges[Size2D](world, changes, refresh)
		}
		if workspace.versions[6] != versions[6] {
			collectUITransformChanges[Sprite2D](world, changes, refresh)
		}
		if workspace.versions[7] != versions[7] {
			collectUITransformChanges[Text2D](world, changes, refresh)
		}
		if workspace.versions[8] != versions[8] {
			for entity, access := range world.Query[UIGlobalTransform](changes.Changed[UIGlobalTransform](), ecs.With[UITransform]()).All {
				value := access.Get()

				if node := workspace.node(entity); node != nil && (node.resolved.matrix != value.Matrix || node.resolved.clip != value.clip) {
					workspace.mark(entity)
				}
			}
		}
	}
	for index := 0; index < len(workspace.queue); index++ {
		node := workspace.node(workspace.queue[index])
		if node == nil {
			continue
		}
		for child := hierarchy.FirstChild(node.entity); child != 0; child = hierarchy.NextSibling(child) {
			workspace.mark(child)
		}
	}

	for _, entity := range workspace.queue {
		if workspace.node(entity) == nil {
			continue
		}
		resolved, err := resolveUITransform(workspace, entity)
		if err != nil {
			workspace.initialized = false
			return err
		}
		access := workspace.bindings.global.Access(world.Row(entity))
		if global := access.Ref(); global != nil {
			if global.Matrix != resolved.matrix || global.clip != resolved.clip {
				ecs.CommitComponent(access, UIGlobalTransform{Matrix: resolved.matrix, clip: resolved.clip})
			}
			continue
		}
		if err := world.Insert(entity, UIGlobalTransform{Matrix: resolved.matrix, clip: resolved.clip}); err != nil {
			return err
		}
	}
	workspace.initialized = true
	workspace.versions, workspace.structures = uiTransformRevisions(world)
	workspace.lastRun = world.ChangeTick()
	workspace.viewport, workspace.scaleFactor = viewport, scaleFactor
	return nil
}

func collectUITransformChanges[T any](world *ecs.World, changes *ecs.SystemContext, refresh func(ecs.Entity)) {
	for entity, _ := range world.Query[T](changes.Changed[T](), ecs.With[UITransform]()).All {
		refresh(entity)
	}
}

func (workspace *uiTransformWorkspace) node(entity ecs.Entity) *uiTransformNode {
	return workspace.nodes.Get(entity)
}

func (workspace *uiTransformWorkspace) mark(entity ecs.Entity) {
	if node := workspace.node(entity); node != nil && !node.dirty {
		node.dirty, node.state = true, uiTransformUnvisited
		workspace.queue = append(workspace.queue, entity)
	}
}

func (workspace *uiTransformWorkspace) refresh(world *ecs.World, entity ecs.Entity, viewport mgl32.Vec2, scaleFactor float32, force bool) {
	node := workspace.nodes.Ensure(entity)
	if node.seen == workspace.generation && node.entity == entity {
		return
	}
	if node.entity != entity {
		*node = uiTransformNode{}
	}
	row := world.Row(entity)
	b := &workspace.bindings
	transform, _ := b.transform.Get(row)
	parent, _ := b.parent.Get(row)
	clip, hasClip := b.clip.Get(row)
	rounded, hasRounded := b.rounded.Get(row)
	exempt := b.exempt.Access(row).Ref() != nil
	baseSize := b.baseSize(row)
	global, hasGlobal := b.global.Get(row)
	topologyChanged := node.entity != entity || node.parent != parent.Entity
	dirty := force || topologyChanged || !hasGlobal || global.Matrix != node.resolved.matrix || global.clip != node.resolved.clip || node.transform != transform || node.baseSize != baseSize ||
		node.localClip != clip || node.hasClip != hasClip || node.localRoundedClip != rounded || node.hasRoundedClip != hasRounded || node.clipExempt != exempt
	node.entity, node.seen = entity, workspace.generation
	node.transform, node.parent, node.baseSize = transform, parent.Entity, baseSize
	node.localClip, node.hasClip, node.localRoundedClip, node.hasRoundedClip, node.clipExempt = clip, hasClip, rounded, hasRounded, exempt
	if dirty {
		node.local = transform.LocalMatrix(UIResolveContext{ScaleFactor: scaleFactor, BaseSize: baseSize, Viewport: viewport})
		workspace.mark(entity)
	}
}

func resolveUITransform(workspace *uiTransformWorkspace, entity ecs.Entity) (resolvedUITransform, error) {
	node := workspace.node(entity)
	if node == nil {
		return resolvedUITransform{matrix: mgl32.Ident4()}, nil
	}
	if node.state == uiTransformResolved {
		return node.resolved, nil
	}
	if node.state == uiTransformVisiting {
		return resolvedUITransform{}, fmt.Errorf("ecs: cycle detected in UI Parent hierarchy at entity %d", entity)
	}
	node.state = uiTransformVisiting
	parentTransform := resolvedUITransform{matrix: mgl32.Ident4()}
	if node.parent != 0 {
		var err error
		parentTransform, err = resolveUITransform(workspace, node.parent)
		if err != nil {
			return resolvedUITransform{}, err
		}
	}
	if !node.dirty && node.parentRevision == parentTransform.revision {
		node.state = uiTransformResolved
		return node.resolved, nil
	}
	resolved := resolvedUITransform{matrix: parentTransform.matrix.Mul4(node.local), clip: parentTransform.clip, revision: node.resolved.revision + 1}
	if node.clipExempt {
		resolved.clip = clipRect2D{}
	}
	if node.hasClip {
		resolved.clip = intersectClipRect2D(resolved.clip, clipRectFromLocal2D(resolved.matrix, node.localClip))
	}
	if node.hasRoundedClip {
		resolved.clip = addRoundedClip2D(resolved.clip, roundedClipFromLocal2D(resolved.matrix, node.localRoundedClip))
	}
	node.resolved = resolved
	node.dirty = false
	node.parentRevision = parentTransform.revision
	workspace.recomputed++
	node.state = uiTransformResolved
	return resolved, nil
}

func (b *uiTransformBindings) baseSize(row ecs.EntityRow) mgl32.Vec2 {
	if size, ok := b.size.Get(row); ok {
		return mgl32.Vec2{size.Width, size.Height}
	}
	if sprite := b.sprite.Access(row).Ref(); sprite != nil {
		width, height := sprite.Size()
		return mgl32.Vec2{width, height}
	}
	if text := b.text.Access(row).Ref(); text != nil {
		width, height, _ := MeasureText2D(text)
		return mgl32.Vec2{width, height}
	}
	if clip, ok := b.clip.Get(row); ok {
		return mgl32.Vec2{clip.Width, clip.Height}
	}
	return mgl32.Vec2{}
}
