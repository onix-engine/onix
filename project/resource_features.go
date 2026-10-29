package project

import (
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/resource"
	onixtext "github.com/diiyw/onix/text"
	"github.com/diiyw/onix/window"
)

// ResourceFeature provides project resource implementations selected by the
// application. Feature types are composed at the generic loading boundary so
// unused implementations are not reachable from the final binary.
type ResourceFeature interface {
	ConfigureResources(ResourceLoadContext, ResourceEntries) error
}

// ImageFeature installs a statically selected image decoder without importing
// the font, icon, or audio decoder bundles in project/assets.
type ImageFeature[I ImageDecoder] struct{}

func (ImageFeature[I]) ConfigureResources(context ResourceLoadContext, _ ResourceEntries) error {
	var decoder I
	context.SetImageDecoder(decoder)
	return nil
}

// IconFeature installs the decoder used by a project's window.icon setting.
type IconFeature[K window.IconDecoder] struct{}

func (IconFeature[K]) ConfigureResources(context ResourceLoadContext, _ ResourceEntries) error {
	var decoder K
	context.SetWindowIconDecoder(decoder)
	return nil
}

// ResourceFeatureSet composes two statically selected resource features. A nil
// interface type argument is skipped so callers may use ResourceFeature when a
// slot is intentionally empty.
type ResourceFeatureSet[A, B ResourceFeature] struct{}

func (ResourceFeatureSet[A, B]) ConfigureResources(context ResourceLoadContext, entries ResourceEntries) error {
	var first A
	if any(first) != nil {
		if err := first.ConfigureResources(context, entries); err != nil {
			return err
		}
	}
	var second B
	if any(second) != nil {
		return second.ConfigureResources(context, entries)
	}
	return nil
}

// ResourceEntries is the manifest subset handled by an injected feature.
type ResourceEntries struct {
	Audio []FileResource
}

// ResourceLoadContext gives a feature controlled access to the resource store.
type ResourceLoadContext struct {
	resources *Resources
	source    iofs.FS
	baseDir   string
}

// ReadFile reads a path relative to the project directory.
func (context ResourceLoadContext) ReadFile(path string) ([]byte, error) {
	if context.source == nil {
		return nil, fmt.Errorf("project resource source is nil")
	}
	resolved, err := ResolveResourcePath(context.baseDir, path)
	if err != nil {
		return nil, err
	}
	return readProjectFile(context.source, resolved)
}

// OpenBlob opens and pins a path relative to the project directory. Ownership
// of the returned Blob is transferred to the caller.
func (context ResourceLoadContext) OpenBlob(path string) (*resource.Blob, error) {
	if context.source == nil {
		return nil, fmt.Errorf("project resource source is nil")
	}
	resolved, err := ResolveResourcePath(context.baseDir, path)
	if err != nil {
		return nil, err
	}
	return openProjectBlob(context.source, resolved)
}

// ImageDecoder decodes scene images selected by an injected resource feature.
type ImageDecoder interface {
	DecodeImage([]byte, string, graphics.TextureOptions) (*graphics.Texture, error)
}

// FontDecoder decodes a project font selected by an injected resource feature.
type FontDecoder interface {
	DecodeFont([]byte, string) (*onixtext.Font, error)
}

// SetImageDecoder configures the image decoder used by scene sprites and
// atlases. Passing nil disables image resources for this project.
func (context ResourceLoadContext) SetImageDecoder(decoder ImageDecoder) {
	if context.resources != nil {
		context.resources.imageDecoder = decoder
	}
}

// SetFontDecoder configures the font decoder used by project font resources.
func (context ResourceLoadContext) SetFontDecoder(decoder FontDecoder) {
	if context.resources != nil && decoder != nil {
		context.resources.fontDecoder = decoder
	}
}

// SetWindowIconDecoder configures the decoder used when a project declares a
// window icon.
func (context ResourceLoadContext) SetWindowIconDecoder(decoder window.IconDecoder) {
	if context.resources != nil {
		context.resources.iconDecoder = decoder
	}
}

type namedResourceTable[T any] struct {
	cache *resourceCache[string, T]
}

type namedResourceDisposer interface {
	disposeResources() error
	statistics() cacheStatistics
}

// ResourceRegistration defines both halves of a named resource lifecycle.
// Dispose may be nil for values that own no explicit native resources.
type ResourceRegistration[T any] struct {
	Load    func() (T, error)
	Dispose func(T) error
}

// RegisterNamedResource installs one lazily loaded feature resource. The
// concrete resource type remains outside the project core package.
func (context ResourceLoadContext) RegisterNamedResource[T any](kind, id string, registration ResourceRegistration[T]) error {
	if context.resources == nil {
		return fmt.Errorf("project resources are nil")
	}
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if kind == "" || id == "" || registration.Load == nil {
		return fmt.Errorf("named resource requires kind, id, and loader")
	}
	if context.resources.named == nil {
		context.resources.named = make(map[string]any)
	}
	context.resources.namedMu.Lock()
	defer context.resources.namedMu.Unlock()
	table, ok := context.resources.named[kind]
	if !ok {
		values := &namedResourceTable[T]{cache: newResourceCache[string, T](0)}
		context.resources.named[kind] = values
		table = values
	}
	values, ok := table.(*namedResourceTable[T])
	if !ok {
		return fmt.Errorf("named resource kind %q has a different value type", kind)
	}
	if err := values.cache.register(id, registration.Load, registration.Dispose); err != nil {
		if err == iofs.ErrExist {
			return fmt.Errorf("duplicate %s resource %q", kind, id)
		}
		return err
	}
	return nil
}

// LoadNamedResource returns a lazily loaded resource registered by a feature.
func (resources *Resources) LoadNamedResource[T any](kind, id string) (T, error) {
	if resources == nil {
		var zero T
		return zero, fmt.Errorf("resources are nil")
	}
	resources.namedMu.RLock()
	values, ok := resources.named[strings.TrimSpace(kind)].(*namedResourceTable[T])
	resources.namedMu.RUnlock()
	if !ok || values == nil {
		var zero T
		return zero, fmt.Errorf("%s resources are not configured", kind)
	}
	value, exists, err := values.cache.get(strings.TrimSpace(id))
	if err != nil {
		var zero T
		return zero, fmt.Errorf("load %s resource %q: %w", kind, strings.TrimSpace(id), err)
	}
	if !exists {
		var zero T
		return zero, fmt.Errorf("%s resource %q not found", kind, strings.TrimSpace(id))
	}
	return value, nil
}

func (table *namedResourceTable[T]) disposeResources() error {
	if table == nil {
		return nil
	}
	return table.cache.disposeResources()
}

func (table *namedResourceTable[T]) statistics() cacheStatistics {
	if table == nil {
		return cacheStatistics{}
	}
	return table.cache.statistics()
}

type builtinFontDecoder struct{}

func (builtinFontDecoder) DecodeFont(data []byte, _ string) (*onixtext.Font, error) {
	return onixtext.LoadFont(data)
}

func (resources *Resources) decodeFont(data []byte, path string) (*onixtext.Font, error) {
	if resources == nil || resources.fontDecoder == nil {
		return nil, fmt.Errorf("font decoder is not configured")
	}
	return resources.fontDecoder.DecodeFont(data, path)
}

func configureResourceFeature(resources *Resources, source iofs.FS, baseDir string, feature ResourceFeature, entries ResourceEntries) error {
	if resources == nil || feature == nil {
		return nil
	}
	return feature.ConfigureResources(ResourceLoadContext{
		resources: resources,
		source:    source,
		baseDir:   baseDir,
	}, entries)
}
