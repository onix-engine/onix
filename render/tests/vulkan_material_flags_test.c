// Offscreen pixel regression: perspective interpolation must not change flags.
#include "../vulkan_native.c"
#include <math.h>

static void require(int success, const char *operation) {
    if (!success) {
        fprintf(stderr, "%s: %s\n", operation, onix_vk_last_error());
        exit(1);
    }
}

static OnixVkShader load_shader(const char *path) {
    FILE *file = fopen(path, "rb");
    require(file != NULL, path);
    fseek(file, 0, SEEK_END);
    long length = ftell(file);
    rewind(file);
    uint32_t *code = malloc((size_t)length);
    require(code != NULL && fread(code, 1, (size_t)length, file) == (size_t)length, "read shader");
    fclose(file);
    return (OnixVkShader){code, (size_t)length};
}

static void check_sprite_uploads(OnixVkDriver *driver) {
    const uint8_t white[] = {255,255,255,255};
    uint32_t texture = onix_vk_create_texture_with_format(driver, 1, 1, 1, ONIX_TEXTURE_FORMAT_RGBA8, 0, white, sizeof(white), 0, 0);
    OnixVkShader vertex = load_shader("shaders/vulkan_sprite.vert.spv");
    OnixVkShader fragment = load_shader("shaders/vulkan_sprite.frag.spv");
    uint32_t shader = onix_vk_create_material2d_shader(driver, vertex, fragment);
    require(texture && shader, "create sprite layout resources");
    require(driver->material2d_pipelines[shader-1].pipeline == VK_NULL_HANDLE, "custom sprite pipeline starts lazy");
    OnixVkSpritePush push = {.projection = {[0]=.125f,[5]=.125f,[10]=1,[12]=-1,[13]=-1,[15]=1}};
    OnixVkInstance instances[4];
    const float colors[4][4] = {{1,0,0,1},{0,1,0,1},{0,0,1,1},{1,1,0,1}};
    for (int i = 0; i < 4; i++) {
        instances[i] = (OnixVkInstance){.model={4,0,(float)i*4,0,16,0},.texture_model={1,0,0,0,1,0}};
        memcpy(instances[i].color, colors[i], sizeof(colors[i]));
        // Nonzero discarded lanes must not move clipping or texture slots.
        instances[i].custom[0][0] = 123;
        instances[i].custom[1][0] = 456;
    }
    // The first compact draw is clipped to its left half.
    instances[0].custom[2][2] = 2;
    instances[0].custom[2][3] = 16;
    instances[0].custom[3][1] = 1;
    OnixNativeSpriteDraw draws[] = {
        {.texture=texture,.instance_count=2},
        {.shader=shader,.texture=texture,.first_instance=2,.instance_count=1},
        {.texture=texture,.first_instance=3,.instance_count=1},
    };
    const float clear[] = {0,0,0,0};
    for (int depth = 0; depth < 2; depth++) {
        uint32_t color, target = onix_vk_create_render_target(driver, 16, 16, 0, depth, &color);
        require(target != 0, "create sprite layout target");
        require(onix_vk_begin_frame(driver, clear), "begin sprite layout frame");
        require(onix_vk_begin_render_target(driver, target, clear, 0, 1, depth), "begin sprite layout target");
        onix_vk_set_viewport(driver, 0, 0, 16, 16);
        require(onix_vk_draw_sprites(driver, &push, instances, 4, draws, 3), "draw mixed sprite layouts");
        require(onix_vk_end_render_target(driver) && onix_vk_present(driver), "submit mixed sprites");
        for (int i = 0; i < 4; i++) {
            uint8_t pixel[4];
            require(onix_vk_read_render_target(driver, target, i*4+1, 8, 1, 1, pixel, 4), "read mixed sprite pixel");
            for (int channel = 0; channel < 4; channel++) {
                int mapped = channel;
                if ((driver->swapchain_format == VK_FORMAT_B8G8R8A8_UNORM || driver->swapchain_format == VK_FORMAT_B8G8R8A8_SRGB) && channel != 1 && channel != 3) mapped = 2-channel;
                require(abs((int)pixel[mapped] - (int)(colors[i][channel]*255)) <= 1, "mixed sprite tint and offset");
            }
        }
        uint8_t clipped[4];
        require(onix_vk_read_render_target(driver, target, 3, 8, 1, 1, clipped, 4), "read compact clipping");
        require(clipped[3] == 0, "compact clipping lanes preserved");
        require(onix_vk_destroy_render_target(driver, target), "destroy sprite layout target");
    }
    require(onix_vk_destroy_material2d_shader(driver, shader), "retire custom sprite pipeline");
    require(onix_vk_destroy_texture(driver, texture), "retire sprite texture");
    free((void *)vertex.code);
    free((void *)fragment.code);
    printf("Mixed compact/custom sprite layouts, clipping and depth variants passed\n");
}

static void check_pipeline_cache(OnixVkDriver *driver) {
    size_t size = onix_vk_pipeline_cache_data(driver, NULL, 0);
    require(size >= 32, "query pipeline cache");
    void *data = malloc(size);
    require(data != NULL && onix_vk_pipeline_cache_data(driver, data, size) == size, "export pipeline cache");
    onix_vk_load_pipeline_cache(driver, data, size);
    ((uint8_t *)data)[16] ^= 255;
    onix_vk_load_pipeline_cache(driver, data, size);
    onix_vk_load_pipeline_cache(driver, data, 4);
    free(data);
    require(onix_vk_pipeline_cache_data(driver, NULL, 0) >= 32, "pipeline cache survives mismatched and truncated data");
    printf("Pipeline cache export, import and device validation passed\n");
}

int main(void) {
    HINSTANCE module = GetModuleHandleW(NULL);
    WNDCLASSW cls = {.lpfnWndProc = DefWindowProcW, .hInstance = module, .lpszClassName = L"OnixMaterialFlagsTest"};
    require(RegisterClassW(&cls) != 0, "register hidden window");
    HWND window = CreateWindowW(cls.lpszClassName, L"", WS_OVERLAPPEDWINDOW, 0, 0, 64, 64, NULL, NULL, module, NULL);
    require(window != NULL, "create hidden window");
    OnixVkShader shaders[] = {
        load_shader("shaders/vulkan_sprite_multi.vert.spv"), load_shader("shaders/vulkan_sprite_multi.frag.spv"),
        load_shader("shaders/vulkan_shape.vert.spv"), load_shader("shaders/vulkan_shape.frag.spv"),
        load_shader("shaders/vulkan_mesh.vert.spv"), load_shader("shaders/vulkan_mesh.frag.spv"),
        load_shader("shaders/vulkan_mesh_unlit.frag.spv"),
        load_shader("shaders/vulkan_mesh_depth.frag.spv"), load_shader("shaders/vulkan_mesh_basic.frag.spv"),
        load_shader("shaders/vulkan_picking.vert.spv"), load_shader("shaders/vulkan_picking.frag.spv"),
    };
    OnixVkSurface surface = {.kind = ONIX_VK_SURFACE_WIN32, .window = (uintptr_t)window, .width = 64, .height = 64};
    OnixVkDriver *driver = onix_vk_create(surface, shaders[0], shaders[1], shaders[2], shaders[3], shaders[4], shaders[5], shaders[6], shaders[7], shaders[8], shaders[9], shaders[10]);
    require(driver != NULL, "create Vulkan driver");
    check_sprite_uploads(driver);
    check_pipeline_cache(driver);
    VkPhysicalDeviceProperties props;
    vkGetPhysicalDeviceProperties(driver->physical_device, &props);
    printf("GPU: %s\n", props.deviceName);
    const uint8_t radiance[] = {255, 255, 255, 126};
    uint32_t texture = onix_vk_create_texture_with_format(driver, 1, 1, 1, ONIX_TEXTURE_FORMAT_RGBA8, 0, radiance, sizeof(radiance), 0, 0);
    uint32_t color, target = onix_vk_create_render_target(driver, 1280, 800, 0, 1, &color);
    require(texture && target, "create offscreen resources");
    const int faces[6][4] = {{4,5,7,6},{1,0,2,3},{5,1,3,7},{0,4,6,2},{6,7,3,2},{0,1,5,4}};
    const float normals[6][3] = {{0,0,1},{0,0,-1},{1,0,0},{-1,0,0},{0,1,0},{0,-1,0}};
    const float uv[4][2] = {{0,1},{1,1},{1,0},{0,0}};
    OnixVkVertex3D vertices[24] = {0};
    uint32_t indices[36];
    for (int f = 0; f < 6; f++) {
        for (int v = 0; v < 4; v++) {
            int i = f*4+v, corner = faces[f][v];
            for (int axis = 0; axis < 3; axis++) {
                vertices[i].position[axis] = (corner & (1<<axis)) ? 1 : -1;
                vertices[i].normal[axis] = normals[f][axis];
            }
            memcpy(vertices[i].uv, uv[v], sizeof(uv[v]));
        }
        const int offsets[] = {0,1,2,0,2,3};
        for (int i = 0; i < 6; i++) indices[f*6+i] = f*4+offsets[i];
    }
    OnixVkInstance3D instance = {.model = {[0]=1,[5]=1,[10]=1,[15]=1}, .base_color = {1,1,1,1}, .material = {.5f,1.5f,.5f,8388608+2048+512}, .uv_transform = {1,1,0,0}};
    OnixNativeMeshDraw3D draw = {.texture = texture, .index_count = 36, .instance_count = 1};
    draw.mesh = onix_vk_create_mesh(driver, vertices, 24, indices, 36);
    require(draw.mesh != 0, "create sky mesh");
    uint32_t deformation[16] = {0};
    float lighting[ONIX_VK_MESH_LIGHTING_BYTES/sizeof(float)] = {[296]=1};
    const float clear[] = {0,0,0,1};
    uint8_t *pixels = malloc(1280*800*4);
    require(pixels != NULL, "allocate readback");
    int bad = 0;
    for (int angle = 0; angle < 12; angle++) {
        float yaw = -.555f + angle*.4f, pitch = .174f + angle*.17f;
        float sy = sinf(yaw), cy = cosf(yaw), sx = sinf(pitch), cx = cosf(pitch);
        // Infinite reverse-Z perspective, with the eye inside the sky cube.
        OnixVkMeshPush push = {.view_projection = {
            1.508883f*cy, 2.414214f*sx*sy, 0, -cx*sy,
            0, 2.414214f*cx, 0, sx,
            -1.508883f*sy, 2.414214f*sx*cy, 0, -cx*cy,
            0,0,.03f,0}};
        instance.model[12] = -2.8f; instance.model[13] = 1.1f; instance.model[14] = 4.5f;
        for (int row = 0; row < 4; row++) {
            push.view_projection[12+row] -= push.view_projection[row]*instance.model[12] + push.view_projection[4+row]*instance.model[13] + push.view_projection[8+row]*instance.model[14];
        }
        lighting[16] = instance.model[12]; lighting[17] = instance.model[13]; lighting[18] = instance.model[14]; lighting[19] = 1;
        require(onix_vk_begin_frame(driver, clear), "begin frame");
        require(onix_vk_begin_render_target(driver, target, clear, 0, 1, 1), "begin target");
        onix_vk_set_viewport(driver, 0, 0, 1280, 800);
        require(onix_vk_draw_meshes(driver, &push, NULL, 0, NULL, 0, &instance, 1, deformation, 16, &draw, 1, lighting), "draw sky");
        require(onix_vk_end_render_target(driver), "end target");
        require(onix_vk_present(driver), "present frame");
        require(onix_vk_read_render_target(driver, target, 0, 0, 1280, 800, pixels, 1280*800*4), "read target");
        int defects = 0;
        for (int i = 0; i < 1280*800; i++) {
            if (abs((int)pixels[i*4]-124)>1 || abs((int)pixels[i*4+1]-124)>1 || abs((int)pixels[i*4+2]-124)>1 || pixels[i*4+3]!=255) defects++;
        }
        printf("perspective %d: %d incorrect sky pixels\n", angle, defects);
        bad += defects;
    }
    free(pixels);
    onix_vk_destroy(driver);
    for (size_t i = 0; i < sizeof(shaders)/sizeof(shaders[0]); i++) free((void *)shaders[i].code);
    DestroyWindow(window);
    UnregisterClassW(cls.lpszClassName, module);
    return bad != 0;
}
