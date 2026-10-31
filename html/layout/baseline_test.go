package layout

import (
	"math"
	"testing"
)

func TestInlineFlexAlignsTextBaselineAndCentersIcon(t *testing.T) {
	for _, font := range []string{"12px/1.2 ui", "14px/1.7 ui", "18px/2 monospace"} {
		t.Run(font, func(t *testing.T) {
			_, _, _, boxes := tableFixture(t, `<p id="line">Before <button id="link"><img id="icon"><span id="label">Link</span></button> After</p>`,
				`p {font:`+font+`} button {display:inline-flex;align-items:baseline;gap:4px;padding:0;border:0;font:inherit} img {align-self:center;width:14px;height:14px;flex-shrink:0}`)
			line, label := boxes["line"], boxes["label"]
			if len(line.TextFragments) != 2 || len(label.TextFragments) != 1 {
				t.Fatal("missing surrounding or link text")
			}
			want := line.TextFragments[0].Glyphs[0].Y
			for _, fragment := range []TextFragment{line.TextFragments[1], label.TextFragments[0]} {
				if got := fragment.Glyphs[0].Y; math.Abs(got-want) > 1e-9 {
					t.Fatalf("%s baseline = %g, want %g", fragment.Text, got, want)
				}
			}
			icon, link := boxes["icon"].BorderBox(), boxes["link"].Content
			if math.Abs(icon.Y+icon.Height/2-link.Y-link.Height/2) > 1e-9 {
				t.Fatalf("icon is not centered in link: icon=%+v link=%+v", icon, link)
			}
			if n := testing.AllocsPerRun(100, func() { firstBoxBaseline(boxes["link"]) }); n != 0 {
				t.Fatalf("baseline lookup allocated: %g", n)
			}
		})
	}
}

func TestInlineFlexSynthesizesFirstImageBaseline(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<p id="line">Before <button id="link"><img id="icon"><span>Link</span></button> After</p>`,
		`p {font:14px/1.7 ui} button {display:inline-flex;align-items:center;padding:0;border:0;font:inherit} img {width:14px;height:14px}`)
	icon := boxes["icon"].BorderBox()
	want := icon.Y + icon.Height
	for _, fragment := range boxes["line"].TextFragments {
		if got := fragment.Glyphs[0].Y; math.Abs(got-want) > 1e-9 {
			t.Fatalf("%s baseline = %g, want first image border at %g", fragment.Text, got, want)
		}
	}
}

func TestInlineFlexSharesBaselineAcrossFontSizes(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<p id="line">Before <span id="link"><span id="small">Small</span><span id="large">Large</span></span> After</p>`,
		`p {font:14px/1.7 ui} #link {display:inline-flex;align-items:baseline} #large {font-size:22px}`)
	want := boxes["line"].TextFragments[0].Glyphs[0].Y
	for _, id := range []string{"small", "large"} {
		if got := boxes[id].TextFragments[0].Glyphs[0].Y; math.Abs(got-want) > 1e-9 {
			t.Fatalf("%s baseline = %g, want %g", id, got, want)
		}
	}
}

func TestInlineFlexExportsFirstWrappedLineBaseline(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<p id="line">Before <span id="link"><img id="icon"><span id="label">Link</span></span> After</p>`,
		`p {font:14px/1.7 ui} #link {display:inline-flex;align-items:baseline;flex-wrap:wrap;width:24px} img {align-self:center;width:14px;height:14px} #label {width:24px}`)
	icon := boxes["icon"].BorderBox()
	want := icon.Y + icon.Height
	if boxes["label"].BorderBox().Y < want {
		t.Fatal("label did not wrap to a second flex line")
	}
	for _, fragment := range boxes["line"].TextFragments {
		if got := fragment.Glyphs[0].Y; math.Abs(got-want) > 1e-9 {
			t.Fatalf("%s baseline = %g, want first line's image border at %g", fragment.Text, got, want)
		}
	}
}
