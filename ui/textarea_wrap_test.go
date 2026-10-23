package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestTextareaTokenWrapPercentTracksViewport(t *testing.T) {
	world := newUIWorld(t)
	field := SpawnTextarea(world, TextOptions{Width: 300, Height: 100, Wrap: true, Value: "prefix extraordinarily_long_identifier suffix"})
	textarea := ecs.GetMut[Textarea](world, field)
	textarea.WrapTokens, textarea.WrapWidthPercent = true, 50
	for _, width := range []float32{300, 200, 400} {
		ecs.GetMut[scene.Size2D](world, field).Width = width
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		textEntity, ok := FindPart[ValuePart, scene.Text2D](world, field)
		if !ok {
			t.Fatal("textarea value text missing")
		}
		text := ecs.GetMut[scene.Text2D](world, textEntity)
		control, _ := controlViewFor(world, field)
		_, _, viewportWidth, _ := textareaContentBox(world, textEntity, textarea, control)
		if text.WrapMode != scene.TextWrapTokens || text.WrapWidth != viewportWidth*.5 {
			t.Fatalf("width %g: mode=%v width=%g viewport=%g", width, text.WrapMode, text.WrapWidth, viewportWidth)
		}
		if scene.TextLineCount2D(text) < 2 {
			t.Fatal("expected token wrapping")
		}
	}
}
