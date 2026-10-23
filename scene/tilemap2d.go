package scene

import "github.com/diiyw/onix/graphics"

type Tilemap2D struct {
	Texture    *graphics.Texture
	Tiles      []int
	Revision   uint64
	Width      int
	Height     int
	TileWidth  int
	TileHeight int
	Columns    int
	Alpha      float32
}

func (t *Tilemap2D) PixelWidth() float32 {
	if t == nil {
		return 0
	}
	return float32(t.Width * t.TileWidth)
}

func (t *Tilemap2D) PixelHeight() float32 {
	if t == nil {
		return 0
	}
	return float32(t.Height * t.TileHeight)
}

func (t *Tilemap2D) Valid() bool {
	return t != nil && t.Texture != nil && t.Texture.Valid() && t.Width > 0 && t.Height > 0 &&
		t.TileWidth > 0 && t.TileHeight > 0 && t.Columns > 0 && len(t.Tiles) >= t.Width*t.Height
}
