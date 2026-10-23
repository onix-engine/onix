package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	onixfont "github.com/diiyw/onix/font"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	onixtext "github.com/diiyw/onix/text"
)

func TestHTMLControlFontKeepsTabWidthAcrossSiblingHover(t *testing.T) {
	face, err := onixfont.Parse(append([]byte(nil), notosanssc.NotoSansSCRegular.Data...))
	if err != nil {
		t.Fatal(err)
	}
	face.Metrics[face.GlyphIndex(' ')].AdvanceWidth += 1000
	codeFont := onixtext.NewFont(face)
	t.Cleanup(func() { _ = codeFont.Dispose() })
	world, owner := incrementalHTMLFixture(t, `<button id="row">File</button><textarea id="editor" style="width:200px;height:100px;font-size:13px;white-space:pre;tab-size:2">&#9;source</textarea><style>#row:hover{background:#445566}</style>`)
	control := incrementalControl(t, world, owner, "editor")
	row := incrementalControl(t, world, owner, "row")
	if err := ecs.Insert(world, control, ControlTextFont{Font: codeFont}); err != nil {
		t.Fatal(err)
	}
	// Pencode installs its selected code face before the next HTML sync.
	var entity ecs.Entity
	for candidate, access := range ecs.Query[ControlText](world).All {
		if access.Ref().Control == control {
			entity = candidate
			break
		}
	}
	text := ecs.GetMut[scene.Text2D](world, entity)
	if text == nil {
		t.Fatal("editor text missing")
	}
	text.Font = codeFont
	text.TabWidth, _ = ControlTextTabWidth(world, control, text)
	advance, _ := codeFont.FontFace(text.FontSize, text.DPI).GlyphAdvance(' ')
	want := float32(advance) / 64 * 2
	check := func() {
		t.Helper()
		text := ecs.GetRef[scene.Text2D](world, entity)
		if text == nil || text.Font != codeFont || text.TabWidth != want {
			t.Fatalf("editor font or tab width changed: %+v, want width %v", text, want)
		}
	}
	for _, hovered := range [...]bool{true, false, true, false} {
		if !SetControlHover(world, row, hovered) {
			t.Fatal("sibling hover failed")
		}
		check()
		incrementalSync(t, world)
		check()
	}
	// A full reconciliation (for example resizing) must honor the binding too.
	ecs.GetMut[Surface](world, owner).Width += 20
	incrementalSync(t, world)
	check()
	if allocs := testing.AllocsPerRun(100, func() {
		incrementalSync(t, world)
	}); allocs != 0 {
		t.Fatalf("stable bound editor sync allocated %g times", allocs)
	}
}
