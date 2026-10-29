package project

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/resource"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/diiyw/onix/spine"
	"github.com/diiyw/onix/ui/html"
)

// OpenModResources installs only this definition's private resource namespace.
// The scoped filesystem also confines references in HTML, atlases and models.
// Instances share this store, while mutable ECS components remain per instance.
func OpenModResources(source fs.FS, mod *ModPlan, feature ResourceFeature) (_ *Resources, resultErr error) {
	r := newResources()
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, r.Dispose())
		}
	}()
	if source != nil {
		var err error
		r.source, err = resource.Sub(source, string(mod.ID))
		if err != nil {
			return nil, err
		}
	}
	r.baseDir = "."
	for i := range mod.Spec.Elements {
		if p := mod.Spec.Elements[i].Components.Particle; p != nil {
			if r.modParticles == nil {
				r.modParticles = make(map[string]*TextureSpec)
			}
			r.modParticles[mod.Spec.Elements[i].ID] = p
		}
	}
	entries := ResourceEntries{}
	for _, entry := range mod.Resources {
		p := mod.Spec.Resources[entry.Name].Path
		switch entry.Kind {
		case "font":
			if err := r.fonts.register(entry.Name, func() (fontResource, error) { return r.loadFont(p, p) }, fontResource.dispose); err != nil {
				return nil, err
			}
		case "audio":
			entries.Audio = append(entries.Audio, FileResource{ID: entry.Name, Path: p})
		case "html":
			if r.modHTMLBases == nil {
				r.modHTMLBases = make(map[string]string)
			}
			r.modHTMLBases[entry.Name] = path.Dir(p)
			if r.html == nil {
				r.html = newResourceCache[string, string](0)
			}
			if err := r.html.(*resourceCache[string, string]).register(entry.Name, func() (string, error) { data, err := readProjectFile(r.source, p); return string(data), err }, nil); err != nil {
				return nil, err
			}
		case "video":
			if r.videos == nil {
				r.videos = newResourceCache[string, *Video](0)
			}
			if err := r.videos.(*resourceCache[string, *Video]).register(entry.Name, func() (*Video, error) {
				blob, err := openProjectBlob(r.source, p)
				if err != nil {
					return nil, err
				}
				return newVideo(blob), nil
			}, (*Video).Close); err != nil {
				return nil, err
			}
		}
	}
	if feature == nil && len(entries.Audio) != 0 {
		return nil, fmt.Errorf("%s: audio decoder is not configured", mod.ID)
	}
	if err := configureResourceFeature(r, r.source, ".", feature, entries); err != nil {
		return nil, err
	}
	return r, nil
}

// ModContent builds private component values; it has no scene/entity registry.
func (r *Resources) ModContent(c ModComponents) (ecs.Bundle, error) {
	b := ecs.NewBundle(16)
	if err := r.AppendModContent(&b, c); err != nil {
		return ecs.Bundle{}, err
	}
	return b, nil
}

func (r *Resources) AppendModContent(b *ecs.Bundle, c ModComponents) error {
	if err := r.AppendModContentLayout(b, c); err != nil {
		return err
	}
	if err := r.AppendModContent2D(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentTrail(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentHTML(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentAnimation(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentParticles(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentTilemaps(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentSpine(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentSpatial(b, c); err != nil {
		return err
	}
	if err := r.AppendModContentPhysics2D(b, c); err != nil {
		return err
	}
	return nil
}

func (r *Resources) AppendModContentLayout(b *ecs.Bundle, c ModComponents) error {
	if c.Layout != nil {
		b.Put(*c.Layout)
	}
	return nil
}

func (r *Resources) AppendModContentTrail(b *ecs.Bundle, c ModComponents) error {
	if c.Trail != nil {
		b.Put(c.Trail.component())
	}

	return nil
}

func (r *Resources) AppendModContentHTML(b *ecs.Bundle, c ModComponents) error {
	width, height := modContentSize(c)
	if h := c.HTML; h != nil {
		value, err := r.HTML(h.Resource)
		if err != nil {
			return err
		}
		surface := html.Surface{Value: value, Width: width, Height: height, PassThrough: h.PassThrough, Images: &html.Images{Provider: modHTMLImages{store: r, base: r.modHTMLBases[h.Resource]}}}
		if h.Font != "" {
			surface.Font, err = r.Font(h.Font)
			if err != nil {
				return err
			}
		}
		if h.BoldFont != "" {
			surface.BoldFont, err = r.Font(h.BoldFont)
			if err != nil {
				return err
			}
		}
		b.Put(surface)
	}

	return nil
}

func (r *Resources) AppendModContentParticles(b *ecs.Bundle, c ModComponents) error {
	if e := c.Emitter; e != nil {
		emitter, err := r.modEmitter(*e)
		if err != nil {
			return err
		}
		b.Put(emitter)
	}

	return nil
}

func (r *Resources) AppendModContentTilemaps(b *ecs.Bundle, c ModComponents) error {
	if t := c.Tilemap; t != nil {
		value := scene.Tilemap2D{Width: t.Width, Height: t.Height, TileWidth: t.TileWidth, TileHeight: t.TileHeight, Tiles: slices.Clone(t.Tiles), Alpha: 1}
		if t.Tileset != "" {
			texture, err := r.texture(&TextureSpec{Image: t.Tileset})
			if err != nil {
				return err
			}
			if texture.Width%t.TileWidth != 0 || texture.Height%t.TileHeight != 0 {
				return fmt.Errorf("tilemap texture dimensions must be divisible by tile dimensions")
			}
			value.Texture, value.Columns = texture, texture.Width/t.TileWidth
		}
		b.Put(value)
	}

	return nil
}

func (r *Resources) AppendModContentSpine(b *ecs.Bundle, c ModComponents) error {
	if s := c.Spine; s != nil {
		if r.spines == nil {
			r.spines = newResourceCache[string, *spine.Resource](0)
		}
		resource, err := r.spines.(*resourceCache[string, *spine.Resource]).load(s.Source.ID, func() (*spine.Resource, error) { return r.loadSpine(s.Source) }, nil)
		if err != nil {
			return err
		}
		player := spine.NewPlayer(resource)
		if s.Speed > 0 {
			player.Speed = s.Speed
		}
		if s.Skin != "" {
			if err := player.SetSkin(s.Skin); err != nil {
				return err
			}
		}
		if s.Animation != "" {
			if err := player.SetAnimation(s.Animation, s.Loop); err != nil {
				return err
			}
		}
		player.Update(0)
		b.Put(scene.Spine2D{Player: player})
	}

	return nil
}

func (r *Resources) AppendModContentSpatial(b *ecs.Bundle, c ModComponents) error {
	var model modelResource
	if m := c.Model; m != nil {
		if r.modModels == nil {
			r.modModels = newResourceCache[string, modelResource](0)
		}
		var err error
		model, err = r.modModels.load(m.ID, func() (modelResource, error) { return r.loadModModel(*m) }, modelResource.dispose)
		if err != nil {
			return err
		}
		if model.mesh != nil {
			b.Put(render3d.Mesh{Mesh: model.mesh})
			materials := slices.Clone(model.materials)
			b.Put(render3d.Material[graphics.StandardMaterial]{Material: &materials[0], Materials: materials})
		}
		if len(model.asset.Nodes) > 0 {
			rig := r.modRigs[m.ID]
			if rig == nil {
				rig, err = newModelRig(model.asset)
				if err != nil {
					return err
				}
				if r.modRigs == nil {
					r.modRigs = make(map[string]*ModelRig)
				}
				r.modRigs[m.ID] = rig
			}
			pose := rig.NewPose()
			b.Put(rig)
			b.Put(pose)
			b.Put(pose.Deformation)
		}
	}
	if camera := c.Camera; camera != nil {
		color, err := ParseColor(camera.ClearColor, 1)
		if err != nil {
			return err
		}
		b.Put(scene.Camera{Viewport: scene.CameraViewport{X: camera.Viewport.X, Y: camera.Viewport.Y, Width: camera.Viewport.Width, Height: camera.Viewport.Height}, Order: camera.Order, Active: true, Clear: true, ClearColor: color})
		b.Put(scene.Projection{Kind: scene.ProjectionPerspective, Perspective: scene.PerspectiveProjection{Fov: camera.Fov, Near: camera.Near}})
		settings, err := camera.renderSettings()
		if err != nil {
			return err
		}
		b.Put(settings)
		environment, err := r.modEnvironment(camera)
		if err != nil {
			return err
		}
		b.Put(environment)
	}
	if l := c.AmbientLight; l != nil {
		color, err := ParseColor(l.Color, 1)
		if err != nil {
			return err
		}
		b.Put(render3d.AmbientLight{Color: color.Linear(), Brightness: l.Brightness})
	}
	if l := c.DirectionalLight; l != nil {
		color, err := ParseColor(l.Color, 1)
		if err != nil {
			return err
		}
		value := render3d.DirectionalLight{Color: color.Linear(), Illuminance: l.Illuminance}
		if l.Shadows != nil {
			value.Shadows = l.Shadows.Settings()
		}
		if err := value.Validate(); err != nil {
			return err
		}
		b.Put(value)
	}
	if l := c.PointLight; l != nil {
		value, err := l.Light()
		if err != nil {
			return err
		}
		b.Put(value)
	}
	if l := c.SpotLight; l != nil {
		value, err := l.Light()
		if err != nil {
			return err
		}
		b.Put(value)
	}
	if l := c.AreaLight; l != nil {
		value, err := l.Light()
		if err != nil {
			return err
		}
		b.Put(value)
	}
	if p := c.RigidBody3D; p != nil {
		b.Put(p.component())
	}
	if p := c.TriangleMeshCollider3D; p != nil {
		if !model.hasCollision || model.collision.Mode != enginephysics.CollisionModeTriangleMesh {
			return fmt.Errorf("triangle mesh collider requires this element's cooked model")
		}
		value := enginephysics.NewCollider3D(model.collision.TriangleMesh)
		p.Collider3DSettingsSpec.applyColliderSettings3D(&value)
		b.Put(value)
	}
	if p := c.ConvexHullCollider3D; p != nil {
		if !model.hasCollision || model.collision.Mode != enginephysics.CollisionModeSingleConvex {
			return fmt.Errorf("convex hull collider requires this element's cooked model")
		}
		value := enginephysics.NewCollider3D(model.collision.ConvexHull)
		p.Collider3DSettingsSpec.applyColliderSettings3D(&value)
		b.Put(value)
	}
	if p := c.CompoundCollider3D; p != nil {
		if !model.hasCollision || model.collision.Mode != enginephysics.CollisionModeConvexDecomposition {
			return fmt.Errorf("compound collider requires this element's cooked model")
		}
		value := enginephysics.NewCollider3D(model.collision.Compound)
		p.Collider3DSettingsSpec.applyColliderSettings3D(&value)
		b.Put(value)
	}

	return nil
}

func (r *Resources) AppendModContentPhysics2D(b *ecs.Bundle, c ModComponents) error {
	width, height := modContentSize(c)
	if p := c.RigidBody2D; p != nil {
		b.Put(p.component())
	}
	if p := c.CircleCollider2D; p != nil {
		radius := p.Radius
		if radius <= 0 {
			radius = float64(min(width, height)) * .5
		}
		value := enginephysics.NewCollider2D(enginephysics.Circle{Center: enginephysics.V(p.Offset[0], p.Offset[1]), Radius: radius})
		p.Collider2DSettingsSpec.applyColliderSettings(&value)
		b.Put(value)
	}
	if p := c.BoxCollider2D; p != nil {
		w, h := p.Width, p.Height
		if w <= 0 {
			w = float64(width)
		}
		if h <= 0 {
			h = float64(height)
		}
		shape := enginephysics.NewBox(w, h)
		shape.Center, shape.Rotation = enginephysics.V(p.Offset[0], p.Offset[1]), p.Rotation
		value := enginephysics.NewCollider2D(shape)
		p.Collider2DSettingsSpec.applyColliderSettings(&value)
		b.Put(value)
	}
	if p := c.SegmentCollider2D; p != nil {
		b.Put(p.component())
	}

	return nil
}

func (r *Resources) modEmitter(e EmitterSpec) (scene.ParticleEmitter2D, error) {
	v := scene.NewParticleEmitter2D()
	v.Width, v.Height, v.Count, v.Rate, v.Lifetime = e.Width, e.Height, e.Count, e.Rate, e.Lifetime
	v.SpeedMin, v.SpeedMax, v.SizeMin, v.SizeMax = e.SpeedMin, e.SpeedMax, e.SizeMin, e.SizeMax
	v.Direction, v.Spread, v.GravityX, v.GravityY = e.Direction, e.Spread, e.GravityX, e.GravityY
	v.AlphaStart, v.AlphaEnd, v.Loop = e.AlphaStart, e.AlphaEnd, e.Loop
	v.Appearances = make([]scene.ParticleAppearance2D, len(e.Particles))
	for i, name := range e.Particles {
		appearance := r.modParticles[name]
		if appearance == nil {
			return v, fmt.Errorf("emitter references unknown particle element %q", name)
		}
		v.Appearances[i].Color = graphics.White
		if appearance.Color != "" {
			color, err := ParseColor(appearance.Color, 1)
			if err != nil {
				return v, err
			}
			v.Appearances[i].Color = color
		} else {
			texture, err := r.texture(appearance)
			if err != nil {
				return v, err
			}
			v.Appearances[i].Texture = texture
		}
	}
	v.Normalize()
	v.Reserve(v.Count)
	return v, nil
}
