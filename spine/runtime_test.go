package spine

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	"github.com/diiyw/onix/render"
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

func TestSpine37ProjectProducesAnimatedMesh(t *testing.T) {
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, "spineboy-pro.json"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, "spineboy.atlas"))
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := os.ReadFile(filepath.Join(base, "spineboy.png"))
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(imageData, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Width <= 0 || resource.Height <= 0 {
		t.Fatalf("invalid skeleton dimensions: %fx%f", resource.Width, resource.Height)
	}
	if len(resource.Animations()) < 2 {
		t.Fatalf("expected animations, got %v", resource.Animations())
	}
	player := NewPlayer(resource)
	if err := player.SetAnimation(resource.Animations()[0], true); err != nil {
		t.Fatal(err)
	}
	player.Update(0.25)
	projection := mgl32.Ortho(-500, 500, -380, 380, -1000, 1000)
	batch := player.MeshBatch(projection, mgl32.Ident4())
	if len(batch.Vertices) == 0 || len(batch.Indices) == 0 || len(batch.Draws) == 0 {
		t.Fatalf("empty mesh: vertices=%d indices=%d draws=%d", len(batch.Vertices), len(batch.Indices), len(batch.Draws))
	}
	for _, index := range batch.Indices {
		if int(index) >= len(batch.Vertices) {
			t.Fatalf("index %d exceeds %d vertices", index, len(batch.Vertices))
		}
	}
	var firstWorldY float32
	for _, slotIndex := range player.pose.drawOrder {
		slot := player.pose.slots[slotIndex]
		attachment := skinAttachment(player.Resource.skins[player.Skin], player.Resource.skins["default"], slotIndex, slot.attachment)
		if attachment == nil {
			continue
		}
		region, ok := player.Resource.atlas[attachment.path]
		if !ok {
			region, ok = player.Resource.atlas[attachment.name]
		}
		if !ok {
			continue
		}
		positions, _, _ := player.attachmentGeometry(slotIndex, attachment, region)
		firstWorldY = positions[1]
		break
	}
	if got := batch.Vertices[0].Position[1]; math.Abs(float64(got-firstWorldY)) > 1e-5 {
		t.Fatalf("mesh batch flipped Spine world Y: got %f want %f", got, firstWorldY)
	}
	model := mgl32.Mat4(batch.Instances[0].Model)
	position := batch.Vertices[0].Position
	clip := projection.Mul4(model).Mul4x1(mgl32.Vec4{position[0], position[1], position[2], 1})
	if depth := clip.Z() / clip.W(); depth <= 0 {
		t.Fatalf("Spine mesh depth %f is rejected by the renderer's reverse-depth comparison", depth)
	}
}

func TestAllBundledSpine37ExportsLoad(t *testing.T) {
	base := bundledSpineExportsDir()
	cases := []struct{ json, atlas, texture string }{
		{"coin-pro.json", "coin.atlas", "coin.png"},
		{"goblins-pro.json", "goblins.atlas", "goblins.png"},
		{"raptor-pro.json", "raptor.atlas", "raptor.png"},
		{"stretchyman-pro.json", "stretchyman.atlas", "stretchyman.png"},
		{"tank-pro.json", "tank.atlas", "tank.png"},
		{"vine-pro.json", "vine.atlas", "vine.png"},
	}
	for _, test := range cases {
		t.Run(test.json, func(t *testing.T) {
			skeleton, err := os.ReadFile(filepath.Join(base, test.json))
			if err != nil {
				t.Fatal(err)
			}
			atlas, err := os.ReadFile(filepath.Join(base, test.atlas))
			if err != nil {
				t.Fatal(err)
			}
			imageData, err := os.ReadFile(filepath.Join(base, test.texture))
			if err != nil {
				t.Fatal(err)
			}
			texture, err := graphicspng.Decode(imageData, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
			if err != nil {
				t.Fatal(err)
			}
			resource, err := Load(skeleton, atlas, texture)
			if err != nil {
				t.Fatal(err)
			}
			player := NewPlayer(resource)
			player.Update(.2)
			if batch := player.MeshBatch(mgl32.Ident4(), mgl32.Ident4()); len(batch.Draws) == 0 {
				t.Fatal("expected drawable attachments")
			}
		})
	}
}

func TestPlayerUpdateDoesNotAllocateAfterWarmup(t *testing.T) {
	for _, name := range []string{"spineboy", "tank", "goblins"} {
		t.Run(name, func(t *testing.T) {
			player := NewPlayer(loadBundledSpineTestResource(t, name, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU}))
			player.Update(1.0 / 60)

			if allocations := testing.AllocsPerRun(100, func() {
				player.Update(1.0 / 60)
			}); allocations != 0 {
				t.Fatalf("player update allocated %.0f objects, want 0", allocations)
			}
		})
	}
}

func TestPlayerMeshBatchIntoDoesNotAllocateAfterWarmup(t *testing.T) {
	for _, name := range []string{"spineboy", "tank", "goblins"} {
		t.Run(name, func(t *testing.T) {
			player := NewPlayer(loadBundledSpineTestResource(t, name, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU}))
			player.Update(1.0 / 60)
			var batch render.MeshBatch3D
			player.MeshBatchInto(&batch, mgl32.Ident4(), mgl32.Ident4())

			if allocations := testing.AllocsPerRun(100, func() {
				player.MeshBatchInto(&batch, mgl32.Ident4(), mgl32.Ident4())
			}); allocations != 0 {
				t.Fatalf("mesh batch allocated %.0f objects, want 0", allocations)
			}
		})
	}
}

func TestPlayerMeshBatchIntoReturnsCurrentBounds(t *testing.T) {
	player := NewPlayer(loadBundledSpineTestResource(t, "spineboy", graphics.TextureOptions{Usage: graphics.ResourceUsageCPU}))
	player.Update(1.0 / 60)
	var batch render.MeshBatch3D
	minX, minY, maxX, maxY, valid := player.MeshBatchInto(&batch, mgl32.Ident4(), mgl32.Ident4())
	wantMinX, wantMinY, wantMaxX, wantMaxY, wantValid := player.Bounds()
	if valid != wantValid || minX != wantMinX || minY != wantMinY || maxX != wantMaxX || maxY != wantMaxY {
		t.Fatalf("mesh bounds = (%v, %v)-(%v, %v), %v; want (%v, %v)-(%v, %v), %v", minX, minY, maxX, maxY, valid, wantMinX, wantMinY, wantMaxX, wantMaxY, wantValid)
	}
}

func TestPlayerMeshBatchIntoSkipsZeroAlpha(t *testing.T) {
	player := NewPlayer(loadBundledSpineTestResource(t, "spineboy", graphics.TextureOptions{Usage: graphics.ResourceUsageCPU}))
	player.Alpha = 0
	var batch render.MeshBatch3D
	_, _, _, _, valid := player.MeshBatchInto(&batch, mgl32.Ident4(), mgl32.Ident4())
	if valid || len(batch.Vertices) != 0 || len(batch.Indices) != 0 || len(batch.Instances) != 0 || len(batch.Draws) != 0 {
		t.Fatalf("zero-alpha mesh batch was generated: %+v", batch)
	}
}

func TestPlayerFrameDoesNotAllocateAfterAnimationWarmup(t *testing.T) {
	const delta = float32(1.0 / 60)
	for _, name := range []string{"spineboy", "tank", "goblins"} {
		t.Run(name, func(t *testing.T) {
			player := NewPlayer(loadBundledSpineTestResource(t, name, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU}))
			duration := player.Resource.animations[player.Animation].duration
			var batch render.MeshBatch3D
			for range max(1, int(math.Ceil(float64(duration/delta)))+1) {
				player.Update(delta)
				player.MeshBatchInto(&batch, mgl32.Ident4(), mgl32.Ident4())
			}

			if allocations := testing.AllocsPerRun(100, func() {
				player.Update(delta)
				player.MeshBatchInto(&batch, mgl32.Ident4(), mgl32.Ident4())
			}); allocations != 0 {
				t.Fatalf("player frame allocated %.0f objects, want 0", allocations)
			}
		})
	}
}

func loadBundledSpineTestResource(t *testing.T, name string, options graphics.TextureOptions) *Resource {
	t.Helper()
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, name+"-pro.json"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, name+".atlas"))
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := os.ReadFile(filepath.Join(base, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(imageData, options)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	return resource
}

func TestTankPathConstraintMatchesSpineTS37(t *testing.T) {
	resource := loadTankTestResource(t)
	if len(resource.paths) != 1 {
		t.Fatalf("tank path constraints = %d, want 1", len(resource.paths))
	}
	path := resource.paths[0]
	attachment := resource.skins["default"][path.target]["treads-path"]
	if attachment == nil || attachment.kind != "path" || !attachment.closed || attachment.vertexCount != 66 {
		t.Fatalf("tank tread path was not preserved: %+v", attachment)
	}

	reference := []struct {
		time  float32
		bones map[string][6]float32
	}{
		{time: 0, bones: map[string][6]float32{
			"tread":   {0.87998, 346.52000, -.91748, .38252, -.37985, -.92395},
			"tread2":  {-74.35318, 315.37254, -.96518, .23505, -.23340, -.97198},
			"tread14": {-405.62327, 11.45258, .99564, -.01189, .00508, .99728},
			"tread27": {617.02786, 136.39892, .41960, -.90436, .90059, .42550},
			"tread36": {71.09903, 313.21547, -.89561, -.42948, .43468, -.90030},
		}},
		{time: .5, bones: map[string][6]float32{
			"tread":   {-97.53260, 329.43392, -.93089, .34812, -.34569, -.93745},
			"tread2":  {-173.86527, 301.08754, -.99230, .03748, -.03721, -.99930},
			"tread14": {-422.21562, 11.72636, .99565, -.00965, .00285, .99731},
			"tread27": {590.50016, 158.35565, -.06725, -.99742, .99126, -.06391},
			"tread36": {-22.68171, 332.98173, -.99528, .02880, -.02204, -.99708},
		}},
	}
	player := NewPlayer(resource)
	if err := player.SetAnimation("drive", false); err != nil {
		t.Fatal(err)
	}
	for _, frame := range reference {
		player.time = frame.time
		player.evaluate()
		for name, expected := range frame.bones {
			index := slices.IndexFunc(resource.bones, func(bone boneData) bool { return bone.name == name })
			if index < 0 {
				t.Fatalf("tank bone %q is missing", name)
			}
			world := player.pose.bones[index].world
			actual := [6]float32{world.x, world.y, world.a, world.b, world.c, world.d}
			for i := range actual {
				if math.Abs(float64(actual[i]-expected[i])) > .05 {
					t.Errorf("time %v bone %q transform = %v, want %v", frame.time, name, actual, expected)
					break
				}
			}
		}
	}
}

func TestTankShootActivatesSmokeAttachments(t *testing.T) {
	resource := loadTankTestResource(t)
	player := NewPlayer(resource)
	if err := player.SetAnimation("shoot", false); err != nil {
		t.Fatal(err)
	}
	player.time = .5
	player.evaluate()

	activeSmoke := 0
	for i, slot := range resource.slots {
		if len(slot.name) >= len("smoke") && slot.name[:len("smoke")] == "smoke" && player.pose.slots[i].attachment != "" {
			activeSmoke++
		}
	}
	if activeSmoke != 48 {
		t.Fatalf("active smoke attachments at 0.5s = %d, want 48", activeSmoke)
	}
	drawableSmoke := 0
	visibleSmoke := 0
	reflectedSmoke := 0
	minX, minY := float32(math.MaxFloat32), float32(math.MaxFloat32)
	maxX, maxY := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	player.forEachDrawable(func(geometry drawableGeometry) {
		if slot := resource.slots[geometry.slotIndex].name; len(slot) >= len("smoke") && slot[:len("smoke")] == "smoke" {
			drawableSmoke++
			if geometry.slot.color[3]*geometry.attach.color[3] > .01 {
				visibleSmoke++
			}
			bone := player.pose.bones[resource.slots[geometry.slotIndex].bone].world
			if bone.a*bone.d-bone.b*bone.c < 0 {
				reflectedSmoke++
			}
			for i := 0; i+1 < len(geometry.positions); i += 2 {
				minX, minY = min(minX, geometry.positions[i]), min(minY, geometry.positions[i+1])
				maxX, maxY = max(maxX, geometry.positions[i]), max(maxY, geometry.positions[i+1])
			}
		}
	})
	if drawableSmoke != activeSmoke {
		t.Fatalf("drawable smoke attachments at 0.5s = %d, want %d", drawableSmoke, activeSmoke)
	}
	if visibleSmoke < 40 {
		t.Fatalf("visible smoke attachments at 0.5s = %d, want at least 40", visibleSmoke)
	}
	if reflectedSmoke != drawableSmoke {
		t.Fatalf("reflected smoke attachments at 0.5s = %d, want %d", reflectedSmoke, drawableSmoke)
	}
	if minX > -4000 || maxX < -1500 || minY > -100 || maxY < 1500 {
		t.Fatalf("smoke bounds at 0.5s = (%f,%f)-(%f,%f), expected a large cloud around the cannon", minX, minY, maxX, maxY)
	}
}

func loadTankTestResource(t *testing.T) *Resource {
	t.Helper()
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, "tank-pro.json"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, "tank.atlas"))
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := os.ReadFile(filepath.Join(base, "tank.png"))
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(imageData, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	return resource
}

func TestCoinUsesClippingTwoColorAndAdditiveBlending(t *testing.T) {
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, "coin-pro.json"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, "coin.atlas"))
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := os.ReadFile(filepath.Join(base, "coin.png"))
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(imageData, graphics.TextureOptions{PremultiplyAlpha: true, Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}

	shineSlot := slices.IndexFunc(resource.slots, func(slot slotData) bool { return slot.name == "coin-front-shine" })
	clipSlot := slices.IndexFunc(resource.slots, func(slot slotData) bool { return slot.name == "clipping" })
	if shineSlot < 0 || clipSlot < 0 {
		t.Fatal("coin slots are missing")
	}
	if resource.slots[shineSlot].blend != render.MeshBlendAdditive {
		t.Fatalf("coin front shine blend = %v, want additive", resource.slots[shineSlot].blend)
	}
	clip := resource.skins["default"][clipSlot]["clipping"]
	if clip == nil || clip.kind != "clipping" || clip.vertexCount != 39 {
		t.Fatalf("coin clipping attachment was not preserved: %+v", clip)
	}
	twoColor := slices.ContainsFunc(resource.animations["animation"].slots, func(timeline slotTimeline) bool {
		return timeline.slot == shineSlot && timeline.kind == "twoColor"
	})
	if !twoColor {
		t.Fatal("coin two-color timeline was not parsed")
	}

	player := NewPlayer(resource)
	if err := player.SetAnimation("animation", true); err != nil {
		t.Fatal(err)
	}
	player.Update(0)
	start := player.MeshBatch(mgl32.Ident4(), mgl32.Ident4())
	if len(start.Draws) != 2 {
		t.Fatalf("coin start draws = %d, want 2 blend runs with the off-coin shine fully clipped", len(start.Draws))
	}
	if start.Draws[1].BlendMode != render.MeshBlendAdditive {
		t.Fatalf("coin overlay blend = %v, want additive", start.Draws[1].BlendMode)
	}
	if len(start.Instances) != 1 || start.Instances[0].Custom[0][3] != 1 {
		t.Fatalf("coin texture alpha mode = %v, want premultiplied", start.Instances[0].Custom[0][3])
	}

	player.Update(.333)
	batch := player.MeshBatch(mgl32.Ident4(), mgl32.Ident4())
	if len(batch.Draws) != 2 {
		t.Fatalf("coin active shine draws = %d, want 2 blend runs", len(batch.Draws))
	}
	if batch.Draws[1].BlendMode != render.MeshBlendAdditive {
		t.Fatalf("coin additive draws were lost: %+v", batch.Draws)
	}
	last := batch.Draws[len(batch.Draws)-1]
	firstVertex := batch.Vertices[batch.Indices[last.FirstIndex]]
	if firstVertex.Normal[2] <= 0 {
		t.Fatalf("coin two-color dark tint was not encoded: %v", firstVertex.Normal)
	}
	for _, index := range batch.Indices[last.FirstIndex : last.FirstIndex+last.IndexCount] {
		position := batch.Vertices[index].Position
		if abs(position[0]) > 142 || abs(position[1]) > 142 {
			t.Fatalf("shine vertex escaped the coin clipping attachment: %v", position)
		}
	}
	player.time = .6696
	player.evaluate()
	drawnSlots := make([]string, 0)
	player.forEachDrawable(func(geometry drawableGeometry) {
		drawnSlots = append(drawnSlots, resource.slots[geometry.slotIndex].name)
	})
	for _, name := range []string{"coin-side", "coin-side-round", "coin-front-texture", "coin-front-shine"} {
		if !slices.Contains(drawnSlots, name) {
			t.Fatalf("edge-on coin attachment %q was culled: %v", name, drawnSlots)
		}
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

func TestSpineboyWalkMatchesTSReference(t *testing.T) {
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, "spineboy-pro.json"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, "spineboy.atlas"))
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := os.ReadFile(filepath.Join(base, "spineboy.png"))
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(imageData, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	player := NewPlayer(resource)
	if err := player.SetAnimation("walk", false); err != nil {
		t.Fatal(err)
	}

	reference := []struct {
		time        float32
		attachments map[string][4]float32
	}{
		{time: 0, attachments: map[string][4]float32{
			"rear-upper-arm": {10.31699, 255.93929, 79.70787, 351.56802},
			"gun":            {23.58798, 88.56221, 254.78290, 313.57565},
			"rear-foot":      {-158.99458, -10.51602, -53.66829, 73.92746},
			"front-shin":     {21.22516, .51175, 128.54314, 188.72986},
			"head":           {-98.95687, 362.74445, 155.69754, 660.12695},
			"torso":          {-44.00620, 206.12674, 62.04218, 397.80157},
			"eye":            {2.55065, 407.66681, 129.37854, 533.39056},
		}},
		{time: .125, attachments: map[string][4]float32{
			"rear-upper-arm": {7.48542, 251.85963, 67.30177, 345.94208},
			"gun":            {3.32286, 76.39110, 266.97369, 335.63116},
			"rear-foot":      {-139.47704, -2.56050, -69.14553, 114.57493},
			"front-shin":     {-2.67047, 4.42491, 87.68581, 188.29576},
			"head":           {-98.04285, 357.073, 158.25423, 657.64203},
			"torso":          {-47.05336, 203.26131, 54.57559, 395.08185},
			"eye":            {3.31024, 402.11063, 129.78526, 527.39636},
		}},
		{time: .25, attachments: map[string][4]float32{
			"rear-upper-arm": {-7.01747, 267.71597, 43.40388, 358.98395},
			"gun":            {-34.25068, 85.35704, 200.47452, 314.05695},
			"rear-foot":      {-44.46097, 6.00194, 33.07178, 125.77293},
			"front-shin":     {-70.06051, 8.04324, 50.81599, 186.32961},
			"head":           {-109.71603, 368.73373, 152.73163, 662.40424},
			"torso":          {-55.92930, 213.05844, 45.95093, 407.10034},
			"eye":            {-.75072, 415.21347, 126.41556, 541.36536},
		}},
		{time: .5, attachments: map[string][4]float32{
			"rear-upper-arm": {-43.34642, 277.58264, 48.43238, 365.02658},
			"gun":            {-187.21422, 48.98672, 104.79719, 340.62057},
			"rear-foot":      {47.73123, -2.07377, 160.79131, 58.77615},
			"front-shin":     {-162.19533, 51.19663, 21.783, 166.69168},
			"head":           {-123.64082, 362.87964, 159.28235, 658.06934},
			"torso":          {-49.51760, 206.50743, 45.43347, 397.03510},
			"eye":            {4.23321, 413.28012, 132.82442, 541.48053},
		}},
		{time: .75, attachments: map[string][4]float32{
			"rear-upper-arm": {-7.91348, 277.81705, 46.68702, 370.47229},
			"gun":            {-113.86288, 41.19071, 172.17352, 325.06018},
			"rear-foot":      {-93.26201, -9.35295, 14.39266, 73.09088},
			"front-shin":     {-24.54202, 67.05837, 101.57630, 243.38081},
			"head":           {-122.77228, 381.85870, 148.48618, 671.20618},
			"torso":          {-53.35313, 225.25476, 46.92555, 417.15088},
			"eye":            {-6.95399, 428.41003, 121.45353, 556.28491},
		}},
	}
	for _, frame := range reference {
		player.time = frame.time
		player.evaluate()
		for slot, expected := range frame.attachments {
			actual, ok := attachmentBounds(player, slot)
			if !ok {
				t.Fatalf("time %f slot %q has no attachment bounds", frame.time, slot)
			}
			for i := range actual {
				if math.Abs(float64(actual[i]-expected[i])) > .02 {
					t.Errorf("time %f slot %q bounds mismatch: got %v want %v", frame.time, slot, actual, expected)
					break
				}
			}
		}
	}
}

func TestSpineboyHoverboardMatchesTSReference(t *testing.T) {
	base := bundledSpineExportsDir()
	skeleton, err := os.ReadFile(filepath.Join(base, "spineboy-pro.json"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := os.ReadFile(filepath.Join(base, "spineboy.atlas"))
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := os.ReadFile(filepath.Join(base, "spineboy.png"))
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(imageData, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := Load(skeleton, atlas, texture)
	if err != nil {
		t.Fatal(err)
	}
	player := NewPlayer(resource)
	if err := player.SetAnimation("hoverboard", false); err != nil {
		t.Fatal(err)
	}

	reference := []struct {
		time  float32
		bones map[string][2]float32
	}{
		{time: 0, bones: map[string][2]float32{
			"front-foot-target": {-80.43768, 88.31715}, "front-leg-target": {-109.07066, 117.14763},
			"rear-foot-target": {75.95243, 90.17597}, "rear-leg-target": {41.74298, 127.24182},
			"front-foot": {-109.32777, 117.37031}, "rear-foot": {40.96600, 127.62951},
		}},
		{time: .25, bones: map[string][2]float32{
			"front-foot-target": {-57.70031, 118.71788}, "front-leg-target": {-91.59525, 141.12718},
			"rear-foot-target": {95.04790, 152.32301}, "rear-leg-target": {54.01910, 181.66243},
			"front-foot": {-91.83702, 141.36644}, "rear-foot": {53.24321, 182.05231},
		}},
		{time: .5, bones: map[string][2]float32{
			"front-foot-target": {-61.43416, 174.87525}, "front-leg-target": {-77.05563, 212.72465},
			"rear-foot-target": {94.96582, 175.48132}, "rear-leg-target": {62.56284, 213.40947},
			"front-foot": {-77.28960, 212.97154}, "rear-foot": {61.82692, 213.87036},
		}},
		{time: .75, bones: map[string][2]float32{
			"front-foot-target": {-71.25023, 129.76486}, "front-leg-target": {-94.01871, 162.75842},
			"rear-foot-target": {84.99679, 122.82304}, "rear-leg-target": {52.12407, 161.46297},
			"front-foot": {-94.31382, 162.92757}, "rear-foot": {51.31083, 161.76732},
		}},
	}
	for _, frame := range reference {
		player.time = frame.time
		player.evaluate()
		for name, expected := range frame.bones {
			index := slices.IndexFunc(player.Resource.bones, func(bone boneData) bool { return bone.name == name })
			if index < 0 {
				t.Fatalf("reference bone %q is missing", name)
			}
			world := player.pose.bones[index].world
			if math.Abs(float64(world.x-expected[0])) > .03 || math.Abs(float64(world.y-expected[1])) > .03 {
				t.Errorf("time %f bone %q position mismatch: got (%f,%f) want (%f,%f)", frame.time, name, world.x, world.y, expected[0], expected[1])
			}
		}
	}
}

func attachmentBounds(player *Player, slotName string) ([4]float32, bool) {
	if player == nil || player.Resource == nil || player.pose == nil {
		return [4]float32{}, false
	}
	slotIndex := slices.IndexFunc(player.Resource.slots, func(slot slotData) bool { return slot.name == slotName })
	if slotIndex < 0 {
		return [4]float32{}, false
	}
	slot := player.pose.slots[slotIndex]
	attachment := skinAttachment(player.Resource.skins[player.Skin], player.Resource.skins["default"], slotIndex, slot.attachment)
	if attachment == nil {
		return [4]float32{}, false
	}
	region, ok := player.Resource.atlas[attachment.path]
	if !ok {
		region, ok = player.Resource.atlas[attachment.name]
	}
	if !ok {
		return [4]float32{}, false
	}
	positions, _, _ := player.attachmentGeometry(slotIndex, attachment, region)
	if len(positions) < 2 {
		return [4]float32{}, false
	}
	minX, minY, maxX, maxY := positions[0], positions[1], positions[0], positions[1]
	for i := 2; i+1 < len(positions); i += 2 {
		minX, minY = min(minX, positions[i]), min(minY, positions[i+1])
		maxX, maxY = max(maxX, positions[i]), max(maxY, positions[i+1])
	}
	return [4]float32{minX, minY, maxX, maxY}, true
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
