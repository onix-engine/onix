package scene

import (
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

var roundTwoClipColor uint32

func BenchmarkRoundedClipChangingColor(b *testing.B) {
	driver := &canvasTestDriver{}
	if err := render.Install(driver); err != nil {
		b.Fatal(err)
	}
	defer render.Uninstall()
	canvas := graphics.Canvas2DContext(100, 80)
	rounded := roundedClip2D{minX: 1, minY: 1, maxX: 99, maxY: 79, radius: 8}
	sprite := Sprite2D{Shape: RoundedRectShape2D(graphics.White, 0), Width: 98, Height: 78, Alpha: .7}
	item := newSpriteRenderItem2D(0, 0, &sprite, graphics.Material2DData{}, AnchorTopLeft,
		mgl32.Translate3D(1, 1, 0),
		clipRect2D{minX: 1, minY: 1, maxX: 99, maxY: 79, valid: true, hasRounded: true, rounded: rounded},
		98, 78, true, 0)
	viewport := renderViewport2D{clipFromLayer: canvas.ProjectionMatrix(), active: true}
	drawShapeRenderItem2D(canvas, &item, viewport)
	canvas.FinishFrame()
	start := driver.nextID
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		roundTwoClipColor++
		value := roundTwoClipColor
		item.sprite.Shape.Fill = graphics.ColorFromRGBA(byte(value), byte(value>>8), byte(value>>16), 173)
		drawShapeRenderItem2D(canvas, &item, viewport)
		if err := canvas.FinishFrame(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(driver.nextID-start)/float64(b.N), "textures/op")
}
