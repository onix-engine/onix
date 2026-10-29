package module

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/diiyw/onix/ui"
	"github.com/diiyw/onix/ui/html"
)

// ContentProfile selects component construction and its entire lifecycle at
// compile time. It must cover both authored content and content created by code.
type ContentProfile interface {
	contentSupport() contentSupport
}

type contentSupport struct {
	features     project.ModContentFeatures
	append       func(*project.Resources, *ecs.Bundle, project.ModComponents) error
	syncHTML     func(*ecs.World) error
	layout       func(*ecs.World) error
	deform       func(*ecs.World) error
	release      func(*ecs.World, ecs.Entity)
	controlID    func(*ecs.World, ecs.Entity) string
	queueUploads func(*graphics.UploadQueue, *ecs.World, ecs.Entity)
}

// Render2D supports sprites, text, cameras, materials and pointer input.
type Render2D struct{}

// FullContent supports every authored component, including HTML and 3D.
type FullContent struct{}

func (Render2D) contentSupport() contentSupport {
	return contentSupport{append: (*project.Resources).AppendModContent2D, queueUploads: project.Queue2DEntityUploads}
}

func (FullContent) contentSupport() contentSupport {
	return contentSupport{
		features: project.ModContentAll, append: (*project.Resources).AppendModContent,
		syncHTML: ui.SyncHTML, layout: ui.UpdateLayout,
		deform:  func(world *ecs.World) error { return render3d.UpdateDeformationsSystem("").Run(world) },
		release: html.Release,
		controlID: func(world *ecs.World, entity ecs.Entity) string {
			control, _ := world.Get[html.Control](entity)
			return control.ID
		},
		queueUploads: project.QueueEntityUploads,
	}
}

func (r *Runtime) releaseContent(entity ecs.Entity) {
	if r.content.release != nil {
		r.content.release(r.world, entity)
	}
}

// WithContent adds one statically selected capability. The runtime and node
// traversal remain shared; writers append into the same destination bundle.
type WithContent[P ContentProfile, F ContentFeature] struct{}
type ContentFeature interface{ contentFeature() contentSupport }

func (WithContent[P, F]) contentSupport() contentSupport {
	var p P
	var f F
	base, extra := p.contentSupport(), f.contentFeature()
	if base.features&extra.features == extra.features {
		return base
	}
	before, after := base.append, extra.append
	base.append = func(r *project.Resources, b *ecs.Bundle, c project.ModComponents) error {
		if err := before(r, b, c); err != nil {
			return err
		}
		return after(r, b, c)
	}
	base.features |= extra.features
	if extra.syncHTML != nil {
		base.syncHTML = extra.syncHTML
	}
	if extra.layout != nil {
		base.layout = extra.layout
	}
	if extra.deform != nil {
		base.deform = extra.deform
	}
	if extra.release != nil {
		base.release = extra.release
	}
	if extra.controlID != nil {
		base.controlID = extra.controlID
	}
	if extra.queueUploads != nil {
		first, second := base.queueUploads, extra.queueUploads
		base.queueUploads = func(q *graphics.UploadQueue, w *ecs.World, e ecs.Entity) { first(q, w, e); second(q, w, e) }
	}
	return base
}

func (s contentSupport) appendTo(r *project.Resources, b *ecs.Bundle, c project.ModComponents) error {
	if c.ContentFeatures()&^s.features != 0 {
		return fmt.Errorf("module component requires an additional content profile capability")
	}
	return s.append(r, b, c)
}
func (s contentSupport) build(r *project.Resources, c project.ModComponents) (ecs.Bundle, error) {
	b := ecs.NewBundle(16)
	if err := s.appendTo(r, &b, c); err != nil {
		return ecs.Bundle{}, err
	}
	return b, nil
}

type AnimationsContent struct{}

func (AnimationsContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentAnimations, append: (*project.Resources).AppendModContentAnimation}
}

type LayoutContent struct{}

func (LayoutContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentLayout, append: (*project.Resources).AppendModContentLayout, layout: ui.UpdateLayout}
}

type TrailsContent struct{}

func (TrailsContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentTrail, append: (*project.Resources).AppendModContentTrail}
}

type HTMLContent struct{}

func (HTMLContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentHTML, append: (*project.Resources).AppendModContentHTML, syncHTML: ui.SyncHTML, layout: ui.UpdateLayout, release: html.Release, controlID: func(w *ecs.World, e ecs.Entity) string { c, _ := w.Get[html.Control](e); return c.ID }}
}

type ParticlesContent struct{}

func (ParticlesContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentParticles, append: (*project.Resources).AppendModContentParticles, queueUploads: project.QueueParticlesEntityUploads}
}

type TilemapsContent struct{}

func (TilemapsContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentTilemaps, append: (*project.Resources).AppendModContentTilemaps, queueUploads: project.QueueTilemapsEntityUploads}
}

type SpineContent struct{}

func (SpineContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentSpine, append: (*project.Resources).AppendModContentSpine, queueUploads: project.QueueSpineEntityUploads}
}

type SpatialContent struct{}

func (SpatialContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentSpatial, append: (*project.Resources).AppendModContentSpatial, deform: func(w *ecs.World) error { return render3d.UpdateDeformationsSystem("").Run(w) }, queueUploads: project.QueueSpatialEntityUploads}
}

type Physics2DContent struct{}

func (Physics2DContent) contentFeature() contentSupport {
	return contentSupport{features: project.ModContentPhysics2D, append: (*project.Resources).AppendModContentPhysics2D}
}
