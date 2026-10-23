package text

import (
	"errors"
	"image"
	"math"
	"math/bits"
	"sync"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

const (
	defaultGlyphAtlasSize = 512
	glyphBitmapPadding    = 2
	glyphAtlasPadding     = 2
)

type Glyph struct {
	Rune    rune
	Atlas   int
	X, Y    int
	Width   int
	Height  int
	OffsetX float32
	OffsetY float32
	Advance float32
}

func (g Glyph) Visible() bool {
	return g.Width > 0 && g.Height > 0
}

type glyphAtlas struct {
	texture *graphics.Texture
	lcd     bool
	size    int
	cursorX int
	cursorY int
	rowH    int
	pending []graphics.TextureRegionUpdate
}

type glyphAtlasPool struct {
	mu      sync.Mutex
	atlases []*glyphAtlas
	dirty   bool
}

func newGlyphAtlas(size int) *glyphAtlas {
	return &glyphAtlas{
		texture: graphics.CreateEmptyTexture(size, size, render.TextureFormatR8, graphics.ResourceUsageDefault),
		size:    size,
	}
}

func (a *glyphAtlas) allocate(width, height int) (int, int, bool) {
	width += glyphAtlasPadding
	height += glyphAtlasPadding
	if width > a.size || height > a.size {
		return 0, 0, false
	}
	if a.cursorX+width > a.size {
		a.cursorX = 0
		a.cursorY += a.rowH
		a.rowH = 0
	}
	if a.cursorY+height > a.size {
		return 0, 0, false
	}
	x, y := a.cursorX, a.cursorY
	a.cursorX += width
	a.rowH = max(a.rowH, height)
	return x, y, true
}

func (a *glyphAtlas) upload(x, y int, bitmap image.Image) {
	if a.lcd {
		rgba := bitmap.(*image.RGBA)
		a.pending = append(a.pending, graphics.TextureRegionUpdate{X: x, Y: y, Width: rgba.Rect.Dx(), Height: rgba.Rect.Dy(), Pixels: rgba.Pix})
		return
	}
	if bitmap == nil {
		return
	}
	bounds := bitmap.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return
	}
	// The atlas stores coverage only; RGB from the temporary glyph bitmap is
	// intentionally discarded before the update reaches the texture shadow.
	var pixels []byte
	alphaBitmap, isAlpha := bitmap.(*image.Alpha)
	if isAlpha && alphaBitmap.Rect == bounds && alphaBitmap.Stride == width &&
		len(alphaBitmap.Pix) == width*height && bounds.Min == (image.Point{}) {
		pixels = alphaBitmap.Pix
	}
	gray, isGray := bitmap.(*image.Gray)
	if pixels == nil && isGray && gray.Rect == bounds && gray.Stride == width {
		if len(gray.Pix) == width*height && bounds.Min == (image.Point{}) {
			pixels = gray.Pix
		}
	}
	if pixels == nil {
		pixels = make([]byte, width*height)
		if isAlpha {
			for row := range height {
				offset := alphaBitmap.PixOffset(bounds.Min.X, bounds.Min.Y+row)
				copy(pixels[row*width:(row+1)*width], alphaBitmap.Pix[offset:offset+width])
			}
		} else if isGray {
			for row := range height {
				offset := gray.PixOffset(bounds.Min.X, bounds.Min.Y+row)
				copy(pixels[row*width:(row+1)*width], gray.Pix[offset:offset+width])
			}
		} else {
			for row := range height {
				for column := range width {
					_, _, _, alpha := bitmap.At(bounds.Min.X+column, bounds.Min.Y+row).RGBA()
					pixels[row*width+column] = uint8(alpha >> 8)
				}
			}
		}
	}
	a.pending = append(a.pending, graphics.TextureRegionUpdate{
		X: x, Y: y, Width: width, Height: height, Pixels: pixels,
	})
}

func (a *glyphAtlas) flushUploads() {
	if len(a.pending) == 0 {
		return
	}
	a.texture.UpdateRegions(a.pending)
	clear(a.pending)
	a.pending = a.pending[:0]
}

type GlyphCache struct {
	mu sync.Mutex

	face           *font.ScaledFace
	pool           *glyphAtlasPool
	ownsPool       bool
	glyphs         map[rune]Glyph
	phasedGlyphs   map[phasedGlyphKey]Glyph
	outlinedGlyphs map[outlinedGlyphKey]Glyph
}

type phasedGlyphKey struct {
	character rune
	x, y      uint8
	lcd       bool
	color     font.LCDColor
	preblend  bool
}

type outlinedGlyphKey struct {
	rune  rune
	width float32
}

func NewGlyphCache(f *Font, size, dpi float64) *GlyphCache {
	return newGlyphCache(f, size, dpi, new(glyphAtlasPool), true)
}

func newGlyphCache(f *Font, size, dpi float64, pool *glyphAtlasPool, ownsPool bool) *GlyphCache {
	var face *font.ScaledFace
	if f != nil {
		face = f.FontFace(size, dpi)
	}
	return &GlyphCache{
		face:           face,
		pool:           pool,
		ownsPool:       ownsPool,
		glyphs:         make(map[rune]Glyph),
		outlinedGlyphs: make(map[outlinedGlyphKey]Glyph),
	}
}

func (c *GlyphCache) Atlas(index int) (*graphics.Texture, bool) {
	if c == nil || c.pool == nil {
		return nil, false
	}
	return c.pool.atlas(index)
}

func (c *GlyphCache) AtlasCount() int {
	if c == nil || c.pool == nil {
		return 0
	}
	return c.pool.count()
}

func (c *GlyphCache) Metrics() font.Metrics {
	if c.face == nil {
		return font.Metrics{}
	}
	return c.face.Metrics()
}

func (c *GlyphCache) Kern(prev, curr rune) float32 {
	if c.face == nil || prev < 0 {
		return 0
	}
	return fixedToFloat32(c.face.Kern(prev, curr))
}

func (c *GlyphCache) Glyph(r rune) Glyph { return c.GlyphAt(r, 0, 0) }

// GlyphMetrics preserves ink bounds and bitmap padding without populating atlases.
// Measurement results are not inserted into the rasterized glyph cache.
func (c *GlyphCache) GlyphMetrics(r rune) Glyph {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g, ok := c.glyphs[r]; ok {
		return g
	}
	g := Glyph{Rune: r}
	dr, advance, ok := c.face.GlyphBounds(r)
	g.Advance = fixedToFloat32(advance)
	if ok && !dr.Empty() {
		g.Width, g.Height = dr.Dx()+2*glyphBitmapPadding, dr.Dy()+2*glyphBitmapPadding
		g.OffsetX, g.OffsetY = float32(dr.Min.X-glyphBitmapPadding), float32(dr.Min.Y-glyphBitmapPadding)
	}
	return g
}

// GlyphAt reuses a bounded set of device-pixel phases, avoiding a second
// bilinear blur when an axis-aligned glyph's origin is fractional.
func (c *GlyphCache) GlyphAt(r rune, phaseX, phaseY uint8) Glyph {
	return c.glyphAt(r, phaseX, phaseY, 0, false)
}

func (c *GlyphCache) GlyphGrayAt(r rune, phaseX, phaseY uint8, color font.LCDColor) Glyph {
	return c.glyphAt(r, phaseX, phaseY, color&511, true)
}

func (c *GlyphCache) glyphAt(r rune, phaseX, phaseY uint8, color font.LCDColor, preblend bool) Glyph {
	c.mu.Lock()
	defer c.mu.Unlock()
	phaseX, phaseY = phaseX&3, phaseY&3
	key := phasedGlyphKey{character: r, x: phaseX, y: phaseY, color: color, preblend: preblend}
	if !preblend && phaseX == 0 && phaseY == 0 {
		if g, ok := c.glyphs[r]; ok {
			return g
		}
	} else if g, ok := c.phasedGlyphs[key]; ok {
		return g
	}
	if c.face == nil {
		g := Glyph{Rune: r}
		c.storeGlyph(key, g)
		return g
	}

	dr, mask, maskp, advance, ok := c.face.Glyph(font.Point26_6{X: font.Int26_6(phaseX) * 16, Y: font.Int26_6(phaseY) * 16}, r)
	if !ok {
		g := Glyph{Rune: r, Advance: fixedToFloat32(advance)}
		c.storeGlyph(key, g)
		return g
	}

	advancePx := fixedToFloat32(advance)
	if dr.Empty() {
		g := Glyph{Rune: r, Advance: advancePx}
		c.storeGlyph(key, g)
		return g
	}

	bitmap := glyphBitmap(mask, maskp, dr)
	if preblend {
		color.PreblendGray(bitmap)
	}
	atlasIndex, x, y := c.allocate(bitmap)

	g := Glyph{
		Rune:    r,
		Atlas:   atlasIndex,
		X:       x,
		Y:       y,
		Width:   bitmap.Bounds().Dx(),
		Height:  bitmap.Bounds().Dy(),
		OffsetX: float32(dr.Min.X - glyphBitmapPadding),
		OffsetY: float32(dr.Min.Y - glyphBitmapPadding),
		Advance: advancePx,
	}
	c.storeGlyph(key, g)
	return g
}

func (c *GlyphCache) storeGlyph(key phasedGlyphKey, glyph Glyph) {
	if !key.lcd && !key.preblend && key.x == 0 && key.y == 0 {
		c.glyphs[key.character] = glyph
		return
	}
	if c.phasedGlyphs == nil {
		c.phasedGlyphs = make(map[phasedGlyphKey]Glyph)
	}
	c.phasedGlyphs[key] = glyph
}

func (c *GlyphCache) GlyphLCDAt(r rune, phaseX uint8) Glyph {
	return c.GlyphLCDColorAt(r, phaseX, 0)
}

func (c *GlyphCache) GlyphLCDColorAt(r rune, phaseX uint8, color font.LCDColor) Glyph {
	color &= 511
	c.mu.Lock()
	defer c.mu.Unlock()
	key := phasedGlyphKey{character: r, x: phaseX & 3, lcd: true, color: color}
	if glyph, ok := c.phasedGlyphs[key]; ok {
		return glyph
	}
	if c.face == nil {
		return Glyph{Rune: r}
	}
	rect, mask, advance, ok := c.face.GlyphLCD(r, key.x)
	glyph := Glyph{Rune: r, Advance: fixedToFloat32(advance)}
	if ok && mask != nil && !rect.Empty() {
		color.PreblendLCD(mask)
		atlas, x, y := c.allocateFormat(mask, true)
		glyph.Atlas, glyph.X, glyph.Y = atlas, x, y
		glyph.Width, glyph.Height = mask.Rect.Dx(), mask.Rect.Dy()
		glyph.OffsetX, glyph.OffsetY = float32(rect.Min.X), float32(rect.Min.Y)
	}
	c.storeGlyph(key, glyph)
	return glyph
}

func (c *GlyphCache) OutlineGlyph(r rune, width float32) Glyph {
	if width <= 0 || math.IsNaN(float64(width)) || math.IsInf(float64(width), 0) {
		return c.Glyph(r)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := outlinedGlyphKey{rune: r, width: width}
	if g, ok := c.outlinedGlyphs[key]; ok {
		return g
	}
	if c.face == nil {
		g := Glyph{Rune: r}
		c.outlinedGlyphs[key] = g
		return g
	}

	dr, mask, maskp, advance, ok := c.face.Glyph(font.Point26_6{}, r)
	if !ok || dr.Empty() {
		g := Glyph{Rune: r, Advance: fixedToFloat32(advance)}
		c.outlinedGlyphs[key] = g
		return g
	}

	padding := glyphBitmapPadding + int(math.Ceil(float64(width)))
	bitmap := glyphBitmapWithPadding(mask, maskp, dr, padding)
	dilateGlyphBitmap(bitmap, width)
	atlasIndex, x, y := c.allocate(bitmap)

	g := Glyph{
		Rune:    r,
		Atlas:   atlasIndex,
		X:       x,
		Y:       y,
		Width:   bitmap.Bounds().Dx(),
		Height:  bitmap.Bounds().Dy(),
		OffsetX: float32(dr.Min.X - padding),
		OffsetY: float32(dr.Min.Y - padding),
		Advance: fixedToFloat32(advance),
	}
	c.outlinedGlyphs[key] = g
	return g
}

func (c *GlyphCache) FlushUploads() {
	if c == nil || c.pool == nil {
		return
	}
	c.pool.flushUploads()
}

func (c *GlyphCache) Dispose() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	pool, ownsPool := c.pool, c.ownsPool
	c.face = nil
	c.pool = nil
	c.ownsPool = false
	clear(c.glyphs)
	clear(c.phasedGlyphs)
	c.glyphs = make(map[rune]Glyph)
	clear(c.outlinedGlyphs)
	c.outlinedGlyphs = make(map[outlinedGlyphKey]Glyph)
	if ownsPool && pool != nil {
		return pool.dispose()
	}
	return nil
}

func (c *GlyphCache) allocate(bitmap image.Image) (int, int, int) {
	return c.allocateFormat(bitmap, false)
}

func (c *GlyphCache) allocateFormat(bitmap image.Image, lcd bool) (int, int, int) {
	if c.pool == nil {
		c.pool = new(glyphAtlasPool)
		c.ownsPool = true
	}
	return c.pool.allocateFormat(bitmap, lcd)
}

func (p *glyphAtlasPool) atlas(index int) (*graphics.Texture, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if index < 0 || index >= len(p.atlases) {
		return nil, false
	}
	return p.atlases[index].texture, true
}

func (p *glyphAtlasPool) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.atlases)
}

func (p *glyphAtlasPool) allocate(bitmap image.Image) (int, int, int) {
	return p.allocateFormat(bitmap, false)
}

func (p *glyphAtlasPool) allocateFormat(bitmap image.Image, lcd bool) (int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	width, height := bitmap.Bounds().Dx(), bitmap.Bounds().Dy()
	p.dirty = true
	for index, atlas := range p.atlases {
		if atlas.lcd != lcd {
			continue
		}
		if x, y, ok := atlas.allocate(width, height); ok {
			atlas.upload(x, y, bitmap)
			return index, x, y
		}
	}
	size := max(defaultGlyphAtlasSize, nextPowerOfTwo(max(width, height)+glyphAtlasPadding))
	var atlas *glyphAtlas
	if lcd {
		atlas = &glyphAtlas{texture: graphics.CreateEmptyTexture(size, size, render.TextureFormatRGBA8, graphics.ResourceUsageDefault), size: size, lcd: true}
	} else {
		atlas = newGlyphAtlas(size)
	}
	p.atlases = append(p.atlases, atlas)
	x, y, _ := atlas.allocate(width, height)
	atlas.upload(x, y, bitmap)
	return len(p.atlases) - 1, x, y
}

func (p *glyphAtlasPool) flushUploads() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.dirty {
		return
	}
	p.dirty = false
	for _, atlas := range p.atlases {
		atlas.flushUploads()
	}
}

func (p *glyphAtlasPool) dispose() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	errList := make([]error, 0, len(p.atlases))
	for _, atlas := range p.atlases {
		if atlas == nil || atlas.texture == nil {
			continue
		}
		if err := atlas.texture.Dispose(); err != nil {
			errList = append(errList, err)
		}
		clear(atlas.pending)
		atlas.pending = nil
	}
	p.atlases = nil
	return errors.Join(errList...)
}

func nextPowerOfTwo(value int) int {
	if value <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(value-1))
}

func glyphBitmap(mask image.Image, maskp image.Point, dr image.Rectangle) *image.Alpha {
	return glyphBitmapWithPadding(mask, maskp, dr, glyphBitmapPadding)
}

func glyphBitmapWithPadding(mask image.Image, maskp image.Point, dr image.Rectangle, padding int) *image.Alpha {
	width := dr.Dx() + padding*2
	height := dr.Dy() + padding*2
	bitmap := image.NewAlpha(image.Rect(0, 0, width, height))
	for y := 0; y < dr.Dy(); y++ {
		for x := 0; x < dr.Dx(); x++ {
			_, _, _, a := mask.At(maskp.X+x, maskp.Y+y).RGBA()
			alpha := uint8(a >> 8)
			offset := bitmap.PixOffset(x+padding, y+padding)
			bitmap.Pix[offset] = alpha
		}
	}
	return bitmap
}

func dilateGlyphBitmap(bitmap image.Image, width float32) {
	if bitmap == nil || width <= 0 || math.IsNaN(float64(width)) {
		return
	}
	bounds := bitmap.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return
	}
	var readAlpha func(x, y int) uint8
	var writeAlpha func(x, y int, value uint8)
	switch value := bitmap.(type) {
	case *image.Alpha:
		readAlpha = func(x, y int) uint8 { return value.Pix[value.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)] }
		writeAlpha = func(x, y int, alpha uint8) { value.Pix[value.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)] = alpha }
	case *image.Gray:
		readAlpha = func(x, y int) uint8 { return value.Pix[value.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)] }
		writeAlpha = func(x, y int, alpha uint8) { value.Pix[value.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)] = alpha }
	case *image.RGBA:
		readAlpha = func(x, y int) uint8 { return value.Pix[value.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)+3] }
		writeAlpha = func(x, y int, alpha uint8) { value.Pix[value.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)+3] = alpha }
	default:
		return
	}
	var localSource [64 * 64]uint8
	source := localSource[:0]
	if size := w * h; size <= cap(source) {
		source = localSource[:size]
	} else {
		source = make([]uint8, size)
	}
	for y := range h {
		for x := range w {
			source[y*w+x] = readAlpha(x, y)
		}
	}
	radius := max(w, h)
	if requested := math.Ceil(float64(width + 1)); requested < float64(radius) {
		radius = int(requested)
	}
	var localKernel [9 * 9]glyphDilationSample
	kernel := localKernel[:0]
	if size := (radius*2 + 1) * (radius*2 + 1); size > cap(kernel) {
		kernel = make([]glyphDilationSample, 0, size)
	}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			coverage := min(max(width+1-float32(math.Hypot(float64(dx), float64(dy))), 0), 1)
			if coverage > 0 {
				kernel = append(kernel, glyphDilationSample{x: dx, y: dy, coverage: coverage})
			}
		}
	}
	for y := range h {
		for x := range w {
			alpha := source[y*w+x]
			if alpha == 0 {
				continue
			}
			for _, sample := range kernel {
				targetY := y + sample.y
				if targetY < 0 || targetY >= h {
					continue
				}
				targetX := x + sample.x
				if targetX < 0 || targetX >= w {
					continue
				}
				value := uint8(float32(alpha) * sample.coverage)
				writeAlpha(targetX, targetY, max(readAlpha(targetX, targetY), value))
			}
		}
	}
}

type glyphDilationSample struct {
	x, y     int
	coverage float32
}

func fixedToFloat32(v font.Int26_6) float32 {
	return float32(v) / 64
}
