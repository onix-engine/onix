package layout

import (
	"testing"

	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestFloatsExcludeInlineTextOnBothSides(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<div id="host"><div id="left"></div><div id="right"></div><p id="text">one two three four five six seven eight nine ten eleven twelve</p></div>`, `#host{width:160px}#left{float:left;width:40px;height:40px}#right{float:right;width:30px;height:60px}`)
	left, right, text := boxes["left"].MarginBox(), boxes["right"].MarginBox(), boxes["text"]
	if left.X != 0 || right.X != 130 {
		t.Fatalf("float positions: left=%+v right=%+v", left, right)
	}
	wrapped := false
	for _, fragment := range text.TextFragments {
		if fragment.Rect.Y < left.Y+left.Height && fragment.Text != " " {
			wrapped = true
			if fragment.Rect.X < left.X+left.Width {
				t.Fatalf("text overlaps left float: %+v", fragment.Rect)
			}
		}
		if fragment.Rect.Y < right.Y+right.Height && fragment.Rect.X+fragment.Rect.Width > right.X+.01 && fragment.Text != " " {
			t.Fatalf("text overlaps right float: %+v", fragment.Rect)
		}
	}
	if !wrapped {
		t.Fatal("text never occupied space alongside floats")
	}
}

func TestFloatClearAndFlowRootContainment(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<div id="host"><div id="left"></div><div id="right"></div><div id="cleared"></div></div><div id="next"></div>`, `#host{display:flow-root;width:100px}#left{float:left;width:20px;height:30px}#right{float:right;width:20px;height:60px}#cleared{clear:both;height:10px}#next{height:5px}`)
	if boxes["cleared"].Content.Y != 60 || boxes["host"].Content.Height != 70 || boxes["next"].Content.Y != 70 {
		t.Fatalf("clear/containment: clear=%+v host=%+v next=%+v", boxes["cleared"].Content, boxes["host"].Content, boxes["next"].Content)
	}
}

func TestFloatAutoWidthUsesShrinkToFit(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<div id="host"><div id="float">Short</div></div>`, `#host{display:flow-root;width:200px}#float{float:left}`)
	floated := boxes["float"]
	if floated.Content.Width != floated.TextFragments[0].Rect.Width || floated.Content.Width >= 200 {
		t.Fatalf("float did not shrink to intrinsic width: %+v", floated.Content)
	}
}

func TestFloatIntrinsicMeasurementPreservesParentInlineText(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<div id="host"><div id="float">Short</div>Parent text</div>`, `#host{display:flow-root;width:200px}#float{float:left}`)
	text := ""
	for _, fragment := range boxes["host"].TextFragments {
		text += fragment.Text
	}
	if text != "Parent text" {
		t.Fatalf("parent text overwritten during float measurement: %q", text)
	}
}

func TestFlowRootAvoidsAdjacentFloat(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<div id="host"><div id="float"></div><div id="separate">Text</div></div>`, `#host{width:150px;display:flow-root}#float{float:left;width:40px;height:50px}#separate{display:flow-root}`)
	if boxes["separate"].BorderBox().X != 40 || boxes["separate"].BorderBox().Width != 110 {
		t.Fatalf("flow-root overlaps float: %+v", boxes["separate"].BorderBox())
	}
}

func TestFloatIncrementalLayoutMatchesFreshAfterWidthChange(t *testing.T) {
	doc, styles, _, boxes := tableFixture(t, `<div id="host"><div id="float"></div><p id="text">one two three four five six seven eight nine ten</p></div>`, `#host{width:160px;display:flow-root}#float{float:left;width:40px;height:60px}`)
	face := loadLayoutTestFace(t)
	cache := Cache{}
	if _, err := cache.Update(doc, styles, face, nil, 320, 200, nil, nil, true, true); err != nil {
		t.Fatal(err)
	}
	node := boxes["float"].Node
	for _, width := range []float64{70, 20, 40} {
		styles.Exclusive(node).Width = style.LengthPx(width)
		cache.Restyle([]*dom.Node{node}, styles)
		got, err := cache.Update(doc, styles, face, nil, 320, 200, []*dom.Node{node}, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		want, err := Build(doc, styles, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, want)
	}
}
