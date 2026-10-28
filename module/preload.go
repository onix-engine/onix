package module

import (
	"context"
	"fmt"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/resource"
)

type preparedContent struct {
	store  *privateResources
	bundle ecs.Bundle
}

type preparedSceneContent struct {
	stores map[project.ModID]*privateResources
	items  []preparedContent
	index  int
}

func (c *preparedSceneContent) discard() {
	for _, store := range c.stores {
		_ = store.store.Dispose()
	}
	c.stores, c.items = nil, nil
}

// ScenePreloader prepares CPU content on a worker, then commits startup and
// budgets GPU uploads on the owning render thread. Lifecycle callbacks always
// run on the caller's thread and remain indivisible work units.
type ScenePreloader struct {
	manager *SceneManager
	request ScenePreloadRequest
	cpu     *resource.Preparation[*preparedSceneContent]
	scene   *Scene
	closed  bool
}

func (m *SceneManager) BeginPreload(request ScenePreloadRequest) (*ScenePreloader, error) {
	r := m.runtime
	if r.closed {
		return nil, ErrDisposed
	}
	if m.preload != nil || m.switching || len(m.switches) != 0 || r.busy || r.loading || r.preparation != nil {
		return nil, ErrBusy
	}
	for _, s := range r.scenes {
		if s.ID() == request.Scene {
			return nil, ErrUnavailable
		}
	}
	var plan *project.ScenePlan
	for i := range r.plan.Scenes {
		if r.plan.Scenes[i].ID == request.Scene {
			plan = &r.plan.Scenes[i]
			break
		}
	}
	if plan == nil {
		return nil, fmt.Errorf("scene %q is not registered", request.Scene)
	}
	// Existing stores already belong to the foreground. Leave their cached
	// component construction on the loop thread rather than sharing mutable
	// module caches with a worker or extending their lifetime from that worker.
	existing := make(map[project.ModID]bool, len(r.contentResources))
	for id := range r.contentResources {
		existing[id] = true
	}
	request.Startup = cloneValue(request.Startup)
	p := &ScenePreloader{manager: m, request: request}
	p.cpu = resource.BeginPreparation(func(ctx context.Context) (*preparedSceneContent, error) {
		content := &preparedSceneContent{stores: make(map[project.ModID]*privateResources)}
		for _, instance := range plan.Instances {
			if int(instance.Mod) >= len(r.plan.Mods) {
				return content, ErrInvalid
			}
			mod := &r.plan.Mods[instance.Mod]
			for index := -1; index < len(mod.Elements); index++ {
				if err := ctx.Err(); err != nil {
					return content, err
				}
				item := preparedContent{}
				if !existing[mod.ID] && r.options.Content == nil {
					store := content.stores[mod.ID]
					if store == nil {
						resources, err := project.OpenModResources(r.options.Source, mod, r.options.Resources)
						if err != nil {
							return content, err
						}
						store = &privateResources{store: resources}
						content.stores[mod.ID] = store
					}
					spec := mod.Spec.Components
					if index >= 0 {
						spec = mod.Elements[index].Spec.Components
					}
					bundle, err := store.store.ModContent(spec)
					if err != nil {
						return content, err
					}
					item = preparedContent{store: store, bundle: bundle}
				}
				content.items = append(content.items, item)
			}
		}
		return content, nil
	}, func(content *preparedSceneContent) {
		if content != nil {
			content.discard()
		}
	}, r.options.Wake)
	m.preload = p
	return p, nil
}

func (p *ScenePreloader) Scene() *Scene { return p.scene }

func (p *ScenePreloader) Ready() bool {
	return p.ReadyFor(p.device())
}

func (p *ScenePreloader) ReadyFor(device *render.Device) bool {
	return p.CPUReady() && p.scene.state != SceneLoading && (device.Driver() == nil || p.scene.uploads.ReadyFor(device))
}

func (p *ScenePreloader) CPUReady() bool {
	return p != nil && !p.closed && p.scene != nil && p.scene.Alive()
}

func (p *ScenePreloader) GPUReadyFor(device *render.Device) bool {
	return p.CPUReady() && p.scene.GPUReadyFor(device)
}

func (p *ScenePreloader) Advance(budget time.Duration) (bool, error) {
	return p.AdvanceFor(p.device(), graphics.UploadBudget{Time: budget})
}

func (p *ScenePreloader) AdvanceFor(device *render.Device, budget graphics.UploadBudget) (ready bool, err error) {
	if p == nil || p.closed || p.manager.runtime.closed {
		return false, ErrDisposed
	}
	m, r := p.manager, p.manager.runtime
	if r.busy || r.loading || r.preparation != nil || m.switching {
		return false, ErrBusy
	}
	if p.scene != nil && !p.scene.Alive() {
		return false, ErrDisposed
	}
	if p.ReadyFor(device) {
		return true, nil
	}
	defer func() {
		if !ready && err == nil && p.CPUReady() && r.options.Wake != nil {
			r.options.Wake()
		}
	}()
	started := time.Now()
	if p.scene == nil {
		content, ready, err := p.cpu.Take()
		if err != nil {
			p.Discard()
			return false, err
		}
		if !ready {
			return false, nil
		}
		for id, store := range content.stores {
			if r.contentResources[id] != nil {
				// Foreground work may have loaded the same definition meanwhile.
				_ = store.store.Dispose()
			} else {
				r.contentResources[id] = store
			}
		}
		r.preparedContent = content
		m.switching = true
		p.scene, err = m.prepare(p.request.Scene, SceneStartup{Value: p.request.Startup})
		m.switching = false
		r.preparedContent = nil
		clear(content.items)
		for id, store := range content.stores {
			if store.refs == 0 && r.contentResources[id] == store {
				delete(r.contentResources, id)
				_ = store.store.Dispose()
			}
		}
		if err != nil {
			p.cpu.Finish(err)
			p.Discard()
			return false, err
		}
		p.scene.state = SceneLoading
		// Startup, layout and text measurement were one committed CPU unit.
		// Begin GPU work on the next call if that unit consumed the budget.
		remaining := budget.Time - time.Since(started)
		if remaining <= 0 {
			return false, nil
		}
		budget.Time = remaining
	}
	ready, err = p.scene.AdvanceGPUFor(device, budget)
	if err != nil {
		p.cpu.Finish(err)
		p.Discard()
		return false, err
	}
	if ready || device.Driver() == nil {
		if p.scene.state == SceneLoading {
			p.scene.state = SceneReady
		}
		if ready {
			p.cpu.Finish(nil)
		}
		if m.preload == p {
			m.preload = nil
		}
		return true, nil
	}
	return false, nil
}

func (p *ScenePreloader) Discard() {
	if p == nil || p.closed {
		return
	}
	p.closed = true
	p.cpu.Cancel()
	if p.scene != nil && p.scene.state != SceneActive && p.scene.state != SceneSuspended {
		_ = p.scene.Dispose()
	}
	if p.manager.preload == p {
		p.manager.preload = nil
	}
}

func (s *Scene) GPUReadyFor(device *render.Device) bool {
	return s != nil && s.Alive() && device.Driver() != nil && s.uploads.ReadyFor(device)
}

func (s *Scene) AdvanceGPUFor(device *render.Device, budget graphics.UploadBudget) (bool, error) {
	if s == nil || !s.Alive() {
		return false, ErrDisposed
	}
	return s.uploads.Advance(device, budget)
}

func (p *ScenePreloader) device() *render.Device {
	if p == nil {
		return nil
	}
	return graphics.DeviceFor(p.manager.runtime.world)
}
