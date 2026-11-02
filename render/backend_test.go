package render

import (
	"go/build"
	"slices"
	"strings"
	"testing"
)

func TestBackendBuildIsolation(t *testing.T) {
	for platform, want := range map[string]Backend{"linux": BackendOpenGL, "android": BackendVulkan} {
		if got, err := ResolveBackend(platform, ""); err != nil || got != want {
			t.Fatalf("%s default = %s, %v; want %s", platform, got, err, want)
		}
	}
	implementations := map[Backend]string{
		BackendDirectX: "directx_native.c", BackendMetal: "metal_native.m",
		BackendVulkan: "vulkan_native.c", BackendOpenGL: "opengles.go",
		BackendWebGPU: "webgpu_js.go", BackendWebGL: "webgl2_js.go",
	}
	for _, platform := range []string{"windows", "macos", "linux", "web", "wechat", "android", "ios"} {
		for index, backend := range Backends(platform) {
			for _, explicit := range []bool{false, true} {
				if !explicit && index != 0 {
					continue
				}
				t.Run(platform+"/"+string(backend)+"/explicit="+map[bool]string{false: "false", true: "true"}[explicit], func(t *testing.T) {
					context := build.Default
					context.GOOS, context.GOARCH, context.CgoEnabled = platform, "amd64", true
					context.BuildTags = nil
					switch platform {
					case "macos":
						context.GOOS = "darwin"
					case "web", "wechat":
						context.GOOS, context.GOARCH, context.CgoEnabled = "js", "wasm", false
						if platform == "wechat" {
							context.BuildTags = append(context.BuildTags, "wechat")
						}
					case "android", "ios":
						context.GOARCH = "arm64"
					}
					if explicit {
						context.BuildTags = append(context.BuildTags, backend.BuildTag())
					}
					pkg, err := context.ImportDir(".", 0)
					if err != nil {
						t.Fatal(err)
					}
					files := append(append(append(append(slices.Clone(pkg.GoFiles), pkg.CgoFiles...), pkg.CFiles...), pkg.MFiles...), pkg.CXXFiles...)
					for candidate, file := range implementations {
						if got := slices.Contains(files, file); got != (candidate == backend) {
							t.Errorf("%s included=%v; selected %s", file, got, backend)
						}
					}
					if slices.Contains(files, "backend_invalid.go") {
						t.Fatal("valid build includes invalid-backend guard")
					}
					// Embedding an unselected shader also makes its renderer leak into a build.
					for _, pattern := range pkg.EmbedPatterns {
						if backend != BackendVulkan && strings.Contains(pattern, "vulkan_") ||
							backend != BackendDirectX && strings.Contains(pattern, "directx_") ||
							backend != BackendOpenGL && backend != BackendWebGL && strings.Contains(pattern, "gles_") ||
							backend != BackendWebGPU && strings.HasSuffix(pattern, ".wgsl") {
							t.Errorf("unselected shader embedded: %s", pattern)
						}
					}
					if platform == "linux" {
						for _, drm := range []bool{false, true} {
							windowContext := context
							if drm {
								windowContext.BuildTags = append(slices.Clone(context.BuildTags), "onix_drm")
							}
							for file, want := range map[string]bool{
								"render_opengl_native.go": backend == BackendOpenGL && !drm,
								"render_vulkan_native.go": backend == BackendVulkan && !drm,
								"drm_gles_linux.go":       backend == BackendOpenGL && drm,
								"drm_vulkan_linux.go":     backend == BackendVulkan && drm,
							} {
								if got, err := windowContext.MatchFile("../window", file); err != nil || got != want {
									t.Errorf("%s drm=%v: included=%v, error=%v; want %v", file, drm, got, err, want)
								}
							}
						}
					}
					if slices.Contains(files, "directx_composition.cc") != (backend == BackendDirectX) {
						t.Fatal("DirectX composition compiled with another backend")
					}
				})
			}
		}
	}
}

func TestBackendRejectsInvalidSelections(t *testing.T) {
	for _, selection := range []struct {
		platform string
		tags     []string
	}{
		{"windows", []string{"onix_vulkan", "onix_directx"}},
		{"linux", []string{"onix_metal"}},
		{"js", []string{"wechat", "onix_webgpu"}},
	} {
		context := build.Default
		context.GOOS, context.GOARCH = selection.platform, "amd64"
		if selection.platform == "js" {
			context.GOARCH = "wasm"
		}
		context.BuildTags = selection.tags
		matched, err := context.MatchFile(".", "backend_invalid.go")
		if err != nil || !matched {
			t.Errorf("%s %v: guard=%v, error=%v", selection.platform, selection.tags, matched, err)
		}
	}
	if _, err := ResolveBackend("wechat", BackendWebGPU); err == nil {
		t.Fatal("WeChat accepted WebGPU")
	}
}
