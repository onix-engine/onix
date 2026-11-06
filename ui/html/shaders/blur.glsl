#version 300 es

precision highp float;
precision highp int;

struct Input {
    vec2 uv;
    vec4 color;
    vec4 parameters;
    vec4 box;
    vec4 radiusX;
    vec4 radiusY;
};
uniform highp sampler2D _group_0_binding_1_fs;

in vec2 _vs2fs_location0;
in vec4 _vs2fs_location1;
flat in vec4 _vs2fs_location2;
flat in vec4 _vs2fs_location3;
flat in vec4 _vs2fs_location4;
flat in vec4 _vs2fs_location5;
layout(location = 0) out vec4 _fs2p_location0;

bool insideRoundedBox(vec2 point, vec2 size, vec4 rx, vec4 ry) {
    bool local_4 = false;
    bool local_5 = false;
    bool local_6 = false;
    vec2 center = vec2(0.0);
    vec2 radius = vec2(0.0);
    bool local_7 = false;
    bool local_8 = false;
    bool local_9 = false;
    bool local_10 = false;
    bool local_11 = false;
    if (!((point.x < 0.0))) {
        local_4 = (point.y < 0.0);
    } else {
        local_4 = true;
    }
    bool _e14 = local_4;
    if (!(_e14)) {
        local_5 = (point.x >= size.x);
    } else {
        local_5 = true;
    }
    bool _e22 = local_5;
    if (!(_e22)) {
        local_6 = (point.y >= size.y);
    } else {
        local_6 = true;
    }
    bool _e30 = local_6;
    if (_e30) {
        return false;
    }
    if ((point.x < rx.x)) {
        local_7 = (point.y < ry.x);
    } else {
        local_7 = false;
    }
    bool _e47 = local_7;
    if (_e47) {
        radius = vec2(rx.x, ry.x);
        vec2 _e51 = radius;
        center = _e51;
    } else {
        if ((point.x > (size.x - rx.y))) {
            local_8 = (point.y < ry.y);
        } else {
            local_8 = false;
        }
        bool _e63 = local_8;
        if (_e63) {
            radius = vec2(rx.y, ry.y);
            float _e69 = radius.x;
            float _e72 = radius.y;
            center = vec2((size.x - _e69), _e72);
        } else {
            if ((point.x > (size.x - rx.z))) {
                local_9 = (point.y > (size.y - ry.z));
            } else {
                local_9 = false;
            }
            bool _e87 = local_9;
            if (_e87) {
                radius = vec2(rx.z, ry.z);
                vec2 _e91 = radius;
                center = (size - _e91);
            } else {
                if ((point.x < rx.w)) {
                    local_10 = (point.y > (size.y - ry.w));
                } else {
                    local_10 = false;
                }
                bool _e104 = local_10;
                if (_e104) {
                    radius = vec2(rx.w, ry.w);
                    float _e109 = radius.x;
                    float _e112 = radius.y;
                    center = vec2(_e109, (size.y - _e112));
                }
            }
        }
    }
    float _e116 = radius.x;
    if (!((_e116 <= 0.0))) {
        float _e123 = radius.y;
        local_11 = (_e123 <= 0.0);
    } else {
        local_11 = true;
    }
    bool _e127 = local_11;
    if (_e127) {
        return true;
    }
    vec2 _e129 = center;
    vec2 _e131 = radius;
    vec2 normalized = ((point - _e129) / _e131);
    return (dot(normalized, normalized) <= 1.0);
}

void main() {
    Input input_ = Input(_vs2fs_location0, _vs2fs_location1, _vs2fs_location2, _vs2fs_location3, _vs2fs_location4, _vs2fs_location5);
    bool local = false;
    bool local_1 = false;
    vec4 result = vec4(0.0);
    float total = 0.0;
    int index = -12;
    bool local_2 = false;
    bool local_3 = false;
    float mode = abs(input_.parameters.w);
    vec2 sampleUV = vec2(input_.uv.x, ((input_.parameters.w < 0.0) ? (1.0 - input_.uv.y) : input_.uv.y));
    vec2 gradientsX = dFdx(sampleUV);
    vec2 gradientsY = dFdy(sampleUV);
    if ((mode > 2.5)) {
        local = (mode < 3.5);
    } else {
        local = false;
    }
    bool _e27 = local;
    if (!(_e27)) {
        local_1 = (mode > 5.5);
    } else {
        local_1 = true;
    }
    bool _e34 = local_1;
    if (_e34) {
        vec4 pixel = textureGrad(_group_0_binding_1_fs, vec2(sampleUV), gradientsX, gradientsY);
        if ((mode > 5.5)) {
            float alpha = (min(pixel.x, pixel.w) * input_.color.w);
            _fs2p_location0 = vec4((input_.color.xyz * alpha), alpha);
            return;
        }
        _fs2p_location0 = vec4(((pixel.xyz * input_.color.xyz) * input_.color.w), (pixel.w * input_.color.w));
        return;
    }
    float sigma = max(input_.parameters.z, 0.001);
    int limit = int(ceil((sigma * 3.0)));
    bool loop_init = true;
    while(true) {
        if (!loop_init) {
            int _e119 = index;
            index = (_e119 + 1);
        }
        loop_init = false;
        int _e77 = index;
        if ((_e77 <= 12)) {
        } else {
            break;
        }
        {
            int _e80 = index;
            if ((abs(_e80) > limit)) {
                continue;
            }
            int _e83 = index;
            float position = float(_e83);
            float weight = exp(((-(position) * position) / ((2.0 * sigma) * sigma)));
            vec2 uv = (sampleUV + (input_.parameters.xy * position));
            vec4 pixel_1 = textureGrad(_group_0_binding_1_fs, vec2(uv), gradientsX, gradientsY);
            vec4 premultiplied = ((mode > 3.5) ? vec4(min(pixel_1.x, pixel_1.w)) : ((mode > 0.5) ? pixel_1 : vec4((pixel_1.xyz * pixel_1.w), pixel_1.w)));
            vec4 _e114 = result;
            result = (_e114 + (premultiplied * weight));
            float _e117 = total;
            total = (_e117 + weight);
        }
    }
    vec4 _e122 = result;
    float _e123 = total;
    result = (_e122 / vec4(_e123));
    if ((mode > 1.5)) {
        local_2 = (mode < 2.5);
    } else {
        local_2 = false;
    }
    bool _e133 = local_2;
    if (!(_e133)) {
        local_3 = (mode > 4.5);
    } else {
        local_3 = true;
    }
    bool _e140 = local_3;
    if (_e140) {
        vec2 pixel_2 = (input_.uv * vec2(uvec2(textureSize(_group_0_binding_1_fs, 0).xy)));
        bool _e153 = insideRoundedBox((pixel_2 - input_.box.xy), input_.box.zw, input_.radiusX, input_.radiusY);
        if (_e153) {
            _fs2p_location0 = vec4(0.0);
            return;
        }
    }
    float alpha_1 = input_.color.w;
    vec4 _e158 = result;
    float _e165 = result.w;
    _fs2p_location0 = vec4(((_e158.xyz * input_.color.xyz) * alpha_1), (_e165 * alpha_1));
    return;
}

