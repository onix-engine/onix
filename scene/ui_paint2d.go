package scene

import (
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

// UIShapeFragment2D is a paint-only rectangle in a UI entity's local
// coordinates. It shares the entity's transform, clipping and camera routing.
type UIShapeFragment2D struct {
	StackOffset         int
	Shape               Shape2D
	X, Y, Width, Height float32
}

// UIPaint2D owns the paint fragments attached to one UI entity.
type UIPaint2D struct {
	fragments []UIShapeFragment2D
}

// Fragments returns the entity's fragments for read-only access.
func (paint *UIPaint2D) Fragments() []UIShapeFragment2D {
	return paint.fragments
}

// SetUIShapeFragments2D replaces the paint fragments, reusing their storage.
func SetUIShapeFragments2D(world *ecs.World, entity ecs.Entity, fragments []UIShapeFragment2D) {
	paint := world.GetRef[UIPaint2D](entity)
	if paint == nil {
		if len(fragments) != 0 {
			_ = world.Insert(entity, UIPaint2D{fragments: slices.Clone(fragments)})
		}
		return
	}
	if !slices.Equal(paint.fragments, fragments) {
		paint := world.GetMut[UIPaint2D](entity)
		paint.fragments = append(paint.fragments[:0], fragments...)
	}
}

// SetUIShapeFragment2D updates the geometry and paint of a single fragment.
func SetUIShapeFragment2D(world *ecs.World, entity ecs.Entity, index int, fragment UIShapeFragment2D) {
	paint := world.GetRef[UIPaint2D](entity)
	if paint != nil && index >= 0 && index < len(paint.fragments) && paint.fragments[index] != fragment {
		world.GetMut[UIPaint2D](entity).fragments[index] = fragment
	}
}

// SetUIShapeFragmentFill2D updates the fill of a single fragment.
func SetUIShapeFragmentFill2D(world *ecs.World, entity ecs.Entity, index int, color graphics.ColorRGBA) {
	paint := world.GetRef[UIPaint2D](entity)
	if paint != nil && index >= 0 && index < len(paint.fragments) && paint.fragments[index].Shape.Fill != color {
		world.GetMut[UIPaint2D](entity).fragments[index].Shape.Fill = color
	}
}

func collectUIPaint2D(c *graphics.Canvas2D, world *ecs.World, snapshots *renderSnapshots2D, hierarchy *renderHierarchy2DCache, items []renderItem2D, cull bool) []renderItem2D {
	viewport := renderViewport2DForSpace(c, world, true)
	for entity, access := range world.Query3[UIPaint2D, UIGlobalTransform, Visibility]().All {
		transform := access.B.Ref()
		if !access.C.Ref().Visible {
			continue
		}
		alpha := float32(1)
		if sprite := world.GetRef[Sprite2D](entity); sprite != nil {
			alpha = sprite.Alpha
		} else if text := world.GetRef[Text2D](entity); text != nil {
			alpha = text.Alpha
		}
		if alpha <= 0 {
			continue
		}
		info := renderEntityInfo2D(world, entity, true, 0, hierarchy)
		for _, fragment := range access.A.Ref().fragments {
			shape, width, height := fragment.Shape, fragment.Width, fragment.Height
			if width <= 0 || height <= 0 || shape.Kind == 0 || shape.Fill.A <= 0 && shape.Border.A <= 0 {
				continue
			}
			matrix := transform.Matrix.Mul4(mgl32.Translate3D(fragment.X, fragment.Y, 0))
			// Retained fragments stay addressable when fully clipped. A scroll
			// can expose them via a transform-only update.
			if cull && !renderItemVisible2D(viewport, matrix, AnchorTopLeft, width, height, true, transform.clip, true) {
				continue
			}
			sprite := snapshots.addSprite(Sprite2D{Shape: shape, Width: width, Height: height, Alpha: alpha})
			item := newSpriteRenderItem2D(uint32(len(items)), entity, sprite, nil, AnchorTopLeft, matrix, transform.clip, width, height, true, info.sortIndex+fragment.StackOffset)
			item.entity, item.layers, item.target, item.scene, item.group = entity, info.layers, info.target, info.scene, info.group
			items = append(items, item)
		}
	}
	return items
}
