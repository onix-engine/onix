struct Input {
    @location(0) uv: vec2<f32>,
    @location(1) color: vec4<f32>,
    @location(2) @interpolate(flat) parameters: vec4<f32>,
    @location(3) @interpolate(flat) box: vec4<f32>,
    @location(4) @interpolate(flat) radiusX: vec4<f32>,
    @location(5) @interpolate(flat) radiusY: vec4<f32>,
};

@group(0) @binding(1) var source: texture_2d<f32>;
@group(0) @binding(2) var sourceSampler: sampler;

fn insideRoundedBox(point: vec2<f32>, size: vec2<f32>, rx: vec4<f32>, ry: vec4<f32>) -> bool {
    if (point.x < 0.0 || point.y < 0.0 || point.x >= size.x || point.y >= size.y) { return false; }
    var center = vec2<f32>(0.0);
    var radius = vec2<f32>(0.0);
    if (point.x < rx.x && point.y < ry.x) {
        radius = vec2<f32>(rx.x, ry.x);
        center = radius;
    } else if (point.x > size.x - rx.y && point.y < ry.y) {
        radius = vec2<f32>(rx.y, ry.y);
        center = vec2<f32>(size.x - radius.x, radius.y);
    } else if (point.x > size.x - rx.z && point.y > size.y - ry.z) {
        radius = vec2<f32>(rx.z, ry.z);
        center = size - radius;
    } else if (point.x < rx.w && point.y > size.y - ry.w) {
        radius = vec2<f32>(rx.w, ry.w);
        center = vec2<f32>(radius.x, size.y - radius.y);
    }
    if (radius.x <= 0.0 || radius.y <= 0.0) { return true; }
    let normalized = (point - center) / radius;
    return dot(normalized, normalized) <= 1.0;
}

@fragment fn sprite_fragment(input: Input) -> @location(0) vec4<f32> {
    let mode = abs(input.parameters.w);
    let sampleUV = vec2<f32>(input.uv.x, select(input.uv.y, 1.0 - input.uv.y, input.parameters.w < 0.0));
    let gradientsX = dpdx(sampleUV);
    let gradientsY = dpdy(sampleUV);
    if (mode > 2.5) {
        let pixel = textureSampleGrad(source, sourceSampler, sampleUV, gradientsX, gradientsY);
        return vec4<f32>(pixel.rgb * input.color.rgb * input.color.a, pixel.a * input.color.a);
    }
    let sigma = max(input.parameters.z, 0.001);
    let limit = i32(ceil(sigma * 3.0));
    var result = vec4<f32>(0.0);
    var total = 0.0;
    for (var index = -12; index <= 12; index = index + 1) {
        if (abs(index) > limit) { continue; }
        let position = f32(index);
        let weight = exp(-position * position / (2.0 * sigma * sigma));
        let uv = sampleUV + input.parameters.xy * position;
        let pixel = textureSampleGrad(source, sourceSampler, uv, gradientsX, gradientsY);
        let premultiplied = select(vec4<f32>(pixel.rgb * pixel.a, pixel.a), pixel, mode > 0.5);
        result = result + premultiplied * weight;
        total = total + weight;
    }
    result = result / total;
    if (mode > 1.5) {
        let pixel = input.uv * vec2<f32>(textureDimensions(source));
        if (insideRoundedBox(pixel - input.box.xy, input.box.zw, input.radiusX, input.radiusY)) {
            return vec4<f32>(0.0);
        }
    }
    let alpha = input.color.a;
    return vec4<f32>(result.rgb * input.color.rgb * alpha, result.a * alpha);
}
