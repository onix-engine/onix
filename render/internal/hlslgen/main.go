// Command hlslgen regenerates DirectX builtins from the shared WGSL sources.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diiyw/onix/internal/shaderhlsl"
	"github.com/gogpu/naga"
)

func main() {
	parsed, err := parser.ParseFile(token.NewFileSet(), "render/webgpu_js.go", nil, 0)
	check(err)
	constants := map[string]string{}
	ast.Inspect(parsed, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		if literal, ok := spec.Values[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
			value, err := strconv.Unquote(literal.Value)
			check(err)
			constants[spec.Names[0].Name] = value
		}
		return true
	})
	for _, name := range []string{"sprite", "sprite_multi", "shape", "mesh", "mesh_unlit", "mesh_depth", "mesh_basic", "picking"} {
		for _, stage := range []string{"vert", "frag"} {
			if (name == "mesh_unlit" || name == "mesh_depth" || name == "mesh_basic") && stage == "vert" {
				continue
			}
			suffix, entry := "Vertex", "vertex"
			if stage == "frag" {
				suffix, entry = "Fragment", "fragment"
			}
			key := "webGPU" + strings.ToUpper(name[:1]) + name[1:] + suffix
			source := constants[key]
			if name == "sprite_multi" {
				source = read("render/shaders/sprite_multi.wgsl")
			}
			if strings.HasPrefix(name, "mesh") && stage == "frag" {
				source = read("render/shaders/" + name + ".frag.wgsl")
			}
			if source == "" {
				panic("missing WGSL: " + key)
			}
			ast, err := naga.Parse(source)
			check(err)
			module, err := naga.LowerWithSource(ast, source)
			check(err)
			pipeline := name
			if name == "sprite_multi" {
				pipeline = "sprite"
			}
			if strings.HasPrefix(name, "mesh") || name == "picking" {
				pipeline = "mesh"
			}
			found := false
			for index, point := range module.EntryPoints {
				if point.Name != pipeline+"_"+entry {
					continue
				}
				data, err := shaderhlsl.Compile(module, index)
				check(err)
				path := filepath.Join("render/shaders", "directx_"+name+"."+stage+".hlsl")
				check(os.WriteFile(path, data, 0644))
				found = true
			}
			if !found {
				panic(fmt.Sprintf("missing %s entrypoint in %s", entry, name))
			}
		}
	}
}
func read(path string) string { data, err := os.ReadFile(path); check(err); return string(data) }
func check(err error) {
	if err != nil {
		panic(err)
	}
}
