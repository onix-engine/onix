package project

import (
	"io/fs"
	"net/url"
	"path"
	"strings"

	"github.com/diiyw/onix/graphics"
)

type modHTMLImages struct {
	store *Resources
	base  string
}

func (p modHTMLImages) ResolveHTMLImage(source string) (*graphics.Texture, bool) {
	location, err := url.Parse(source)
	if err != nil || location.Scheme != "" || location.Host != "" || location.Opaque != "" {
		return nil, false
	}
	source = location.Path
	if source == "" || strings.HasPrefix(source, "/") || strings.ContainsAny(source, `\:`) {
		return nil, false
	}
	file := path.Join(p.base, source)
	if !fs.ValidPath(file) || file == "." {
		return nil, false
	}
	texture, err := p.store.texture(&TextureSpec{Image: file})
	return texture, err == nil && texture != nil
}
