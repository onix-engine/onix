package project

import "github.com/diiyw/onix/graphics"

// ModMaterial2D keeps material uniforms per instance.
type ModMaterial2D struct {
	Custom [4][4]float32
}

func (m ModMaterial2D) Material2D() graphics.Material2DData {
	return graphics.Material2DData{Custom: m.Custom}
}
