package layout

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestAbsoluteAutoWidthShrinksToContent(t *testing.T) {
	for _, inset := range []string{"left:20px", "right:20px", ""} {
		t.Run(inset, func(t *testing.T) {
			_, _, _, boxes := tableFixture(t, `<div id="host"><div id="popup"><pre id="text">Short</pre></div></div>`, `#host{position:relative;width:300px}#popup{position:absolute;`+inset+`;padding:8px 10px;border:1px solid black}pre{margin:0;white-space:pre-wrap}`)
			popup, text := boxes["popup"], boxes["text"]
			if len(text.TextFragments) != 1 || !closeFloat(popup.Content.Width, text.TextFragments[0].Rect.Width) || popup.BorderBox().Width >= 300 {
				t.Fatalf("absolute popup did not shrink to text: popup=%+v text=%+v", popup.Content, text.TextFragments)
			}
		})
	}
}

func TestAbsoluteAutoWidthIncludesPreformattedTabs(t *testing.T) {
	for _, tabSize := range []string{"2", "8", "36px"} {
		t.Run(tabSize, func(t *testing.T) {
			_, _, _, boxes := tableFixture(t, `<div id="host"><div id="popup"><pre id="text"><code><span>type</span> Provider struct {
	<span>APIKey</span>&#9;<span>string</span>&#9;<span>`+"`json:&quot;api_key,omitempty&quot;`"+`</span>
}</code></pre></div></div>`, `*{box-sizing:border-box}#host{position:relative;width:1000px}#popup{position:absolute;left:20px;max-width:80%;padding:4px 6px;border:1px solid black}pre{margin:0;white-space:pre-wrap;tab-size:`+tabSize+`}`)
			popup, text := boxes["popup"], boxes["text"]
			if popup.BorderBox().Width >= 800 {
				t.Fatalf("short tabbed code should fit below the width limit: %+v", popup.BorderBox())
			}
			found := false
			for _, fragment := range text.TextFragments {
				if strings.Contains(fragment.Text, "json:") {
					found = true
					if fragment.Line != 1 {
						t.Fatalf("tabbed field wrapped before the width limit: popup=%+v line=%d", popup.BorderBox(), fragment.Line)
					}
				}
			}
			if !found {
				t.Fatal("struct field tag was not rendered")
			}
		})
	}
}

func TestRetainedAbsoluteTabbedWidthDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div style="position:relative;width:1000px"><div id="popup" style="position:absolute;left:16px;max-width:80%"><pre style="margin:0;white-space:pre-wrap"><span>APIKey</span>&#9;<span>string</span>&#9;<span>json:&quot;api_key,omitempty&quot;</span></pre></div></div>`)
	var cache Cache
	popup := retainedNode(root, "popup")
	dirty := []*dom.Node{popup}
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(popup)
	allocations := testing.AllocsPerRun(100, func() {
		for _, left := range [...]float64{20, 16} {
			mutable.Left = style.LengthPx(left)
			if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
				panic(err)
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("warm tabbed popup layout allocated %g objects", allocations)
	}
}

func TestAbsoluteAutoWidthNestedFlexMenus(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<header><nav id="menus"><div class="menu" id="file-host"><button id="file">File</button><div class="popup">Wide popup content must not size the menu</div></div><div class="menu" id="edit-host"><button id="edit">Edit</button></div><div class="menu" id="debug-host"><button id="debug">Debug</button></div><div class="menu" id="about-host"><button id="about">About</button></div></nav></header>`, `*{box-sizing:border-box}header{position:relative;width:300px;height:38px}nav{position:absolute;left:16px;top:0;display:flex;gap:4px;height:36px}.menu{position:relative;display:flex;align-items:center;height:36px}button{display:flex;align-items:center;justify-content:center;padding:0 8px;border:0;white-space:nowrap}.popup{position:absolute;top:36px;width:300px}`)
	wantWidth, previousRight := 0.0, boxes["menus"].Content.X
	for i, id := range []string{"file", "edit", "debug", "about"} {
		button, host := boxes[id], boxes[id+"-host"]
		if len(button.Children) != 1 || len(button.Children[0].TextFragments) != 1 {
			t.Fatalf("missing menu text for %s", id)
		}
		textWidth := button.Children[0].TextFragments[0].Rect.Width
		if !closeFloat(button.Content.Width, textWidth) || !closeFloat(host.Content.Width, textWidth+16) {
			t.Fatalf("%s compressed: host=%+v button=%+v text width=%g", id, host.Content, button.Content, textWidth)
		}
		if i > 0 {
			wantWidth += 4
			if !closeFloat(host.BorderBox().X-previousRight, 4) {
				t.Fatalf("%s lost its menu gap", id)
			}
		}
		wantWidth += host.BorderBox().Width
		previousRight = host.BorderBox().X + host.BorderBox().Width
	}
	if !closeFloat(boxes["menus"].Content.Width, wantWidth) {
		t.Fatalf("menu width=%g, want %g", boxes["menus"].Content.Width, wantWidth)
	}
}

func TestAbsoluteAutoWidthFlexAxesAndWrapping(t *testing.T) {
	for _, fixture := range []struct {
		css   string
		width float64
	}{
		{"flex-direction:row", 112},
		{"flex-direction:row-reverse", 112},
		{"flex-direction:column", 62},
		{"flex-direction:column-reverse", 62},
		{"flex-wrap:wrap", 90},
	} {
		t.Run(fixture.css, func(t *testing.T) {
			_, _, _, boxes := tableFixture(t, `<div id="host"><div id="flex"><div id="first"></div><div id="second"></div><div class="popup"></div></div></div>`, `#host{position:relative;width:90px}#flex{position:absolute;display:flex;gap:6px;padding:2px;border:1px solid black;`+fixture.css+`}#first{width:40px;margin:0 2px}#second{width:50px;padding:0 2px;border:1px solid black}.popup{position:absolute;width:500px}`)
			if got := boxes["flex"].BorderBox().Width; !closeFloat(got, fixture.width) {
				t.Fatalf("flex width=%g, want %g", got, fixture.width)
			}
		})
	}
}

func TestRetainedAbsoluteFlexIntrinsicWidthDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<header style="position:relative;width:300px"><nav id="menus" style="position:absolute;left:16px;display:flex;gap:4px"><div style="display:flex"><button style="display:flex;padding:0 8px;white-space:nowrap">File</button></div><div style="display:flex"><button style="display:flex;padding:0 8px;white-space:nowrap">Edit</button></div></nav></header>`)
	var cache Cache
	menus := retainedNode(root, "menus")
	dirty := []*dom.Node{menus}
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(menus)
	allocations := testing.AllocsPerRun(100, func() {
		for _, left := range [...]float64{20, 16} {
			mutable.Left = style.LengthPx(left)
			if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
				panic(err)
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("warm absolute flex layout allocated %g objects", allocations)
	}
}

func TestAbsoluteAutoWidthWrapsAndRespectsInsets(t *testing.T) {
	for _, fixture := range []struct {
		css   string
		width float64
	}{
		{"left:20px", 258},
		{"right:20px", 258},
		{"left:20px;right:30px", 228},
		{"left:20px;max-width:160px", 160},
	} {
		t.Run(fixture.css, func(t *testing.T) {
			_, _, _, boxes := tableFixture(t, `<div id="host"><div id="popup"><pre id="text">`+strings.Repeat("short words ", 30)+`</pre></div></div>`, `#host{position:relative;width:300px}#popup{position:absolute;`+fixture.css+`;padding:8px 10px;border:1px solid black}pre{margin:0;white-space:pre-wrap}`)
			popup := boxes["popup"]
			if !closeFloat(popup.Content.Width, fixture.width) || boxes["text"].Content.Height <= 20 {
				t.Fatalf("width/wrapping mismatch: popup=%+v text=%+v", popup.Content, boxes["text"].Content)
			}
		})
	}
}
