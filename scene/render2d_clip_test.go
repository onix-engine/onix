package scene

import (
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type roundedClipDrawDriver struct {
	canvasTestDriver
	instance render.Instance2D
}

func (d *roundedClipDrawDriver) DrawSprites(batch render.SpriteBatch) error {
	d.sprites += len(batch.Instances)
	if len(batch.Instances) > 0 {
		d.instance = batch.Instances[len(batch.Instances)-1]
	}
	return nil
}

func TestSolidShapeRoundedClipIgnoresInvisibleBorder(t *testing.T) {
	for _, test := range []struct {
		name  string
		color graphics.ColorRGBA
		width float32
	}{
		{"no border", graphics.ColorRGBA{}, 0},
		{"zero width opaque border", graphics.White, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			render.Uninstall()
			driver := &roundedClipDrawDriver{}
			if err := render.Install(driver); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(render.Uninstall)
			canvas := graphics.Canvas2DContext(100, 80)
			rounded := roundedClip2D{minX: 1, minY: 1, maxX: 99, maxY: 79, radius: 8}
			sprite := Sprite2D{
				Shape: OutlinedRoundedRectShape2D(graphics.Black, test.color, 0, test.width),
				Width: 98, Height: 78, Alpha: 1,
			}
			item := newSpriteRenderItem2D(0, 0, &sprite, graphics.Material2DData{}, AnchorTopLeft,
				mgl32.Translate3D(1, 1, 0),
				clipRect2D{minX: 1, minY: 1, maxX: 99, maxY: 79, valid: true, hasRounded: true, rounded: rounded},
				98, 78, true, 0)
			drawShapeRenderItem2D(canvas, &item, renderViewport2D{clipFromLayer: canvas.ProjectionMatrix(), active: true})
			if err := canvas.FinishFrame(); err != nil {
				t.Fatal(err)
			}
			if driver.sprites != 1 || driver.shapes != 0 {
				t.Fatalf("rounded clip bypassed: sprite submissions=%d, unmasked shape submissions=%d", driver.sprites, driver.shapes)
			}
			want := roundedClipMaterial2D([4][4]float32{}, rounded)
			if driver.instance.Custom != want {
				t.Fatalf("draw submission lost rounded mask: got %v, want %v", driver.instance.Custom, want)
			}
		})
	}
}
