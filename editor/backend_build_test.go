package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/onix-engine/onix/project"
	"github.com/onix-engine/onix/render"
)

func TestBackendBuildFlagsAndShaderTargets(t *testing.T) {
	for _, platform := range buildPlatforms() {
		for _, backend := range render.Backends(string(platform)) {
			for _, mode := range []BuildMode{BuildModeDebug, BuildModeRelease} {
				args := appendBuildFlags([]string{"build"}, mode, backend.BuildTag())
				if platform == BuildPlatformWindows {
					args = windowsBuildArgs(mode, "", backend.BuildTag())
				} else if platform == BuildPlatformMacOS {
					args = macOSBuildArgs(mode, backend.BuildTag())
				}
				index := slices.Index(args, "-tags")
				if index < 0 || index+1 == len(args) {
					t.Fatalf("%s/%s has no backend tag: %v", platform, backend, args)
				}
				tags := strings.Split(args[index+1], ",")
				for _, candidate := range render.Backends(string(platform)) {
					if slices.Contains(tags, candidate.BuildTag()) != (candidate == backend) {
						t.Fatalf("%s/%s: incorrect backend tags %v", platform, backend, tags)
					}
				}
				if target := shaderTargetForBackend(backend); target == project.ShaderTargetAll {
					t.Fatalf("%s/%s compiles all shader targets", platform, backend)
				}
			}
		}
	}
}

func TestWebGLLoaderHasNoWebGPUDependency(t *testing.T) {
	options := DefaultBuildOptions(t.TempDir())
	options.Platform, options.Backend = BuildPlatformWeb, render.BackendWebGL
	vars := wasmTemplateVars(options, false)
	if vars["__WEB_RENDERER_SCRIPT__"] != "" || vars["__WEB_RENDERER_CHECK__"] != "" {
		t.Fatal("WebGL loader includes WebGPU runtime or capability checks")
	}
	options.Backend = render.BackendWebGPU
	vars = wasmTemplateVars(options, false)
	if !strings.Contains(vars["__WEB_RENDERER_SCRIPT__"], "runtime/webgpu.js") || !strings.Contains(vars["__WEB_RENDERER_CHECK__"], "navigator.gpu") {
		t.Fatal("WebGPU loader is missing its runtime or capability check")
	}
}

func TestNagaDirectXUsesIndividualSamplers(t *testing.T) {
	result, err := (NagaShaderCompiler{}).CompileShader(project.ShaderCompileRequest{
		ID: "texture", SourcePath: "shaders/texture.wgsl", Target: project.ShaderTargetDirectX,
		Source: []byte(`@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var sampleState: sampler;
@fragment fn sprite_fragment() -> @location(0) vec4<f32> {
    return textureSample(tex, sampleState, vec2<f32>(0.5));
}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	source := string(result.Fragment)
	if !strings.Contains(source, "register(t0)") || !strings.Contains(source, "register(s0)") || strings.Contains(source, "nagaSamplerHeap") || strings.Contains(source, "SamplerIndexArray") {
		t.Fatalf("DirectX shader does not bind D3D11 texture/sampler slots:\n%s", source)
	}
}
