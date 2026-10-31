package render3d

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
)

func TestRenderViewReclaimsBurstStorageAndPreservesSnapshots(t *testing.T) {
	view := renderView3D{
		extracted:    make([]extractedMesh3D, 2048, 4096),
		instances:    make([]render.Instance3D, 2048, 4096),
		draws:        make([]render.MeshDraw3D, 2048, 4096),
		vertices:     make([]render.Vertex3D, 2048, 4096),
		indices:      make([]uint32, 2048, 4096),
		deformations: make([]render.MeshDeformationData, 2048, 4096),
	}
	view.releaseUnusedExtracted()
	for frame := range reuse.RetentionFrames + 1 {
		view.resetExtracted()
		view.extracted = append(view.extracted, extractedMesh3D{entity: ecs.Entity(frame + 1)})
		view.instances = view.instances[:1]
		view.draws = view.draws[:1]
		view.vertices = view.vertices[:1]
		view.indices = view.indices[:1]
		view.deformations = view.deformations[:0]
		view.releaseUnusedExtracted()
		if len(view.extracted) != 1 || view.extracted[0].entity != ecs.Entity(frame+1) {
			t.Fatal("shrinking changed a live extracted mesh")
		}
	}
	if cap(view.extracted) != 64 || cap(view.instances) != 64 || cap(view.draws) != 64 || cap(view.vertices) != 256 || cap(view.indices) != 384 || cap(view.deformations) != 0 {
		t.Fatalf("view retained burst capacity: %d/%d/%d/%d/%d/%d", cap(view.extracted), cap(view.instances), cap(view.draws), cap(view.vertices), cap(view.indices), cap(view.deformations))
	}
	if allocs := testing.AllocsPerRun(100, func() {
		view.resetExtracted()
		view.extracted = append(view.extracted, extractedMesh3D{entity: 7})
		view.releaseUnusedExtracted()
	}); allocs != 0 {
		t.Fatalf("stable view allocated %g times", allocs)
	}
}
