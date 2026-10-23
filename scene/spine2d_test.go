package scene

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestSpineModelMatrix2DMatchesWorldAndUIDisplayYAxis(t *testing.T) {
	transform := mgl32.Translate3D(30, 40, 0)
	worldModel := mgl32.Scale3D(2, -2, 1).Mul4(SpineModelMatrix2D(transform, false))
	uiModel := mgl32.Scale3D(2, 2, 1).Mul4(SpineModelMatrix2D(transform, true))

	worldOrigin := worldModel.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	worldUp := worldModel.Mul4x1(mgl32.Vec4{0, 1, 0, 1})
	uiOrigin := uiModel.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	uiUp := uiModel.Mul4x1(mgl32.Vec4{0, 1, 0, 1})
	worldY := worldUp.Y() - worldOrigin.Y()
	uiY := uiUp.Y() - uiOrigin.Y()
	if worldY != -2 || uiY != worldY {
		t.Fatalf("Spine screen Y direction differs: world=%v ui=%v", worldY, uiY)
	}
}
