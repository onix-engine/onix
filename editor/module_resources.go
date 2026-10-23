package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"sort"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/resource"
)

type moduleAssetCollector struct {
	directory string
	files     map[string]resourceInput
	owners    map[string]string
	index     moduleOwnerIndex
	kinds     map[string]string
	queue     []string
}

// collectModuleAssets collects declared resources and dependencies, once per
// used definition across all registered scenes. No layout is imposed on modules.
func collectModuleAssets(directory string, plan *project.ProjectPlan, config resourcePackageConfig) (map[string]resourceInput, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	c := moduleAssetCollector{directory: directory, index: newModuleOwnerIndex(plan), files: make(map[string]resourceInput), owners: make(map[string]string), kinds: make(map[string]string)}
	if icon := plan.Spec.Window.Icon; icon != "" {
		if !fs.ValidPath(icon) {
			return nil, fmt.Errorf("invalid window icon %q", icon)
		}
		name := path.Join("resources", icon)
		absolute := filepath.Join(directory, filepath.FromSlash(name))
		real, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(directory, real)
		if err != nil || !pathWithinRoot(relative) {
			return nil, fmt.Errorf("window icon escapes project")
		}
		info, err := os.Stat(real)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("window icon must be a file")
		}
		c.files[name] = resourceInput{source: absolute}
	}
	used := make([]bool, len(plan.Mods))
	for _, s := range plan.Scenes {
		for _, i := range s.Instances {
			used[i.Mod] = true
		}
	}
	for i, m := range plan.Mods {
		if !used[i] {
			continue
		}
		owner := string(m.ID)
		for _, r := range m.Resources {
			c.kinds[r.Path] = r.Kind
			if err := c.add(owner, r.Path); err != nil {
				return nil, fmt.Errorf("%s/mod.json resources.%s: %w", owner, r.Name, err)
			}
		}
		texture := func(t *project.TextureSpec) error {
			if t == nil {
				return nil
			}
			if t.Atlas != "" {
				c.kinds[path.Join(owner, t.Atlas)] = "atlas"
			}
			for _, p := range []string{t.Image, t.Atlas} {
				if p != "" {
					if err := c.add(owner, path.Join(owner, p)); err != nil {
						return err
					}
				}
			}
			return nil
		}
		content := func(components project.ModComponents) error {
			if shader := components.Shader; shader != nil {
				for _, p := range shader.SourcePaths() {
					if p != "" {
						if err := c.add(owner, path.Join(owner, p)); err != nil {
							return err
						}
					}
				}
			}
			if m := components.Model; m != nil {
				for _, p := range []string{m.Path, m.AnimationPath, m.CollisionPath} {
					if p != "" {
						if err := c.add(owner, path.Join(owner, p)); err != nil {
							return err
						}
					}
				}
			}
			if s := components.Spine; s != nil {
				c.kinds[path.Join(owner, s.Source.Skeleton)] = "spine"
				c.kinds[path.Join(owner, s.Source.Atlas)] = "spine"
				for _, p := range []string{s.Source.Skeleton, s.Source.Atlas, s.Source.Texture} {
					if err := c.add(owner, path.Join(owner, p)); err != nil {
						return err
					}
				}
			}
			if components.Sprite != nil {
				if err := texture(components.Sprite.Texture); err != nil {
					return err
				}
			}
			if err := texture(components.Particle); err != nil {
				return err
			}
			if t := components.Tilemap; t != nil && t.Tileset != "" {
				if err := c.add(owner, path.Join(owner, t.Tileset)); err != nil {
					return err
				}
			}
			if camera := components.Camera; camera != nil && camera.Environment != nil && camera.Environment.Path != "" {
				name := path.Join(owner, camera.Environment.Path)
				if err := c.add(owner, name); err != nil {
					return err
				}
				input := c.files[name]
				input.preserveTexture = true
				c.files[name] = input
			}
			if a := components.Animation; a != nil {
				for _, track := range a.Clip.TextureTracks {
					for _, key := range track.Keyframes {
						if err := texture(&key.Value); err != nil {
							return err
						}
					}
				}
			}
			return nil
		}
		if err := content(m.Spec.Components); err != nil {
			return nil, err
		}
		for _, element := range m.Elements {
			if err := content(element.Spec.Components); err != nil {
				return nil, err
			}
		}
	}
	// Extra dynamic resources must be declared by a project keep/include pattern.
	// They still belong to one module and cannot reach a sibling or engine tree.
	var patterns []string
	patterns = append(patterns, config.Keep...)
	for _, r := range config.Packages {
		patterns = append(patterns, r.Include...)
	}
	if len(patterns) > 0 {
		compiled := make([]compiledPackagePattern, len(patterns))
		matched := make([]bool, len(patterns))
		for i, p := range patterns {
			if !filepath.IsLocal(filepath.FromSlash(p)) {
				return nil, fmt.Errorf("resource pattern %q escapes project", p)
			}
			compiled[i] = strings.Split(path.Clean(p), "/")
		}
		err := filepath.WalkDir(directory, func(name string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(directory, name)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if e.IsDir() {
				if e.Name() == "dist" || strings.HasPrefix(e.Name(), ".") && rel != "." {
					return filepath.SkipDir
				}
				return nil
			}
			if e.Name() == "mod.json" || e.Name() == "go.mod" || e.Name() == "go.sum" || ignoredResourceFile(e) {
				return nil
			}
			parts := strings.Split(rel, "/")
			for i, p := range compiled {
				if !matchPackagePattern(p, parts) {
					continue
				}
				matched[i] = true
				owner := c.owner(rel)
				if owner == "" {
					return fmt.Errorf("%s: dynamic resources must belong to a module", rel)
				}
				if err := c.add(owner, rel); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for i, ok := range matched {
			if !ok {
				return nil, fmt.Errorf("resource pattern %q matched no module assets", patterns[i])
			}
		}
	}
	for i := 0; i < len(c.queue); i++ {
		name := c.queue[i]
		// Spine export metadata and atlas region names are not file references;
		// runtime dependencies are its explicitly declared atlas and texture.
		if c.kinds[name] == "data" || c.kinds[name] == "spine" {
			continue
		}
		extension := strings.ToLower(path.Ext(name))
		if !scannableResource(name) && extension != ".html" && extension != ".css" {
			continue
		}
		data, err := c.files[name].bytes()
		if err != nil {
			return nil, err
		}
		visitor := func(reference string) error {
			if !looksLikeResourceReference(reference) || strings.HasPrefix(reference, "data:") {
				return nil
			}
			u, err := url.Parse(reference)
			if err != nil {
				return fmt.Errorf("%s: invalid resource reference %q", name, reference)
			}
			if u.IsAbs() || u.Host != "" {
				return nil
			}
			p := u.Path
			if strings.HasPrefix(p, "/") || strings.ContainsAny(p, `\:`) {
				return fmt.Errorf("%s: resource reference %q escapes module %q", name, reference, c.owners[name])
			}
			resolved := path.Clean(path.Join(path.Dir(name), p))
			if !fs.ValidPath(resolved) || c.owner(resolved) != c.owners[name] {
				return fmt.Errorf("%s: resource reference %q escapes module %q", name, reference, c.owners[name])
			}
			if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(resolved))); os.IsNotExist(err) {
				return fmt.Errorf("%s: missing resource dependency %q", name, reference)
			} else if err != nil {
				return err
			}
			return c.add(c.owners[name], resolved)
		}
		switch {
		case c.kinds[name] == "atlas":
			var atlas struct{ Meta struct{ Image string } }
			if err = json.Unmarshal(data, &atlas); err == nil {
				err = visitor(atlas.Meta.Image)
			}
		case extension == ".gltf":
			err = visitGLTFReferences(data, visitor)
		case extension == ".glb":
			if document, ok := glbJSON(data); ok {
				err = visitGLTFReferences(document, visitor)
			}
		case extension == ".json":
			err = visitJSONStrings(data, visitor)
		case extension == ".atlas":
			if len(data) > 0 && strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
				err = visitJSONStrings(data, visitor)
			} else {
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if looksLikeResourceReference(line) && !strings.ContainsAny(line, "\t :") {
						if err = visitor(line); err != nil {
							break
						}
					}
				}
			}
		case extension == ".html" || extension == ".css":
			for _, match := range moduleHTMLAssetReference.FindAllSubmatch(data, -1) {
				value := match[1]
				if len(value) == 0 {
					value = match[2]
				}
				if err = visitor(string(value)); err != nil {
					break
				}
			}
		default:
			err = visitTextReferences(data, visitor)
		}
		if err != nil {
			return nil, err
		}
	}
	if plan.Spec.Window.Icon != "" {
		p, err := project.NormalizeLocalResourcePath(plan.Spec.Window.Icon)
		if err != nil {
			return nil, err
		}
		name := path.Join("resources", p)
		c.files[name] = resourceInput{source: filepath.Join(directory, filepath.FromSlash(name))}
	}
	return c.files, nil
}

func (c *moduleAssetCollector) owner(name string) string {
	return c.index.owner(name)
}

type moduleOwnerIndex map[string]struct{}

func newModuleOwnerIndex(plan *project.ProjectPlan) moduleOwnerIndex {
	index := make(moduleOwnerIndex, len(plan.Mods))
	for _, m := range plan.Mods {
		index[string(m.ID)] = struct{}{}
	}
	return index
}
func (index moduleOwnerIndex) owner(name string) string {
	for name != "." {
		if _, ok := index[name]; ok {
			return name
		}
		parent := path.Dir(name)
		if parent == name {
			break
		}
		name = parent
	}
	return ""
}
func (c *moduleAssetCollector) add(owner, name string) error {
	if !fs.ValidPath(name) || c.owner(name) != owner {
		return fmt.Errorf("%s must stay within module %q", name, owner)
	}
	if _, ok := c.files[name]; ok {
		return nil
	}
	absolute := filepath.Join(c.directory, filepath.FromSlash(name))
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	realOwner, err := filepath.EvalSymlinks(filepath.Join(c.directory, filepath.FromSlash(owner)))
	if err != nil {
		return err
	}
	realProject, err := filepath.EvalSymlinks(c.directory)
	if err != nil {
		return err
	}
	projectRelative, err := filepath.Rel(realProject, real)
	if err != nil || !filepath.IsLocal(projectRelative) || c.owner(filepath.ToSlash(projectRelative)) != owner {
		return fmt.Errorf("%s: symlink escapes module %q", name, owner)
	}
	rel, err := filepath.Rel(realOwner, real)
	if err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("%s: symlink escapes module %q", name, owner)
	}
	if info, err := os.Stat(real); err != nil {
		return err
	} else if info.IsDir() {
		return fmt.Errorf("%s: resource must be a file", name)
	}
	c.files[name] = resourceInput{source: absolute}
	c.owners[name] = owner
	c.queue = append(c.queue, name)
	return nil
}

func partitionModuleAssets(files map[string]resourceInput, config resourcePackageConfig) (map[string]map[string]resourceInput, map[string]resourcePackageRule, error) {
	if err := validateResourcePackageConfig(config); err != nil {
		return nil, nil, err
	}
	rules := make(map[string]resourcePackageRule, len(config.Packages))
	compiled := make([]compiledPackageRule, 0, len(config.Packages))
	for _, r := range config.Packages {
		x := compiledPackageRule{resourcePackageRule: r}
		rules[r.Name] = r
		for _, p := range r.Include {
			x.include = append(x.include, strings.Split(path.Clean(p), "/"))
		}
		for _, p := range r.Exclude {
			x.exclude = append(x.exclude, strings.Split(path.Clean(p), "/"))
		}
		compiled = append(compiled, x)
	}
	groups := map[string]map[string]resourceInput{resource.BasePackage: {}}
	for name, input := range files {
		owner := resource.BasePackage
		if name != project.ModulePlanFile {
			parts := strings.Split(name, "/")
			for _, r := range compiled {
				if matchesAny(r.include, parts) && !matchesAny(r.exclude, parts) {
					if owner != resource.BasePackage {
						return nil, nil, fmt.Errorf("resource %q matches multiple packages", name)
					}
					owner = r.Name
				}
			}
		}
		if groups[owner] == nil {
			groups[owner] = make(map[string]resourceInput)
		}
		groups[owner][name] = input
	}
	for _, r := range config.Packages {
		if len(groups[r.Name]) == 0 {
			return nil, nil, fmt.Errorf("resource package %q matched no published assets", r.Name)
		}
	}
	return groups, rules, nil
}

func writeModuleResourcePackages(root, directory, output string, options resourcePackageOptions) (resource.Catalog, []byte, error) {
	defer debug.FreeOSMemory()
	reportBuildActivity(options.Progress, "Collecting module resources...", 0)
	plan, err := project.CompileProjectDirectory(directory)
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	config, err := loadResourcePackageConfig(directory)
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	files, err := collectModuleAssets(directory, plan, config)
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	data, err := project.MarshalModulePlan(plan)
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	files[project.ModulePlanFile] = memoryResourceInput(data)
	digests := loadResourceDigestCache(directory)
	fingerprint, err := fingerprintResourceBuild(files, config, options, digests)
	digests.save()
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	if catalog, bytes, ok := restoreCachedResourceBuild(directory, output, fingerprint, options); ok {
		return catalog, bytes, nil
	}
	groups, rules, err := partitionModuleAssets(files, config)
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	clear(files)
	if err := os.MkdirAll(output, 0o755); err != nil {
		return resource.Catalog{}, nil, err
	}
	temporary, err := os.MkdirTemp(output, ".onix-module-preprocess-")
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	defer os.RemoveAll(temporary)
	context := resourcePreprocessContext{Root: root, ProjectDir: directory, TempDir: temporary, Options: options, Config: config}
	target := textureTargetForBuildPlatform(options.Platform)
	icon := path.Join("resources", plan.Spec.Window.Icon)
	if plan.Spec.Engine.TextureCompression && target != TextureTargetSource && !target.Encodable() {
		return resource.Catalog{}, nil, fmt.Errorf("texture compression %s is unavailable for %s", target, options.Platform)
	}
	reportBuildActivity(options.Progress, "Compiling module resources...", .2)
	if err := preprocessModuleContent(context, plan, groups); err != nil {
		return resource.Catalog{}, nil, err
	}
	if plan.Spec.Engine.TextureCompression && target.Encodable() {
		for _, group := range groups {
			for name, input := range group {
				if input.preserveTexture || name == icon || textureCompressionExcluded(plan.Spec.Engine.TextureCompressionExcludes, name) || !compressibleTextureSource(name) {
					continue
				}
				if options.Progress != nil {
					reportBuildActivity(options.Progress, "Compressing texture "+name, .5)
				}
				data, err := input.bytes()
				if err != nil {
					return resource.Catalog{}, nil, err
				}
				// These vectors are colored at runtime. Rasterizing them here
				// freezes currentColor to SVG's default black before a theme exists.
				if strings.EqualFold(path.Ext(name), ".svg") && bytes.Contains(bytes.ToLower(data), []byte("currentcolor")) {
					continue
				}
				packed, err := packTextureSource(data, name, target, options.Mode, false)
				if err != nil {
					return resource.Catalog{}, nil, err
				}
				if packed != nil {
					group[name], err = context.store(name, packed)
					if err != nil {
						return resource.Catalog{}, nil, err
					}
				}
			}
		}
	}
	if err := preprocessPackageAssets(context, groups); err != nil {
		return resource.Catalog{}, nil, err
	}
	infos := make([]resource.PackageInfo, 0, len(groups))
	report := newResourceBuildReport(fingerprint, options, len(groups))
	packages := make([]string, 0, len(groups))
	var total, completed int
	for name, group := range groups {
		packages = append(packages, name)
		total += 2 * len(group)
	}
	sort.Strings(packages)
	for packageIndex, name := range packages {
		file := name + resource.Extension
		if options.Progress != nil {
			options.Progress(BuildProgress{Message: fmt.Sprintf("Packing module package %s (%d/%d)", name, packageIndex+1, len(groups)), Value: .6 + .35*float32(completed)/float32(max(1, total))})
		}
		var progress func(resource.PackProgress)
		if options.Progress != nil {
			progress = func(p resource.PackProgress) {
				verb := "Checking"
				if p.Writing {
					verb = "Packing"
				}
				options.Progress(BuildProgress{Message: fmt.Sprintf("%s %s / %s (%d/%d)", verb, name, p.Path, p.Completed, p.Total), Value: .6 + .35*float32(completed+p.Completed)/float32(max(1, total)), Indeterminate: p.Active})
			}
		}
		written, err := writeResourcePackageWithProgress(filepath.Join(output, file), groups[name], progress)
		if err != nil {
			return resource.Catalog{}, nil, err
		}
		completed += 2 * len(groups[name])
		if options.Progress != nil {
			options.Progress(BuildProgress{Message: fmt.Sprintf("Packed module package %s (%d/%d, %d resources)", name, packageIndex+1, len(groups), written.stats.Files), Value: .6 + .35*float32(completed)/float32(max(1, total))})
		}
		clear(groups[name])
		// All authored scenes are immediately switchable. Their asset packages are
		// startup dependencies; splitting does not make a scene silently incomplete.
		info := resource.PackageInfo{Name: name, File: file, SHA256: written.digest, Size: written.size, Assets: written.assets, Required: true}
		if rule, ok := rules[name]; ok {
			info.Dependencies = append([]string{resource.BasePackage}, rule.Dependencies...)
			sort.Strings(info.Dependencies)
			info.Dependencies = slices.Compact(info.Dependencies)
		}
		infos = append(infos, info)
		report.addPackage(name, file, written.stats)
	}
	catalog := resource.Catalog{Version: resource.CatalogVersion, Packages: infos}
	bytes, err := resource.MarshalCatalog(catalog)
	if err != nil {
		return resource.Catalog{}, nil, err
	}
	if err := writeAtomic(filepath.Join(output, resource.CatalogFile), bytes); err != nil {
		return resource.Catalog{}, nil, err
	}
	if err := removeStalePackages(output, catalog); err != nil {
		return resource.Catalog{}, nil, err
	}
	report.sortPackages()
	reportBuildActivity(options.Progress, "Saving resource build cache...", .98)
	storeCachedResourceBuild(directory, output, catalog, bytes, report)
	if options.Progress != nil {
		options.Progress(BuildProgress{Message: "Resource packages ready", Value: 1})
	}
	return catalog, bytes, nil
}

var moduleHTMLAssetReference = regexp.MustCompile(`(?i)(?:src|href|poster)\s*=\s*["']([^"']+)["']|url\(\s*["']?([^"')\s]+)["']?\s*\)`)

func preprocessModuleContent(context resourcePreprocessContext, plan *project.ProjectPlan, groups map[string]map[string]resourceInput) error {
	compiler := context.Options.ShaderCompiler
	if compiler == nil {
		compiler = NagaShaderCompiler{}
	}
	options := project.PreprocessOptions{ShaderTarget: shaderTargetForBuildPlatform(context.Options.Platform), ShaderCompiler: compiler}
	if context.Options.Progress != nil {
		options.Progress = func(module, asset string, completed, total int) {
			context.Options.Progress(BuildProgress{Message: fmt.Sprintf("Compiling module %s / %s (%d/%d)", module, asset, completed, total), Value: .2 + .3*float32(completed)/float32(max(1, total)), Indeterminate: asset != "ready"})
		}
	}
	target := textureTargetForBuildPlatform(context.Options.Platform)
	if plan.Spec.Engine.TextureCompression && target.Encodable() {
		options.TextureEncoder = func(data []byte, mime string, opts graphics.TextureOptions) ([]byte, error) {
			return packEmbeddedTextureSource(data, mime, target, context.Options.Mode, opts)
		}
	}
	files, err := project.PreprocessModuleAssets(os.DirFS(context.ProjectDir), plan, options)
	if err != nil {
		return err
	}
	for pass := range 2 {
		for _, file := range files {
			generated := file.SourcePath != ""
			if generated != (pass == 0) {
				continue
			}
			input := resourceInput{}
			if !file.Remove {
				input, err = context.store(file.Path, file.Data)
				if err != nil {
					return err
				}
			}
			if updateGroupedResource(groups, file.Path, input, file.Remove) {
				continue
			}
			if generated && addGeneratedResource(groups, file.SourcePath, file.Path, input) {
				continue
			}
			return fmt.Errorf("module asset %q was not collected", file.Path)
		}
	}
	data, err := project.MarshalModulePlan(plan)
	if err != nil {
		return err
	}
	groups[resource.BasePackage][project.ModulePlanFile] = memoryResourceInput(data)
	return nil
}
