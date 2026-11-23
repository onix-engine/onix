package project

import (
	"io/fs"
	"net/url"
	"path"
	"strings"

	"github.com/onix-engine/onix/graphics"
)

type modHTMLImages struct {
	store *Resources
	base  string
}

func (p modHTMLImages) ResolveHTMLImage(source string, width, height float32) (*graphics.Texture, bool) {
	file, ok := htmlImagePath(p.base, source)
	if !ok {
		return nil, false
	}
	texture, err := p.store.texture(&TextureSpec{Image: file})
	return texture, err == nil && texture != nil
}

// Packaged images use relative URL paths. Queries and fragments are not part
// of the filename. Split them before decoding so escaped delimiters stay in it.
func htmlImagePath(base, source string) (string, bool) {
	reference, fragment, _ := strings.Cut(source, "#")
	for i := range len(reference) {
		if reference[i] < 0x20 || reference[i] == 0x7f {
			return "", false
		}
	}
	rawPath, _, _ := strings.Cut(reference, "?")
	name, err := url.PathUnescape(rawPath)
	if err != nil || name == "" || name[0] == '/' || strings.ContainsAny(name, `\:`) {
		return "", false
	}
	if strings.Contains(fragment, "%") {
		if _, err := url.PathUnescape(fragment); err != nil {
			return "", false
		}
	}
	file := path.Join(base, name)
	return file, file != "." && fs.ValidPath(file)
}
