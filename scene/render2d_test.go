package scene

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	onixtext "github.com/diiyw/onix/text"
	"github.com/go-gl/mathgl/mgl32"
)

// A laid-out glyph is stored once per character of every document a code editor
// keeps open, so its size is the engine's dominant text memory cost.
func TestTextGlyph2DStaysCompact(t *testing.T) {
	if size := unsafe.Sizeof(textGlyph2D{}); size != 88 {
		t.Fatalf("textGlyph2D size = %d bytes, want 88", size)
	}
}

func TestEntityInSubtree2DResolvesAncestorsWithoutAllocating(t *testing.T) {
	world := ecs.NewWorld()
	root := world.Spawn(ecs.NewBundle(0))
	child := world.Spawn(ecs.NewBundle(0))
	grandchild := world.Spawn(ecs.NewBundle(0))
	other := world.Spawn(ecs.NewBundle(0))
	if err := ecs.Insert(world, child, Parent{Entity: root}); err != nil {
		t.Fatal(err)
	}
	if err := ecs.Insert(world, grandchild, Parent{Entity: child}); err != nil {
		t.Fatal(err)
	}
	if entityInSubtree2D(world, root, root) != true || entityInSubtree2D(world, grandchild, root) != true || entityInSubtree2D(world, other, root) {
		t.Fatal("subtree ancestry was resolved incorrectly")
	}
}

func TestText2DMaxWidthEllipsizesLongLines(t *testing.T) {
	text := NewText2D("The quick brown fox jumps")
	text.Font = notosanssc.NotoSansSCRegular
	fullWidth, _, ok := MeasureText2D(&text)
	if !ok || fullWidth <= 0 {
		t.Fatal("expected full text to be measurable")
	}
	text.MaxWidth = fullWidth * 0.55
	width, _, ok := MeasureText2D(&text)
	if !ok {
		t.Fatal("expected constrained text to be measurable")
	}
	value := text.ellipsizedValue()
	if value == text.Value || !strings.HasSuffix(value, "...") {
		t.Fatalf("expected ellipsized text, got %q", value)
	}
	if width > text.MaxWidth+1 {
		t.Fatalf("ellipsized width %.2f exceeds max width %.2f", width, text.MaxWidth)
	}

	text.MaxWidth = 0
	if got := text.ellipsizedValue(); got != text.Value {
		t.Fatalf("zero max width should preserve text, got %q", got)
	}
}

func TestText2DRasterScalePreservesLogicalLayout(t *testing.T) {
	base := NewText2D("A界")
	base.Font = notosanssc.NotoSansSCRegular
	baseWidth, baseHeight, ok := MeasureText2D(&base)
	if !ok {
		t.Fatal("expected base text to be measurable")
	}

	text := NewText2D(base.Value)
	text.Font = base.Font
	text.rasterScale = 2
	width, height, ok := MeasureText2D(&text)
	if !ok {
		t.Fatal("expected high-resolution text to be measurable")
	}
	if math.Abs(float64(width-baseWidth)) > 2 || math.Abs(float64(height-baseHeight)) > 1 {
		t.Fatalf("logical text size changed at 2x: got (%v, %v), want (%v, %v)", width, height, baseWidth, baseHeight)
	}
	if !text.ensureLayout() || !base.ensureLayout() {
		t.Fatal("expected text to be shaped")
	}
	if len(text.glyphs) == 0 || text.glyphs[0].raster.Width <= base.glyphs[0].raster.Width {
		t.Fatalf("glyph atlas was not supersampled: base=%+v raster=%+v", base.glyphs[0].raster, text.glyphs[0].raster)
	}
}

func TestText2DClayWordWrapping(t *testing.T) {
	text := NewText2D("alpha beta gamma")
	text.Font = notosanssc.NotoSansSCRegular
	wordWidth, ok := MeasureTextMinimumWidth2D(&text)
	if !ok {
		t.Fatal("expected word to be measurable")
	}
	text.WrapWidth = wordWidth + 2
	width, wrappedHeight, ok := MeasureText2D(&text)
	if !ok || text.lineCount != 3 {
		t.Fatalf("word-wrapped text width=%v height=%v lines=%d ok=%v", width, wrappedHeight, text.lineCount, ok)
	}
	if width > text.WrapWidth+1 {
		t.Fatalf("wrapped line width %.2f exceeds wrap width %.2f", width, text.WrapWidth)
	}
	_, lineHeight, _ := MeasureTextValue2D(&text, "alpha")
	if wrappedHeight < lineHeight*3 {
		t.Fatalf("wrapped height %.2f is less than three lines %.2f", wrappedHeight, lineHeight*3)
	}
}

func TestText2DClayWrappingPreservesLongWordsAndNewlines(t *testing.T) {
	longWord := "extraordinary"
	text := NewText2D(longWord)
	text.Font = notosanssc.NotoSansSCRegular
	wordWidth, _, _ := MeasureTextValue2D(&text, longWord)
	text.WrapWidth = wordWidth * .5
	width, _, ok := MeasureText2D(&text)
	if !ok || text.lineCount != 1 || width <= text.WrapWidth {
		t.Fatalf("long word width=%v wrap=%v lines=%d ok=%v", width, text.WrapWidth, text.lineCount, ok)
	}

	text.Value = "alpha beta\ngamma delta"
	text.WrapMode = TextWrapNewlines
	text.WrapWidth = 1
	if _, _, ok := MeasureText2D(&text); !ok || text.lineCount != 2 {
		t.Fatalf("newline wrapping lines=%d ok=%v", text.lineCount, ok)
	}
	text.WrapMode = TextWrapNone
	if _, _, ok := MeasureText2D(&text); !ok || text.lineCount != 2 {
		t.Fatalf("no-wrap explicit newlines lines=%d ok=%v", text.lineCount, ok)
	}
}

func TestText2DClayLetterSpacingAndLineHeight(t *testing.T) {
	text := NewText2D("ABCD")
	text.Font = notosanssc.NotoSansSCRegular
	plainWidth, _, ok := MeasureText2D(&text)
	if !ok {
		t.Fatal("expected text measurement")
	}

	text.LetterSpacing = 3
	spacedWidth, _, ok := MeasureText2D(&text)
	if !ok || math.Abs(float64(spacedWidth-plainWidth-9)) > .01 {
		t.Fatalf("letter-spaced width=%v plain=%v ok=%v", spacedWidth, plainWidth, ok)
	}

	text.Value = "AB\nCD"
	text.LineHeight = 40
	_, height, ok := MeasureText2D(&text)
	lines, linesOK := TextLineRects2D(&text)
	if !ok || !linesOK || height != 80 || len(lines) != 2 || lines[0].Height != 40 || lines[1].Y != 40 {
		t.Fatalf("line-height layout height=%v lines=%#v ok=%v/%v", height, lines, ok, linesOK)
	}
}

func TestText2DClayLineAlignmentAndCaretGeometry(t *testing.T) {
	text := NewText2D("alpha\nb")
	text.Font = notosanssc.NotoSansSCRegular
	text.WrapMode = TextWrapNewlines
	text.WrapWidth = 160
	text.TextAlignment = TextAlignCenter
	text.LetterSpacing = 2

	width, _, ok := MeasureText2D(&text)
	lines, linesOK := TextLineRects2D(&text)
	if !ok || !linesOK || width != 160 || len(lines) != 2 || lines[0].X <= 0 || lines[1].X <= lines[0].X {
		t.Fatalf("centered text width=%v lines=%#v ok=%v/%v", width, lines, ok, linesOK)
	}
	if index := TextCaretIndex2D(&text, text.Value, lines[1].X-1, lines[1].Y+1); index != 6 {
		t.Fatalf("caret before centered second line=%d, want 6", index)
	}

	text.TextAlignment = TextAlignRight
	rightLines, rightOK := TextLineRects2D(&text)
	if !rightOK || rightLines[0].X <= lines[0].X || rightLines[1].X <= lines[1].X {
		t.Fatalf("right-aligned lines=%#v centered=%#v", rightLines, lines)
	}
}

func TestText2DLineBaselineSplitsFontLineGap(t *testing.T) {
	font, err := onixtext.LoadFont(append([]byte(nil), notosanssc.NotoSansSCRegular.Data...))
	if err != nil {
		t.Fatalf("load test font: %v", err)
	}
	font.Face.Hhea.LineGap = 400
	defer font.Dispose()

	text := NewText2D("gy")
	text.Font = font
	if !text.ensureLayout() || len(text.glyphs) != 2 {
		t.Fatal("expected both descender glyphs to be laid out")
	}
	metrics := text.face.Metrics()
	lineHeight := fixed26_6ToFloat32(metrics.Height)
	ascent := fixed26_6ToFloat32(metrics.Ascent)
	descent := fixed26_6ToFloat32(metrics.Descent)
	leading := lineHeight - ascent - descent
	if leading <= 0 {
		t.Fatalf("test font line gap = %v, want positive", leading)
	}
	baseline := text.glyphs[0].y - text.glyphs[0].raster.OffsetY
	want := ascent + leading*.5
	if math.Abs(float64(baseline-want)) > 1.0/64 {
		t.Fatalf("gy baseline = %v, want %v (ascent=%v leading=%v)", baseline, want, ascent, leading)
	}
}

func TestText2DEllipsizedValueDoesNotAllocateWhenTextFits(t *testing.T) {
	text := NewText2D("Play")
	text.Font = notosanssc.NotoSansSCRegular
	text.MaxWidth = 200
	text.ensureFace()
	if allocations := testing.AllocsPerRun(100, func() {
		if got := text.ellipsizedValue(); got != text.Value {
			t.Fatalf("ellipsized value = %q, want %q", got, text.Value)
		}
	}); allocations != 0 {
		t.Fatalf("fitting text allocated %.0f objects, want 0", allocations)
	}
}

func TestDrawScreenText2DDoesNotAllocateAfterWarmup(t *testing.T) {
	ctx := graphics.Canvas2DContext(321, 181)
	text := NewText2D("Immediate")
	text.Font = notosanssc.NotoSansSCRegular
	if !DrawScreenText2D(ctx, &text, mgl32.Ident4(), AnchorTopLeft) {
		t.Fatal("expected immediate text to draw")
	}

	if allocations := testing.AllocsPerRun(100, func() {
		if !DrawScreenText2D(ctx, &text, mgl32.Ident4(), AnchorTopLeft) {
			t.Fatal("expected immediate text to draw")
		}
	}); allocations != 0 {
		t.Fatalf("immediate text allocated %.0f objects, want 0", allocations)
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatal(err)
	}
}

func TestText2DOutlineUsesExpandedGlyphLayerBehindFill(t *testing.T) {
	text := NewText2D("A")
	text.Font = notosanssc.NotoSansSCRegular
	text.OutlineWidth = 2.5
	text.OutlineColor = graphics.ColorFromRGBA(12, 34, 56, 255)
	if !text.ensureLayout() || len(text.glyphs) != 1 {
		t.Fatal("expected outlined text layout")
	}
	glyph := text.glyphs[0]
	if len(text.outlineGlyphs) != len(text.glyphs) {
		t.Fatalf("outline layer is not index-parallel: glyphs=%d outlines=%d", len(text.glyphs), len(text.outlineGlyphs))
	}
	if outline := text.outlineGlyphs[0]; outline.Width <= glyph.raster.Width || outline.Height <= glyph.raster.Height {
		t.Fatalf("outline glyph was not expanded: fill=%+v outline=%+v", glyph.raster, outline)
	}
	items := text.appendRenderItems(nil, 7, mgl32.Ident4(), clipRect2D{}, true, 3)
	if len(items) != 2 || !items[0].outlined || items[1].outlined {
		t.Fatalf("outline and fill render layers = %+v", items)
	}
	if items[0].color != text.OutlineColor || items[0].sequence >= items[1].sequence {
		t.Fatalf("outline layer order/color is invalid: %+v", items)
	}
}

func TestTextCaretIndex2DHandlesUnicodeAndMultipleLines(t *testing.T) {
	text := NewText2D("你a\ncd")
	text.Font = notosanssc.NotoSansSCRegular
	_, lineHeight, ok := MeasureTextValue2D(&text, "")
	if !ok {
		t.Fatal("expected text face")
	}
	if index := TextCaretIndex2D(&text, text.Value, -1, 0); index != 0 {
		t.Fatalf("unexpected first-line caret index: %d", index)
	}
	if index := TextCaretIndex2D(&text, text.Value, 1000, lineHeight+1); index != 5 {
		t.Fatalf("unexpected second-line caret index: %d", index)
	}
}

func TestMeasureTextRangeRects2DSplitsUTF8RangeAcrossLines(t *testing.T) {
	text := NewText2D("prefix 你好\nworld link")
	text.Font = notosanssc.NotoSansSCRegular
	start := strings.Index(text.Value, "你好")
	end := strings.Index(text.Value, "link") + len("link")
	rects, ok := MeasureTextRangeRects2D(&text, start, end)
	if !ok || len(rects) != 2 {
		t.Fatalf("range rectangles = %#v ok=%v", rects, ok)
	}
	if rects[0].Width <= 0 || rects[1].Width <= 0 || rects[1].Y <= rects[0].Y || rects[0].Height != rects[1].Height {
		t.Fatalf("invalid range rectangles: %#v", rects)
	}
	if _, ok := MeasureTextRangeRects2D(&text, -1, end); ok {
		t.Fatal("accepted an invalid text range")
	}
}

func TestText2DInlineObjectsReserveWidthAndExpandLines(t *testing.T) {
	value := "A\ufffcB\nC\ufffcD"
	first := strings.Index(value, "\ufffc")
	second := strings.LastIndex(value, "\ufffc")
	text := NewText2D(value)
	text.Font = notosanssc.NotoSansSCRegular
	_, defaultLineHeight, ok := MeasureTextValue2D(&text, "A")
	if !ok {
		t.Fatal("expected font measurement")
	}
	text.InlineObjects = []TextInlineObject2D{
		{Start: first, End: first + len("\ufffc"), Width: 52, Height: 30, Baseline: 22},
		{Start: second, End: second + len("\ufffc"), Width: 34, Height: 24, Baseline: 17},
	}
	width, height, ok := MeasureText2D(&text)
	if !ok || width < 52 || height <= defaultLineHeight*2 {
		t.Fatalf("inline layout width=%v height=%v defaultLine=%v ok=%v", width, height, defaultLineHeight, ok)
	}
	if !text.ensureLayout() {
		t.Fatal("expected inline text to be shaped")
	}
	if len(text.glyphs) != 4 {
		t.Fatalf("placeholder glyphs were rendered: %#v", text.glyphs)
	}
	rects, ok := TextInlineObjectRects2D(&text)
	if !ok || len(rects) != 2 {
		t.Fatalf("inline object rects = %#v ok=%v", rects, ok)
	}
	if rects[0].Rect.Width != 52 || rects[0].Rect.Height != 30 || rects[1].Rect.Y <= rects[0].Rect.Y || rects[1].Rect.Width != 34 {
		t.Fatalf("inline object geometry = %#v", rects)
	}
	firstRect := rects[0].Rect
	if index := TextCaretIndex2D(&text, text.Value, firstRect.X+firstRect.Width*0.25, firstRect.Y+firstRect.Height/2); index != 1 {
		t.Fatalf("caret before inline object midpoint = %d; want 1", index)
	}
	if index := TextCaretIndex2D(&text, text.Value, firstRect.X+firstRect.Width*0.75, firstRect.Y+firstRect.Height/2); index != 2 {
		t.Fatalf("caret after inline object midpoint = %d; want 2", index)
	}
	lineRects, ok := TextLineRects2D(&text)
	if !ok || len(lineRects) != 2 || lineRects[0].Height < 30 || lineRects[1].Y != lineRects[0].Height || lineRects[1].Height < 24 {
		t.Fatalf("dynamic line rects = %#v ok=%v", lineRects, ok)
	}
	rangeRects, ok := MeasureTextRangeRects2D(&text, first, second+len("\ufffc"))
	if !ok || len(rangeRects) != 2 || rangeRects[0].Width < 52 || rangeRects[1].Width < 34 {
		t.Fatalf("inline range rects = %#v ok=%v", rangeRects, ok)
	}

	previousWidth := width
	text.InlineObjects[0].Width = 80
	width, _, ok = MeasureText2D(&text)
	if !ok || width <= previousWidth+20 {
		t.Fatalf("inline object cache was not invalidated: before=%v after=%v", previousWidth, width)
	}
}

func TestText2DInlineObjectsParticipateInEllipsis(t *testing.T) {
	value := "A\ufffcB"
	start := strings.Index(value, "\ufffc")
	text := NewText2D(value)
	text.Font = notosanssc.NotoSansSCRegular
	text.InlineObjects = []TextInlineObject2D{{Start: start, End: start + len("\ufffc"), Width: 100, Height: 20, Baseline: 15}}
	text.MaxWidth = 60
	if !text.ensureFace() {
		t.Fatal("expected text face")
	}
	if got := text.ellipsizedValue(); !strings.HasSuffix(got, "...") || strings.Contains(got, "\ufffc") {
		t.Fatalf("ellipsized inline text = %q", got)
	}
	width, _, ok := MeasureText2D(&text)
	if !ok || width > text.MaxWidth+1 {
		t.Fatalf("ellipsized inline width=%v max=%v ok=%v", width, text.MaxWidth, ok)
	}
}

func TestSyncGlobalTransformSystemPropagatesParentMatrix(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(3)
	ecs.Put(&parentBundle, NewTransform())
	ecs.Put(&parentBundle, GlobalTransform{})
	ecs.Put(&parentBundle, Visibility{Visible: true})
	parent := world.Spawn(parentBundle)
	childBundle := ecs.NewBundle(4)
	childTransform := TransformFromXYZ(4, 6, 0)
	childTransform.Scale = mgl32.Vec3{2, 3, 1}
	ecs.Put(&childBundle, childTransform)
	ecs.Put(&childBundle, Parent{Entity: parent})
	ecs.Put(&childBundle, GlobalTransform{})
	ecs.Put(&childBundle, Visibility{Visible: true})
	child := world.Spawn(childBundle)
	if transform := ecs.GetMut[Transform](world, parent); transform != nil {
		transform.Translation = mgl32.Vec3{10, 20, 0}
	}

	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatalf("sync transforms: %v", err)
	}

	global, ok := ecs.Get[GlobalTransform](world, child)
	if !ok {
		t.Fatalf("expected child global transform")
	}
	point := global.Matrix.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	if point.X() != 14 || point.Y() != 26 {
		t.Fatalf("unexpected child translation: (%f, %f)", point.X(), point.Y())
	}
}

func TestSprite2DFromTextureCopiesTextureSize(t *testing.T) {
	stub := graphicsTextureStub{width: 32, height: 18}
	sprite := Sprite2DFromTexture(stub.texture())
	if sprite.Width != 32 || sprite.Height != 18 || sprite.Tint != graphics.White {
		t.Fatalf("unexpected sprite size: %f x %f", sprite.Width, sprite.Height)
	}
}

func TestColorSpritesShareWhiteTextureAndUseInstanceTint(t *testing.T) {
	red := NewColorSprite2D(graphics.ColorFromRGBA(255, 0, 0, 255), 20, 10)
	blue := NewColorSprite2D(graphics.ColorFromRGBA(0, 0, 255, 255), 20, 10)
	if red.Texture != blue.Texture || red.Texture.BatchKey() != blue.Texture.BatchKey() {
		t.Fatal("color sprites did not share the white batch texture")
	}
	if red.Tint == blue.Tint || red.Tint.R != 1 || blue.Tint.B != 1 {
		t.Fatalf("color sprite tints = %#v / %#v", red.Tint, blue.Tint)
	}
}

func TestSprite2DFromTextureUsesLogicalTrimmedSize(t *testing.T) {
	texture := graphics.CreateTextureByColor(graphics.White)
	texture.SetSourceRect(24, 20, 3, 4)
	sprite := Sprite2DFromTexture(texture)
	width, height := sprite.Size()
	if sprite.Width != 24 || sprite.Height != 20 || width != 24 || height != 20 {
		t.Fatalf("unexpected logical sprite size: %f x %f", width, height)
	}
}

func TestNineSliceAxisPreservesBordersAndCollapsesSmallCenter(t *testing.T) {
	source, target := nineSliceAxis(60, 120, 12, 8)
	if source != [4]float32{0, 12, 52, 60} || target != [4]float32{0, 12, 112, 120} {
		t.Fatalf("unexpected expanded axis: source=%v target=%v", source, target)
	}

	_, target = nineSliceAxis(60, 10, 12, 8)
	if target != [4]float32{0, 6, 6, 10} {
		t.Fatalf("small target must proportionally fit borders: %v", target)
	}
}

func TestNineSliceAxisSpansPreserveTrimmedTransparentMargins(t *testing.T) {
	spans, count := nineSliceAxisSpans(55, 3, 50, 100, 15, 15)
	if count != 3 {
		t.Fatalf("span count = %d; want 3", count)
	}
	want := [3]nineSliceAxisSpan{
		{textureOffset: 0, textureSize: 12.0 / 50, targetOffset: 3, targetSize: 12},
		{textureOffset: 12.0 / 50, textureSize: 25.0 / 50, targetOffset: 15, targetSize: 70},
		{textureOffset: 37.0 / 50, textureSize: 13.0 / 50, targetOffset: 85, targetSize: 13},
	}
	if spans != want {
		t.Fatalf("trimmed spans = %#v; want %#v", spans, want)
	}
}

func TestNineSliceAxisSpansScaleTrimmedCenter(t *testing.T) {
	spans, count := nineSliceAxisSpans(178, 3, 172, 356, 0, 0)
	if count != 1 {
		t.Fatalf("span count = %d; want 1", count)
	}
	want := nineSliceAxisSpan{textureOffset: 0, textureSize: 1, targetOffset: 6, targetSize: 344}
	if spans[0] != want {
		t.Fatalf("trimmed center span = %#v; want %#v", spans[0], want)
	}
}

func TestNineSliceAxisSpansUseLogicalDimensionsForHighResolutionTextures(t *testing.T) {
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 40, 20)))
	texture.SetLogicalSize(20, 10)
	if texture.Trimmed() {
		t.Fatal("high-resolution texture was treated as trimmed")
	}
	scaleX, scaleY := texture.RasterScale()
	xSpans, xCount := nineSliceAxisSpans(float32(texture.LogicalWidth()), float32(texture.OffsetX)/scaleX, float32(texture.Width)/scaleX, 60, 4, 4)
	ySpans, yCount := nineSliceAxisSpans(float32(texture.LogicalHeight()), float32(texture.OffsetY)/scaleY, float32(texture.Height)/scaleY, 30, 3, 3)
	if xCount != 3 || yCount != 3 || xSpans[2].textureOffset+xSpans[2].textureSize != 1 || ySpans[2].textureOffset+ySpans[2].textureSize != 1 {
		t.Fatalf("high-resolution slices did not cover the complete texture: x=%#v/%d y=%#v/%d", xSpans, xCount, ySpans, yCount)
	}
}

func TestRenderViewport2DVisibleRejectsOffscreenBounds(t *testing.T) {
	viewport := renderViewport2D{
		clipFromLayer: mgl32.Ortho(-50, 50, -40, 40, -1000, 1000),
		active:        true,
	}

	visible := viewport.visible(mgl32.Translate3D(10, 10, 0), 16, 16)
	if !visible {
		t.Fatalf("expected onscreen bounds to be visible")
	}

	visible = viewport.visible(mgl32.Translate3D(120, 10, 0), 16, 16)
	if visible {
		t.Fatalf("expected offscreen bounds to be culled")
	}
}

func TestDynamic2DItemBoundsRejectOffscreenTextAndSpine(t *testing.T) {
	viewport := renderViewport2D{clipFromLayer: mgl32.Ortho2D(0, 100, 80, 0), active: true}
	text := NewText2D("bounds")
	text.OutlineWidth = 2
	textItem := renderItem2D{text: &text, transform: mgl32.Translate3D(10, 10, 0), width: 30, height: 12}
	if !textRenderItemVisible2D(viewport, &textItem) {
		t.Fatal("onscreen text bounds were culled")
	}
	textItem.transform = mgl32.Translate3D(200, 10, 0)
	if textRenderItemVisible2D(viewport, &textItem) {
		t.Fatal("offscreen text bounds were visible")
	}

	if !spineRenderItemVisible2D(viewport, mgl32.Translate3D(20, 20, 0), clipRect2D{}, -5, -5, 5, 5, true) {
		t.Fatal("onscreen Spine bounds were culled")
	}
	if spineRenderItemVisible2D(viewport, mgl32.Translate3D(200, 20, 0), clipRect2D{}, -5, -5, 5, 5, true) {
		t.Fatal("offscreen Spine bounds were visible")
	}
	clip := clipRect2D{minX: 60, minY: 60, maxX: 70, maxY: 70, valid: true}
	if spineRenderItemVisible2D(viewport, mgl32.Translate3D(20, 20, 0), clip, -5, -5, 5, 5, true) {
		t.Fatal("Spine bounds outside their UI clip were visible")
	}
}

func TestAppendSpineMeshBatchBakesTransformsAndMergesDraws(t *testing.T) {
	texture := render.NewHandle[render.TextureResource](1, 1)
	makeBatch := func(x float32, blend render.MeshBlendMode) render.MeshBatch3D {
		return render.MeshBatch3D{
			ViewProjection: [16]float32(mgl32.Ident4()),
			Vertices: []render.Vertex3D{
				{Position: [3]float32{0, 1, 0}},
				{Position: [3]float32{1, 0, 0}},
				{Position: [3]float32{2, 1, 0}},
			},
			Indices:   []uint32{0, 1, 2},
			Instances: []render.Instance3D{{Model: [16]float32(mgl32.Translate3D(x, 0, 0))}},
			Draws:     []render.MeshDraw3D{{Texture: texture, IndexCount: 3, InstanceCount: 1, AlphaMode: uint32(graphics.AlphaBlend), BlendMode: blend}},
		}
	}

	var merged render.MeshBatch3D
	appendSpineMeshBatch(&merged, makeBatch(10, render.MeshBlendNormal))
	appendSpineMeshBatch(&merged, makeBatch(20, render.MeshBlendNormal))
	if len(merged.Instances) != 1 || len(merged.Draws) != 1 || merged.Draws[0].IndexCount != 6 {
		t.Fatalf("merged Spine batch = instances %d, draws %d, index count %d; want 1/1/6", len(merged.Instances), len(merged.Draws), merged.Draws[0].IndexCount)
	}
	if got := merged.Vertices[0].Position[0]; got != 10 {
		t.Fatalf("first baked vertex x = %v, want 10", got)
	}
	if got := merged.Vertices[3].Position[0]; got != 20 {
		t.Fatalf("second baked vertex x = %v, want 20", got)
	}
	if got := merged.Indices[3]; got != 3 {
		t.Fatalf("second geometry index = %d, want 3", got)
	}

	var split render.MeshBatch3D
	appendSpineMeshBatch(&split, makeBatch(10, render.MeshBlendNormal))
	appendSpineMeshBatch(&split, makeBatch(20, render.MeshBlendAdditive))
	if len(split.Draws) != 2 || split.Draws[1].BlendMode != render.MeshBlendAdditive {
		t.Fatalf("blend modes were merged incorrectly: %+v", split.Draws)
	}
}

func registerTestRenderers2D(world *ecs.World) {
	RegisterSpriteRenderer2D(world)
	RegisterSpriteLayersRenderer2D(world)
	RegisterTextRenderer2D(world)
	RegisterUIRenderer2D(world)
	RegisterSpriteTrailRenderer2D(world)
	RegisterParticleRenderer2D(world)
	RegisterTilemapRenderer2D(world)
	RegisterSpineRenderer2D(world)
}

func TestCollectRenderItems2DCullsBeforeSorting(t *testing.T) {
	world := ecs.NewWorld()
	registerTestRenderers2D(world)
	cameraEntity := SpawnCamera2D(world)
	camera := ecs.GetMut[Camera](world, cameraEntity)
	camera.UpdateViewport(100, 80)
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for _, x := range []float32{10, 500} {
		bundle := ecs.NewBundle(3)
		ecs.Put(&bundle, Sprite2D{
			Shape:  RoundedRectShape2D(graphics.White, 4),
			Width:  20,
			Height: 20,
			Alpha:  1,
		})
		ecs.Put(&bundle, GlobalTransform{Matrix: mgl32.Translate3D(x, 10, 0)})
		ecs.Put(&bundle, Visibility{Visible: true})
		world.Spawn(bundle)
	}

	items := collectRenderItems2D(graphics.Canvas2DContext(100, 80), world, true)
	if len(items) != 1 || items[0].transform[12] != 10 {
		t.Fatalf("unexpected visible render items: %+v", items)
	}
}

func TestRender2DCullsQuadsAtDrawTime(t *testing.T) {
	render.Uninstall()
	driver := &canvasTestDriver{viewport: render.Rect{Width: 103, Height: 83}}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install render driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	ctx := graphics.Canvas2DContext(103, 83)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	world := ecs.NewWorld()

	graphics.BindFrame(world, graphics.CurrentContext())
	RegisterSpriteRenderer2D(world)
	texture := graphics.CreateTextureByColor(graphics.White)
	for _, x := range []float32{10, 500} {
		for _, sprite := range []Sprite2D{
			Sprite2DFromTexture(texture),
			{Shape: RoundedRectShape2D(graphics.White, 2), Width: 20, Height: 20, Alpha: 1},
		} {
			bundle := ecs.NewBundle(3)
			ecs.Put(&bundle, sprite)
			ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Translate3D(x, 10, 0)})
			ecs.Put(&bundle, Visibility{Visible: true})
			world.Spawn(bundle)
		}
	}

	items := collectRenderItems2D(ctx, world, false)
	if len(items) != 4 {
		t.Fatalf("extracted render items = %d, want 4 before per-camera culling", len(items))
	}
	drawRenderItems2D(ctx, world, items, DrawViews2D{}, true)
	if err := ctx.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	if driver.sprites != 1 || driver.shapes != 1 {
		t.Fatalf("submitted sprite/shape instances = %d/%d, want 1/1", driver.sprites, driver.shapes)
	}
}

func TestCollectRenderItems2DSkipsZeroAlphaVisuals(t *testing.T) {
	world := ecs.NewWorld()
	registerTestRenderers2D(world)
	texture := graphics.CreateTextureByColor(graphics.White)
	for _, bundle := range []ecs.Bundle{
		func() ecs.Bundle {
			value := ecs.NewBundle(3)
			ecs.Put(&value, Sprite2D{Shape: RoundedRectShape2D(graphics.White, 4), Width: 20, Height: 20})
			ecs.Put(&value, GlobalTransform{Matrix: mgl32.Ident4()})
			ecs.Put(&value, Visibility{Visible: true})
			return value
		}(),
		func() ecs.Bundle {
			value := ecs.NewBundle(5)
			ecs.Put(&value, Tilemap2D{
				Texture: texture, Tiles: []int{1},
				Width: 1, Height: 1, TileWidth: 16, TileHeight: 16, Columns: 1,
			})
			ecs.Put(&value, GlobalTransform{Matrix: mgl32.Ident4()})
			ecs.Put(&value, Visibility{Visible: true})
			return value
		}(),
		func() ecs.Bundle {
			value := ecs.NewBundle(3)
			ecs.Put(&value, Sprite2D{Texture: texture, Tint: graphics.ColorRGBA{}, Width: 20, Height: 20, Alpha: 1})
			ecs.Put(&value, GlobalTransform{Matrix: mgl32.Ident4()})
			ecs.Put(&value, Visibility{Visible: true})
			return value
		}(),
		func() ecs.Bundle {
			value := ecs.NewBundle(4)
			ecs.Put(&value, Sprite2D{})
			ecs.Put(&value, SpriteLayers2D{Layers: []SpriteLayer2D{{
				Sprite: Sprite2DFromTexture(texture), Visible: true,
			}}})
			ecs.Put(&value, GlobalTransform{Matrix: mgl32.Ident4()})
			ecs.Put(&value, Visibility{Visible: true})
			return value
		}(),
		func() ecs.Bundle {
			value := ecs.NewBundle(6)
			ecs.Put(&value, UIGlobalTransform{Matrix: mgl32.Ident4()})
			ecs.Put(&value, Visibility{Visible: true})
			ecs.Put(&value, Size2D{Width: 20, Height: 20})
			ecs.Put(&value, UIBackground2D{Shape: RoundedRectShape2D(graphics.White, 4)})
			ecs.Put(&value, UIImage2D{Texture: texture, Tint: graphics.White})
			ecs.Put(&value, UIBorder2D{Shape: OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, graphics.White, 4, 1)})
			return value
		}(),
	} {
		world.Spawn(bundle)
	}

	items := collectRenderItems2D(graphics.Canvas2DContext(100, 80), world, false)
	if len(items) != 0 {
		t.Fatalf("zero-alpha render items = %d, want 0", len(items))
	}
}

func TestText2DRenderItemsSkipTransparentColorRuns(t *testing.T) {
	text := NewText2D("A")
	text.Font = notosanssc.NotoSansSCRegular
	text.Color = graphics.ColorRGBA{}
	if !text.ensureLayout() {
		t.Fatal("expected transparent text layout")
	}
	if items := text.appendRenderItems(nil, 1, mgl32.Ident4(), clipRect2D{}, true, 0); len(items) != 0 {
		t.Fatalf("transparent text render items = %d, want 0", len(items))
	}
}

func TestExtractRender2DCopiesSimulationComponents(t *testing.T) {
	ctx := graphics.Canvas2DContext(100, 80)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.CurrentContext())
	RegisterSpriteRenderer2D(world)
	bundle := ecs.NewBundle(3)
	ecs.Put(&bundle, Sprite2D{Shape: RoundedRectShape2D(graphics.White, 2), Width: 20, Height: 10, Alpha: 1})
	ecs.Put(&bundle, GlobalTransform{Matrix: mgl32.Translate3D(5, 7, 0)})
	ecs.Put(&bundle, Visibility{Visible: true})
	entity := world.Spawn(bundle)

	if err := ExtractRender2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*RenderWorld](world)
	if len(state.Items2D) != 1 {
		t.Fatalf("extracted items = %d, want 1", len(state.Items2D))
	}
	item := state.Items2D[0]
	component := ecs.GetMut[Sprite2D](world, entity)
	component.Width = 99
	global := ecs.GetMut[GlobalTransform](world, entity)
	global.Matrix = mgl32.Translate3D(50, 70, 0)

	if item.sprite == component {
		t.Fatal("render item retained a pointer to the simulation component")
	}
	if item.sprite.Width != 20 || item.transform[12] != 5 || item.transform[13] != 7 {
		t.Fatalf("render snapshot changed with simulation world: %+v transform=%v", item.sprite, item.transform)
	}
}

func TestUIVisualLayersExtractBackgroundImageAndBorder(t *testing.T) {
	world := ecs.NewWorld()
	registerTestRenderers2D(world)
	texture := graphics.CreateTextureByColor(graphics.White)
	slice := NineSlice2D{Left: 1, Right: 1, Top: 1, Bottom: 1}
	tint := graphics.ColorFromRGBA(12, 34, 56, 255)
	bundle := ecs.NewBundle(8)
	ecs.Put(&bundle, NewUITransform())
	ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Ident4()})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, AnchorTopLeft)
	ecs.Put(&bundle, Size2D{Width: 40, Height: 20})
	ecs.Put(&bundle, NewUIBackground2D(RoundedRectShape2D(graphics.Black, 4)))
	ecs.Put(&bundle, UIImage2D{Texture: texture, Slice: &slice, Mode: UIImageSliced, Tint: tint, Alpha: 0.35})
	ecs.Put(&bundle, NewUIBorder2D(OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, graphics.White, 4, 2)))
	entity := world.Spawn(bundle)

	items := collectRenderItems2D(graphics.Canvas2DContext(100, 80), world, false)
	buildRenderPhase2DBatches(items)
	if len(items) != 3 {
		t.Fatalf("UI visual render items = %d, want 3", len(items))
	}
	if items[0].entity != entity || items[0].kind != renderItemShape2D || items[1].kind != renderItemSprite2D || items[2].kind != renderItemShape2D {
		t.Fatalf("unexpected UI visual layer order: %+v", items)
	}
	if items[1].sprite.Texture != texture || items[1].sprite.Slice == nil || *items[1].sprite.Slice != slice || items[1].sprite.Alpha != 0.35 || items[1].sprite.Tint != tint {
		t.Fatalf("unexpected UI image item: %+v", items[1])
	}
	if items[1].width != 40 || items[1].height != 20 {
		t.Fatalf("UI image size = %vx%v, want 40x20", items[1].width, items[1].height)
	}
}

func TestUIImageAutoFitsInsideNode(t *testing.T) {
	world := ecs.NewWorld()
	registerTestRenderers2D(world)
	texture := graphics.CreateTextureByColor(graphics.White)
	texture.SourceWidth, texture.SourceHeight = 20, 10
	bundle := ecs.NewBundle(5)
	ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Ident4()})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, AnchorTopLeft)
	ecs.Put(&bundle, Size2D{Width: 30, Height: 30})
	ecs.Put(&bundle, NewUIImage2D(texture))
	world.Spawn(bundle)

	items := collectRenderItems2D(graphics.Canvas2DContext(100, 80), world, false)
	if len(items) != 1 || items[0].width != 30 || items[0].height != 15 || items[0].transform[13] != 7.5 {
		t.Fatalf("auto-fitted UI image = %+v", items)
	}
}

type customMaterial2DTest struct {
	shader *graphics.Material2DShader
	value  float32
}

func (m customMaterial2DTest) Material2D() graphics.Material2DData {
	return graphics.Material2DData{Shader: m.shader, Custom: [4][4]float32{{m.value}}}
}

func TestRegisteredMaterial2DIsExtractedWithSprite(t *testing.T) {
	render.Uninstall()
	shader, err := graphics.NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatalf("create deferred Material2D shader: %v", err)
	}
	world := ecs.NewWorld()
	RegisterSpriteRenderer2D(world)
	RegisterMaterial2D[customMaterial2DTest](world)
	material := customMaterial2DTest{shader: shader, value: 0.65}
	bundle := ecs.NewBundle(4)
	ecs.Put(&bundle, Sprite2DFromTexture(graphics.CreateTextureByColor(graphics.White)))
	ecs.Put(&bundle, MeshMaterial2D[customMaterial2DTest]{Material: &material})
	ecs.Put(&bundle, GlobalTransform{Matrix: mgl32.Ident4()})
	ecs.Put(&bundle, Visibility{Visible: true})
	world.Spawn(bundle)

	items := collectRenderItems2D(graphics.Canvas2DContext(100, 80), world, false)
	if len(items) != 1 || items[0].material2D.Shader != shader || items[0].material2D.Custom[0][0] != material.value {
		t.Fatalf("unexpected extracted Material2D: %+v", items)
	}
	if items[0].material.shaderID != shader.BatchKey() {
		t.Fatalf("Material2D batch key = %d, want %d", items[0].material.shaderID, shader.BatchKey())
	}
}

func TestRender2DCoordinateSpaceComesFromTransformType(t *testing.T) {
	ctx := graphics.Canvas2DContext(100, 80)
	world := ecs.NewWorld()
	RegisterSpriteRenderer2D(world)
	for _, ui := range []bool{false, true} {
		bundle := ecs.NewBundle(4)
		if ui {
			ecs.Put(&bundle, UIStackIndex(-100))
		} else {
			ecs.Put(&bundle, RenderOrder2D(100))
		}
		ecs.Put(&bundle, Sprite2D{Shape: RoundedRectShape2D(graphics.White, 2), Width: 20, Height: 10, Alpha: 1})
		if ui {
			ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Ident4()})
		} else {
			ecs.Put(&bundle, GlobalTransform{Matrix: mgl32.Ident4()})
		}
		ecs.Put(&bundle, Visibility{Visible: true})
		world.Spawn(bundle)
	}
	items := collectRenderItems2D(ctx, world, false)
	if len(items) != 2 || items[0].ui || !items[1].ui {
		t.Fatalf("coordinate spaces were inferred from layer values: %+v", items)
	}
}

func TestCollectRenderItemsExpandsSpriteLayers(t *testing.T) {
	world := ecs.NewWorld()
	RegisterSpriteLayersRenderer2D(world)
	first := graphics.CreateTextureByColor(graphics.White)
	second := graphics.CreateTextureByColor(graphics.Black)
	t.Cleanup(func() {
		_ = first.Dispose()
		_ = second.Dispose()
	})
	layers := SpriteLayers2D{Layers: []SpriteLayer2D{
		{ID: "back", Order: 1, Sprite: Sprite2DFromTexture(first), Visible: true},
		{ID: "front", Order: 2, Sprite: Sprite2DFromTexture(second), Visible: true},
	}}
	spawner := ecs.NewSpawner5[Sprite2D, SpriteLayers2D, GlobalTransform, Visibility, RenderOrder2D](world)
	spawner.Spawn(NewSprite2D(), layers, GlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, 10)
	items := collectRenderItems2D(graphics.Canvas2DContext(64, 64), world, false)
	if len(items) != 2 || items[0].sortIndex != 11 || items[1].sortIndex != 12 || items[0].texture != first || items[1].texture != second {
		t.Fatalf("layer render items = %+v", items)
	}
}

func TestCollectRenderItemsExpandsUISpriteLayers(t *testing.T) {
	world := ecs.NewWorld()
	RegisterSpriteLayersRenderer2D(world)
	texture := graphics.CreateTextureByColor(graphics.White)
	t.Cleanup(func() { _ = texture.Dispose() })
	layers := SpriteLayers2D{Layers: []SpriteLayer2D{{
		ID: "layer", Order: 3, Sprite: Sprite2DFromTexture(texture), Visible: true,
	}}}
	spawner := ecs.NewSpawner5[Sprite2D, SpriteLayers2D, UIGlobalTransform, Visibility, UIStackIndex](world)
	spawner.Spawn(NewSprite2D(), layers, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, 8)
	items := collectRenderItems2D(graphics.Canvas2DContext(64, 64), world, false)
	if len(items) != 1 || !items[0].ui || items[0].sortIndex != 11 || items[0].texture != texture {
		t.Fatalf("UI layer render items = %+v", items)
	}
}

func TestVisibleRenderItems2DRespectsLayersPipelineAndUITarget(t *testing.T) {
	world := ecs.NewWorld()
	defaultUI := ecs.Entity(10)
	otherCamera := ecs.Entity(20)
	items := []renderItem2D{
		{entity: 1, layers: RenderLayer(2)},
		{entity: 2, layers: RenderLayer(3)},
		{entity: 3, layers: RenderLayer(2), ui: true},
		{entity: 4, layers: RenderLayer(2), ui: true, target: otherCamera},
	}
	camera2D := ExtractedCamera{Entity: defaultUI, Pipeline: CameraPipeline2D, RenderLayers: RenderLayer(2)}
	visible := visibleRenderItems2D(world, items, camera2D, defaultUI)
	if len(visible) != 2 || visible[0].entity != 1 || visible[1].entity != 3 {
		t.Fatalf("default 2D camera items = %+v", visible)
	}
	camera3D := ExtractedCamera{Entity: otherCamera, Pipeline: CameraPipeline3D, RenderLayers: RenderLayer(2)}
	visible = visibleRenderItems2D(world, items, camera3D, defaultUI)
	if len(visible) != 1 || visible[0].entity != 4 {
		t.Fatalf("targeted 3D camera UI items = %+v", visible)
	}
}

func TestUIRenderConfigurationInheritsFromParent(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(3)
	ecs.Put(&parentBundle, RenderLayer(130))
	ecs.Put(&parentBundle, UITargetCamera{Entity: 42})
	ecs.Put(&parentBundle, UIStackIndex(17))
	parent := world.Spawn(parentBundle)
	childBundle := ecs.NewBundle(1)
	ecs.Put(&childBundle, Parent{Entity: parent})
	child := world.Spawn(childBundle)
	cache := frameRenderHierarchy2DCache(world)
	info := renderEntityInfo2D(world, child, true, 0, &cache)
	if !info.layers.Contains(130) || info.layers.Contains(0) {
		t.Fatalf("inherited UI render layers = %+v", info.layers)
	}
	if info.target != 42 {
		t.Fatalf("inherited UI target camera = %d, want 42", info.target)
	}
	if info.sortIndex != 17 {
		t.Fatalf("inherited UI stack index = %d, want 17", info.sortIndex)
	}
	worldInfo := renderEntityInfo2D(world, child, false, 0, &cache)
	if !worldInfo.layers.Contains(0) || worldInfo.layers.Contains(130) {
		t.Fatalf("world render layers unexpectedly inherited: %+v", worldInfo.layers)
	}
}

func BenchmarkCollectRenderItems2D10000(b *testing.B) {
	world := ecs.NewWorld()
	RegisterSpriteRenderer2D(world)
	for index := range 10000 {
		x := float32(index % 200 * 16)
		y := float32(index / 200 * 16)
		bundle := ecs.NewBundle(3)
		ecs.Put(&bundle, Sprite2D{
			Shape:  RoundedRectShape2D(graphics.White, 2),
			Width:  12,
			Height: 12,
			Alpha:  1,
		})
		ecs.Put(&bundle, GlobalTransform{Matrix: mgl32.Translate3D(x, y, 0)})
		ecs.Put(&bundle, Visibility{Visible: true})
		world.Spawn(bundle)
	}
	context := graphics.Canvas2DContext(1280, 720)
	items := collectRenderItems2D(context, world, false)
	if len(items) != 10000 {
		b.Fatalf("collected %d render items, want 10000", len(items))
	}
	drawRenderPhase2D(context, world, items, DrawViews2D{}, false, nil, func(renderBatch2D, renderViewport2D, bool) {})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		items = collectRenderItems2D(context, world, false)
		drawRenderPhase2D(context, world, items, DrawViews2D{}, false, nil, func(renderBatch2D, renderViewport2D, bool) {})
	}
}

func TestBuildRenderPhase2DBatchesSortsByIndexDepthAndOrder(t *testing.T) {
	items := []renderItem2D{
		{order: 2, depth: 0, sortIndex: 8, material: renderMaterialKey{textureID: 7}},
		{order: 1, depth: 10, sortIndex: 4, material: renderMaterialKey{textureID: 3}},
		{order: 0, depth: -2, sortIndex: 4, material: renderMaterialKey{textureID: 3}},
	}

	batches := buildRenderPhase2DBatches(items)

	got := []uint64{items[0].order, items[1].order, items[2].order}
	want := []uint64{0, 1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected render order at %d: got %v want %v", i, got, want)
		}
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %+v", batches)
	}
	if batches[0].start != 0 || batches[0].end != 2 || batches[0].sortIndex != 4 {
		t.Fatalf("unexpected first batch: %+v", batches[0])
	}
}

func TestBuildRenderPhase2DBatchesKeepsUIAfterWorldRegardlessOfIndex(t *testing.T) {
	items := []renderItem2D{
		{entity: 1, ui: true, sortIndex: -100},
		{entity: 2, ui: false, sortIndex: 100},
	}

	buildRenderPhase2DBatches(items)
	if items[0].entity != 2 || items[1].entity != 1 {
		t.Fatalf("render spaces were ordered by sort index: %+v", items)
	}
}

func TestCollectRenderItemsIncludesMountedTilemap(t *testing.T) {
	world := ecs.NewWorld()
	RegisterTilemapRenderer2D(world)
	bundle := ecs.NewBundle(5)
	ecs.Put(&bundle, NewTransform())
	ecs.Put(&bundle, GlobalTransform{})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, AnchorCenter)
	ecs.Put(&bundle, Tilemap2D{
		Texture: graphics.CreateTextureByColor(graphics.White), Tiles: []int{1},
		Width: 1, Height: 1, TileWidth: 16, TileHeight: 16, Columns: 1, Alpha: 1,
	})
	entity := world.Spawn(bundle)
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatalf("sync transforms: %v", err)
	}

	items := collectRenderItems2D(graphics.Canvas2DContext(64, 64), world, false)
	if len(items) != 1 || items[0].kind != renderItemTilemap2D || items[0].order != uint64(entity) {
		t.Fatalf("unexpected tilemap render items: %+v", items)
	}
}

func TestBuildRenderPhase2DBatchesDrawsHigherZInFront(t *testing.T) {
	items := []renderItem2D{
		{order: 0, depth: 12, sortIndex: 0},
		{order: 1, depth: -4, sortIndex: 0},
		{order: 2, depth: 3, sortIndex: 0},
	}

	buildRenderPhase2DBatches(items)
	if items[0].depth != -4 || items[1].depth != 3 || items[2].depth != 12 {
		t.Fatalf("render depth order = [%v, %v, %v], want ascending so higher Z is submitted last", items[0].depth, items[1].depth, items[2].depth)
	}
}

func TestBuildRenderPhase2DBatchesKeepsAdjacentMaterialRuns(t *testing.T) {
	items := []renderItem2D{
		{order: 0, sortIndex: 5, material: renderMaterialKey{textureID: 2}},
		{order: 1, sortIndex: 5, material: renderMaterialKey{textureID: 3}},
		{order: 2, sortIndex: 5, material: renderMaterialKey{textureID: 3}},
		{order: 3, sortIndex: 5, material: renderMaterialKey{textureID: 2}},
		{order: 4, sortIndex: 6, material: renderMaterialKey{textureID: 2}},
	}

	batches := buildRenderPhase2DBatches(items)

	if len(batches) != 4 {
		t.Fatalf("expected 4 batches, got %+v", batches)
	}
	want := []renderBatch2D{
		{start: 0, end: 1, sortIndex: 5, material: renderMaterialKey{textureID: 2}},
		{start: 1, end: 3, sortIndex: 5, material: renderMaterialKey{textureID: 3}},
		{start: 3, end: 4, sortIndex: 5, material: renderMaterialKey{textureID: 2}},
		{start: 4, end: 5, sortIndex: 6, material: renderMaterialKey{textureID: 2}},
	}
	for i := range want {
		if batches[i] != want[i] {
			t.Fatalf("unexpected batch at %d: got %+v want %+v", i, batches[i], want[i])
		}
	}
}

func TestBuildRenderPhase2DBatchesUsesExplicitSequenceForEqualKeys(t *testing.T) {
	items := []renderItem2D{
		{order: 1, sequence: 2, sortIndex: 4},
		{order: 1, sequence: 0, sortIndex: 4},
		{order: 1, sequence: 1, sortIndex: 4},
	}
	buildRenderPhase2DBatches(items)
	for index := range items {
		if items[index].sequence != uint32(index) {
			t.Fatalf("unexpected sequence at %d: %d", index, items[index].sequence)
		}
	}
}

var benchmarkTextResult string

func BenchmarkText2DEllipsizedLongLine(b *testing.B) {
	text := NewText2D(strings.Repeat("The quick brown fox jumps over the lazy dog ", 8))
	text.Font = notosanssc.NotoSansSCRegular
	text.MaxWidth = 240
	text.ensureFace()
	b.ReportAllocs()
	for range b.N {
		benchmarkTextResult = text.ellipsizedValue()
	}
}

func TestText2DStyleSpansUseUTF8ByteRanges(t *testing.T) {
	base := graphics.ColorFromRGBA(10, 20, 30, 255)
	highlight := graphics.ColorFromRGBA(200, 80, 40, 255)
	text := NewText2D("A界B")
	text.Font = notosanssc.NotoSansSCRegular
	text.Color = base
	text.Spans = []TextSpan2D{{Start: 1, End: 4, Color: highlight}}
	if !text.ensureLayout() {
		t.Fatal("text layout failed")
	}
	if len(text.glyphs) != 3 {
		t.Fatalf("glyphs = %#v", text.glyphs)
	}
	wantIndexes := []int{0, 1, 4}
	for index, glyph := range text.glyphs {
		if glyph.byteIndex != int32(wantIndexes[index]) {
			t.Fatalf("glyph %d byte index = %d, want %d", index, glyph.byteIndex, wantIndexes[index])
		}
		wantColor := base
		if index == 1 {
			wantColor = highlight
		}
		if got := text.spanColor(int(glyph.byteIndex)); got != wantColor {
			t.Fatalf("glyph %d color = %#v, want %#v", index, got, wantColor)
		}
	}
}

func TestTextGlyphBandShapesOnlyWindowedLines(t *testing.T) {
	value := "one two\nthree four\nfive six\nseven eight"
	full := NewText2D(value)
	full.Font = notosanssc.NotoSansSCRegular
	if !full.ensureLayout() {
		t.Fatal("full text layout failed")
	}
	text := NewText2D(value)
	text.Font = notosanssc.NotoSansSCRegular
	second := full.lines[1]
	text.glyphBand = textGlyphBand{top: second.y + 1, bottom: second.y + second.height - 1, valid: true}
	if !text.ensureLayout() {
		t.Fatal("windowed text layout failed")
	}
	if len(text.lines) != len(full.lines) || text.layoutHeight != full.layoutHeight || text.layoutWidth != full.layoutWidth {
		t.Fatalf("windowed layout changed document geometry: lines=%d want=%d size=%v,%v want=%v,%v", len(text.lines), len(full.lines), text.layoutWidth, text.layoutHeight, full.layoutWidth, full.layoutHeight)
	}
	if len(text.glyphs) == 0 || len(text.glyphs) >= len(full.glyphs) {
		t.Fatalf("windowed glyphs = %d, full glyphs = %d", len(text.glyphs), len(full.glyphs))
	}
	shaped := make(map[int32]bool, len(text.glyphs))
	for _, glyph := range text.glyphs {
		if glyph.line != 1 {
			t.Fatalf("glyph for line %d shaped outside the band", glyph.line)
		}
		shaped[glyph.byteIndex] = true
	}
	for _, glyph := range full.glyphs {
		if _, want := shaped[glyph.byteIndex]; want != (glyph.line == 1) {
			t.Fatalf("glyph at byte %d on line %d shaped=%v", glyph.byteIndex, glyph.line, want)
		}
	}
}

func TestTextMeasurementDoesNotRasterizeOffscreenGlyphs(t *testing.T) {
	f, err := onixtext.LoadFont(notosanssc.NotoSansSCRegular.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Dispose()
	text := NewText2D("AjfyéÅ 中文。\n第二行\n\tEnd")
	text.Font, text.DPI, text.rasterScale = f, 117, 1.25
	if !text.ensureMeasuredLayout() {
		t.Fatal("measurement failed")
	}
	if text.cache.AtlasCount() != 0 || len(text.glyphs) != 0 || cap(text.outlineGlyphs) != 0 {
		t.Fatal("measurement retained glyph bitmaps or paint records")
	}
	width, height, lines := text.layoutWidth, text.layoutHeight, slices.Clone(text.lines)
	if !text.ensureLayout() || text.cache.AtlasCount() == 0 || len(text.glyphs) == 0 {
		t.Fatal("paint after measurement did not rasterize glyphs")
	}
	if text.layoutWidth != width || text.layoutHeight != height || !slices.Equal(text.lines, lines) {
		t.Fatal("rasterization changed measured geometry")
	}
	full := NewText2D(text.Value)
	full.Font, full.DPI, full.rasterScale = f, text.DPI, text.rasterScale
	if !full.ensureLayout() || !slices.Equal(text.glyphs, full.glyphs) || !slices.Equal(text.lines, full.lines) {
		t.Fatal("measurement followed by paint differs from a fresh layout")
	}
}

func TestTextGlyphBandReleasesDocumentSizedGlyphBuffer(t *testing.T) {
	text := NewText2D(strings.Repeat("The quick brown fox jumps over the lazy dog\n", 400))
	text.Font = notosanssc.NotoSansSCRegular
	if !text.ensureLayout() {
		t.Fatal("full text layout failed")
	}
	document, lines := cap(text.glyphs), text.lines
	text.glyphBand = textGlyphBand{top: lines[1].y, bottom: lines[1].y + lines[1].height, valid: true}
	if !text.ensureLayout() {
		t.Fatal("windowed text layout failed")
	}
	if document <= 8192 {
		t.Fatalf("document glyph capacity = %d, want a buffer large enough to release", document)
	}
	if got := cap(text.glyphs); got >= document {
		t.Fatalf("glyph capacity = %d after windowing, want it released from %d", got, document)
	}
}

func TestTextGlyphBandKeepsWindowUntilItMovesOutOfRange(t *testing.T) {
	text := NewText2D("text")
	text.setGlyphBand(textGlyphBand{top: 100, bottom: 200, valid: true})
	overscanned := text.glyphBand
	if !overscanned.valid || overscanned.top > 100 || overscanned.bottom < 200 {
		t.Fatalf("band = %#v, want the window covered with overscan", overscanned)
	}
	text.setGlyphBand(textGlyphBand{top: overscanned.top + 1, bottom: overscanned.bottom - 1, valid: true})
	if text.glyphBand != overscanned {
		t.Fatalf("band = %#v, want the previous range kept", text.glyphBand)
	}
	text.setGlyphBand(textGlyphBand{top: 900, bottom: 1000, valid: true})
	if got := text.glyphBand; !got.valid || got.top > 900 || got.bottom < 1000 {
		t.Fatalf("band = %#v, want the escaped window re-centered", got)
	}
}

func BenchmarkTextCaretIndex2DLongLine(b *testing.B) {
	text := NewText2D(strings.Repeat("The quick brown fox jumps over the lazy dog ", 8))
	text.Font = notosanssc.NotoSansSCRegular
	text.ensureFace()
	b.ReportAllocs()
	for range b.N {
		benchmarkTotal += float32(TextCaretIndex2D(&text, text.Value, 200, 0))
	}
}

func TestDrawRenderPhase2DBatchesUsesWorldAndCameraViews(t *testing.T) {
	ctx := graphics.Canvas2DContext(320, 180)
	world := ecs.NewWorld()
	cameraEntity := SpawnCamera2D(world)
	camera := ecs.GetMut[Camera](world, cameraEntity)
	camera.UpdateViewport(320, 180)
	transform := ecs.GetMut[Transform](world, cameraEntity)
	transform.Translation = mgl32.Vec3{24, 36, 0}
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	items := []renderItem2D{{order: 1, sortIndex: 0}}
	view := mgl32.Translate3D(7, 11, 0)

	drawRenderPhase2D(ctx, world, items, DrawViews2D{World: view}, false, nil, func(renderBatch2D, renderViewport2D, bool) {})
	assertMat4Near(t, ctx.ViewMatrix(), view)

	drawRenderPhase2D(ctx, world, items, DrawViews2D{World: view}, true, nil, func(renderBatch2D, renderViewport2D, bool) {})
	globalTransform, _ := ecs.Get[GlobalTransform](world, cameraEntity)
	assertMat4Near(t, ctx.ViewMatrix(), globalTransform.Matrix.Inv())
}

func TestBounds2DUsesBevyCenterAndTopLeftAnchors(t *testing.T) {
	x0, y0, x1, y1, ok := Bounds2D(mgl32.Translate3D(10, 20, 0), AnchorCenter, 100, 40)
	if !ok || x0 != -40 || y0 != 0 || x1 != 60 || y1 != 40 {
		t.Fatalf("unexpected center bounds: (%v,%v)-(%v,%v) ok=%v", x0, y0, x1, y1, ok)
	}
	x0, y0, x1, y1, ok = Bounds2D(mgl32.Translate3D(10, 20, 0), AnchorTopLeft, 100, 40)
	if !ok || x0 != 10 || y0 != -20 || x1 != 110 || y1 != 20 {
		t.Fatalf("unexpected top-left bounds: (%v,%v)-(%v,%v) ok=%v", x0, y0, x1, y1, ok)
	}
}

func TestUIGlobalTransformPropagatesIntersectsAndExemptsClip(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(3)
	ecs.Put(&parentBundle, UITransformFromXY(10, 20))
	ecs.Put(&parentBundle, Visibility{Visible: true})
	ecs.Put(&parentBundle, ClipRect2D{Width: 100, Height: 80})
	parent := world.Spawn(parentBundle)
	childBundle := ecs.NewBundle(4)
	ecs.Put(&childBundle, UITransformFromXY(30, 10))
	ecs.Put(&childBundle, Parent{Entity: parent})
	ecs.Put(&childBundle, Visibility{Visible: true})
	ecs.Put(&childBundle, ClipRect2D{Width: 50, Height: 90})
	child := world.Spawn(childBundle)
	leafBundle := ecs.NewBundle(3)
	ecs.Put(&leafBundle, NewUITransform())
	ecs.Put(&leafBundle, Parent{Entity: child})
	ecs.Put(&leafBundle, Visibility{Visible: true})
	leaf := world.Spawn(leafBundle)
	exemptBundle := ecs.NewBundle(4)
	ecs.Put(&exemptBundle, NewUITransform())
	ecs.Put(&exemptBundle, Parent{Entity: parent})
	ecs.Put(&exemptBundle, Visibility{Visible: true})
	ecs.Put(&exemptBundle, ClipExempt2D{})
	exempt := world.Spawn(exemptBundle)

	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatalf("sync transforms: %v", err)
	}

	leafTransform, ok := ecs.Get[UIGlobalTransform](world, leaf)
	if !ok {
		t.Fatal("expected leaf global transform")
	}
	clip := leafTransform.clip
	if !clip.valid || clip.minX != 40 || clip.minY != 30 || clip.maxX != 90 || clip.maxY != 100 {
		t.Fatalf("unexpected inherited clip: %#v", clip)
	}
	exemptTransform, ok := ecs.Get[UIGlobalTransform](world, exempt)
	if !ok {
		t.Fatal("expected exempt global transform")
	}
	if exemptTransform.clip.valid {
		t.Fatalf("expected exempt child to ignore inherited clip, got %#v", exemptTransform.clip)
	}
}

func TestIntersectClipRects2DKeepsEmptyClipValid(t *testing.T) {
	a := clipRect2D{minX: 0, minY: 0, maxX: 10, maxY: 10, valid: true}
	b := clipRect2D{minX: 20, minY: 20, maxX: 30, maxY: 30, valid: true}

	clip := intersectClipRect2D(a, b)
	if !clip.valid {
		t.Fatalf("expected empty intersection to remain a valid clip")
	}
	if _, _, ok := clippedQuad2D(mgl32.Ident4(), mgl32.Ident3(), 10, 10, clip); ok {
		t.Fatalf("expected empty clip to hide quad")
	}
}

func TestClippedQuad2DCropsModelAndTexture(t *testing.T) {
	clip := clipRect2D{minX: 20, minY: 10, maxX: 70, maxY: 40, valid: true}
	model, texture, ok := clippedQuad2D(mgl32.Ident4(), mgl32.Ident3(), 100, 50, clip)
	if !ok {
		t.Fatalf("expected clipped quad")
	}

	topLeft := model.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	bottomRight := model.Mul4x1(mgl32.Vec4{1, 1, 0, 1})
	if topLeft.X() != 20 || topLeft.Y() != 10 || bottomRight.X() != 70 || bottomRight.Y() != 40 {
		t.Fatalf("unexpected clipped model bounds: %v -> %v", topLeft, bottomRight)
	}

	texTopLeft := texture.Mul3x1(mgl32.Vec3{0, 0, 1})
	texBottomRight := texture.Mul3x1(mgl32.Vec3{1, 1, 1})
	if !nearFloat32(texTopLeft.X(), 0.2) || !nearFloat32(texTopLeft.Y(), 0.2) ||
		!nearFloat32(texBottomRight.X(), 0.7) || !nearFloat32(texBottomRight.Y(), 0.8) {
		t.Fatalf("unexpected clipped texture bounds: %v -> %v", texTopLeft, texBottomRight)
	}
}

func assertMat4Near(t *testing.T, got, want mgl32.Mat4) {
	t.Helper()
	for i := range got {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Fatalf("unexpected matrix at %d: got %v want %v", i, got, want)
		}
	}
}

func nearFloat32(got, want float32) bool {
	return math.Abs(float64(got-want)) <= 1e-5
}

type graphicsTextureStub struct {
	width  int
	height int
}

func (s *graphicsTextureStub) texture() *graphics.Texture {
	return &graphics.Texture{
		Width:  s.width,
		Height: s.height,
		MaxU:   1,
		MaxV:   1,
	}
}

func TestDrawScreenLCDTextDoesNotAllocateAfterWarmup(t *testing.T) {
	ctx := graphics.Canvas2DContext(323, 183)
	value := NewText2D("AV 中文")
	value.Font = notosanssc.NotoSansSCRegular
	value.LCD = true
	value.Color = graphics.ColorFromRGBA(40, 100, 200, 255)
	transform := mgl32.Translate3D(0.25, 0.5, 0)
	if !DrawScreenText2D(ctx, &value, transform, AnchorTopLeft) {
		t.Fatal("missing text")
	}
	if n := testing.AllocsPerRun(100, func() {
		DrawScreenText2D(ctx, &value, transform, AnchorTopLeft)
	}); n != 0 {
		t.Fatalf("warm LCD drawing allocations = %g", n)
	}
	value.Color.A = 0.5
	DrawScreenText2D(ctx, &value, transform, AnchorTopLeft)
	if n := testing.AllocsPerRun(100, func() { DrawScreenText2D(ctx, &value, transform, AnchorTopLeft) }); n != 0 {
		t.Fatalf("warm translucent LCD allocations=%g", n)
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkText2DWindowedScroll(b *testing.B) {
	text := NewText2D(strings.Repeat("The quick brown fox jumps over the lazy dog\n", 400))
	text.Font = notosanssc.NotoSansSCRegular
	if !text.ensureLayout() {
		b.Fatal("document layout failed")
	}
	lines := slices.Clone(text.lines)
	scroll := func() {
		for top := 1; top+40 < len(lines); top += 40 {
			text.glyphBand = textGlyphBand{top: lines[top].y, bottom: lines[top+39].y + lines[top+39].height, valid: true}
			if !text.ensureLayout() {
				panic("windowed layout failed")
			}
		}
	}
	scroll()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		scroll()
	}
}

// BenchmarkText2DWindowRefit covers the sequence the caret path produced in a
// live session: an unwindowed shape of the whole document, then a windowed one.
// The second rebuild releases the document-sized buffer, so its allocations show.
func BenchmarkText2DWindowRefit(b *testing.B) {
	text := NewText2D(strings.Repeat("The quick brown fox jumps over the lazy dog\n", 400))
	text.Font = notosanssc.NotoSansSCRegular
	if !text.ensureLayout() {
		b.Fatal("document layout failed")
	}
	second := text.lines[1]
	refit := func() {
		text.glyphBand = textGlyphBand{}
		if !text.ensureLayout() {
			panic("document layout failed")
		}
		text.glyphBand = textGlyphBand{top: second.y, bottom: second.y + second.height, valid: true}
		if !text.ensureLayout() {
			panic("windowed layout failed")
		}
	}
	refit()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		refit()
	}
}
