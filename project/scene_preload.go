package project

import (
	"context"
	"fmt"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/resource"
	sceneapi "github.com/diiyw/onix/scene"
)

type scenePreloadPhase uint8

const (
	scenePreloadCPU scenePreloadPhase = iota
	scenePreloadSpawn
	scenePreloadResolve
	scenePreloadPrepare
	scenePreloadUpload
	scenePreloadReady
)

// ScenePreloader incrementally builds a scene in the active world. Its nodes
// remain hidden and its cameras inactive until Activate is called.
type ScenePreloader struct {
	bundle     *Bundle
	scene      *SceneInstance
	plan       sceneSpawnPlan
	entities   []ecs.Entity
	phase      scenePreloadPhase
	index      int
	cpu        *resource.Preparation[[]ecs.Bundle]
	components []ecs.Bundle
	uploads    graphics.UploadQueue
	wake       func()
}

// BeginPreloadScene starts an incremental scene preload. Advance must be
// called until Ready reports true before Activate may be used.
func (b *Bundle) BeginPreloadScene(world *ecs.World, id SceneID) (*ScenePreloader, error) {
	scene, plan, err := b.newSceneInstance(world, id)
	if err != nil {
		return nil, err
	}
	p := &ScenePreloader{
		bundle:   b,
		scene:    scene,
		plan:     plan,
		entities: make([]ecs.Entity, len(plan.Nodes)),
	}
	request, _ := world.GetResource[*sceneapi.RenderRequest]()
	p.wake = request.Request
	p.cpu = resource.BeginPreparation(func(ctx context.Context) ([]ecs.Bundle, error) {
		values := make([]ecs.Bundle, len(plan.Nodes))
		for i, node := range plan.Nodes {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			components, err := b.ecsComponentsForPlanNode(node)
			if err != nil {
				return nil, err
			}
			values[i] = components
		}
		return values, nil
	}, nil, request.Request)
	return p, nil
}

// Ready reports whether construction, resource loading, and static text
// measurement have completed, and uploads are ready for the owning world's frame device.
func (p *ScenePreloader) Ready() bool {
	return p.ReadyFor(p.device())
}

// ReadyFor permits headless activation after CPU preparation. With a device,
// every queued asset must already be resident on that exact device.
func (p *ScenePreloader) ReadyFor(device *render.Device) bool {
	return p != nil && p.phase == scenePreloadReady && p.scene != nil && (device.Driver() == nil || p.uploads.ReadyFor(device))
}

func (p *ScenePreloader) CPUReady() bool {
	return p != nil && p.scene != nil && p.phase >= scenePreloadUpload
}

func (p *ScenePreloader) GPUReadyFor(device *render.Device) bool {
	return p.CPUReady() && device.Driver() != nil && p.uploads.ReadyFor(device)
}

// Advance builds scene nodes until the supplied time budget is exhausted.
// A non-positive budget still completes one unit of work so callers always
// make progress in deterministic tests.
func (p *ScenePreloader) Advance(budget time.Duration) (bool, error) {
	return p.AdvanceFor(p.device(), graphics.UploadBudget{Time: budget})
}

// AdvanceFor commits CPU results and uploads on the caller's render thread.
// Reading and decoding continue on the worker while this method returns.
func (p *ScenePreloader) AdvanceFor(device *render.Device, budget graphics.UploadBudget) (ready bool, err error) {
	if p == nil || p.scene == nil {
		return false, fmt.Errorf("scene preloader is nil")
	}
	if p.ReadyFor(device) {
		return true, nil
	}
	defer func() {
		if !ready && err == nil && p.scene != nil && p.phase > scenePreloadCPU {
			p.wake()
		}
	}()
	deadline := time.Now().Add(budget.Time)
	for {
		if p.phase == scenePreloadCPU {
			components, ready, err := p.cpu.Take()
			if err != nil {
				p.Discard()
				return false, err
			}
			if !ready {
				return false, nil
			}
			p.components, p.phase = components, scenePreloadSpawn
		}
		if p.phase >= scenePreloadUpload {
			budget.Time = max(0, time.Until(deadline))
			ready, err := p.uploads.Advance(device, budget)
			if err != nil {
				p.cpu.Finish(err)
				p.Discard()
				return false, err
			}
			if ready || device.Driver() == nil {
				p.phase = scenePreloadReady
				if ready {
					p.cpu.Finish(nil)
				}
				return true, nil
			}
			return false, nil
		}
		if err := p.advanceOne(); err != nil {
			p.Discard()
			return false, err
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
	}
}

func (p *ScenePreloader) advanceOne() error {
	switch p.phase {
	case scenePreloadSpawn:
		if p.index == len(p.plan.Nodes) {
			p.phase, p.index = scenePreloadResolve, 0
			return nil
		}
		if err := p.bundle.spawnPreparedScenePlanNode(p.scene, p.plan, p.entities, p.index, p.components[p.index]); err != nil {
			return err
		}
		p.components[p.index] = ecs.Bundle{}
		entity := p.entities[p.index]
		if err := p.scene.World.Insert(entity, sceneapi.Activity{Active: false}); err != nil {
			return err
		}
		if visibility := p.scene.World.GetMut[sceneapi.Visibility](entity); visibility != nil {
			visibility.Visible = false
		}
		if camera := p.scene.World.GetMut[sceneapi.Camera](entity); camera != nil {
			camera.Active = false
		}
		p.index++
		return nil
	case scenePreloadResolve:
		if p.index == len(p.plan.Nodes) {
			if err := bindSceneAnimationTargets(p.scene.World, p.plan, p.entities); err != nil {
				return err
			}
			p.phase, p.index = scenePreloadPrepare, 0
			return nil
		}
		if err := p.bundle.resolveScenePlanNode(p.scene, p.plan, p.entities, p.index); err != nil {
			return err
		}
		p.index++
		return nil
	case scenePreloadPrepare:
		if p.index == len(p.entities) {
			p.components = nil
			p.phase = scenePreloadUpload
			return nil
		}
		entity := p.entities[p.index]
		if text := p.scene.World.GetMut[sceneapi.Text2D](entity); text != nil {
			sceneapi.PrepareText2D(text)
		}
		QueueEntityUploads(&p.uploads, p.scene.World, entity)
		p.index++
		return nil
	default:
		return nil
	}
}

// Activate replaces current with this already-built scene without spawning it
// a second time. Both scenes must belong to the same world.
func (p *ScenePreloader) Activate(current *SceneInstance) error {
	if !p.Ready() {
		return fmt.Errorf("scene preload is not ready")
	}
	if current == nil || current.World == nil || current.World != p.scene.World {
		return fmt.Errorf("scene preload world does not match the active scene")
	}
	registry := current.scriptRegistry
	for index, entity := range p.entities {
		p.scene.World.Remove[sceneapi.Activity](entity)
		node := p.plan.Nodes[index]
		if visibility := p.scene.World.GetMut[sceneapi.Visibility](entity); visibility != nil {
			visibility.Visible = node.Visible
		}
		if camera := p.scene.World.GetMut[sceneapi.Camera](entity); camera != nil {
			camera.Active = node.Camera.Active
		}
	}
	p.scene.scriptRegistry = registry
	if err := p.scene.BindScripts(); err != nil {
		p.Discard()
		return err
	}
	if err := p.scene.RefreshTransforms(); err != nil {
		p.Discard()
		return err
	}
	current.dispose(false)
	*current = *p.scene
	p.scene = nil
	p.entities = nil
	p.uploads.Reset()
	return nil
}

// Discard removes the hidden scene when the preload is no longer needed.
func (p *ScenePreloader) Discard() {
	if p == nil || p.scene == nil {
		return
	}
	p.cpu.Cancel()
	p.scene.dispose(false)
	p.scene = nil
	p.entities = nil
	p.components = nil
	p.uploads.Reset()
}

func (p *ScenePreloader) device() *render.Device {
	if p == nil {
		return nil
	}
	if p.scene == nil {
		return nil
	}
	return graphics.DeviceFor(p.scene.World)
}
