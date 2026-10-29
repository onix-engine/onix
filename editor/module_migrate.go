package editor

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/diiyw/onix/project"
)

type ModuleMigrationIssue struct {
	File    string `json:"file"`
	Element string `json:"element,omitempty"`
	Reason  string `json:"reason"`
}

type ModuleMigrationReport struct {
	Source string                 `json:"source"`
	Output string                 `json:"output"`
	Ready  bool                   `json:"ready"`
	Issues []ModuleMigrationIssue `json:"issues,omitempty"`
}

// These source-only readers belong to the one-time migration tool. Neither the
// module runtime nor the authoring compiler accepts the old scene format.
type migrationResources struct {
	Fonts, Audio, Videos, HTML []project.FileResource
	Effects                    []project.ParticleSpec
	Tilemaps                   []project.TilemapSpec
	Models                     []project.ModelSpec
	Spines                     []project.SpineSpec
	Shaders                    []project.ShaderSpec
	Prefabs                    []project.PrefabResourceSpec
}

type moduleMigration struct {
	source, output             string
	report                     ModuleMigrationReport
	resources                  migrationResources
	assets                     map[string][]byte
	fonts, audio, videos, html map[string]project.FileResource
	particles                  map[string]project.ParticleSpec
	tilemaps                   map[string]project.TilemapSpec
	models                     map[string]project.ModelSpec
	spines                     map[string]project.SpineSpec
	shaders                    map[string]project.ShaderSpec
}

func migrationIndex[T any](entries []T, id func(T) string) map[string]T {
	result := make(map[string]T, len(entries))
	for _, entry := range entries {
		result[id(entry)] = entry
	}
	return result
}

// MigrateModuleProject writes a reviewable draft to a new, separate directory.
// Scripts, controls and external entity bindings become explicit report items;
// they are never silently retained as runtime compatibility code.
func MigrateModuleProject(source, output string) (ModuleMigrationReport, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return ModuleMigrationReport{}, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return ModuleMigrationReport{}, err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return ModuleMigrationReport{}, err
	}
	output, err = migrationResolvedOutput(output)
	if err != nil {
		return ModuleMigrationReport{}, err
	}
	if rel, err := filepath.Rel(source, output); err != nil || pathWithinRoot(rel) {
		return ModuleMigrationReport{}, fmt.Errorf("migration output must be separate from the source project")
	}
	if rel, err := filepath.Rel(output, source); err == nil && pathWithinRoot(rel) {
		return ModuleMigrationReport{}, fmt.Errorf("migration output cannot contain the source project")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		return ModuleMigrationReport{}, fmt.Errorf("migration output must not already exist: %s", output)
	}
	m := moduleMigration{source: source, output: output, report: ModuleMigrationReport{Source: source, Output: output}, assets: map[string][]byte{}}
	spec, err := migrationRead[project.ProjectSpec](source, "resources/project.json")
	if err != nil {
		return m.report, err
	}
	catalog, err := migrationRead[struct {
		Entry  project.SceneID
		Scenes []project.SceneCatalogEntry
	}](source, "resources/scenes.json")
	if err != nil {
		return m.report, err
	}
	seenScenes := make(map[string]bool, len(catalog.Scenes))
	seenIDs := make(map[project.SceneID]bool, len(catalog.Scenes))
	entryFound := false
	for _, entry := range catalog.Scenes {
		if entry.ID == "" || seenIDs[entry.ID] {
			return m.report, fmt.Errorf("invalid or duplicate legacy scene id %q", entry.ID)
		}
		seenIDs[entry.ID] = true
		if !fs.ValidPath(entry.Path) || strings.ContainsAny(entry.Path, `\:`) || !strings.HasSuffix(entry.Path, ".scene.json") {
			return m.report, fmt.Errorf("invalid legacy scene path %q", entry.Path)
		}
		name := strings.TrimSuffix(path.Base(entry.Path), ".scene.json") + ".json"
		if seenScenes[strings.ToLower(name)] {
			return m.report, fmt.Errorf("legacy scenes produce duplicate scene file %q", name)
		}
		seenScenes[strings.ToLower(name)] = true
		entryFound = entryFound || entry.ID == catalog.Entry
	}
	if !entryFound {
		return m.report, fmt.Errorf("legacy entry scene is not registered")
	}
	m.resources, err = migrationRead[migrationResources](source, "resources/resources.json")
	if err != nil && !os.IsNotExist(err) {
		return m.report, err
	}
	fileID := func(v project.FileResource) string { return v.ID }
	m.fonts = migrationIndex(m.resources.Fonts, fileID)
	m.audio = migrationIndex(m.resources.Audio, fileID)
	m.videos = migrationIndex(m.resources.Videos, fileID)
	m.html = migrationIndex(m.resources.HTML, fileID)
	m.particles = migrationIndex(m.resources.Effects, func(v project.ParticleSpec) string { return v.ID })
	m.tilemaps = migrationIndex(m.resources.Tilemaps, func(v project.TilemapSpec) string { return v.ID })
	m.models = migrationIndex(m.resources.Models, func(v project.ModelSpec) string { return v.ID })
	m.spines = migrationIndex(m.resources.Spines, func(v project.SpineSpec) string { return v.ID })
	m.shaders = make(map[string]project.ShaderSpec, len(m.resources.Shaders))
	for _, v := range m.resources.Shaders {
		v.WGSLVertex = m.privateFile(v.WGSLVertex)
		v.WGSLFragment = m.privateFile(v.WGSLFragment)
		m.shaders[v.ID] = v
	}
	for _, prefab := range m.resources.Prefabs {
		m.issue(prefab.Path, "", "prefab business boundary must be split into independent modules")
	}
	err = filepath.WalkDir(filepath.Join(source, "resources"), func(file string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if e.Name() == "scenes" {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("migration asset %s must be a regular file", file)
		}
		name := e.Name()
		if name == "project.json" || name == "resources.json" || name == "scenes.json" || strings.HasSuffix(name, ".prefab.json") {
			return nil
		}
		rel, err := filepath.Rel(filepath.Join(source, "resources"), file)
		if err != nil {
			return err
		}
		data, err := migrationBytes(source, path.Join("resources", filepath.ToSlash(rel)))
		if err != nil {
			return err
		}
		m.assets[path.Join("assets", filepath.ToSlash(rel))] = data
		return nil
	})
	if err != nil {
		return m.report, err
	}
	if err = m.inspectCode(); err != nil {
		return m.report, err
	}
	width, height := spec.Window.Width, spec.Window.Height
	if width <= 0 || height <= 0 {
		width, height = spec.Viewport.Width, spec.Viewport.Height
	}
	if width <= 0 || height <= 0 {
		width, height = 750, 1624
	}
	if _, err := CreateModuleProject(output, project.CreateProjectOptions{Width: width, Height: height, Title: spec.Window.Title}); err != nil {
		return m.report, err
	}
	if err := os.Remove(filepath.Join(output, filepath.FromSlash(project.ScenePath(project.DefaultSceneFile)))); err != nil {
		return m.report, err
	}
	if spec.Window.Icon != "" {
		data, err := migrationBytes(source, "resources/"+spec.Window.Icon)
		if err != nil {
			return m.report, err
		}
		if err := migrationWrite(output, path.Join("resources", spec.Window.Icon), data); err != nil {
			return m.report, err
		}
	}
	spec.Entry = ""
	spec.Scenes = nil
	for _, entry := range catalog.Scenes {
		old, err := migrationRead[struct{ Nodes []project.SceneNodeSpec }](source, path.Join("resources", entry.Path))
		if err != nil {
			return m.report, err
		}
		name := strings.TrimSuffix(path.Base(entry.Path), ".scene.json") + ".json"
		if entry.ID == catalog.Entry {
			spec.Entry = project.SceneID(name)
		}
		spec.Scenes = append(spec.Scenes, project.SceneID(name))
		scene := project.SceneSpec{Modules: []project.ModInstanceSpec{}}
		spaces := map[project.CoordinateSpace]*project.ModSpec{}
		var visit func([]project.SceneNodeSpec, string, project.CoordinateSpace) error
		visit = func(nodes []project.SceneNodeSpec, parent string, inherited project.CoordinateSpace) error {
			nodes = slices.Clone(nodes)
			slices.SortStableFunc(nodes, func(a, b project.SceneNodeSpec) int {
				if a.Order < b.Order {
					return -1
				}
				if a.Order > b.Order {
					return 1
				}
				return 0
			})
			for _, node := range nodes {
				space := node.CoordinateSpace
				if space == "" {
					space = project.CoordinateSpaceWorld
				}
				mod := spaces[space]
				if mod == nil {
					mod = &project.ModSpec{Name: strings.TrimSuffix(name, ".json") + " presentation", Space: space, Resources: map[string]project.ModResourceSpec{}}
					spaces[space] = mod
				}
				effectiveParent := parent
				if parent != "" && space != inherited {
					m.issue(entry.Path, node.ID, "coordinate-space boundary needs an explicit module placement")
					effectiveParent = ""
				}
				for _, script := range node.Scripts {
					m.issue(entry.Path, node.ID, "rewrite script "+script.Name+" as mod.go behavior with private handles and OnMessage")
				}
				if node.Control != nil || node.Widget != nil || node.Layout != nil {
					m.issue(entry.Path, node.ID, "rewrite control/layout/widget composition inside its owning module")
				}
				if node.TargetCamera != "" {
					m.issue(entry.Path, node.ID, "replace external camera binding with an engine service event")
				}
				if node.Disabled || node.Visible != nil && !*node.Visible {
					m.issue(entry.Path, node.ID, "review authored disabled/hidden private content")
				}
				components, err := m.content(node, mod)
				if err != nil {
					return err
				}
				rotation := []float32{node.RotationX, node.RotationY, node.RotationZ}
				if node.Rotation != 0 {
					rotation[2] = node.Rotation
				}
				scale := []float32{node.ScaleX, node.ScaleY, node.ScaleZ}
				for i := range scale {
					if scale[i] == 0 {
						scale[i] = 1
					}
				}
				mod.Elements = append(mod.Elements, project.ModElementSpec{ID: node.ID, Parent: effectiveParent, Transform: project.ModTransformSpec{Position: []float32{node.X, node.Y, node.Z}, Rotation: rotation, Scale: scale}, Components: components})
				if err := visit(node.Children, node.ID, space); err != nil {
					return err
				}
			}
			return nil
		}
		if err := visit(old.Nodes, "", project.CoordinateSpaceWorld); err != nil {
			return m.report, err
		}
		for _, space := range []project.CoordinateSpace{project.CoordinateSpaceWorld, project.CoordinateSpaceUI} {
			mod := spaces[space]
			if mod == nil {
				continue
			}
			id := path.Join(strings.TrimSuffix(name, ".json"), string(space))
			for _, key := range sortedWorkspaceKeys(m.assets) {
				data := m.assets[key]
				if err := migrationWrite(output, path.Join(id, key), data); err != nil {
					return m.report, err
				}
				resourceName := fmt.Sprintf("asset-%d", len(mod.Resources))
				mod.Resources[resourceName] = project.ModResourceSpec{Kind: "data", Path: key}
			}
			if err := migrationJSON(output, path.Join(id, "mod.json"), mod); err != nil {
				return m.report, err
			}
			scene.Modules = append(scene.Modules, project.ModInstanceSpec{ID: string(space), Mod: project.ModID(id)})
		}
		if err := migrationJSON(output, project.ScenePath(project.SceneID(name)), scene); err != nil {
			return m.report, err
		}
	}
	if spec.Entry == "" {
		return m.report, fmt.Errorf("legacy entry scene is not registered")
	}
	if err := migrationJSON(output, "resources/project.json", spec); err != nil {
		return m.report, err
	}
	if _, err := GenerateModuleProject(output); err != nil {
		m.issue("resources/project.json", "", "draft validation: "+err.Error())
	}
	m.report.Ready = len(m.report.Issues) == 0
	if err := migrationJSON(output, "resources/migration-report.json", m.report); err != nil {
		return m.report, err
	}
	return m.report, nil
}

func migrationResolvedOutput(file string) (string, error) {
	resolved, err := filepath.EvalSymlinks(file)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) || filepath.Dir(file) == file {
		return "", err
	}
	parent, err := migrationResolvedOutput(filepath.Dir(file))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(file)), nil
}

func migrationRead[T any](directory, name string) (T, error) {
	var value T
	data, err := migrationBytes(directory, name)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}
func migrationBytes(directory, name string) ([]byte, error) {
	if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
		return nil, fmt.Errorf("migration path %q must stay within its root", name)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(filepath.FromSlash(name))
}
func migrationWrite(directory, name string, data []byte) error {
	if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
		return fmt.Errorf("migration path %q must stay within its root", name)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	file := filepath.FromSlash(name)
	if err := root.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	return root.WriteFile(file, data, 0644)
}
func migrationJSON(directory, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return migrationWrite(directory, name, append(data, '\n'))
}
func (m *moduleMigration) issue(file, element, reason string) {
	m.report.Issues = append(m.report.Issues, ModuleMigrationIssue{File: file, Element: element, Reason: reason})
}

func (m *moduleMigration) inspectCode() error {
	return filepath.WalkDir(m.source, func(file string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if e.Name() == "dist" || e.Name() == "resources" || strings.HasPrefix(e.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(m.source, file)
		rel = filepath.ToSlash(rel)
		if rel != "main.go" {
			m.issue(rel, "", "rewrite project code within module ownership; do not copy scene entity access")
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			return err
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				if fn.Name.Name != "main" {
					m.issue(rel, "", "rewrite custom entry function "+fn.Name.Name+" into module behavior")
				}
				if fn.Name.Name == "main" {
					ast.Inspect(fn.Body, func(n ast.Node) bool {
						switch statement := n.(type) {
						case *ast.IfStmt:
							guard, ok := statement.Cond.(*ast.BinaryExpr)
							standard := false
							if ok && guard.Op == token.NEQ {
								left, lok := guard.X.(*ast.Ident)
								right, rok := guard.Y.(*ast.Ident)
								standard = lok && rok && left.Name == "err" && right.Name == "nil"
							}
							if !standard {
								m.issue(rel, "", "review entry conditional behavior before replacing main.go")
							}
						case *ast.AssignStmt:
							for _, rhs := range statement.Rhs {
								if _, ok := rhs.(*ast.CallExpr); !ok {
									m.issue(rel, "", "review entry assignments before replacing main.go")
									break
								}
							}
							for _, lhs := range statement.Lhs {
								if _, ok := lhs.(*ast.Ident); !ok {
									m.issue(rel, "", "review entry configuration writes before replacing main.go")
									break
								}
							}
						case *ast.ForStmt, *ast.RangeStmt, *ast.DeferStmt, *ast.GoStmt, *ast.IncDecStmt, *ast.DeclStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.BranchStmt, *ast.ReturnStmt:
							m.issue(rel, "", "review entry control flow and state before replacing main.go")
						}
						call, ok := n.(*ast.CallExpr)
						if !ok {
							return true
						}
						fun := call.Fun
						if indexed, ok := fun.(*ast.IndexExpr); ok {
							fun = indexed.X
						}
						switch fun := fun.(type) {
						case *ast.Ident:
							if fun.Name == "panic" || fun.Name == "InstallOnixScripts" || fun.Name == "LoadOnixProject" {
								return true
							}
						case *ast.SelectorExpr:
							if object, ok := fun.X.(*ast.Ident); ok {
								name := object.Name + "." + fun.Sel.Name
								switch name {
								case "onix.NewApp", "app.InstallSystems", "app.LoadScene", "app.Run", "project.MustLoadDefault", "project.MustDefault":
									return true
								}
							}
						}
						m.issue(rel, "", "review custom entry call before replacing main.go")
						return true
					})
				}
				continue
			}
			if g, ok := d.(*ast.GenDecl); ok && g.Tok != token.IMPORT {
				m.issue(rel, "", "review entry state and declarations as module-private state")
			}
		}
		return nil
	})
}

func (m *moduleMigration) privateFile(file string) string {
	if file == "" {
		return ""
	}
	if strings.HasPrefix(file, "resources/") {
		return path.Join("assets", strings.TrimPrefix(file, "resources/"))
	}
	m.issue(file, "", "resource outside the authored resources tree needs explicit relocation")
	return file
}

func (m *moduleMigration) content(node project.SceneNodeSpec, mod *project.ModSpec) (project.ModComponents, error) {
	old := node.Components
	anchor := node.Anchor
	c := project.ModComponents{Size: []float32{float32(node.Width), float32(node.Height)}, Anchor: &anchor, Text: old.Text, Animation: old.Animation, Camera: old.Camera, AmbientLight: old.AmbientLight, DirectionalLight: old.DirectionalLight, PointLight: old.PointLight, SpotLight: old.SpotLight, AreaLight: old.AreaLight, RigidBody2D: old.RigidBody2D, CircleCollider2D: old.CircleCollider2D, BoxCollider2D: old.BoxCollider2D, SegmentCollider2D: old.SegmentCollider2D, RigidBody3D: old.RigidBody3D, TriangleMeshCollider3D: old.TriangleMeshCollider3D, ConvexHullCollider3D: old.ConvexHullCollider3D, CompoundCollider3D: old.CompoundCollider3D}
	texture := func(t *project.TextureSpec) {
		if t != nil {
			t.Image = m.privateFile(t.Image)
			t.Atlas = m.privateFile(t.Atlas)
		}
	}
	if s := old.Sprite; s != nil {
		texture(s.Texture)
		alpha := node.Alpha
		c.Sprite = &project.ModSpriteSpec{Texture: s.Texture, Size: c.Size, Alpha: &alpha}
		c.Trail = s.Trail
	}
	if a := c.Animation; a != nil {
		for i := range a.Clip.TextureTracks {
			for j := range a.Clip.TextureTracks[i].Keyframes {
				texture(&a.Clip.TextureTracks[i].Keyframes[j].Value)
			}
		}
	}
	if camera := c.Camera; camera != nil && camera.Environment != nil {
		camera.Environment.Path = m.privateFile(camera.Environment.Path)
	}
	files := func(entries map[string]project.FileResource, kind, id string) error {
		if id == "" {
			return nil
		}
		if _, exists := mod.Resources[id]; exists {
			return nil
		}
		if entry, ok := entries[id]; ok {
			root, file := m.source, entry.Path
			if kind == "font" && strings.HasPrefix(file, "../fonts/") {
				root, file = filepath.Dir(root), strings.TrimPrefix(file, "../")
			}
			data, err := migrationBytes(root, file)
			if err != nil {
				return err
			}
			name := path.Join("assets", kind+"-"+id+path.Ext(entry.Path))
			m.assets[name] = data
			mod.Resources[id] = project.ModResourceSpec{Kind: kind, Path: name}
			if kind == "font" {
				license := path.Join(path.Dir(path.Dir(file)), "OFL.txt")
				if data, err := migrationBytes(root, license); err == nil {
					m.assets[path.Join("assets", kind+"-"+id+"-OFL.txt")] = data
				}
			}
			return nil
		}
		return fmt.Errorf("missing private %s resource %q", kind, id)
	}
	if c.Text != nil {
		if err := files(m.fonts, "font", c.Text.Font); err != nil {
			return c, err
		}
	}
	if old.HTML != nil {
		if err := files(m.html, "html", old.HTML.Resource); err != nil {
			return c, err
		}
		c.HTML = &project.ModHTMLSpec{Resource: old.HTML.Resource, PassThrough: node.PassThrough}
	}
	if old.Particle != nil {
		if p, ok := m.particles[old.Particle.Resource]; ok {
			texture(p.Texture)
			appearance := p.Texture
			if appearance == nil {
				color := "#ffffff"
				if len(p.Colors) > 0 {
					color = p.Colors[0]
				}
				appearance = &project.TextureSpec{Color: color}
			}
			c.Particle = appearance
			c.Emitter = &project.EmitterSpec{
				Particles: []string{node.ID}, Width: p.Width, Height: p.Height,
				Count: p.Count, Rate: p.Rate, Lifetime: p.Lifetime,
				SpeedMin: p.SpeedMin, SpeedMax: p.SpeedMax, SizeMin: p.SizeMin, SizeMax: p.SizeMax,
				Direction: p.Direction, Spread: p.Spread, GravityX: p.GravityX, GravityY: p.GravityY,
				AlphaStart: p.AlphaStart, AlphaEnd: p.AlphaEnd, Loop: p.Loop,
			}
		}
		if c.Emitter == nil {
			return c, fmt.Errorf("missing particle %q", old.Particle.Resource)
		}
	}
	if old.Tilemap != nil {
		if t, ok := m.tilemaps[old.Tilemap.Resource]; ok {
			t.Tileset = m.privateFile(t.Tileset)
			t.Normalize()
			c.Tilemap = &t
		}
		if c.Tilemap == nil {
			return c, fmt.Errorf("missing tilemap %q", old.Tilemap.Resource)
		}
	}
	if old.Model != nil {
		if v, ok := m.models[old.Model.Resource]; ok {
			v.Path = m.privateFile(v.Path)
			v.AnimationPath = m.privateFile(v.AnimationPath)
			v.CollisionPath = m.privateFile(v.CollisionPath)
			if name := v.Material.Shader; name != "" {
				shader, ok := m.shaders[name]
				if !ok {
					return c, fmt.Errorf("missing shader %q", name)
				}
				if shader.WGSLVertex == "" && shader.WGSLFragment == "" {
					return c, fmt.Errorf("shader %q requires WGSL sources for migration", name)
				}
				c.Shader = &project.ShaderComponentSpec{WGSLVertex: shader.WGSLVertex, WGSLFragment: shader.WGSLFragment}
				v.Material.Shader = ""
			}
			c.Model = &v
		}
		if c.Model == nil {
			return c, fmt.Errorf("missing model %q", old.Model.Resource)
		}
	}
	if old.Spine != nil {
		if v, ok := m.spines[old.Spine.Resource]; ok {
			v.Skeleton = m.privateFile(v.Skeleton)
			v.Atlas = m.privateFile(v.Atlas)
			v.Texture = m.privateFile(v.Texture)
			c.Spine = &project.ModSpineSpec{Source: v, Animation: old.Spine.Animation, Skin: old.Spine.Skin, Loop: old.Spine.Loop, Speed: old.Spine.Speed}
		}
		if c.Spine == nil {
			return c, fmt.Errorf("missing spine %q", old.Spine.Resource)
		}
	}
	return c, nil
}
