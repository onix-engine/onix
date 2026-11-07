package html

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/scene"
	notosanssc "github.com/onix-engine/onix/testdata/fonts/Noto_Sans_SC"
	"testing"
)

func TestHTMLItalicAndBoldItalic(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	regular, bold := notosanssc.NotoSansSCRegular, notosanssc.NotoSansSCBold
	bundle.Put(Surface{Value: `<div>regular <em>italic</em> <span style="font:italic bold 16px sans-serif">both</span><i style="font-style:normal">reset</i></div>`, Width: 400, Height: 80, Font: regular, BoldFont: bold})
	owner := world.Spawn(bundle)
	found := map[string]bool{}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	for entity, access := range world.Query[htmlChild]().All {
		if access.Get().Owner != owner {
			continue
		}
		text, ok := world.Get[scene.Text2D](entity)
		if !ok {
			continue
		}
		switch text.Value {
		case "italic":
			found["italic"] = text.Font == regular.Italic()
		case "both":
			found["both"] = text.Font == bold.Italic()
		case "reset":
			found["reset"] = text.Font == regular
		}
	}
	for _, key := range []string{"italic", "both", "reset"} {
		if !found[key] {
			t.Errorf("%s font selection failed", key)
		}
	}
}

func TestItalicInvalidatesTextRaster(t *testing.T) {
	computed := &style.ComputedStyle{}
	box := &layout.Box{TextFragments: []layout.TextFragment{{Style: computed}}}
	regular := htmlTextRasterHash(box)
	computed.FontStyle = style.FontItalic
	if htmlTextRasterHash(box) == regular {
		t.Fatal("italic reused regular text raster")
	}
}
