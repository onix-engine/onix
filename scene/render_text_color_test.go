package scene

import (
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/go-gl/mathgl/mgl32"
)

type colorTextDriver struct {
	canvasTestDriver
	instances int
	blend     render.BlendState
	invalid   bool
}

func (d *colorTextDriver) SetBlend(blend render.BlendState) error { d.blend = blend; return nil }
func (d *colorTextDriver) DrawSprites(batch render.SpriteBatch) error {
	for _, instance := range batch.Instances {
		d.instances++
		if instance.Color[0] != 1 || instance.Color[1] != 1 || instance.Color[2] != 1 || d.blend != render.PremultipliedAlphaBlend {
			d.invalid = true
		}
	}
	return nil
}

func TestColorEmojiRetainsColorAndAlphaAcrossTextRendering(t *testing.T) {
	font := notosanssc.NotoSansSCRegular
	glyph := font.GlyphCache(18, 72).Glyph('🙂')
	if !glyph.RGBA() {
		t.Skip("no color emoji fallback installed")
	}
	render.Uninstall()
	driver := &colorTextDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.Canvas2DContext(200, 100)
	text := NewText2D("🙂\ufe0f")
	text.Font, text.LCD, text.FontSize = font, true, 18
	text.Color = graphics.ColorFromRGBA(40, 100, 200, 255)
	draw := func() {
		t.Helper()
		if !DrawScreenText2D(canvas, &text, mgl32.Ident4(), AnchorTopLeft) {
			t.Fatal("emoji was not drawn")
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	for _, alpha := range []float32{1, .5} {
		text.Color.A = alpha
		driver.instances = 0
		draw()
		if driver.instances != 1 || driver.invalid {
			t.Fatalf("emoji render: instances=%d invalid tint/blend=%v", driver.instances, driver.invalid)
		}
	}
	if allocations := testing.AllocsPerRun(50, draw); allocations != 0 {
		t.Fatalf("warm emoji rendering allocated %g", allocations)
	}
}
