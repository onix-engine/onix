package editor

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/diiyw/onix/resource"
)

const packageConfigFile = "packages.json"

type resourcePackageConfig struct {
	Keep     []string              `json:"keep,omitempty"`
	Packages []resourcePackageRule `json:"packages"`
}

type resourcePackageRule struct {
	Name         string   `json:"name"`
	Include      []string `json:"include"`
	Exclude      []string `json:"exclude,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Required     bool     `json:"required,omitempty"`
}

func validateResourcePackageConfig(config resourcePackageConfig) error {
	for _, pattern := range config.Keep {
		if err := validateResourcePackagePattern(pattern); err != nil {
			return fmt.Errorf("resource package keep: %w", err)
		}
	}
	rules := make(map[string]resourcePackageRule, len(config.Packages))
	for _, rule := range config.Packages {
		if !resource.ValidPackageName(rule.Name) || rule.Name == resource.BasePackage {
			return fmt.Errorf("resource package name %q is invalid or reserved", rule.Name)
		}
		if _, exists := rules[rule.Name]; exists {
			return fmt.Errorf("resource package %q is declared more than once", rule.Name)
		}
		if len(rule.Include) == 0 {
			return fmt.Errorf("resource package %q requires at least one include pattern", rule.Name)
		}
		for _, pattern := range rule.Include {
			if err := validateResourcePackagePattern(pattern); err != nil {
				return fmt.Errorf("resource package %q include: %w", rule.Name, err)
			}
		}
		for _, pattern := range rule.Exclude {
			if err := validateResourcePackagePattern(pattern); err != nil {
				return fmt.Errorf("resource package %q exclude: %w", rule.Name, err)
			}
		}
		for _, dependency := range rule.Dependencies {
			if dependency == resource.BasePackage || dependency == rule.Name || !resource.ValidPackageName(dependency) {
				return fmt.Errorf("resource package %q has invalid dependency %q", rule.Name, dependency)
			}
		}
		rules[rule.Name] = rule
	}
	for name, rule := range rules {
		for _, dependency := range rule.Dependencies {
			if _, ok := rules[dependency]; !ok {
				return fmt.Errorf("resource package %q dependency %q is not declared", name, dependency)
			}
		}
	}
	state := make(map[string]uint8, len(rules))
	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("resource package dependency cycle includes %q", name)
		case 2:
			return nil
		}
		state[name] = 1
		for _, dependency := range rules[name].Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for name := range rules {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func validateResourcePackagePattern(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.ContainsRune(pattern, '\\') || filepath.IsAbs(filepath.FromSlash(pattern)) {
		return fmt.Errorf("invalid pattern %q", pattern)
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
	}
	return nil
}

type compiledPackageRule struct {
	resourcePackageRule
	include []compiledPackagePattern
	exclude []compiledPackagePattern
}

type compiledPackagePattern []string

type resourceInput struct {
	// Environment files carry HDR metadata or encoded radiance.
	preserveTexture bool
	source          string
	data            []byte
	memory          bool
}

func memoryResourceInput(data []byte) resourceInput {
	return resourceInput{data: data, memory: true}
}

func (input resourceInput) bytes() ([]byte, error) {
	if input.memory {
		return input.data, nil
	}
	return os.ReadFile(input.source)
}

func (input resourceInput) packInput(name string) resource.PackInput {
	if input.memory {
		return resource.BytesInput(name, input.data)
	}
	return resource.FileInput(name, input.source)
}

type writtenResourcePackage struct {
	size   int64
	digest [sha256.Size]byte
	assets []resource.AssetInfo
	stats  resource.PackStats
}

func writeResourcePackage(output string, files map[string]resourceInput) (writtenResourcePackage, error) {
	return writeResourcePackageWithProgress(output, files, nil)
}

func writeResourcePackageWithProgress(output string, files map[string]resourceInput, progress func(resource.PackProgress)) (writtenResourcePackage, error) {
	temporary := output + ".tmp"
	file, err := os.Create(temporary)
	if err != nil {
		return writtenResourcePackage{}, err
	}
	hash := sha256.New()
	inputs := make([]resource.PackInput, 0, len(files))
	for name, input := range files {
		inputs = append(inputs, input.packInput(name))
	}
	result, packErr := resource.PackWithProgress(io.MultiWriter(file, hash), inputs, progress)
	closeErr := file.Close()
	if packErr != nil {
		_ = os.Remove(temporary)
		return writtenResourcePackage{}, packErr
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return writtenResourcePackage{}, closeErr
	}
	if err := os.Rename(temporary, output); err != nil {
		_ = os.Remove(temporary)
		return writtenResourcePackage{}, err
	}
	if err := os.Chmod(output, 0o644); err != nil {
		return writtenResourcePackage{}, err
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	return writtenResourcePackage{
		size: result.Stats.PackedBytes, digest: digest, assets: result.Assets, stats: result.Stats,
	}, nil
}

func writeAtomic(name string, data []byte) error {
	temporary := name + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, name); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Chmod(name, 0o644)
}

func removeStalePackages(outputDir string, catalog resource.Catalog) error {
	keep := make(map[string]struct{}, len(catalog.Packages))
	for _, info := range catalog.Packages {
		keep[info.File] = struct{}{}
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("list package output directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), resource.Extension) {
			continue
		}
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(outputDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale package %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func loadResourcePackageConfig(projectDir string) (resourcePackageConfig, error) {
	name := filepath.Join(projectDir, "resources", packageConfigFile)
	return readOptionalJSONFile[resourcePackageConfig](name)
}

func matchesAny(patterns []compiledPackagePattern, name []string) bool {
	for _, pattern := range patterns {
		if matchPackagePattern(pattern, name) {
			return true
		}
	}
	return false
}

func matchPackagePattern(pattern compiledPackagePattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		return matchPackagePattern(pattern[1:], name) || len(name) > 0 && matchPackagePattern(pattern, name[1:])
	}
	if len(name) == 0 {
		return false
	}
	matched, _ := path.Match(pattern[0], name[0])
	return matched && matchPackagePattern(pattern[1:], name[1:])
}
