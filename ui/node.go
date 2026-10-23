package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// NodeOptions describes an unrotated UI node with unit scale. Zero options
// create a visible top-left node at the origin; Hidden explicitly hides it.
// A nil Anchor selects TopLeft; use &scene.AnchorCenter for a centered node.
type NodeOptions struct {
	Parent     ecs.Entity
	X, Y       float32
	StackIndex int
	Hidden     bool
	Anchor     *scene.Anchor
}

func (o NodeOptions) anchor() scene.Anchor {
	if o.Anchor != nil {
		return *o.Anchor
	}
	return scene.AnchorTopLeft
}

// NodeSpawner binds the six common UI components plus T to one archetype.
// T must differ from the fixed components; duplicates panic at construction.
type NodeSpawner[T any] struct {
	spawner ecs.Spawner7[scene.UITransform, scene.UIGlobalTransform, scene.Parent, scene.Anchor, scene.Visibility, scene.UIStackIndex, T]
}

func NewNodeSpawner[T any](world *ecs.World) NodeSpawner[T] {
	return NodeSpawner[T]{spawner: ecs.NewSpawner7[scene.UITransform, scene.UIGlobalTransform, scene.Parent, scene.Anchor, scene.Visibility, scene.UIStackIndex, T](world)}
}

func (s *NodeSpawner[T]) Reserve(capacity int) { s.spawner.Reserve(capacity) }

func (s *NodeSpawner[T]) Spawn(options NodeOptions, value T) ecs.Entity {
	return s.spawner.Spawn(scene.UITransformFromXY(options.X, options.Y), scene.UIGlobalTransform{},
		scene.Parent{Entity: options.Parent}, options.anchor(), scene.Visibility{Visible: !options.Hidden},
		scene.UIStackIndex(options.StackIndex), value)
}

// NodeSpawner2 creates nodes with two additional components in the same write.
// Use it for e.g. Size2D plus a visual, avoiding a follow-up Insert and migration.
type NodeSpawner2[A, B any] struct {
	spawner ecs.Spawner8[scene.UITransform, scene.UIGlobalTransform, scene.Parent, scene.Anchor, scene.Visibility, scene.UIStackIndex, A, B]
}

func NewNodeSpawner2[A, B any](world *ecs.World) NodeSpawner2[A, B] {
	return NodeSpawner2[A, B]{spawner: ecs.NewSpawner8[scene.UITransform, scene.UIGlobalTransform, scene.Parent, scene.Anchor, scene.Visibility, scene.UIStackIndex, A, B](world)}
}

func (s *NodeSpawner2[A, B]) Reserve(capacity int) { s.spawner.Reserve(capacity) }

func (s *NodeSpawner2[A, B]) Spawn(options NodeOptions, a A, b B) ecs.Entity {
	return s.spawner.Spawn(scene.UITransformFromXY(options.X, options.Y), scene.UIGlobalTransform{},
		scene.Parent{Entity: options.Parent}, options.anchor(), scene.Visibility{Visible: !options.Hidden},
		scene.UIStackIndex(options.StackIndex), a, b)
}

type nodeFactory[T any] struct {
	nodes       NodeSpawner[T]
	initialized bool
}

func nodeSpawnerFor[T any](world *ecs.World) *NodeSpawner[T] {
	factory := ecs.FrameValue[nodeFactory[T], nodeFactory[T]](world)
	if !factory.initialized {
		factory.nodes = NewNodeSpawner[T](world)
		factory.initialized = true
	}
	return &factory.nodes
}

// SpawnNode reuses a factory owned by world. Retain a NewNodeSpawner result
// when a hot loop should avoid even the factory's type-key lookup.
func SpawnNode[T any](world *ecs.World, options NodeOptions, value T) ecs.Entity {
	return nodeSpawnerFor[T](world).Spawn(options, value)
}

type nodeFactory2[A, B any] struct {
	nodes       NodeSpawner2[A, B]
	initialized bool
}

func nodeSpawner2For[A, B any](world *ecs.World) *NodeSpawner2[A, B] {
	factory := ecs.FrameValue[nodeFactory2[A, B], nodeFactory2[A, B]](world)
	if !factory.initialized {
		factory.nodes = NewNodeSpawner2[A, B](world)
		factory.initialized = true
	}
	return &factory.nodes
}

func SpawnNode2[A, B any](world *ecs.World, options NodeOptions, a A, b B) ecs.Entity {
	return nodeSpawner2For[A, B](world).Spawn(options, a, b)
}
