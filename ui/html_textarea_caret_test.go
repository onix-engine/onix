package ui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLTextareaCaretRespectsWhiteSpaceWrapping(t *testing.T) {
	value := strings.Repeat("source ", 40) + "\nsecond line\n\nend"
	for _, test := range []struct {
		name       string
		whiteSpace string
		wrap       bool
	}{
		{name: "default", wrap: true},
		{name: "pre", whiteSpace: "pre"},
		{name: "nowrap", whiteSpace: "nowrap"},
		{name: "pre-wrap", whiteSpace: "pre-wrap", wrap: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			RegisterRuntimeFeature[HTMLRenderer](world)
			style := "width:200px;height:800px;padding:20px 24px;border:0;font-size:13px;line-height:1.65"
			initialStyle := style
			if test.whiteSpace != "" {
				initialStyle += ";white-space:" + test.whiteSpace
			}
			html := htmlui.New(`<textarea id="ed" style="` + initialStyle + `">` + value + `</textarea>`)
			html.Width, html.Height, html.Font = 220, 820, notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, html)
			owner := world.Spawn(bundle)
			verify := func(wrap bool) {
				t.Helper()
				for range 2 {
					if err := SyncSystem("").Run(world); err != nil {
						t.Fatal(err)
					}
				}
				control := htmlControlByID(t, world, owner, "ed")
				entity, _ := htmlTextareaValueText(t, world, control)
				text := ecs.GetMut[scene.Text2D](world, entity)
				caret := strings.IndexByte(value, '\n') + 1 + len("second")
				x, y, _, height, ok := scene.TextCaretRect2D(text, value, caret)
				if !ok || math.Abs(float64(height-21.45)) > 0.01 {
					t.Fatalf("caret line height = %v, ok=%v", height, ok)
				}
				if wrap && y <= height || !wrap && math.Abs(float64(y-height)) > 0.01 {
					t.Fatalf("second source line y=%v, height=%v, wrap=%v", y, height, wrap)
				}
				if got := TextCaretIndex(text, value, x, y+height/2); got != caret {
					t.Fatalf("click on second source line placed caret at %d, want %d", got, caret)
				}
				blankCaret := strings.Index(value, "\n\n") + 1
				x, y, _, height, ok = scene.TextCaretRect2D(text, value, blankCaret)
				if !ok || x != 0 || TextCaretIndex(text, value, 100, y+height/2) != blankCaret {
					t.Fatalf("blank line caret geometry = (%v, %v), ok=%v", x, y, ok)
				}
			}
			verify(test.wrap)
			node, _ := htmlui.NodeByID(world, owner, "ed")
			nextWhiteSpace := "pre-wrap"
			if test.wrap {
				nextWhiteSpace = "pre"
			}
			if !node.SetAttribute("style", style+";white-space:"+nextWhiteSpace) {
				t.Fatal("could not change textarea white-space")
			}
			verify(!test.wrap)
		})
	}
}

// An HTML <textarea> overlays its highlighted code, so its caret must sit at the
// CSS content-box origin. The widget's default text padding (12,10) would offset
// the caret from the lines it edits.
func TestHTMLTextareaCaretUsesCSSContentOrigin(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	ecs.InsertResource(world, input.NewState())

	const lines = 12
	body := make([]string, lines)
	for i := range body {
		body[i] = fmt.Sprintf("line %d", i)
	}
	value := strings.Join(body, "\n")
	// Height comfortably exceeds the content (12 * 21.45 + 40 padding) so the
	// textarea never scrolls internally and the value text rests at its origin.
	html := htmlui.New(`<textarea id="ed" style="width:300px;height:400px;margin:0;padding:20px 24px;border:0;font-size:13px;line-height:1.65;white-space:pre">` + value + `</textarea>`)
	html.Width, html.Height, html.Font = 360, 420, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}

	control := htmlControlByID(t, world, owner, "ed")
	_, valueTransform := htmlTextareaValueText(t, world, control)
	if got := float64(valueTransform.Translation.X.Value); math.Abs(got-24) > 0.01 {
		t.Fatalf("value text x = %.2f, want the CSS content origin 24", got)
	}
	if got := float64(valueTransform.Translation.Y.Value); math.Abs(got-20) > 0.01 {
		t.Fatalf("value text y = %.2f, want the CSS content origin 20", got)
	}

	// Move the caret to the start of line 8 and confirm it lands on that line's
	// box: content origin + line index * line-height (13px * 1.65 = 21.45).
	const caretLine = 8
	caret := 0
	for i := 0; i < caretLine; i++ {
		caret += len(body[i]) + 1
	}
	if field := ecs.GetMut[TextField](world, control); field != nil {
		field.Focused, field.Caret, field.Anchor = true, caret, caret
	} else {
		t.Fatal("textarea control has no TextField")
	}
	for range 2 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}

	wantY := 20 + caretLine*21.45
	caretTransform, ok := ecs.Get[scene.UITransform](world, htmlCaretVisual(t, world, control))
	if !ok {
		t.Fatal("caret visual has no UITransform")
	}
	if got := float64(caretTransform.Translation.Y.Value); math.Abs(got-wantY) > 0.01 {
		t.Fatalf("caret y = %.2f, want %.2f (line %d at the CSS content origin)", got, wantY, caretLine)
	}
	if got := float64(caretTransform.Translation.X.Value); math.Abs(got-24) > 0.01 {
		t.Fatalf("caret x = %.2f, want 24 (start of line at the CSS content origin)", got)
	}
}

func htmlTextareaValueText(t *testing.T, world *ecs.World, control ecs.Entity) (ecs.Entity, *scene.UITransform) {
	t.Helper()
	var entity ecs.Entity
	for candidate, access := range ecs.Query[Relation[scene.Text2D]](world).All {
		relation := access.Ref()

		if relation.Owner == control && ecs.Has[Part[ValuePart]](world, candidate) {
			entity = candidate
			break
		}
	}
	if entity == 0 {
		t.Fatal("textarea has no value text visual")
	}
	transform := ecs.GetMut[scene.UITransform](world, entity)
	if transform == nil {
		t.Fatal("value text visual has no UITransform")
	}
	return entity, transform
}

func htmlCaretVisual(t *testing.T, world *ecs.World, control ecs.Entity) ecs.Entity {
	t.Helper()
	var found ecs.Entity
	for entity, _ := range ecs.Query[Part[CaretPart]](world).All {
		if relation, ok := ecs.Get[Relation[SurfaceStyle]](world, entity); ok && relation.Owner == control {
			found = entity
			break
		}
	}
	if found == 0 {
		t.Fatal("textarea has no caret visual")
	}
	return found
}
