package html

import (
	"image"
	"image/color"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/internal/reuse"
)

type htmlTextureKey struct {
	gradient htmlGradientKey
	node     *dom.Node
	variant  string
}

type htmlOwnedTexture struct {
	key     htmlTextureKey
	texture *graphics.Texture
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
	for _, entry := range cache.unused {
		entry.queued = false
		if entry.refs == 0 {
			_ = entry.texture.Dispose()
			if cache.entries[entry.key] == entry {
				delete(cache.entries, entry.key)
			}
			delete(cache.textures, entry.texture)
			if raster := cache.paint[entry.key]; raster != nil && raster.texture == entry.texture {
				delete(cache.paint, entry.key)
			}
		}
	}
	clear(cache.unused)
	cache.unused = cache.unused[:0]
}

func (cache *htmlTextureCache) dispose() {
	for _, entry := range cache.textures {
		_ = entry.texture.Dispose()
	}
	clear(cache.entries)
	clear(cache.textures)
	clear(cache.entities)
	clear(cache.unused)
	cache.unused = cache.unused[:0]
	clear(cache.rasters)
	clear(cache.paint)
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
	filterScratch               htmlFilterScratch
}

func cleanupHTMLTextures(_ *ecs.World, _ ecs.Entity, state *htmlRenderState) {
	state.Textures.dispose()
}

func cleanupHTMLSurface(world *ecs.World, entity ecs.Entity, _ *Surface) {
	if state := ecs.GetRef[htmlRenderState](world, entity); state != nil {
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
	if owners, ok := ecs.GetResource[*htmlSurfaceOwners](world); ok {
		for _, child := range owners.Children[owner] {
			world.Despawn(child)
		}
		delete(owners.Children, owner)
	}
	ecs.Remove[htmlRenderState](world, owner)
}

func Dispose(world *ecs.World) {
	for _, access := range ecs.Query[htmlRenderState](world).All {
		state := access.Ref()

		state.Textures.dispose()
	}
}

type htmlOpacityChild struct {
	node    *dom.Node
	version uint64
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
	version             uint64
	uploaded            uint64
	style               *style.ComputedStyle
	geometry, textPaint uint64
	face, boldFace      *font.Face
	paint               htmlPaintRaster
	filterScratch       htmlFilterScratch
}

func htmlOpacityRasterFor(cache *htmlTextureCache, box *layout.Box) *htmlOpacityRaster {
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
	if box.Style.AnimatedOpacity {
		opacity = style.Number(1)
	}
	radius := htmlOverflowClipRadius(box.Style, r.Width, r.Height)
	geometry, textPaint := box.OwnGeometryHash(), htmlTextRasterHash(box)
	dirty := raster.version == 0 || raster.style != box.Style || raster.background != background || raster.opacity != opacity || raster.radii != box.Style.BorderRadius || raster.geometry != geometry || raster.textPaint != textPaint || raster.face != cache.face || raster.boldFace != cache.boldFace
	for index, child := range box.Children {
		value := htmlOpacityRasterFor(cache, child)
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
	if bounds.Width <= 0 || bounds.Height <= 0 {
		raster.image.Pix = raster.image.Pix[:0]
		raster.image.Rect, raster.image.Stride = image.Rectangle{}, 0
		raster.rect, raster.style, raster.geometry, raster.textPaint = bounds, box.Style, geometry, textPaint
		raster.boxRect, raster.background, raster.opacity, raster.radii = r, background, opacity, box.Style.BorderRadius
		raster.face, raster.boldFace = cache.face, cache.boldFace
		raster.version++
		return raster
	}
	w, h := max(1, int(math.Ceil(bounds.Width))), max(1, int(math.Ceil(bounds.Height)))
	length := w * h * 4
	if cap(raster.image.Pix) < length {
		raster.image.Pix = make([]byte, length)
	} else {
		raster.image.Pix = raster.image.Pix[:length]
		clear(raster.image.Pix)
	}
	raster.image.Stride, raster.image.Rect = w*4, image.Rect(0, 0, w, h)
	raster.rect, raster.background, raster.opacity, raster.radius, raster.radii = bounds, background, opacity, radius, box.Style.BorderRadius
	raster.boxRect = r
	raster.style, raster.geometry, raster.textPaint, raster.face, raster.boldFace = box.Style, geometry, textPaint, cache.face, cache.boldFace
	if htmlStyleVisible(box.Style) {
		raster.paint.gradients = prepareHTMLRasterGradients(raster.paint.gradients, box.Style)
		rasterHTMLBoxPaintPrepared(&raster.paint.image, box.Style, r.Width, r.Height, 1, raster.paint.gradients)
		htmlCompositeRGBA(&raster.image, &raster.paint.image, int(math.Round(r.X-minX)), int(math.Round(r.Y-minY)))
	}
	rasterHTMLText(&raster.image, box, cache.face, cache.boldFace, minX, minY)
	for _, child := range box.Children {
		value := cache.rasters[child.Node]
		htmlCompositeRGBA(&raster.image, &value.image, int(math.Round(value.rect.X-minX)), int(math.Round(value.rect.Y-minY)))
	}
	applyHTMLFilters(&raster.image, box.Style.Filters, &raster.filterScratch)
	alpha := float64(1)
	if opacity.Kind == style.ValueNumber {
		alpha = opacity.Number
	}
	if alpha < 1 {
		for index := 3; index < len(raster.image.Pix); index += 4 {
			raster.image.Pix[index] = uint8(math.Round(float64(raster.image.Pix[index]) * alpha))
		}
	}
	raster.version++
	return raster
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
