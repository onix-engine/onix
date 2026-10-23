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
	r.modModels = newResourceCache[string, modelResource](0)
	r.modClips = make(map[*AnimationComponentSpec]*scene.AnimationClip)
	r.modHTMLBases = make(map[string]string)
	r.modRigs = make(map[string]*ModelRig)
	r.modParticles = make(map[string]*TextureSpec, len(mod.Spec.Elements))
	for i := range mod.Spec.Elements {
		if p := mod.Spec.Elements[i].Components.Particle; p != nil {
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
	if c.Layout != nil {
		ecs.Put(&b, *c.Layout)
	}
	width, height := float32(0), float32(0)
	if len(c.Size) == 2 {
		width, height = c.Size[0], c.Size[1]
		ecs.Put(&b, scene.Size2D{Width: width, Height: height})
	}
	if s := c.Sprite; s != nil {
		tint := graphics.White
		color := s.Color
		if s.Texture != nil {
			color = s.Texture.Color
		}
		if color != "" {
			var err error
			tint, err = ParseColor(color, 1)
			if err != nil {
				return ecs.Bundle{}, err
			}
		}
		value := scene.NewSprite2D()
		if s.Texture != nil && (s.Texture.Image != "" || s.Texture.Atlas != "") {
			texture, err := r.texture(s.Texture)
			if err != nil {
				return ecs.Bundle{}, err
			}
			value = scene.Sprite2DFromTexture(texture)
			if slice := s.Texture.Slice; slice != nil {
				if err := slice.Validate(texture.LogicalWidth(), texture.LogicalHeight()); err != nil {
					return ecs.Bundle{}, err
				}
				value.Slice = &scene.NineSlice2D{Left: float32(slice.Left), Right: float32(slice.Right), Top: float32(slice.Top), Bottom: float32(slice.Bottom)}
			}
		} else {
			value.Shape = scene.RoundedRectShape2D(tint, 0)
		}
		if c.Shader != nil && c.Shader.HasSource() {
			if value.Texture == nil {
				value.Texture = graphics.CreateTextureByColor(tint)
				value.Shape = scene.Shape2D{}
			}
		}
		if len(s.Size) == 2 {
			width, height = s.Size[0], s.Size[1]
		}
		if width > 0 {
			value.Width = width
		}
		if height > 0 {
			value.Height = height
		}
		if s.Alpha != nil {
			value.Alpha = *s.Alpha
		}
		ecs.Put(&b, value)
	}
	if c.Trail != nil {
		ecs.Put(&b, c.Trail.component())
	}
	if spec := c.Material; spec != nil {
		material := &ModMaterial2D{Custom: spec.Custom}
		ecs.Put(&b, scene.MeshMaterial2D[ModMaterial2D]{Material: material})
	}
	if err := r.modShader(&b, c.Shader, c.Model != nil); err != nil {
		return ecs.Bundle{}, err
	}
	if t := c.Text; t != nil {
		value := scene.NewText2D(t.Value)
		if t.Font != "" {
			font, err := r.Font(t.Font)
			if err != nil {
				return ecs.Bundle{}, err
			}
			value.Font = font
		}
		if t.FontSize > 0 {
			value.FontSize = t.FontSize
		}
		if err := configureTextLayout(&value, t, "module text"); err != nil {
			return ecs.Bundle{}, err
		}
		value.OutlineWidth, value.MaxWidth = t.OutlineWidth, t.MaxWidth
		if t.Color != "" {
			color, err := ParseColor(t.Color, 1)
			if err != nil {
				return ecs.Bundle{}, err
			}
			value.Color = color
		}
		if t.OutlineColor != "" {
			color, err := ParseColor(t.OutlineColor, 1)
			if err != nil {
				return ecs.Bundle{}, err
			}
			value.OutlineColor = color
		}
		if t.Shadow != nil {
			color, err := ParseColor(t.Shadow.Color, 1)
			if err != nil {
				return ecs.Bundle{}, err
			}
			value.ShadowX, value.ShadowY, value.ShadowColor = t.Shadow.OffsetX, t.Shadow.OffsetY, color
		}
		ecs.Put(&b, value)
	}
	if h := c.HTML; h != nil {
		value, err := r.HTML(h.Resource)
		if err != nil {
			return ecs.Bundle{}, err
		}
		surface := html.Surface{Value: value, Width: width, Height: height, PassThrough: h.PassThrough, Images: &html.Images{Provider: modHTMLImages{store: r, base: r.modHTMLBases[h.Resource]}}}
		if h.Font != "" {
			surface.Font, err = r.Font(h.Font)
			if err != nil {
				return ecs.Bundle{}, err
			}
		}
		if h.BoldFont != "" {
			surface.BoldFont, err = r.Font(h.BoldFont)
			if err != nil {
				return ecs.Bundle{}, err
			}
		}
		ecs.Put(&b, surface)
	}
	if a := c.Animation; a != nil {
		clip := r.modClips[a]
		if clip == nil {
			var err error
			clip, err = r.compileAnimationClipSpec(a.Clip, "module animation")
			if err != nil {
				return ecs.Bundle{}, err
			}
			r.modClips[a] = clip
		}
		animator := scene.NewAnimator(clip, a.Clip.Loop)
		if a.Speed > 0 {
			animator.Speed = a.Speed
		}
		if c.Sprite == nil && len(clip.TextureTracks) > 0 {
			sprite := scene.Sprite2DFromTexture(animator.CurrentTexture())
			if width > 0 {
				sprite.Width = width
			}
			if height > 0 {
				sprite.Height = height
			}
			ecs.Put(&b, sprite)
		}
		ecs.Put(&b, animator)
	}
	if e := c.Emitter; e != nil {
		emitter, err := r.modEmitter(*e)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, emitter)
	}
	if t := c.Tilemap; t != nil {
		value := scene.Tilemap2D{Width: t.Width, Height: t.Height, TileWidth: t.TileWidth, TileHeight: t.TileHeight, Tiles: slices.Clone(t.Tiles), Alpha: 1}
		if t.Tileset != "" {
			texture, err := r.texture(&TextureSpec{Image: t.Tileset})
			if err != nil {
				return ecs.Bundle{}, err
			}
			if texture.Width%t.TileWidth != 0 || texture.Height%t.TileHeight != 0 {
				return ecs.Bundle{}, fmt.Errorf("tilemap texture dimensions must be divisible by tile dimensions")
			}
			value.Texture, value.Columns = texture, texture.Width/t.TileWidth
		}
		ecs.Put(&b, value)
	}
	if s := c.Spine; s != nil {
		if r.spines == nil {
			r.spines = newResourceCache[string, *spine.Resource](0)
		}
		resource, err := r.spines.(*resourceCache[string, *spine.Resource]).load(s.Source.ID, func() (*spine.Resource, error) { return r.loadSpine(s.Source) }, nil)
		if err != nil {
			return ecs.Bundle{}, err
		}
		player := spine.NewPlayer(resource)
		if s.Speed > 0 {
			player.Speed = s.Speed
		}
		if s.Skin != "" {
			if err := player.SetSkin(s.Skin); err != nil {
				return ecs.Bundle{}, err
			}
		}
		if s.Animation != "" {
			if err := player.SetAnimation(s.Animation, s.Loop); err != nil {
				return ecs.Bundle{}, err
			}
		}
		player.Update(0)
		ecs.Put(&b, scene.Spine2D{Player: player})
	}
	var model modelResource
	if m := c.Model; m != nil {
		var err error
		model, err = r.modModels.load(m.ID, func() (modelResource, error) { return r.loadModModel(*m) }, modelResource.dispose)
		if err != nil {
			return ecs.Bundle{}, err
		}
		if model.mesh != nil {
			ecs.Put(&b, render3d.Mesh{Mesh: model.mesh})
			materials := slices.Clone(model.materials)
			ecs.Put(&b, render3d.Material[graphics.StandardMaterial]{Material: &materials[0], Materials: materials})
		}
		if len(model.asset.Nodes) > 0 {
			rig := r.modRigs[m.ID]
			if rig == nil {
				rig, err = newModelRig(model.asset)
				if err != nil {
					return ecs.Bundle{}, err
				}
				r.modRigs[m.ID] = rig
			}
			pose := rig.NewPose()
			ecs.Put(&b, rig)
			ecs.Put(&b, pose)
			ecs.Put(&b, pose.Deformation)
		}
	}
	if spec := c.Camera2D; spec != nil {
		camera := scene.NewCamera()
		camera.Order, camera.Viewport = spec.Order, spec.Viewport
		if spec.Clear != nil {
			camera.Clear = *spec.Clear
		}
		if spec.ClearColor != "" {
			color, err := ParseColor(spec.ClearColor, 1)
			if err != nil {
				return ecs.Bundle{}, err
			}
			camera.ClearColor = color
		}
		projection := scene.OrthographicProjection2D()
		if spec.Scale > 0 {
			projection.Orthographic.Scale = spec.Scale
		}
		ecs.Put(&b, camera)
		ecs.Put(&b, scene.Camera2D{})
		ecs.Put(&b, projection)
	}
	if camera := c.Camera; camera != nil {
		color, err := ParseColor(camera.ClearColor, 1)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, scene.Camera{Viewport: scene.CameraViewport{X: camera.Viewport.X, Y: camera.Viewport.Y, Width: camera.Viewport.Width, Height: camera.Viewport.Height}, Order: camera.Order, Active: true, Clear: true, ClearColor: color})
		ecs.Put(&b, scene.Projection{Kind: scene.ProjectionPerspective, Perspective: scene.PerspectiveProjection{Fov: camera.Fov, Near: camera.Near}})
		settings, err := camera.renderSettings()
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, settings)
		environment, err := r.modEnvironment(camera)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, environment)
	}
	if l := c.AmbientLight; l != nil {
		color, err := ParseColor(l.Color, 1)
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, render3d.AmbientLight{Color: color.Linear(), Brightness: l.Brightness})
	}
	if l := c.DirectionalLight; l != nil {
		color, err := ParseColor(l.Color, 1)
		if err != nil {
			return ecs.Bundle{}, err
		}
		value := render3d.DirectionalLight{Color: color.Linear(), Illuminance: l.Illuminance}
		if l.Shadows != nil {
			value.Shadows = l.Shadows.Settings()
		}
		if err := value.Validate(); err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, value)
	}
	if l := c.PointLight; l != nil {
		value, err := l.Light()
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, value)
	}
	if l := c.SpotLight; l != nil {
		value, err := l.Light()
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, value)
	}
	if l := c.AreaLight; l != nil {
		value, err := l.Light()
		if err != nil {
			return ecs.Bundle{}, err
		}
		ecs.Put(&b, value)
	}
	if p := c.RigidBody2D; p != nil {
		ecs.Put(&b, p.component())
	}
	if p := c.CircleCollider2D; p != nil {
		radius := p.Radius
		if radius <= 0 {
			radius = float64(min(width, height)) * .5
		}
		value := enginephysics.NewCollider2D(enginephysics.Circle{Center: enginephysics.V(p.Offset[0], p.Offset[1]), Radius: radius})
		applyColliderSettings(p.Collider2DSettingsSpec, &value)
		ecs.Put(&b, value)
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
		applyColliderSettings(p.Collider2DSettingsSpec, &value)
		ecs.Put(&b, value)
	}
	if p := c.SegmentCollider2D; p != nil {
		ecs.Put(&b, p.component())
	}
	if p := c.RigidBody3D; p != nil {
		ecs.Put(&b, p.component())
	}
	if p := c.TriangleMeshCollider3D; p != nil {
		if !model.hasCollision || model.collision.Mode != enginephysics.CollisionModeTriangleMesh {
			return ecs.Bundle{}, fmt.Errorf("triangle mesh collider requires this element's cooked model")
		}
		value := enginephysics.NewCollider3D(model.collision.TriangleMesh)
		applyColliderSettings3D(p.Collider3DSettingsSpec, &value)
		ecs.Put(&b, value)
	}
	if p := c.ConvexHullCollider3D; p != nil {
		if !model.hasCollision || model.collision.Mode != enginephysics.CollisionModeSingleConvex {
			return ecs.Bundle{}, fmt.Errorf("convex hull collider requires this element's cooked model")
		}
		value := enginephysics.NewCollider3D(model.collision.ConvexHull)
		applyColliderSettings3D(p.Collider3DSettingsSpec, &value)
		ecs.Put(&b, value)
	}
	if p := c.CompoundCollider3D; p != nil {
		if !model.hasCollision || model.collision.Mode != enginephysics.CollisionModeConvexDecomposition {
			return ecs.Bundle{}, fmt.Errorf("compound collider requires this element's cooked model")
		}
		value := enginephysics.NewCollider3D(model.collision.Compound)
		applyColliderSettings3D(p.Collider3DSettingsSpec, &value)
		ecs.Put(&b, value)
	}
	return b, nil
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
