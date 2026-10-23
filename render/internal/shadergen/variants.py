"""Fixed built-in specializations; all lighting equations stay in the shared source."""
import re


def remove_block(source, marker):
    start = source.index(marker)
    brace = source.index('{', start)
    level = 1
    end = brace + 1
    while level:
        level += (source[end] == '{') - (source[end] == '}')
        end += 1
    return source[:start] + source[end:]


def prune(source):
    # Remove unreachable functions before validation, then unused input fields
    # and constant tables. Naga's compaction alone retains global constants.
    functions = {}
    for match in re.finditer(r'(?:@fragment )?fn (\w+)\(', source):
        start = match.start()
        brace = source.index('{', match.end())
        level, end = 1, brace + 1
        while level:
            level += (source[end] == '{') - (source[end] == '}')
            end += 1
        functions[match[1]] = (start, end, source[start:end])
    reachable, pending = set(), ['mesh_fragment']
    while pending:
        name = pending.pop()
        if name in reachable:
            continue
        reachable.add(name)
        pending.extend(n for n in re.findall(r'\b(\w+)\(', functions[name][2]) if n in functions and n not in reachable)
    for name, (start, end, _) in sorted(functions.items(), key=lambda item: item[1][0], reverse=True):
        if name not in reachable:
            source = source[:start] + source[end:]
    for name in ['SHEEN_ALBEDO', 'SSAO_DISK']:
        if len(re.findall(r'\b'+name+r'\b', source)) == 1:
            source = re.sub(r'const '+name+r':.*?;\n', '', source, flags=re.S)
    for location, name in enumerate(['worldPosition', 'normal', 'uv', 'baseColor', 'emissiveMetallic', 'material', 'custom0', 'custom1', 'custom2', 'custom3', 'tangent']):
        if not re.search(r'\.'+name+r'\b', source):
            source = re.sub(r'@location\('+str(location)+r'\)\s*(?:@interpolate\(flat\)\s*)?'+name+r':\s*\w+(?:<\w+>)?,', '', source)
    return source


def specialize(wgsl, kind):
    if kind == 'unlit':
        return prune(wgsl[:wgsl.index('    // PBR materials below this point require lighting.')] + '    return vec4<f32>(0.0);\n}\n')
    if kind == 'depth':
        prefix = wgsl[:wgsl.index('@fragment fn mesh_fragment')]
        # Keep winding, sidedness and alpha-test semantics of the shared path.
        return prune(prefix + '''@fragment fn mesh_fragment(input:PBRInput)->@location(0) vec4<f32> {
    let flags=u32(input.material.w+0.5);
    let front=(input.front!=(pbr.renderParams.w>0.5))!=((flags&1024u)!=0u);
    if ((flags&512u)==0u && !front) { discard; }
    if ((flags&3u)==1u) {
        if (textureSample(tex,texSampler,input.uv).a*input.baseColor.a<input.material.z) { discard; }
    }
    let depth=u32(clamp(input.position.z,0.0,1.0)*16777215.0);
    return vec4<f32>(vec3<f32>(f32((depth>>16u)&255u),f32((depth>>8u)&255u),f32(depth&255u))/255.0,1.0);
}
''')
    if kind != 'basic':
        raise ValueError(kind)
    source = wgsl
    for marker in ['    if (s.anisotropy>0.0) {', '    if (any(s.sheen>vec3<f32>(0.0)) && nl>0.0) {', '    if (s.coat>0.0) {']:
        # The anisotropy/reflection and coat/reflection helpers are removed by
        # reachability below; specialize the BRDF itself explicitly.
        start = source.index('fn pbr_brdf(')
        source = source[:start] + remove_block(source[start:], marker)
    a = source.index('    var frame=mat3x3<f32>', source.index('@fragment fn mesh_fragment'))
    b = source.index('    let v=select(', a)
    source = source[:a] + '''    if ((flags&4u)!=0u) {
        n=pbr_mapped_normal(pbr_basis(input,n),textureSample(normalTex,normalSampler,input.uv).xyz*2.0-1.0,input.custom0.x);
    }
''' + source[b:]
    a = source.index('    var sheenResponse=')
    b = source.index('    let ambientF=', a)
    source = source[:a] + '''    let sheenScale=1.0;
    let coatFresnel=0.0;
    let surface=PBRSurface(n,n,vec3<f32>(0.0),vec3<f32>(0.0),base.rgb,metallic,roughness,f0,f90,diffuseF0,specular,0.0,1.0,0.0,vec3<f32>(0.0),1.0,0.0,1.0,0.0,0.0);
''' + source[b:]
    source = source.replace('let reflectionNormal=pbr_reflection_normal(surface,v);', 'let reflectionNormal=n;')
    source = remove_block(source, '        if (any(sheen>vec3<f32>(0.0))) {')
    source = remove_block(source, '        if (coat>0.0) {')
    # Basic is used only for lit/capture/occlusion passes on PBR instances.
    a = source.index('@fragment fn mesh_fragment')
    b = source.index('    // PBR materials below this point require lighting.', a)
    source = source[:a] + '''@fragment fn mesh_fragment(input:PBRInput)->@location(0) vec4<f32> {
    let flags=u32(input.material.w+0.5);
    var base=textureSample(tex,texSampler,input.uv)*input.baseColor;
    if ((flags&3u)==1u && base.a<input.material.z) { discard; }
    let front=(input.front!=(pbr.renderParams.w>0.5))!=((flags&1024u)!=0u);
    if ((flags&512u)==0u && !front) { discard; }
    if ((flags&3u)!=2u) { base.a=1.0; }
    if (pbr.renderParams.y==3.0 && input.material.y<0.0) { return vec4<f32>(0.0); }
''' + source[b:]
    return prune(source)
