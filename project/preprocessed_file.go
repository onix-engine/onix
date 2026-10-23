package project

import "github.com/diiyw/onix/graphics"

// PreprocessedFile is a source resource replacement or removal.
type PreprocessedFile struct {
	Path string
	// SourcePath identifies the source resource's path when Data is generated
	// rather than replacing a file already collected from disk.
	SourcePath string
	Data       []byte
	SourceSize int
	Remove     bool
}

// PreprocessOptions selects the project root and build target for resource
// compilation.
type PreprocessOptions struct {
	Root           string
	ShaderTarget   ShaderTarget
	ShaderCompiler ShaderCompiler
	// Progress reports the module and asset family about to be compiled.
	Progress func(module, asset string, completed, total int)
	// TextureEncoder replaces model images with a runtime texture container.
	// External glTF images are read during model preprocessing so their material
	// sampling options select the same compression profile as embedded images.
	TextureEncoder func([]byte, string, graphics.TextureOptions) ([]byte, error)
}

// ShaderCompiler converts one WGSL module into the native representation for
// a single build target. The compiler is invoked only during preprocessing;
// runtime packages contain the returned native bytes.
type ShaderCompiler interface {
	CompileShader(ShaderCompileRequest) (ShaderCompileResult, error)
}

type ShaderCompileRequest struct {
	ID         string
	SourcePath string
	Source     []byte
	Target     ShaderTarget
}

type ShaderCompileResult struct {
	Vertex           []byte
	Fragment         []byte
	Metal            []byte
	HasVertexEntry   bool
	HasFragmentEntry bool
	WebGL2Generated  bool
}
