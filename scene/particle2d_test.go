package scene

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestUpdateParticleEmitter2DSystemSpawnsLoopingParticles(t *testing.T) {
	world := ecs.NewWorld()
	emitter := NewParticleEmitter2D()
	emitter.Loop = true
	emitter.Count = 3
	emitter.Rate = 12
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, emitter)
	world.Spawn(bundle)
	ecs.InsertResource(world, &Time{Delta: time.Second})

	if err := UpdateParticleEmitter2DSystem("").Run(world); err != nil {
		t.Fatalf("update particle emitter: %v", err)
	}

	count := 0
	for _, access := range ecs.Query[ParticleEmitter2D](world).All {
		emitter := access.Mut()

		count = len(emitter.particles)
		break
	}
	if count != 3 {
		t.Fatalf("expected 3 live particles, got %d", count)
	}
}

func TestParticleEmitter2DNormalizeProvidesDefaults(t *testing.T) {
	emitter := ParticleEmitter2D{}
	emitter.Normalize()

	if emitter.Count != 12 {
		t.Fatalf("unexpected defaults: %+v", emitter)
	}
	if len(emitter.Colors) != 1 {
		t.Fatalf("expected default color palette, got %+v", emitter.Colors)
	}
}

func TestParticleEmitter2DActiveTracksBurstLifecycle(t *testing.T) {
	emitter := NewParticleEmitter2D()
	if !emitter.Active() {
		t.Fatal("expected new burst emitter to be active before its first update")
	}

	emitter.update(emitter.Lifetime * 2)
	if emitter.Active() {
		t.Fatal("expected finished burst emitter to be inactive")
	}
	emitter.Restart()
	if !emitter.Active() || len(emitter.particles) != 0 {
		t.Fatal("expected restarted burst emitter to be armed with no live particles")
	}

	emitter.Loop = true
	if !emitter.Active() {
		t.Fatal("expected looping emitter to stay active")
	}
}

func TestParticleEmitter2DStopAndReserveSupportAllocationFreeBursts(t *testing.T) {
	emitter := NewParticleEmitter2D()
	emitter.Count = 3
	emitter.Reserve(emitter.Count)
	if cap(emitter.particles) != emitter.Count {
		t.Fatalf("reserved capacity = %d, want %d", cap(emitter.particles), emitter.Count)
	}

	emitter.Restart()
	emitter.update(0)
	emitter.Stop()
	if emitter.Active() || len(emitter.particles) != 0 || cap(emitter.particles) != emitter.Count {
		t.Fatalf("stopped emitter = active %v len %d cap %d", emitter.Active(), len(emitter.particles), cap(emitter.particles))
	}

	if allocations := testing.AllocsPerRun(1_000, func() {
		emitter.Restart()
		emitter.update(0)
		emitter.Stop()
	}); allocations != 0 {
		t.Fatalf("reserved burst allocated %f objects", allocations)
	}
}

func TestParticleEmitter2DMultipleAppearancesStayPairedAndAllocateNothing(t *testing.T) {
	emitter := NewParticleEmitter2D()
	emitter.Count = 512
	emitter.Appearances = []ParticleAppearance2D{
		{Texture: graphics.CreateTextureByColor(graphics.White), Color: graphics.ColorFromRGBA(255, 0, 0, 255)},
		{Texture: graphics.CreateTextureByColor(graphics.Black), Color: graphics.ColorFromRGBA(0, 0, 255, 128)},
	}
	emitter.Reserve(emitter.Count)
	emitter.update(0)
	seen := [2]bool{}
	for _, particle := range emitter.particles {
		matched := false
		for i, appearance := range emitter.Appearances {
			if particle.texture == appearance.Texture && particle.clr == appearance.Color {
				seen[i], matched = true, true
				break
			}
		}
		if !matched {
			t.Fatal("particle texture and colour came from different elements")
		}
	}
	if !seen[0] || !seen[1] {
		t.Fatalf("burst did not use both appearances: %v", seen)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		emitter.Restart()
		emitter.update(0)
		emitter.Stop()
	}); allocations != 0 {
		t.Fatalf("multi-appearance burst allocated %f objects", allocations)
	}
}

func TestParticleSnapshotsIsolateAppearancesAndReuseStorage(t *testing.T) {
	emitter := NewParticleEmitter2D()
	emitter.Appearances = []ParticleAppearance2D{{Color: graphics.White}, {Color: graphics.Black}}
	emitter.Reserve(emitter.Count)
	emitter.update(0)
	before := emitter.particles[0]
	var snapshots renderSnapshots2D
	snapshot := snapshots.addEmitter(emitter)
	emitter.Appearances[0].Color = graphics.Black
	emitter.particles[0].clr = graphics.Black
	if snapshot.Appearances[0].Color != graphics.White || snapshot.particles[0] != before {
		t.Fatal("render snapshot aliases simulation state")
	}
	if &snapshot.particles[0] == &emitter.particles[0] {
		t.Fatal("render snapshot shares live particle storage")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		snapshots.reset()
		snapshots.addEmitter(emitter)
		snapshots.releaseUnused()
	}); allocations != 0 {
		t.Fatalf("multi-appearance snapshots allocated %f objects", allocations)
	}
}

type particleDrawRecord struct {
	texture render.TextureHandle
	uv      [6]float32
	color   [4]float32
}

type particleDrawDriver struct {
	canvasTestDriver
	particles []particleDrawRecord
}

func (d *particleDrawDriver) DrawSprites(batch render.SpriteBatch) error {
	for _, draw := range batch.Draws {
		for _, instance := range batch.Instances[draw.FirstInstance : draw.FirstInstance+draw.InstanceCount] {
			d.particles = append(d.particles, particleDrawRecord{texture: draw.TextureAt(int(instance.TextureSlot)), uv: instance.TextureModel, color: instance.Color})
		}
	}
	return nil
}

func TestParticleRendererBindsEachTextureAndRestoresFollowingSprite(t *testing.T) {
	driver := &particleDrawDriver{canvasTestDriver: canvasTestDriver{viewport: render.Rect{Width: 100, Height: 100}}}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.Canvas2DContext(100, 100)
	restore := graphics.SetCurrentContextForTesting(canvas)
	t.Cleanup(restore)
	white := graphics.CreateTextureByColor(graphics.White)
	black := graphics.CreateTextureByColor(graphics.Black)
	atlasRegion := graphics.NewSubTexture(black, 1, 1, .25, .5, .5, .25, 0)
	emitter := NewParticleEmitter2D()
	emitter.particles = []particleState2D{
		{x: 10, y: 10, size: 4, lifetime: 1, clr: graphics.White, texture: white},
		{x: 20, y: 10, size: 4, lifetime: 1, clr: graphics.White, texture: atlasRegion},
		{x: 30, y: 10, size: 4, lifetime: 1, clr: graphics.ColorFromRGBA(255, 0, 0, 255)},
		{x: 40, y: 10, size: 4, lifetime: 1, clr: graphics.White, texture: black},
	}
	world := ecs.NewWorld()
	RegisterParticleRenderer2D(world)
	RegisterSpriteRenderer2D(world)
	item := newParticleRenderItem2D(0, 1, &emitter, mgl32.Ident4(), clipRect2D{}, false, 0, white)
	sprite := Sprite2DFromTexture(white)
	following := renderItem2D{kind: renderItemSprite2D, material: item.material, texture: white, sprite: &sprite, transform: mgl32.Translate3D(50, 10, 0), width: 1, height: 1}
	drawRenderItems2D(canvas, world, []renderItem2D{item, following}, DrawViews2D{World: mgl32.Ident4(), UI: mgl32.Ident4()}, false)
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	want := []*graphics.Texture{white, black, white, black, white}
	if len(driver.particles) != len(want) {
		t.Fatalf("drawn instances = %d, want %d", len(driver.particles), len(want))
	}
	for i, texture := range want {
		if driver.particles[i].texture != texture.RenderHandleFor(canvas.Device()) {
			t.Fatalf("instance %d used another particle's texture", i)
		}
	}
	if driver.particles[1].uv != [6]float32{.5, 0, .25, 0, .25, .5} {
		t.Fatalf("particle atlas coordinates = %v", driver.particles[1].uv)
	}
	if driver.particles[2].color[0] != 1 || driver.particles[2].color[1] != 0 || driver.particles[2].color[2] != 0 {
		t.Fatal("solid particle lost its element colour")
	}
}

func TestParticleEmitter2DUpdateCompactsOnlyExpiredParticles(t *testing.T) {
	emitter := NewParticleEmitter2D()
	emitter.GravityX = 2
	emitter.burstDone = true
	emitter.particles = []particleState2D{
		{x: 1, vx: 1, lifetime: 10},
		{x: 2, vx: 2, life: 9, lifetime: 10},
		{x: 3, vx: 3, lifetime: 10},
	}
	emitter.update(1)
	if len(emitter.particles) != 2 {
		t.Fatalf("unexpected live particles: %+v", emitter.particles)
	}
	if emitter.particles[0].x != 4 || emitter.particles[1].x != 8 {
		t.Fatalf("unexpected compacted particle updates: %+v", emitter.particles)
	}
}

func TestParticleEmitter2DTextureFallsBackToWhite(t *testing.T) {
	emitter := NewParticleEmitter2D()
	texture := emitter.texture()
	if texture == nil {
		t.Fatalf("expected fallback particle texture")
	}
	if texture.Width != 1 || texture.Height != 1 {
		t.Fatalf("expected 1x1 fallback texture, got %dx%d", texture.Width, texture.Height)
	}

	custom := graphics.CreateTextureByColor(graphics.Black)
	emitter.Texture = custom
	if emitter.texture() != custom {
		t.Fatalf("expected custom particle texture to be preserved")
	}
}

func TestParticleEmitterBoundsUsesParticleSizeOnce(t *testing.T) {
	emitter := NewParticleEmitter2D()
	emitter.particles = []particleState2D{
		{x: 20, y: 30, size: 10, lifetime: 1, clr: graphics.White},
	}

	x0, y0, x1, y1, ok := ParticleEmitterBounds2D(&emitter, mgl32.Ident4(), false)
	if !ok || x0 != 15 || y0 != 25 || x1 != 25 || y1 != 35 {
		t.Fatalf("unexpected particle bounds: (%v, %v)-(%v, %v) ok=%v", x0, y0, x1, y1, ok)
	}
	x0, y0, x1, y1, ok = ParticleEmitterBounds2D(&emitter, mgl32.Ident4(), true)
	if !ok || x0 != 15 || y0 != -35 || x1 != 25 || y1 != -25 {
		t.Fatalf("unexpected UI particle bounds: (%v, %v)-(%v, %v) ok=%v", x0, y0, x1, y1, ok)
	}
}

func TestParticleModelMatrixKeepsYAxisUpInWorldAndUI(t *testing.T) {
	particle := particleState2D{x: 20, y: 30, size: 10}
	for name, test := range map[string]struct {
		ui    bool
		wantY float32
	}{
		"world": {wantY: 30},
		"ui":    {ui: true, wantY: -30},
	} {
		t.Run(name, func(t *testing.T) {
			center := particleModelMatrix(mgl32.Ident4(), particle, test.ui).Mul4x1(mgl32.Vec4{0.5, 0.5, 0, 1})
			if center.X() != 20 || center.Y() != test.wantY {
				t.Fatalf("particle center = (%v, %v), want (20, %v)", center.X(), center.Y(), test.wantY)
			}
		})
	}
}
