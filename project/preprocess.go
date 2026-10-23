//go:build !onix_release

package project

import (
	"errors"
	"fmt"
	iofs "io/fs"
	pathpkg "path"
	"slices"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	spineruntime "github.com/diiyw/onix/spine"
)

// PreprocessManifests compiles project, resource, prefab, catalog, and scene manifests.
func PreprocessManifests(source iofs.FS, options PreprocessOptions) ([]PreprocessedFile, error) {
	if source == nil {
		return nil, fmt.Errorf("project resource source is nil")
	}
	if !options.ShaderTarget.valid() {
		return nil, fmt.Errorf("unsupported shader target %d", options.ShaderTarget)
	}
	root := options.Root
	root = pathpkg.Clean(root)
	if root != "." && (!iofs.ValidPath(root) || strings.ContainsRune(root, '\\')) {
		return nil, fmt.Errorf("invalid project root %q", root)
	}
	manifestDir := pathpkg.Join(root, "resources")
	result := make([]PreprocessedFile, 0, 4)
	atlasPaths := make(map[string]struct{})

	projectPath := pathpkg.Join(manifestDir, projectFileName)
	if exists, err := projectFileExists(source, projectPath); err != nil {
		return nil, err
	} else if exists {
		manifest, err := readSourceJSON[gameManifest](source, projectPath)
		if err != nil {
			return nil, err
		}
		if err := manifest.Engine.Validate(); err != nil {
			return nil, fmt.Errorf("%s engine: %w", projectFileName, err)
		}
		compiled, err := preprocessManifest(source, projectPath, manifest)
		if err != nil {
			return nil, err
		}
		result = append(result, compiled)
	}

	resourcePath := pathpkg.Join(manifestDir, resourceFileName)
	if exists, err := projectFileExists(source, resourcePath); err != nil {
		return nil, err
	} else if exists {
		manifest, err := readSourceJSON[resourceManifest](source, resourcePath)
		if err != nil {
			return nil, err
		}
		models, err := preprocessModelResources(source, root, manifest.Models, options.TextureEncoder)
		if err != nil {
			return nil, err
		}
		result = append(result, models...)
		collisions, err := preprocessCollisionResources(source, root, manifest.Models)
		if err != nil {
			return nil, err
		}
		result = append(result, collisions...)
		for index := range manifest.Tilemaps {
			manifest.Tilemaps[index].Normalize()
			manifest.Tilemaps[index].Compact()
		}
		shaders, err := preprocessShaders(source, root, &manifest, options.ShaderTarget, options.ShaderCompiler)
		if err != nil {
			return nil, err
		}
		result = append(result, shaders...)
		if err := collectResourceAtlasPaths(atlasPaths, root, manifest); err != nil {
			return nil, err
		}
		spines, err := preprocessSpines(source, root, manifest.Spines)
		if err != nil {
			return nil, err
		}
		result = append(result, spines...)
		prefabs, err := normalizePrefabResourceSpecs(manifest.Prefabs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", resourceFileName, err)
		}
		for _, entry := range prefabs {
			prefabPath, err := ResolveResourcePath(root, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("resolve prefab %q: %w", entry.ID, err)
			}
			prefab, err := readSourceJSON[prefabManifest](source, prefabPath)
			if err != nil {
				return nil, fmt.Errorf("load prefab %q: %w", entry.ID, err)
			}
			if err := collectGameObjectAtlasPaths(atlasPaths, root, []SceneNodeSpec{prefab.Root}); err != nil {
				return nil, fmt.Errorf("prefab %q: %w", entry.ID, err)
			}
			if err := compilePrefabManifest(string(entry.ID), &prefab); err != nil {
				return nil, err
			}
			compiled, err := preprocessManifest(source, prefabPath, prefab)
			if err != nil {
				return nil, err
			}
			result = append(result, compiled)
		}
		manifest.Prepared = true
		compiled, err := preprocessManifest(source, resourcePath, manifest)
		if err != nil {
			return nil, err
		}
		result = append(result, compiled)
	}

	catalogPath := pathpkg.Join(manifestDir, sceneCatalogFileName)
	if exists, err := projectFileExists(source, catalogPath); err != nil {
		return nil, err
	} else if exists {
		catalog, err := readSourceJSON[sceneCatalog](source, catalogPath)
		if err != nil {
			return nil, err
		}
		if err := normalizeSceneCatalog(&catalog); err != nil {
			return nil, err
		}
		compiledCatalog, err := preprocessManifest(source, catalogPath, catalog)
		if err != nil {
			return nil, err
		}
		result = append(result, compiledCatalog)
		for _, entry := range catalog.Scenes {
			scenePath := pathpkg.Join(manifestDir, pathpkg.Clean(entry.Path))
			manifest, err := readSourceJSON[sceneManifest](source, scenePath)
			if err != nil {
				return nil, fmt.Errorf("load scene %q: %w", entry.ID, err)
			}
			if err := collectGameObjectAtlasPaths(atlasPaths, root, manifest.Nodes); err != nil {
				return nil, fmt.Errorf("scene %q: %w", entry.ID, err)
			}
			if err := compileSceneManifest(string(entry.ID), &manifest); err != nil {
				return nil, err
			}
			compiled, err := preprocessManifest(source, scenePath, manifest)
			if err != nil {
				return nil, err
			}
			result = append(result, compiled)
		}
	}
	atlases, err := preprocessAtlases(source, atlasPaths)
	if err != nil {
		return nil, err
	}
	result = append(result, atlases...)

	slices.SortFunc(result, func(left, right PreprocessedFile) int {
		return strings.Compare(left.Path, right.Path)
	})
	return result, nil
}

func preprocessModelResources(source iofs.FS, root string, models []ModelSpec, textureEncoder func([]byte, string, graphics.TextureOptions) ([]byte, error)) ([]PreprocessedFile, error) {
	compiled := make([]PreprocessedFile, 0, len(models))
	loaded := make(map[string]struct{}, len(models))
	type textureEncodeKey struct {
		path    string
		mime    string
		options graphics.TextureOptions
	}
	encodedTextures := make(map[textureEncodeKey][]byte)
	for _, model := range models {
		if err := model.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", resourceFileName, err)
		}
		if model.AnimationPath != "" {
			path, err := ResolveResourcePath(root, model.AnimationPath)
			if err != nil {
				return nil, err
			}
			if _, exists := loaded[path]; !exists {
				clips, err := readSourceJSON[[]ModelAnimationSource](source, path)
				if err != nil {
					return nil, err
				}
				file, err := preprocessManifest(source, path, clips)
				if err != nil {
					return nil, err
				}
				compiled = append(compiled, file)
				loaded[path] = struct{}{}
			}
		}
		modelPath := strings.TrimSpace(model.Path)
		if modelPath == "" {
			continue
		}
		normalized, err := NormalizeResourcePath(modelPath)
		if err != nil {
			return nil, fmt.Errorf("model %q path: %w", model.ID, err)
		}
		resolved, err := ResolveResourcePath(root, normalized)
		if err != nil {
			return nil, fmt.Errorf("model %q path: %w", model.ID, err)
		}
		if _, exists := loaded[resolved]; exists {
			continue
		}
		data, err := readProjectFile(source, resolved)
		if err != nil {
			if strings.TrimSpace(model.CollisionPath) != "" && errors.Is(err, iofs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read model resource %q: %w", modelPath, err)
		}
		resource, err := decodeModelResource(source, resolved, data)
		if err != nil {
			return nil, fmt.Errorf("model resource %q: %w", modelPath, err)
		}
		if model.AnimationPath != "" {
			path, err := ResolveResourcePath(root, model.AnimationPath)
			if err != nil {
				return nil, err
			}
			clips, err := readSourceJSON[[]ModelAnimationSource](source, path)
			if err != nil {
				return nil, err
			}
			if _, err = compileAnimationSources(clips, resource.Nodes); err != nil {
				return nil, fmt.Errorf("model %q animationPath: %w", model.ID, err)
			}
		}
		if err := resource.validate(); err != nil {
			return nil, fmt.Errorf("model resource %q: %w", modelPath, err)
		}
		if textureEncoder != nil {
			for materialIndex := range resource.Materials {
				material := &resource.Materials[materialIndex]
				type textureInput struct {
					name    string
					data    *[]byte
					path    *string
					mime    *string
					options graphics.TextureOptions
				}
				textures := [...]textureInput{
					{"base color", &material.BaseColorTextureData, &material.BaseColorTexture, &material.BaseColorTextureMime, material.BaseColorSampling},
					{"normal", &material.NormalTextureData, &material.NormalTexture, &material.NormalTextureMime, material.NormalSampling},
					{"metallic roughness", &material.MetallicRoughnessData, &material.MetallicRoughness, &material.MetallicRoughnessMime, material.MetallicRoughnessSampling},
				}
				surfaceNames := [...]string{"occlusion", "emissive", "specular", "specular color", "clearcoat", "clearcoat roughness", "clearcoat normal", "sheen color", "sheen roughness", "anisotropy"}
				for slot := range len(textures) + len(material.Surface.Textures) {
					var texture textureInput
					if slot < len(textures) {
						texture = textures[slot]
					} else {
						index := slot - len(textures)
						t := &material.Surface.Textures[index]
						texture = textureInput{surfaceNames[index], &t.Data, &t.Path, &t.Mime, t.Sampling}
					}
					data := *texture.data
					texturePath := strings.TrimSpace(*texture.path)
					if len(data) == 0 && texturePath != "" {
						resolvedTexture, err := ResolveResourcePath(".", texturePath)
						if err != nil {
							return nil, fmt.Errorf("model resource %q material %d %s texture: %w", modelPath, materialIndex, texture.name, err)
						}
						data, err = readProjectFile(source, resolvedTexture)
						if err != nil {
							return nil, fmt.Errorf("model resource %q material %d %s texture %q: %w", modelPath, materialIndex, texture.name, texturePath, err)
						}
					}
					if len(data) == 0 {
						continue
					}
					key := textureEncodeKey{path: texturePath, mime: *texture.mime, options: texture.options}
					encoded := encodedTextures[key]
					var err error
					if encoded == nil || texturePath == "" {
						encoded, err = textureEncoder(data, *texture.mime, texture.options)
						if err != nil {
							return nil, fmt.Errorf("model resource %q material %d %s texture: %w", modelPath, materialIndex, texture.name, err)
						}
						if texturePath != "" {
							encodedTextures[key] = encoded
						}
					}
					*texture.data, *texture.mime, *texture.path = encoded, "application/x-onixtex", ""
				}
			}
		}
		encoded, err := typedbin.Marshal(resource)
		if err != nil {
			return nil, fmt.Errorf("preprocess model resource %q: %w", modelPath, err)
		}
		compiled = append(compiled, PreprocessedFile{Path: resolved, Data: encoded, SourceSize: len(data)})
		loaded[resolved] = struct{}{}
	}
	return compiled, nil
}

func preprocessCollisionResources(source iofs.FS, root string, models []ModelSpec) ([]PreprocessedFile, error) {
	compiled := make([]PreprocessedFile, 0, len(models))
	loaded := make(map[string]CollisionResourceSpec, len(models))
	for _, model := range models {
		collisionPath := strings.TrimSpace(model.CollisionPath)
		if collisionPath == "" {
			continue
		}
		normalized, err := normalizeCollisionResourcePath(collisionPath)
		if err != nil {
			return nil, fmt.Errorf("model %q collisionPath: %w", model.ID, err)
		}
		resolved, err := ResolveResourcePath(root, normalized)
		if err != nil {
			return nil, fmt.Errorf("model %q collisionPath: %w", model.ID, err)
		}
		spec, exists := loaded[resolved]
		if !exists {
			spec, err = readSourceJSON[CollisionResourceSpec](source, resolved)
			if err != nil {
				return nil, fmt.Errorf("read collision resource %q: %w", collisionPath, err)
			}
			loaded[resolved] = spec
		}
		cooked, err := spec.cook(model.Collision)
		if err != nil {
			return nil, fmt.Errorf("collision resource %q: %w", collisionPath, err)
		}
		spec.Cooked = &cooked
		if exists {
			continue
		}
		file, err := preprocessManifest(source, resolved, spec)
		if err != nil {
			return nil, fmt.Errorf("preprocess collision resource %q: %w", collisionPath, err)
		}
		compiled = append(compiled, file)
	}
	return compiled, nil
}

func preprocessShaders(source iofs.FS, root string, manifest *resourceManifest, target ShaderTarget, compiler ShaderCompiler) ([]PreprocessedFile, error) {
	if manifest == nil {
		return nil, nil
	}
	return preprocessShaderSpecs(source, root, manifest.Shaders, target, compiler)
}

func preprocessShaderSpecs(source iofs.FS, root string, specs []ShaderSpec, target ShaderTarget, compiler ShaderCompiler) ([]PreprocessedFile, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	if target == ShaderTargetAll {
		for _, shader := range specs {
			if shader.WGSLVertex != "" || shader.WGSLFragment != "" {
				return nil, fmt.Errorf("%s shader %q requires a concrete build target for WGSL compilation", resourceFileName, shader.ID)
			}
		}
		return nil, nil
	}
	paths := make(map[string]bool, len(specs)*5)
	result := make([]PreprocessedFile, 0, len(specs)*3)
	for index := range specs {
		original := specs[index]
		if original.WGSLVertex != "" || original.WGSLFragment != "" {
			if compiler == nil {
				return nil, fmt.Errorf("%s shader %q requires a WGSL compiler for %s", resourceFileName, original.ID, target)
			}
			compiled, generated, sourcePaths, err := compileWGSLShader(source, root, original, target, compiler)
			if err != nil {
				return nil, fmt.Errorf("%s shader %q: %w", resourceFileName, original.ID, err)
			}
			specs[index] = compiled
			for _, name := range sourcePaths {
				paths[name] = false
			}
			for _, file := range generated {
				paths[file.Path] = true
			}
			result = append(result, generated...)
			for _, name := range compiled.SourcePaths() {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				resolved, err := ResolveResourcePath(root, name)
				if err != nil {
					return nil, fmt.Errorf("shader %q source %q: %w", original.ID, name, err)
				}
				paths[resolved] = true
			}
			continue
		}
		compiled, err := original.forTarget(target)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", resourceFileName, err)
		}
		specs[index] = compiled
		for _, name := range original.SourcePaths() {
			name = strings.TrimSpace(name)
			if name != "" {
				resolved, err := ResolveResourcePath(root, name)
				if err != nil {
					return nil, fmt.Errorf("shader %q source %q: %w", original.ID, name, err)
				}
				if _, exists := paths[resolved]; !exists {
					paths[resolved] = false
				}
			}
		}
		for _, name := range compiled.SourcePaths() {
			name = strings.TrimSpace(name)
			if name != "" {
				resolved, err := ResolveResourcePath(root, name)
				if err != nil {
					return nil, fmt.Errorf("shader %q source %q: %w", original.ID, name, err)
				}
				paths[resolved] = true
			}
		}
	}
	unused, err := removeUnusedShaderSources(source, target, paths)
	if err != nil {
		return nil, err
	}
	result = append(result, unused...)
	return result, nil
}

func removeUnusedShaderSources(source iofs.FS, target ShaderTarget, paths map[string]bool) ([]PreprocessedFile, error) {
	result := make([]PreprocessedFile, 0, len(paths))
	for name, kept := range paths {
		if kept {
			continue
		}
		info, err := iofs.Stat(source, name)
		if err != nil {
			return nil, fmt.Errorf("stat unused %s shader source %s: %w", target, name, err)
		}
		if info.IsDir() || info.Size() < 0 || uint64(info.Size()) > uint64(^uint(0)>>1) {
			return nil, fmt.Errorf("unused %s shader source %s has invalid size", target, name)
		}
		result = append(result, PreprocessedFile{Path: name, SourceSize: int(info.Size()), Remove: true})
	}
	return result, nil
}

type wgslCompiledSource struct {
	path   string
	result ShaderCompileResult
}

func compileWGSLShader(source iofs.FS, root string, original ShaderSpec, target ShaderTarget, compiler ShaderCompiler) (ShaderSpec, []PreprocessedFile, []string, error) {
	if target == ShaderTargetAndroid {
		vulkan, vulkanFiles, paths, err := compileWGSLShader(source, root, original, ShaderTargetVulkan, compiler)
		if err != nil {
			return ShaderSpec{}, nil, nil, err
		}
		gles, glesFiles, _, err := compileWGSLShader(source, root, original, ShaderTargetWebGL2, compiler)
		if err != nil {
			return ShaderSpec{}, nil, nil, err
		}
		vulkan.WebGL2Vertex = gles.WebGL2Vertex
		vulkan.WebGL2Fragment = gles.WebGL2Fragment
		vulkan.WebGL2Generated = gles.WebGL2Generated
		return vulkan, append(vulkanFiles, glesFiles...), paths, nil
	}
	paths := make([]string, 0, 2)
	for _, name := range []string{original.WGSLVertex, original.WGSLFragment} {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		resolved, err := ResolveResourcePath(root, name)
		if err != nil {
			return ShaderSpec{}, nil, nil, fmt.Errorf("resolve WGSL source %q: %w", name, err)
		}
		if !slices.Contains(paths, resolved) {
			paths = append(paths, resolved)
		}
	}
	if target == ShaderTargetMetal && len(paths) > 1 {
		return ShaderSpec{}, nil, nil, fmt.Errorf("Metal requires vertex and fragment entrypoints in the same WGSL module")
	}
	compiled := original
	compiled.WGSLVertex, compiled.WGSLFragment = "", ""
	compiled.VulkanVertex, compiled.VulkanFragment, compiled.Metal = "", "", ""
	compiled.WebGL2Vertex, compiled.WebGL2Fragment = "", ""
	compiled.WebGPUVertex, compiled.WebGPUFragment = "", ""
	compiled.WebGL2Generated = false

	results := make([]wgslCompiledSource, 0, len(paths))
	for _, path := range paths {
		data, err := readProjectFile(source, path)
		if err != nil {
			return ShaderSpec{}, nil, nil, fmt.Errorf("read WGSL source %q: %w", path, err)
		}
		result, err := compiler.CompileShader(ShaderCompileRequest{ID: original.ID, SourcePath: path, Source: data, Target: target})
		if err != nil {
			return ShaderSpec{}, nil, nil, err
		}
		results = append(results, wgslCompiledSource{path: path, result: result})
	}
	generated := make([]PreprocessedFile, 0, 3)
	addStage := func(stage, extension, logicalSourcePath, sourcePath string, data []byte) string {
		if len(data) == 0 {
			return ""
		}
		path := sourcePath + ".onix." + strings.ToLower(target.String()) + "." + stage + "." + extension
		generated = append(generated, PreprocessedFile{Path: path, SourcePath: sourcePath, Data: data, SourceSize: len(data)})
		return logicalSourcePath + ".onix." + strings.ToLower(target.String()) + "." + stage + "." + extension
	}
	find := func(name string) (wgslCompiledSource, bool) {
		resolved, err := ResolveResourcePath(root, name)
		if err != nil {
			return wgslCompiledSource{}, false
		}
		for _, item := range results {
			if item.path == resolved {
				return item, true
			}
		}
		return wgslCompiledSource{}, false
	}
	if original.WGSLVertex != "" {
		item, ok := find(original.WGSLVertex)
		if !ok || !item.result.HasVertexEntry {
			return ShaderSpec{}, nil, nil, fmt.Errorf("WGSL source has no vertex entrypoint")
		}
		switch target {
		case ShaderTargetVulkan:
			compiled.VulkanVertex = addStage("vertex", "spv", original.WGSLVertex, item.path, item.result.Vertex)
		case ShaderTargetWebGL2:
			compiled.WebGL2Vertex = addStage("vertex", "glsl", original.WGSLVertex, item.path, item.result.Vertex)
			compiled.WebGL2Generated = item.result.WebGL2Generated
		case ShaderTargetWebGPU:
			compiled.WebGPUVertex = original.WGSLVertex
		case ShaderTargetMetal:
			compiled.Metal = addStage("module", "metal", original.WGSLVertex, item.path, item.result.Metal)
		}
	}
	if original.WGSLFragment != "" {
		item, ok := find(original.WGSLFragment)
		if !ok || !item.result.HasFragmentEntry {
			return ShaderSpec{}, nil, nil, fmt.Errorf("WGSL source has no fragment entrypoint")
		}
		switch target {
		case ShaderTargetVulkan:
			compiled.VulkanFragment = addStage("fragment", "spv", original.WGSLFragment, item.path, item.result.Fragment)
		case ShaderTargetWebGL2:
			compiled.WebGL2Fragment = addStage("fragment", "glsl", original.WGSLFragment, item.path, item.result.Fragment)
			compiled.WebGL2Generated = item.result.WebGL2Generated
		case ShaderTargetWebGPU:
			compiled.WebGPUFragment = original.WGSLFragment
		case ShaderTargetMetal:
			if compiled.Metal == "" {
				compiled.Metal = addStage("module", "metal", original.WGSLFragment, item.path, item.result.Metal)
			}
		}
	}
	if err := compiled.Validate(); err != nil {
		return ShaderSpec{}, nil, nil, err
	}
	return compiled, generated, paths, nil
}

func collectResourceAtlasPaths(paths map[string]struct{}, root string, manifest resourceManifest) error {
	for _, effect := range manifest.Effects {
		if err := collectTextureAtlasPath(paths, root, effect.Texture); err != nil {
			return fmt.Errorf("effect %q: %w", effect.ID, err)
		}
	}
	return nil
}

func collectGameObjectAtlasPaths(paths map[string]struct{}, root string, items []SceneNodeSpec) error {
	for index := range items {
		if sprite := items[index].Components.Sprite; sprite != nil {
			if err := collectTextureAtlasPath(paths, root, sprite.Texture); err != nil {
				return fmt.Errorf("node %q sprite: %w", items[index].ID, err)
			}
		}
		if animation := items[index].Components.Animation; animation != nil {
			clip := &animation.Clip
			for trackIndex := range clip.TextureTracks {
				for keyIndex := range clip.TextureTracks[trackIndex].Keyframes {
					if err := collectTextureAtlasPath(paths, root, &clip.TextureTracks[trackIndex].Keyframes[keyIndex].Value); err != nil {
						return fmt.Errorf("node %q animation: %w", items[index].ID, err)
					}
				}
			}
		}
		if err := collectGameObjectAtlasPaths(paths, root, items[index].Children); err != nil {
			return err
		}
	}
	return nil
}

func collectTextureAtlasPath(paths map[string]struct{}, root string, texture *TextureSpec) error {
	if texture == nil || strings.TrimSpace(texture.Atlas) == "" {
		return nil
	}
	resolved, err := ResolveResourcePath(root, texture.Atlas)
	if err != nil {
		return fmt.Errorf("atlas %q: %w", texture.Atlas, err)
	}
	paths[resolved] = struct{}{}
	return nil
}

func preprocessAtlases(source iofs.FS, paths map[string]struct{}) ([]PreprocessedFile, error) {
	result := make([]PreprocessedFile, 0, len(paths))
	for path := range paths {
		data, err := readProjectFile(source, path)
		if err != nil {
			return nil, fmt.Errorf("preprocess atlas metadata %s: %w", path, err)
		}
		compiled, atlas, err := graphics.PreprocessAtlas(data)
		if err != nil {
			return nil, fmt.Errorf("preprocess atlas metadata %s: %w", path, err)
		}
		if !atlas {
			return nil, fmt.Errorf("preprocess atlas metadata %s: source is not TexturePacker JSON", path)
		}
		result = append(result, PreprocessedFile{Path: path, Data: compiled, SourceSize: len(data)})
	}
	return result, nil
}

func preprocessSpines(source iofs.FS, root string, specs []SpineSpec) ([]PreprocessedFile, error) {
	atlases := make(map[string]string, len(specs))
	atlasData := make(map[string][]byte, len(specs))
	result := make([]PreprocessedFile, 0, len(specs)*2)
	for _, spec := range specs {
		if err := spec.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", resourceFileName, err)
		}
		skeletonPath, err := ResolveResourcePath(root, spec.Skeleton)
		if err != nil {
			return nil, fmt.Errorf("spine %q skeleton: %w", spec.ID, err)
		}
		atlasPath, err := ResolveResourcePath(root, spec.Atlas)
		if err != nil {
			return nil, fmt.Errorf("spine %q atlas: %w", spec.ID, err)
		}
		if previous, exists := atlases[skeletonPath]; exists {
			if previous != atlasPath {
				return nil, fmt.Errorf("%s: Spine skeleton %q uses conflicting atlases %q and %q", resourceFileName, skeletonPath, previous, atlasPath)
			}
			continue
		}
		atlases[skeletonPath] = atlasPath
		skeleton, err := readProjectFile(source, skeletonPath)
		if err != nil {
			return nil, fmt.Errorf("preprocess Spine skeleton %s: %w", skeletonPath, err)
		}
		atlas, exists := atlasData[atlasPath]
		if !exists {
			atlas, err = readProjectFile(source, atlasPath)
			if err != nil {
				return nil, fmt.Errorf("preprocess Spine atlas %s: %w", atlasPath, err)
			}
			atlasData[atlasPath] = atlas
		}
		compiled, err := spineruntime.Preprocess(skeleton, atlas)
		if err != nil {
			return nil, fmt.Errorf("preprocess Spine skeleton %s: %w", skeletonPath, err)
		}
		result = append(result, PreprocessedFile{Path: skeletonPath, Data: compiled, SourceSize: len(skeleton)})
	}
	for path, data := range atlasData {
		result = append(result, PreprocessedFile{Path: path, SourceSize: len(data), Remove: true})
	}
	return result, nil
}

func preprocessManifest[T any](source iofs.FS, path string, value T) (PreprocessedFile, error) {
	data, err := typedbin.Marshal(value)
	if err != nil {
		return PreprocessedFile{}, fmt.Errorf("preprocess %s: %w", path, err)
	}
	sourceData, err := readProjectFile(source, path)
	if err != nil {
		return PreprocessedFile{}, err
	}
	return PreprocessedFile{Path: path, Data: data, SourceSize: len(sourceData)}, nil
}

func readSourceJSON[T any](source iofs.FS, path string) (T, error) {
	var target T
	data, err := readProjectFile(source, path)
	if err != nil {
		return target, err
	}
	return decodeJSON[T](data, path)
}
