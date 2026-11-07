package scene

import (
	"runtime"
	"slices"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/internal/smallsort"
	"github.com/go-gl/mathgl/mgl32"
)

func buildRenderPhase2DBatches[T renderPhaseItem2D](items []T) []renderBatch2D {
	return buildRenderPhase2DBatchesInto(items, nil)
}

func TestStreamRenderPhase2DExplicitViewsCullInCanvasSpace(t *testing.T) {
	canvas := graphics.Canvas2DContext(400, 300)
	world := ecs.NewWorld()
	items := []renderItem2D{{}, {ui: true}}
	views := DrawViews2D{World: mgl32.Translate3D(200, 150, 0).Mul4(mgl32.Scale3D(2, -2, 1)), UI: mgl32.Translate3D(200, 150, 0)}
	drawRenderPhase2D(canvas, world, items, views, false, nil, func(batch renderBatch2D, viewport renderViewport2D, _ bool) {
		// Negative module coordinates land inside the canvas after the view transform.
		if !viewport.visible(mgl32.Translate3D(-80, -60, 0), 20, 20) {
			t.Fatalf("ui=%v: visible negative coordinates were culled", batch.ui)
		}
		if viewport.visible(mgl32.Translate3D(1000, 1000, 0), 20, 20) {
			t.Fatalf("ui=%v: off-canvas geometry was not culled", batch.ui)
		}
	})
}

func buildRenderPhase2DBatchesInto[T renderPhaseItem2D](items []T, batches []renderBatch2D) []renderBatch2D {
	smallsort.SortFunc(items, compareRenderPhaseItem2D[T])

	if len(items) == 0 {
		return batches[:0]
	}
	batches = batches[:0]
	if cap(batches) < len(items) {
		batches = make([]renderBatch2D, 0, len(items))
	}
	for start := 0; start < len(items); {
		end := start + 1
		key := items[start].renderSortKey()
		material := items[start].renderMaterial()
		for end < len(items) {
			nextKey := items[end].renderSortKey()
			if nextKey.ui != key.ui || nextKey.sortIndex != key.sortIndex || items[end].renderMaterial() != material {
				break
			}
			end++
		}
		batches = append(batches, renderBatch2D{
			start:     start,
			end:       end,
			ui:        key.ui,
			sortIndex: key.sortIndex,
			material:  material,
		})
		start = end
	}
	return batches
}

func TestStreamRenderPhase2DPreservesMixedBatchBoundaries(t *testing.T) {
	items := []renderItem2D{
		{order: 8, kind: renderItemSprite2D, ui: true, material: renderMaterialKey{textureID: 2}},
		{order: 3, kind: renderItemSpine2D, material: renderMaterialKey{textureID: 3}},
		{order: 1, kind: renderItemSprite2D, material: renderMaterialKey{textureID: 1}},
		{order: 2, kind: renderItemSprite2D, material: renderMaterialKey{textureID: 1}},
		{order: 4, kind: renderItemSpine2D, material: renderMaterialKey{textureID: 3}},
		{order: 5, kind: renderItemSprite2D, material: renderMaterialKey{textureID: 1}},
		{order: 6, kind: renderItemShape2D, material: renderMaterialKey{textureID: 4}},
		{order: 7, kind: renderItemSprite2D, material: renderMaterialKey{textureID: 1}},
	}
	world := ecs.NewWorld()
	canvas := graphics.Canvas2DContext(320, 180)
	batched := func(item *renderItem2D) bool { return item.kind == renderItemSpine2D }
	var orders []uint64
	var begins []uint64
	var sizes []int
	smallsort.SortFunc(items, compareRenderPhaseItem2D[renderItem2D])
	drawRenderPhase2D(canvas, world, items, DrawViews2D{}, false, batched, func(batch renderBatch2D, _ renderViewport2D, begin bool) {
		if begin {
			begins = append(begins, items[batch.start].order)
		}
		sizes = append(sizes, batch.end-batch.start)
		for _, item := range items[batch.start:batch.end] {
			orders = append(orders, item.order)
		}
	})
	if !slices.Equal(orders, []uint64{1, 2, 3, 4, 5, 6, 7, 8}) || !slices.Equal(begins, []uint64{1, 3, 5, 6, 7, 8}) || !slices.Equal(sizes, []int{1, 1, 2, 1, 1, 1, 1}) {
		t.Fatalf("orders=%v begins=%v sizes=%v", orders, begins, sizes)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		drawRenderPhase2D(canvas, world, items, DrawViews2D{}, false, batched, func(renderBatch2D, renderViewport2D, bool) {})
	}); allocations != 0 {
		t.Fatalf("stream allocations = %v", allocations)
	}
}

func TestIndexedRenderPhase2DPreservesFilteredBatches(t *testing.T) {
	items := []renderItem2D{
		{order: 1, kind: renderItemSprite2D},
		{order: 2, kind: renderItemShape2D},
		{order: 3, kind: renderItemSpine2D},
		{order: 4, kind: renderItemShape2D},
		{order: 5, kind: renderItemSpine2D},
		{order: 6, kind: renderItemSprite2D},
		{order: 7, kind: renderItemSprite2D, ui: true},
	}
	for i := range items {
		items[i].material.textureID = uint64(items[i].kind)
	}
	indices := []uint32{0, 2, 4, 5, 6}
	filtered := renderItemList2D{items: items, indices: indices, count: len(indices)}
	world, canvas := ecs.NewWorld(), graphics.Canvas2DContext(320, 180)
	var orders []uint64
	var sizes []int
	batched := func(item *renderItem2D) bool { return item.kind == renderItemSpine2D }
	drawRenderPhaseList2D[renderItem2D, *renderItem2D](canvas, world, filtered, DrawViews2D{}, false, batched, func(batch renderBatch2D, _ renderViewport2D, _ bool) {
		subset := filtered.slice(batch.start, batch.end)
		sizes = append(sizes, subset.len())
		for i := 0; i < subset.len(); i++ {
			if subset.at(i) != &items[indices[batch.start+i]] {
				t.Fatal("filtered batch copied a render item")
			}
			orders = append(orders, subset.at(i).order)
		}
	})
	if !slices.Equal(orders, []uint64{1, 3, 5, 6, 7}) || !slices.Equal(sizes, []int{1, 2, 1, 1}) {
		t.Fatalf("filtered orders=%v sizes=%v", orders, sizes)
	}
	drawRenderPhaseList2D[renderItem2D, *renderItem2D](canvas, world, renderItemList2D{items: items}, DrawViews2D{}, false, batched, func(renderBatch2D, renderViewport2D, bool) {
		t.Fatal("empty camera filter drew an unfiltered item")
	})
	if allocations := testing.AllocsPerRun(100, func() {
		drawRenderPhaseList2D[renderItem2D, *renderItem2D](canvas, world, filtered, DrawViews2D{}, false, batched, func(renderBatch2D, renderViewport2D, bool) {})
	}); allocations != 0 {
		t.Fatalf("filtered traversal allocated %v times", allocations)
	}
}

func BenchmarkRenderPhase2DTraversal(b *testing.B) {
	for _, alternating := range []bool{false, true} {
		name := "shared"
		if alternating {
			name = "alternating"
		}
		b.Run(name, func(b *testing.B) {
			items := make([]renderItem2D, 10000)
			for i := range items {
				items[i].order = uint64(i)
				if alternating {
					items[i].material.textureID = uint64(i % 2)
				}
			}
			canvas, world := graphics.Canvas2DContext(320, 180), ecs.NewWorld()
			var total uint64
			b.Run("materialized", func(b *testing.B) {
				batches := buildRenderPhase2DBatches(items)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					batches = buildRenderPhase2DBatchesInto(items, batches)
					for _, batch := range batches {
						for i := batch.start; i < batch.end; i++ {
							total += items[i].order
						}
					}
				}
			})
			b.Run("stream", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					drawRenderPhase2D(canvas, world, items, DrawViews2D{}, false, nil, func(batch renderBatch2D, _ renderViewport2D, _ bool) { total += items[batch.start].order })
				}
			})
			runtime.KeepAlive(total)
		})
	}
}
