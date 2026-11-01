//go:build onix_opengl && (windows || linux || android || (darwin && !ios)) && !onix_directx && !onix_metal && !onix_vulkan && !onix_webgpu && !onix_webgl

package render

const CompiledBackend Backend = BackendOpenGL
