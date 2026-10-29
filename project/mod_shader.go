package project

import (
	"errors"
	"fmt"
	"sync"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

// Programs and their native source blobs are shared within the module's store.
// Uniforms live in each entity's Shader component.
type modShaderResource struct {
	mu      sync.Mutex
	sources shaderResource
	sprite  *graphics.Material2DShader
	mesh    *graphics.MeshShader
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

func (r *Resources) modShader(b *ecs.Bundle, spec *ShaderComponentSpec, mesh bool) error {
	if spec == nil {
		return nil
	}
	if !spec.HasSource() {
		if mesh {
			b.Put(scene.Shader[graphics.MeshShader]{Custom: spec.Custom})
		} else {
			b.Put(scene.Shader[graphics.Material2DShader]{Custom: spec.Custom})
		}
		return nil
	}
	if !spec.Compiled.HasSource() {
		return fmt.Errorf("shader contains uncompiled WGSL; run the editor resource build first")
	}
	resource, err := r.modShaders.load(spec.Compiled, func() (*modShaderResource, error) {
		sources, err := (ShaderSpec{ID: "element", ShaderSources: spec.Compiled}).load(r.source, ".")
		if err != nil {
			return nil, err
		}
		return &modShaderResource{sources: sources}, nil
	}, (*modShaderResource).dispose)
	if err != nil {
		return err
	}
	resource.mu.Lock()
	defer resource.mu.Unlock()
	if mesh {
		if resource.mesh == nil {
			resource.mesh, err = graphics.NewMeshShader(resource.sources.descriptor)
		}
		if err == nil {
			b.Put(scene.Shader[graphics.MeshShader]{Program: resource.mesh, Custom: spec.Custom})
		}
	} else {
		if resource.sprite == nil {
			resource.sprite, err = graphics.NewMaterial2DShader(resource.sources.descriptor)
		}
		if err == nil {
			b.Put(scene.Shader[graphics.Material2DShader]{Program: resource.sprite, Custom: spec.Custom})
		}
	}
	return err
}
