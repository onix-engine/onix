package layout

import (
	"math"
	"testing"

	"github.com/onix-engine/onix/html/dom"
)

func TestInlineImageMiddleAlignsParentFontAndMarginBox(t *testing.T) {
	root, styles, _, boxes := tableFixture(t,
		`<p id="line">Before <span id="link"><img id="icon" src="missing.svg">Link</span> After
Tail</p>`,
		`p {font:14px/1.7 ui;white-space:pre-line} span {font-size:20px} img {font-size:8px;width:14px;height:40px;margin:3px 4px 5px 6px;vertical-align:middle}`)
	line := boxes["line"]
	if len(line.ImageFragments) != 1 {
		t.Fatal("missing inline image")
	}
	icon := line.ImageFragments[0].Rect
	if icon.Width != 14 || icon.Height != 40 {
		t.Fatalf("image size includes margins: %+v", icon)
	}
	parent, _ := styles.Get(line.ImageFragments[0].Node.Parent)
	face := loadLayoutTestFace(t)
	xHeight := float64(face.XHeight()) * parent.FontSize.LengthPx / float64(face.Head.UnitsPerEm)
	var foundLink, foundTail bool
	for _, text := range line.TextFragments {
		switch text.Text {
		case "Link":
			foundLink = true
			center := icon.Y - 3 + (3+icon.Height+5)/2
			if want := text.Glyphs[0].Y - xHeight/2; math.Abs(center-want) > .01 {
				t.Errorf("margin-box center = %g, want %g from parent font", center, want)
			}
			if gap := text.Rect.X - icon.X - icon.Width; math.Abs(gap-4) > .01 {
				t.Errorf("image-to-text gap = %g, want 4", gap)
			}
		case "Tail":
			foundTail = true
			if text.Rect.Y < icon.Y+icon.Height+5-.01 {
				t.Errorf("next line overlaps image margin box: image=%+v tail=%+v", icon, text.Rect)
			}
		}
	}
	if !foundLink || !foundTail {
		t.Fatal("missing link label or following line")
	}
	var cache Cache
	dirty := []*dom.Node{line.Node}
	if n := testing.AllocsPerRun(20, func() {
		if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
			panic(err)
		}
	}); n != 0 {
		t.Fatalf("warm inline image layout allocated %g times", n)
	}
}

func TestInlineImageMarginsParticipateInWrapping(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<p id="line"><img src="one.svg"><img src="two.svg"></p>`,
		`p {width:30px} img {width:14px;height:14px;margin-right:4px}`)
	images := boxes["line"].ImageFragments
	if len(images) != 2 || images[0].Line != 0 || images[1].Line != 1 || images[1].Rect.Y < images[0].Rect.Y+14 {
		t.Fatalf("image margins did not wrap the second image: %+v", images)
	}
}
