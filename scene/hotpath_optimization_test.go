package scene

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/diiyw/onix/ecs"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/text/piecetree"
	"github.com/go-gl/mathgl/mgl32"
)

func TestCharacterWrapPreservesMetricsAndInlineObjects(t *testing.T) {
	for _, value := range []string{"AV界AV", "A\tB\t界", "a\r\nb\n", "\n\n", "abcdef"} {
		for _, width := range []float32{1, 18, 40, 80} {
			text := NewText2D(value)
			text.Font = notosanssc.NotoSansSCRegular
			text.LetterSpacing, text.TabWidth = 1.5, 30
			if !text.ensureFace() {
				t.Fatal("font unavailable")
			}
			var expected []textWrapRange2D
			for start := 0; start <= len(value); {
				end := len(value)
				if offset := strings.IndexByte(value[start:], '\n'); offset >= 0 {
					end = start + offset
				}
				lineStart := start
				for cursor := start; cursor < end; {
					_, size := utf8.DecodeRuneInString(value[cursor:end])
					candidate := cursor + size
					if cursor > lineStart && textWidthWithInlineObjects(text.face, value[lineStart:candidate], lineStart, nil, text.LetterSpacing, text.TabWidth) > width {
						expected = append(expected, textWrapRange2D{lineStart, cursor})
						lineStart = cursor
					}
					cursor = candidate
				}
				expected = append(expected, textWrapRange2D{lineStart, end})
				if end == len(value) {
					break
				}
				start = end + 1
			}
			if got := text.wrapPreservedRanges(piecetree.FromString(value), nil, width, false); !slices.Equal(got, expected) {
				t.Fatalf("%q width=%v: got %v want %v", value, width, got, expected)
			}
		}
	}
	text := NewText2D("aXYZb")
	text.Font = notosanssc.NotoSansSCRegular
	text.WrapMode, text.WrapWidth = TextWrapCharacters, 20
	text.InlineObjects = []TextInlineObject2D{{Start: 1, End: 4, Width: 30, Height: 20, Baseline: 15}}
	if !text.ensureLayout() {
		t.Fatal("layout failed")
	}
	if len(text.lines) != 3 || text.lines[1].start != 1 || text.lines[1].end != 4 || len(text.inlineObjectLayouts) != 1 {
		t.Fatalf("inline object split across lines: %v", text.lines)
	}
}

func TestTextGlyphWindowReusesLineGeometry(t *testing.T) {
	for _, alignment := range []TextAlignment{TextAlignLeft, TextAlignCenter, TextAlignRight} {
		for _, scale := range []float64{1, 1.3, 2} {
			for _, height := range []float32{0, 12, 24} {
				text := NewText2D(strings.Repeat("AV\t界\ufffc tail\n", 12))
				text.Font = notosanssc.NotoSansSCRegular
				text.WrapWidth, text.TextAlignment, text.TabWidth = 250, alignment, 40
				text.LineHeight, text.LetterSpacing, text.OutlineWidth = height, 1.5, 1
				text.rasterScale = scale
				text.InlineObjects = []TextInlineObject2D{{Start: 6, End: 9, Width: 20, Height: 28, Baseline: 18}}
				if !text.ensureLayout() {
					t.Fatal("layout failed")
				}
				lines := slices.Clone(text.lines)
				objects := slices.Clone(text.inlineObjectLayouts)
				for _, band := range []textGlyphBand{{top: lines[3].y, bottom: lines[6].y, valid: true}, {top: lines[8].y, bottom: lines[10].y, valid: true}, {}, {top: -50, bottom: 70, valid: true}} {
					text.glyphBand = band
					if !text.ensureLayout() {
						t.Fatal("window layout failed")
					}
					if !slices.Equal(lines, text.lines) || !slices.Equal(objects, text.inlineObjectLayouts) {
						t.Fatal("window changed line or inline object geometry")
					}
					glyphs := slices.Clone(text.glyphs)
					outlines := slices.Clone(text.outlineGlyphs)
					text.changed = true
					if !text.ensureLayout() || len(text.glyphs) != len(glyphs) || !slices.Equal(outlines, text.outlineGlyphs) {
						t.Fatalf("cached window differs from fresh layout: alignment=%v scale=%v height=%v band=%v glyphs=%v/%v", alignment, scale, height, band, len(glyphs), len(text.glyphs))
					}
					for i, glyph := range glyphs {
						got := text.glyphs[i]
						if glyph.raster != got.raster || glyph.logicalWidth != got.logicalWidth || glyph.logicalHeight != got.logicalHeight || glyph.byteIndex != got.byteIndex || glyph.line != got.line || math.Abs(float64(glyph.x-got.x)) > 1e-4 || math.Abs(float64(glyph.y-got.y)) > 1e-4 {
							t.Fatalf("glyph %d differs: %v / %v", i, glyph, got)
						}
					}
				}
			}
		}
	}
}

func TestTextMeasuredLayoutBuildsAndInvalidatesGlyphWindow(t *testing.T) {
	text := NewText2D(strings.Repeat("A long line of text\n", 20))
	text.Font = notosanssc.NotoSansSCRegular
	text.rasterScale = 1.3
	text.MaxWidth = 90
	if !text.ensureMeasuredLayout() || !text.glyphlessLayout {
		t.Fatal("measure-only layout failed")
	}
	text.glyphBand = textGlyphBand{top: text.lines[3].y, bottom: text.lines[6].y, valid: true}
	if !text.ensureLayout() || len(text.glyphs) == 0 {
		t.Fatal("paint did not rebuild measured geometry")
	}
	glyphs := slices.Clone(text.glyphs)
	text.changed = true
	if !text.ensureLayout() || !slices.Equal(glyphs, text.glyphs) {
		t.Fatal("measured paint differs from fresh layout")
	}
	text.Value = "New text"
	text.MaxWidth = 0
	text.rasterScale = 2
	text.glyphBand = textGlyphBand{}
	if !text.ensureLayout() || text.layoutValue != text.Text() || text.lineCount != 1 || text.lastRasterScale != 2 {
		t.Fatal("text/scale change reused obsolete line geometry")
	}
}

func TestTransformIgnoresUnrelatedParentAndDoesNotAllocate(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner3[Transform, GlobalTransform, Parent]()
	root := spawner.Spawn(NewTransform(), GlobalTransform{}, Parent{})
	child := spawner.Spawn(TransformFromXYZ(1, 0, 0), GlobalTransform{}, Parent{root})
	uiSpawner := world.NewSpawner2[UITransform, Parent]()
	ui := uiSpawner.Spawn(NewUITransform(), Parent{})
	sync := SyncGlobalTransformSystem("")
	run := func() {
		t.Helper()
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	run()
	parent := world.GetMut[Parent](ui)
	parent.Entity = child
	run()
	if world.FrameValue[transformWorkspaceTag, transformWorkspace]().recomputed != 0 {
		t.Fatal("unrelated UI parent recomputed world transforms")
	}
	world.Remove[Parent](ui)
	run()
	if world.FrameValue[transformWorkspaceTag, transformWorkspace]().recomputed != 0 {
		t.Fatal("unrelated parent removal recomputed world transforms")
	}
	step := func() { pose := world.GetMut[Transform](root); pose.Translation[0]++; run() }
	step()
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("incremental transform allocations=%v", allocs)
	}
}

func TestUITransformIncrementalReparentAndRepair(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner4[UITransform, UIGlobalTransform, Parent, Size2D]()
	first := spawner.Spawn(UITransformFromXY(10, 0), UIGlobalTransform{}, Parent{}, Size2D{})
	second := spawner.Spawn(UITransformFromXY(50, 0), UIGlobalTransform{}, Parent{}, Size2D{})
	child := spawner.Spawn(UITransformFromXY(2, 0), UIGlobalTransform{}, Parent{first}, Size2D{})
	leaf := spawner.Spawn(UITransformFromXY(3, 0), UIGlobalTransform{}, Parent{child}, Size2D{})
	sync := SyncUIGlobalTransformSystem("")
	run := func() {
		t.Helper()
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	run()
	parent := world.GetMut[Parent](child)
	parent.Entity = second
	run()
	global, _ := world.Get[UIGlobalTransform](leaf)
	if global.Matrix[12] != 55 || world.FrameValue[uiTransformWorkspaceScratch, uiTransformWorkspace]().recomputed != 2 {
		t.Fatalf("reparented leaf=%v", global.Matrix[12])
	}
	changed := world.GetMut[UIGlobalTransform](child)
	changed.Matrix = mgl32.Ident4()
	run()
	global, _ = world.Get[UIGlobalTransform](child)
	if global.Matrix[12] != 52 {
		t.Fatal("external global write was not corrected")
	}
	world.Remove[UIGlobalTransform](child)
	if err := world.Insert(child, UIGlobalTransform{Matrix: mgl32.Ident4()}); err != nil {
		t.Fatal(err)
	}
	run()
	global, _ = world.Get[UIGlobalTransform](child)
	if global.Matrix[12] != 52 {
		t.Fatal("reinserted global transform was not corrected")
	}
	step := func() { pose := world.GetMut[UITransform](second); pose.Translation.X.Value++; run() }
	step()
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("incremental UI allocations=%v", allocs)
	}
	secondTransform, _ := world.Get[UITransform](second)
	expectedLeafX := secondTransform.Translation.X.Value + 5
	parent = world.GetMut[Parent](second)
	parent.Entity = leaf
	if sync.Run(world) == nil {
		t.Fatal("parent cycle was not detected")
	}
	parent = world.GetMut[Parent](second)
	parent.Entity = 0
	run()
	global, _ = world.Get[UIGlobalTransform](leaf)
	if global.Matrix[12] != expectedLeafX {
		t.Fatalf("cycle repair did not restore hierarchy: leaf x=%v", global.Matrix[12])
	}
	parent = world.GetMut[Parent](child)
	parent.Entity = first
	world.InsertResource(ViewportSize{Width: 400, Height: 200, ScaleFactor: 1})
	run()
	firstTransform := world.GetMut[UITransform](first)
	firstTransform.Translation.X.Value++
	run()
	global, _ = world.Get[UIGlobalTransform](leaf)
	if global.Matrix[12] != 16 {
		t.Fatalf("viewport/reparent left stale child links: leaf x=%v", global.Matrix[12])
	}
	world.Despawn(child)
	world.Despawn(leaf)
	run()
}

func BenchmarkHotpathTransformSingleChange(b *testing.B) {
	for _, ui := range []bool{false, true} {
		b.Run(fmt.Sprintf("UI=%t", ui), func(b *testing.B) {
			world := ecs.NewWorld()
			var entity ecs.Entity
			var sync ecs.SystemConfig
			if ui {
				spawner := world.NewSpawner3[UITransform, UIGlobalTransform, Size2D]()
				for range 10000 {
					entity = spawner.Spawn(NewUITransform(), UIGlobalTransform{}, Size2D{Width: 10, Height: 10})
				}
				sync = SyncUIGlobalTransformSystem("")
			} else {
				spawner := world.NewSpawner2[Transform, GlobalTransform]()
				for range 10000 {
					entity = spawner.Spawn(NewTransform(), GlobalTransform{})
				}
				sync = SyncGlobalTransformSystem("")
			}
			if err := sync.Run(world); err != nil {
				b.Fatal(err)
			}
			step := func() {
				if ui {
					pose := world.GetMut[UITransform](entity)
					pose.Translation.X.Value++
				} else {
					pose := world.GetMut[Transform](entity)
					pose.Translation[0]++
				}
				if err := sync.Run(world); err != nil {
					b.Fatal(err)
				}
			}
			step()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				step()
			}
		})
	}
}

func BenchmarkHotpathCharacterWrap(b *testing.B) {
	text := NewText2D(strings.Repeat("a", 4000))
	text.Font = notosanssc.NotoSansSCRegular
	text.WrapMode, text.WrapWidth = TextWrapCharacters, 600
	if !text.ensureMeasuredLayout() {
		b.Fatal("layout failed")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		text.changed = true
		if !text.ensureMeasuredLayout() {
			b.Fatal("layout failed")
		}
	}
}

func BenchmarkHotpathTextBandShift(b *testing.B) {
	text := NewText2D(strings.Repeat("The quick brown fox jumps over the lazy dog\n", 400))
	text.Font = notosanssc.NotoSansSCRegular
	if !text.ensureLayout() {
		b.Fatal("layout failed")
	}
	bands := [2]textGlyphBand{{top: text.lines[1].y, bottom: text.lines[40].y + text.lines[40].height, valid: true}, {top: text.lines[81].y, bottom: text.lines[120].y + text.lines[120].height, valid: true}}
	for _, band := range bands {
		text.glyphBand = band
		if !text.ensureLayout() {
			b.Fatal("layout failed")
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		text.glyphBand = bands[i%2]
		if !text.ensureLayout() {
			b.Fatal("layout failed")
		}
	}
}
