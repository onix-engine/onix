package scene

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/go-gl/mathgl/mgl32"
)

type lcdBatchDriver struct {
	canvasTestDriver
	blend   render.BlendState
	draws   int
	uploads int
	colors  []render.Color
}

func (d *lcdBatchDriver) SetBlend(value render.BlendState) error {
	d.blend = value
	return nil
}

func (d *lcdBatchDriver) DrawSprites(batch render.SpriteBatch) error {
	d.draws += len(batch.Draws)
	d.sprites += len(batch.Instances)
	d.colors = append(d.colors, d.blend.Constant)
	return nil
}

func (d *lcdBatchDriver) UpdateTextures([]render.TextureUpdate) error {
	d.uploads++
	return nil
}

func lcdBatchFixture(t testing.TB, value string) (*graphics.Canvas2D, *Text2D, *lcdBatchDriver) {
	t.Helper()
	render.Uninstall()
	driver := &lcdBatchDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.Canvas2DContext(20000, 100)
	text := NewText2D(value)
	text.Font, text.LCD = notosanssc.NotoSansSCRegular, true
	text.Color = graphics.ColorFromRGBA(40, 100, 200, 255)
	return canvas, &text, driver
}

func TestLCDTextBatchesGlyphsAndUploads(t *testing.T) {
	canvas, text, driver := lcdBatchFixture(t, strings.Repeat("Text", 250))
	draw := func() {
		t.Helper()
		if !DrawScreenText2D(canvas, text, mgl32.Ident4(), AnchorTopLeft) {
			t.Fatal("missing text")
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	for _, opacity := range []float32{1, .5} {
		text.Color.A = opacity
		driver.draws, driver.sprites, driver.uploads = 0, 0, 0
		draw()
		if driver.draws != 1 || driver.sprites != 1000 {
			t.Fatalf("LCD text: %d draws, %d instances; want one draw for 1000 glyphs", driver.draws, driver.sprites)
		}
		if driver.uploads > 2 {
			t.Fatalf("glyph uploads were not batched per atlas: %d submissions", driver.uploads)
		}
		if canvas.BlendState() != render.PremultipliedAlphaBlend {
			t.Fatal("LCD text leaked its blend state")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		driver.colors = driver.colors[:0]
		draw()
	}); allocations != 0 {
		t.Fatalf("warm LCD drawing allocated %g times", allocations)
	}
}

func TestLCDTextColorRunsKeepPainterOrder(t *testing.T) {
	canvas, text, driver := lcdBatchFixture(t, "RedGreenBlue")
	text.LetterSpacing = -6 // Exercise overlapping glyphs without reordering.
	text.Spans = []TextSpan2D{
		{Start: 0, End: 3, Color: graphics.ColorFromRGBA(255, 0, 0, 255)},
		{Start: 3, End: 8, Color: graphics.ColorFromRGBA(0, 255, 0, 128)},
		{Start: 8, End: 12, Color: graphics.ColorFromRGBA(0, 0, 255, 255)},
	}
	DrawScreenText2D(canvas, text, mgl32.Ident4(), AnchorTopLeft)
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	want := []render.Color{{R: 1}, {G: 1}, {B: 1}}
	if driver.draws != len(want) || driver.sprites != 12 || len(driver.colors) != len(want) {
		t.Fatalf("color runs: %d draws, %d instances, colors=%v", driver.draws, driver.sprites, driver.colors)
	}
	for index, color := range want {
		if driver.colors[index] != color {
			t.Fatalf("color run %d = %v, want %v", index, driver.colors[index], color)
		}
	}
}

func BenchmarkLCDText1000Glyphs(b *testing.B) {
	canvas, text, driver := lcdBatchFixture(b, strings.Repeat("Text", 250))
	DrawScreenText2D(canvas, text, mgl32.Ident4(), AnchorTopLeft)
	canvas.FinishFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		driver.colors = driver.colors[:0]
		driver.draws = 0
		DrawScreenText2D(canvas, text, mgl32.Ident4(), AnchorTopLeft)
		if err := canvas.FinishFrame(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(driver.draws), "draws/op")
}
