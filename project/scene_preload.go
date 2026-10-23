package project

import (
	"fmt"
	"time"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
)

type scenePreloadPhase uint8

const (
	scenePreloadSpawn scenePreloadPhase = iota
	scenePreloadResolve
	scenePreloadMeasureText
	scenePreloadReady
)

// ScenePreloader incrementally builds a scene in the active world. Its nodes
// remain hidden and its cameras inactive until Activate is called.
type ScenePreloader struct {
	bundle   *Bundle
	scene    *SceneInstance
	plan     sceneSpawnPlan
	entities []ecs.Entity
	texts    []ecs.Entity
	phase    scenePreloadPhase
	index    int
}

// BeginPreloadScene starts an incremental scene preload. Advance must be
// called until Ready reports true before Activate may be used.
func (b *Bundle) BeginPreloadScene(world *ecs.World, id SceneID) (*ScenePreloader, error) {
	scene, plan, err := b.newSceneInstance(world, id)
	if err != nil {
		return nil, err
	}
	return &ScenePreloader{
		bundle:   b,
		scene:    scene,
		plan:     plan,
		entities: make([]ecs.Entity, len(plan.Nodes)),
		texts:    make([]ecs.Entity, 0, len(plan.Nodes)),
	}, nil
}

// Ready reports whether construction, resource loading, and static text
// measurement have completed.
func (p *ScenePreloader) Ready() bool {
	return p != nil && p.phase == scenePreloadReady && p.scene != nil
}

// Advance builds scene nodes until the supplied time budget is exhausted.
// A non-positive budget still completes one unit of work so callers always
// make progress in deterministic tests.
func (p *ScenePreloader) Advance(budget time.Duration) (bool, error) {
	if p == nil || p.scene == nil {
		return false, fmt.Errorf("scene preloader is nil")
	}
	if p.Ready() {
		return true, nil
	}
	deadline := time.Now().Add(budget)
	for {
		ready, err := p.advanceOne()
		if err != nil {
			p.Discard()
			return false, err
		}
		if ready {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
	}
}

func (p *ScenePreloader) advanceOne() (bool, error) {
	switch p.phase {
	case scenePreloadSpawn:
		if p.index == len(p.plan.Nodes) {
			p.phase, p.index = scenePreloadResolve, 0
			return false, nil
		}
		if err := p.bundle.spawnScenePlanNode(p.scene, p.plan, p.entities, p.index); err != nil {
			return false, err
		}
		entity := p.entities[p.index]
		if err := ecs.Insert(p.scene.World, entity, sceneapi.Activity{Active: false}); err != nil {
			return false, err
		}
		if visibility := ecs.GetMut[sceneapi.Visibility](p.scene.World, entity); visibility != nil {
			visibility.Visible = false
		}
		if camera := ecs.GetMut[sceneapi.Camera](p.scene.World, entity); camera != nil {
			camera.Active = false
		}
		if ecs.Has[sceneapi.Text2D](p.scene.World, entity) {
			p.texts = append(p.texts, entity)
		}
		p.index++
		return false, nil
	case scenePreloadResolve:
		if p.index == len(p.plan.Nodes) {
			if err := bindSceneAnimationTargets(p.scene.World, p.plan, p.entities); err != nil {
				return false, err
			}
			p.phase, p.index = scenePreloadMeasureText, 0
			return false, nil
		}
		if err := p.bundle.resolveScenePlanNode(p.scene, p.plan, p.entities, p.index); err != nil {
			return false, err
		}
		p.index++
		return false, nil
	case scenePreloadMeasureText:
		if p.index == len(p.texts) {
			p.phase = scenePreloadReady
			return true, nil
		}
		if text := ecs.GetMut[sceneapi.Text2D](p.scene.World, p.texts[p.index]); text != nil {
			sceneapi.MeasureText2D(text)
		}
		p.index++
		return false, nil
	default:
		return true, nil
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
		ecs.Remove[sceneapi.Activity](p.scene.World, entity)
		node := p.plan.Nodes[index]
		if visibility := ecs.GetMut[sceneapi.Visibility](p.scene.World, entity); visibility != nil {
			visibility.Visible = node.Visible
		}
		if camera := ecs.GetMut[sceneapi.Camera](p.scene.World, entity); camera != nil {
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
	p.texts = nil
	return nil
}

// Discard removes the hidden scene when the preload is no longer needed.
func (p *ScenePreloader) Discard() {
	if p == nil || p.scene == nil {
		return
	}
	p.scene.dispose(false)
	p.scene = nil
	p.entities = nil
	p.texts = nil
}
