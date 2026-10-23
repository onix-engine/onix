package scene

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestAnchorPresetsMatchBevyGeometry(t *testing.T) {
	tests := []struct {
		name string
		got  Anchor
		want Anchor
	}{
		{name: "bottom left", got: AnchorBottomLeft, want: Anchor{-0.5, -0.5}},
		{name: "bottom center", got: AnchorBottomCenter, want: Anchor{0, -0.5}},
		{name: "bottom right", got: AnchorBottomRight, want: Anchor{0.5, -0.5}},
		{name: "center left", got: AnchorCenterLeft, want: Anchor{-0.5, 0}},
		{name: "center", got: AnchorCenter, want: Anchor{0, 0}},
		{name: "center right", got: AnchorCenterRight, want: Anchor{0.5, 0}},
		{name: "top left", got: AnchorTopLeft, want: Anchor{-0.5, 0.5}},
		{name: "top center", got: AnchorTopCenter, want: Anchor{0, 0.5}},
		{name: "top right", got: AnchorTopRight, want: Anchor{0.5, 0.5}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("anchor = %v, want %v", test.got, test.want)
			}
			x0, y0, x1, y1, ok := Bounds2D(mgl32.Ident4(), test.got, 100, 40)
			if !ok {
				t.Fatal("anchor bounds failed")
			}
			if !near32(x0, (-0.5-test.want.X())*100) || !near32(x1, (0.5-test.want.X())*100) ||
				!near32(y0, (-0.5-test.want.Y())*40) || !near32(y1, (0.5-test.want.Y())*40) {
				t.Fatalf("bounds = (%v, %v)-(%v, %v)", x0, y0, x1, y1)
			}
		})
	}
}

func TestTopLeftAnchorUsesOppositeYGeometryForWorldAndUI(t *testing.T) {
	transform := mgl32.Translate3D(10, 20, 0)
	worldX0, worldY0, worldX1, worldY1, ok := BoundsInSpace2D(transform, AnchorTopLeft, 100, 40, false)
	if !ok {
		t.Fatal("world bounds failed")
	}
	uiX0, uiY0, uiX1, uiY1, ok := BoundsInSpace2D(transform, AnchorTopLeft, 100, 40, true)
	if !ok {
		t.Fatal("ui bounds failed")
	}
	if worldX0 != 10 || worldX1 != 110 || worldY0 != -20 || worldY1 != 20 {
		t.Fatalf("world bounds = (%v, %v)-(%v, %v)", worldX0, worldY0, worldX1, worldY1)
	}
	if uiX0 != 10 || uiX1 != 110 || uiY0 != 20 || uiY1 != 60 {
		t.Fatalf("ui bounds = (%v, %v)-(%v, %v)", uiX0, uiY0, uiX1, uiY1)
	}
}
