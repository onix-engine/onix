package spine

import (
	"image"
	"math"
	"slices"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

func TestPackRGBPreservesEightBitChannels(t *testing.T) {
	if got := packRGB(0x12/255.0, 0x34/255.0, 0x56/255.0); got != float32(0x563412) {
		t.Fatalf("packed RGB = %#x, want 0x563412", uint32(got))
	}
	if got := packRGB(1, 1, 1); got != float32(0xffffff) {
		t.Fatalf("packed white = %#x, want 0xffffff", uint32(got))
	}
}

func TestLoadPreservesExplicitZeroScale(t *testing.T) {
	texture := graphics.CreateTextureByImageWithUsage(image.NewRGBA(image.Rect(0, 0, 1, 1)), false, graphics.ResourceUsageCPU)
	skeleton := []byte(`{
		"bones": [{"name":"root","scaleX":0,"scaleY":0}],
		"slots": [{"name":"slot","bone":"root","attachment":"region"}],
		"skins": {"default":{"slot":{"region":{"width":1,"height":1,"scaleX":0,"scaleY":0}}}}
	}`)
	atlas := []byte("page.png\nregion\nsize: 1, 1\n")

	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	if resource.bones[0].scaleX != 0 || resource.bones[0].scaleY != 0 {
		t.Fatalf("explicit bone scale was replaced: %f,%f", resource.bones[0].scaleX, resource.bones[0].scaleY)
	}
	region := resource.skins["default"][0]["region"]
	if region.scaleX != 0 || region.scaleY != 0 {
		t.Fatalf("explicit attachment scale was replaced: %f,%f", region.scaleX, region.scaleY)
	}
}

func TestAnimationBoundsCoverCompleteAnimationWithoutChangingPose(t *testing.T) {
	texture := graphics.CreateTextureByImageWithUsage(image.NewRGBA(image.Rect(0, 0, 10, 10)), false, graphics.ResourceUsageCPU)
	skeleton := []byte(`{
		"bones": [{"name":"root"}],
		"slots": [{"name":"slot","bone":"root","attachment":"region"}],
		"skins": {"default":{"slot":{"region":{"width":10,"height":10}}}},
		"animations": {"move":{"bones":{"root":{"translate":[{"time":0,"x":0},{"time":1,"x":100}]}}}}
	}`)
	atlas := []byte("page.png\nsize: 10, 10\nregion\n  rotate: false\n  xy: 0, 0\n  size: 10, 10\n  orig: 10, 10\n  offset: 0, 0\n")
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	player := NewPlayer(resource)
	if err := player.SetAnimation("move", true); err != nil {
		t.Fatal(err)
	}
	player.Update(.5)
	beforeTime := player.time
	before := player.MeshBatch(mgl32.Ident4(), mgl32.Ident4()).Vertices

	minX, minY, maxX, maxY, ok := player.AnimationBounds()
	if !ok || math.Abs(float64(minX+5)) > 1e-5 || math.Abs(float64(maxX-105)) > 1e-5 || math.Abs(float64(minY+5)) > 1e-5 || math.Abs(float64(maxY-5)) > 1e-5 {
		t.Fatalf("animation bounds = (%v,%v)-(%v,%v), want (-5,-5)-(105,5)", minX, minY, maxX, maxY)
	}
	if player.time != beforeTime || !slices.Equal(before, player.MeshBatch(mgl32.Ident4(), mgl32.Ident4()).Vertices) {
		t.Fatal("calculating animation bounds changed the player's current pose")
	}
	if len(player.bounds) != 1 {
		t.Fatalf("animation bounds cache size = %d, want 1", len(player.bounds))
	}
}

func TestRegionAttachmentGeometryMatchesSpineRuntime(t *testing.T) {
	texture := graphics.CreateTextureByImageWithUsage(image.NewRGBA(image.Rect(0, 0, 100, 200)), false, graphics.ResourceUsageCPU)
	resource := &Resource{
		Texture: texture,
		bones:   []boneData{{parent: -1, scaleX: 1, scaleY: 1}},
		slots:   []slotData{{bone: 0}},
	}
	resource.buildUpdateCache()
	player := NewPlayer(resource)
	attachment := &attachment{kind: "region", x: 5, y: -3, scaleX: 1.5, scaleY: .5, width: 40, height: 60}
	region := atlasRegion{x: 10, y: 20, width: 30, height: 40, originalWidth: 40, originalHeight: 60, offsetX: 2, offsetY: 3, rotate: true}

	positions, uvs, indices := player.attachmentGeometry(0, attachment, region)
	assertFloat32Slice(t, positions, []float32{-22, -16.5, -22, 3.5, 23, 3.5, 23, -16.5})
	assertFloat32Slice(t, uvs, []float32{.5, .25, .1, .25, .1, .1, .5, .1})
	if want := []uint32{0, 1, 2, 2, 3, 0}; !slices.Equal(indices, want) {
		t.Fatalf("unexpected region indices: got %v want %v", indices, want)
	}
}

func TestMeshAttachmentUVsMatchSpineRuntime(t *testing.T) {
	input := []float32{0, 0, 1, 0, 1, 1, 0, 1}
	tests := []struct {
		name   string
		region atlasRegion
		want   []float32
	}{
		{
			name:   "trimmed",
			region: atlasRegion{x: 10, y: 20, width: 30, height: 40, originalWidth: 40, originalHeight: 60, offsetX: 2, offsetY: 3},
			want:   []float32{.08, .015, .48, .015, .48, .315, .08, .315},
		},
		{
			name:   "rotated trimmed",
			region: atlasRegion{x: 10, y: 20, width: 30, height: 40, originalWidth: 40, originalHeight: 60, offsetX: 2, offsetY: 3, rotate: true},
			want:   []float32{-.07, .26, -.07, .06, .53, .06, .53, .26},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertFloat32Slice(t, meshAttachmentUVs(test.region, 100, 200, input), test.want)
		})
	}
}

func assertFloat32Slice(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected slice length: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Fatalf("unexpected value at %d: got %v want %v", i, got, want)
		}
	}
}
