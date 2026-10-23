package scene

import (
	"math"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestTilemapVisibleRangePreservesVisibleCells(t *testing.T) {
	tilemap := Tilemap2D{Width: 80, Height: 60, TileWidth: 16, TileHeight: 24}
	viewport := renderViewport2D{active: true, clipFromLayer: mgl32.Ortho2D(0, 320, 240, 0)}
	transforms := []mgl32.Mat4{
		mgl32.Ident4(),
		mgl32.Translate3D(-300, -200, 0),
		mgl32.Translate3D(320, 240, 0),
		mgl32.Translate3D(-10000, -10000, 0),
		mgl32.Translate3D(100, 200, 0).Mul4(mgl32.Scale3D(-1, -2, 1)),
		mgl32.Scale3D(0, 1, 1),
		mgl32.Scale3D(0.00001, 0.00001, 1),
	}
	for angle := float32(0); angle < 360; angle += 13 {
		transforms = append(transforms, mgl32.Translate3D(100, -100, 0).Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(angle))))
	}
	shear := mgl32.Ident4()
	shear[4] = 0.75
	transforms = append(transforms, shear)
	for index, base := range transforms {
		bounds := tilemapVisibleRange2D(viewport, base, &tilemap)
		if bounds.minX < 0 || bounds.minY < 0 || bounds.maxX > tilemap.Width || bounds.maxY > tilemap.Height {
			t.Fatalf("transform %d: invalid bounds %+v", index, bounds)
		}
		for row := range tilemap.Height {
			for column := range tilemap.Width {
				model := base.Mul4(mgl32.Translate3D(float32(column*tilemap.TileWidth), float32(row*tilemap.TileHeight), 0))
				if viewport.visible(model, float32(tilemap.TileWidth), float32(tilemap.TileHeight)) &&
					(column < bounds.minX || column >= bounds.maxX || row < bounds.minY || row >= bounds.maxY) {
					t.Fatalf("transform %d: visible cell (%d,%d) outside %+v", index, column, row, bounds)
				}
			}
		}
	}
}

func TestTilemapVisibleRangeSpecialProjections(t *testing.T) {
	tilemap := Tilemap2D{Width: 100, Height: 100, TileWidth: 16, TileHeight: 16}
	if got := tilemapVisibleRange2D(renderViewport2D{}, mgl32.Ident4(), &tilemap); got != (tileRange2D{}) {
		t.Fatalf("inactive viewport = %+v", got)
	}
	for _, projection := range []mgl32.Mat4{mgl32.Perspective(1, 1, 0.1, 100), {}, {0: float32(math.NaN()), 5: 1, 15: 1}} {
		got := tilemapVisibleRange2D(renderViewport2D{active: true, clipFromLayer: projection}, mgl32.Ident4(), &tilemap)
		if got != (tileRange2D{maxX: 100, maxY: 100}) {
			t.Fatalf("non-invertible/projective bounds = %+v", got)
		}
	}
}

func TestTilemapSnapshotsReuseAndIsolateStorage(t *testing.T) {
	var snapshots renderSnapshots2D
	tiles := make([]int, 4097)
	tiles[1024], tiles[4096] = 1, 2
	snapshot := snapshots.addTilemap(Tilemap2D{Tiles: tiles})
	storage := &snapshot.Tiles[0]
	tiles[1024] = 3
	if snapshot.Tiles[1024] != 1 {
		t.Fatal("snapshot aliases mutable source")
	}
	snapshots.reset()
	snapshot = snapshots.addTilemap(Tilemap2D{Tiles: tiles})
	if &snapshot.Tiles[0] != storage || !slices.Equal(snapshot.Tiles, tiles) {
		t.Fatal("snapshot did not reuse and update its retained storage")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		snapshots.reset()
		tiles[2048]++
		snapshots.addTilemap(Tilemap2D{Tiles: tiles})
		snapshots.releaseUnused()
	}); allocations != 0 {
		t.Fatalf("snapshot allocations = %v", allocations)
	}
	snapshots.reset()
	snapshots.addTilemap(Tilemap2D{Tiles: tiles[:10]})
	snapshots.reset()
	if got := snapshots.addTilemap(Tilemap2D{Tiles: tiles}); !slices.Equal(got.Tiles, tiles) {
		t.Fatal("snapshot lost values after shrinking and regrowing")
	}
	snapshots.reset()
	snapshots.releaseUnused()
	if snapshots.tilemaps[:1][0].value.Tiles != nil || snapshots.tilemaps[:1][0].source != nil {
		t.Fatal("unused snapshot retains tile storage")
	}
}

func BenchmarkTilemapSnapshotUnchanged(b *testing.B) {
	tilemap := Tilemap2D{Tiles: make([]int, 1024*1024), Revision: 1}
	var snapshots renderSnapshots2D
	snapshots.addTilemap(tilemap)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snapshots.reset()
		snapshots.addTilemap(tilemap)
	}
}

func TestTilemapSnapshotRevisionInvalidation(t *testing.T) {
	var snapshots renderSnapshots2D
	tilemap := Tilemap2D{Tiles: []int{1, 2, 3}, Revision: 1, Alpha: 1}
	snapshot := snapshots.addTilemap(tilemap)
	storage := &snapshot.Tiles[0]
	tilemap.Tiles[1] = 8
	tilemap.Revision++
	snapshots.reset()
	snapshot = snapshots.addTilemap(tilemap)
	if snapshot.Tiles[1] != 8 || &snapshot.Tiles[0] != storage {
		t.Fatal("revision change did not refresh retained storage")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		snapshots.reset()
		snapshot = snapshots.addTilemap(tilemap)
	}); allocations != 0 {
		t.Fatalf("cached snapshot allocations = %v", allocations)
	}
	tilemap.Alpha = 0.5
	snapshots.reset()
	snapshot = snapshots.addTilemap(tilemap)
	if snapshot.Alpha != 0.5 || &snapshot.Tiles[0] != storage {
		t.Fatal("cached tiles prevented metadata refresh")
	}
	tilemap.Tiles = []int{4, 5, 6}
	snapshots.reset()
	snapshot = snapshots.addTilemap(tilemap)
	if !slices.Equal(snapshot.Tiles, tilemap.Tiles) {
		t.Fatal("source replacement reused stale tiles")
	}
	tilemap.Tiles = tilemap.Tiles[:2]
	snapshots.reset()
	snapshot = snapshots.addTilemap(tilemap)
	if !slices.Equal(snapshot.Tiles, tilemap.Tiles) {
		t.Fatal("length change reused stale tiles")
	}
	tilemap.Revision = 0
	tilemap.Tiles[0] = 9
	snapshots.reset()
	snapshot = snapshots.addTilemap(tilemap)
	if snapshot.Tiles[0] != 9 {
		t.Fatal("unversioned source reused stale tiles")
	}
}

func BenchmarkTilemapSnapshotCopy(b *testing.B) {
	tiles := make([]int, 1024*1024)
	snapshot := make([]int, len(tiles))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		copy(snapshot, tiles)
	}
	if snapshot[len(snapshot)-1] != 0 {
		b.Fatal("invalid snapshot")
	}
}

func BenchmarkTilemapVisibleTraversal(b *testing.B) {
	for _, bounded := range []bool{false, true} {
		name := "FullMap"
		if bounded {
			name = "VisibleRange"
		}
		b.Run(name, func(b *testing.B) {
			tilemap := Tilemap2D{Width: 1024, Height: 1024, TileWidth: 16, TileHeight: 16}
			viewport := renderViewport2D{active: true, clipFromLayer: mgl32.Ortho2D(0, 320, 240, 0)}
			base := mgl32.Translate3D(-1000, -1000, 0)
			b.ReportAllocs()
			for range b.N {
				bounds := tileRange2D{maxX: tilemap.Width, maxY: tilemap.Height}
				if bounded {
					bounds = tilemapVisibleRange2D(viewport, base, &tilemap)
				}
				visible := 0
				for row := bounds.minY; row < bounds.maxY; row++ {
					for column := bounds.minX; column < bounds.maxX; column++ {
						model := base.Mul4(mgl32.Translate3D(float32(column*tilemap.TileWidth), float32(row*tilemap.TileHeight), 0))
						if viewport.visible(model, float32(tilemap.TileWidth), float32(tilemap.TileHeight)) {
							visible++
						}
					}
				}
				if visible == 0 {
					b.Fatal("no visible cells")
				}
			}
		})
	}
}
