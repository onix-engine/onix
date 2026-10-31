package html

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
)

type htmlTextureKey struct {
	gradient htmlGradientKey
	node     *dom.Node
	variant  string
}

type htmlOwnedTexture struct {
	key     htmlTextureKey
	texture *graphics.Texture
	target  *render.RenderTarget
	refs    int
	queued  bool
}

type htmlTextureCache struct {
	face, boldFace *font.Face
	entries        map[htmlTextureKey]*htmlOwnedTexture
	textures       map[*graphics.Texture]*htmlOwnedTexture
	entities       map[ecs.Entity]*htmlOwnedTexture
	unused         []*htmlOwnedTexture
	rasters        map[*dom.Node]*htmlOpacityRaster
	paint          map[htmlTextureKey]*htmlPaintRaster
	gpuBlur        map[htmlTextureKey]*htmlGPUBlur
	blurShader     *graphics.Material2DShader
	blurScratch    htmlGPUBlurScratch
}

func (cache *htmlTextureCache) add(key htmlTextureKey, texture *graphics.Texture) *htmlOwnedTexture {
	if cache.entries == nil {
		cache.entries = make(map[htmlTextureKey]*htmlOwnedTexture)
		cache.textures = make(map[*graphics.Texture]*htmlOwnedTexture)
		cache.entities = make(map[ecs.Entity]*htmlOwnedTexture)
	}
	entry := &htmlOwnedTexture{key: key, texture: texture, queued: true}
	cache.entries[key], cache.textures[texture] = entry, entry
	cache.unused = append(cache.unused, entry)
	return entry
}

func (cache *htmlTextureCache) bind(entity ecs.Entity, texture *graphics.Texture) {
	previous, next := cache.entities[entity], cache.textures[texture]
	if previous == next {
		return
	}
	if previous != nil {
		previous.refs--
		if previous.refs == 0 && !previous.queued {
			previous.queued = true
			cache.unused = append(cache.unused, previous)
		}
		delete(cache.entities, entity)
	}
	if next != nil {
		next.refs++
		cache.entities[entity] = next
	}
}

func (cache *htmlTextureCache) sweep() {
	retained := cache.unused[:0]
	for _, entry := range cache.unused {
		entry.queued = false
		if entry.refs == 0 {
			if effect := cache.gpuBlur[entry.key]; effect != nil && effect.source == entry && effect.output.refs > 0 {
				entry.queued = true
				retained = append(retained, entry)
				continue
			}
			_ = entry.texture.Dispose()
			if entry.target != nil {
				_ = entry.target.Dispose()
			}
			if variant, ok := strings.CutPrefix(entry.key.variant, "gpu-"); ok {
				key := htmlTextureKey{node: entry.key.node, variant: variant}
				if effect := cache.gpuBlur[key]; effect != nil && effect.output == entry {
					delete(cache.gpuBlur, key)
				}
			}
			if cache.entries[entry.key] == entry {
				delete(cache.entries, entry.key)
			}
			delete(cache.textures, entry.texture)
			if raster := cache.paint[entry.key]; raster != nil && raster.texture == entry.texture {
				delete(cache.paint, entry.key)
			}
		}
	}
	clear(cache.unused[len(retained):])
	cache.unused = retained
	if len(cache.gpuBlur) == 0 {
		cache.blurScratch.dispose()
	}
}

func (cache *htmlTextureCache) dispose() {
	for _, entry := range cache.textures {
		_ = entry.texture.Dispose()
		if entry.target != nil {
			_ = entry.target.Dispose()
		}
	}
	cache.blurScratch.dispose()
	if cache.blurShader != nil {
		_ = cache.blurShader.Dispose()
		cache.blurShader = nil
	}
	clear(cache.entries)
	clear(cache.textures)
	clear(cache.entities)
	clear(cache.unused)
	cache.unused = cache.unused[:0]
	clear(cache.rasters)
	clear(cache.paint)
	clear(cache.gpuBlur)
}

type htmlPaintRaster struct {
	colors                      [5]style.Color
	image                       image.RGBA
	style                       *style.ComputedStyle
	width                       int
	height                      int
	texture                     *graphics.Texture
	gradients                   []htmlRasterGradient
	logicalWidth, logicalHeight float32
	version                     uint64
	shadow                      style.Shadow
	radii                       style.Corners
	padding                     float32
}

func cleanupHTMLTextures(_ *ecs.World, _ ecs.Entity, state *htmlRenderState) {
	state.Textures.dispose()
}

func cleanupHTMLSurface(world *ecs.World, entity ecs.Entity, _ *Surface) {
	if state := world.GetRef[htmlRenderState](entity); state != nil {
		state.Textures.dispose()
		state.Root = nil
	}
}

// Dispose releases renderer-owned textures while the device is still alive.

// Release removes retained children before their owner is despawned. Call
// outside ECS queries and removal hooks, which forbid structural changes.
func Release(world *ecs.World, owner ecs.Entity) {
	if world == nil {
		return
	}
	if owners, ok := world.GetResource[*htmlSurfaceOwners](); ok {
		for _, child := range owners.Children[owner] {
			world.Despawn(child)
		}
		delete(owners.Children, owner)
	}
	world.Remove[htmlRenderState](owner)
}

func Dispose(world *ecs.World) {
	for _, access := range world.Query[htmlRenderState]().All {
		state := access.Ref()

		state.Textures.dispose()
	}
}

type htmlOpacityChild struct {
	node    *dom.Node
	version uint64
}

type htmlOpacityInline struct {
	node       *dom.Node
	style      *style.ComputedStyle
	background style.Value
	radii      style.Corners
	rect       layout.Rect
}

type htmlOpacityRaster struct {
	image               image.RGBA
	rect                layout.Rect
	boxRect             layout.Rect
	background          style.Value
	opacity             style.Value
	radius              float32
	radii               style.Corners
	children            []htmlOpacityChild
	inline              []htmlOpacityInline
	version             uint64
	uploaded            uint64
	style               *style.ComputedStyle
	geometry, textPaint uint64
	face, boldFace      *font.Face
	gradients           []htmlRasterGradient
	cached              bool
	gpuBlur             bool
}

func htmlOpacityRasterFor(cache *htmlTextureCache, box *layout.Box) *htmlOpacityRaster {
	return updateHTMLOpacityRaster(cache, box, true, false, false)
}

func updateHTMLOpacityRaster(cache *htmlTextureCache, box *layout.Box, root, cacheSubtree, gpuBlur bool) *htmlOpacityRaster {
	if cache.rasters == nil {
		cache.rasters = make(map[*dom.Node]*htmlOpacityRaster)
	}
	raster := cache.rasters[box.Node]
	if raster == nil {
		raster = &htmlOpacityRaster{}
		cache.rasters[box.Node] = raster
	}
	r := box.BorderBox()
	bounds := htmlOwnVisiblePaintBounds(box)
	background, opacity := box.Style.BackgroundColor, box.Style.Opacity
	if root || box.Style.AnimatedOpacity {
		opacity = style.Number(1)
	}
	group := root || len(box.Style.Filters) != 0 || (opacity.Kind == style.ValueNumber && opacity.Number < 1)
	// Keep one layer of subtree caches under an effect group so changing its
	// own paint does not rerasterize static descendants. Deeper ordinary
	// wrappers need only invalidation metadata, not another full image each.
	cached := group || cacheSubtree
	radius := htmlOverflowClipRadius(box.Style, r.Width, r.Height)
	geometry, textPaint := box.OwnGeometryHash(), htmlTextRasterHash(box)
	styleChanged := raster.style != box.Style && (!root || !style.EqualExceptOpacity(raster.style, box.Style))
	dirty := raster.version == 0 || raster.cached != cached || raster.gpuBlur != gpuBlur || styleChanged || raster.background != background || raster.opacity != opacity || raster.radii != box.Style.BorderRadius || raster.geometry != geometry || raster.textPaint != textPaint || raster.face != cache.face || raster.boldFace != cache.boldFace
	for index, background := range box.InlineBackgrounds {
		dirty = reuse.UpdateAt(&raster.inline, index, htmlOpacityInline{background.Node, background.Style, background.Style.BackgroundColor, background.Style.BorderRadius, background.Rect}) || dirty
	}
	dirty = len(raster.inline) != len(box.InlineBackgrounds) || dirty
	clear(raster.inline[len(box.InlineBackgrounds):])
	raster.inline = raster.inline[:len(box.InlineBackgrounds)]
	for index, child := range box.Children {
		value := updateHTMLOpacityRaster(cache, child, false, group, false)
		bounds = htmlUnionPaintBounds(bounds, value.rect)
		dirty = reuse.UpdateAt(&raster.children, index, htmlOpacityChild{child.Node, value.version}) || dirty
	}
	dirty = len(raster.children) != len(box.Children) || dirty
	clear(raster.children[len(box.Children):])
	raster.children = raster.children[:len(box.Children)]
	minX, minY, maxX, maxY := bounds.X, bounds.Y, bounds.X+bounds.Width, bounds.Y+bounds.Height
	if bounds.Width > 0 && bounds.Height > 0 {
		minX, minY, maxX, maxY = htmlFilterBounds(box.Style.Filters, minX, minY, maxX, maxY)
		bounds = layout.Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
	}
	// The box rect can change within an unchanged subtree bounding rectangle.
	dirty = dirty || raster.rect != bounds || raster.boxRect != r
	if !dirty {
		return raster
	}
	raster.rect, raster.background, raster.opacity, raster.radius, raster.radii = bounds, background, opacity, radius, box.Style.BorderRadius
	raster.boxRect = r
	raster.style, raster.geometry, raster.textPaint, raster.face, raster.boldFace = box.Style, geometry, textPaint, cache.face, cache.boldFace
	raster.cached = cached
	raster.gpuBlur = gpuBlur
	raster.version++
	if !cached || bounds.Width <= 0 || bounds.Height <= 0 {
		raster.image = image.RGBA{}
		raster.uploaded = 0
		return raster
	}
	w, h := max(1, int(math.Ceil(bounds.Width))), max(1, int(math.Ceil(bounds.Height)))
	length := w * h * 4
	if entry := cache.entries[htmlTextureKey{node: box.Node}]; entry != nil && (entry.texture.Width != w || entry.texture.Height != h) {
		raster.image.Pix = nil
	}
	if cap(raster.image.Pix) < length {
		raster.image.Pix = make([]byte, length)
	} else {
		raster.image.Pix = raster.image.Pix[:length]
		clear(raster.image.Pix)
	}
	raster.image.Stride, raster.image.Rect = w*4, image.Rect(0, 0, w, h)
	// Nested effect groups have finished painting before borrowing scratch.
	scratch := acquireHTMLRasterScratch()
	defer releaseHTMLRasterScratch(scratch)
	paintHTMLOpacityContents(cache, box, &raster.image, minX, minY, scratch)
	if !gpuBlur {
		applyHTMLFilters(&raster.image, box.Style.Filters, &scratch.htmlFilterScratch)
	}
	alpha := float64(1)
	if opacity.Kind == style.ValueNumber {
		alpha = opacity.Number
	}
	if alpha < 1 {
		for index := 3; index < len(raster.image.Pix); index += 4 {
			raster.image.Pix[index] = uint8(math.Round(float64(raster.image.Pix[index]) * alpha))
		}
	}
	return raster
}

func paintHTMLOpacityContents(cache *htmlTextureCache, box *layout.Box, dst *image.RGBA, originX, originY float64, scratch *htmlRasterScratch) {
	raster := cache.rasters[box.Node]
	r := box.BorderBox()
	if htmlStyleVisible(box.Style) {
		raster.gradients = prepareHTMLRasterGradients(raster.gradients, box.Style)
		rasterHTMLBoxPaintPrepared(&scratch.paint, box.Style, r.Width, r.Height, 1, raster.gradients)
		htmlCompositeRGBA(dst, &scratch.paint, int(math.Round(r.X-originX)), int(math.Round(r.Y-originY)))
	}
	for _, background := range box.InlineBackgrounds {
		if !htmlStyleVisible(background.Style) {
			continue
		}
		r := background.Rect
		raster.gradients = prepareHTMLRasterGradients(raster.gradients, background.Style)
		rasterHTMLBoxPaintPrepared(&scratch.paint, background.Style, r.Width, r.Height, 1, raster.gradients)
		htmlCompositeRGBA(dst, &scratch.paint, int(math.Round(r.X-originX)), int(math.Round(r.Y-originY)))
	}
	rasterHTMLText(dst, box, cache.face, cache.boldFace, originX, originY)
	for _, child := range box.Children {
		value := cache.rasters[child.Node]
		if value.cached {
			htmlCompositeRGBA(dst, &value.image, int(math.Round(value.rect.X-originX)), int(math.Round(value.rect.Y-originY)))
		} else {
			paintHTMLOpacityContents(cache, child, dst, originX, originY, scratch)
		}
	}
}

func rasterHTMLBackground(dst *image.RGBA, box *layout.Box, originX, originY float64) {
	r := box.BorderBox()
	value := box.Style.BackgroundColor
	if value.Kind != style.ValueColor || value.Color.A == 0 {
		return
	}
	for y := 0; y < int(math.Ceil(r.Height)); y++ {
		for x := 0; x < int(math.Ceil(r.Width)); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if htmlRoundedPixel(box.Style, px, py, r.Width, r.Height) {
				dst.SetRGBA(int(math.Floor(r.X-originX))+x, int(math.Floor(r.Y-originY))+y, color.RGBA(value.Color))
			}
		}
	}
}
