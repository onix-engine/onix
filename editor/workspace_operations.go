package editor

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/diiyw/onix/project"
)

func (w *WorkspaceDocument) CreateScene(id project.SceneID) error {
	if !fs.ValidPath(string(id)) || strings.ContainsAny(string(id), `/\:`) || !strings.HasSuffix(string(id), ".json") || id == project.ModFileName {
		return fmt.Errorf("scene must be a JSON filename in %s", project.SceneDirectory)
	}
	if w.Scenes[id] != nil {
		return fmt.Errorf("scene %q already exists", id)
	}
	if _, err := fs.Stat(workspaceFS{w: w}, project.ScenePath(id)); err == nil {
		return fmt.Errorf("file %q already exists", id)
	}
	spec := w.Spec
	spec.Scenes = append(slices.Clone(spec.Scenes), id)
	settings, err := documentJSON(spec)
	if err != nil {
		return err
	}
	scene, err := documentJSON(project.SceneSpec{Modules: []project.ModInstanceSpec{}})
	if err != nil {
		return err
	}
	return w.commit(workspaceCommand{Label: "Create scene", Edits: []documentEdit{
		{Target: "resources/project.json", After: settings}, {Target: sceneDocumentID(id), After: scene},
	}})
}

func validateNewModID(id project.ModID) error {
	if _, err := project.NormalizeModPath(id); err != nil {
		return err
	}
	if strings.Contains(string(id), "/") || strings.HasPrefix(string(id), ".") || id == "resources" {
		return fmt.Errorf("new module must name a project-root folder other than resources")
	}
	return nil
}

func (w *WorkspaceDocument) CreateMod(id project.ModID, spec project.ModSpec) error {
	if err := validateNewModID(id); err != nil {
		return err
	}
	if mod := w.Mods[id]; mod != nil && !mod.Deleted {
		return fmt.Errorf("module %q already exists", id)
	}
	if _, err := fs.Stat(workspaceFS{w: w}, string(id)); err == nil {
		return fmt.Errorf("folder %q already exists", id)
	}
	return w.editValue(modDocumentID(id), "Create module definition", spec)
}

// CreateSceneModule creates a definition and its placement as one undoable edit.
// Existing definitions are reused through the usual instance wiring rules.
func (w *WorkspaceDocument) CreateSceneModule(id string, mod project.ModID, parent string) error {
	if definition := w.Mods[mod]; definition != nil && !definition.Deleted {
		return w.AddInstance(id, mod, parent)
	}
	if err := validateNewModID(mod); err != nil {
		return err
	}
	if _, err := fs.Stat(workspaceFS{w: w}, string(mod)); err == nil {
		return fmt.Errorf("folder %q already exists", mod)
	}
	spec, err := decodeDocument[project.SceneSpec](w.states[sceneDocumentID(w.Active)].data)
	if err != nil {
		return err
	}
	instance := project.ModInstanceSpec{ID: id, Mod: mod}
	definition := project.ModSpec{Name: string(mod)}
	if parent == "" {
		spec.Modules = append(spec.Modules, instance)
	} else if owner := findModInstance(spec.Modules, parent); owner != nil {
		definition.Space = w.Mods[owner.Mod].Spec.Space
		owner.Children = append(owner.Children, instance)
	} else {
		return fmt.Errorf("unknown parent %q", parent)
	}
	sceneData, err := documentJSON(spec)
	if err != nil {
		return err
	}
	modData, err := documentJSON(definition)
	if err != nil {
		return err
	}
	return w.commit(workspaceCommand{Label: "Create scene module", Edits: []documentEdit{
		{Target: modDocumentID(mod), After: modData},
		{Target: sceneDocumentID(w.Active), After: sceneData},
	}})
}

// CopyMod clones the complete private directory layout, including mod.go.
// It never rewrites main.go; registration of a new behavior is a build-stage task.
func (w *WorkspaceDocument) CopyMod(source, target project.ModID) error {
	mod := w.Mods[source]
	if mod == nil || mod.Deleted {
		return fmt.Errorf("unknown module %q", source)
	}
	if err := validateNewModID(target); err != nil {
		return err
	}
	if other := w.Mods[target]; other != nil && !other.Deleted {
		return fmt.Errorf("module %q already exists", target)
	}
	if _, err := fs.Stat(workspaceFS{w: w}, string(target)); err == nil {
		return fmt.Errorf("folder %q already exists", target)
	}
	snapshot, err := w.newAssetSnapshot()
	if err != nil {
		return err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(snapshot)
		}
	}()
	pending := make(map[string]string)
	dirs := make(map[string]bool)
	for _, file := range w.ModuleFiles(source) {
		if file.IsDir {
			dirs[path.Join(string(target), strings.TrimPrefix(file.Path, string(source)+"/"))] = true
			continue
		}
		if file.Path == string(modDocumentID(source)) {
			continue
		}
		name := strings.TrimPrefix(file.Path, string(source)+"/")
		stored := filepath.Join(snapshot, filepath.FromSlash(name))
		if err := copyWorkspaceAsset(workspaceFS{w: w}, file.Path, stored); err != nil {
			return err
		}
		pending[path.Join(string(target), name)] = stored
	}
	for name, data := range pending {
		w.assets[name] = data
	}
	for dir := range dirs {
		w.assetDirs[dir] = true
	}
	spec := mod.Spec
	spec.Name = string(target)
	if err := w.editValue(modDocumentID(target), "Copy as new module", spec); err != nil {
		for name := range pending {
			delete(w.assets, name)
		}
		for dir := range dirs {
			delete(w.assetDirs, dir)
		}
		return err
	}
	installed = true
	return nil
}

func (w *WorkspaceDocument) DeleteMod(id project.ModID) error {
	mod := w.Mods[id]
	if mod == nil || mod.Deleted {
		return fmt.Errorf("unknown module %q", id)
	}
	command := workspaceCommand{Label: "Delete module definition"}
	for _, owner := range sortedWorkspaceKeys(w.Mods) {
		if !w.Mods[owner].Deleted && (owner == id || strings.HasPrefix(string(owner), string(id)+"/")) {
			command.Edits = append(command.Edits, documentEdit{Target: modDocumentID(owner)})
		}
	}
	return w.commit(command)
}

// DeleteDocument removes one JSON document. Scenes also leave the project's
// scene list; files move to editor trash on save.
func (w *WorkspaceDocument) DeleteDocument(file string) error {
	id := DocumentID(file)
	state := w.states[id]
	if state == nil || state.data == nil {
		return fmt.Errorf("unknown document %q", file)
	}
	if dir, name := path.Split(string(id)); dir == project.SceneDirectory+"/" && w.Scenes[project.SceneID(name)] != nil {
		if name == string(w.Spec.Entry) || len(w.Spec.Scenes) <= 1 {
			return fmt.Errorf("the entry or last scene cannot be deleted")
		}
		spec := w.Spec
		spec.Scenes = slices.DeleteFunc(slices.Clone(spec.Scenes), func(value project.SceneID) bool { return value == project.SceneID(name) })
		settings, err := documentJSON(spec)
		if err != nil {
			return err
		}
		return w.commit(workspaceCommand{Label: "Delete scene", Edits: []documentEdit{
			{Target: "resources/project.json", After: settings}, {Target: id},
		}})
	}
	return w.commit(workspaceCommand{Label: "Delete document", Edits: []documentEdit{{Target: id}}})
}

// CreateFolder makes a plain directory inside the project and refreshes the
// cached inventory so the file browser shows it immediately.
func (w *WorkspaceDocument) CreateFolder(dir string) error {
	if _, err := fs.Stat(workspaceFS{w: w}, dir); err == nil {
		return fmt.Errorf("file %q already exists", dir)
	}
	if err := os.MkdirAll(w.Absolute(dir), 0o755); err != nil {
		return err
	}
	return w.RefreshFiles()
}

// undeclareResources drops declarations that pointed at a deleted path so the
// remaining definitions keep compiling. Unchanged commits are no-ops.
func (w *WorkspaceDocument) undeclareResources(file string) {
	for id, mod := range w.Mods {
		if mod.Deleted {
			continue
		}
		relative, ok := strings.CutPrefix(file, string(id)+"/")
		if !ok {
			continue
		}
		_ = w.EditMod(id, "Delete resource file", func(spec *project.ModSpec) error {
			for name, resource := range spec.Resources {
				if resource.Path == relative || strings.HasPrefix(resource.Path, relative+"/") {
					delete(spec.Resources, name)
				}
			}
			return nil
		})
	}
}

func findModInstance(instances []project.ModInstanceSpec, id string) *project.ModInstanceSpec {
	for i := range instances {
		if instances[i].ID == id {
			return &instances[i]
		}
		if found := findModInstance(instances[i].Children, id); found != nil {
			return found
		}
	}
	return nil
}

func (w *WorkspaceDocument) Instance(id string) *project.ModInstanceSpec {
	if scene := w.Scene(); scene != nil {
		return findModInstance(scene.Spec.Modules, id)
	}
	return nil
}

func (w *WorkspaceDocument) EditInstance(id, label string, edit func(*project.ModInstanceSpec) error) error {
	return w.EditScene(w.Active, label, func(spec *project.SceneSpec) error {
		instance := findModInstance(spec.Modules, id)
		if instance == nil {
			return fmt.Errorf("unknown instance %q", id)
		}
		return edit(instance)
	})
}

func (w *WorkspaceDocument) AddInstance(id string, mod project.ModID, parent string) error {
	return w.EditScene(w.Active, "Add module instance", func(spec *project.SceneSpec) error {
		instance := project.ModInstanceSpec{ID: id, Mod: mod}
		if parent == "" {
			spec.Modules = append(spec.Modules, instance)
		} else {
			owner := findModInstance(spec.Modules, parent)
			if owner == nil {
				return fmt.Errorf("unknown parent %q", parent)
			}
			owner.Children = append(owner.Children, instance)
		}
		return nil
	})
}

// DuplicateInstance clones placement/overrides while reusing each definition.
// Descendant identities are remapped as one scene-document command.
func (w *WorkspaceDocument) DuplicateInstance(id, newID string) error {
	return w.EditScene(w.Active, "Duplicate instance", func(spec *project.SceneSpec) error {
		original := findModInstance(spec.Modules, id)
		if original == nil {
			return fmt.Errorf("unknown instance %q", id)
		}
		data, err := documentJSON(*original)
		if err != nil {
			return err
		}
		duplicate, err := decodeDocument[project.ModInstanceSpec](data)
		if err != nil {
			return err
		}
		var rename func(*project.ModInstanceSpec, string)
		rename = func(value *project.ModInstanceSpec, replacement string) {
			value.ID = replacement
			for i := range value.Children {
				rename(&value.Children[i], replacement+"_"+value.Children[i].ID)
			}
		}
		rename(&duplicate, newID)
		var insert func(*[]project.ModInstanceSpec) bool
		insert = func(values *[]project.ModInstanceSpec) bool {
			for i := range *values {
				if (*values)[i].ID == id {
					*values = slices.Insert(*values, i+1, duplicate)
					return true
				}
				if insert(&(*values)[i].Children) {
					return true
				}
			}
			return false
		}
		insert(&spec.Modules)
		return nil
	})
}

func removeModInstance(values *[]project.ModInstanceSpec, id string) (project.ModInstanceSpec, bool) {
	for i := range *values {
		if (*values)[i].ID == id {
			value := (*values)[i]
			*values = slices.Delete(*values, i, i+1)
			return value, true
		}
		if value, ok := removeModInstance(&(*values)[i].Children, id); ok {
			return value, true
		}
	}
	return project.ModInstanceSpec{}, false
}

func (w *WorkspaceDocument) DeleteInstance(id string) error {
	return w.EditScene(w.Active, "Delete instance", func(spec *project.SceneSpec) error {
		_, ok := removeModInstance(&spec.Modules, id)
		if !ok {
			return fmt.Errorf("unknown instance %q", id)
		}
		return nil
	})
}

// DeleteInstances removes selected subtrees in one undoable edit.
func (w *WorkspaceDocument) DeleteInstances(selected map[string]bool) error {
	return w.EditScene(w.Active, "Delete selected instances", func(spec *project.SceneSpec) error {
		removed := make(map[string]bool)
		var prune func([]project.ModInstanceSpec, bool) []project.ModInstanceSpec
		prune = func(values []project.ModInstanceSpec, parentRemoved bool) []project.ModInstanceSpec {
			kept := values[:0]
			for _, value := range values {
				drop := parentRemoved || selected[value.ID]
				value.Children = prune(value.Children, drop)
				if drop {
					removed[value.ID] = true
				} else {
					kept = append(kept, value)
				}
			}
			clear(values[len(kept):])
			return kept
		}
		spec.Modules = prune(spec.Modules, false)
		if len(removed) == 0 {
			return fmt.Errorf("no selected instances")
		}
		return nil
	})
}

// MoveInstance uses the destination index after removal when reordering or
// reparenting an instance through the scene tree.
func (w *WorkspaceDocument) MoveInstance(id, parent string, index int) error {
	return w.EditScene(w.Active, "Move instance", func(spec *project.SceneSpec) error {
		original := findModInstance(spec.Modules, id)
		if original == nil {
			return fmt.Errorf("unknown instance %q", id)
		}
		if parent == id || findModInstance(original.Children, parent) != nil {
			return fmt.Errorf("cannot move an instance into itself or its descendants")
		}
		value, _ := removeModInstance(&spec.Modules, id)
		values := &spec.Modules
		if parent != "" {
			owner := findModInstance(spec.Modules, parent)
			if owner == nil {
				return fmt.Errorf("unknown parent %q", parent)
			}
			values = &owner.Children
		}
		*values = slices.Insert(*values, max(0, min(index, len(*values))), value)
		return nil
	})
}

// AddElement appends a private element to the module definition with the first
// unused elementN name. The new ID is returned so callers can report it.
func (w *WorkspaceDocument) AddElement(mod project.ModID) (string, error) {
	id := ""
	err := w.EditMod(mod, "Add module element", func(spec *project.ModSpec) error {
		taken := make(map[string]bool, len(spec.Elements))
		for _, element := range spec.Elements {
			taken[element.ID] = true
		}
		for index := 1; ; index++ {
			candidate := "element" + strconv.Itoa(index)
			if !taken[candidate] {
				id = candidate
				break
			}
		}
		spec.Elements = append(spec.Elements, project.ModElementSpec{ID: id})
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// AddResource declares a file that already lives inside the module directory,
// keyed by a name derived from its filename and suffixed when that name is taken.
func (w *WorkspaceDocument) AddResource(mod project.ModID, kind, file string) (string, error) {
	id := ""
	err := w.EditMod(mod, "Add module resource", func(spec *project.ModSpec) error {
		for name, resource := range spec.Resources {
			if resource.Path == file {
				return fmt.Errorf("resource %q already declares %s", name, file)
			}
		}
		base, _, _ := strings.Cut(path.Base(file), ".")
		if base == "" {
			base = "resource"
		}
		id = base
		for index := 2; ; index++ {
			if _, taken := spec.Resources[id]; !taken {
				break
			}
			id = base + strconv.Itoa(index)
		}
		if spec.Resources == nil {
			spec.Resources = make(map[string]project.ModResourceSpec, 1)
		}
		spec.Resources[id] = project.ModResourceSpec{Kind: kind, Path: file}
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// RemoveElement deletes an element and its descendants from the definition.
func (w *WorkspaceDocument) RemoveElement(mod project.ModID, id string) error {
	return w.EditMod(mod, "Delete module element", func(spec *project.ModSpec) error {
		removed := map[string]bool{id: true}
		found := false
		for _, element := range spec.Elements {
			if element.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown element %q", id)
		}
		for changed := true; changed; {
			changed = false
			kept := spec.Elements[:0]
			for _, element := range spec.Elements {
				if removed[element.ID] || removed[element.Parent] {
					removed[element.ID] = true
					changed = true
					continue
				}
				kept = append(kept, element)
			}
			spec.Elements = kept
		}
		return nil
	})
}
