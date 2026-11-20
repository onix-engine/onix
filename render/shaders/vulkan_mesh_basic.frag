#version 450
// Generated basic variant from mesh.frag.wgsl.




struct PBRUniforms {
    mat4x4 projection;
    vec4 cameraPosition;
    vec4 ambientLight;
    vec4 lightDirection;
    vec4 directionalLight;
    vec4 lightPosition[16];
    vec4 lightDirections[16];
    vec4 lightColors[16];
    vec4 lightParams[16];
    vec4 lightMeta;
    vec4 environment;
    vec4 renderParams;
    vec4 cameraForward;
    vec4 lightRight[16];
    vec4 lightUp[16];
    mat4x4 shadowMatrices[16];
    vec4 shadowRects[16];
    vec4 shadowParams[16];
    vec4 lightShadows[16];
    mat4x4 occlusionInverseProjection;
};
struct PBRInput {
    vec4 position;
    bool front;
    vec3 worldPosition;
    vec3 normal;
    vec2 uv;
    vec4 baseColor;
    vec4 emissiveMetallic;
    vec4 material;
    vec4 custom0_;
    vec4 custom1_;
    vec4 tangent;
};
struct PBRSurface {
    vec3 n;
    vec3 coatNormal;
    vec3 tangent;
    vec3 bitangent;
    vec3 base;
    float metallic;
    float roughness;
    vec3 f0_;
    float f90_;
    float diffuseF0_;
    float diffuseF90_;
    float coat;
    float coatRoughness;
    float coatFresnel;
    vec3 sheen;
    float sheenRoughness;
    float sheenAlbedo;
    float sheenScale;
    float sheenG1_;
    float anisotropy;
};
const float PI = 3.1415927;
const bool SHADOW_FLIP_Y = true;
const vec2 SSAO_DISK[32] = vec2[32](vec2(0.70710677, 0.0), vec2(-0.90308875, 0.8273033), vec2(0.1382322, -1.5750847), vec2(1.1382849, 1.484691), vec2(-2.0888927, -0.36949572), vec2(1.9787816, -1.2587389), vec2(-0.6618637, 2.4621), vec2(-1.2622459, -2.4303777), vec2(2.7385685, 1.0001209), vec2(-2.8490243, 1.1760359), vec2(1.373418, -2.9349146), vec2(1.014921, 3.235728), vec2(-3.0589836, -1.7727435), vec2(3.588536, -0.7889296), vec2(-2.1900277, 3.115089), vec2(-0.5059471, -3.9043589), vec2(3.106019, 2.6177561), vec2(-4.179728, 0.17284487), vec2(3.0487907, -3.033954), vec2(-0.2039759, 4.411167), vec2(-2.900934, -3.4762886), vec2(4.5954, 0.61830455), vec2(-3.893673, 2.7091162), vec2(1.0639755, -4.7294774), vec2(2.4609199, 4.294633), vec2(-4.8108625, -1.5347965), vec2(4.673141, -2.1591096), vec2(-2.0245214, 4.8374906), vec2(-1.8068408, -5.0234776), vec2(4.8078094, 2.5268497), vec2(-5.340266, 1.407677), vec2(3.0354443, -4.7208133));

layout(set=2,binding=1,std140) uniform PBRUniforms_block_0Fragment { PBRUniforms _group_0_binding_0_fs; };

layout(set=0,binding=0) uniform sampler2D tex;

layout(set=1,binding=0) uniform sampler2D normalTex;

layout(set=3,binding=0) uniform sampler2D metallicRoughnessTex;

layout(set=3,binding=1) uniform sampler2D occlusionTex;

layout(set=3,binding=2) uniform sampler2D emissiveTex;

layout(set=3,binding=3) uniform sampler2D specularTex;

layout(set=3,binding=4) uniform sampler2D specularColorTex;

layout(set=3,binding=5) uniform sampler2D environmentTex;

layout(set=3,binding=6) uniform sampler2D shadowTex;

layout(set=3,binding=7) uniform sampler2D screenDepthTex;

layout(location=0) smooth in vec3 _vs2fs_location0;
layout(location=1) smooth in vec3 _vs2fs_location1;
layout(location=2) smooth in vec2 _vs2fs_location2;
layout(location=3) smooth in vec4 _vs2fs_location3;
layout(location=4) smooth in vec4 _vs2fs_location4;
layout(location=5) flat in vec4 _vs2fs_location5;
layout(location=6) flat in vec4 _vs2fs_location6;
layout(location=7) flat in vec4 _vs2fs_location7;
layout(location=10) smooth in vec4 _vs2fs_location10;
layout(location = 0) out vec4 _fs2p_location0;

vec3 pbr_srgb(vec3 linear) {
    vec3 c = max(linear, vec3(0.0));
    return mix(((1.055 * pow(c, vec3(0.41666666))) - vec3(0.055)), (12.92 * c), lessThanEqual(c, vec3(0.0031308)));
}

vec3 pbr_tone_map(vec3 linear_1) {
    float _e4 = _group_0_binding_0_fs.renderParams.x;
    float _e8 = _group_0_binding_0_fs.renderParams.x;
    float exposure = ((_e8 <= 0.0) ? 1.0 : _e4);
    vec3 hdr = clamp((linear_1 * exposure), vec3(0.0), vec3(1e20));
    vec3 reinhard = (hdr / (hdr + vec3(1.0)));
    float _e27 = _group_0_binding_0_fs.lightShadows[7].w;
    if ((_e27 < 0.5)) {
        return reinhard;
    }
    mat3x3 inputMatrix = mat3x3(vec3(0.59719, 0.076, 0.0284), vec3(0.35458, 0.90834, 0.13383), vec3(0.04823, 0.01566, 0.83777));
    mat3x3 outputMatrix = mat3x3(vec3(1.60475, -0.10208, -0.00327), vec3(-0.53108, 1.10813, -0.07276), vec3(-0.07367, -0.00605, 1.07602));
    vec3 aces = (inputMatrix * (hdr / vec3(0.6)));
    vec3 reciprocal = (vec3(1.0) / (aces + vec3(1.0)));
    vec3 normalized = (aces * reciprocal);
    vec3 squared = (normalized * normalized);
    vec3 crossTerm = (normalized * reciprocal);
    vec3 fit = (((squared + (0.0245786 * crossTerm)) - ((9.0537e-5 * reciprocal) * reciprocal)) / (((0.983729 * squared) + (0.432951 * crossTerm)) + ((0.238081 * reciprocal) * reciprocal)));
    return clamp((outputMatrix * fit), vec3(0.0), vec3(1.0));
}

vec4 pbr_encode_radiance(vec3 linear_2) {
    vec3 c_1 = max(linear_2, vec3(0.0));
    float exponent = clamp(ceil(log2(max(max(max(c_1.x, c_1.y), c_1.z), 1e-30))), -120.0, 120.0);
    return vec4((c_1 * exp2(-(exponent))), ((exponent + 128.0) / 255.0));
}

vec4 pbr_output(vec3 linear_3, float alpha) {
    float _e5 = _group_0_binding_0_fs.renderParams.y;
    if ((_e5 == 3.0)) {
        vec4 _e8 = pbr_encode_radiance(linear_3);
        return _e8;
    }
    vec3 _e9 = pbr_tone_map(linear_3);
    vec3 _e10 = pbr_srgb(_e9);
    return vec4((_e10 * alpha), alpha);
}

vec3 pbr_fog_color() {
    float _e4 = _group_0_binding_0_fs.lightShadows[0].w;
    float _e9 = _group_0_binding_0_fs.lightShadows[1].w;
    float _e14 = _group_0_binding_0_fs.lightShadows[2].w;
    return vec3(_e4, _e9, _e14);
}

float pbr_fog_factor(vec3 position) {
    float mode = _group_0_binding_0_fs.lightShadows[6].w;
    if ((mode < 0.5)) {
        return 0.0;
    }
    vec4 _e11 = _group_0_binding_0_fs.cameraPosition;
    vec3 delta = (position - _e11.xyz);
    vec4 _e17 = _group_0_binding_0_fs.cameraForward;
    float _e25 = _group_0_binding_0_fs.cameraPosition.w;
    float distance_ = ((_e25 < 0.0) ? max(dot(delta, _e17.xyz), 0.0) : length(delta));
    float start = _group_0_binding_0_fs.lightShadows[3].w;
    if ((mode < 1.5)) {
        float _e41 = _group_0_binding_0_fs.lightShadows[4].w;
        return clamp(((distance_ - start) / max((_e41 - start), 1e-6)), 0.0, 1.0);
    }
    float _e56 = _group_0_binding_0_fs.lightShadows[5].w;
    float opticalDepth = (max((distance_ - start), 0.0) * _e56);
    return (1.0 - exp((-(opticalDepth) * opticalDepth)));
}

vec4 pbr_surface_output(vec3 linear_4, float alpha_1, vec3 position_1) {
    float _e6 = _group_0_binding_0_fs.renderParams.y;
    if ((_e6 == 3.0)) {
        vec4 _e9 = pbr_encode_radiance(linear_4);
        return _e9;
    }
    vec3 _e10 = pbr_fog_color();
    float _e11 = pbr_fog_factor(position_1);
    vec4 _e13 = pbr_output(mix(linear_4, _e10, _e11), alpha_1);
    return _e13;
}

vec4 pbr_decode_radiance(vec4 encoded) {
    return vec4((encoded.xyz * exp2((roundEven((encoded.w * 255.0)) - 128.0))), ((encoded.w > 0.0) ? 1.0 : 0.0));
}

vec4 pbr_atlas(vec2 uv, float level) {
    float _e5 = _group_0_binding_0_fs.environment.z;
    float _e9 = _group_0_binding_0_fs.environment.w;
    vec2 inset = vec2(_e5, _e9);
    vec2 local_21 = ((clamp(uv, vec2(0.0), vec2(1.0)) * (vec2(1.0) - (2.0 * inset))) + inset);
    vec4 _e32 = textureLod(environmentTex, vec2(vec2(local_21.x, ((local_21.y + level) / 8.0))), 0.0);
    return _e32;
}

vec3 pbr_rgbe_atlas(vec2 uv_1, float level_1) {
    ivec2 dims = ivec2(uvec2(textureSize(environmentTex, 0).xy));
    float _e8 = _group_0_binding_0_fs.environment.z;
    float _e12 = _group_0_binding_0_fs.environment.w;
    vec2 inset_1 = vec2(_e8, _e12);
    vec2 local_22 = ((clamp(uv_1, vec2(0.0), vec2(1.0)) * (vec2(1.0) - (2.0 * inset_1))) + inset_1);
    vec2 coord = ((vec2(local_22.x, ((local_22.y + level_1) / 8.0)) * vec2(dims)) - vec2(0.5));
    ivec2 base_1 = ivec2(floor(coord));
    vec2 weight_1 = fract(coord);
    int tileHeight = (dims.y / 8);
    ivec2 first = ivec2(0, (int(level_1) * tileHeight));
    ivec2 last = ivec2((dims.x - 1), ((first.y + tileHeight) - 1));
    vec4 _e58 = texelFetch(environmentTex, min(max(base_1, first), last), 0);
    vec4 _e59 = pbr_decode_radiance(_e58);
    vec3 a = _e59.xyz;
    vec4 _e68 = texelFetch(environmentTex, min(max((base_1 + ivec2(1, 0)), first), last), 0);
    vec4 _e69 = pbr_decode_radiance(_e68);
    vec3 b = _e69.xyz;
    vec4 _e78 = texelFetch(environmentTex, min(max((base_1 + ivec2(0, 1)), first), last), 0);
    vec4 _e79 = pbr_decode_radiance(_e78);
    vec3 c_2 = _e79.xyz;
    vec4 _e87 = texelFetch(environmentTex, min(max((base_1 + ivec2(1)), first), last), 0);
    vec4 _e88 = pbr_decode_radiance(_e87);
    vec3 d = _e88.xyz;
    return mix(mix(a, b, weight_1.x), mix(c_2, d, weight_1.x), weight_1.y);
}

vec2 pbr_environment_uv(vec3 direction) {
    float _e11 = _group_0_binding_0_fs.environment.y;
    return vec2(fract((((atan(direction.z, direction.x) / 6.2831855) + 0.5) + (_e11 / 6.2831855))), (acos(clamp(direction.y, -1.0, 1.0)) / PI));
}

vec3 pbr_environment(vec3 direction_1, float level_2) {
    vec2 _e2 = pbr_environment_uv(direction_1);
    float _e7 = _group_0_binding_0_fs.lightShadows[8].w;
    if ((_e7 > 0.5)) {
        float lower = floor(level_2);
        float upper = ceil(level_2);
        vec3 _e12 = pbr_rgbe_atlas(_e2, lower);
        if ((lower == upper)) {
            return _e12;
        }
        vec3 _e14 = pbr_rgbe_atlas(_e2, upper);
        return mix(_e12, _e14, fract(level_2));
    }
    vec4 _e18 = pbr_atlas(_e2, floor(level_2));
    vec4 _e20 = pbr_atlas(_e2, ceil(level_2));
    return (mix((_e18.xyz * _e18.w), (_e20.xyz * _e20.w), fract(level_2)) * 16.0);
}

float pbr_shadow_map(uint index_1, vec3 position_2, vec3 normal, mat4x3 receiver) {
    vec2 gradient = vec2(0.0);
    bool local_11 = false;
    bool local_12 = false;
    bool local_13 = false;
    float visibility = 0.0;
    int y = 0;
    int x = 0;
    vec4 params = _group_0_binding_0_fs.shadowParams[index_1];
    mat4x4 _e11 = _group_0_binding_0_fs.shadowMatrices[index_1];
    vec4 clip = (_e11 * vec4((position_2 + (normal * params.y)), 1.0));
    if ((clip.w <= 0.0)) {
        return 1.0;
    }
    vec3 ndc = (clip.xyz / vec3(clip.w));
    vec2 localUV = vec2(((ndc.x * 0.5) + 0.5), (SHADOW_FLIP_Y ? (0.5 - (ndc.y * 0.5)) : ((ndc.y * 0.5) + 0.5)));
    vec4 rect = _group_0_binding_0_fs.shadowRects[index_1];
    vec2 origin = vec2(rect.x, (SHADOW_FLIP_Y ? rect.y : ((1.0 - rect.y) - rect.w)));
    vec2 uv_2 = (origin + (localUV * rect.zw));
    ivec2 dims_1 = ivec2(uvec2(textureSize(shadowTex, 0).xy));
    ivec2 tileMin = ivec2(roundEven((origin * vec2(dims_1))));
    ivec2 tileMax = ((tileMin + ivec2(roundEven((rect.zw * vec2(dims_1))))) - ivec2(1));
    vec2 texel = ((uv_2 * vec2(dims_1)) - vec2(0.5));
    vec2 base_2 = floor(texel);
    vec2 fraction = (texel - base_2);
    mat4x4 _e87 = _group_0_binding_0_fs.shadowMatrices[index_1];
    vec4 cx = (_e87 * vec4((receiver[0] + (params.y * receiver[2])), 0.0));
    mat4x4 _e99 = _group_0_binding_0_fs.shadowMatrices[index_1];
    vec4 cy = (_e99 * vec4((receiver[1] + (params.y * receiver[3])), 0.0));
    vec3 scale_1 = vec3(((vec2(dims_1) * rect.zw) * vec2(0.5, -0.5)), 1.0);
    vec3 dx = ((cx.xyz - (ndc * cx.w)) * scale_1);
    vec3 dy = ((cy.xyz - (ndc * cy.w)) * scale_1);
    float determinant_ = ((dx.x * dy.y) - (dx.y * dy.x));
    if ((abs(determinant_) > 1e-8)) {
        gradient = (vec2(((dy.y * dx.z) - (dx.y * dy.z)), ((dx.x * dy.z) - (dy.x * dx.z))) / vec2(determinant_));
    }
    if (!(any(lessThan(localUV, vec2(0.0))))) {
        local_11 = any(greaterThan(localUV, vec2(1.0)));
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
    vec4 weightsX = vec4((1.0 - fraction.x), 1.0, 1.0, fraction.x);
    vec4 weightsY = vec4((1.0 - fraction.y), 1.0, 1.0, fraction.y);
    bool loop_init = true;
    while(true) {
        if (!loop_init) {
            int _e251 = y;
            y = (_e251 + 1);
        }
        loop_init = false;
        int _e205 = y;
        if ((_e205 < 4)) {
        } else {
            break;
        }
        {
            x = 0;
            bool loop_init_1 = true;
            while(true) {
                if (!loop_init_1) {
                    int _e248 = x;
                    x = (_e248 + 1);
                }
                loop_init_1 = false;
                int _e210 = x;
                if ((_e210 < 4)) {
                } else {
                    break;
                }
                {
                    int _e214 = x;
                    int _e217 = y;
                    ivec2 pixel = min(max((ivec2(base_2) + ivec2((_e214 - 1), (_e217 - 1))), tileMin), tileMax);
                    vec4 _e225 = texelFetch(shadowTex, pixel, 0);
                    float depth = _e225.x;
                    vec2 _e228 = gradient;
                    float receiverDepth = ((ndc.z + dot(_e228, (vec2(pixel) - texel))) + params.x);
                    float _e235 = visibility;
                    int _e236 = x;
                    int _e238 = y;
                    visibility = (_e235 + ((weightsX[_e236] * weightsY[_e238]) * ((receiverDepth >= depth) ? 1.0 : 0.0)));
                }
            }
        }
    }
    float _e253 = visibility;
    return (_e253 / 9.0);
}

float pbr_light_shadow(uint light, vec3 position_3, vec3 normal_1, mat4x3 receiver_1) {
    uint face = 0u;
    bool local_14 = false;
    bool local_15 = false;
    uint cascade = 0u;
    float next = 0.0;
    bool local_16 = false;
    vec4 mapping = _group_0_binding_0_fs.lightShadows[light];
    if ((mapping.x < 0.5)) {
        return 1.0;
    }
    uint first_1 = (uint(mapping.x) - 1u);
    uint count = uint(mapping.y);
    if ((count == 1u)) {
        float _e20 = pbr_shadow_map(first_1, position_3, normal_1, receiver_1);
        return _e20;
    }
    float _e25 = _group_0_binding_0_fs.lightDirections[light].w;
    if ((_e25 == 2.0)) {
        vec4 _e31 = _group_0_binding_0_fs.lightPosition[light];
        vec3 delta_1 = (position_3 - _e31.xyz);
        vec3 axis = abs(delta_1);
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
        float _e76 = pbr_shadow_map((first_1 + _e74), position_3, normal_1, receiver_1);
        return _e76;
    }
    vec4 _e79 = _group_0_binding_0_fs.cameraPosition;
    vec4 _e84 = _group_0_binding_0_fs.cameraForward;
    float distance_1 = dot((position_3 - _e79.xyz), _e84.xyz);
    float _e91 = _group_0_binding_0_fs.shadowParams[first_1].z;
    if ((distance_1 < _e91)) {
        return 1.0;
    }
    bool loop_init_2 = true;
    while(true) {
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
            vec4 params_1 = _group_0_binding_0_fs.shadowParams[index_3];
            if ((distance_1 <= params_1.w)) {
                float _e109 = pbr_shadow_map(index_3, position_3, normal_1, receiver_1);
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
                    float _e135 = pbr_shadow_map((index_3 + 1u), position_3, normal_1, receiver_1);
                    next = _e135;
                }
                float _e136 = next;
                return mix(_e109, _e136, blend);
            }
        }
    }
    return 1.0;
}

vec3 pbr_occlusion_clip(vec3 position_4) {
    mat4x4 m = _group_0_binding_0_fs.occlusionInverseProjection;
    vec3 a_1 = (m[0].xyz - (position_4 * m[0].w));
    vec3 b_1 = (m[1].xyz - (position_4 * m[1].w));
    vec3 c_3 = (m[2].xyz - (position_4 * m[2].w));
    vec3 d_1 = ((position_4 * m[3].w) - m[3].xyz);
    float determinant_1 = dot(a_1, cross(b_1, c_3));
    return (vec3(dot(d_1, cross(b_1, c_3)), dot(a_1, cross(d_1, c_3)), dot(a_1, cross(b_1, d_1))) / vec3(determinant_1));
}

float pbr_screen_occlusion(vec3 position_5, vec3 normal_2) {
    bool local_17 = false;
    float blocked = 0.0;
    uint sampleIndex = 0u;
    bool local_18 = false;
    float radius_1 = _group_0_binding_0_fs.lightMeta.z;
    float strength = _group_0_binding_0_fs.renderParams.z;
    if (!((strength <= 0.0))) {
        local_17 = (radius_1 <= 0.0);
    } else {
        local_17 = true;
    }
    bool _e18 = local_17;
    if (_e18) {
        return 1.0;
    }
    mat4x4 m_1 = _group_0_binding_0_fs.occlusionInverseProjection;
    vec3 _e23 = pbr_occlusion_clip(position_5);
    vec2 uv_3 = vec2(((_e23.x * 0.5) + 0.5), (SHADOW_FLIP_Y ? (0.5 - (_e23.y * 0.5)) : ((_e23.y * 0.5) + 0.5)));
    vec2 dims_2 = vec2(uvec2(textureSize(screenDepthTex, 0).xy));
    vec4 homogeneous = (m_1 * vec4(_e23, 1.0));
    vec2 axes = vec2(length((m_1[0].xyz - (position_5 * m_1[0].w))), length((m_1[1].xyz - (position_5 * m_1[1].w))));
    vec2 denominator = (axes - (radius_1 * vec2(m_1[0].w, m_1[1].w)));
    vec2 extent = max((abs((vec2((radius_1 * homogeneous.w)) / denominator)) * 0.5), (vec2(1.0) / dims_2));
    float _e84 = _group_0_binding_0_fs.cameraForward.w;
    uint count_1 = uint(clamp(_e84, 8.0, 32.0));
    float diskScale = inversesqrt(float(count_1));
    bool loop_init_3 = true;
    while(true) {
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
            vec2 sampleUV = (uv_3 + ((SSAO_DISK[_e101] * diskScale) * extent));
            if (!(any(lessThan(sampleUV, vec2(0.0))))) {
                local_18 = any(greaterThanEqual(sampleUV, vec2(1.0)));
            } else {
                local_18 = true;
            }
            bool _e118 = local_18;
            if (_e118) {
                continue;
            }
            ivec2 pixel_1 = ivec2((sampleUV * dims_2));
            vec4 _e123 = texelFetch(screenDepthTex, pixel_1, 0);
            float depth_1 = _e123.x;
            if ((depth_1 <= 0.0)) {
                continue;
            }
            vec2 center = ((vec2(pixel_1) + vec2(0.5)) / dims_2);
            vec3 ndc_1 = vec3(((center.x * 2.0) - 1.0), (SHADOW_FLIP_Y ? (1.0 - (center.y * 2.0)) : ((center.y * 2.0) - 1.0)), depth_1);
            vec4 world = (m_1 * vec4(ndc_1, 1.0));
            if ((abs(world.w) < 1e-8)) {
                continue;
            }
            vec3 delta_2 = ((world.xyz / vec3(world.w)) - position_5);
            float distance_2 = length(delta_2);
            float _e167 = _group_0_binding_0_fs.lightMeta.w;
            float horizon = (max((dot(normal_2, delta_2) - _e167), 0.0) / max(distance_2, 1e-6));
            float _e174 = blocked;
            blocked = (_e174 + (horizon * (1.0 - smoothstep((radius_1 * 0.5), radius_1, distance_2))));
        }
    }
    float _e187 = blocked;
    return clamp((1.0 - (((strength * 2.0) * _e187) / float(count_1))), 0.0, 1.0);
}

vec3 pbr_fresnel(float cosine, vec3 f0_, float f90_) {
    return (f0_ + ((vec3(f90_) - f0_) * pow((1.0 - clamp(cosine, 0.0, 1.0)), 5.0)));
}

float pbr_ggx(float nl, float nv, float nh, float roughness_1) {
    float a_2 = (roughness_1 * roughness_1);
    float a2_ = (a_2 * a_2);
    float denominator_1 = (((nh * nh) * (a2_ - 1.0)) + 1.0);
    float d_2 = (a2_ / max(((PI * denominator_1) * denominator_1), 1e-7));
    float gv = (nl * sqrt((((nv * nv) * (1.0 - a2_)) + a2_)));
    float maskingLight = (nv * sqrt((((nl * nl) * (1.0 - a2_)) + a2_)));
    return ((d_2 * 0.5) / max((gv + maskingLight), 0.0001));
}

vec3 pbr_environment_reflection(vec3 normal_3, vec3 v, float roughness_2) {
    float roughnessSquared = (roughness_2 * roughness_2);
    vec3 direction_2 = normalize(mix(reflect(-(v), normal_3), normal_3, (roughnessSquared * roughnessSquared)));
    vec3 _e11 = pbr_environment(direction_2, (roughness_2 * 5.0));
    return _e11;
}

vec3 pbr_multiscattering(vec3 f0_1, float f90_1, vec2 brdf) {
    vec3 single = ((f0_1 * brdf.x) + vec3((f90_1 * brdf.y)));
    float missing = max(0.0, ((1.0 - brdf.x) - brdf.y));
    vec3 average = (f0_1 + ((vec3(1.0) - f0_1) / vec3(21.0)));
    return (((single * average) * missing) / max(vec3(0.0001), (vec3(1.0) - (average * missing))));
}

vec3 pbr_brdf(PBRSurface s, vec3 v_1, vec3 l_1) {
    float specular_1 = 0.0;
    vec3 result = vec3(0.0);
    float nl_1 = max(dot(s.n, l_1), 0.0);
    float nv_1 = max(dot(s.n, v_1), 0.0001);
    vec3 halfSum = (v_1 + l_1);
    vec3 h = (halfSum / vec3(max(length(halfSum), 0.0001)));
    float nh_1 = max(dot(s.n, h), 0.0);
    float vh = max(dot(v_1, h), 0.0);
    float _e25 = pbr_ggx(nl_1, nv_1, nh_1, s.roughness);
    specular_1 = _e25;
    vec3 _e29 = pbr_fresnel(vh, s.f0_, s.f90_);
    float diffuseF = (s.diffuseF0_ + ((s.diffuseF90_ - s.diffuseF0_) * pow((1.0 - clamp(vh, 0.0, 1.0)), 5.0)));
    vec3 diffuse = ((((1.0 - diffuseF) * (1.0 - s.metallic)) * s.base) / vec3(3.1415927));
    float _e54 = specular_1;
    result = ((diffuse + (_e54 * _e29)) * nl_1);
    vec3 _e59 = result;
    return _e59;
}

mat3x3 pbr_basis(PBRInput input_1, vec3 n_1) {
    vec3 t = vec3(0.0);
    float handedness = 0.0;
    bool local_19 = false;
    bool local_20 = false;
    t = (input_1.tangent.xyz - (n_1 * dot(n_1, input_1.tangent.xyz)));
    handedness = ((input_1.tangent.w >= 0.0) ? 1.0 : -1.0);
    if (!((abs(input_1.tangent.w) < 0.5))) {
        vec3 _e26 = t;
        vec3 _e27 = t;
        local_19 = (dot(_e26, _e27) < 1e-8);
    } else {
        local_19 = true;
    }
    bool _e32 = local_19;
    if (_e32) {
        vec3 dp1_ = dFdx(input_1.worldPosition);
        vec3 dp2_ = dFdy(input_1.worldPosition);
        vec2 duv1_ = dFdx(input_1.uv);
        vec2 duv2_ = dFdy(input_1.uv);
        float determinant_2 = ((duv1_.x * duv2_.y) - (duv1_.y * duv2_.x));
        float signUV = ((determinant_2 >= 0.0) ? 1.0 : -1.0);
        vec3 raw = (((dp1_ * duv2_.y) - (dp2_ * duv1_.y)) * signUV);
        t = (raw - (n_1 * dot(n_1, raw)));
        vec3 b_2 = (((dp2_ * duv1_.x) - (dp1_ * duv2_.x)) * signUV);
        vec3 _e68 = t;
        handedness = ((dot(cross(n_1, _e68), b_2) >= 0.0) ? 1.0 : -1.0);
        vec3 _e76 = t;
        vec3 _e77 = t;
        if (!((dot(_e76, _e77) < 1e-8))) {
            local_20 = (abs(determinant_2) < 1e-8);
        } else {
            local_20 = true;
        }
        bool _e88 = local_20;
        if (_e88) {
            vec3 axis_1 = ((abs(n_1.y) > 0.99) ? vec3(1.0, 0.0, 0.0) : vec3(0.0, 1.0, 0.0));
            t = cross(axis_1, n_1);
            handedness = 1.0;
        }
    }
    vec3 _e104 = t;
    t = normalize(_e104);
    vec3 _e106 = t;
    vec3 _e107 = t;
    float _e109 = handedness;
    return mat3x3(_e106, (cross(n_1, _e107) * _e109), n_1);
}

vec3 pbr_mapped_normal(mat3x3 frame, vec3 mapped, float scale) {
    vec3 value = (frame * vec3((mapped.xy * scale), mapped.z));
    return ((dot(value, value) > 1e-8) ? (value / vec3(max(length(value), 1e-6))) : frame[2]);
}

float light_range_window(float distanceSquared, float range) {
    float ratio = (distanceSquared / max((range * range), 1e-8));
    float window = max((1.0 - (ratio * ratio)), 0.0);
    return (window * window);
}

float light_distance_attenuation(float distanceSquared_1, float range_1, float radius) {
    float _e3 = light_range_window(distanceSquared_1, range_1);
    return (_e3 / max(distanceSquared_1, max((radius * radius), 0.0001)));
}

vec3 area_lighting(uint index_2, vec3 position_6, PBRSurface s_1, vec3 v_2) {
    vec3 result_1 = vec3(0.0);
    uint sampleIndex_1 = 0u;
    vec3 offset = vec3(0.0);
    float measure = 0.0;
    float weight = 0.0;
    float kind = _group_0_binding_0_fs.lightDirections[index_2].w;
    vec4 _e12 = _group_0_binding_0_fs.lightPosition[index_2];
    vec3 center_1 = _e12.xyz;
    vec4 right = _group_0_binding_0_fs.lightRight[index_2];
    vec4 up = _group_0_binding_0_fs.lightUp[index_2];
    vec4 _e25 = _group_0_binding_0_fs.lightDirections[index_2];
    vec3 forward = _e25.xyz;
    bool loop_init_4 = true;
    while(true) {
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
            offset = vec3(0.0);
            measure = ((right.w * up.w) / 16.0);
            if ((kind == 6.0)) {
                uint _e46 = sampleIndex_1;
                uint _e50 = sampleIndex_1;
                vec2 uv_4 = (((vec2(float((_e46 % 4u)), float((_e50 / 4u))) + vec2(0.5)) / vec2(4.0)) - vec2(0.5));
                offset = ((right.xyz * (uv_4.x * right.w)) + (up.xyz * (uv_4.y * up.w)));
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
            vec3 _e120 = offset;
            vec3 delta_3 = ((center_1 + _e120) - position_6);
            float distanceSquared_2 = dot(delta_3, delta_3);
            vec3 l_2 = (delta_3 * inversesqrt(max(distanceSquared_2, 1e-8)));
            float _e128 = measure;
            float _e133 = _group_0_binding_0_fs.lightParams[index_2].x;
            float _e135 = light_distance_attenuation(distanceSquared_2, _e133, 0.0);
            weight = (_e128 * _e135);
            if ((kind != 8.0)) {
                float _e140 = weight;
                weight = (_e140 * max(dot(forward, -(l_2)), 0.0));
            }
            vec3 _e146 = result_1;
            vec3 _e147 = pbr_brdf(s_1, v_2, l_2);
            float _e148 = weight;
            result_1 = (_e146 + (_e147 * _e148));
        }
    }
    vec3 _e154 = result_1;
    vec4 _e158 = _group_0_binding_0_fs.lightColors[index_2];
    float _e165 = _group_0_binding_0_fs.lightParams[index_2].y;
    return ((_e154 * _e158.xyz) * _e165);
}

void main() {
    PBRInput input_ = PBRInput(gl_FragCoord, gl_FrontFacing, _vs2fs_location0, _vs2fs_location1, _vs2fs_location2, _vs2fs_location3, _vs2fs_location4, _vs2fs_location5, _vs2fs_location6, _vs2fs_location7, _vs2fs_location10);
    vec4 base = vec4(0.0);
    bool local = false;
    bool local_1 = false;
    bool local_2 = false;
    float metallic = 0.0;
    float roughness = 0.0;
    vec3 n = vec3(0.0);
    bool local_3 = false;
    float ao = 1.0;
    bool local_4 = false;
    bool local_5 = false;
    float specular = 0.0;
    vec3 specColor = vec3(0.0);
    vec3 emission = vec3(0.0);
    vec3 lit = vec3(0.0);
    vec3 indirect = vec3(0.0);
    float shadow = 1.0;
    uint index = 0u;
    vec3 l = vec3(0.0);
    float attenuation = 0.0;
    bool local_6 = false;
    bool local_7 = false;
    bool local_8 = false;
    float angle = 0.0;
    bool local_9 = false;
    bool local_10 = false;
    uint flags = uint((input_.material.w + 0.5));
    vec4 _e9 = texture(tex, vec2(input_.uv));
    base = (_e9 * input_.baseColor);
    if (((flags & 3u) == 1u)) {
        float _e20 = base.w;
        local = (_e20 < input_.material.z);
    } else {
        local = false;
    }
    bool _e25 = local;
    if (_e25) {
        discard;
    }
    float _e30 = _group_0_binding_0_fs.renderParams.w;
    bool front = ((input_.front != (_e30 > 0.5)) != ((flags & 1024u) != 0u));
    if (((flags & 512u) == 0u)) {
        local_1 = !(front);
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
    float _e57 = _group_0_binding_0_fs.renderParams.y;
    if ((_e57 == 3.0)) {
        local_2 = (input_.material.y < 0.0);
    } else {
        local_2 = false;
    }
    bool _e67 = local_2;
    if (_e67) {
        _fs2p_location0 = vec4(0.0);
        return;
    }
    metallic = input_.emissiveMetallic.w;
    roughness = input_.material.x;
    if (((flags & 8u) != 0u)) {
        vec4 mr = texture(metallicRoughnessTex, vec2(input_.uv));
        float _e84 = metallic;
        metallic = (_e84 * mr.z);
        float _e87 = roughness;
        roughness = clamp((_e87 * mr.y), 0.045, 1.0);
    }
    vec3 _e94 = dFdx(input_.normal);
    vec3 _e97 = dFdy(input_.normal);
    vec3 normalVariation = max(abs(_e94), abs(_e97));
    float geometryRoughness = max(max(normalVariation.x, normalVariation.y), normalVariation.z);
    float _e105 = roughness;
    roughness = min(1.0, (max(_e105, 0.0525) + geometryRoughness));
    n = normalize(input_.normal);
    if (((flags & 512u) != 0u)) {
        local_3 = !(front);
    } else {
        local_3 = false;
    }
    bool _e122 = local_3;
    if (_e122) {
        vec3 _e123 = n;
        n = -(_e123);
    }
    vec3 shadowNormal = n;
    vec3 _e127 = dFdx(input_.worldPosition);
    vec3 _e129 = dFdy(input_.worldPosition);
    vec3 _e130 = n;
    vec3 _e131 = dFdx(_e130);
    vec3 _e132 = n;
    vec3 _e133 = dFdy(_e132);
    mat4x3 receiver_2 = mat4x3(_e127, _e129, _e131, _e133);
    if (((flags & 16u) != 0u)) {
        vec4 _e144 = texture(occlusionTex, vec2(input_.uv));
        ao = mix(1.0, _e144.x, input_.custom0_.y);
    }
    float _e153 = _group_0_binding_0_fs.renderParams.y;
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
        float _e174 = pbr_screen_occlusion(input_.worldPosition, shadowNormal);
        ao = (_e172 * _e174);
    }
    float _e179 = _group_0_binding_0_fs.renderParams.y;
    if ((_e179 == 6.0)) {
        float _e182 = ao;
        _fs2p_location0 = vec4(_e182, 0.0, 0.0, 1.0);
        return;
    }
    if (((flags & 4u) != 0u)) {
        vec3 _e191 = n;
        mat3x3 _e192 = pbr_basis(input_, _e191);
        vec4 _e196 = texture(normalTex, vec2(input_.uv));
        vec3 _e205 = pbr_mapped_normal(_e192, ((_e196.xyz * 2.0) - vec3(1.0)), input_.custom0_.x);
        n = _e205;
    }
    vec4 _e208 = _group_0_binding_0_fs.cameraPosition;
    vec4 _e215 = _group_0_binding_0_fs.cameraForward;
    float _e221 = _group_0_binding_0_fs.cameraPosition.w;
    vec3 v_3 = ((_e221 < 0.0) ? -(_e215.xyz) : normalize((_e208.xyz - input_.worldPosition)));
    specular = input_.custom0_.z;
    specColor = input_.custom1_.xyz;
    if (((flags & 64u) != 0u)) {
        float _e235 = specular;
        vec4 _e239 = texture(specularTex, vec2(input_.uv));
        specular = (_e235 * _e239.w);
    }
    if (((flags & 128u) != 0u)) {
        vec3 _e246 = specColor;
        vec4 _e250 = texture(specularColorTex, vec2(input_.uv));
        specColor = (_e246 * _e250.xyz);
    }
    float iorRatio = ((input_.material.y == 0.0) ? 1.0 : (1.0 - (2.0 / (max(input_.material.y, 1.0) + 1.0))));
    vec3 _e271 = specColor;
    float _e276 = specular;
    vec3 dielectricF0_ = (min((vec3((iorRatio * iorRatio)) * _e271), vec3(1.0)) * _e276);
    float diffuseF0_ = max(max(dielectricF0_.x, dielectricF0_.y), dielectricF0_.z);
    vec4 _e283 = base;
    float _e285 = metallic;
    vec3 f0_2 = mix(dielectricF0_, _e283.xyz, _e285);
    float _e287 = specular;
    float _e288 = metallic;
    float f90_2 = mix(_e287, 1.0, _e288);
    emission = input_.emissiveMetallic.xyz;
    if (((flags & 32u) != 0u)) {
        vec3 _e298 = emission;
        vec4 _e302 = texture(emissiveTex, vec2(input_.uv));
        emission = (_e298 * _e302.xyz);
    }
    vec3 _e305 = n;
    float nv_2 = max(dot(_e305, v_3), 0.0001);
    vec3 _e311 = n;
    vec3 _e312 = n;
    vec4 _e317 = base;
    float _e319 = metallic;
    float _e320 = roughness;
    float _e321 = specular;
    PBRSurface surface = PBRSurface(_e311, _e312, vec3(0.0), vec3(0.0), _e317.xyz, _e319, _e320, f0_2, f90_2, diffuseF0_, _e321, 0.0, 1.0, 0.0, vec3(0.0), 1.0, 0.0, 1.0, 0.0, 0.0);
    float _e333 = specular;
    float ambientF = (diffuseF0_ + ((_e333 - diffuseF0_) * pow((1.0 - clamp(nv_2, 0.0, 1.0)), 5.0)));
    float _e346 = metallic;
    float diffuseWeight = ((1.0 - ambientF) * (1.0 - _e346));
    vec4 _e350 = base;
    vec4 _e354 = _group_0_binding_0_fs.ambientLight;
    float _e359 = ao;
    vec3 _e361 = emission;
    lit = ((((((_e350.xyz * _e354.xyz) * diffuseWeight) * 1.0) * _e359) + _e361) * (1.0 - 0.0));
    float _e370 = _group_0_binding_0_fs.environment.x;
    if ((_e370 > 0.0)) {
        vec3 _e373 = n;
        vec3 _e375 = pbr_environment(_e373, 6.0);
        vec3 reflectionNormal = n;
        float _e377 = roughness;
        vec3 _e378 = pbr_environment_reflection(reflectionNormal, v_3, _e377);
        float _e379 = roughness;
        vec4 _e382 = pbr_atlas(vec2(nv_2, _e379), 7.0);
        vec2 brdf_1 = _e382.xy;
        vec3 dielectricSingle = ((dielectricF0_ * brdf_1.x) + vec3((f90_2 * brdf_1.y)));
        vec4 _e390 = base;
        vec3 metallicSingle = ((_e390.xyz * brdf_1.x) + vec3((f90_2 * brdf_1.y)));
        vec3 _e398 = pbr_multiscattering(dielectricF0_, f90_2, brdf_1);
        vec4 _e399 = base;
        vec3 _e401 = pbr_multiscattering(_e399.xyz, f90_2, brdf_1);
        float _e402 = metallic;
        vec3 single_1 = mix(dielectricSingle, metallicSingle, _e402);
        float _e404 = metallic;
        vec3 multiple = mix(_e398, _e401, _e404);
        vec4 _e406 = base;
        float _e408 = metallic;
        vec3 diffuse_1 = ((_e406.xyz * (1.0 - _e408)) * max(vec3(0.0), ((vec3(1.0) - dielectricSingle) - _e398)));
        indirect = (((_e378 * single_1) + (_e375 * (multiple + diffuse_1))) * 1.0);
        vec3 _e426 = indirect;
        indirect = (_e426 * (1.0 - 0.0));
        vec3 _e430 = lit;
        vec3 _e431 = indirect;
        float _e435 = _group_0_binding_0_fs.environment.x;
        float _e437 = ao;
        lit = (_e430 + ((_e431 * _e435) * _e437));
    }
    float _e443 = _group_0_binding_0_fs.lightMeta.x;
    if ((_e443 == 0.0)) {
        if (((flags & 4096u) == 0u)) {
            float _e454 = pbr_light_shadow(0u, input_.worldPosition, shadowNormal, receiver_2);
            shadow = _e454;
        }
        vec3 _e455 = lit;
        vec4 _e458 = _group_0_binding_0_fs.lightDirection;
        vec3 _e462 = pbr_brdf(surface, v_3, normalize(-(_e458.xyz)));
        vec4 _e465 = _group_0_binding_0_fs.directionalLight;
        float _e468 = shadow;
        lit = (_e455 + ((_e462 * _e465.xyz) * _e468));
    }
    bool loop_init_5 = true;
    while(true) {
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
            float _e481 = _group_0_binding_0_fs.lightMeta.x;
            if ((float(_e476) >= _e481)) {
                break;
            }
            uint _e485 = index;
            float kind_1 = _group_0_binding_0_fs.lightDirections[_e485].w;
            if ((kind_1 == 0.0)) {
                continue;
            }
            if ((kind_1 >= 6.0)) {
                vec3 _e493 = lit;
                uint _e494 = index;
                vec3 _e496 = area_lighting(_e494, input_.worldPosition, surface, v_3);
                lit = (_e493 + _e496);
                continue;
            }
            uint _e500 = index;
            vec4 _e502 = _group_0_binding_0_fs.lightDirections[_e500];
            l = -(_e502.xyz);
            attenuation = 1.0;
            if ((kind_1 >= 2.0)) {
                uint _e512 = index;
                vec4 _e514 = _group_0_binding_0_fs.lightPosition[_e512];
                vec3 delta_4 = (_e514.xyz - input_.worldPosition);
                float distanceSquared_3 = dot(delta_4, delta_4);
                uint _e521 = index;
                float range_2 = _group_0_binding_0_fs.lightParams[_e521].x;
                uint _e527 = index;
                vec4 _e529 = _group_0_binding_0_fs.lightDirections[_e527];
                vec3 forward_1 = _e529.xyz;
                uint _e533 = index;
                vec4 right_1 = _group_0_binding_0_fs.lightRight[_e533];
                uint _e538 = index;
                vec4 up_1 = _group_0_binding_0_fs.lightUp[_e538];
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
                    float _e579 = _group_0_binding_0_fs.lightParams[_e576].y;
                    attenuation = _e579;
                    uint _e582 = index;
                    float _e585 = _group_0_binding_0_fs.lightParams[_e582].z;
                    if ((_e585 > 0.5)) {
                        float _e588 = attenuation;
                        float _e590 = light_range_window((depth_2 * depth_2), range_2);
                        attenuation = (_e588 * _e590);
                    }
                } else {
                    if ((distanceSquared_3 >= (range_2 * range_2))) {
                        continue;
                    }
                    l = (delta_4 * inversesqrt(max(distanceSquared_3, 1e-8)));
                    uint _e600 = index;
                    float _e603 = _group_0_binding_0_fs.lightColors[_e600].w;
                    float _e604 = light_distance_attenuation(distanceSquared_3, range_2, _e603);
                    uint _e607 = index;
                    float _e610 = _group_0_binding_0_fs.lightParams[_e607].y;
                    attenuation = (_e604 * _e610);
                    if ((kind_1 == 3.0)) {
                        vec3 _e614 = l;
                        float cone = dot(forward_1, -(_e614));
                        uint _e619 = index;
                        float inner = _group_0_binding_0_fs.lightParams[_e619].z;
                        uint _e625 = index;
                        float outer = _group_0_binding_0_fs.lightParams[_e625].w;
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
                float _e681 = pbr_light_shadow(_e679, input_.worldPosition, shadowNormal, receiver_2);
                attenuation = (_e678 * _e681);
            }
            vec3 _e683 = lit;
            vec3 _e684 = l;
            vec3 _e685 = pbr_brdf(surface, v_3, _e684);
            uint _e688 = index;
            vec4 _e690 = _group_0_binding_0_fs.lightColors[_e688];
            float _e693 = attenuation;
            lit = (_e683 + ((_e685 * _e690.xyz) * _e693));
        }
    }
    vec3 _e699 = lit;
    float _e701 = base.w;
    vec4 _e703 = pbr_surface_output(_e699, _e701, input_.worldPosition);
    _fs2p_location0 = _e703;
    return;
}
