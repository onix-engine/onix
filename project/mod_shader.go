package project

import (
	"errors"
	"sync"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/scene"
)

// Programs and their native source blobs are shared within the module's store.
// Uniforms live in each entity's Shader component.
type modShaderResource struct {
	sprite  *graphics.Material2DShader
	mesh    *graphics.MeshShader
	sources shaderResource
	mu      sync.Mutex
}

func (s *modShaderResource) dispose() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var spriteErr, meshErr error
	if s.sprite != nil {
		spriteErr = s.sprite.Dispose()
	}
	if s.mesh != nil {
		meshErr = s.mesh.Dispose()
	}
	return errors.Join(spriteErr, meshErr, s.sources.dispose())
}

func (r *Resources) modShaderSources(compiled ShaderSources) (*modShaderResource, error) {
	return r.modShaders.load(compiled, func() (*modShaderResource, error) {
		sources, err := (ShaderSpec{ID: "element", ShaderSources: compiled}).load(r.source, ".")
		if err != nil {
			return nil, err
		}
		return &modShaderResource{sources: sources}, nil
	}, (*modShaderResource).dispose)
}

func (s *modShaderResource) spriteProgram() (*graphics.Material2DShader, error) {
	if s.sprite == nil {
		var err error
		s.sprite, err = graphics.NewMaterial2DShader(s.sources.descriptor)
		return s.sprite, err
	}
	return s.sprite, nil
}

func (s *modShaderResource) meshProgram() (*graphics.MeshShader, error) {
	if s.mesh == nil {
		var err error
		s.mesh, err = graphics.NewMeshShader(s.sources.descriptor)
		return s.mesh, err
	}
	return s.mesh, nil
}

func appendModShader[T graphics.Material2DShader | graphics.MeshShader](r *Resources, b *ecs.Bundle, spec *ShaderComponentSpec, program func(*modShaderResource) (*T, error)) error {
	shader := scene.Shader[T]{Custom: spec.Custom}
	if spec.Compiled.HasSource() {
		resource, err := r.modShaderSources(spec.Compiled)
		if err != nil {
			return err
		}
		resource.mu.Lock()
		shader.Program, err = program(resource)
		resource.mu.Unlock()
		if err != nil {
			return err
		}
	} else if spec.wgslSources().HasSource() {
		return errors.New("shader contains uncompiled WGSL; run the editor resource build first")
	}
	b.Put(shader)
	return nil
}
