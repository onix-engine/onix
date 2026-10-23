package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"hash"
	"io"
	"io/fs"
	"maps"
	"math"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/diiyw/onix/resource"
	"github.com/go-gl/mathgl/mgl32"
)

type modIndices struct {
	slots   map[string]uint32
	models  map[string]ModelSpec
	spines  map[string]SpineSpec
	physics bool
}

type modCompiler struct {
	source fs.FS
	plan   ProjectPlan
	mods   map[ModID]uint32
	index  []modIndices
	cache  *CompileCache
	files  map[string]struct{}
	hasher hash.Hash
	buffer []byte
}

// CompileCache retains immutable module plans between authoring transactions.
// Scenes are always rebuilt so changed slots, types and hierarchy are validated.
// A cache belongs to one sequential compilation owner.
type CompileCache struct {
	types map[ValueType]ValueSchema
	mods  map[ModID]cachedModPlan
}

type cachedModPlan struct {
	digest [sha256.Size]byte
	plan   ModPlan
	index  modIndices
	files  []string
}

func (cache *CompileCache) Compile(source fs.FS, root string, definitions []ModID) (*ProjectPlan, error) {
	return compileProjectDefinitions(source, root, definitions, cache)
}

// CompileProject reads only the module-system format. It never falls back to
// scenes.json, node hierarchies, prefabs, or project-global resource references.
// Compilation owns allocations; the resulting plans use indices and typed data.
func CompileProject(source fs.FS, root string) (*ProjectPlan, error) {
	return compileProjectDefinitions(source, root, nil, nil)
}

// CompileProjectDefinitions also validates definitions already opened by an
// authoring tool, even when a nested definition has no remaining scene instance.
func CompileProjectDefinitions(source fs.FS, root string, definitions []ModID) (*ProjectPlan, error) {
	return compileProjectDefinitions(source, root, definitions, nil)
}

func compileProjectDefinitions(source fs.FS, root string, definitions []ModID, cache *CompileCache) (*ProjectPlan, error) {
	if source == nil {
		return nil, fmt.Errorf("project source is nil")
	}
	if root != "." {
		if !fs.ValidPath(root) || strings.ContainsAny(root, `\:`) {
			return nil, fmt.Errorf("invalid project root %q", root)
		}
		var err error
		source, err = resource.Sub(source, root)
		if err != nil {
			return nil, err
		}
	}
	spec, err := readModJSON[ProjectSpec](source, "resources/project.json")
	if err != nil {
		return nil, err
	}
	spec.Scenes, err = discoverScenes(source, spec.Scenes)
	if err != nil {
		return nil, err
	}
	if err := ValidateProjectSpec(spec); err != nil {
		return nil, fmt.Errorf("resources/project.json: %w", err)
	}
	if cache != nil {
		if cache.mods == nil {
			cache.mods = make(map[ModID]cachedModPlan)
		} else if !maps.EqualFunc(cache.types, spec.Types, func(a, b ValueSchema) bool { return maps.Equal(a.Fields, b.Fields) }) {
			clear(cache.mods)
		}
		cache.types = spec.Types
	}
	compiler := modCompiler{source: source, plan: ProjectPlan{Spec: spec}, mods: make(map[ModID]uint32), cache: cache}
	// Only mod.json identifies a module. Folder names such as resources, images,
	// or contracts have no special meaning to the module discovery process.
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, fmt.Errorf("discover modules: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := path.Join(entry.Name(), ModFileName)
		if _, err := fs.Stat(source, name); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if _, err := compiler.loadMod(ModID(entry.Name())); err != nil {
			return nil, err
		}
	}
	for _, id := range definitions {
		if _, err := compiler.loadMod(id); err != nil {
			return nil, err
		}
	}
	for index, id := range spec.Scenes {
		scene, err := readModJSON[SceneSpec](source, ScenePath(id))
		if err != nil {
			return nil, err
		}
		if scene.Modules == nil {
			return nil, fmt.Errorf("%s: modules must be a JSON array", id)
		}
		compiled, err := compiler.compileScene(id, scene)
		if err != nil {
			return nil, err
		}
		compiler.plan.Scenes = append(compiler.plan.Scenes, compiled)
		if id == spec.Entry {
			compiler.plan.Entry = uint32(index)
		}
	}
	if cache != nil {
		for id := range cache.mods {
			if _, loaded := compiler.mods[id]; !loaded {
				delete(cache.mods, id)
			}
		}
	}
	return &compiler.plan, nil
}

func CompileProjectDirectory(directory string) (*ProjectPlan, error) {
	return CompileProject(os.DirFS(directory), ".")
}

func ValidateProjectSpec(spec ProjectSpec) error {
	if err := spec.Engine.Validate(); err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	seen := make(map[SceneID]struct{}, len(spec.Scenes))
	for index, id := range spec.Scenes {
		if err := validateSceneFile(id); err != nil {
			return fmt.Errorf("scenes[%d]: %w", index, err)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("scenes[%d]: duplicate scene %q", index, id)
		}
		seen[id] = struct{}{}
	}
	if err := validateSceneFile(spec.Entry); err != nil {
		return fmt.Errorf("entry: %w", err)
	}
	if _, exists := seen[spec.Entry]; !exists {
		return fmt.Errorf("entry scene %q is not registered in scenes", spec.Entry)
	}
	if spec.ClearColor != "" {
		if _, err := parseColorValue(spec.ClearColor, 1); err != nil {
			return fmt.Errorf("clearColor: %w", err)
		}
	}
	if spec.Window.Width < 0 || spec.Window.Height < 0 || spec.Viewport.Width < 0 || spec.Viewport.Height < 0 {
		return fmt.Errorf("window and viewport dimensions cannot be negative")
	}
	for _, name := range sortedModKeys(spec.Types) {
		if !token.IsIdentifier(string(name)) || isModValueType(name) || isModProtocolType(name) {
			return fmt.Errorf("types.%s: expected a non-reserved Go type name", name)
		}
		if _, err := compileSlotValue(name, nil, spec.Types, 0); err != nil {
			return fmt.Errorf("types.%s: %w", name, err)
		}
	}
	return nil
}

func validateSceneFile(id SceneID) error {
	name := string(id)
	if !fs.ValidPath(name) || strings.ContainsAny(name, `/\:`) || !strings.HasSuffix(name, ".json") || name == ModFileName {
		return fmt.Errorf("scene %q must be a JSON filename in %s", name, SceneDirectory)
	}
	return nil
}

// ScenePath resolves a scene ID to its authored file, relative to the project.
func ScenePath(id SceneID) string { return path.Join(SceneDirectory, string(id)) }

func discoverScenes(source fs.FS, ordered []SceneID) ([]SceneID, error) {
	entries, err := fs.ReadDir(source, SceneDirectory)
	if err != nil {
		return nil, fmt.Errorf("discover scenes: %w", err)
	}
	indices := make(map[SceneID]int, len(ordered))
	result := make([]SceneID, len(ordered), len(ordered)+len(entries))
	copy(result, ordered)
	for index, id := range ordered {
		if err := validateSceneFile(id); err != nil {
			return nil, err
		}
		if _, exists := indices[id]; exists {
			return nil, fmt.Errorf("duplicate scene %q", id)
		}
		indices[id] = index + 1
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := SceneID(entry.Name())
		if _, exists := indices[id]; exists {
			indices[id] = 0
		} else {
			result = append(result, id)
		}
	}
	for _, id := range ordered {
		if indices[id] != 0 {
			return nil, fmt.Errorf("scene file %q does not exist", ScenePath(id))
		}
	}
	return result, nil
}

func NormalizeModPath(id ModID) (ModID, error) {
	value := string(id)
	if !fs.ValidPath(value) || value == "." || strings.ContainsAny(value, `\:`) {
		return "", fmt.Errorf("module path %q must stay within the project", id)
	}
	return id, nil
}

// ResolveModResourcePath accepts arbitrary module-owned folder layouts.
// File references cannot escape a module and are never resolved at project scope.
func ResolveModResourcePath(id ModID, file string) (string, error) {
	if _, err := NormalizeModPath(id); err != nil {
		return "", err
	}
	if !fs.ValidPath(file) || file == "." || strings.ContainsAny(file, `\:`) {
		return "", fmt.Errorf("resource path %q must stay within module %q", file, id)
	}
	return path.Join(string(id), file), nil
}

func (compiler *modCompiler) loadMod(id ModID) (uint32, error) {
	if _, err := NormalizeModPath(id); err != nil {
		return 0, err
	}
	if index, exists := compiler.mods[id]; exists {
		return index, nil
	}
	file := path.Join(string(id), ModFileName)
	var digest [sha256.Size]byte
	if compiler.cache != nil {
		if cached, ok := compiler.cache.mods[id]; ok {
			var err error
			digest, err = compiler.modDigest(file)
			if err != nil {
				return 0, fmt.Errorf("%s: %w", file, err)
			}
			if cached.digest == digest {
				for _, name := range cached.files {
					info, err := fs.Stat(compiler.source, name)
					if err != nil {
						return 0, fmt.Errorf("%s: %s: %w", file, name, err)
					}
					if info.IsDir() {
						return 0, fmt.Errorf("%s: %s must be a file", file, name)
					}
				}
				return compiler.addMod(cached.plan, cached.index), nil
			}
		}
	}
	data, err := fs.ReadFile(compiler.source, file)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", file, err)
	}
	if compiler.cache != nil {
		// Hash exactly the bytes being compiled, including after a cache miss.
		digest = sha256.Sum256(data)
		compiler.files = make(map[string]struct{})
	}
	var spec ModSpec
	if err := decodeModJSON(data, &spec); err != nil {
		return 0, fmt.Errorf("%s: %w", file, err)
	}
	space := spec.Space
	if space == "" {
		space = CoordinateSpaceUI
	}
	if space != CoordinateSpaceUI && space != CoordinateSpaceWorld {
		return 0, fmt.Errorf("%s: space %q must be ui or world", file, space)
	}
	transform, err := compileModTransform(spec.Transform)
	if err != nil {
		return 0, fmt.Errorf("%s: transform: %w", file, err)
	}
	plan := ModPlan{ID: id, Spec: spec, Transform: transform}
	if len(spec.Elements) != 0 {
		plan.Elements = make([]ModElementPlan, 0, len(spec.Elements))
	}
	if len(spec.Slots) != 0 {
		plan.Slots = make([]SlotPlan, 0, len(spec.Slots))
	}
	if len(spec.Resources) != 0 {
		plan.Resources = make([]ModResourcePlan, 0, len(spec.Resources))
	}
	indices := modIndices{slots: make(map[string]uint32, len(spec.Slots)), models: map[string]ModelSpec{}, spines: map[string]SpineSpec{}}
	indices.physics = spec.Components.RigidBody2D != nil || spec.Components.RigidBody3D != nil
	animated := spec.Components.Animation != nil
	for _, name := range sortedModKeys(spec.Resources) {
		resource := spec.Resources[name]
		if !validModName(name) || !validModResourceKind(resource.Kind) {
			return 0, fmt.Errorf("%s: resources.%s: invalid resource name or kind %q", file, name, resource.Kind)
		}
		resolved, err := compiler.resourceFile(id, resource.Path)
		if err != nil {
			return 0, fmt.Errorf("%s: resources.%s.path: %w", file, name, err)
		}
		plan.Resources = append(plan.Resources, ModResourcePlan{Name: name, Kind: resource.Kind, Path: resolved})
	}
	for _, name := range sortedModKeys(spec.Slots) {
		slot := spec.Slots[name]
		if !validModName(name) {
			return 0, fmt.Errorf("%s: slots.%s: invalid slot name", file, name)
		}
		if slot.Type == "" {
			return 0, fmt.Errorf("%s: slots.%s.type: a value type is required", file, name)
		}
		value, err := compileSlotValue(slot.Type, slot.Default, compiler.plan.Spec.Types, 0)
		if err != nil {
			return 0, fmt.Errorf("%s: slots.%s.default: %w", file, name, err)
		}
		placement, err := compileModTransform(slot.Placement)
		if err != nil {
			return 0, fmt.Errorf("%s: slots.%s.placement: %w", file, name, err)
		}
		indices.slots[name] = uint32(len(plan.Slots))
		plan.Slots = append(plan.Slots, SlotPlan{Name: name, Type: slot.Type, Default: value, Writable: slot.Writable == nil || *slot.Writable, Placement: placement})
	}
	if spec.Behavior != "" {
		if !token.IsIdentifier(spec.Behavior) || !ast.IsExported(spec.Behavior) {
			return 0, fmt.Errorf("%s: behavior must name an exported Go type", file)
		}
		if _, err := compiler.resourceFile(id, "mod.go"); err != nil {
			return 0, fmt.Errorf("%s: behavior: %w", file, err)
		}
	}
	private := make(map[string]uint32, len(spec.Elements)+len(indices.slots))
	parents := make(map[string]int32, len(spec.Elements))
	for name, index := range indices.slots {
		private[name] = index
	}
	for i, element := range spec.Elements {
		indices.physics = indices.physics || element.Components.RigidBody2D != nil || element.Components.RigidBody3D != nil
		animated = animated || element.Components.Animation != nil
		if element.Space != "" && element.Space != CoordinateSpaceWorld && element.Space != CoordinateSpaceUI {
			return 0, fmt.Errorf("%s: element %s: invalid coordinate space", file, element.ID)
		}
		if !validModName(element.ID) {
			return 0, fmt.Errorf("%s: elements[%d]: invalid private id", file, i)
		}
		if _, exists := private[element.ID]; exists {
			return 0, fmt.Errorf("%s: duplicate private id %q", file, element.ID)
		}
		parent := int32(-1)
		if element.Parent != "" {
			var found bool
			parent, found = parents[element.Parent]
			if !found {
				return 0, fmt.Errorf("%s: element %s parent %q must precede its child", file, element.ID, element.Parent)
			}
		}
		transform, err := compileModTransform(element.Transform)
		if err != nil {
			return 0, fmt.Errorf("%s: element %s: %w", file, element.ID, err)
		}
		private[element.ID] = uint32(i)
		parents[element.ID] = int32(i)
		plan.Elements = append(plan.Elements, ModElementPlan{Spec: element, Parent: parent, Transform: transform})
	}
	// Resolve animation targets against the complete private namespace, including
	// forward targets. No scene instance or foreign module can be addressed.
	indices.slots = private
	if plan.Spec.Components.Shader != nil {
		return 0, fmt.Errorf("%s: shader must be attached to an element", file)
	}
	if err := compiler.validateContent(id, plan.Spec.Components, &plan, indices); err != nil {
		return 0, fmt.Errorf("%s: components: %w", file, err)
	}
	for _, element := range plan.Elements {
		if err := compiler.validateContent(id, element.Spec.Components, &plan, indices); err != nil {
			return 0, fmt.Errorf("%s: element %s: %w", file, element.Spec.ID, err)
		}
	}
	if animated {
		if err := validateModAnimationTargets(&plan); err != nil {
			return 0, fmt.Errorf("%s: %w", file, err)
		}
	}
	if indices.physics {
		if err := validateModPhysicsTransforms(&plan, mgl32.Ident4()); err != nil {
			return 0, fmt.Errorf("%s: %w", file, err)
		}
	}
	if compiler.cache != nil {
		compiler.cache.mods[id] = cachedModPlan{digest: digest, plan: plan, index: indices, files: sortedModKeys(compiler.files)}
		compiler.files = nil
	}
	return compiler.addMod(plan, indices), nil
}

func (compiler *modCompiler) modDigest(name string) (digest [sha256.Size]byte, err error) {
	file, err := compiler.source.Open(name)
	if err != nil {
		return digest, err
	}
	defer file.Close()
	if compiler.hasher == nil {
		compiler.hasher = sha256.New()
	}
	compiler.hasher.Reset()
	if writer, ok := file.(io.WriterTo); ok {
		_, err = writer.WriteTo(compiler.hasher)
	} else {
		if compiler.buffer == nil {
			compiler.buffer = make([]byte, 32*1024)
		}
		_, err = io.CopyBuffer(compiler.hasher, file, compiler.buffer)
	}
	compiler.hasher.Sum(digest[:0])
	return digest, err
}

func (compiler *modCompiler) addMod(plan ModPlan, indices modIndices) uint32 {
	index := uint32(len(compiler.plan.Mods))
	compiler.plan.Mods = append(compiler.plan.Mods, plan)
	compiler.index = append(compiler.index, indices)
	compiler.mods[plan.ID] = index
	return index
}

func (compiler *modCompiler) resourceFile(id ModID, file string) (string, error) {
	resolved, err := ResolveModResourcePath(id, file)
	if err != nil {
		return "", err
	}
	info, err := fs.Stat(compiler.source, resolved)
	if err != nil {
		return "", fmt.Errorf("%s: %w", resolved, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s must be a file", resolved)
	}
	if compiler.files != nil {
		compiler.files[resolved] = struct{}{}
	}
	return resolved, nil
}

func (compiler *modCompiler) validateContent(id ModID, components ModComponents, plan *ModPlan, indices modIndices) error {
	if components.RigidBody2D != nil && components.RigidBody3D != nil {
		return fmt.Errorf("an element cannot contain both 2D and 3D rigid bodies")
	}
	if components.Anchor != nil && !finiteModValues(components.Anchor[:]) {
		return fmt.Errorf("anchor must be finite")
	}
	if len(components.Size) != 0 && (len(components.Size) != 2 || !finiteModValues(components.Size) || components.Size[0] < 0 || components.Size[1] < 0) {
		return fmt.Errorf("size must contain two finite non-negative numbers")
	}
	if material := components.Material; material != nil {
		for _, row := range material.Custom {
			if !finiteModValues(row[:]) {
				return fmt.Errorf("material.custom must be finite")
			}
		}
	}
	if shader := components.Shader; shader != nil {
		if shader.SourcePaths() != ([2]string{}) {
			if err := shader.wgslSources().Validate(); err != nil {
				return err
			}
			for _, p := range shader.SourcePaths() {
				if p != "" {
					if _, err := compiler.resourceFile(id, p); err != nil {
						return err
					}
				}
			}
		}
		if components.Model != nil && components.Sprite != nil {
			return fmt.Errorf("shader cannot target both a model and a sprite on the same element")
		}
		for _, row := range shader.Custom {
			if !finiteModValues(row[:]) {
				return fmt.Errorf("shader.custom must be finite")
			}
		}
	}
	if trail := components.Trail; trail != nil {
		if err := trail.validate(); err != nil {
			return err
		}
	}
	if sprite := components.Sprite; sprite != nil {
		if len(sprite.Size) != 0 && (len(sprite.Size) != 2 || !finiteModValues(sprite.Size) || sprite.Size[0] < 0 || sprite.Size[1] < 0) {
			return fmt.Errorf("sprite.size must contain two finite non-negative numbers")
		}
		if sprite.Color != "" && sprite.Texture != nil {
			return fmt.Errorf("sprite cannot declare both color and texture")
		}
		if sprite.Color != "" {
			if _, err := parseColorValue(sprite.Color, 1); err != nil {
				return fmt.Errorf("sprite.color: %w", err)
			}
		}
		if err := compiler.validateTexture(id, sprite.Texture); err != nil {
			return fmt.Errorf("sprite.texture: %w", err)
		}
		if sprite.Alpha != nil && (!finiteModValues([]float32{*sprite.Alpha}) || *sprite.Alpha < 0 || *sprite.Alpha > 1) {
			return fmt.Errorf("sprite.alpha must be between 0 and 1")
		}
	}
	if html := components.HTML; html != nil {
		resource, exists := plan.Spec.Resources[html.Resource]
		if !exists || resource.Kind != "html" {
			return fmt.Errorf("html.resource %q must be declared by this module", html.Resource)
		}
		for _, name := range []string{html.Font, html.BoldFont} {
			if name == "" {
				continue
			}
			font, ok := plan.Spec.Resources[name]
			if !ok || font.Kind != "font" {
				return fmt.Errorf("html.font %q must be privately declared", html.Font)
			}
		}
	}
	if e := components.Emitter; e != nil {
		if !finiteModValues([]float32{e.Width, e.Height, e.Rate, e.Lifetime, e.SpeedMin, e.SpeedMax, e.SizeMin, e.SizeMax, e.Direction, e.Spread, e.GravityX, e.GravityY, e.AlphaStart, e.AlphaEnd}) || e.Width < 0 || e.Height < 0 || e.Count <= 0 || e.Rate < 0 || e.Lifetime <= 0 || e.SpeedMin < 0 || e.SpeedMax < e.SpeedMin || e.SizeMin < 0 || e.SizeMax < e.SizeMin || e.Spread < 0 || e.AlphaStart < 0 || e.AlphaStart > 1 || e.AlphaEnd < 0 || e.AlphaEnd > 1 {
			return fmt.Errorf("emitter requires finite values, positive count and lifetime, ordered speed/size ranges and alpha between 0 and 1")
		}
		if len(e.Particles) == 0 {
			return fmt.Errorf("emitter requires a particle element")
		}
		selected := make(map[string]struct{}, len(e.Particles))
		for _, name := range e.Particles {
			index, found := indices.slots[name]
			if !found || int(index) >= len(plan.Elements) || plan.Elements[index].Spec.ID != name {
				return fmt.Errorf("emitter references unknown particle element %q", name)
			}
			element := &plan.Elements[index].Spec
			if element.Components.Particle == nil {
				return fmt.Errorf("emitter particle element %q has no particle component", name)
			}
			if _, duplicate := selected[name]; duplicate {
				return fmt.Errorf("emitter repeats particle element %q", name)
			}
			selected[name] = struct{}{}
		}
	}
	if err := compiler.validateTexture(id, components.Particle); err != nil {
		return fmt.Errorf("particle: %w", err)
	}
	if t := components.Tilemap; t != nil {
		if t.Width <= 0 || t.Height <= 0 || t.TileWidth <= 0 || t.TileHeight <= 0 {
			return fmt.Errorf("tilemap dimensions must be positive")
		}
		if t.Tileset != "" {
			if _, err := compiler.resourceFile(id, t.Tileset); err != nil {
				return err
			}
		}
		if count, ok := tilemapCellCount(t.Width, t.Height); !ok || count != len(t.Tiles) {
			return fmt.Errorf("tilemap tiles must match dimensions")
		}
	}
	if m := components.Model; m != nil {
		if previous, ok := indices.models[m.ID]; ok && !equalModelSpec(previous, *m) {
			return fmt.Errorf("conflicting private model id %q", m.ID)
		}
		indices.models[m.ID] = *m
		if err := m.Validate(); err != nil {
			return err
		}
		for _, p := range []string{m.Path, m.AnimationPath, m.CollisionPath} {
			if p != "" {
				if _, err := compiler.resourceFile(id, p); err != nil {
					return err
				}
			}
		}
		if m.Material.Shader != "" {
			return fmt.Errorf("model.material.shader must be replaced by an element shader component")
		}
	}
	if s := components.Spine; s != nil {
		if previous, ok := indices.spines[s.Source.ID]; ok && previous != s.Source {
			return fmt.Errorf("conflicting private spine id %q", s.Source.ID)
		}
		indices.spines[s.Source.ID] = s.Source
		if err := s.Source.Validate(); err != nil {
			return err
		}
		for _, p := range []string{s.Source.Skeleton, s.Source.Atlas, s.Source.Texture} {
			if _, err := compiler.resourceFile(id, p); err != nil {
				return err
			}
		}
	}
	if components.Model != nil || components.Camera != nil || components.Camera2D != nil || components.AmbientLight != nil || components.DirectionalLight != nil || components.PointLight != nil || components.SpotLight != nil || components.AreaLight != nil || components.RigidBody2D != nil || components.RigidBody3D != nil {
		if effectiveModSpace(plan.Spec.Space) != CoordinateSpaceWorld {
			return fmt.Errorf("camera, lighting and physics require world space")
		}
	}
	if camera := components.Camera2D; camera != nil {
		if components.Camera != nil {
			return fmt.Errorf("an element cannot contain both 2D and 3D cameras")
		}
		if !finiteModValues([]float32{camera.Scale, camera.Viewport.X, camera.Viewport.Y, camera.Viewport.Width, camera.Viewport.Height}) || camera.Scale < 0 || camera.Viewport.Width < 0 || camera.Viewport.Height < 0 {
			return fmt.Errorf("camera2D requires finite non-negative scale and viewport")
		}
		if camera.ClearColor != "" {
			if _, err := ParseColor(camera.ClearColor, 1); err != nil {
				return err
			}
		}
	}
	if camera := components.Camera; camera != nil && camera.Environment != nil && camera.Environment.Path != "" {
		if _, err := compiler.resourceFile(id, camera.Environment.Path); err != nil {
			return err
		}
	}
	if camera := components.Camera; camera != nil {
		if !finiteModValues([]float32{camera.Fov, camera.Near, camera.Viewport.X, camera.Viewport.Y, camera.Viewport.Width, camera.Viewport.Height}) || camera.Fov <= 0 || camera.Fov >= math.Pi || camera.Near <= 0 || camera.Viewport.Width < 0 || camera.Viewport.Height < 0 {
			return fmt.Errorf("camera requires a finite field of view between 0 and pi, positive near plane and non-negative viewport")
		}
		if _, err := ParseColor(camera.ClearColor, 1); err != nil {
			return fmt.Errorf("camera.clearColor: %w", err)
		}
		if _, err := camera.renderSettings(); err != nil {
			return fmt.Errorf("camera: %w", err)
		}
	}
	if (components.CircleCollider2D != nil || components.BoxCollider2D != nil || components.SegmentCollider2D != nil) && components.RigidBody2D == nil {
		return fmt.Errorf("2D collider requires a rigid body on the same element")
	}
	if (components.TriangleMeshCollider3D != nil || components.ConvexHullCollider3D != nil || components.CompoundCollider3D != nil) && components.RigidBody3D == nil {
		return fmt.Errorf("3D collider requires a rigid body on the same element")
	}
	if err := validatePhysics2DComponents(string(id), "private content", SceneComponents{RigidBody2D: components.RigidBody2D, CircleCollider2D: components.CircleCollider2D, BoxCollider2D: components.BoxCollider2D, SegmentCollider2D: components.SegmentCollider2D}); err != nil {
		return err
	}
	if err := validatePhysics3DComponents(string(id), "private content", SceneComponents{RigidBody3D: components.RigidBody3D, TriangleMeshCollider3D: components.TriangleMeshCollider3D, ConvexHullCollider3D: components.ConvexHullCollider3D, CompoundCollider3D: components.CompoundCollider3D}); err != nil {
		return err
	}
	for _, p := range []string{func() string {
		if components.TriangleMeshCollider3D != nil {
			return components.TriangleMeshCollider3D.Model
		}
		return ""
	}(), func() string {
		if components.ConvexHullCollider3D != nil {
			return components.ConvexHullCollider3D.Model
		}
		return ""
	}(), func() string {
		if components.CompoundCollider3D != nil {
			return components.CompoundCollider3D.Model
		}
		return ""
	}()} {
		if p != "" {
			return fmt.Errorf("3D colliders use this element's model; external model references are forbidden")
		}
	}
	if (components.TriangleMeshCollider3D != nil || components.ConvexHullCollider3D != nil || components.CompoundCollider3D != nil) && components.Model == nil {
		return fmt.Errorf("3D collider requires a private model on the same element")
	}
	if collision := components.Collision; collision != nil {
		if collision.Emit == "" {
			return fmt.Errorf("collision.emit requires a message topic")
		}
		if (components.RigidBody2D == nil) == (components.RigidBody3D == nil) {
			return fmt.Errorf("collision requires exactly one rigid body dimension on the same element")
		}
	}
	if text := components.Text; text != nil && text.Font != "" {
		resource, exists := plan.Spec.Resources[text.Font]
		if !exists || resource.Kind != "font" {
			return fmt.Errorf("text.font %q must be a font declared by this module", text.Font)
		}
	}
	if animation := components.Animation; animation != nil {
		if err := animation.Clip.Validate(); err != nil {
			return fmt.Errorf("animation.clip: %w", err)
		}
		if err := validateModAnimationPaths(animation.Clip.FloatTracks, indices.slots); err != nil {
			return err
		}
		if err := validateModAnimationPaths(animation.Clip.BoolTracks, indices.slots); err != nil {
			return err
		}
		for index := range animation.Clip.TextureTracks {
			if err := validateModAnimationPath(animation.Clip.TextureTracks[index].Binding.Path, indices.slots); err != nil {
				return err
			}
			for frame, key := range animation.Clip.TextureTracks[index].Keyframes {
				texture := key.Value
				if err := compiler.validateTexture(id, &texture); err != nil {
					return fmt.Errorf("animation.clip.textureTracks[%d].keyframes[%d]: %w", index, frame, err)
				}
			}
		}
	}
	return nil
}

func validateModAnimationPaths[T any](tracks []AnimationTrackSpec[T], slots map[string]uint32) error {
	for _, track := range tracks {
		if err := validateModAnimationPath(track.Binding.Path, slots); err != nil {
			return err
		}
	}
	return nil
}

func validateModAnimationPath(binding string, slots map[string]uint32) error {
	if binding == "" {
		return nil
	}
	if _, exists := slots[binding]; !exists {
		return fmt.Errorf("animation binding %q must target this module or one of its declared slots", binding)
	}
	return nil
}

func (compiler *modCompiler) validateTexture(id ModID, texture *TextureSpec) error {
	if texture == nil {
		return nil
	}
	value := *texture
	if err := value.validate(true); err != nil {
		return err
	}
	for _, file := range [...]string{texture.Image, texture.Atlas} {
		if file != "" {
			if _, err := compiler.resourceFile(id, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func (compiler *modCompiler) compileScene(id SceneID, spec SceneSpec) (ScenePlan, error) {
	plan := ScenePlan{ID: id, Spec: spec}
	switch spec.ExitPolicy {
	case "", SceneExitDestroy, SceneExitRetain:
	default:
		return plan, fmt.Errorf("%s: exitPolicy: expected destroy or retain, got %q", id, spec.ExitPolicy)
	}
	var err error
	if spec.Behavior != "" {
		if !token.IsIdentifier(spec.Behavior) || !ast.IsExported(spec.Behavior) {
			return plan, fmt.Errorf("%s: behavior must name an exported Go type", id)
		}
		if _, err := fs.Stat(compiler.source, strings.TrimSuffix(string(id), ".json")+".go"); err != nil {
			return plan, fmt.Errorf("%s: behavior: %w", id, err)
		}
	}
	plan.Transform, err = compileModTransform(spec.Transform)
	if err != nil {
		return plan, fmt.Errorf("%s: transform: %w", id, err)
	}
	instances := make(map[string]int32)
	var appendInstances func([]ModInstanceSpec, int32, string, mgl32.Mat4) error
	appendInstances = func(values []ModInstanceSpec, parent int32, field string, parentMatrix mgl32.Mat4) error {
		for index := range values {
			instance := &values[index]
			location := fmt.Sprintf("%s[%d]", field, index)
			if !validModName(instance.ID) {
				return fmt.Errorf("%s.id: invalid instance id %q", location, instance.ID)
			}
			if _, exists := instances[instance.ID]; exists {
				return fmt.Errorf("%s.id: duplicate instance id %q", location, instance.ID)
			}
			mod, err := compiler.loadMod(instance.Mod)
			if err != nil {
				return fmt.Errorf("%s.mod: %w", location, err)
			}
			placement, err := compileModTransform(instance.Placement)
			if err != nil {
				return fmt.Errorf("%s.placement: %w", location, err)
			}
			definition := &compiler.plan.Mods[mod]
			if parent >= 0 && effectiveModSpace(definition.Spec.Space) != effectiveModSpace(compiler.plan.Mods[plan.Instances[parent].Mod].Spec.Space) {
				return fmt.Errorf("%s.mod: parent and child coordinate spaces differ", location)
			}
			slots := make([]SlotValue, len(definition.Slots))
			for name, data := range instance.Slots {
				slotIndex, exists := compiler.index[mod].slots[name]
				if !exists {
					return fmt.Errorf("%s.slots.%s: slot is not declared by module %q", location, name, instance.Mod)
				}
				value, err := compileSlotValue(definition.Slots[slotIndex].Type, data, compiler.plan.Spec.Types, 0)
				if err != nil {
					return fmt.Errorf("%s.slots.%s: %w", location, name, err)
				}
				slots[slotIndex] = value
			}
			for slotIndex, slot := range definition.Slots {
				if slots[slotIndex].Type == "" {
					slots[slotIndex] = cloneSlotValue(slot.Default)
				}
			}
			matrix := parentMatrix.Mul4(modWorldMatrix(placement))
			if compiler.index[mod].physics {
				if err := validateModPhysicsTransforms(definition, matrix); err != nil {
					return fmt.Errorf("%s: instance %s: %w", id, instance.ID, err)
				}
			}
			current := int32(len(plan.Instances))
			instances[instance.ID] = current
			plan.Instances = append(plan.Instances, ModInstancePlan{ID: instance.ID, Mod: mod, Parent: parent, Placement: placement, Slots: slots, Enabled: instance.Enabled == nil || *instance.Enabled, Visible: instance.Visible == nil || *instance.Visible})
			if err := appendInstances(instance.Children, current, location+".children", matrix); err != nil {
				return err
			}
		}
		return nil
	}
	if err := appendInstances(spec.Modules, -1, string(id)+": modules", modWorldMatrix(plan.Transform)); err != nil {
		return plan, err
	}
	return plan, nil
}

func compileModTransform(spec ModTransformSpec) (ModTransform, error) {
	result := ModTransform{Scale: [3]float32{1, 1, 1}}
	for _, field := range []struct {
		name string
		data []float32
		dst  *[3]float32
	}{{"position", spec.Position, &result.Position}, {"scale", spec.Scale, &result.Scale}} {
		if len(field.data) != 0 && len(field.data) != 2 && len(field.data) != 3 || !finiteModValues(field.data) {
			return result, fmt.Errorf("%s must contain two or three finite numbers", field.name)
		}
		copy(field.dst[:], field.data)
	}
	if len(spec.Rotation) != 0 && len(spec.Rotation) != 1 && len(spec.Rotation) != 3 || !finiteModValues(spec.Rotation) {
		return result, fmt.Errorf("rotation must contain one or three finite numbers (radians)")
	}
	if len(spec.Rotation) == 1 {
		result.Rotation[2] = spec.Rotation[0]
	} else {
		copy(result.Rotation[:], spec.Rotation)
	}
	return result, nil
}

func finiteModValues(values []float32) bool {
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

func effectiveModSpace(space CoordinateSpace) CoordinateSpace {
	if space == "" {
		return CoordinateSpaceUI
	}
	return space
}

func validModName(name string) bool {
	return name != "" && name == strings.TrimSpace(name) && !strings.ContainsAny(name, "/\\:.")
}

func validModResourceKind(kind string) bool {
	switch kind {
	case "image", "atlas", "font", "audio", "video", "animation", "model", "spine", "shader", "html", "tilemap", "data":
		return true
	}
	return false
}

func sortedModKeys[K ~string, V any](values map[K]V) []K {
	result := make([]K, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}

func readModJSON[T any](source fs.FS, name string) (T, error) {
	var result T
	data, err := fs.ReadFile(source, name)
	if err != nil {
		return result, fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeModJSON(data, &result); err != nil {
		return result, fmt.Errorf("%s: %w", name, err)
	}
	return result, nil
}

func decodeModJSON(data []byte, result any) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("null is not a valid configuration value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
