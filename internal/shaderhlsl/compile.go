package shaderhlsl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gogpu/naga/hlsl"
	"github.com/gogpu/naga/ir"
)

func Compile(module *ir.Module, entrypoint int) ([]byte, error) {
	options := hlsl.DefaultOptions()
	options.ShaderModel = hlsl.ShaderModel5_0
	options.EntryPoint = module.EntryPoints[entrypoint].Name
	options.BindingMap = make(map[hlsl.ResourceBinding]hlsl.BindTarget)
	for _, global := range module.GlobalVariables {
		if global.Binding == nil {
			continue
		}
		binding := global.Binding
		slot := uint32(0)
		if global.Space == ir.SpaceStorage {
			slot = 16 + binding.Binding
		} else if global.Space != ir.SpaceUniform {
			if binding.Group != 0 {
				slot = binding.Group
			} else if binding.Binding >= 3 {
				slot = (binding.Binding - 2) / 2
				if binding.Binding <= 4 {
					slot = 1
				}
			}
		}
		options.BindingMap[hlsl.ResourceBinding{Group: binding.Group, Binding: binding.Binding}] = hlsl.BindTarget{Register: slot}
	}
	source, _, err := hlsl.Compile(module, options)
	if err != nil {
		return nil, err
	}
	// D3D11 binds individual samplers. Naga's descriptor heaps target D3D12.
	source = regexp.MustCompile(`(?m)^(?:SamplerState nagaSamplerHeap|SamplerComparisonState nagaComparisonSamplerHeap|StructuredBuffer<uint> nagaGroup\d+SamplerIndexArray)[^;]*;\n`).ReplaceAllString(source, "")
	samplerIndex := 0
	source = regexp.MustCompile(`static const SamplerState (\w+) = nagaSamplerHeap\[[^;]+;`).ReplaceAllStringFunc(source, func(declaration string) string {
		name := strings.Fields(declaration)[3]
		for samplerIndex < len(module.GlobalVariables) {
			global := module.GlobalVariables[samplerIndex]
			samplerIndex++
			if _, ok := module.Types[global.Type].Inner.(ir.SamplerType); !ok || global.Binding == nil {
				continue
			}
			slot := options.BindingMap[hlsl.ResourceBinding{Group: global.Binding.Group, Binding: global.Binding.Binding}].Register
			return fmt.Sprintf("SamplerState %s : register(s%d);", name, slot)
		}
		return declaration
	})
	source = strings.ReplaceAll(source, "while(true)", "[loop] while(true)")
	source = alignFragmentInputs(source, options.EntryPoint)
	return []byte(source), nil
}

// D3D11 links stage registers as well as semantics. Keep unused native varyings
// in the input declaration so a reduced fragment variant still matches the
// shared vertex shader's register order.
func alignFragmentInputs(source, entry string) string {
	if !strings.HasSuffix(entry, "_fragment") {
		return source
	}
	pattern := regexp.MustCompile(`(?s)(struct FragmentInput_` + regexp.QuoteMeta(entry) + ` \{)(.*?)(\n\};)`)
	locationPattern := regexp.MustCompile(`\bLOC(\d+);`)
	return pattern.ReplaceAllStringFunc(source, func(declaration string) string {
		parts := pattern.FindStringSubmatch(declaration)
		fields := map[int]string{}
		var builtins []string
		for _, line := range strings.Split(parts[2], "\n") {
			if location := locationPattern.FindStringSubmatch(line); location != nil {
				index, _ := strconv.Atoi(location[1])
				fields[index] = line
			} else if strings.TrimSpace(line) != "" {
				builtins = append(builtins, line)
			}
		}
		var types []string
		switch entry {
		case "mesh_fragment":
			if strings.Contains(fields[0], "uint ") { // Integer picking uses its own vertex shader.
				return declaration
			}
			types = []string{"float3", "float3", "float2", "float4", "float4", "nointerpolation float4", "nointerpolation float4", "nointerpolation float4", "nointerpolation float4", "nointerpolation float4", "float4"}
		case "sprite_fragment":
			types = []string{"float2", "float4", "nointerpolation float4", "nointerpolation float4", "nointerpolation float4", "nointerpolation float4"}
			if strings.Contains(fields[2], "uint ") {
				types[2], types[3] = "nointerpolation uint", "float2"
			}
		default:
			return declaration
		}
		var result strings.Builder
		result.WriteString(parts[1])
		for index, kind := range types {
			result.WriteByte('\n')
			if field, ok := fields[index]; ok {
				result.WriteString(field)
			} else {
				fmt.Fprintf(&result, "    %s onix_unused_%d : LOC%d;", kind, index, index)
			}
		}
		for _, field := range builtins {
			result.WriteByte('\n')
			result.WriteString(field)
		}
		result.WriteString(parts[3])
		return result.String()
	})
}
