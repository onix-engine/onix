package editor

import (
	"testing"

	"github.com/diiyw/onix/graphics"
)

func TestEditorImageDecoderLoadsSVG(t *testing.T) {
	texture, err := (editorImageDecoder{}).DecodeImage([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><path d="M0 0h16v16H0z" fill="red"/></svg>`), "resources/icon.svg", graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatalf("decode SVG: %v", err)
	}
	defer texture.Dispose()
	if texture.Width != 16 || texture.Height != 16 || !texture.Valid() {
		t.Fatalf("SVG texture = %#v", texture)
	}
}
