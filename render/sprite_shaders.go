package render

import _ "embed"

//go:embed shaders/gles_sprite_multi.vert
var spriteMultiVertexGLSL string

//go:embed shaders/gles_sprite_multi.frag
var spriteMultiFragmentGLSL string

//go:embed shaders/sprite_multi.wgsl
var spriteMultiWGSL string
