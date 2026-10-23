package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diiyw/onix/resource"
)

// LoadDirectoryConfigured loads a project using an explicit EngineConfig while
// retaining the caller's statically selected resource feature type.
func LoadDirectoryConfigured[F ResourceFeature](dir string, config EngineConfig) (*Bundle, error) {
	if source, root, ok := resource.Default(); ok {
		return LoadConfiguredWith[F](source, root, config)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	root, err := moduleRoot(absDir)
	if err != nil {
		return nil, err
	}
	projectRoot, err := filepath.Rel(root, absDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	return LoadConfiguredWith[F](os.DirFS(root), filepath.ToSlash(projectRoot), config)
}

// LoadDirectoryWithProfile loads a project through a statically selected
// scene/resource profile. The profile type is a link boundary: only the
// resource and scene builders implemented by P are reachable from the final
// executable.
func LoadDirectoryWithProfile[P LoadProfile, F ResourceFeature](dir string) (*Bundle, error) {
	if source, root, ok := resource.Default(); ok {
		var profile P
		var feature F
		return loadProfileBundle(source, root, profile, feature)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	root, err := moduleRoot(absDir)
	if err != nil {
		return nil, err
	}
	projectRoot, err := filepath.Rel(root, absDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	var profile P
	var feature F
	return loadProfileBundle(os.DirFS(root), filepath.ToSlash(projectRoot), profile, feature)
}

// LoadDirectoryFromManifest loads a project using the explicit EngineConfig
// stored in resources/project.json. The resource feature type remains a
// compile-time decoder/link boundary while runtime features stay project data.
func LoadDirectoryFromManifest[F ResourceFeature](dir string) (*Bundle, error) {
	if source, root, ok := resource.Default(); ok {
		manifest, _, err := loadGameManifest(source, root)
		if err != nil {
			return nil, err
		}
		return LoadConfiguredWith[F](source, root, manifest.Engine)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	root, err := moduleRoot(absDir)
	if err != nil {
		return nil, err
	}
	projectRoot, err := filepath.Rel(root, absDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	projectRoot = filepath.ToSlash(projectRoot)
	source := os.DirFS(root)
	manifest, _, err := loadGameManifest(source, projectRoot)
	if err != nil {
		return nil, err
	}
	return LoadConfiguredWith[F](source, projectRoot, manifest.Engine)
}

// LoadDefaultConfigured loads a project from the resource source mounted by
// generated build code.
func LoadDefaultConfigured[F ResourceFeature](config EngineConfig) (*Bundle, error) {
	source, root, ok := resource.Default()
	if !ok {
		return nil, fmt.Errorf("default project resource source is not mounted")
	}
	return LoadConfiguredWith[F](source, root, config)
}

// MustLoadDefault invokes a generated project loader against the mounted
// resource package and panics when loading fails.
func MustLoadDefault(loader func(string) (*Bundle, error)) *Bundle {
	if loader == nil {
		panic("project loader is nil")
	}
	bundle, err := loader("")
	if err != nil {
		panic(err)
	}
	return bundle
}

// MustDefault loads the mounted project resource package and panics on error.
func MustDefault() *Bundle {
	bundle, err := LoadDefault()
	if err != nil {
		panic(err)
	}
	return bundle
}

func moduleRoot(start string) (string, error) {
	start = filepath.Clean(start)
	for {
		info, err := os.Stat(filepath.Join(start, "go.mod"))
		if err == nil {
			if !info.IsDir() {
				return start, nil
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect module root %s: %w", start, err)
		}
		parent := filepath.Dir(start)
		if parent == start {
			return "", fmt.Errorf("go.mod not found from project directory %s", start)
		}
		start = parent
	}
}
