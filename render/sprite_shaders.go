//go:build (cgo && onix_opengl && !ios && (windows || linux || darwin || android)) || (js && wasm && (wechat || onix_webgl) && !onix_directx && !onix_metal && !onix_vulkan && !onix_opengl && !onix_webgpu)

package render

import _ "embed"

//go:embed shaders/gles_sprite_multi.vert
var spriteMultiVertexGLSL string

//go:embed shaders/gles_sprite_multi.frag
var spriteMultiFragmentGLSL string
