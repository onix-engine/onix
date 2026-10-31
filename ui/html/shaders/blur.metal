// language: metal2.3
#include <metal_stdlib>
#include <simd/simd.h>

using metal::uint;
struct DefaultConstructible {
    template<typename T>
    operator T() && {
        return T {};
    }
};

struct Input {
    metal::float2 uv;
    char _pad1[8];
    metal::float4 color;
    metal::float4 parameters;
    metal::float4 box;
    metal::float4 radiusX;
    metal::float4 radiusY;
};

bool insideRoundedBox(
    metal::float2 point,
    metal::float2 size,
    metal::float4 rx,
    metal::float4 ry
) {
    bool local = {};
    bool local_1 = {};
    bool local_2 = {};
    metal::float2 center = metal::float2(0.0);
    metal::float2 radius = metal::float2(0.0);
    bool local_3 = {};
    bool local_4 = {};
    bool local_5 = {};
    bool local_6 = {};
    bool local_7 = {};
    if (!(point.x < 0.0)) {
        local = point.y < 0.0;
    } else {
        local = true;
    }
    bool _e14 = local;
    if (!(_e14)) {
        local_1 = point.x >= size.x;
    } else {
        local_1 = true;
    }
    bool _e22 = local_1;
    if (!(_e22)) {
        local_2 = point.y >= size.y;
    } else {
        local_2 = true;
    }
    bool _e30 = local_2;
    if (_e30) {
        return false;
    }
    if (point.x < rx.x) {
        local_3 = point.y < ry.x;
    } else {
        local_3 = false;
    }
    bool _e47 = local_3;
    if (_e47) {
        radius = metal::float2(rx.x, ry.x);
        metal::float2 _e51 = radius;
        center = _e51;
    } else {
        if (point.x > (size.x - rx.y)) {
            local_4 = point.y < ry.y;
        } else {
            local_4 = false;
        }
        bool _e63 = local_4;
        if (_e63) {
            radius = metal::float2(rx.y, ry.y);
            float _e69 = radius.x;
            float _e72 = radius.y;
            center = metal::float2(size.x - _e69, _e72);
        } else {
            if (point.x > (size.x - rx.z)) {
                local_5 = point.y > (size.y - ry.z);
            } else {
                local_5 = false;
            }
            bool _e87 = local_5;
            if (_e87) {
                radius = metal::float2(rx.z, ry.z);
                metal::float2 _e91 = radius;
                center = size - _e91;
            } else {
                if (point.x < rx.w) {
                    local_6 = point.y > (size.y - ry.w);
                } else {
                    local_6 = false;
                }
                bool _e104 = local_6;
                if (_e104) {
                    radius = metal::float2(rx.w, ry.w);
                    float _e109 = radius.x;
                    float _e112 = radius.y;
                    center = metal::float2(_e109, size.y - _e112);
                }
            }
        }
    }
    float _e116 = radius.x;
    if (!(_e116 <= 0.0)) {
        float _e123 = radius.y;
        local_7 = _e123 <= 0.0;
    } else {
        local_7 = true;
    }
    bool _e127 = local_7;
    if (_e127) {
        return true;
    }
    metal::float2 _e129 = center;
    metal::float2 _e131 = radius;
    metal::float2 normalized = (point - _e129) / _e131;
    return metal::dot(normalized, normalized) <= 1.0;
}
int naga_f2i32(float value) {
    return static_cast<int>(metal::clamp(value, -2147483600.0, 2147483500.0));
}

int naga_abs(int val) {
    return metal::select(as_type<int>(-as_type<uint>(val)), val, val >= 0);
}


struct sprite_fragmentInput {
    metal::float2 uv [[user(loc0), center_perspective]];
    metal::float4 color [[user(loc1), center_perspective]];
    metal::float4 parameters [[user(loc2), flat]];
    metal::float4 box [[user(loc3), flat]];
    metal::float4 radiusX [[user(loc4), flat]];
    metal::float4 radiusY [[user(loc5), flat]];
};
struct sprite_fragmentOutput {
    metal::float4 member [[color(0)]];
};
fragment sprite_fragmentOutput sprite_fragment(
  sprite_fragmentInput varyings [[stage_in]]
, metal::texture2d<float, metal::access::sample> source [[texture(0)]]
) {
    constexpr metal::sampler sourceSampler(
        metal::s_address::clamp_to_edge,
        metal::t_address::clamp_to_edge,
        metal::r_address::clamp_to_edge,
        metal::mag_filter::linear,
        metal::min_filter::linear,
        metal::mip_filter::linear,
        metal::coord::normalized
    );
    const Input input = { varyings.uv, {}, varyings.color, varyings.parameters, varyings.box, varyings.radiusX, varyings.radiusY };
    metal::float4 result = metal::float4(0.0);
    float total = 0.0;
    int index = -12;
    float mode = metal::abs(input.parameters.w);
    metal::float2 sampleUV = metal::float2(input.uv.x, (input.parameters.w < 0.0) ? 1.0 - input.uv.y : input.uv.y);
    metal::float2 gradientsX = metal::dfdx(sampleUV);
    metal::float2 gradientsY = metal::dfdy(sampleUV);
    if (mode > 2.5) {
        metal::float4 pixel = source.sample(sourceSampler, sampleUV, metal::gradient2d(gradientsX, gradientsY));
        return sprite_fragmentOutput { metal::float4((pixel.xyz * input.color.xyz) * input.color.w, pixel.w * input.color.w) };
    }
    float sigma = metal::max(input.parameters.z, 0.001);
    int limit = naga_f2i32(metal::ceil(sigma * 3.0));
    uint2 loop_bound = uint2(4294967295u);
    bool loop_init = true;
    while(true) {
        if (metal::all(loop_bound == uint2(0u))) { break; }
        loop_bound -= uint2(loop_bound.y == 0u, 1u);
        if (!loop_init) {
            int _e87 = index;
            index = as_type<int>(as_type<uint>(_e87) + as_type<uint>(1));
        }
        loop_init = false;
        int _e52 = index;
        if (_e52 <= 12) {
        } else {
            break;
        }
        {
            int _e55 = index;
            if (naga_abs(_e55) > limit) {
                continue;
            }
            int _e58 = index;
            float position = static_cast<float>(_e58);
            float weight = metal::exp((-(position) * position) / ((2.0 * sigma) * sigma));
            metal::float2 uv = sampleUV + (input.parameters.xy * position);
            metal::float4 pixel_1 = source.sample(sourceSampler, uv, metal::gradient2d(gradientsX, gradientsY));
            metal::float4 premultiplied = (mode > 0.5) ? pixel_1 : metal::float4(pixel_1.xyz * pixel_1.w, pixel_1.w);
            metal::float4 _e82 = result;
            result = _e82 + (premultiplied * weight);
            float _e85 = total;
            total = _e85 + weight;
        }
    }
    metal::float4 _e90 = result;
    float _e91 = total;
    result = _e90 / metal::float4(_e91);
    if (mode > 1.5) {
        metal::float2 pixel_2 = input.uv * static_cast<metal::float2>(metal::uint2(source.get_width(), source.get_height()));
        bool _e108 = insideRoundedBox(pixel_2 - input.box.xy, input.box.zw, input.radiusX, input.radiusY);
        if (_e108) {
            return sprite_fragmentOutput { metal::float4(0.0) };
        }
    }
    float alpha = input.color.w;
    metal::float4 _e113 = result;
    float _e120 = result.w;
    return sprite_fragmentOutput { metal::float4((_e113.xyz * input.color.xyz) * alpha, _e120 * alpha) };
}
