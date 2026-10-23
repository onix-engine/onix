package project

import (
	"fmt"
	iofs "io/fs"
	pathpkg "path"
	"strings"
)

type resourcePathScope interface {
	allowParent() bool
}

type sharedResourcePath struct{}

func (sharedResourcePath) allowParent() bool { return true }

type localResourcePath struct{}

func (localResourcePath) allowParent() bool { return false }

type resourceManifestFile interface {
	suffix() string
	description() string
}

type sceneResourceFile struct{}

func (sceneResourceFile) suffix() string      { return ".scene.json" }
func (sceneResourceFile) description() string { return "scene file" }

type prefabResourceFile struct{}

func (prefabResourceFile) suffix() string      { return ".prefab.json" }
func (prefabResourceFile) description() string { return "prefab file" }

// NormalizeResourcePath validates and canonicalizes a project-relative file
// reference. Parent segments are retained so projects mounted below an fs.FS
// root can share repository resources; ResolveResourcePath enforces that root.
func NormalizeResourcePath(value string) (string, error) {
	return normalizeResourcePath[sharedResourcePath](value)
}

// NormalizeLocalResourcePath validates a file reference that cannot contain
// parent segments.
func NormalizeLocalResourcePath(value string) (string, error) {
	return normalizeResourcePath[localResourcePath](value)
}

// ResolveResourcePath resolves a project-relative file reference without
// allowing it to escape the mounted resource source.
func ResolveResourcePath(baseDir, value string) (string, error) {
	normalized, err := NormalizeResourcePath(value)
	if err != nil {
		return "", err
	}
	resolved := pathpkg.Clean(pathpkg.Join(baseDir, normalized))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") || !iofs.ValidPath(resolved) {
		return "", fmt.Errorf("resource path %q escapes the resource source", value)
	}
	return resolved, nil
}

// ResolveLocalResourcePath resolves a file that must remain below baseDir.
func ResolveLocalResourcePath(baseDir, value string) (string, error) {
	normalized, err := NormalizeLocalResourcePath(value)
	if err != nil {
		return "", err
	}
	resolved := pathpkg.Clean(pathpkg.Join(baseDir, normalized))
	if resolved == "." || !iofs.ValidPath(resolved) {
		return "", fmt.Errorf("resource path %q is invalid", value)
	}
	return resolved, nil
}

func normalizeResourceFilePath[T resourceManifestFile](value string) (string, error) {
	normalized, err := normalizeResourcePath[localResourcePath](value)
	var file T
	if err != nil || !strings.HasSuffix(strings.ToLower(normalized), file.suffix()) {
		return "", fmt.Errorf("%s must be a relative *%s path", file.description(), file.suffix())
	}
	return normalized, nil
}

func normalizeResourcePath[T resourcePathScope](value string) (string, error) {
	original := value
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("resource path is required")
	}
	if strings.ContainsRune(value, '\\') {
		return "", fmt.Errorf("resource path %q must use forward slashes", original)
	}
	if pathpkg.IsAbs(value) || windowsDrivePath(value) {
		return "", fmt.Errorf("resource path %q must be relative", original)
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("resource path %q must reference a file", original)
	}

	normalized := pathpkg.Clean(value)
	if normalized == "." || normalized == ".." {
		return "", fmt.Errorf("resource path %q must reference a file", original)
	}
	var scope T
	if !scope.allowParent() && strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("resource path %q must stay within its directory", original)
	}
	validatable := normalized
	for strings.HasPrefix(validatable, "../") {
		validatable = strings.TrimPrefix(validatable, "../")
	}
	if validatable == "" || !iofs.ValidPath(validatable) {
		return "", fmt.Errorf("resource path %q is invalid", original)
	}
	return normalized, nil
}

func windowsDrivePath(value string) bool {
	return len(value) >= 2 && value[1] == ':' && (value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z')
}
