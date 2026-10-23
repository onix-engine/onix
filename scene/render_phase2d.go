package scene

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

type renderPhaseItem2D interface {
	renderSortKey() renderSortKey2D
	renderMaterial() renderMaterialKey
}

// Read both keys through a pointer so traversal does not copy the full render
// item (which also contains transforms, materials and geometry references).
type renderPhaseReader2D[T any] interface {
	*T
	renderPhaseKeys() (renderSortKey2D, renderMaterialKey)
}

type renderSortKey2D struct {
	scene     uint64
	ui        bool
	sortIndex int
	depth     float32
	order     uint64
	sequence  uint32
}

type renderBatch2D struct {
	scene     uint64
	start     int
	end       int
	ui        bool
	sortIndex int
	material  renderMaterialKey
}

func compareRenderPhaseItem2D[T renderPhaseItem2D](a, b T) int {
	aKey, bKey := a.renderSortKey(), b.renderSortKey()
	if aKey.scene < bKey.scene {
		return -1
	}
	if aKey.scene > bKey.scene {
		return 1
	}
	if aKey.ui != bKey.ui {
		if aKey.ui {
			return 1
		}
		return -1
	}
	if aKey.sortIndex < bKey.sortIndex {
		return -1
	}
	if aKey.sortIndex > bKey.sortIndex {
		return 1
	}
	if aKey.depth < bKey.depth {
		return -1
	}
	if aKey.depth > bKey.depth {
		return 1
	}
	if aKey.order < bKey.order {
		return -1
	}
	if aKey.order > bKey.order {
		return 1
	}
	if aKey.sequence < bKey.sequence {
		return -1
	}
	if aKey.sequence > bKey.sequence {
		return 1
	}
	return 0
}

// drawRenderPhase2D consumes sorted items, preserving order while gathering
// ranges only for renderers that require a complete batch (such as Spine).
func drawRenderPhase2D[T renderPhaseItem2D, P renderPhaseReader2D[T]](c *graphics.Canvas2D, world *ecs.World, items []T, views DrawViews2D, cull bool, batched func(*T) bool, draw func(renderBatch2D, renderViewport2D, bool)) {
	currentUI := false
	spaceReady := false
	var viewport renderViewport2D
	var previous renderBatch2D
	for start := 0; start < len(items); {
		key, material := P(&items[start]).renderPhaseKeys()
		batch := renderBatch2D{scene: key.scene, start: start, end: start + 1, ui: key.ui, sortIndex: key.sortIndex, material: material}
		begin := start == 0 || previous.scene != batch.scene || previous.ui != batch.ui || previous.sortIndex != batch.sortIndex || previous.material != batch.material

		if !spaceReady || currentUI != batch.ui {
			currentUI = batch.ui
			spaceReady = true
			if cull {
				viewport = renderViewport2DForSpace(c, world, batch.ui)
				if batch.ui {
					c.ResetProjectionMatrix()
					c.SetViewMatrix(mgl32.Ident4())
				} else if camera, projection, transform, ok := activeCamera2DComponents(world); ok {
					viewport := camera.ResolvedViewport()
					c.SetProjectionMatrix(projection.Matrix(viewport.Width, viewport.Height))
					c.SetViewMatrix(transform.Matrix.Inv())
				}
			} else {
				c.ResetProjectionMatrix()
				if batch.ui {
					c.SetViewMatrix(views.UI)
				} else {
					c.SetViewMatrix(views.World)
				}
				viewport = renderViewport2D{clipFromLayer: c.ProjectionMatrix().Mul4(c.ViewMatrix()), active: true}
			}
		}
		if begin && batched != nil && batched(&items[start]) {
			for batch.end < len(items) {
				next, nextMaterial := P(&items[batch.end]).renderPhaseKeys()
				if next.scene != key.scene || next.ui != key.ui || next.sortIndex != key.sortIndex || nextMaterial != material {
					break
				}
				batch.end++
			}
		}
		draw(batch, viewport, begin)
		previous = batch
		start = batch.end
	}
}
