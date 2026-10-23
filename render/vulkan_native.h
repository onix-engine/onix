#ifndef ONIX_ENGINE_RENDER_VULKAN_NATIVE_H
#define ONIX_ENGINE_RENDER_VULKAN_NATIVE_H

#define ONIX_VK_MESH_LIGHTING_BYTES 3584

#include <stddef.h>
#include <stdint.h>
#include "native_buffer.h"
#include "native_draw.h"

enum {
    ONIX_VK_SURFACE_XLIB = 1,
    ONIX_VK_SURFACE_WAYLAND = 2,
    ONIX_VK_SURFACE_ANDROID = 3,
    ONIX_VK_SURFACE_WIN32 = 4,
    ONIX_VK_SURFACE_DRM = 5,
};

typedef struct OnixVkDriver OnixVkDriver;
void onix_vk_buffer_memory(OnixVkDriver *driver, OnixBufferMemory *memory);

typedef struct {
    int kind;
    void *display;
    uintptr_t window;
    uint32_t width;
    uint32_t height;
    int transparent;
    int32_t drm_fd;
    uint32_t drm_connector_id;
    uint32_t drm_refresh_rate;
} OnixVkSurface;

typedef struct {
    const uint32_t *code;
    size_t size;
} OnixVkShader;

typedef struct {
	float model[6];
	float texture_model[6];
	float color[4];
	float custom[4][4];
	float texture_slot;
} OnixVkInstance;

typedef struct {
	float model[6];
	float local_model[6];
	float fill[4];
	float border[4];
	float params[4];
	uint32_t kind;
} OnixVkShapeInstance;

typedef struct {
	uint32_t texture;
	int32_t x;
	int32_t y;
	uint32_t width;
	uint32_t height;
	uint64_t data_offset;
	uint64_t data_length;
} OnixVkTextureUpdate;

/* These values mirror render.TextureFormat so the driver can pass the Go enum
 * straight through. Block-compressed entries start at BC1 and are contiguous,
 * which is what onix_vk_texture_format_is_compressed relies on. */
#define ONIX_TEXTURE_FORMAT_RGBA8 0
#define ONIX_TEXTURE_FORMAT_R8 1
#define ONIX_TEXTURE_FORMAT_SRGBA8 2
#define ONIX_TEXTURE_FORMAT_BC1 3
#define ONIX_TEXTURE_FORMAT_BC1_SRGB 4
#define ONIX_TEXTURE_FORMAT_BC3 5
#define ONIX_TEXTURE_FORMAT_BC3_SRGB 6
#define ONIX_TEXTURE_FORMAT_BC4 7
#define ONIX_TEXTURE_FORMAT_BC5 8
#define ONIX_TEXTURE_FORMAT_BC7 9
#define ONIX_TEXTURE_FORMAT_BC7_SRGB 10
#define ONIX_TEXTURE_FORMAT_ASTC_4X4 11
#define ONIX_TEXTURE_FORMAT_ASTC_4X4_SRGB 12
#define ONIX_TEXTURE_FORMAT_COUNT 13

typedef struct {
    float projection[16];
	float shader_padding[8];
} OnixVkSpritePush;

typedef struct {
	float position[3];
	float normal[3];
	float uv[2];
	float tangent[4];
} OnixVkVertex3D;

typedef OnixNativeInstance3D OnixVkInstance3D;

typedef struct {
	float view_projection[16];
	float camera_position[4];
	float ambient_light[4];
	float light_direction[4];
	float directional_light[4];
} OnixVkMeshPush;

const char *onix_vk_last_error(void);
const char *onix_vk_device_name(OnixVkDriver *driver);

OnixVkDriver *onix_vk_create(
    OnixVkSurface surface,
    OnixVkShader sprite_vertex,
	OnixVkShader sprite_fragment,
	OnixVkShader shape_vertex,
	OnixVkShader shape_fragment,
	OnixVkShader mesh_vertex,
	OnixVkShader mesh_fragment,
	OnixVkShader mesh_unlit_fragment,
	OnixVkShader mesh_depth_fragment,
	OnixVkShader mesh_basic_fragment,
	OnixVkShader picking_vertex,
	OnixVkShader picking_fragment);
void onix_vk_destroy(OnixVkDriver *driver);
int onix_vk_resize(OnixVkDriver *driver, uint32_t width, uint32_t height);

int onix_vk_begin_frame(OnixVkDriver *driver, const float clear_color[4]);
void onix_vk_set_sprite_blend(OnixVkDriver *driver, int mode, float red, float green, float blue, float alpha);
void onix_vk_set_viewport(OnixVkDriver *driver, int32_t x, int32_t y, int32_t width, int32_t height);
void onix_vk_set_scissor(OnixVkDriver *driver, int enabled, int32_t x, int32_t y, int32_t width, int32_t height);
void onix_vk_set_vsync(OnixVkDriver *driver, int enabled);
void onix_vk_set_corner_radius(OnixVkDriver *driver, int radius);
int onix_vk_present(OnixVkDriver *driver);

/* levels is the depth of the mip chain the caller wants. An uncompressed
 * payload supplies the base level only and the driver blits the rest; a
 * block-compressed payload supplies every level, because the device cannot
 * generate mips for block data. */
uint32_t onix_vk_create_texture_with_format(OnixVkDriver *driver, uint32_t width, uint32_t height, uint32_t levels, uint32_t format, int mipmap, const uint8_t *pixels, size_t length, uint32_t wrap_u, uint32_t wrap_v);
/* Encoded size of a mip chain, exposed so the Go side can check its own
 * layout arithmetic against this one. */
uint64_t onix_vk_texture_chain_size(uint32_t format, uint32_t width, uint32_t height, uint32_t levels);
uint32_t onix_vk_texture_mip_level_count(uint32_t width, uint32_t height);
uint32_t onix_vk_create_external_d3d11_texture(OnixVkDriver *driver, uintptr_t texture, uint32_t width, uint32_t height);
uint32_t onix_vk_create_external_dmabuf_texture(OnixVkDriver *driver, int fd, uint32_t width, uint32_t height, uint32_t stride, uint32_t offset, uint64_t modifier);
int onix_vk_refresh_external_texture(OnixVkDriver *driver, uint32_t texture);
int onix_vk_update_textures(OnixVkDriver *driver, const OnixVkTextureUpdate *updates, uint32_t update_count, const uint8_t *pixels, size_t length);
/* Prepare validates the entire batch and returns native mapped storage. Fill
 * every data range before calling commit or any other driver entry point.
 * data_offset values must be multiples of four. */
uint8_t *onix_vk_prepare_texture_updates(OnixVkDriver *driver, const OnixVkTextureUpdate *updates, uint32_t update_count, size_t length);
int onix_vk_commit_texture_updates(OnixVkDriver *driver);
int onix_vk_destroy_texture(OnixVkDriver *driver, uint32_t texture);

uint32_t onix_vk_create_render_target(OnixVkDriver *driver, uint32_t width, uint32_t height, uint32_t color_format, int depth, uint32_t *color_texture);
int onix_vk_resize_render_target(OnixVkDriver *driver, uint32_t target, uint32_t width, uint32_t height, uint32_t color_format, int depth, uint32_t *color_texture);
int onix_vk_destroy_render_target(OnixVkDriver *driver, uint32_t target);
int onix_vk_begin_render_target(OnixVkDriver *driver, uint32_t target, const float clear_color[4], float clear_depth, int clear_color_enabled, int clear_depth_enabled);
int onix_vk_end_render_target(OnixVkDriver *driver);
int onix_vk_read_render_target(OnixVkDriver *driver, uint32_t target, int32_t x, int32_t y, uint32_t width, uint32_t height, uint8_t *destination, size_t length);

uint32_t onix_vk_create_mesh(OnixVkDriver *driver, const OnixVkVertex3D *vertices, uint32_t vertex_count, const uint32_t *indices, uint32_t index_count);
int onix_vk_set_mesh_deformation(OnixVkDriver *driver, uint32_t mesh, const uint32_t *words, uint32_t word_count, uint32_t vertex_count);
int onix_vk_destroy_mesh(OnixVkDriver *driver, uint32_t mesh);

uint32_t onix_vk_create_mesh_shader(OnixVkDriver *driver, OnixVkShader vertex, OnixVkShader fragment);
const uint8_t *onix_vk_pipeline_cache_uuid(OnixVkDriver *driver);
void onix_vk_load_pipeline_cache(OnixVkDriver *driver, const void *data, size_t size);
size_t onix_vk_pipeline_cache_data(OnixVkDriver *driver, void *data, size_t size);
int onix_vk_destroy_mesh_shader(OnixVkDriver *driver, uint32_t shader);
uint32_t onix_vk_create_material2d_shader(OnixVkDriver *driver, OnixVkShader vertex, OnixVkShader fragment);
int onix_vk_destroy_material2d_shader(OnixVkDriver *driver, uint32_t shader);

/* Sprite and mesh batches validate each draw while recording commands. On
 * failure, earlier draws in the batch may already have been recorded. */
int onix_vk_draw_sprites(
	OnixVkDriver *driver,
	const OnixVkSpritePush *push,
	const OnixVkInstance *instances,
	uint32_t instance_count,
	const OnixNativeSpriteDraw *draws,
	uint32_t draw_count);

int onix_vk_draw_shapes(
	OnixVkDriver *driver,
	const OnixVkSpritePush *push,
	const OnixVkShapeInstance *instances,
	uint32_t instance_count);

int onix_vk_draw_meshes(
	OnixVkDriver *driver,
	const OnixVkMeshPush *push,
	const OnixVkVertex3D *vertices,
	uint32_t vertex_count,
	const uint32_t *indices,
	uint32_t index_count,
	const OnixVkInstance3D *instances,
	uint32_t instance_count,
	const uint32_t *deformation_words,
	uint32_t deformation_word_count,
	const OnixNativeMeshDraw3D *draws,
	uint32_t draw_count, const float *lighting);

int onix_vk_draw_mesh_picking(
	OnixVkDriver *driver,
	const OnixVkMeshPush *push,
	const OnixVkVertex3D *vertices,
	uint32_t vertex_count,
	const uint32_t *indices,
	uint32_t index_count,
	const OnixVkInstance3D *instances,
	uint32_t instance_count,
	const OnixNativeMeshDraw3D *draws,
	uint32_t draw_count);
#endif
