struct PBRUniforms {
    row_major float4x4 projection;
    float4 cameraPosition;
    float4 ambientLight;
    float4 lightDirection;
    float4 directionalLight;
    float4 lightPosition[16];
    float4 lightDirections[16];
    float4 lightColors[16];
    float4 lightParams[16];
    float4 lightMeta;
    float4 environment;
    float4 renderParams;
    float4 cameraForward;
    float4 lightRight[16];
    float4 lightUp[16];
    row_major float4x4 shadowMatrices[16];
    float4 shadowRects[16];
    float4 shadowParams[16];
    float4 lightShadows[16];
    row_major float4x4 occlusionInverseProjection;
};

struct PBRInput {
    float4 position : SV_Position;
    bool front : SV_IsFrontFace;
    float3 worldPosition : LOC0;
    float3 normal : LOC1;
    float2 uv : LOC2;
    float4 baseColor : LOC3;
    float4 emissiveMetallic : LOC4;
    nointerpolation float4 material : LOC5;
    nointerpolation float4 custom0_ : LOC6;
    nointerpolation float4 custom1_ : LOC7;
    float4 tangent : LOC10;
};

struct PBRSurface {
    float3 n;
    int _pad1_0;
    float3 coatNormal;
    int _pad2_0;
    float3 tangent;
    int _pad3_0;
    float3 bitangent;
    int _pad4_0;
    float3 base;
    float metallic;
    float roughness;
    int _pad7_0;
    int _pad7_1;
    int _pad7_2;
    float3 f0_;
    float f90_;
    float diffuseF0_;
    float diffuseF90_;
    float coat;
    float coatRoughness;
    float coatFresnel;
    int _pad14_0;
    int _pad14_1;
    int _pad14_2;
    float3 sheen;
    float sheenRoughness;
    float sheenAlbedo;
    float sheenScale;
    float sheenG1_;
    float anisotropy;
};

typedef float2 ret_Constructarray32_float2_[32];
ret_Constructarray32_float2_ Constructarray32_float2_(float2 arg0, float2 arg1, float2 arg2, float2 arg3, float2 arg4, float2 arg5, float2 arg6, float2 arg7, float2 arg8, float2 arg9, float2 arg10, float2 arg11, float2 arg12, float2 arg13, float2 arg14, float2 arg15, float2 arg16, float2 arg17, float2 arg18, float2 arg19, float2 arg20, float2 arg21, float2 arg22, float2 arg23, float2 arg24, float2 arg25, float2 arg26, float2 arg27, float2 arg28, float2 arg29, float2 arg30, float2 arg31) {
    float2 ret[32] = { arg0, arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8, arg9, arg10, arg11, arg12, arg13, arg14, arg15, arg16, arg17, arg18, arg19, arg20, arg21, arg22, arg23, arg24, arg25, arg26, arg27, arg28, arg29, arg30, arg31 };
    return ret;
}

static const float PI = 3.1415927;
static const bool SHADOW_FLIP_Y = true;
static const float2 SSAO_DISK[32] = Constructarray32_float2_(float2(0.70710677, 0.0), float2(-0.90308875, 0.8273033), float2(0.1382322, -1.5750847), float2(1.1382849, 1.484691), float2(-2.0888927, -0.36949572), float2(1.9787816, -1.2587389), float2(-0.6618637, 2.4621), float2(-1.2622459, -2.4303777), float2(2.7385685, 1.0001209), float2(-2.8490243, 1.1760359), float2(1.373418, -2.9349146), float2(1.014921, 3.235728), float2(-3.0589836, -1.7727435), float2(3.588536, -0.7889296), float2(-2.1900277, 3.115089), float2(-0.5059471, -3.9043589), float2(3.106019, 2.6177561), float2(-4.179728, 0.17284487), float2(3.0487907, -3.033954), float2(-0.2039759, 4.411167), float2(-2.900934, -3.4762886), float2(4.5954, 0.61830455), float2(-3.893673, 2.7091162), float2(1.0639755, -4.7294774), float2(2.4609199, 4.294633), float2(-4.8108625, -1.5347965), float2(4.673141, -2.1591096), float2(-2.0245214, 4.8374906), float2(-1.8068408, -5.0234776), float2(4.8078094, 2.5268497), float2(-5.340266, 1.407677), float2(3.0354443, -4.7208133));

cbuffer pbr : register(b0) { PBRUniforms pbr; }
Texture2D<float4> tex : register(t0);
SamplerState texSampler : register(s0);
Texture2D<float4> normalTex : register(t1);
SamplerState normalSampler : register(s1);
Texture2D<float4> metallicRoughnessTex : register(t2);
SamplerState metallicRoughnessSampler : register(s2);
Texture2D<float4> occlusionTex : register(t3);
SamplerState occlusionSampler : register(s3);
Texture2D<float4> emissiveTex : register(t4);
SamplerState emissiveSampler : register(s4);
Texture2D<float4> specularTex : register(t5);
SamplerState specularSampler : register(s5);
Texture2D<float4> specularColorTex : register(t6);
SamplerState specularColorSampler : register(s6);
Texture2D<float4> environmentTex : register(t7);
SamplerState environmentSampler : register(s7);
Texture2D<float4> shadowTex : register(t8);
SamplerState shadowSampler : register(s8);
Texture2D<float4> screenDepthTex : register(t9);
SamplerState screenDepthSampler : register(s9);
Texture2D<float4> clearcoatTex : register(t10);
SamplerState clearcoatSampler : register(s10);
Texture2D<float4> clearcoatRoughnessTex : register(t11);
SamplerState clearcoatRoughnessSampler : register(s11);
Texture2D<float4> clearcoatNormalTex : register(t12);
SamplerState clearcoatNormalSampler : register(s12);
Texture2D<float4> sheenColorTex : register(t13);
SamplerState sheenColorSampler : register(s13);
Texture2D<float4> sheenRoughnessTex : register(t14);
SamplerState sheenRoughnessSampler : register(s14);
Texture2D<float4> anisotropyTex : register(t15);
SamplerState anisotropySampler : register(s15);

struct FragmentInput_mesh_fragment {
    float3 worldPosition : LOC0;
    float3 normal_4 : LOC1;
    float2 uv_2 : LOC2;
    float4 baseColor : LOC3;
    float4 emissiveMetallic : LOC4;
    nointerpolation float4 material : LOC5;
    nointerpolation float4 custom0_ : LOC6;
    nointerpolation float4 custom1_ : LOC7;
    nointerpolation float4 onix_unused_8 : LOC8;
    nointerpolation float4 onix_unused_9 : LOC9;
    float4 tangent : LOC10;
    float4 position_7 : SV_Position;
    bool front : SV_IsFrontFace;
};

float3 pbr_srgb(float3 linear_)
{
    float3 c = max(linear_, (0.0).xxx);
    return ((c <= (0.0031308).xxx) ? (12.92 * c) : ((1.055 * pow(c, (0.41666666).xxx)) - (0.055).xxx));
}

float3 pbr_tone_map(float3 linear_1)
{
    float _e4 = pbr.renderParams.x;
    float _e8 = pbr.renderParams.x;
    float exposure = ((_e8 <= 0.0) ? 1.0 : _e4);
    float3 hdr = clamp((linear_1 * exposure), (0.0).xxx, (1e20).xxx);
    float3 reinhard = (hdr / (hdr + (1.0).xxx));
    float _e27 = pbr.lightShadows[7].w;
    if ((_e27 < 0.5)) {
        return reinhard;
    }
    float3x3 inputMatrix = float3x3(float3(0.59719, 0.076, 0.0284), float3(0.35458, 0.90834, 0.13383), float3(0.04823, 0.01566, 0.83777));
    float3x3 outputMatrix = float3x3(float3(1.60475, -0.10208, -0.00327), float3(-0.53108, 1.10813, -0.07276), float3(-0.07367, -0.00605, 1.07602));
    float3 aces = mul((hdr / (0.6).xxx), inputMatrix);
    float3 reciprocal = ((1.0).xxx / (aces + (1.0).xxx));
    float3 normalized = (aces * reciprocal);
    float3 squared = (normalized * normalized);
    float3 crossTerm = (normalized * reciprocal);
    float3 fit = (((squared + (0.0245786 * crossTerm)) - ((9.0537e-05 * reciprocal) * reciprocal)) / (((0.983729 * squared) + (0.432951 * crossTerm)) + ((0.238081 * reciprocal) * reciprocal)));
    return clamp(mul(fit, outputMatrix), (0.0).xxx, (1.0).xxx);
}

float4 pbr_encode_radiance(float3 linear_2)
{
    float3 c_1 = max(linear_2, (0.0).xxx);
    float exponent = clamp(ceil(log2(max(max(max(c_1.x, c_1.y), c_1.z), 1e-30))), -120.0, 120.0);
    return float4((c_1 * exp2(-(exponent))), ((exponent + 128.0) / 255.0));
}

float4 pbr_output(float3 linear_3, float alpha)
{
    float _e5 = pbr.renderParams.y;
    if ((_e5 == 3.0)) {
        const float4 _e8 = pbr_encode_radiance(linear_3);
        return _e8;
    }
    const float3 _e9 = pbr_tone_map(linear_3);
    const float3 _e10 = pbr_srgb(_e9);
    return float4((_e10 * alpha), alpha);
}

float3 pbr_fog_color()
{
    float _e4 = pbr.lightShadows[0].w;
    float _e9 = pbr.lightShadows[1].w;
    float _e14 = pbr.lightShadows[2].w;
    return float3(_e4, _e9, _e14);
}

float pbr_fog_factor(float3 position)
{
    float mode = pbr.lightShadows[6].w;
    if ((mode < 0.5)) {
        return 0.0;
    }
    float4 _e11 = pbr.cameraPosition;
    float3 delta = (position - _e11.xyz);
    float4 _e17 = pbr.cameraForward;
    float _e25 = pbr.cameraPosition.w;
    float distance_ = ((_e25 < 0.0) ? max(dot(delta, _e17.xyz), 0.0) : length(delta));
    float start = pbr.lightShadows[3].w;
    if ((mode < 1.5)) {
        float _e41 = pbr.lightShadows[4].w;
        return clamp(((distance_ - start) / max((_e41 - start), 1e-06)), 0.0, 1.0);
    }
    float _e56 = pbr.lightShadows[5].w;
    float opticalDepth = (max((distance_ - start), 0.0) * _e56);
    return (1.0 - exp((-(opticalDepth) * opticalDepth)));
}

float4 pbr_surface_output(float3 linear_4, float alpha_1, float3 position_1)
{
    float _e6 = pbr.renderParams.y;
    if ((_e6 == 3.0)) {
        const float4 _e9 = pbr_encode_radiance(linear_4);
        return _e9;
    }
    const float3 _e10 = pbr_fog_color();
    const float _e11 = pbr_fog_factor(position_1);
    const float4 _e13 = pbr_output(lerp(linear_4, _e10, _e11), alpha_1);
    return _e13;
}

float4 pbr_decode_radiance(float4 encoded)
{
    return float4((encoded.xyz * exp2((round((encoded.w * 255.0)) - 128.0))), ((encoded.w > 0.0) ? 1.0 : 0.0));
}

float4 pbr_atlas(float2 uv, float level)
{
    float _e5 = pbr.environment.z;
    float _e9 = pbr.environment.w;
    float2 inset = float2(_e5, _e9);
    float2 local_21 = ((clamp(uv, (0.0).xx, (1.0).xx) * ((1.0).xx - (2.0 * inset))) + inset);
    float4 _e32 = environmentTex.SampleLevel(environmentSampler, float2(local_21.x, ((local_21.y + level) / 8.0)), 0.0);
    return _e32;
}

int naga_div(int lhs, int rhs) {
    return lhs / (((lhs == int(-2147483647 - 1) & rhs == -1) | (rhs == 0)) ? 1 : rhs);
}

int2 naga_f2i32(float2 value) {
    return int2(clamp(value, -2147483600.0, 2147483500.0));
}

int naga_f2i32(float value) {
    return int(clamp(value, -2147483600.0, 2147483500.0));
}

uint2 NagaDimensions2D(Texture2D<float4> tex)
{
    uint4 ret;
    tex.GetDimensions(0, ret.x, ret.y, ret.z);
    return ret.xy;
}

float3 pbr_rgbe_atlas(float2 uv_1, float level_1)
{
    int2 dims = int2(NagaDimensions2D(environmentTex));
    float _e8 = pbr.environment.z;
    float _e12 = pbr.environment.w;
    float2 inset_1 = float2(_e8, _e12);
    float2 local_22 = ((clamp(uv_1, (0.0).xx, (1.0).xx) * ((1.0).xx - (2.0 * inset_1))) + inset_1);
    float2 coord = ((float2(local_22.x, ((local_22.y + level_1) / 8.0)) * float2(dims)) - (0.5).xx);
    int2 base_1 = naga_f2i32(floor(coord));
    float2 weight_1 = frac(coord);
    int tileHeight = naga_div(dims.y, int(8));
    int2 first = int2(int(0), asint(asuint(naga_f2i32(level_1)) * asuint(tileHeight)));
    int2 last = int2(asint(asuint(dims.x) - asuint(int(1))), asint(asuint(asint(asuint(first.y) + asuint(tileHeight))) - asuint(int(1))));
    float4 _e58 = environmentTex.Load(int3(clamp(base_1, first, last), int(0)));
    const float4 _e59 = pbr_decode_radiance(_e58);
    float3 a = _e59.xyz;
    float4 _e68 = environmentTex.Load(int3(clamp(asint(asuint(base_1) + asuint(int2(int(1), int(0)))), first, last), int(0)));
    const float4 _e69 = pbr_decode_radiance(_e68);
    float3 b = _e69.xyz;
    float4 _e78 = environmentTex.Load(int3(clamp(asint(asuint(base_1) + asuint(int2(int(0), int(1)))), first, last), int(0)));
    const float4 _e79 = pbr_decode_radiance(_e78);
    float3 c_2 = _e79.xyz;
    float4 _e87 = environmentTex.Load(int3(clamp(asint(asuint(base_1) + asuint((int(1)).xx)), first, last), int(0)));
    const float4 _e88 = pbr_decode_radiance(_e87);
    float3 d = _e88.xyz;
    return lerp(lerp(a, b, weight_1.x), lerp(c_2, d, weight_1.x), weight_1.y);
}

float2 pbr_environment_uv(float3 direction)
{
    float _e11 = pbr.environment.y;
    return float2(frac((((atan2(direction.z, direction.x) / 6.2831855) + 0.5) + (_e11 / 6.2831855))), (acos(clamp(direction.y, -1.0, 1.0)) / PI));
}

float3 pbr_environment(float3 direction_1, float level_2)
{
    const float2 _e2 = pbr_environment_uv(direction_1);
    float _e7 = pbr.lightShadows[8].w;
    if ((_e7 > 0.5)) {
        float lower = floor(level_2);
        float upper = ceil(level_2);
        const float3 _e12 = pbr_rgbe_atlas(_e2, lower);
        if ((lower == upper)) {
            return _e12;
        }
        const float3 _e14 = pbr_rgbe_atlas(_e2, upper);
        return lerp(_e12, _e14, frac(level_2));
    }
    const float4 _e18 = pbr_atlas(_e2, floor(level_2));
    const float4 _e20 = pbr_atlas(_e2, ceil(level_2));
    return (lerp((_e18.xyz * _e18.w), (_e20.xyz * _e20.w), frac(level_2)) * 16.0);
}

float pbr_shadow_map(uint index_1, float3 position_2, float3 normal, float4x3 receiver)
{
    float2 gradient = (0.0).xx;
    bool local_11 = (bool)0;
    bool local_12 = (bool)0;
    bool local_13 = (bool)0;
    float visibility = 0.0;
    int y = int(0);
    int x = (int)0;

    float4 params = pbr.shadowParams[index_1];
    float4x4 _e11 = pbr.shadowMatrices[index_1];
    float4 clip_ = mul(float4((position_2 + (normal * params.y)), 1.0), _e11);
    if ((clip_.w <= 0.0)) {
        return 1.0;
    }
    float3 ndc = (clip_.xyz / (clip_.w).xxx);
    float2 localUV = float2(((ndc.x * 0.5) + 0.5), (SHADOW_FLIP_Y ? (0.5 - (ndc.y * 0.5)) : ((ndc.y * 0.5) + 0.5)));
    float4 rect = pbr.shadowRects[index_1];
    float2 origin = float2(rect.x, (SHADOW_FLIP_Y ? rect.y : ((1.0 - rect.y) - rect.w)));
    float2 uv_3 = (origin + (localUV * rect.zw));
    int2 dims_1 = int2(NagaDimensions2D(shadowTex));
    int2 tileMin = naga_f2i32(round((origin * float2(dims_1))));
    int2 tileMax = asint(asuint(asint(asuint(tileMin) + asuint(naga_f2i32(round((rect.zw * float2(dims_1))))))) - asuint((int(1)).xx));
    float2 texel = ((uv_3 * float2(dims_1)) - (0.5).xx);
    float2 base_2 = floor(texel);
    float2 fraction = (texel - base_2);
    float4x4 _e87 = pbr.shadowMatrices[index_1];
    float4 cx = mul(float4((receiver[0] + (params.y * receiver[2])), 0.0), _e87);
    float4x4 _e99 = pbr.shadowMatrices[index_1];
    float4 cy = mul(float4((receiver[1] + (params.y * receiver[3])), 0.0), _e99);
    float3 scale_1 = float3(((float2(dims_1) * rect.zw) * float2(0.5, -0.5)), 1.0);
    float3 dx = ((cx.xyz - (ndc * cx.w)) * scale_1);
    float3 dy = ((cy.xyz - (ndc * cy.w)) * scale_1);
    float determinant_ = ((dx.x * dy.y) - (dx.y * dy.x));
    if ((abs(determinant_) > 1e-08)) {
        gradient = (float2(((dy.y * dx.z) - (dx.y * dy.z)), ((dx.x * dy.z) - (dy.x * dx.z))) / (determinant_).xx);
    }
    if (!(any((localUV < (0.0).xx)))) {
        local_11 = any((localUV > (1.0).xx));
    } else {
        local_11 = true;
    }
    bool _e169 = local_11;
    if (!(_e169)) {
        local_12 = (ndc.z < 0.0);
    } else {
        local_12 = true;
    }
    bool _e177 = local_12;
    if (!(_e177)) {
        local_13 = (ndc.z > 1.0);
    } else {
        local_13 = true;
    }
    bool _e185 = local_13;
    if (_e185) {
        return 1.0;
    }
    float4 weightsX = float4((1.0 - fraction.x), 1.0, 1.0, fraction.x);
    float4 weightsY = float4((1.0 - fraction.y), 1.0, 1.0, fraction.y);
    uint2 loop_bound = uint2(4294967295u, 4294967295u);
    bool loop_init = true;
    [loop] while(true) {
        if (all(loop_bound == uint2(0u, 0u))) { break; }
        loop_bound -= uint2(loop_bound.y == 0u, 1u);
        if (!loop_init) {
            int _e251 = y;
            y = asint(asuint(_e251) + asuint(int(1)));
        }
        loop_init = false;
        int _e205 = y;
        if ((_e205 < int(4))) {
        } else {
            break;
        }
        {
            x = int(0);
            uint2 loop_bound_1 = uint2(4294967295u, 4294967295u);
            bool loop_init_1 = true;
            [loop] while(true) {
                if (all(loop_bound_1 == uint2(0u, 0u))) { break; }
                loop_bound_1 -= uint2(loop_bound_1.y == 0u, 1u);
                if (!loop_init_1) {
                    int _e248 = x;
                    x = asint(asuint(_e248) + asuint(int(1)));
                }
                loop_init_1 = false;
                int _e210 = x;
                if ((_e210 < int(4))) {
                } else {
                    break;
                }
                {
                    int _e214 = x;
                    int _e217 = y;
                    int2 pixel = clamp(asint(asuint(naga_f2i32(base_2)) + asuint(int2(asint(asuint(_e214) - asuint(int(1))), asint(asuint(_e217) - asuint(int(1)))))), tileMin, tileMax);
                    float4 _e225 = shadowTex.Load(int3(pixel, int(0)));
                    float depth = _e225.x;
                    float2 _e228 = gradient;
                    float receiverDepth = ((ndc.z + dot(_e228, (float2(pixel) - texel))) + params.x);
                    float _e235 = visibility;
                    int _e236 = x;
                    int _e238 = y;
                    visibility = (_e235 + ((weightsX[min(uint(_e236), 3u)] * weightsY[min(uint(_e238), 3u)]) * ((receiverDepth >= depth) ? 1.0 : 0.0)));
                }
            }
        }
    }
    float _e253 = visibility;
    return (_e253 / 9.0);
}

uint naga_f2u32(float value) {
    return uint(clamp(value, 0.0, 4294967000.0));
}

float pbr_light_shadow(uint light, float3 position_3, float3 normal_1, float4x3 receiver_1)
{
    uint face = (uint)0;
    bool local_14 = (bool)0;
    bool local_15 = (bool)0;
    uint cascade = 0u;
    float next = (float)0;
    bool local_16 = (bool)0;

    float4 mapping = pbr.lightShadows[light];
    if ((mapping.x < 0.5)) {
        return 1.0;
    }
    uint first_1 = (naga_f2u32(mapping.x) - 1u);
    uint count = naga_f2u32(mapping.y);
    if ((count == 1u)) {
        const float _e20 = pbr_shadow_map(first_1, position_3, normal_1, receiver_1);
        return _e20;
    }
    float _e25 = pbr.lightDirections[light].w;
    if ((_e25 == 2.0)) {
        float4 _e31 = pbr.lightPosition[light];
        float3 delta_1 = (position_3 - _e31.xyz);
        float3 axis = abs(delta_1);
        face = ((delta_1.x >= 0.0) ? 0u : 1u);
        if ((axis.y > axis.x)) {
            local_14 = (axis.y >= axis.z);
        } else {
            local_14 = false;
        }
        bool _e51 = local_14;
        if (_e51) {
            face = ((delta_1.y >= 0.0) ? 2u : 3u);
        } else {
            if ((axis.z > axis.x)) {
                local_15 = (axis.z > axis.y);
            } else {
                local_15 = false;
            }
            bool _e67 = local_15;
            if (_e67) {
                face = ((delta_1.z >= 0.0) ? 4u : 5u);
            }
        }
        uint _e74 = face;
        const float _e76 = pbr_shadow_map((first_1 + _e74), position_3, normal_1, receiver_1);
        return _e76;
    }
    float4 _e79 = pbr.cameraPosition;
    float4 _e84 = pbr.cameraForward;
    float distance_1 = dot((position_3 - _e79.xyz), _e84.xyz);
    float _e91 = pbr.shadowParams[first_1].z;
    if ((distance_1 < _e91)) {
        return 1.0;
    }
    uint2 loop_bound_2 = uint2(4294967295u, 4294967295u);
    bool loop_init_2 = true;
    [loop] while(true) {
        if (all(loop_bound_2 == uint2(0u, 0u))) { break; }
        loop_bound_2 -= uint2(loop_bound_2.y == 0u, 1u);
        if (!loop_init_2) {
            uint _e139 = cascade;
            cascade = (_e139 + 1u);
        }
        loop_init_2 = false;
        uint _e96 = cascade;
        if ((_e96 < 4u)) {
        } else {
            break;
        }
        {
            uint _e99 = cascade;
            if ((_e99 >= count)) {
                break;
            }
            uint _e101 = cascade;
            uint index_3 = (first_1 + _e101);
            float4 params_1 = pbr.shadowParams[index_3];
            if ((distance_1 <= params_1.w)) {
                const float _e109 = pbr_shadow_map(index_3, position_3, normal_1, receiver_1);
                float blendWidth = ((params_1.w - params_1.z) * mapping.z);
                if ((blendWidth <= 0.0)) {
                    return _e109;
                }
                float blend = smoothstep((params_1.w - blendWidth), params_1.w, distance_1);
                next = 1.0;
                uint _e123 = cascade;
                if (((_e123 + 1u) < count)) {
                    local_16 = (blend > 0.0);
                } else {
                    local_16 = false;
                }
                bool _e132 = local_16;
                if (_e132) {
                    const float _e135 = pbr_shadow_map((index_3 + 1u), position_3, normal_1, receiver_1);
                    next = _e135;
                }
                float _e136 = next;
                return lerp(_e109, _e136, blend);
            }
        }
    }
    return 1.0;
}

float3 pbr_occlusion_clip(float3 position_4)
{
    float4x4 m = pbr.occlusionInverseProjection;
    float3 a_1 = (m[0].xyz - (position_4 * m[0].w));
    float3 b_1 = (m[1].xyz - (position_4 * m[1].w));
    float3 c_3 = (m[2].xyz - (position_4 * m[2].w));
    float3 d_1 = ((position_4 * m[3].w) - m[3].xyz);
    float determinant_1 = dot(a_1, cross(b_1, c_3));
    return (float3(dot(d_1, cross(b_1, c_3)), dot(a_1, cross(d_1, c_3)), dot(a_1, cross(b_1, d_1))) / (determinant_1).xxx);
}

float pbr_screen_occlusion(float3 position_5, float3 normal_2)
{
    bool local_17 = (bool)0;
    float blocked = 0.0;
    uint sampleIndex = 0u;
    bool local_18 = (bool)0;

    float radius_1 = pbr.lightMeta.z;
    float strength = pbr.renderParams.z;
    if (!((strength <= 0.0))) {
        local_17 = (radius_1 <= 0.0);
    } else {
        local_17 = true;
    }
    bool _e18 = local_17;
    if (_e18) {
        return 1.0;
    }
    float4x4 m_1 = pbr.occlusionInverseProjection;
    const float3 _e23 = pbr_occlusion_clip(position_5);
    float2 uv_4 = float2(((_e23.x * 0.5) + 0.5), (SHADOW_FLIP_Y ? (0.5 - (_e23.y * 0.5)) : ((_e23.y * 0.5) + 0.5)));
    float2 dims_2 = float2(NagaDimensions2D(screenDepthTex));
    float4 homogeneous = mul(float4(_e23, 1.0), m_1);
    float2 axes = float2(length((m_1[0].xyz - (position_5 * m_1[0].w))), length((m_1[1].xyz - (position_5 * m_1[1].w))));
    float2 denominator = (axes - (radius_1 * float2(m_1[0].w, m_1[1].w)));
    float2 extent = max((abs((((radius_1 * homogeneous.w)).xx / denominator)) * 0.5), ((1.0).xx / dims_2));
    float _e84 = pbr.cameraForward.w;
    uint count_1 = naga_f2u32(clamp(_e84, 8.0, 32.0));
    float diskScale = rsqrt(float(count_1));
    uint2 loop_bound_3 = uint2(4294967295u, 4294967295u);
    bool loop_init_3 = true;
    [loop] while(true) {
        if (all(loop_bound_3 == uint2(0u, 0u))) { break; }
        loop_bound_3 -= uint2(loop_bound_3.y == 0u, 1u);
        if (!loop_init_3) {
            uint _e183 = sampleIndex;
            sampleIndex = (_e183 + 1u);
        }
        loop_init_3 = false;
        uint _e95 = sampleIndex;
        if ((_e95 < 32u)) {
        } else {
            break;
        }
        {
            uint _e98 = sampleIndex;
            if ((_e98 >= count_1)) {
                break;
            }
            uint _e101 = sampleIndex;
            float2 sampleUV = (uv_4 + ((SSAO_DISK[min(uint(_e101), 31u)] * diskScale) * extent));
            if (!(any((sampleUV < (0.0).xx)))) {
                local_18 = any((sampleUV >= (1.0).xx));
            } else {
                local_18 = true;
            }
            bool _e118 = local_18;
            if (_e118) {
                continue;
            }
            int2 pixel_1 = naga_f2i32((sampleUV * dims_2));
            float4 _e123 = screenDepthTex.Load(int3(pixel_1, int(0)));
            float depth_1 = _e123.x;
            if ((depth_1 <= 0.0)) {
                continue;
            }
            float2 center = ((float2(pixel_1) + (0.5).xx) / dims_2);
            float3 ndc_1 = float3(((center.x * 2.0) - 1.0), (SHADOW_FLIP_Y ? (1.0 - (center.y * 2.0)) : ((center.y * 2.0) - 1.0)), depth_1);
            float4 world = mul(float4(ndc_1, 1.0), m_1);
            if ((abs(world.w) < 1e-08)) {
                continue;
            }
            float3 delta_2 = ((world.xyz / (world.w).xxx) - position_5);
            float distance_2 = length(delta_2);
            float _e167 = pbr.lightMeta.w;
            float horizon = (max((dot(normal_2, delta_2) - _e167), 0.0) / max(distance_2, 1e-06));
            float _e174 = blocked;
            blocked = (_e174 + (horizon * (1.0 - smoothstep((radius_1 * 0.5), radius_1, distance_2))));
        }
    }
    float _e187 = blocked;
    return clamp((1.0 - (((strength * 2.0) * _e187) / float(count_1))), 0.0, 1.0);
}

float3 pbr_fresnel(float cosine, float3 f0_, float f90_)
{
    return (f0_ + (((f90_).xxx - f0_) * pow((1.0 - clamp(cosine, 0.0, 1.0)), 5.0)));
}

float pbr_ggx(float nl, float nv, float nh, float roughness_1)
{
    float a_2 = (roughness_1 * roughness_1);
    float a2_ = (a_2 * a_2);
    float denominator_1 = (((nh * nh) * (a2_ - 1.0)) + 1.0);
    float d_2 = (a2_ / max(((PI * denominator_1) * denominator_1), 1e-07));
    float gv = (nl * sqrt((((nv * nv) * (1.0 - a2_)) + a2_)));
    float maskingLight = (nv * sqrt((((nl * nl) * (1.0 - a2_)) + a2_)));
    return ((d_2 * 0.5) / max((gv + maskingLight), 0.0001));
}

float3 pbr_environment_reflection(float3 normal_3, float3 v, float roughness_2)
{
    float roughnessSquared = (roughness_2 * roughness_2);
    float3 direction_2 = normalize(lerp(reflect(-(v), normal_3), normal_3, (roughnessSquared * roughnessSquared)));
    const float3 _e11 = pbr_environment(direction_2, (roughness_2 * 5.0));
    return _e11;
}

float3 pbr_multiscattering(float3 f0_1, float f90_1, float2 brdf)
{
    float3 single = ((f0_1 * brdf.x) + ((f90_1 * brdf.y)).xxx);
    float missing = max(0.0, ((1.0 - brdf.x) - brdf.y));
    float3 average = (f0_1 + (((1.0).xxx - f0_1) / (21.0).xxx));
    return (((single * average) * missing) / max((0.0001).xxx, ((1.0).xxx - (average * missing))));
}

float3 pbr_brdf(PBRSurface s, float3 v_1, float3 l_1)
{
    float specular_1 = (float)0;
    float3 result = (float3)0;

    float nl_1 = max(dot(s.n, l_1), 0.0);
    float nv_1 = max(dot(s.n, v_1), 0.0001);
    float3 halfSum = (v_1 + l_1);
    float3 h = (halfSum / (max(length(halfSum), 0.0001)).xxx);
    float nh_1 = max(dot(s.n, h), 0.0);
    float vh = max(dot(v_1, h), 0.0);
    const float _e25 = pbr_ggx(nl_1, nv_1, nh_1, s.roughness);
    specular_1 = _e25;
    const float3 _e29 = pbr_fresnel(vh, s.f0_, s.f90_);
    float diffuseF = (s.diffuseF0_ + ((s.diffuseF90_ - s.diffuseF0_) * pow((1.0 - clamp(vh, 0.0, 1.0)), 5.0)));
    float3 diffuse = ((((1.0 - diffuseF) * (1.0 - s.metallic)) * s.base) / (3.1415927).xxx);
    float _e54 = specular_1;
    result = ((diffuse + (_e54 * _e29)) * nl_1);
    float3 _e59 = result;
    return _e59;
}

float3x3 pbr_basis(PBRInput input_1, float3 n_1)
{
    float3 t = (float3)0;
    float handedness = (float)0;
    bool local_19 = (bool)0;
    bool local_20 = (bool)0;

    t = (input_1.tangent.xyz - (n_1 * dot(n_1, input_1.tangent.xyz)));
    handedness = ((input_1.tangent.w >= 0.0) ? 1.0 : -1.0);
    if (!((abs(input_1.tangent.w) < 0.5))) {
        float3 _e26 = t;
        float3 _e27 = t;
        local_19 = (dot(_e26, _e27) < 1e-08);
    } else {
        local_19 = true;
    }
    bool _e32 = local_19;
    if (_e32) {
        float3 dp1_ = ddx(input_1.worldPosition);
        float3 dp2_ = ddy(input_1.worldPosition);
        float2 duv1_ = ddx(input_1.uv);
        float2 duv2_ = ddy(input_1.uv);
        float determinant_2 = ((duv1_.x * duv2_.y) - (duv1_.y * duv2_.x));
        float signUV = ((determinant_2 >= 0.0) ? 1.0 : -1.0);
        float3 raw = (((dp1_ * duv2_.y) - (dp2_ * duv1_.y)) * signUV);
        t = (raw - (n_1 * dot(n_1, raw)));
        float3 b_2 = (((dp2_ * duv1_.x) - (dp1_ * duv2_.x)) * signUV);
        float3 _e68 = t;
        handedness = ((dot(cross(n_1, _e68), b_2) >= 0.0) ? 1.0 : -1.0);
        float3 _e76 = t;
        float3 _e77 = t;
        if (!((dot(_e76, _e77) < 1e-08))) {
            local_20 = (abs(determinant_2) < 1e-08);
        } else {
            local_20 = true;
        }
        bool _e88 = local_20;
        if (_e88) {
            float3 axis_1 = ((abs(n_1.y) > 0.99) ? float3(1.0, 0.0, 0.0) : float3(0.0, 1.0, 0.0));
            t = cross(axis_1, n_1);
            handedness = 1.0;
        }
    }
    float3 _e104 = t;
    t = normalize(_e104);
    float3 _e106 = t;
    float3 _e107 = t;
    float _e109 = handedness;
    return float3x3(_e106, (cross(n_1, _e107) * _e109), n_1);
}

float3 pbr_mapped_normal(float3x3 frame, float3 mapped, float scale)
{
    float3 value = mul(float3((mapped.xy * scale), mapped.z), frame);
    return ((dot(value, value) > 1e-08) ? (value / (max(length(value), 1e-06)).xxx) : frame[2]);
}

float light_range_window(float distanceSquared, float range)
{
    float ratio = (distanceSquared / max((range * range), 1e-08));
    float window = max((1.0 - (ratio * ratio)), 0.0);
    return (window * window);
}

float light_distance_attenuation(float distanceSquared_1, float range_1, float radius)
{
    const float _e3 = light_range_window(distanceSquared_1, range_1);
    return (_e3 / max(distanceSquared_1, max((radius * radius), 0.0001)));
}

uint naga_mod(uint lhs, uint rhs) {
    return lhs % (rhs == 0u ? 1u : rhs);
}

uint naga_div(uint lhs, uint rhs) {
    return lhs / (rhs == 0u ? 1u : rhs);
}

float3 area_lighting(uint index_2, float3 position_6, PBRSurface s_1, float3 v_2)
{
    float3 result_1 = (0.0).xxx;
    uint sampleIndex_1 = 0u;
    float3 offset = (float3)0;
    float measure = (float)0;
    float weight = (float)0;

    float kind = pbr.lightDirections[index_2].w;
    float4 _e12 = pbr.lightPosition[index_2];
    float3 center_1 = _e12.xyz;
    float4 right = pbr.lightRight[index_2];
    float4 up = pbr.lightUp[index_2];
    float4 _e25 = pbr.lightDirections[index_2];
    float3 forward = _e25.xyz;
    uint2 loop_bound_4 = uint2(4294967295u, 4294967295u);
    bool loop_init_4 = true;
    [loop] while(true) {
        if (all(loop_bound_4 == uint2(0u, 0u))) { break; }
        loop_bound_4 -= uint2(loop_bound_4.y == 0u, 1u);
        if (!loop_init_4) {
            uint _e152 = sampleIndex_1;
            sampleIndex_1 = (_e152 + 1u);
        }
        loop_init_4 = false;
        uint _e32 = sampleIndex_1;
        if ((_e32 < 16u)) {
        } else {
            break;
        }
        {
            offset = (0.0).xxx;
            measure = ((right.w * up.w) / 16.0);
            if ((kind == 6.0)) {
                uint _e46 = sampleIndex_1;
                uint _e50 = sampleIndex_1;
                float2 uv_5 = (((float2(float(naga_mod(_e46, 4u)), float(naga_div(_e50, 4u))) + (0.5).xx) / (4.0).xx) - (0.5).xx);
                offset = ((right.xyz * (uv_5.x * right.w)) + (up.xyz * (uv_5.y * up.w)));
            } else {
                if ((kind == 7.0)) {
                    uint _e77 = sampleIndex_1;
                    float radius_2 = (sqrt(((float(_e77) + 0.5) / 16.0)) * right.w);
                    uint _e86 = sampleIndex_1;
                    float angle_1 = (float(_e86) * 2.3999631);
                    offset = (radius_2 * ((cos(angle_1) * right.xyz) + (sin(angle_1) * up.xyz)));
                    measure = (((PI * right.w) * right.w) / 16.0);
                } else {
                    uint _e106 = sampleIndex_1;
                    offset = (right.xyz * ((((float(_e106) + 0.5) / 16.0) - 0.5) * right.w));
                    measure = (right.w / 16.0);
                }
            }
            float3 _e120 = offset;
            float3 delta_3 = ((center_1 + _e120) - position_6);
            float distanceSquared_2 = dot(delta_3, delta_3);
            float3 l_2 = (delta_3 * rsqrt(max(distanceSquared_2, 1e-08)));
            float _e128 = measure;
            float _e133 = pbr.lightParams[index_2].x;
            const float _e135 = light_distance_attenuation(distanceSquared_2, _e133, 0.0);
            weight = (_e128 * _e135);
            if ((kind != 8.0)) {
                float _e140 = weight;
                weight = (_e140 * max(dot(forward, -(l_2)), 0.0));
            }
            float3 _e146 = result_1;
            const float3 _e147 = pbr_brdf(s_1, v_2, l_2);
            float _e148 = weight;
            result_1 = (_e146 + (_e147 * _e148));
        }
    }
    float3 _e154 = result_1;
    float4 _e158 = pbr.lightColors[index_2];
    float _e165 = pbr.lightParams[index_2].y;
    return ((_e154 * _e158.xyz) * _e165);
}

PBRSurface ConstructPBRSurface(float3 arg0, float3 arg1, float3 arg2, float3 arg3, float3 arg4, float arg5, float arg6, float3 arg7, float arg8, float arg9, float arg10, float arg11, float arg12, float arg13, float3 arg14, float arg15, float arg16, float arg17, float arg18, float arg19) {
    PBRSurface ret = (PBRSurface)0;
    ret.n = arg0;
    ret.coatNormal = arg1;
    ret.tangent = arg2;
    ret.bitangent = arg3;
    ret.base = arg4;
    ret.metallic = arg5;
    ret.roughness = arg6;
    ret.f0_ = arg7;
    ret.f90_ = arg8;
    ret.diffuseF0_ = arg9;
    ret.diffuseF90_ = arg10;
    ret.coat = arg11;
    ret.coatRoughness = arg12;
    ret.coatFresnel = arg13;
    ret.sheen = arg14;
    ret.sheenRoughness = arg15;
    ret.sheenAlbedo = arg16;
    ret.sheenScale = arg17;
    ret.sheenG1_ = arg18;
    ret.anisotropy = arg19;
    return ret;
}

float4 mesh_fragment(FragmentInput_mesh_fragment fragmentinput_mesh_fragment) : SV_Target0
{
    PBRInput input = { fragmentinput_mesh_fragment.position_7, fragmentinput_mesh_fragment.front, fragmentinput_mesh_fragment.worldPosition, fragmentinput_mesh_fragment.normal_4, fragmentinput_mesh_fragment.uv_2, fragmentinput_mesh_fragment.baseColor, fragmentinput_mesh_fragment.emissiveMetallic, fragmentinput_mesh_fragment.material, fragmentinput_mesh_fragment.custom0_, fragmentinput_mesh_fragment.custom1_, fragmentinput_mesh_fragment.tangent };
    float4 base = (float4)0;
    bool local = (bool)0;
    bool local_1 = (bool)0;
    bool local_2 = (bool)0;
    float metallic = (float)0;
    float roughness = (float)0;
    float3 n = (float3)0;
    bool local_3 = (bool)0;
    float ao = 1.0;
    bool local_4 = (bool)0;
    bool local_5 = (bool)0;
    float specular = (float)0;
    float3 specColor = (float3)0;
    float3 emission = (float3)0;
    float3 lit_ = (float3)0;
    float3 indirect = (float3)0;
    float shadow = 1.0;
    uint index = 0u;
    float3 l = (float3)0;
    float attenuation = (float)0;
    bool local_6 = (bool)0;
    bool local_7 = (bool)0;
    bool local_8 = (bool)0;
    float angle = (float)0;
    bool local_9 = (bool)0;
    bool local_10 = (bool)0;

    uint flags = naga_f2u32((input.material.w + 0.5));
    float4 _e9 = tex.Sample(texSampler, input.uv);
    base = (_e9 * input.baseColor);
    if (((flags & 3u) == 1u)) {
        float _e20 = base.w;
        local = (_e20 < input.material.z);
    } else {
        local = false;
    }
    bool _e25 = local;
    if (_e25) {
        discard;
    }
    float _e30 = pbr.renderParams.w;
    bool front_1 = ((input.front != (_e30 > 0.5)) != ((flags & 1024u) != 0u));
    if (((flags & 512u) == 0u)) {
        local_1 = !(front_1);
    } else {
        local_1 = false;
    }
    bool _e47 = local_1;
    if (_e47) {
        discard;
    }
    if (((flags & 3u) != 2u)) {
        base.w = 1.0;
    }
    float _e57 = pbr.renderParams.y;
    if ((_e57 == 3.0)) {
        local_2 = (input.material.y < 0.0);
    } else {
        local_2 = false;
    }
    bool _e67 = local_2;
    if (_e67) {
        return (0.0).xxxx;
    }
    metallic = input.emissiveMetallic.w;
    roughness = input.material.x;
    if (((flags & 8u) != 0u)) {
        float4 mr = metallicRoughnessTex.Sample(metallicRoughnessSampler, input.uv);
        float _e84 = metallic;
        metallic = (_e84 * mr.z);
        float _e87 = roughness;
        roughness = clamp((_e87 * mr.y), 0.045, 1.0);
    }
    float3 _e94 = ddx(input.normal);
    float3 _e97 = ddy(input.normal);
    float3 normalVariation = max(abs(_e94), abs(_e97));
    float geometryRoughness = max(max(normalVariation.x, normalVariation.y), normalVariation.z);
    float _e105 = roughness;
    roughness = min(1.0, (max(_e105, 0.0525) + geometryRoughness));
    n = normalize(input.normal);
    if (((flags & 512u) != 0u)) {
        local_3 = !(front_1);
    } else {
        local_3 = false;
    }
    bool _e122 = local_3;
    if (_e122) {
        float3 _e123 = n;
        n = -(_e123);
    }
    float3 shadowNormal = n;
    float3 _e127 = ddx(input.worldPosition);
    float3 _e129 = ddy(input.worldPosition);
    float3 _e130 = n;
    float3 _e131 = ddx(_e130);
    float3 _e132 = n;
    float3 _e133 = ddy(_e132);
    float4x3 receiver_2 = float4x3(_e127, _e129, _e131, _e133);
    if (((flags & 16u) != 0u)) {
        float4 _e144 = occlusionTex.Sample(occlusionSampler, input.uv);
        ao = lerp(1.0, _e144.x, input.custom0_.y);
    }
    float _e153 = pbr.renderParams.y;
    if ((_e153 != 4.0)) {
        local_4 = ((flags & 32768u) == 0u);
    } else {
        local_4 = false;
    }
    bool _e163 = local_4;
    if (_e163) {
        local_5 = ((flags & 3u) != 2u);
    } else {
        local_5 = false;
    }
    bool _e171 = local_5;
    if (_e171) {
        float _e172 = ao;
        const float _e174 = pbr_screen_occlusion(input.worldPosition, shadowNormal);
        ao = (_e172 * _e174);
    }
    float _e179 = pbr.renderParams.y;
    if ((_e179 == 6.0)) {
        float _e182 = ao;
        return float4(_e182, 0.0, 0.0, 1.0);
    }
    if (((flags & 4u) != 0u)) {
        float3 _e191 = n;
        const float3x3 _e192 = pbr_basis(input, _e191);
        float4 _e196 = normalTex.Sample(normalSampler, input.uv);
        const float3 _e205 = pbr_mapped_normal(_e192, ((_e196.xyz * 2.0) - (1.0).xxx), input.custom0_.x);
        n = _e205;
    }
    float4 _e208 = pbr.cameraPosition;
    float4 _e215 = pbr.cameraForward;
    float _e221 = pbr.cameraPosition.w;
    float3 v_3 = ((_e221 < 0.0) ? -(_e215.xyz) : normalize((_e208.xyz - input.worldPosition)));
    specular = input.custom0_.z;
    specColor = input.custom1_.xyz;
    if (((flags & 64u) != 0u)) {
        float _e235 = specular;
        float4 _e239 = specularTex.Sample(specularSampler, input.uv);
        specular = (_e235 * _e239.w);
    }
    if (((flags & 128u) != 0u)) {
        float3 _e246 = specColor;
        float4 _e250 = specularColorTex.Sample(specularColorSampler, input.uv);
        specColor = (_e246 * _e250.xyz);
    }
    float iorRatio = ((input.material.y == 0.0) ? 1.0 : (1.0 - (2.0 / (max(input.material.y, 1.0) + 1.0))));
    float3 _e271 = specColor;
    float _e276 = specular;
    float3 dielectricF0_ = (min((((iorRatio * iorRatio)).xxx * _e271), (1.0).xxx) * _e276);
    float diffuseF0_ = max(max(dielectricF0_.x, dielectricF0_.y), dielectricF0_.z);
    float4 _e283 = base;
    float _e285 = metallic;
    float3 f0_2 = lerp(dielectricF0_, _e283.xyz, _e285);
    float _e287 = specular;
    float _e288 = metallic;
    float f90_2 = lerp(_e287, 1.0, _e288);
    emission = input.emissiveMetallic.xyz;
    if (((flags & 32u) != 0u)) {
        float3 _e298 = emission;
        float4 _e302 = emissiveTex.Sample(emissiveSampler, input.uv);
        emission = (_e298 * _e302.xyz);
    }
    float3 _e305 = n;
    float nv_2 = max(dot(_e305, v_3), 0.0001);
    float3 _e311 = n;
    float3 _e312 = n;
    float4 _e317 = base;
    float _e319 = metallic;
    float _e320 = roughness;
    float _e321 = specular;
    PBRSurface surface = ConstructPBRSurface(_e311, _e312, (0.0).xxx, (0.0).xxx, _e317.xyz, _e319, _e320, f0_2, f90_2, diffuseF0_, _e321, 0.0, 1.0, 0.0, (0.0).xxx, 1.0, 0.0, 1.0, 0.0, 0.0);
    float _e333 = specular;
    float ambientF = (diffuseF0_ + ((_e333 - diffuseF0_) * pow((1.0 - clamp(nv_2, 0.0, 1.0)), 5.0)));
    float _e346 = metallic;
    float diffuseWeight = ((1.0 - ambientF) * (1.0 - _e346));
    float4 _e350 = base;
    float4 _e354 = pbr.ambientLight;
    float _e359 = ao;
    float3 _e361 = emission;
    lit_ = ((((((_e350.xyz * _e354.xyz) * diffuseWeight) * 1.0) * _e359) + _e361) * (1.0 - 0.0));
    float _e370 = pbr.environment.x;
    if ((_e370 > 0.0)) {
        float3 _e373 = n;
        const float3 _e375 = pbr_environment(_e373, 6.0);
        float3 reflectionNormal = n;
        float _e377 = roughness;
        const float3 _e378 = pbr_environment_reflection(reflectionNormal, v_3, _e377);
        float _e379 = roughness;
        const float4 _e382 = pbr_atlas(float2(nv_2, _e379), 7.0);
        float2 brdf_1 = _e382.xy;
        float3 dielectricSingle = ((dielectricF0_ * brdf_1.x) + ((f90_2 * brdf_1.y)).xxx);
        float4 _e390 = base;
        float3 metallicSingle = ((_e390.xyz * brdf_1.x) + ((f90_2 * brdf_1.y)).xxx);
        const float3 _e398 = pbr_multiscattering(dielectricF0_, f90_2, brdf_1);
        float4 _e399 = base;
        const float3 _e401 = pbr_multiscattering(_e399.xyz, f90_2, brdf_1);
        float _e402 = metallic;
        float3 single_1 = lerp(dielectricSingle, metallicSingle, _e402);
        float _e404 = metallic;
        float3 multiple = lerp(_e398, _e401, _e404);
        float4 _e406 = base;
        float _e408 = metallic;
        float3 diffuse_1 = ((_e406.xyz * (1.0 - _e408)) * max((0.0).xxx, (((1.0).xxx - dielectricSingle) - _e398)));
        indirect = (((_e378 * single_1) + (_e375 * (multiple + diffuse_1))) * 1.0);
        float3 _e426 = indirect;
        indirect = (_e426 * (1.0 - 0.0));
        float3 _e430 = lit_;
        float3 _e431 = indirect;
        float _e435 = pbr.environment.x;
        float _e437 = ao;
        lit_ = (_e430 + ((_e431 * _e435) * _e437));
    }
    float _e443 = pbr.lightMeta.x;
    if ((_e443 == 0.0)) {
        if (((flags & 4096u) == 0u)) {
            const float _e454 = pbr_light_shadow(0u, input.worldPosition, shadowNormal, receiver_2);
            shadow = _e454;
        }
        float3 _e455 = lit_;
        float4 _e458 = pbr.lightDirection;
        const float3 _e462 = pbr_brdf(surface, v_3, normalize(-(_e458.xyz)));
        float4 _e465 = pbr.directionalLight;
        float _e468 = shadow;
        lit_ = (_e455 + ((_e462 * _e465.xyz) * _e468));
    }
    uint2 loop_bound_5 = uint2(4294967295u, 4294967295u);
    bool loop_init_5 = true;
    [loop] while(true) {
        if (all(loop_bound_5 == uint2(0u, 0u))) { break; }
        loop_bound_5 -= uint2(loop_bound_5.y == 0u, 1u);
        if (!loop_init_5) {
            uint _e697 = index;
            index = (_e697 + 1u);
        }
        loop_init_5 = false;
        uint _e473 = index;
        if ((_e473 < 16u)) {
        } else {
            break;
        }
        {
            uint _e476 = index;
            float _e481 = pbr.lightMeta.x;
            if ((float(_e476) >= _e481)) {
                break;
            }
            uint _e485 = index;
            float kind_1 = pbr.lightDirections[_e485].w;
            if ((kind_1 == 0.0)) {
                continue;
            }
            if ((kind_1 >= 6.0)) {
                float3 _e493 = lit_;
                uint _e494 = index;
                const float3 _e496 = area_lighting(_e494, input.worldPosition, surface, v_3);
                lit_ = (_e493 + _e496);
                continue;
            }
            uint _e500 = index;
            float4 _e502 = pbr.lightDirections[_e500];
            l = -(_e502.xyz);
            attenuation = 1.0;
            if ((kind_1 >= 2.0)) {
                uint _e512 = index;
                float4 _e514 = pbr.lightPosition[_e512];
                float3 delta_4 = (_e514.xyz - input.worldPosition);
                float distanceSquared_3 = dot(delta_4, delta_4);
                uint _e521 = index;
                float range_2 = pbr.lightParams[_e521].x;
                uint _e527 = index;
                float4 _e529 = pbr.lightDirections[_e527];
                float3 forward_1 = _e529.xyz;
                uint _e533 = index;
                float4 right_1 = pbr.lightRight[_e533];
                uint _e538 = index;
                float4 up_1 = pbr.lightUp[_e538];
                if ((kind_1 == 5.0)) {
                    float depth_2 = dot(-(delta_4), forward_1);
                    if (!((depth_2 < 0.0))) {
                        local_6 = (depth_2 >= range_2);
                    } else {
                        local_6 = true;
                    }
                    bool _e552 = local_6;
                    if (!(_e552)) {
                        local_7 = (abs(dot(delta_4, right_1.xyz)) > right_1.w);
                    } else {
                        local_7 = true;
                    }
                    bool _e562 = local_7;
                    if (!(_e562)) {
                        local_8 = (abs(dot(delta_4, up_1.xyz)) > up_1.w);
                    } else {
                        local_8 = true;
                    }
                    bool _e572 = local_8;
                    if (_e572) {
                        continue;
                    }
                    l = -(forward_1);
                    uint _e576 = index;
                    float _e579 = pbr.lightParams[_e576].y;
                    attenuation = _e579;
                    uint _e582 = index;
                    float _e585 = pbr.lightParams[_e582].z;
                    if ((_e585 > 0.5)) {
                        float _e588 = attenuation;
                        const float _e590 = light_range_window((depth_2 * depth_2), range_2);
                        attenuation = (_e588 * _e590);
                    }
                } else {
                    if ((distanceSquared_3 >= (range_2 * range_2))) {
                        continue;
                    }
                    l = (delta_4 * rsqrt(max(distanceSquared_3, 1e-08)));
                    uint _e600 = index;
                    float _e603 = pbr.lightColors[_e600].w;
                    const float _e604 = light_distance_attenuation(distanceSquared_3, range_2, _e603);
                    uint _e607 = index;
                    float _e610 = pbr.lightParams[_e607].y;
                    attenuation = (_e604 * _e610);
                    if ((kind_1 == 3.0)) {
                        float3 _e614 = l;
                        float cone = dot(forward_1, -(_e614));
                        uint _e619 = index;
                        float inner = pbr.lightParams[_e619].z;
                        uint _e625 = index;
                        float outer = pbr.lightParams[_e625].w;
                        angle = ((cone >= outer) ? 1.0 : 0.0);
                        if ((inner > outer)) {
                            angle = clamp(((cone - outer) / (inner - outer)), 0.0, 1.0);
                        }
                        float _e641 = attenuation;
                        float _e642 = angle;
                        float _e643 = angle;
                        attenuation = (_e641 * (_e642 * _e643));
                    } else {
                        if ((kind_1 == 4.0)) {
                            float depth_3 = dot(-(delta_4), forward_1);
                            if (!((depth_3 <= 0.0))) {
                                local_9 = (abs(dot(delta_4, right_1.xyz)) > (depth_3 * right_1.w));
                            } else {
                                local_9 = true;
                            }
                            bool _e662 = local_9;
                            if (!(_e662)) {
                                local_10 = (abs(dot(delta_4, up_1.xyz)) > (depth_3 * up_1.w));
                            } else {
                                local_10 = true;
                            }
                            bool _e673 = local_10;
                            if (_e673) {
                                continue;
                            }
                        }
                    }
                }
            }
            if (((flags & 4096u) == 0u)) {
                float _e678 = attenuation;
                uint _e679 = index;
                const float _e681 = pbr_light_shadow(_e679, input.worldPosition, shadowNormal, receiver_2);
                attenuation = (_e678 * _e681);
            }
            float3 _e683 = lit_;
            float3 _e684 = l;
            const float3 _e685 = pbr_brdf(surface, v_3, _e684);
            uint _e688 = index;
            float4 _e690 = pbr.lightColors[_e688];
            float _e693 = attenuation;
            lit_ = (_e683 + ((_e685 * _e690.xyz) * _e693));
        }
    }
    float3 _e699 = lit_;
    float _e701 = base.w;
    const float4 _e703 = pbr_surface_output(_e699, _e701, input.worldPosition);
    return _e703;
}
