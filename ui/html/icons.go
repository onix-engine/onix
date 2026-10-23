package html

import (
	_ "embed"
	"sync"

	"github.com/diiyw/onix/graphics"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
	"github.com/diiyw/onix/images/svg"
)

// htmlSelectIconData is the lucide:chevron-down icon supplied by better-icons.
// It is kept as an engine resource so HTML controls do not depend on a project
// asset directory.
//
//go:embed resources/icons/chevron-down.svg
var htmlSelectIconData []byte

var htmlSelectIcon struct {
	sync.Once
	texture *graphics.Texture
}

func defaultHTMLSelectIcon() *graphics.Texture {
	htmlSelectIcon.Do(func() {
		texture, err := graphicssvg.Decode(htmlSelectIconData, svg.Options{}, graphics.TextureOptions{Usage: graphics.ResourceUsageDefault})
		if err != nil {
			panic(err)
		}
		htmlSelectIcon.texture = texture
	})
	return htmlSelectIcon.texture
}
