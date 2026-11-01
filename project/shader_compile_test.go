package project

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/internal/typedbin"
)

type shaderCompileTestCompiler struct{}

type metalShaderCompileTestCompiler struct{}

func decodeResourceManifest(data []byte) (resourceManifest, error) {
	return typedbin.Unmarshal[resourceManifest](data)
}

func (shaderCompileTestCompiler) CompileShader(request ShaderCompileRequest) (ShaderCompileResult, error) {
	if request.Target == ShaderTargetVulkan {
		return ShaderCompileResult{Fragment: []byte{0x03, 0x02, 0x23, 0x07}, HasFragmentEntry: true}, nil
	}
	return ShaderCompileResult{Fragment: []byte("generated fragment"), HasFragmentEntry: true, WebGL2Generated: request.Target == ShaderTargetWebGL2}, nil
}

func (metalShaderCompileTestCompiler) CompileShader(ShaderCompileRequest) (ShaderCompileResult, error) {
	return ShaderCompileResult{Metal: []byte("generated metal"), HasVertexEntry: true, HasFragmentEntry: true}, nil
}

type shaderCompileOverlayFS struct {
	base      fs.FS
	generated fstest.MapFS
}

func (f shaderCompileOverlayFS) Open(name string) (fs.File, error) {
	if _, ok := f.generated[name]; ok {
		return f.generated.Open(name)
	}
	return f.base.Open(name)
}

func preprocessShaderTestSource(source fs.FS, root string) (fs.FS, error) {
	files, err := PreprocessManifests(source, PreprocessOptions{
		Root:           root,
		ShaderTarget:   ShaderTargetVulkan,
		ShaderCompiler: shaderCompileTestCompiler{},
	})
	if err != nil {
		return nil, err
	}
	generated := make(fstest.MapFS, len(files))
	for _, file := range files {
		if !file.Remove && file.Data != nil {
			generated[file.Path] = &fstest.MapFile{Data: file.Data, Mode: 0o444}
		}
	}
	return shaderCompileOverlayFS{base: source, generated: generated}, nil
}

func TestPreprocessWGSLShader(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"shaders":[{"id":"effect","wgslFragment":"shaders/effect.wgsl"}]}`)},
		"shaders/effect.wgsl":      {Data: []byte("@fragment fn sprite_fragment() -> @location(0) vec4<f32> { return vec4<f32>(1.0); }")},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{
		Root:           ".",
		ShaderTarget:   ShaderTargetWebGL2,
		ShaderCompiler: shaderCompileTestCompiler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var manifest resourceManifest
	var generated, removed bool
	for _, file := range files {
		switch file.Path {
		case "resources/resources.json":
			manifest, err = decodeResourceManifest(file.Data)
			if err != nil {
				t.Fatal(err)
			}
		case "shaders/effect.wgsl":
			removed = file.Remove
		default:
			if file.SourcePath == "shaders/effect.wgsl" && string(file.Data) == "generated fragment" {
				generated = true
			}
		}
	}
	if len(manifest.Shaders) != 1 || manifest.Shaders[0].WebGL2Fragment == "" || !manifest.Shaders[0].WebGL2Generated {
		t.Fatalf("compiled shader manifest = %+v", manifest.Shaders)
	}
	if !generated || !removed {
		t.Fatalf("generated=%v removed=%v", generated, removed)
	}
}

func TestPreprocessWGSLShaderIncludesOnlySelectedBackend(t *testing.T) {
	for _, target := range []ShaderTarget{ShaderTargetVulkan, ShaderTargetWebGL2, ShaderTargetDirectX} {
		t.Run(target.String(), func(t *testing.T) {
			source := fstest.MapFS{
				"resources/resources.json": {Data: []byte(`{"shaders":[{"id":"effect","wgslFragment":"shaders/effect.wgsl"}]}`)},
				"shaders/effect.wgsl":      {Data: []byte("@fragment fn sprite_fragment() -> @location(0) vec4<f32> { return vec4<f32>(1.0); }")},
			}
			files, err := PreprocessManifests(source, PreprocessOptions{Root: ".", ShaderTarget: target, ShaderCompiler: shaderCompileTestCompiler{}})
			if err != nil {
				t.Fatal(err)
			}
			var manifest resourceManifest
			generated := 0
			for _, file := range files {
				if file.Path == "resources/resources.json" {
					manifest, err = decodeResourceManifest(file.Data)
					if err != nil {
						t.Fatal(err)
					}
				} else if !file.Remove {
					generated++
				}
			}
			if len(manifest.Shaders) != 1 || generated != 1 {
				t.Fatalf("generated shaders=%d, manifest=%+v", generated, manifest)
			}
			shader := manifest.Shaders[0]
			if (shader.VulkanFragment != "") != (target == ShaderTargetVulkan) ||
				(shader.WebGL2Fragment != "") != (target == ShaderTargetWebGL2) ||
				(shader.DirectXFragment != "") != (target == ShaderTargetDirectX) || shader.Metal != "" || shader.WebGPUFragment != "" {
				t.Fatalf("unselected sources retained: %+v", shader)
			}
		})
	}
}

func TestPreprocessWGSLShaderRequiresConcreteTarget(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"shaders":[{"id":"effect","wgslFragment":"shaders/effect.wgsl"}]}`)},
		"shaders/effect.wgsl":      {Data: []byte("@fragment fn sprite_fragment() -> @location(0) vec4<f32> { return vec4<f32>(1.0); }")},
	}
	if _, err := PreprocessManifests(source, PreprocessOptions{Root: ".", ShaderTarget: ShaderTargetAll}); err == nil {
		t.Fatal("WGSL preprocessing without a concrete target succeeded")
	}
}

func TestPreprocessWGSLMetalModuleGeneratedOnce(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"shaders":[{"id":"effect","wgslVertex":"./shaders/effect.wgsl","wgslFragment":"shaders/effect.wgsl"}]}`)},
		"shaders/effect.wgsl": {Data: []byte(`
@vertex fn sprite_vertex() -> @builtin(position) vec4<f32> { return vec4<f32>(0.0); }
@fragment fn sprite_fragment() -> @location(0) vec4<f32> { return vec4<f32>(1.0); }
`)},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{
		Root:           ".",
		ShaderTarget:   ShaderTargetMetal,
		ShaderCompiler: metalShaderCompileTestCompiler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	generated := 0
	for _, file := range files {
		if file.SourcePath == "shaders/effect.wgsl" && string(file.Data) == "generated metal" {
			generated++
		}
	}
	if generated != 1 {
		t.Fatalf("generated Metal modules = %d, want 1", generated)
	}
}
