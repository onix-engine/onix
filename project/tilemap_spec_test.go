package project

import (
	"slices"
	"testing"

	"github.com/diiyw/onix/internal/typedbin"
)

func TestTilemapCompactRoundTrip(t *testing.T) {
	want := make([]int, 64)
	want[7], want[8] = 2, 2
	want[32] = 4
	spec := TilemapSpec{ID: "map", Width: 8, Height: 8, Tiles: slices.Clone(want)}
	spec.Normalize()
	spec.Compact()
	if len(spec.Tiles) != 0 || len(spec.TileRuns) >= len(want)/2 {
		t.Fatalf("compact tilemap = tiles=%d runs=%d", len(spec.Tiles), len(spec.TileRuns))
	}
	if err := spec.Expand(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(spec.Tiles, want) || len(spec.TileRuns) != 0 {
		t.Fatalf("expanded tilemap = tiles=%v runs=%v", spec.Tiles, spec.TileRuns)
	}
}

func TestTilemapCompactKeepsDenseChanges(t *testing.T) {
	tiles := make([]int, 32)
	for index := range tiles {
		tiles[index] = index % 2
	}
	spec := TilemapSpec{Width: 8, Height: 4, Tiles: tiles}
	spec.Normalize()
	spec.Compact()
	if len(spec.TileRuns) != 0 || len(spec.Tiles) != len(tiles) {
		t.Fatalf("dense tilemap was unexpectedly compacted: tiles=%d runs=%d", len(spec.Tiles), len(spec.TileRuns))
	}
}

func TestTilemapCompactDenseDoesNotAllocate(t *testing.T) {
	tiles := make([]int, 1024)
	for index := range tiles {
		tiles[index] = index % 2
	}
	spec := TilemapSpec{Width: 32, Height: 32}
	allocations := testing.AllocsPerRun(100, func() {
		spec.Tiles = tiles
		spec.TileRuns = nil
		spec.Compact()
	})
	if allocations != 0 {
		t.Fatalf("dense tilemap compaction allocated %.0f times", allocations)
	}
}

func TestTilemapExpandRejectsInvalidRuns(t *testing.T) {
	spec := TilemapSpec{ID: "map", Width: 2, Height: 2, TileRuns: []TileRun{{Value: 1, Length: 5}}}
	if err := spec.Expand(); err == nil {
		t.Fatal("invalid tile run was accepted")
	}
}

func TestTilemapExpandRejectsInvalidRepresentations(t *testing.T) {
	tests := []struct {
		name string
		spec TilemapSpec
	}{
		{name: "missing tiles", spec: TilemapSpec{ID: "map", Width: 2, Height: 2, Tiles: []int{1}}},
		{name: "mixed tiles and runs", spec: TilemapSpec{ID: "map", Width: 1, Height: 1, Tiles: []int{1}, TileRuns: []TileRun{{Value: 1, Length: 1}}}},
		{name: "incomplete runs", spec: TilemapSpec{ID: "map", Width: 2, Height: 2, TileRuns: []TileRun{{Value: 1, Length: 3}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.spec.Expand(); err == nil {
				t.Fatal("invalid tilemap representation was accepted")
			}
		})
	}
}

func TestTilemapNormalizeDoesNotOverflowDimensions(t *testing.T) {
	spec := TilemapSpec{Width: int(^uint(0) >> 1), Height: 2, Tiles: []int{1}}
	spec.Normalize()
	if !slices.Equal(spec.Tiles, []int{1}) {
		t.Fatalf("overflowing tilemap was modified: %v", spec.Tiles)
	}
	if err := (&TilemapSpec{ID: "overflow", Width: int(^uint(0) >> 1), Height: 2, TileRuns: []TileRun{{Value: 1, Length: 1}}}).Expand(); err == nil {
		t.Fatal("overflowing tilemap dimensions were accepted")
	}
}

func TestTilemapRunsSurviveTypedBinaryPreprocessing(t *testing.T) {
	want := slices.Repeat([]int{3}, 64)
	spec := TilemapSpec{ID: "map", Width: 8, Height: 8, Tiles: want}
	spec.Normalize()
	spec.Compact()
	encoded, err := typedbin.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typedbin.Unmarshal[TilemapSpec](encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.Expand(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(decoded.Tiles, want) {
		t.Fatalf("typed binary tilemap = %v, want %v", decoded.Tiles, want)
	}
}
