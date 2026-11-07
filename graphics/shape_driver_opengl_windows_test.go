//go:build windows && cgo && onix_opengl

package graphics

import "github.com/onix-engine/onix/render"

func newShapeCoverageDriver(surface render.NativeSurface) (render.Driver, error) {
	return render.NewOpenGLDriver(surface)
}
