package graphics

import (
	"math"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestShapeCoverageQuadPreservesContourAndClipping(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 800, Height: 600}}
	installCanvasTestDriver(t, driver)
	c := NewCanvas2D(render.ActiveDevice(), 400, 300)
	for _, projection := range []bool{false, true} {
		c.projected, c.projection = projection, mgl32.Ortho2D(-100, 700, 450, -150)
		for _, transform := range []mgl32.Mat4{
			mgl32.Translate3D(30.25, 40.75, 0),
			mgl32.HomogRotate3DZ(.08726646),
			mgl32.HomogRotate3DZ(.7).Mul4(mgl32.Scale3D(-.5, 2, 1)),
			{1, .2, 0, 0, .8, 1, 0, 0, 10, 20, 1, 0, 30, 40, 0, 1},
		} {
			model := affineFromMat4(transform.Mul4(mgl32.Scale3D(170, 44, 1)))
			for _, local := range [][6]float32{{1, 0, 0, 0, 1, 0}, {.75, 0, .25, 0, .5, 0}, {.5, 0, .25, 0, .5, .25}} {
				padded, coords := c.shapeCoverageQuad(model, local)
				// Solve where the old corners lie in the expanded geometry. Their
				// original screen positions must still map to the same shape points.
				for _, point := range [][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
					x := (local[0]*point[0] + local[2] - coords[2]) / coords[0]
					y := (local[4]*point[1] + local[5] - coords[5]) / coords[4]
					for row := range 2 {
						i := row * 3
						got := padded[i]*x + padded[i+1]*y + padded[i+2]
						want := model[i]*point[0] + model[i+1]*point[1] + model[i+2]
						if math.Abs(float64(got-want)) > .0001 {
							t.Fatalf("contour moved: %v, want %v", got, want)
						}
					}
				}
				for _, edge := range [][2]float32{{local[2], coords[2]}, {local[5], coords[5]}, {local[2] + local[0], coords[2] + coords[0]}, {local[5] + local[4], coords[5] + coords[4]}} {
					if edge[0] > 0 && edge[0] < 1 && math.Abs(float64(edge[0]-edge[1])) > .000001 {
						t.Fatalf("clip edge expanded: %v", edge)
					}
				}
				if local == ([6]float32{1, 0, 0, 0, 1, 0}) && (coords[2] >= 0 || coords[5] >= 0 || coords[2]+coords[0] <= 1 || coords[5]+coords[4] <= 1) {
					t.Fatal("shape lacks exterior coverage", coords)
				}
			}
		}
	}
}

func TestShapeCoverageSubmissionAllocationsAndMasks(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 400, Height: 300}}
	installCanvasTestDriver(t, driver)
	c := NewCanvas2D(render.ActiveDevice(), 400, 300)
	model := mgl32.HomogRotate3DZ(.08726646).Mul4(mgl32.Scale3D(170, 44, 1))
	if got := testing.AllocsPerRun(100, func() {
		c.shapes = c.shapes[:0]
		c.DrawShapeMatrices(model, mgl32.Ident3(), 170, 44, render.ShapeRoundedRect, White, ColorRGBA{}, 4, 0, .65)
	}); got != 0 {
		t.Fatal("warm shape submission allocated", got)
	}
	c.shapes = c.shapes[:0]
	c.DrawShapeMatrices(model, mgl32.Ident3(), 170, 44, render.ShapeCornerMask, White, ColorRGBA{}, 4, 0, 1)
	if c.shapes[0].Model != affineFromMat4(model) || c.shapes[0].LocalModel != affineFromMat3(mgl32.Ident3()) {
		t.Fatal("corner mask expanded beyond its quadrant")
	}
	for _, model := range [][6]float32{{}, {1, 2, 0, 2, 4, 0}} {
		local := [6]float32{1, 0, 0, 0, 1, 0}
		got, coords := c.shapeCoverageQuad(model, local)
		if got != model || coords != local {
			t.Fatal("singular shape produced a fringe", got, coords)
		}
	}
}
