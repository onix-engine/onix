package editor

import (
	"fmt"
	"strings"

	"github.com/diiyw/onix/project"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/naga/spirv"
)

const (
	onixNagaCompilerVersion = "gogpu-naga-v0.18.0-onix-1"
	webGLCoordinateEpilogue = "gl_Position.yz = vec2(-gl_Position.y, gl_Position.z * 2.0 - gl_Position.w);"
	webGLDepthOnlyEpilogue  = "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;"
	missingShaderEntrypoint = -1
)

// NagaShaderCompiler compiles WGSL with the pure Go gogpu/naga package.
type NagaShaderCompiler struct{}

type nagaShaderEntrypoints struct {
	pipeline string
	vertex   int
	fragment int
}

func (NagaShaderCompiler) CompileShader(request project.ShaderCompileRequest) (project.ShaderCompileResult, error) {
	if strings.TrimSpace(request.SourcePath) == "" {
		return project.ShaderCompileResult{}, fmt.Errorf("shader %q has no source path", request.ID)
	}
	source := string(request.Source)
	ast, err := naga.Parse(source)
	if err != nil {
		return project.ShaderCompileResult{}, fmt.Errorf("parse WGSL %q: %w", request.SourcePath, err)
	}
	module, err := naga.LowerWithSource(ast, source)
	if err != nil {
		return project.ShaderCompileResult{}, fmt.Errorf("lower WGSL %q: %w", request.SourcePath, err)
	}
	if err := validateNagaModule(module); err != nil {
		return project.ShaderCompileResult{}, fmt.Errorf("validate WGSL %q: %w", request.SourcePath, err)
	}
	entrypoints, err := selectNagaEntrypoints(module)
	if err != nil {
		return project.ShaderCompileResult{}, fmt.Errorf("WGSL %q: %w", request.SourcePath, err)
	}
	result := project.ShaderCompileResult{
		HasVertexEntry:   entrypoints.vertex != missingShaderEntrypoint,
		HasFragmentEntry: entrypoints.fragment != missingShaderEntrypoint,
		WebGL2Generated:  request.Target == project.ShaderTargetWebGL2,
	}

	switch request.Target {
	case project.ShaderTargetVulkan:
		if result.HasVertexEntry {
			result.Vertex, err = compileNagaSPIRV(module, entrypoints.vertex)
		}
		if err == nil && result.HasFragmentEntry {
			result.Fragment, err = compileNagaSPIRV(module, entrypoints.fragment)
		}
	case project.ShaderTargetMetal:
		result.Metal, err = compileNagaMSL(module, entrypoints)
	case project.ShaderTargetWebGL2:
		if result.HasVertexEntry {
			result.Vertex, err = compileNagaGLSL(module, entrypoints.vertex)
		}
		if err == nil && result.HasFragmentEntry {
			result.Fragment, err = compileNagaGLSL(module, entrypoints.fragment)
		}
	case project.ShaderTargetWebGPU:
		return result, nil
	default:
		return project.ShaderCompileResult{}, fmt.Errorf("unsupported shader target %d", request.Target)
	}
	if err != nil {
		return project.ShaderCompileResult{}, fmt.Errorf("compile WGSL %q for %s: %w", request.SourcePath, request.Target, err)
	}
	return result, nil
}

func validateNagaModule(module *ir.Module) error {
	errors, err := naga.Validate(module)
	if err != nil {
		return err
	}
	if len(errors) > 0 {
		return &errors[0]
	}
	return nil
}

func selectNagaEntrypoints(module *ir.Module) (nagaShaderEntrypoints, error) {
	result := nagaShaderEntrypoints{vertex: missingShaderEntrypoint, fragment: missingShaderEntrypoint}
	for index := range module.EntryPoints {
		entrypoint := &module.EntryPoints[index]
		pipeline, stage := "", ir.StageCompute
		switch entrypoint.Name {
		case "sprite_vertex":
			pipeline, stage = "sprite", ir.StageVertex
		case "sprite_fragment":
			pipeline, stage = "sprite", ir.StageFragment
		case "mesh_vertex":
			pipeline, stage = "mesh", ir.StageVertex
		case "mesh_fragment":
			pipeline, stage = "mesh", ir.StageFragment
		default:
			continue
		}
		if entrypoint.Stage != stage {
			return nagaShaderEntrypoints{}, fmt.Errorf("entrypoint %q has the wrong shader stage", entrypoint.Name)
		}
		if result.pipeline != "" && result.pipeline != pipeline {
			return nagaShaderEntrypoints{}, fmt.Errorf("cannot mix sprite and mesh entrypoints")
		}
		result.pipeline = pipeline
		switch stage {
		case ir.StageVertex:
			if result.vertex != missingShaderEntrypoint {
				return nagaShaderEntrypoints{}, fmt.Errorf("contains more than one recognized vertex entrypoint")
			}
			result.vertex = index
		case ir.StageFragment:
			if result.fragment != missingShaderEntrypoint {
				return nagaShaderEntrypoints{}, fmt.Errorf("contains more than one recognized fragment entrypoint")
			}
			result.fragment = index
		}
	}
	if result.pipeline == "" {
		return nagaShaderEntrypoints{}, fmt.Errorf("must define a sprite_* or mesh_* entrypoint")
	}
	return result, nil
}

func nagaNativeModule(module *ir.Module, rename bool, indexes ...int) *ir.Module {
	native := *module
	native.GlobalVariables = append([]ir.GlobalVariable(nil), module.GlobalVariables...)
	for index := range native.GlobalVariables {
		global := &native.GlobalVariables[index]
		if global.Space == ir.SpaceUniform && global.Binding != nil && global.Binding.Group == 0 && global.Binding.Binding == 0 {
			global.Space = ir.SpaceImmediate
			global.Binding = nil
		}
	}
	native.EntryPoints = make([]ir.EntryPoint, 0, len(indexes))
	for _, index := range indexes {
		if index == missingShaderEntrypoint {
			continue
		}
		entrypoint := module.EntryPoints[index]
		if rename {
			entrypoint.Name = "main"
		}
		native.EntryPoints = append(native.EntryPoints, entrypoint)
	}
	return &native
}

func compileNagaSPIRV(module *ir.Module, entrypoint int) ([]byte, error) {
	native := nagaNativeModule(module, true, entrypoint)
	if err := validateNagaModule(native); err != nil {
		return nil, err
	}
	options := spirv.DefaultOptions()
	options.Version = spirv.Version1_0
	options.AdjustCoordinateSpace = true
	return naga.GenerateSPIRV(native, options)
}

func compileNagaMSL(module *ir.Module, entrypoints nagaShaderEntrypoints) ([]byte, error) {
	native := nagaNativeModule(module, false, entrypoints.vertex, entrypoints.fragment)
	if err := validateNagaModule(native); err != nil {
		return nil, err
	}
	textureSlot, immediateSlot := uint8(0), uint8(2)
	mipFilter := msl.SamplerFilterLinear
	resources := msl.EntryPointResources{
		Resources: map[ir.ResourceBinding]msl.BindTarget{
			{Group: 0, Binding: 1}: {Texture: &textureSlot},
			{Group: 0, Binding: 2}: {Sampler: &msl.BindSamplerTarget{IsInline: true, Slot: 0}},
		},
		ImmediatesBuffer: &immediateSlot,
	}
	options := msl.DefaultOptions()
	options.LangVersion = msl.Version2_3
	options.PerEntryPointMap = make(map[string]msl.EntryPointResources, len(native.EntryPoints))
	for index := range native.EntryPoints {
		options.PerEntryPointMap[native.EntryPoints[index].Name] = resources
	}
	options.InlineSamplers = []msl.InlineSampler{{
		Address:   [3]msl.SamplerAddress{msl.SamplerAddressClampToEdge, msl.SamplerAddressClampToEdge, msl.SamplerAddressClampToEdge},
		MagFilter: msl.SamplerFilterLinear,
		MinFilter: msl.SamplerFilterLinear,
		MipFilter: &mipFilter,
	}}
	source, _, err := msl.Compile(native, options)
	return []byte(source), err
}

func compileNagaGLSL(module *ir.Module, entrypoint int) ([]byte, error) {
	native := nagaNativeModule(module, false, entrypoint)
	if err := validateNagaModule(native); err != nil {
		return nil, err
	}
	options := glsl.DefaultOptions()
	options.LangVersion = glsl.VersionES300
	options.EntryPoint = native.EntryPoints[0].Name
	options.WriterFlags = glsl.WriterFlagAdjustCoordinateSpace
	source, _, err := glsl.Compile(native, options)
	if err != nil {
		return nil, err
	}
	if native.EntryPoints[0].Stage == ir.StageVertex {
		if !strings.Contains(source, webGLCoordinateEpilogue) {
			return nil, fmt.Errorf("GLSL vertex output is missing its coordinate-space epilogue")
		}
		source = strings.ReplaceAll(source, webGLCoordinateEpilogue, webGLDepthOnlyEpilogue)
	}
	return []byte(source), nil
}
