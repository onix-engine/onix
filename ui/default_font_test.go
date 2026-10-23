package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
)

func TestDefaultControlThemeUsesSharedSystemFont(t *testing.T) {
	font := onixtext.DefaultFont()
	if font == nil {
		t.Skip("system font unavailable")
	}
	world := ecs.NewWorld()
	button := SpawnButton(world, ButtonOptions{Width: 100, Height: 32, Label: "System font"})
	label, ok := FindPart[LabelPart, scene.Text2D](world, button)
	if !ok || ecs.GetRef[scene.Text2D](world, label).Font != font || DefaultTheme().Font != font {
		t.Fatal("default button did not use the shared system font")
	}
}
