# 3D color, materials and environment lighting

StandardMaterial computes GGX lighting in linear space, then camera distance fog,
exposure, Reinhard (default) or ACES tone mapping, and the exact sRGB transfer
function for SDR output. HDR/Ultra HDR imports retain full-resolution skies and
separate prefiltered lighting atlases. Components live in scene/render3d: install
onix.Render3D, or call render3d.Register(world) when scheduling ECS systems directly.

## Color conventions

BaseColor, Emissive, SpecularColor and ECS light colors are linear. Convert sRGB/hex
code colors with ColorRGBA.Linear(). Project JSON color strings are converted by
the loader; glTF factors are already linear. Base/emissive/specular-color textures
use SRGB: true; GPU decoding affects RGB, while alpha remains linear. Normal,
metallic-roughness, AO, specular strength and environment atlases are data textures
in ordinary RGBA8. 2D defaults to RGBA8; direct drivers can request TextureFormatSRGBA8.

## Photometric units and exposure

World units are meters. Directional lights use lux; point/cone/pyramid lights enter
shaders as candela; area lights use nits. Linear white (1,1,1) is untinted.

| Component / shape | Fields | Units | Geometry |
| --- | --- | --- | --- |
| DirectionalLight | illuminance | lux | Local -Z, no distance falloff |
| PointLight | intensity, unit | lumen, candela, lux, ev100 | Omnidirectional |
| SpotLight / cone | intensity, unit | lumen, candela, lux, ev100 | Local -Z cone |
| SpotLight / pyramid | intensity, unit | lumen, candela, lux, ev100 | Rectangular section; aspectRatio controls proportions |
| SpotLight / box | intensity, unit | lux | Parallel width x height projection |
| AreaLight / rectangle | intensity, unit | lumen, nits, ev100 | width x height, one-sided local -Z |
| AreaLight / disc | intensity, unit | lumen, nits, ev100 | radius, one-sided local -Z |
| AreaLight / tube | intensity, unit | lumen, nits, ev100 | Local-X line, width is length, omnidirectional |

luxDistance defaults to 1 m; range must be positive. Explicit emitter dimensions
ignore node scaling. Spot angles are full angles in radians; convert degrees by
pi/180. Pyramid outerConeAngle describes the shorter side. Box lights attenuate
with depth only when rangeAttenuation is enabled.

Conversions follow [Unity LightUnitUtils](https://github.com/Unity-Technologies/Graphics/blob/master/Packages/com.unity.render-pipelines.core/Runtime/Utilities/LightUnitUtils.cs):

- Point: candela = lumen / (4*pi), or lux * luxDistance^2.
- Cone with reflector: candela = lumen / (2*pi*(1-cos(outerAngle/2))). With reflector
  disabled use the full sphere conversion; the cone still limits illumination.
- Rectangle: nits = lumen / (pi*width*height). Disc: lumen / (pi^2*radius^2).
- Tube: nits = lumen / (4*pi*length), following the HDRP line-source convention
  without a physical emitting surface area.
- Light EV100: candela or nits = 2^EV100 / 8. Negative EV is valid; EV 0 emits light.
  Hide the entity or set a linear intensity to zero to disable it.

Point/cone/pyramid lights use inverse-square falloff multiplied by
max(1-(distance/range)^4,0)^2. Radius limits near-field illuminance, with at least
1 cm protected distance. Cone angular weights are squared between inner and outer
angles, with a hard edge when equal. Area lights integrate GGX using 16 fixed samples
(4x4 rectangle, equal-area disc rings, line midpoints). No per-frame sample buffers
are allocated. This is not LTC: smooth materials and nearby emitters can show
integration error. Discs support real-time illumination here.

Camera fixed exposure defaults to EV100 13; an explicit fixed mode can express 0.
Physical EV100 = log2(aperture^2 / shutterSpeed * 100 / ISO).
Multiplier = 2^(compensation-EV100) / 1.2. One positive compensation stop doubles
radiance before tone mapping. Effective exposure is clamped to -32..64 EV100.
Use graphics.NewExposure() when constructing a camera.

Lights, emission, sky and IBL share a linear scale. Environment exposure is an
extra multiplier. AmbientLight.Brightness defaults to 800 nits as a uniform-environment
approximation. Uncalibrated HDR/PNG data needs intensity calibration; HDR does not
imply nits. Emissive RGB represents nits; SetEmission accepts LightNits/LightEV100
but does not illuminate other objects without light components.

The editor supports light creation/removal/reset, unit-preserving conversion, shape
outlines and exposure controls. Extraction reuses light arrays; each view holds at
most 16 records including ambient. The old point/spot illuminance field and half-angle
semantics were removed: use intensity/unit and double old half angles. Old directional
and local scale divisors (10000/1000) were removed. Rebuild typed resources and update
custom shader units/exposure. There are no area shadows, LTC, cookies, IES, volumetrics
or real-time GI, and matching Unity units does not promise matching pixels.

## Tone mapping and distance fog

Camera toneMapping accepts reinhard or aces. ACES uses the sRGB/AP1 matrices,
Stephen Hill RRT/ODT fit and exposure division by 0.6 used by
[three.js](https://github.com/mrdoob/three.js/blob/master/src/renderers/shaders/ShaderChunk/tonemapping_pars_fragment.glsl.js),
remaining finite at extreme HDR values. It is an SDR fit.

Add a camera fog object under resources/scenes to enable it; remove it to disable.
Defaults: linear mode, color #aabed2ff, brightness 800 nits, start 20 m, end 120 m,
density .02 per meter. Color converts sRGB to linear and multiplies brightness;
its alpha does not alter object opacity. Linear requires end > start >= 0 and uses
clamp((distance-start)/(end-start),0,1). Exp2 uses
1-exp(-(max(distance-start,0)*density)^2), ignores end and vanishes at zero density.
Parameters must be finite and nonnegative.

Perspective uses radial camera distance; orthographic uses axial forward distance.
PBR mixes radiance and fog before exposure/tone mapping/sRGB/premultiplication,
including mask/blend materials. Unlit retains its original color and mixes mapped
fog in display-linear space. Sky uses tone mapping without fog. UI, editor grids
and custom fragments do not automatically fog. SSR captures raw radiance so fog
applies once; FXAA follows final display conversion. Fog is constant-color distance
fog without height scattering or volumetric shadows. Inspector/preview settings
are camera-specific; quality levels do not toggle them. No additional pass, texture
or uniform bytes are needed. Rebuild older Camera3DSpec binary resources.

## glTF materials

NewStandardMaterial sets IOR 1.5 and EmissiveStrength 1. NewMaterialSurface defaults
normal/AO/specular strength and specular color to one. Explicit constructors must
supply these defaults themselves.

| Property | Input / handling |
| --- | --- |
| Base color | sRGB texture times linear RGBA factor |
| Metallic / roughness | Data channels B / G times factors |
| Normal | Tangent-space normal with XY scale; vertex tangent preferred, UV derivatives otherwise |
| Occlusion | R channel blended by strength, indirect light only |
| Emissive | sRGB texture times linear RGB and EmissiveStrength |
| KHR_materials_ior | Actual IOR >=1 or zero for infinite; default 1.5 |
| KHR_materials_emissive_strength | Finite nonnegative multiplier, default 1, applied before exposure |
| KHR_materials_specular | Alpha strength times factor; sRGB color times linear factor |
| KHR_materials_clearcoat | R factor, G roughness, separate normal/scale, fixed IOR 1.5 |
| KHR_materials_sheen | sRGB RGB times linear color, alpha roughness times factor |
| KHR_materials_anisotropy | Linear RG direction and B strength; rotation in radians |
| KHR_materials_unlit | Original color without lighting/tone mapping; mapped fog when enabled |
| Alpha | Opaque ignores alpha; Mask clips cutoff and writes opaque coverage; Blend premultiplies |
| Double sided | Flip back-face normals; single-sided culls; mirrored transforms correct winding |

Only TEXCOORD_0 is supported; other sets are rejected. Imported TANGENT preserves
mirrored UV handedness. Missing tangents accumulate UV tangent/bitangent contributions
at shared vertices; degenerate UVs use a geometric shader basis. Morph/skin deform
tangents; skin normals use inverse transpose, and mirrored joints correct handedness.
Fields survive typed binary preprocessing; rebuild after vertex/schema changes.

Clearcoat layers independent GGX above sheen/base, attenuating lower layers/emission
by view Fresnel. Sheen uses Charlie and integrated correlated Smith masking with
compiled directional-albedo tables. Anisotropic GGX uses principal alpha
mix(roughness^2,1,strength^2). All light types share these layered BRDFs. IBL uses
the existing GGX atlas: independent coat roughness, Charlie-albedo approximation
for sheen and bent-normal approximation for anisotropy, without dedicated convolutions.
These extensions cannot import alongside Unlit. Factors default off, coat normal
scale defaults one; base roughness has a .0525 floor plus geometric-derivative roughness.
See Khronos [clearcoat](https://github.com/KhronosGroup/glTF/tree/main/extensions/2.0/Khronos/KHR_materials_clearcoat),
[sheen](https://github.com/KhronosGroup/glTF/tree/main/extensions/2.0/Khronos/KHR_materials_sheen)
and [anisotropy](https://github.com/KhronosGroup/glTF/tree/main/extensions/2.0/Khronos/KHR_materials_anisotropy).

Dielectric F0 = ((IOR-1)/(IOR+1))^2; zero IOR means F0=1. Numerically use
1-2/(IOR+1) before squaring. Schlick handles angular Fresnel. Specular color multiplies
F0 and clamps to one before strength. Pure metals ignore dielectric IOR/specular;
coat remains IOR 1.5. Partial-metal diffuse attenuates by dielectric Fresnel and
(1-metallic), without applying mixed-metal specular twice. All direct/area/IBL/SSR
paths share these parameters. See Khronos [IOR](https://github.com/KhronosGroup/glTF/tree/main/extensions/2.0/Khronos/KHR_materials_ior)
and [emissive strength](https://github.com/KhronosGroup/glTF/tree/main/extensions/2.0/Khronos/KHR_materials_emissive_strength).
Neither imports with Unlit. SetEmission resets EmissiveStrength to one. Transmission,
volume and refraction are not implemented.

Manifest models[].material can override ior/emissiveStrength/emissive for all slots.
Omission retains imported values or procedural defaults; explicit zero survives
JSON, typed binaries and preview. Blank Inspector inputs clear overrides.
Bundle.ModelMaterials returns original import values. Reflectance was replaced
without aliases; old r maps to (1+.4*r)/(1-.4*r), so .5 maps to IOR 1.5. Update
custom location-9 consumers and rebuild resources.

## Sky and IBL

BakeEnvironment[S RadianceSource] accepts SkyRadiance or PanoramaRadiance. Radiance
codec hdr.Decode returns a four-byte-per-pixel RGBE source retaining values above one,
with allocation-free bilinear directional sampling. Cubemap import is not supported.
The atlas has even width and height four times width: eight 2:1 tiles hold six GGX
roughness levels, diffuse irradiance/pi and a split-sum BRDF LUT. The first seven
use RGBM in 0..16. Disable sRGB, premultiplication and mipmaps for baked atlases.

Camera environment paths are project-relative under resources. PNG loads RGBM;
2:1 HDR or gain-map JPEG imports preserve full-resolution RGBE background and bake
a 256x1024 atlas with 256 convolution samples. Content IDs share atlas/background
across files/cameras. Bundle owns cleanup; packaging includes referenced files and
skips texture compression to preserve gain maps. Cold decode/baking costs CPU;
there is no per-frame convolution.

Prefiltering builds solid-angle-weighted linear mips and chooses sample areas from
GGX PDFs. Diffuse and roughness-one use a nine-term spherical-harmonic approximation
of the common normalized cosine kernel, reducing missed bright texels without claiming
exact convolution. GGX multiscattering compensation, energy allocation and roughness^4
normal bias apply; results differ from three.js PMREM. Imported RGBE is decoded before
bilinear/roughness interpolation with samples clamped to each tile. Sky uses a separate
texture, wraps longitude and clamps latitude. Environment.Encoding selects RGBM/RGBE;
Panorama is RGBE and the LUT ordinary RGBA8. Output remains SDR.

Ultra HDR follows the [Android format specification](https://developer.android.com/media/platform/hdr-image-format):
full gain in native linear primaries, then linear sRGB conversion. Supports two-image
MPF, Adobe XMP scalar/RGB arrays, ISO 21496-1 single/RGB metadata, grayscale/RGB gain
maps and bilinear upsampling. RGB matrix/TRC ICC supports transfer/primaries/D50-to-D65.
Rejects SDR JPEG, malformed metadata, HDR primary images, ISO alternate gamut, LUT
ICC and MPF with more than two images. Limits: 16384 per side, 2^24 pixels per JPEG.
No display-headroom reduction or nonlinear three.js gain approximation is applied.

hdr.ImportEnvironment and ultrahdr.ImportEnvironment expose custom bake resolutions;
BakeEnvironment writes an offline atlas. Set RGBE Encoding explicitly for imported
atlases. A sharp background needs separate Panorama; PNG defaults to atlas level 0.
Radiance accepts raw RGBE, channel RLE, all eight orientations, LF/CRLF; rejects XYZE,
EXR and old pixel-repeat encoding. Limits: 16384 per side, 2^26 pixels. Primaries stay
unchanged; EXPOSURE/COLORCORR already represented in pixels are not reapplied. See
[Radiance formats](https://radsite.lbl.gov/radiance/refer/filefmts.pdf).

Intensity scales indirect light; rotation is Y-axis radians; exposure is an additional
post-camera multiplier, defaulting to one when omitted/zero. Background toggles sky
without disabling IBL. Exposure-only configuration may omit path if intensity=0
and background=false. Direct callers dispose atlas/panorama separately; loaded bundles
manage them. Stable sky/IBL frames reuse meshes/view buffers without allocation.

## Shadows

Adding shadows enables them; removing the field disables them. NewShadowSettings
defaults to one directional map. Resolution is a power of two 64..8192 per tile.
Single maps use radius as orthographic half-width, depth as coverage, following the
camera with texel snapping. Cascades 2..4 cover near through distance using a uniform/
logarithmic splitLambda, stable bounding spheres and texel snapping. Depth extends
caster coverage; radius is unused. CascadeBlend spans 0..0.5; overlapping frusta
blend and the last cascade fades at distance. Perspective/orthographic cameras work.

NewPunctualShadowSettings configures PointLight/SpotLight.Shadows. Points use six
90-degree faces chosen by dominant axis; cone/pyramid use perspective and box uses
orthographic projection. Require 0 < near < range. Inspector exposes parameters.
Each lit camera owns one atlas, at most 16 tiles: directional=cascade count,
point=6, spot=1. Allocate whole light groups in selected-light order; insufficient
budget skips shadows while retaining illumination. Atlas sides cap at 8192; reduce
requested resolutions together until they fit, minimum 64.

Reversed-Z RGB24 depth in RGBA8 uses bilinear 3x3 PCF: 16 comparison reads over a
shared 4x4 footprint, interpolating comparisons rather than encoded colors. Sampling
stays within tiles; point faces clamp without cross-face PCF. Receiver-plane gradients
reduce acne, and normalBias uses unperturbed geometric normals. Offscreen casters,
skin, morph and alpha mask work; blend/unlit/custom shaders do not cast. NoShadow
excludes both casting and receiving. No area-light shadows are available.

Color+depth typically cost 8 bytes/pixel: one 2048-square tile about 32 MiB, four
about 128 MiB, per camera. Targets/buffers persist across frames and release when
shadows turn off or cameras disappear. One model traversal selects main and shadow
views together. Recorded regressions cover culling, cascades, six faces, atlas
isolation, PCF and warm allocation reuse. Rebuild old uniform/binary resources.

## SSAO

Camera ssao defaults: radius .5 m, bias .025 m, strength 1, resolutionScale .5,
samples 16. Require radius > bias >=0, strength 0..4, scale .25..1, samples 8/16/32.
Zero strength disables the prepass and releases targets. Only depth resolution scales;
AO evaluates at lit pixels. Preview/Inspector follow the selected scene camera.
Each camera uses RGBA8+depth storing RGB24 reversed Z and coverage. Mask/skin/morph
work. Reconstruct neighbors in world space using geometric normals and a fixed spiral
kernel with angle/distance falloff; skip background/offscreen samples. Perspective
and rotated orthographic views work without normal attachments or random textures.

AO multiplies material AO on ambient/IBL only. Blend, unlit and custom shaders skip
the prepass; blend does not receive. NoAmbientOcclusion independently excludes both
casting/receiving. Half-resolution 1080p targets cost roughly 4 MiB. Main-view prepared
resources and buffers are reused; warm extraction/preparation/depth packing allocate
nothing. Disabling/removing releases targets; temporary picking/unlit retains caches.
Screen-hidden geometry, temporal accumulation, denoising, bent normals and GI are absent;
low samples/resolution can band or distort silhouettes.

## SSR

Camera ssr defaults: maxDistance 50 m, thickness .1 m, strength 1, maxRoughness .6,
edgeFade .1, steps 64. Zero camera settings disable it; zero strength releases targets.
Require maxDistance > thickness >0, strength 0..1, roughness (.045,1], edgeFade (0,.5],
and steps 32/64/128. Thickness also offsets origins; excessive values leak around edges.
Reflections fade over the last quarter of the roughness range.

Builtin opaque/mask materials trace base GGX, coat and sheen rays, with bent-normal
anisotropy. Perspective/orthographic/rotated cameras, subviewports/offscreen work.
NoScreenReflection disables receiving while preserving source visibility. Blends
render afterward as neither sources nor receivers. Unlit provides linear base-color
radiance while keeping its normal output.

RGBA8 RGBE captures raw radiance before exposure/tone mapping. Decode individual texels
before bilinear filtering. Hits replace the corresponding IBL specular with BRDF-weighted
radiance while preserving diffuse/direct/shadow/emission. Misses retain IBL. Without
an atlas an analytic split-sum approximation still supports visible lit/emissive sources.
With SSAO a second color tile preserves AO. Projected ray traversal uses perspective-
correct world positions and near/far/screen clipping, six binary refinements, then
thickness rejection. See [McGuire/Mara](https://jcgt.org/published/0003/04/04/).
There is no Hi-Z, temporal accumulation, denoising, rough convolution or multibounce;
hidden/offscreen surfaces fall back to IBL and low steps can miss thin geometry.
RGBE quantizes. Custom standard mesh/mask shapes occlude with invalid source color;
custom vertex displacement/clipping does not automatically update SSR depth.

## Quality and light selection

Camera quality inherits world render3d.Quality when empty. Unset world quality retains
custom upper limits. Presets limit enabled effects without enabling them or rewriting
source settings. Validate raw values before applying limits. Camera3DSpec changed;
rebuild precompiled resources.

| Preset | Light records incl. ambient | Shadow tiles | Max tile | Cascades | SSAO maximum | SSR maximum |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| low | 8 | 4 | 512 | 1 | Off | Off |
| medium | 12 | 8 | 1024 | 2 | 16 samples, half resolution | Off |
| high | 16 | 16 | 2048 | 4 | 32 samples, full resolution | 64 steps |
| unset | 16 | 16 | 8192 | 4 | 32 samples, full resolution | 128 steps |

Per-camera single-pass candidate selection filters RenderLayers/frustum, prioritizes
ambient/directional, then estimates local influence from brightness, photometric
intensity, size and distance, breaking ties by entity ID. Later nearby lights can
replace distant ones. This is fixed-size forward lighting, not clustered/tiled shading.
Shadow slots follow selected order. Low's four tiles cannot fit a six-face point
shadow. Clamping cascades to one still fits the full shadow distance. Atlas fitting
can further reduce resolution.

## FXAA

Set camera fxaa:true or Camera{FXAA:true}; default off. Each camera retains viewport
RGBA8+depth containing final opaque/custom/transparent objects, sky and ground after
shadow/AO/SSR work. A fullscreen directional filter processes SDR before UI, following
[NVIDIA integration guidance](https://developer.download.nvidia.com/assets/gamedev/files/sdk/11/FXAA_WhitePaper.pdf).
It implements local contrast/directional sampling rather than full FXAA 3.11 Quality.
There are no motion vectors/history; shimmering remains and detail can soften.

Premultiplied RGBA filtering includes alpha, smoothing black transparent edges.
Clamp sampling to texel centers. Clear=false cameras clear private sources transparent
then composite over existing output. Multiple cameras/subviews/offscreen work. Resize
rebuilds targets; disabling/removal releases them; picking retains but bypasses them.
Final composition copies color only, without scene depth; offscreen depth clears to
zero. Put external depth-sharing 3D draws in the camera scene stage or disable FXAA.
WebGPU MSAA and editor supersampling remain independent.

## Shader generation and verification

The builtin fragment source is render/shaders/mesh.frag.wgsl. The generator needs
naga and glslangValidator or glslc on PATH, updating GLES/WebGL2, Vulkan GLSL/SPIR-V
and Metal includes. WebGPU embeds WGSL. Custom shaders compile separately during
resource builds. Use bounded serial package checks on small machines. Pixel tests
use small offscreen targets; editor visual acceptance and Metal behavior are manual.
See [custom shader ABI](custom-shaders.md) and the [performance guide](performance.md).

## Configuration and command examples

```json
{
  "camera": {
    "exposure": { "mode": "fixed", "ev100": 13, "compensation": 0 }
  }
}
```

```json
{
  "camera": {
    "exposure": {
      "mode": "physical",
      "aperture": 16,
      "shutterSpeed": 0.008,
      "iso": 100,
      "compensation": 0
    }
  }
}
```

```json
{
  "nodes": [
    { "id": "camera", "z": 6, "components": { "camera": { "exposure": { "mode": "fixed", "ev100": 6 } } } },
    { "id": "sun", "rotationX": -0.8, "components": { "directionalLight": { "illuminance": 1000 } } },
    { "id": "bulb", "x": 2, "y": 2, "components": { "pointLight": { "intensity": 600, "unit": "lumen", "range": 10 } } },
    { "id": "spot", "z": 3, "components": { "spotLight": { "shape": "cone", "intensity": 600, "unit": "lumen", "outerConeAngle": 1.0471976, "reflector": true } } },
    { "id": "panel", "x": -2, "z": 2, "components": { "areaLight": { "shape": "rectangle", "intensity": 200, "unit": "nits", "width": 2, "height": 1 } } }
  ]
}
```

```json
{
  "camera": {
    "toneMapping": "aces",
    "fog": {
      "mode": "linear",
      "color": "#aabed2ff",
      "brightness": 800,
      "start": 20,
      "end": 120,
      "density": 0.02
    }
  }
}
```

```go
camera := render3d.Camera{
    Fog: render3d.NewFogSettings(),
    ToneMapping: graphics.ToneMappingACES,
}
camera.Fog.Mode = render3d.FogExp2
camera.Fog.Density = .02
```

```go
material := graphics.NewStandardMaterial(graphics.White)
material.Surface.ClearcoatFactor = 1
material.Surface.ClearcoatRoughness = .15
material.Surface.SheenColor = graphics.ColorRGBA{R: .3, G: .08, B: .04, A: 1}
material.Surface.SheenRoughness = .6
material.Surface.AnisotropyStrength = .7
material.Surface.AnisotropyRotation = .5 // radians
```

```go
material := graphics.NewStandardMaterial(graphics.White)
material.IOR = 2.42 // diamond; 1.33 for water
material.Emissive = graphics.ColorRGBA{R: .25, G: .5, B: 1, A: 1}
material.EmissiveStrength = 64 // final linear RGB: (16, 32, 64)
```

```go
atlas, err := graphics.BakeEnvironment(graphics.SkyRadiance{
	Zenith:  mgl32.Vec3{.12, .32, .65},
	Horizon: mgl32.Vec3{.8, .95, 1.1},
	Ground:  mgl32.Vec3{.10, .11, .12},
}, 256, 128)
if err != nil {
	return err
}
return png.Encode(output, atlas)
```

```json
{
  "environment": {
    "path": "resources/environment/daylight.png",
    "intensity": 6500,
    "rotation": 0,
    "exposure": 1.2,
    "background": true
  }
}
```

```go
import graphicshdr "github.com/diiyw/onix/graphics/codec/hdr"

images, err := graphicshdr.ImportEnvironment(data, 256, 256)
if err != nil {
	return err
}
options := graphics.TextureOptions{Usage: graphics.ResourceUsageDefault}
atlas := graphics.TextureFromOwnedNRGBA(images.Atlas, options)
panorama := graphics.TextureFromOwnedNRGBA(images.Panorama, options)
environment := render3d.Environment{
	Atlas: atlas, Panorama: panorama, Encoding: render.EnvironmentRGBE,
	Intensity: 1, Exposure: 1, Background: true,
}
```

```json
{
  "color": "#fff3dc",
  "illuminance": 30000,
  "shadows": {
    "resolution": 2048,
    "radius": 48,
    "depth": 180,
    "bias": 0.0003,
    "normalBias": 0.035,
    "cascades": 4,
    "distance": 120,
    "splitLambda": 0.6,
    "cascadeBlend": 0.1
  }
}
```

```json
{
  "pointLight": {
    "intensity": 600,
    "unit": "lumen",
    "range": 10,
    "shadows": { "resolution": 512, "near": 0.05, "bias": 0.0003, "normalBias": 0.01 }
  },
  "spotLight": {
    "shape": "cone",
    "intensity": 600,
    "unit": "lumen",
    "range": 15,
    "shadows": { "resolution": 1024, "near": 0.05 }
  }
}
```

```json
{
  "camera": {
    "ssao": {
      "radius": 0.5,
      "bias": 0.025,
      "strength": 1,
      "resolutionScale": 0.5,
      "samples": 16
    }
  }
}
```

```json
{
  "camera": {
    "ssr": {
      "maxDistance": 50,
      "thickness": 0.1,
      "strength": 1,
      "maxRoughness": 0.6,
      "edgeFade": 0.1,
      "steps": 64
    }
  }
}
```

```json
{
  "camera": {
    "quality": "medium",
    "fxaa": true,
    "ssao": { "samples": 16, "resolutionScale": 0.5 }
  }
}
```

```sh
python3 render/internal/shadergen/generate.py
```

```sh
GOMAXPROCS=2 GOMEMLIMIT=512MiB LP_NUM_THREADS=2 \
  go test -p 1 -parallel 1 -timeout 90s ./graphics ./ecs ./render
```
