package project

import (
	"errors"
	"fmt"
	"strings"

	"github.com/diiyw/onix/graphics"
	graphicshdr "github.com/diiyw/onix/graphics/codec/hdr"
	"github.com/diiyw/onix/graphics/codec/ultrahdr"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene/render3d"
)

type environmentResource struct{ atlas, panorama *graphics.Texture }

func (e environmentResource) dispose() error {
	return errors.Join(e.atlas.Dispose(), e.panorama.Dispose())
}

// NormalizeEnvironmentPath accepts a baked PNG atlas, RGBE HDR, or Ultra HDR JPEG panorama.
func NormalizeEnvironmentPath(value string) (string, error) {
	path, err := NormalizeResourcePath(value)
	lower := strings.ToLower(path)
	if err != nil || !(strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".hdr") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg")) {
		return "", fmt.Errorf("environment must be a PNG atlas, HDR or Ultra HDR JPEG panorama")
	}
	return path, nil
}

func validateEnvironmentSpec(e *EnvironmentSpec) error {
	if e == nil {
		return nil
	}
	for _, v := range [...]float32{e.Intensity, e.Exposure} {
		if !finiteFloat32(v) || v < 0 {
			return fmt.Errorf("environment intensity and exposure must be finite and non-negative")
		}
	}
	if !finiteFloat32(e.Rotation) {
		return fmt.Errorf("environment rotation must be finite")
	}
	if e.Path == "" {
		if e.Intensity > 0 || e.Background {
			return fmt.Errorf("environment atlas path is required")
		}
		return nil
	}
	_, err := NormalizeEnvironmentPath(e.Path)
	return err
}

func (b *Bundle) cameraEnvironment(spec *Camera3DSpec) (render3d.Environment, error) {
	return b.resources.modEnvironment(spec)
}

func (r *Resources) modEnvironment(spec *Camera3DSpec) (render3d.Environment, error) {
	if spec == nil || spec.Environment == nil {
		return render3d.Environment{}, nil
	}
	e := spec.Environment
	if err := validateEnvironmentSpec(e); err != nil {
		return render3d.Environment{}, err
	}
	if e.Path == "" {
		return render3d.Environment{Exposure: e.Exposure}, nil
	}
	path, err := ResolveResourcePath(r.baseDir, e.Path)
	if err != nil {
		return render3d.Environment{}, err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".png") {
		resource, err := r.panoramaEnvironmentFromPath(path)
		if err != nil {
			return render3d.Environment{}, fmt.Errorf("environment panorama: %w", err)
		}
		return render3d.Environment{Atlas: resource.atlas, Panorama: resource.panorama, Encoding: render.EnvironmentRGBE, Intensity: e.Intensity, Rotation: e.Rotation, Exposure: e.Exposure, Background: e.Background}, nil
	}
	texture, _, err := r.textureFromPath(path, graphics.TextureOptions{Usage: graphics.ResourceUsageDefault})
	if err != nil {
		return render3d.Environment{}, fmt.Errorf("environment atlas: %w", err)
	}
	if texture.Width < 16 || texture.Width%2 != 0 || texture.Height != texture.Width*4 {
		return render3d.Environment{}, fmt.Errorf("environment atlas must contain eight 2:1 tiles from BakeEnvironment")
	}
	return render3d.Environment{Atlas: texture, Intensity: e.Intensity, Rotation: e.Rotation, Exposure: e.Exposure, Background: e.Background}, nil
}

func (r *Resources) panoramaEnvironmentFromPath(path string) (environmentResource, error) {
	blob, err := openProjectBlob(r.source, path)
	if err != nil {
		return environmentResource{}, err
	}
	defer blob.Close()
	return r.environments.load(blob.ContentID(), func() (environmentResource, error) {
		data, err := blob.Bytes()
		if err != nil {
			return environmentResource{}, err
		}
		bake := graphicshdr.ImportEnvironment
		if !strings.HasSuffix(strings.ToLower(path), ".hdr") {
			bake = ultrahdr.ImportEnvironment
		}
		images, err := bake(data, 256, 256)
		if err != nil {
			return environmentResource{}, err
		}
		options := graphics.TextureOptions{Usage: graphics.ResourceUsageDefault}
		resource := environmentResource{
			atlas:    graphics.TextureFromOwnedNRGBA(images.Atlas, options),
			panorama: graphics.TextureFromOwnedNRGBA(images.Panorama, options),
		}
		if err := errors.Join(resource.atlas.UploadError(), resource.panorama.UploadError()); err != nil {
			_ = resource.dispose()
			return environmentResource{}, err
		}
		return resource, nil
	}, environmentResource.dispose)
}
