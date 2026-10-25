//go:build (windows || linux || android) && cgo && (!(linux && onix_drm) || onix_drm_vulkan)

package render

/*
#cgo windows LDFLAGS: -static -ld3d11 -ldxgi -lole32 -luuid
#cgo linux,!android LDFLAGS: -l:libvulkan.so.1
#cgo linux,onix_drm CFLAGS: -DONIX_VK_DRM
#cgo android LDFLAGS: -lvulkan -landroid
// These entry points consume or copy their Go arguments before returning.
#cgo noescape onix_vk_begin_frame
#cgo nocallback onix_vk_begin_frame
#cgo noescape onix_vk_begin_render_target
#cgo nocallback onix_vk_begin_render_target
#cgo noescape onix_vk_create_render_target
#cgo nocallback onix_vk_create_render_target
#cgo noescape onix_vk_resize_render_target
#cgo nocallback onix_vk_resize_render_target
#cgo noescape onix_vk_draw_sprites
#cgo nocallback onix_vk_draw_sprites
#cgo noescape onix_vk_draw_shapes
#cgo nocallback onix_vk_draw_shapes
#cgo noescape onix_vk_draw_meshes
#cgo nocallback onix_vk_draw_meshes
#cgo noescape onix_vk_draw_mesh_picking
#cgo nocallback onix_vk_draw_mesh_picking
#cgo noescape onix_vk_update_textures
#cgo nocallback onix_vk_update_textures
#cgo noescape onix_vk_prepare_texture_updates
#cgo nocallback onix_vk_prepare_texture_updates
#cgo nocallback onix_vk_commit_texture_updates
#cgo noescape onix_vk_buffer_memory
#cgo nocallback onix_vk_buffer_memory
#include "vulkan_native.h"
*/
import "C"

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

//go:embed shaders/vulkan_sprite.vert.spv
var vulkanSpriteVertex []byte

//go:embed shaders/vulkan_sprite.frag.spv
var vulkanSpriteFragment []byte

//go:embed shaders/vulkan_sprite_multi.vert.spv
var vulkanSpriteMultiVertex []byte

//go:embed shaders/vulkan_sprite_multi.frag.spv
var vulkanSpriteMultiFragment []byte

//go:embed shaders/vulkan_shape.vert.spv
var vulkanShapeVertex []byte

//go:embed shaders/vulkan_shape.frag.spv
var vulkanShapeFragment []byte

//go:embed shaders/vulkan_mesh.vert.spv
var vulkanMeshVertex []byte

//go:embed shaders/vulkan_mesh.frag.spv
var vulkanMeshFragment []byte

//go:embed shaders/vulkan_mesh_unlit.frag.spv
var vulkanMeshUnlitFragment []byte

//go:embed shaders/vulkan_mesh_depth.frag.spv
var vulkanMeshDepthFragment []byte

//go:embed shaders/vulkan_mesh_basic.frag.spv
var vulkanMeshBasicFragment []byte

//go:embed shaders/vulkan_picking.vert.spv
var vulkanPickingVertex []byte

//go:embed shaders/vulkan_picking.frag.spv
var vulkanPickingFragment []byte

type VulkanSurfaceKind int

const (
	VulkanSurfaceXlib    VulkanSurfaceKind = C.ONIX_VK_SURFACE_XLIB
	VulkanSurfaceWayland VulkanSurfaceKind = C.ONIX_VK_SURFACE_WAYLAND
	VulkanSurfaceAndroid VulkanSurfaceKind = C.ONIX_VK_SURFACE_ANDROID
	VulkanSurfaceWin32   VulkanSurfaceKind = C.ONIX_VK_SURFACE_WIN32
	VulkanSurfaceDRM     VulkanSurfaceKind = C.ONIX_VK_SURFACE_DRM
)

type VulkanSurface struct {
	Kind           VulkanSurfaceKind
	Display        unsafe.Pointer
	Window         uintptr
	Width          int
	Height         int
	Transparent    bool
	DRMFD          int
	DRMConnectorID uint32
	DRMRefreshRate uint32 // Millihertz; zero selects the highest rate at Width x Height.
}

type vulkanDriver struct {
	driverStatistics
	native     *C.OnixVkDriver
	deviceName string
	nativeDrawState
	spritePush     C.OnixVkSpritePush
	shapePush      C.OnixVkSpritePush
	lighting3D     [meshLightUniformFloatCount]float32
	updates        []C.OnixVkTextureUpdate
	width          int
	height         int
	viewport       Rect
	viewportSet    bool
	blend          BlendState
	blendSet       bool
	scissor        Rect
	scissorEnabled bool
	scissorSet     bool

	updateRetention scratchRetention
}

func NewVulkanDriver(surface VulkanSurface) (Driver, error) {
	if surface.Kind == 0 || surface.Width <= 0 || surface.Height <= 0 {
		return nil, errors.New("render: invalid Vulkan surface")
	}
	if surface.Kind == VulkanSurfaceDRM {
		if surface.DRMFD < 0 || surface.DRMConnectorID == 0 || surface.Transparent {
			return nil, errors.New("render: invalid Vulkan DRM surface")
		}
	} else if surface.Window == 0 {
		return nil, errors.New("render: invalid Vulkan window")
	}
	native := C.onix_vk_create(
		C.OnixVkSurface{
			kind:             C.int(surface.Kind),
			display:          surface.Display,
			window:           C.uintptr_t(surface.Window),
			width:            C.uint32_t(surface.Width),
			height:           C.uint32_t(surface.Height),
			transparent:      C.int(boolInt(surface.Transparent)),
			drm_fd:           C.int32_t(surface.DRMFD),
			drm_connector_id: C.uint32_t(surface.DRMConnectorID),
			drm_refresh_rate: C.uint32_t(surface.DRMRefreshRate),
		},
		vulkanShader(vulkanSpriteMultiVertex),
		vulkanShader(vulkanSpriteMultiFragment),
		vulkanShader(vulkanShapeVertex),
		vulkanShader(vulkanShapeFragment),
		vulkanShader(vulkanMeshVertex),
		vulkanShader(vulkanMeshFragment),
		vulkanShader(vulkanMeshUnlitFragment),
		vulkanShader(vulkanMeshDepthFragment),
		vulkanShader(vulkanMeshBasicFragment),
		vulkanShader(vulkanPickingVertex),
		vulkanShader(vulkanPickingFragment),
	)
	if native == nil {
		return nil, vulkanError("create driver")
	}
	deviceName := C.GoString(C.onix_vk_device_name(native))
	loadVulkanPipelineCache(native)
	logSelectedVulkanDevice(deviceName)
	return &vulkanDriver{
		native:      native,
		deviceName:  deviceName,
		width:       surface.Width,
		height:      surface.Height,
		viewport:    Rect{Width: int32(surface.Width), Height: int32(surface.Height)},
		viewportSet: true,
	}, nil
}

func (d *vulkanDriver) BufferMemoryStatistics() BufferMemoryStatistics {
	if d.native == nil {
		return BufferMemoryStatistics{}
	}
	var memory C.OnixBufferMemory
	C.onix_vk_buffer_memory(d.native, &memory)
	return BufferMemoryStatistics{
		InstanceBytes:      uint64(memory.instances),
		StagingBytes:       uint64(memory.staging),
		ReadbackBytes:      uint64(memory.readback),
		NativeScratchBytes: uint64(memory.scratch),
		ScratchBytes: sliceCapacityBytes(d.draws) + sliceCapacityBytes(d.draws3D) +
			sliceCapacityBytes(d.deformationWords) + sliceCapacityBytes(d.updates),
	}
}

func (d *vulkanDriver) RendererInfo() RendererInfo {
	return RendererInfo{Backend: "Vulkan", Device: d.deviceName}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (d *vulkanDriver) BeginFrame(clear Color) error {
	values := [4]C.float{C.float(clear.R), C.float(clear.G), C.float(clear.B), C.float(clear.A)}
	if C.onix_vk_begin_frame(d.native, &values[0]) == 0 {
		return vulkanError("begin frame")
	}
	d.updates = trimScratch(d.updates, &d.updateRetention, 32)
	d.trimScratch()
	d.beginFrameStatistics(d.width, d.height)
	return nil
}

func (d *vulkanDriver) SetViewport(viewport Rect) {
	if d.viewportSet && d.viewport == viewport {
		return
	}
	d.viewport = viewport
	d.viewportSet = true
	C.onix_vk_set_viewport(
		d.native,
		C.int32_t(viewport.X),
		C.int32_t(viewport.Y),
		C.int32_t(viewport.Width),
		C.int32_t(viewport.Height),
	)
}

func (d *vulkanDriver) Viewport() Rect { return d.viewport }

func (d *vulkanDriver) SetBlend(blend BlendState) error {
	mode, ok := spriteBlendMode(blend)
	if !ok {
		return fmt.Errorf("render: Vulkan blend state is not compiled: %+v", blend)
	}
	if d.blendSet && d.blend == blend {
		return nil
	}
	d.blend = blend
	d.blendSet = true
	C.onix_vk_set_sprite_blend(d.native, C.int(mode), C.float(blend.Constant.R), C.float(blend.Constant.G), C.float(blend.Constant.B), C.float(blend.Constant.A))
	return nil
}

func (d *vulkanDriver) SetScissor(scissor *Rect) {
	if scissor == nil {
		if d.scissorSet && !d.scissorEnabled {
			return
		}
		d.scissorSet = true
		d.scissorEnabled = false
		C.onix_vk_set_scissor(d.native, 0, 0, 0, 0, 0)
		return
	}
	if d.scissorSet && d.scissorEnabled && d.scissor == *scissor {
		return
	}
	d.scissor = *scissor
	d.scissorSet = true
	d.scissorEnabled = true
	C.onix_vk_set_scissor(
		d.native,
		1,
		C.int32_t(scissor.X),
		C.int32_t(scissor.Y),
		C.int32_t(scissor.Width),
		C.int32_t(scissor.Height),
	)
}

func (d *vulkanDriver) CreateTexture(descriptor TextureDescriptor, pixels []byte) (TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return TextureHandle{}, err
	}
	// A compressed payload already carries its chain because the device cannot
	// generate mips for block data; an uncompressed payload carries the base
	// level and the driver blits the rest of the chain from it.
	levels := 1
	if descriptor.Format.Compressed() {
		levels = descriptor.MipLevels()
	} else if descriptor.Filter == TextureFilterLinearMipmapLinear {
		levels = MipLevels(descriptor.Width, descriptor.Height)
	}
	handle := C.onix_vk_create_texture_with_format(
		d.native,
		C.uint32_t(descriptor.Width),
		C.uint32_t(descriptor.Height),
		C.uint32_t(levels),
		C.uint32_t(descriptor.Format),
		C.int(boolInt(descriptor.Filter == TextureFilterLinearMipmapLinear)),
		(*C.uint8_t)(unsafe.Pointer(&pixels[0])),
		C.size_t(len(pixels)),
		C.uint32_t(descriptor.WrapU), C.uint32_t(descriptor.WrapV),
	)
	if handle == 0 {
		return TextureHandle{}, vulkanError("create texture")
	}
	d.recordTextureUploadStatistics(1, uint64(len(pixels)))
	return d.textures.Insert(uint32(handle)), nil
}

func (d *vulkanDriver) CreateExternalTexture(descriptor ExternalTextureDescriptor) (TextureHandle, error) {
	if err := descriptor.Validate(); err != nil {
		return TextureHandle{}, err
	}
	if descriptor.Kind == ExternalTextureDMABuf {
		handle := C.onix_vk_create_external_dmabuf_texture(d.native, C.int(descriptor.Handle), C.uint32_t(descriptor.Width), C.uint32_t(descriptor.Height), C.uint32_t(descriptor.Stride), C.uint32_t(descriptor.Offset), C.uint64_t(descriptor.Modifier))
		if handle == 0 {
			return TextureHandle{}, vulkanError("import DMA-BUF texture")
		}
		return d.textures.Insert(uint32(handle)), nil
	}
	if descriptor.Kind != ExternalTextureD3D11 || descriptor.Format != ExternalTextureBGRA8 {
		return TextureHandle{}, errors.New("render: Vulkan external texture requires a D3D11 BGRA surface")
	}
	handle := C.onix_vk_create_external_d3d11_texture(
		d.native,
		C.uintptr_t(descriptor.Handle),
		C.uint32_t(descriptor.Width),
		C.uint32_t(descriptor.Height),
	)
	if handle == 0 {
		return TextureHandle{}, vulkanError("import D3D11 texture")
	}
	return d.textures.Insert(uint32(handle)), nil
}

func (d *vulkanDriver) RefreshExternalTexture(handle TextureHandle) error {
	native, ok := d.textures.Get(handle)
	if !ok {
		return fmt.Errorf("render: invalid Vulkan texture handle %s", handle)
	}
	if C.onix_vk_refresh_external_texture(d.native, C.uint32_t(native)) == 0 {
		return vulkanError("refresh D3D11 texture")
	}
	return nil
}

func (d *vulkanDriver) UpdateTextures(updates []TextureUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	d.updates = resetScratch(d.updates, &d.updateRetention)
	var length int
	var uploadBytes uint64
	for _, update := range updates {
		native, ok := d.textures.Get(update.Texture)
		region := update.Region
		if !ok || region.X < 0 || region.Y < 0 || region.Width <= 0 || region.Height <= 0 ||
			len(update.Pixels) == 0 {
			return errors.New("render: invalid Vulkan texture update")
		}
		// Vulkan buffer-to-image offsets must be aligned even for R8 payloads.
		offset := (length + 3) &^ 3
		if offset < length || len(update.Pixels) > int(^uint(0)>>1)-offset {
			return errors.New("render: Vulkan texture updates are too large")
		}
		length = offset + len(update.Pixels)
		uploadBytes += uint64(len(update.Pixels))
		d.updates = append(d.updates, C.OnixVkTextureUpdate{
			texture:     C.uint32_t(native),
			x:           C.int32_t(region.X),
			y:           C.int32_t(region.Y),
			width:       C.uint32_t(region.Width),
			height:      C.uint32_t(region.Height),
			data_offset: C.uint64_t(offset),
			data_length: C.uint64_t(len(update.Pixels)),
		})
	}
	if len(updates) == 1 {
		pixels := updates[0].Pixels
		if C.onix_vk_update_textures(d.native, &d.updates[0], 1, (*C.uint8_t)(unsafe.Pointer(&pixels[0])), C.size_t(len(pixels))) == 0 {
			return vulkanError("update textures")
		}
	} else {
		pixels := C.onix_vk_prepare_texture_updates(d.native, &d.updates[0], C.uint32_t(len(d.updates)), C.size_t(length))
		if pixels == nil {
			return vulkanError("prepare texture updates")
		}
		// Only native mapped memory is queued; Go pixel pointers stay in Go.
		staging := unsafe.Slice((*byte)(unsafe.Pointer(pixels)), length)
		for index, update := range updates {
			offset := int(d.updates[index].data_offset)
			copy(staging[offset:offset+len(update.Pixels)], update.Pixels)
		}
		if C.onix_vk_commit_texture_updates(d.native) == 0 {
			return vulkanError("commit texture updates")
		}
	}
	d.recordTextureUploadStatistics(len(updates), uploadBytes)
	return nil
}

func (d *vulkanDriver) DestroyTexture(handle TextureHandle) error {
	return destroyNativeResource(&d.textures, handle, "Vulkan texture", func(native uint32) error {
		if C.onix_vk_destroy_texture(d.native, C.uint32_t(native)) == 0 {
			return vulkanError("destroy texture")
		}
		return nil
	})
}

func (d *vulkanDriver) CreateRenderTarget(descriptor RenderTargetDescriptor) (RenderTargetHandle, TextureHandle, error) {
	if err := descriptor.Validate(); err != nil {
		return RenderTargetHandle{}, TextureHandle{}, err
	}
	var color C.uint32_t
	target := C.onix_vk_create_render_target(
		d.native,
		C.uint32_t(descriptor.Width),
		C.uint32_t(descriptor.Height),
		C.uint32_t(descriptor.ColorFormat),
		C.int(boolInt(descriptor.Depth)),
		&color,
	)
	if target == 0 || color == 0 {
		return RenderTargetHandle{}, TextureHandle{}, vulkanError("create render target")
	}
	colorHandle := d.textures.Insert(uint32(color))
	targetHandle := d.targets.Insert(nativeRenderTarget{native: uint32(target), color: colorHandle})
	return targetHandle, colorHandle, nil
}

func (d *vulkanDriver) ResizeRenderTarget(handle RenderTargetHandle, descriptor RenderTargetDescriptor) (TextureHandle, error) {
	if err := descriptor.Validate(); err != nil {
		return TextureHandle{}, err
	}
	target := d.targets.getRef(handle)
	if target == nil {
		return TextureHandle{}, fmt.Errorf("render: invalid Vulkan render target handle %s", handle)
	}
	var color C.uint32_t
	if C.onix_vk_resize_render_target(
		d.native,
		C.uint32_t(target.native),
		C.uint32_t(descriptor.Width),
		C.uint32_t(descriptor.Height),
		C.uint32_t(descriptor.ColorFormat),
		C.int(boolInt(descriptor.Depth)),
		&color,
	) == 0 || color == 0 {
		return TextureHandle{}, vulkanError("resize render target")
	}
	d.textures.Remove(target.color)
	target.color = d.textures.Insert(uint32(color))
	return target.color, nil
}

func (d *vulkanDriver) DestroyRenderTarget(handle RenderTargetHandle) error {
	target := d.targets.getRef(handle)
	if target == nil {
		return fmt.Errorf("render: invalid Vulkan render target handle %s", handle)
	}
	if C.onix_vk_destroy_render_target(d.native, C.uint32_t(target.native)) == 0 {
		return vulkanError("destroy render target")
	}
	d.textures.Remove(target.color)
	d.targets.Remove(handle)
	return nil
}

func (d *vulkanDriver) BeginRenderTarget(handle RenderTargetHandle, clear RenderTargetClear) error {
	target := d.targets.getRef(handle)
	if target == nil {
		return fmt.Errorf("render: invalid Vulkan render target handle %s", handle)
	}
	color := [4]C.float{C.float(clear.Color.R), C.float(clear.Color.G), C.float(clear.Color.B), C.float(clear.Color.A)}
	if C.onix_vk_begin_render_target(
		d.native,
		C.uint32_t(target.native),
		&color[0],
		C.float(clear.Depth),
		C.int(boolInt(clear.ClearColor)),
		C.int(boolInt(clear.ClearDepth)),
	) == 0 {
		return vulkanError("begin render target")
	}
	return nil
}

func (d *vulkanDriver) EndRenderTarget() error {
	if C.onix_vk_end_render_target(d.native) == 0 {
		return vulkanError("end render target")
	}
	return nil
}

func (d *vulkanDriver) ReadRenderTarget(handle RenderTargetHandle, region RenderTargetRegion, destination []byte) error {
	target := d.targets.getRef(handle)
	if target == nil {
		return fmt.Errorf("render: invalid Vulkan render target readback %s", handle)
	}
	if C.onix_vk_read_render_target(d.native, C.uint32_t(target.native), C.int32_t(region.X), C.int32_t(region.Y), C.uint32_t(region.Width), C.uint32_t(region.Height), (*C.uint8_t)(unsafe.Pointer(&destination[0])), C.size_t(len(destination))) == 0 {
		return vulkanError("read render target")
	}
	return nil
}

func (d *vulkanDriver) DrawSprites(batch SpriteBatch) error {
	if len(batch.Instances) == 0 || len(batch.Draws) == 0 {
		return nil
	}
	sprites, err := d.packSpriteDraws(&batch, nativeVulkan)
	if err != nil {
		return err
	}
	push := &d.spritePush
	copyFloat32ToC(push.projection[:], batch.Projection[:])
	if C.onix_vk_draw_sprites(
		d.native,
		push,
		(*C.OnixVkInstance)(unsafe.Pointer(&batch.Instances[0])),
		C.uint32_t(len(batch.Instances)),
		&d.draws[0],
		C.uint32_t(len(d.draws)),
	) == 0 {
		return vulkanError("draw sprites")
	}
	d.recordSpriteStatistics(batch, sprites, d.spriteBytes)
	return nil
}

func (d *vulkanDriver) CreateMaterial2DShader(descriptor ShaderDescriptor) (Material2DShaderHandle, error) {
	return createVulkanShader(descriptor, vulkanSpriteVertex, vulkanSpriteFragment, "Material2D", &d.shaders2D, func(vertex, fragment C.OnixVkShader) C.uint32_t {
		return C.onix_vk_create_material2d_shader(d.native, vertex, fragment)
	})
}

func (d *vulkanDriver) DestroyMaterial2DShader(handle Material2DShaderHandle) error {
	return destroyNativeResource(&d.shaders2D, handle, "Vulkan Material2D shader", func(shader uint32) error {
		if C.onix_vk_destroy_material2d_shader(d.native, C.uint32_t(shader)) == 0 {
			return vulkanError("destroy Material2D shader")
		}
		return nil
	})
}

func createVulkanShader[R any](descriptor ShaderDescriptor, defaultVertex, defaultFragment []byte, kind string, pool *GenerationPool[R, uint32], create func(C.OnixVkShader, C.OnixVkShader) C.uint32_t) (Handle[R], error) {
	if err := descriptor.Validate(); err != nil {
		return Handle[R]{}, err
	}
	if len(descriptor.VulkanVertexSPIRV) == 0 && len(descriptor.VulkanFragmentSPIRV) == 0 {
		return Handle[R]{}, fmt.Errorf("render: %s shader has no Vulkan SPIR-V", kind)
	}
	vertex := descriptor.VulkanVertexSPIRV
	if len(vertex) == 0 {
		vertex = defaultVertex
	}
	fragment := descriptor.VulkanFragmentSPIRV
	if len(fragment) == 0 {
		fragment = defaultFragment
	}
	shader := create(vulkanShader(vertex), vulkanShader(fragment))
	if shader == 0 {
		return Handle[R]{}, vulkanError("create " + kind + " shader")
	}
	return pool.Insert(uint32(shader)), nil
}

func (d *vulkanDriver) DrawShapes(batch ShapeBatch) error {
	if len(batch.Instances) == 0 {
		return nil
	}
	push := &d.shapePush
	copyFloat32ToC(push.projection[:], batch.Projection[:])
	if C.onix_vk_draw_shapes(
		d.native,
		push,
		(*C.OnixVkShapeInstance)(unsafe.Pointer(&batch.Instances[0])),
		C.uint32_t(len(batch.Instances)),
	) == 0 {
		return vulkanError("draw shapes")
	}
	d.recordShapeStatistics(batch)
	return nil
}

func (d *vulkanDriver) CreateMesh(vertices []Vertex3D, indices []uint32) (MeshHandle, error) {
	if len(vertices) == 0 || len(indices) == 0 {
		return MeshHandle{}, errors.New("render: Vulkan mesh data is empty")
	}
	mesh := C.onix_vk_create_mesh(
		d.native,
		(*C.OnixVkVertex3D)(unsafe.Pointer(&vertices[0])), C.uint32_t(len(vertices)),
		(*C.uint32_t)(unsafe.Pointer(&indices[0])), C.uint32_t(len(indices)),
	)
	if mesh == 0 {
		return MeshHandle{}, vulkanError("create mesh")
	}
	return d.meshes.Insert(nativeMesh{native: uint32(mesh)}), nil
}

func (d *vulkanDriver) DestroyMesh(handle MeshHandle) error {
	mesh := d.meshes.getRef(handle)
	if mesh == nil {
		return fmt.Errorf("render: invalid Vulkan mesh handle %s", handle)
	}
	if C.onix_vk_destroy_mesh(d.native, C.uint32_t(mesh.native)) == 0 {
		return vulkanError("destroy mesh")
	}
	d.meshes.Remove(handle)
	return nil
}

func (d *vulkanDriver) DrawMeshes(batch MeshBatch3D) error {
	return d.drawMeshes(batch, false)
}

func (d *vulkanDriver) DrawMeshPicking(batch MeshBatch3D) error {
	if len(batch.Instances) == 0 || len(batch.Draws) == 0 {
		return nil
	}
	err := d.packMeshPickingDraws(&batch, nativeVulkan)
	if err != nil {
		return err
	}
	dynamic := len(batch.Vertices) != 0 || len(batch.Indices) != 0
	push := vulkanMeshPush(&batch)
	var vertices *C.OnixVkVertex3D
	var indices *C.uint32_t
	if dynamic {
		vertices = (*C.OnixVkVertex3D)(unsafe.Pointer(&batch.Vertices[0]))
		indices = (*C.uint32_t)(unsafe.Pointer(&batch.Indices[0]))
	}
	if C.onix_vk_draw_mesh_picking(
		d.native,
		&push,
		vertices, C.uint32_t(len(batch.Vertices)),
		indices, C.uint32_t(len(batch.Indices)),
		(*C.OnixVkInstance3D)(unsafe.Pointer(&batch.Instances[0])), C.uint32_t(len(batch.Instances)),
		&d.draws3D[0], C.uint32_t(len(d.draws3D)),
	) == 0 {
		return vulkanError("draw mesh picking")
	}
	return nil
}

func vulkanMeshPush(batch *MeshBatch3D) C.OnixVkMeshPush {
	push := C.OnixVkMeshPush{}
	// The mesh and picking vertex shaders already flip clip-space Y for
	// Vulkan's top-down viewport, on both the swapchain and render targets.
	copyFloat32ToC(push.view_projection[:], batch.ViewProjection[:])
	copyFloat32ToC(push.camera_position[:], batch.CameraPosition[:])
	copyFloat32ToC(push.ambient_light[:], batch.AmbientLight[:])
	copyFloat32ToC(push.light_direction[:], batch.LightDirection[:])
	copyFloat32ToC(push.directional_light[:], batch.DirectionalLight[:])
	return push
}

func (d *vulkanDriver) DrawMeshesDeformed(batch MeshBatch3D) error {
	var err error
	d.deformationWords = resetScratch(d.deformationWords, &d.deformationRetention)
	d.deformationWords, err = packMeshDeformations(d.deformationWords, batch, d)
	if err != nil {
		return fmt.Errorf("render: invalid Vulkan deformation: %w", err)
	}
	return d.drawMeshes(batch, true)
}

func (d *vulkanDriver) drawMeshes(batch MeshBatch3D, gpuDeformation bool) error {
	if len(batch.Instances) == 0 || len(batch.Draws) == 0 {
		return nil
	}
	counts, err := d.packMeshDraws(&batch, nativeVulkan, gpuDeformation)
	if err != nil {
		return err
	}
	dynamic := len(batch.Vertices) != 0 || len(batch.Indices) != 0
	push := vulkanMeshPush(&batch)
	var vertices *C.OnixVkVertex3D
	var indices *C.uint32_t
	if dynamic {
		vertices = (*C.OnixVkVertex3D)(unsafe.Pointer(&batch.Vertices[0]))
		indices = (*C.uint32_t)(unsafe.Pointer(&batch.Indices[0]))
	}
	lighting := d.lighting3D[:]
	copy(lighting[:16], batch.ViewProjection[:])
	writeMeshSceneUniforms(lighting, &batch)
	if C.onix_vk_draw_meshes(
		d.native,
		&push,
		vertices, C.uint32_t(len(batch.Vertices)),
		indices, C.uint32_t(len(batch.Indices)),
		(*C.OnixVkInstance3D)(unsafe.Pointer(&batch.Instances[0])), C.uint32_t(len(batch.Instances)),
		(*C.uint32_t)(unsafe.Pointer(&d.deformationWords[0])), C.uint32_t(len(d.deformationWords)),
		&d.draws3D[0], C.uint32_t(len(d.draws3D)),
		(*C.float)(unsafe.Pointer(&lighting[0])),
	) == 0 {
		return vulkanError("draw meshes")
	}
	d.recordMeshStatistics(batch, counts)
	return nil
}

func (d *vulkanDriver) CreateMeshShader(descriptor ShaderDescriptor) (MeshShaderHandle, error) {
	return createVulkanShader(descriptor, vulkanMeshVertex, vulkanMeshFragment, "mesh", &d.shaders, func(vertex, fragment C.OnixVkShader) C.uint32_t {
		return C.onix_vk_create_mesh_shader(d.native, vertex, fragment)
	})
}

func (d *vulkanDriver) DestroyMeshShader(handle MeshShaderHandle) error {
	return destroyNativeResource(&d.shaders, handle, "Vulkan mesh shader", func(shader uint32) error {
		if C.onix_vk_destroy_mesh_shader(d.native, C.uint32_t(shader)) == 0 {
			return vulkanError("destroy mesh shader")
		}
		return nil
	})
}

func (d *vulkanDriver) Present() error {
	if C.onix_vk_present(d.native) == 0 {
		return vulkanError("present")
	}
	return nil
}

func (d *vulkanDriver) ResizeSurface(width, height int) error {
	if width <= 0 || height <= 0 || (d.width == width && d.height == height) {
		return nil
	}
	if C.onix_vk_resize(d.native, C.uint32_t(width), C.uint32_t(height)) == 0 {
		return vulkanError("resize surface")
	}
	d.width, d.height = width, height
	d.viewportSet = false
	return nil
}

func (d *vulkanDriver) SetVSync(enabled bool) {
	value := C.int(0)
	if enabled {
		value = 1
	}
	C.onix_vk_set_vsync(d.native, value)
}

func (d *vulkanDriver) SetSurfaceCornerRadius(radius int) {
	C.onix_vk_set_corner_radius(d.native, C.int(radius))
}

func (d *vulkanDriver) Destroy() {
	if d.native != nil {
		saveVulkanPipelineCache(d.native)
		C.onix_vk_destroy(d.native)
		d.native = nil
		d.textures.Clear()
		d.targets.Clear()
		d.meshes.Clear()
		d.shaders.Clear()
		d.shaders2D.Clear()
	}
}

const vulkanPipelineCacheLimit = 32 << 20

func vulkanPipelineCachePath(native *C.OnixVkDriver) string {
	directory, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	uuid := unsafe.Slice((*byte)(unsafe.Pointer(C.onix_vk_pipeline_cache_uuid(native))), 16)
	return filepath.Join(directory, "onix", "vulkan", fmt.Sprintf("%x.bin", uuid))
}

func loadVulkanPipelineCache(native *C.OnixVkDriver) {
	path := vulkanPipelineCachePath(native)
	info, err := os.Stat(path)
	if err != nil || info.Size() < sha256.Size+32 || info.Size() > vulkanPipelineCacheLimit+sha256.Size {
		return
	}
	data, err := os.ReadFile(path)
	if err == nil && len(data) >= sha256.Size+32 && len(data) <= vulkanPipelineCacheLimit+sha256.Size && sha256.Sum256(data[sha256.Size:]) == [sha256.Size]byte(data[:sha256.Size]) {
		C.onix_vk_load_pipeline_cache(native, unsafe.Pointer(&data[sha256.Size]), C.size_t(len(data)-sha256.Size))
	}
}

func saveVulkanPipelineCache(native *C.OnixVkDriver) {
	path := vulkanPipelineCachePath(native)
	if path == "" {
		return
	}
	size := C.onix_vk_pipeline_cache_data(native, nil, 0)
	if size < 32 || size > vulkanPipelineCacheLimit {
		return
	}
	data := make([]byte, sha256.Size+int(size))
	size = C.onix_vk_pipeline_cache_data(native, unsafe.Pointer(&data[sha256.Size]), size)
	if size == 0 || os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	file, err := os.CreateTemp(filepath.Dir(path), "pipeline-*")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	data = data[:sha256.Size+int(size)]
	digest := sha256.Sum256(data[sha256.Size:])
	copy(data[:sha256.Size], digest[:])
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(file.Name(), path)
	}
}

func vulkanShader(data []byte) C.OnixVkShader {
	return C.OnixVkShader{
		code: (*C.uint32_t)(unsafe.Pointer(&data[0])),
		size: C.size_t(len(data)),
	}
}

func vulkanError(operation string) error {
	message := C.GoString(C.onix_vk_last_error())
	if message == "" {
		message = "unknown Vulkan error"
	}
	return fmt.Errorf("render: %s: %s", operation, message)
}

func copyFloat32ToC(destination []C.float, source []float32) {
	for index := range source {
		destination[index] = C.float(source[index])
	}
}

func (d *vulkanDriver) deformationLayout(handle MeshHandle) (meshDeformationLayout, bool) {
	mesh := d.meshes.getRef(handle)
	if mesh == nil {
		return meshDeformationLayout{}, false
	}
	return mesh.deformation, mesh.deformation.vertexCount != 0
}

func (d *vulkanDriver) SetMeshDeformation(handle MeshHandle, descriptor MeshDeformationDescriptor) error {
	mesh := d.meshes.getRef(handle)
	if mesh == nil {
		return fmt.Errorf("render: invalid mesh handle %s", handle)
	}
	scratch := meshDeformationBuffers.get()
	defer meshDeformationBuffers.put(scratch)
	words, layout, err := packMeshDeformationStatic(scratch.data, descriptor)
	scratch.data = words
	if err != nil {
		return err
	}
	if C.onix_vk_set_mesh_deformation(d.native, C.uint32_t(mesh.native), (*C.uint32_t)(unsafe.Pointer(&words[0])), C.uint32_t(len(words)), C.uint32_t(descriptor.VertexCount)) == 0 {
		return vulkanError("upload mesh deformation")
	}
	mesh.deformation = layout
	return nil
}
