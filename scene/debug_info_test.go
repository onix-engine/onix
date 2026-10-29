//go:build !onix_release

package scene

import (
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

func TestSpawnDebugInfoUsesVisibleDefaults(t *testing.T) {
	world := ecs.NewWorld()
	entity := SpawnDebugInfo(world)
	component, ok := world.Get[DebugInfo](entity)
	if !ok {
		t.Fatal("debug info component was not spawned")
	}
	if component != NewDebugInfo() {
		t.Fatalf("debug info = %+v, want defaults %+v", component, NewDebugInfo())
	}
	if resolved := (DebugInfo{}).resolved(); resolved != NewDebugInfo() {
		t.Fatalf("zero debug info resolved to %+v", resolved)
	}
	if alpha := NewDebugInfo().BackgroundColor.A; alpha != float32(192)/255 {
		t.Fatalf("debug info background alpha = %v, want %v", alpha, float32(192)/255)
	}
}

func TestDebugInfoTextContainsRequestedStatistics(t *testing.T) {
	var text debugInfoText
	text.format(
		render.FrameStatistics{
			SurfaceWidth: 1920, SurfaceHeight: 1080,
			DrawCalls: 7, Sprites: 42, Shapes: 3, Meshes: 2,
			Vertices: 168, Triangles: 84,
			SpriteDrawCalls: 4, ShapeDrawCalls: 1, MeshDrawCalls: 2,
			BufferUploadBytes:    4096,
			DynamicMeshDrawCalls: 1, DynamicMeshUploadBytes: 2048,
			TextureUploads: 2, TextureUploadBytes: 1024,
		},
		12<<20,
		render.BufferMemoryStatistics{InstanceBytes: 2 << 20, StagingBytes: 8 << 20, ReadbackBytes: 1 << 20, ScratchBytes: 64 << 10},
		59.75,
		render.RendererInfo{Backend: "Vulkan", Device: "Example GPU"},
	)
	expected := [...]string{
		"SURFACE     1920X1080",
		"DRAW CALLS  7",
		"DRAWS SPR/SHP/M 4/1/2",
		"INST SPR/SHP/M  42/3/2",
		"VERT/TRI    168/84",
		"BUFFER      4.0 KIB",
		"DYN MESH    1 / 2.0 KIB",
		"TEX UPLOAD  2 / 1.0 KIB",
		"GO HEAP     12.0 MIB",
		"NATIVE BUF  11.0 MIB",
		"GO SCRATCH  64.0 KIB",
		"FPS         59.8",
		"BACKEND     VULKAN",
		"GPU         EXAMPLE GPU",
	}
	for index, want := range expected {
		if got := string(text.line(index)); got != want {
			t.Fatalf("debug info line %d = %q, want %q", index, got, want)
		}
	}
}

func TestDebugInfoSamplerMeasuresRenderRate(t *testing.T) {
	var state debugInfoSystemState
	start := time.Unix(100, 0)
	state.sample(start, nil)
	step := time.Second / 60
	for frame := 1; frame <= 31; frame++ {
		state.sample(start.Add(time.Duration(frame)*step), nil)
	}
	if state.fps < 59 || state.fps > 61 {
		t.Fatalf("sampled FPS = %.2f, want approximately 60", state.fps)
	}
	if state.heapBytes == 0 {
		t.Fatal("expected a runtime heap sample")
	}
}

func TestDebugInfoFrameDoesNotAllocateAfterWarmup(t *testing.T) {
	ctx := graphics.Canvas2DContext(437, 239)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	world := ecs.NewWorld()

	graphics.BindFrame(world, graphics.CurrentContext())
	SpawnDebugInfo(world)
	context := ecs.SystemContext{World: world}
	if err := renderDebugInfo(&context); err != nil {
		t.Fatal(err)
	}
	ctx.DrawFrame()

	if allocations := testing.AllocsPerRun(100, func() {
		if err := renderDebugInfo(&context); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("debug info frame allocated %.0f objects, want 0", allocations)
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	state := context.Local[debugInfoSystemStateTag, debugInfoSystemState]()
	if state.atlas != nil {
		if err := state.atlas.Dispose(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTruncateDebugInfoLinePreservesPanelBounds(t *testing.T) {
	truncated := func(value string, limit int) string {
		bytes := []byte(value)
		end, dots, _ := debugInfoLineBounds(bytes, limit)
		return string(bytes[:end]) + strings.Repeat(".", dots)
	}
	if got := truncated("GPU         A VERY LONG DEVICE NAME", 16); got != "GPU         A..." {
		t.Fatalf("truncated line = %q", got)
	}
	if got := truncated("FPS", 2); got != ".." {
		t.Fatalf("tiny truncated line = %q", got)
	}
	if got := truncated("GPU         设备名称长", 16); got != "GPU         设..." {
		t.Fatalf("UTF-8 truncated line = %q", got)
	}
}

func TestDebugGlyphAtlasContainsPrintableGlyphs(t *testing.T) {
	atlas := newDebugGlyphAtlas()
	if atlas == nil || !atlas.Valid() {
		t.Fatal("debug glyph atlas is invalid")
	}
	if atlas.Width != debugGlyphColumns*debugGlyphCellWidth {
		t.Fatalf("atlas width = %d", atlas.Width)
	}
	if err := atlas.Dispose(); err != nil {
		t.Fatalf("dispose atlas: %v", err)
	}
}
