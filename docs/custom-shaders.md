# Custom 2D and 3D shaders

Attach generic `scene.Shader[T]` components to elements, with T equal to
`graphics.Material2DShader` or `graphics.MeshShader`. Program points to a shared
GPU program; Custom stores four independent vec4 groups per element.

## Configuration and compilation

Use module-relative WGSL paths in components.shader.wgslVertex/wgslFragment.
There is no module-level shaders manifest or material.shader reference. In the
editor, select an element, Add Component > Shader, then select .wgsl source files.
An empty component uses builtin stages; overriding one stage keeps the other builtin.

```json
{
  "space": "ui",
  "elements": [
    {
      "id": "effect",
      "components": {
        "sprite": {"color": "#ffffff", "size": [620, 420]},
        "shader": {"wgslFragment": "shaders/effect.frag.wgsl"}
      }
    }
  ]
}
```

Preview compiles WGSL in memory and caches it without rewriting configuration.
Builds use pure-Go github.com/gogpu/naga for Vulkan SPIR-V, Metal MSL and WebGL2
GLSL ES 3.00; WebGPU consumes WGSL. Android includes SPIR-V and GLSL ES. Compiled
stages and element configuration enter platform resource packages. Generated
backend paths/flags live only in internal binary build plans.

Entry points are sprite_vertex/sprite_fragment or mesh_vertex/mesh_fragment.
When overriding both stages, put them in one WGSL module so Metal gets a complete
library. Source files must belong to their module.

## 2D

The Shader component sits beside Sprite2D/SpriteTrail2D. Solid sprites with shaders
use textured drawing; Shape2D has its own vector pipeline. Update uniforms through
module entity handles:

```go
func UpdateEffect(c *module.Context, effect module.Entity, tick module.Tick) error {
    shader, ok := module.Get[scene.Shader[graphics.Material2DShader]](c, effect)
    if !ok {
        return module.ErrType
    }
    shader.Custom[0][0] += tick.Delta
    return module.Set(c, effect, shader)
}
```

Direct ECS composition:

```go
shader, err := graphics.NewMaterial2DShader(descriptor)
if err != nil {
    return err
}
ecs.Put(&bundle, scene.Shader[graphics.Material2DShader]{Program: shader})
```

| Location | Type | Input |
| --- | --- | --- |
| 0 | vec3 | Quad position |
| 1 | vec2 | Quad UV |
| 2–3 | vec3 | Model affine rows |
| 4–5 | vec3 | UV affine rows |
| 6 | vec4 | Instance color |
| 7–10 | vec4 | Material2DData.Custom[0:4] |
| 11 | float | Builtin sprite texture slot; unused by custom materials |

Instance2D has a 132-byte stride and TextureSlot at byte 128; earlier fields keep
their offsets. Builtin sprites batch four textures; custom materials use one texture
per draw and single-texture fallback stages. Vulkan projection is a push constant,
texture set 0/binding 0. Fallback fragment inputs are fragTexture at location 0,
fragColor at 1 and flat fragCustom0..3 at 2–5.

Metal uses sprite entry points, projection buffer 2 and texture 0. WebGL2 uses
projection/tex uniforms and vTexture/vColor/vCustom0..3 varyings. WebGPU uses
projection group 0/binding 0, texture/sampler bindings 1/2, and fragment locations
0–5 for UV, color and four flat Custom groups. The 2D pipeline uses premultiplied
alpha: output vec4(rgb * alpha, alpha).

## 3D

Attach Model and Shader together. Model keeps standard material colors, roughness,
textures and alpha mode; Shader supplies the program and per-element Custom to
all model material slots.

```json
{
  "space": "world",
  "elements": [
    {
      "id": "cube",
      "components": {
        "model": {
          "id": "cube",
          "mesh": {"primitive": "cube", "size": 2.5},
          "material": {"baseColor": "#ffffff", "roughness": 0.32}
        },
        "shader": {"wgslFragment": "shaders/effect.frag.wgsl"}
      }
    }
  ]
}
```

Update scene.Shader[graphics.MeshShader].Custom, or compose it directly:

```go
shader, err := graphics.NewMeshShader(descriptor)
if err != nil {
    return err
}
ecs.Put(&bundle, scene.Shader[graphics.MeshShader]{
    Program: shader,
    Custom: [4][4]float32{{0.35}},
})
```

The component overrides program/Custom without mutating materials; removal restores
material rendering. render3d.Material[T] still defines surface materials; element
shaders require no material-type registration. Both programs use ShaderDescriptor.
Browser Web uses WebGPU; WeChat uses WebGL2.

### Vertex and instance ABI

| Location | Type | Input |
| --- | --- | --- |
| 0 | vec3 | Position |
| 1 | vec3 | Normal |
| 2 | vec2 | UV |
| 3–6 | vec4 | Model matrix columns |
| 7 | vec4 | Base color |
| 8 | vec4 | Emissive RGB multiplied by EmissiveStrength; metallic in W |
| 9 | vec4 | Roughness, IOR, alpha cutoff, material flags (low two bits: alpha mode) |
| 10 | vec4 | UV scale XY, offset ZW |
| 11–14 | vec4 | Material3DData.Custom[0:4] |
| 15 | vec4 | Tangent XYZ and handedness W; zero W means unavailable |

Vertices are 48 bytes: position/normal/UV/tangent offsets 0/12/24/32. Instances are
192 bytes. Builtin vertex output location 10 is the world tangent and handedness,
corrected for mirrored model/skin transforms. Custom vertex stages paired with
builtin fragments must provide it (zero if unavailable); custom fragments may
ignore it. Rebuild assets and shaders using an older layout.

Material varying location 5 is vec4<f32> with @interpolate(flat) on both stages
(flat on GLSL/Metal). W contains integer flags that interpolation would corrupt.
Material.y is actual IOR: default 1.5, zero for infinite IOR. Reflectance was removed.
EmissiveStrength multiplies RGB during packing; custom Material3DData producers
must explicitly provide it. It consumes no new uniform/texture/Custom slot.

Builtin StandardMaterial Custom layout (custom shaders retain all caller values):

| Group | XYZW |
| --- | --- |
| Custom[0] | normal scale, AO strength, specular factor, clearcoat factor |
| Custom[1] | specular color RGB, clearcoat roughness |
| Custom[2] | sheen color RGB, sheen roughness |
| Custom[3] | anisotropy strength, cos(rotation), sin(rotation), clearcoat normal scale |

### Backend ABI

Generated Vulkan shaders use main, translate WGSL group 0/binding 0 uniform into
push constants, and use descriptor set 0/binding 1–2 for texture/sampler. Builtins
retain a combined image sampler at binding 0. Metal exports mesh_vertex and/or
mesh_fragment, view buffer 2 and material texture 0. WebGL2 uses main, Naga immediate
uniform names and _group_0_binding_1_vs/_fs samplers; preview also updates builtin
uniform names. WebGPU uses mesh entry points and this 3584-byte group 0/binding 0
builtin mesh uniform:

| Field | Offset (bytes) | Size (bytes) |
| --- | ---: | ---: |
| projection, cameraPosition, ambientLight, lightDirection, directionalLight | 0 | 128 |
| lightPosition, lightDirections, lightColors, lightParams (16 vec4 each) | 128 | 1024 |
| lightMeta | 1152 | 16 |
| environment, renderParams, cameraForward | 1168 | 48 |
| lightRight, lightUp (16 vec4 each) | 1216 | 512 |
| shadowMatrices (16 mat4) | 1728 | 1024 |
| shadowRects (16 vec4) | 2752 | 256 |
| shadowParams (16 vec4) | 3008 | 256 |
| lightShadows (16 vec4) | 3264 | 256 |
| occlusionInverseProjection (mat4) | 3520 | 64 |

shadowRects stores atlas UV origin/size; shadowParams stores depth bias, normal
bias and cascade near/far. lightShadows.xyz stores first tile (1-based, zero means
none), count and cascade blend. The first eight W values hold:

| Component | Camera parameter |
| --- | --- |
| lightShadows[0..2].w | Linear fog RGB multiplied by brightness in nits |
| lightShadows[3].w | Fog start in meters |
| lightShadows[4].w | Fog end in meters, linear mode only |
| lightShadows[5].w | Fog density per meter, exp2 only |
| lightShadows[6].w | Fog mode: 0 off, 1 linear, 2 exp2 |
| lightShadows[7].w | Tone mapping: 0 Reinhard, 1 ACES filmic fit |

cameraForward.xyz is world forward. cameraPosition.w is -1 for orthographic
(view direction -cameraForward.xyz), 1 for perspective. Directional colors include
lux; local intensities are candela/nits, or lux for box lights. renderParams is
exposure multiplier, MeshPass, SSAO strength and backend winding flip.

Custom fragments apply fog/tone mapping explicitly, following pbr_fog_factor then
pbr_tone_map. SSR radiance capture skips both; sky skips fog; FXAA filters display
colors. These reuse reserved components without expanding the ABI.

SSAO uses lightMeta.zw for world radius/bias, cameraForward.w for 8/16/32 samples,
and renderParams.z for strength (zero skips sampling). occlusionInverseProjection
is the original reversed-Z inverse view-projection, independent of backend clip/Y
adjustments. Depth is RGB24, alpha 1 for geometry and 0 for clear. Flag 32768 excludes
SSAO; alpha blending also excludes it. Rebuild older uniform consumers.

Passes are 0 lit, 1 shadow, 2 AO depth, 3 SSR RGBE capture, 4 SSR resolve, 5 SSR depth,
6 SSR AO capture, 7 FXAA. SSR resolve binds radiance at shadowTex and depth at
screenDepthTex, uses inverse camera-local projection, shadowMatrices[0] for local
view-projection, shadowParams[0] for distance/thickness/strength/maxRoughness, and
shadowRects[0] for steps/edgeFade/hasAOTile/reserved. Flag 4194304 excludes SSR.
Negative material.y in capture produces invalid radiance (alpha 0), allowing custom
occluders to fall back to IBL. Custom shaders render in the normal pass with their
original shadow/AO data. No extra ABI fields or samplers are introduced.

WebGPU texture/sampler bindings: 1/2 base color; 3/4 normal; 5 deformation storage;
6/7 metallic-roughness; 8/9 AO; 10/11 emissive; 12/13 specular strength; 14/15 specular
color; 16/17 environment; 18/19 shadows; 20/21 AO depth; 22/23 clearcoat; 24/25 clearcoat
roughness; 26/27 clearcoat normal; 28/29 sheen color; 30/31 sheen roughness; 32/33
anisotropy.

Builtin Vulkan uses set 2/binding 1 for the same uniform and binding 0 for deformation.
Set 3/bindings 0–13 hold MR, AO, emissive, specular, specular color, environment,
shadow, AO depth and six extension textures. Base/normal use sets 0/1. Metal uses
buffer 2 and texture/samplers 0–15; GL follows that texture-unit order. Sixteen
textures fit GLES3/WebGPU minimum sampler limits.

MeshDraw3D.SurfaceTextures is AO, emissive, specular, specular color, then the six
extensions. ExtraTextures inserts environment/shadow/AO after the first four.
Extension flags are 65536, 131072, 262144, 524288, 1048576, 2097152. See
render/shaders/mesh.frag.wgsl and [lighting](render-lighting.md).

Fragment-only overrides retain vertex Custom outputs: Vulkan flat fragCustom0..3
at locations 6–9, WebGL2 flat vCustom0..3, Metal MeshOut.custom0..3. Opaque/Mask write
depth; Blend uses premultiplied alpha without depth writes. AlphaMode selects the pipeline.

## Resource lifecycle

Identical source paths within a module share compilation/GPU programs, with separate
Custom values per element. Module cleanup destroys programs before native source
data. Uniform updates and stable previews allocate no memory.

FXAA uses the builtin fullscreen mesh/fragment at pass 7. It reads final exposed,
tone-mapped SDR premultiplied RGBA from the base-color slot. Scene custom shaders
contribute to that source but are not called by the filter; the ABI stays unchanged.
