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
    bool local_4 = {};
    bool local_5 = {};
    bool local_6 = {};
    metal::float2 center = metal::float2(0.0);
    metal::float2 radius = metal::float2(0.0);
    bool local_7 = {};
    bool local_8 = {};
    bool local_9 = {};
    bool local_10 = {};
    bool local_11 = {};
    if (!(point.x < 0.0)) {
        local_4 = point.y < 0.0;
    } else {
        local_4 = true;
    }
    bool _e14 = local_4;
    if (!(_e14)) {
        local_5 = point.x >= size.x;
    } else {
        local_5 = true;
    }
    bool _e22 = local_5;
    if (!(_e22)) {
        local_6 = point.y >= size.y;
    } else {
        local_6 = true;
    }
    bool _e30 = local_6;
    if (_e30) {
        return false;
    }
    if (point.x < rx.x) {
        local_7 = point.y < ry.x;
    } else {
        local_7 = false;
    }
    bool _e47 = local_7;
    if (_e47) {
        radius = metal::float2(rx.x, ry.x);
        metal::float2 _e51 = radius;
        center = _e51;
    } else {
        if (point.x > (size.x - rx.y)) {
            local_8 = point.y < ry.y;
        } else {
            local_8 = false;
        }
        bool _e63 = local_8;
        if (_e63) {
            radius = metal::float2(rx.y, ry.y);
            float _e69 = radius.x;
            float _e72 = radius.y;
            center = metal::float2(size.x - _e69, _e72);
        } else {
            if (point.x > (size.x - rx.z)) {
                local_9 = point.y > (size.y - ry.z);
            } else {
                local_9 = false;
            }
            bool _e87 = local_9;
            if (_e87) {
                radius = metal::float2(rx.z, ry.z);
                metal::float2 _e91 = radius;
                center = size - _e91;
            } else {
                if (point.x < rx.w) {
                    local_10 = point.y > (size.y - ry.w);
                } else {
                    local_10 = false;
                }
                bool _e104 = local_10;
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
        local_11 = _e123 <= 0.0;
    } else {
        local_11 = true;
    }
    bool _e127 = local_11;
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
    bool local = {};
    bool local_1 = {};
    metal::float4 result = metal::float4(0.0);
    float total = 0.0;
    int index = -12;
    bool local_2 = {};
    bool local_3 = {};
    float mode = metal::abs(input.parameters.w);
    metal::float2 sampleUV = metal::float2(input.uv.x, (input.parameters.w < 0.0) ? 1.0 - input.uv.y : input.uv.y);
    metal::float2 gradientsX = metal::dfdx(sampleUV);
    metal::float2 gradientsY = metal::dfdy(sampleUV);
    if (mode > 2.5) {
        local = mode < 3.5;
    } else {
        local = false;
    }
    bool _e27 = local;
    if (!(_e27)) {
        local_1 = mode > 5.5;
    } else {
        local_1 = true;
    }
    bool _e34 = local_1;
    if (_e34) {
        metal::float4 pixel = source.sample(sourceSampler, sampleUV, metal::gradient2d(gradientsX, gradientsY));
        if (mode > 5.5) {
            float alpha = metal::min(pixel.x, pixel.w) * input.color.w;
            return sprite_fragmentOutput { metal::float4(input.color.xyz * alpha, alpha) };
        }
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
            int _e119 = index;
            index = as_type<int>(as_type<uint>(_e119) + as_type<uint>(1));
        }
        loop_init = false;
        int _e77 = index;
        if (_e77 <= 12) {
        } else {
            break;
        }
        {
            int _e80 = index;
            if (naga_abs(_e80) > limit) {
                continue;
            }
            int _e83 = index;
            float position = static_cast<float>(_e83);
            float weight = metal::exp((-(position) * position) / ((2.0 * sigma) * sigma));
            metal::float2 uv = sampleUV + (input.parameters.xy * position);
            metal::float4 pixel_1 = source.sample(sourceSampler, uv, metal::gradient2d(gradientsX, gradientsY));
            metal::float4 premultiplied = (mode > 3.5) ? metal::float4(metal::min(pixel_1.x, pixel_1.w)) : (mode > 0.5) ? pixel_1 : metal::float4(pixel_1.xyz * pixel_1.w, pixel_1.w);
            metal::float4 _e114 = result;
            result = _e114 + (premultiplied * weight);
            float _e117 = total;
            total = _e117 + weight;
        }
    }
    metal::float4 _e122 = result;
    float _e123 = total;
    result = _e122 / metal::float4(_e123);
    if (mode > 1.5) {
        local_2 = mode < 2.5;
    } else {
        local_2 = false;
    }
    bool _e133 = local_2;
    if (!(_e133)) {
        local_3 = mode > 4.5;
    } else {
        local_3 = true;
    }
    bool _e140 = local_3;
    if (_e140) {
        metal::float2 pixel_2 = input.uv * static_cast<metal::float2>(metal::uint2(source.get_width(), source.get_height()));
        bool _e153 = insideRoundedBox(pixel_2 - input.box.xy, input.box.zw, input.radiusX, input.radiusY);
        if (_e153) {
            return sprite_fragmentOutput { metal::float4(0.0) };
        }
    }
    float alpha_1 = input.color.w;
    metal::float4 _e158 = result;
    float _e165 = result.w;
    return sprite_fragmentOutput { metal::float4((_e158.xyz * input.color.xyz) * alpha_1, _e165 * alpha_1) };
}
