package graphics

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/onix-engine/onix/render"
)

var shaderBatchCounter atomic.Uint64

type shaderResource[R render.ShaderResource] struct {
	err         error
	ops         *shaderResourceOps[R]
	residencies render.DeviceValues[render.Handle[R]]
	descriptor  render.ShaderDescriptor
	batchKey    uint64
	handle      render.Handle[R]
	disposed    bool
}

type shaderResourceOps[R render.ShaderResource] struct {
	create      func(render.Driver, render.ShaderDescriptor) (render.Handle[R], bool, error)
	destroy     func(render.Driver, render.Handle[R]) error
	kind        string
	unsupported string
	disposed    string
}

func newShaderResource[R render.ShaderResource](descriptor render.ShaderDescriptor, ops *shaderResourceOps[R]) (*shaderResource[R], error) {
	if err := descriptor.Validate(); err != nil {
		return nil, err
	}
	shader := &shaderResource[R]{descriptor: descriptor, batchKey: shaderBatchCounter.Add(1), ops: ops}
	return shader, nil
}

func (s *shaderResource[R]) Descriptor() render.ShaderDescriptor {
	if s == nil {
		return render.ShaderDescriptor{}
	}
	return s.descriptor
}

func (s *shaderResource[R]) BatchKey() uint64 {
	if s == nil {
		return 0
	}
	return s.batchKey
}

func (s *shaderResource[R]) RenderHandle() render.Handle[R] {
	return s.RenderHandleFor(render.ActiveDevice())
}

func (s *shaderResource[R]) RenderHandleFor(device *render.Device) render.Handle[R] {
	if s == nil {
		return render.Handle[R]{}
	}
	s.ensureUploadedFor(device)
	return s.handle
}

func (s *shaderResource[R]) UploadError() error {
	if s == nil {
		return nil
	}
	return s.err
}

func (s *shaderResource[R]) ensureUploaded() {
	s.ensureUploadedFor(render.ActiveDevice())
}

func (s *shaderResource[R]) ensureUploadedFor(device *render.Device) {
	if s == nil || s.disposed || s.ops == nil {
		return
	}
	ops := s.ops
	if handle, ok := s.residencies.Get(device); ok {
		s.handle, s.err = handle, nil
		return
	}
	s.handle = render.Handle[R]{}
	driver := device.Driver()
	if driver == nil {
		return
	}
	handle, supported, err := ops.create(driver, s.descriptor)
	if !supported {
		s.err = errors.New(ops.unsupported)
		return
	}
	s.handle, s.err = handle, err
	if err != nil {
		s.err = fmt.Errorf("graphics: upload %s shader: %w", ops.kind, err)
		return
	}
	s.residencies.Set(device, s.handle)
}

func (s *shaderResource[R]) Dispose() error {
	if s == nil || s.disposed {
		return nil
	}
	if s.ops == nil {
		return nil
	}
	ops := s.ops
	if err := s.residencies.Release(func(device *render.Device, handle render.Handle[R]) error {
		return ops.destroy(device.Driver(), handle)
	}); err != nil {
		return err
	}
	s.handle = render.Handle[R]{}
	s.descriptor = render.ShaderDescriptor{}
	s.disposed = true
	s.err = errors.New(ops.disposed)
	return nil
}
