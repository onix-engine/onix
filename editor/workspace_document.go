package editor

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/diiyw/onix/project"
)

// DocumentID is the project-relative file that owns an edit and its undo state.
type DocumentID string

type SceneDocument struct {
	ID       project.SceneID
	Spec     project.SceneSpec
	Plan     *project.ScenePlan
	Dirty    bool
	Revision uint64
}

type ModDocument struct {
	ID              project.ModID
	Spec            project.ModSpec
	Plan            *project.ModPlan
	Dirty           bool
	Revision        uint64
	Deleted         bool
	contentRevision uint64
}

type WorkspaceFile struct {
	Path  string
	IsDir bool
}

type documentState struct {
	// JSON versions are immutable; current and saved baselines can share bytes.
	data, saved []byte
	disk        documentDiskState
	revision    uint64
	// dirty caches data != saved so idle frames never rescan document bytes.
	dirty bool
}

type documentDiskState struct {
	digest [sha256.Size]byte
	exists bool
}

func documentDisk(data []byte) documentDiskState {
	if data == nil {
		return documentDiskState{}
	}
	return documentDiskState{digest: sha256.Sum256(data), exists: true}
}

type documentEdit struct {
	Target        DocumentID
	Before, After []byte
}

type workspaceCommand struct {
	Label string
	Edits []documentEdit
}

// WorkspaceDocument loads the new format exclusively. Files and plans are
// retained; idle frames never read or rediscover the project.
type WorkspaceDocument struct {
	ProjectDir string
	Spec       project.ProjectSpec
	Scenes     map[project.SceneID]*SceneDocument
	Mods       map[project.ModID]*ModDocument
	Active     project.SceneID
	Revision   uint64
	Plan       *project.ProjectPlan
	Files      []WorkspaceFile
	states     map[DocumentID]*documentState
	dirtyCount int
	assets     map[string]string // private file snapshots in staging or editor trash
	assetDirs  map[string]bool
	stagingDir string
	compiler   project.CompileCache
	undo, redo []workspaceCommand
}

func LoadWorkspaceDocument(directory string) (*WorkspaceDocument, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	plan, err := project.CompileProjectDirectory(absolute)
	if err != nil {
		return nil, err
	}
	w := &WorkspaceDocument{
		ProjectDir: absolute, Spec: plan.Spec, Plan: plan, Active: plan.Spec.Entry,
		Scenes: make(map[project.SceneID]*SceneDocument), Mods: make(map[project.ModID]*ModDocument),
		states: make(map[DocumentID]*documentState), assets: make(map[string]string), assetDirs: make(map[string]bool),
	}
	load := func(id DocumentID, spec any) error {
		data, err := documentJSON(spec)
		if err != nil {
			return err
		}
		disk, err := os.ReadFile(w.Absolute(string(id)))
		if err != nil {
			return err
		}
		w.states[id] = &documentState{data: data, saved: data, disk: documentDisk(disk), revision: 1}
		return nil
	}
	if err := load("resources/project.json", plan.Spec); err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(w.Absolute("resources/packages.json")); err == nil {
		config, err := decodeDocument[resourcePackageConfig](data)
		if err != nil {
			return nil, err
		}
		if err := load("resources/packages.json", config); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for _, scene := range plan.Scenes {
		if err := load(sceneDocumentID(scene.ID), scene.Spec); err != nil {
			return nil, err
		}
	}
	for _, mod := range plan.Mods {
		if err := load(modDocumentID(mod.ID), mod.Spec); err != nil {
			return nil, err
		}
	}
	if err := w.RefreshFiles(); err != nil {
		return nil, err
	}
	// Seed the cache from canonical documents and share its plans immediately.
	// The first draft must not retain another copy of every unchanged module.
	plan, err = w.compile(nil)
	if err != nil {
		return nil, err
	}
	w.installPlan(plan)
	return w, nil
}

func sceneDocumentID(id project.SceneID) DocumentID {
	return DocumentID(project.ScenePath(id))
}

func modDocumentID(id project.ModID) DocumentID {
	return DocumentID(path.Join(string(id), project.ModFileName))
}
func (w *WorkspaceDocument) Absolute(file string) string {
	return filepath.Join(w.ProjectDir, filepath.FromSlash(file))
}

func documentJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	return append(data, '\n'), err
}

func decodeDocument[T any](data []byte) (T, error) {
	var result T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return result, fmt.Errorf("expected one JSON value")
	}
	return result, nil
}

func (w *WorkspaceDocument) Scene() *SceneDocument { return w.Scenes[w.Active] }

func (w *WorkspaceDocument) SelectScene(id project.SceneID) error {
	if w.Scenes[id] == nil {
		return fmt.Errorf("scene %q is not registered", id)
	}
	w.Active = id
	return nil
}

func (w *WorkspaceDocument) EditScene(id project.SceneID, label string, edit func(*project.SceneSpec) error) error {
	state := w.states[sceneDocumentID(id)]
	if state == nil || state.data == nil {
		return fmt.Errorf("unknown scene %q", id)
	}
	spec, err := decodeDocument[project.SceneSpec](state.data)
	if err != nil {
		return err
	}
	if err := edit(&spec); err != nil {
		return err
	}
	return w.editValue(sceneDocumentID(id), label, spec)
}

func (w *WorkspaceDocument) EditMod(id project.ModID, label string, edit func(*project.ModSpec) error) error {
	state := w.states[modDocumentID(id)]
	if state == nil || state.data == nil {
		return fmt.Errorf("unknown module %q", id)
	}
	spec, err := decodeDocument[project.ModSpec](state.data)
	if err != nil {
		return err
	}
	if err := edit(&spec); err != nil {
		return err
	}
	return w.editValue(modDocumentID(id), label, spec)
}

func (w *WorkspaceDocument) editValue(id DocumentID, label string, value any) error {
	data, err := documentJSON(value)
	if err != nil {
		return err
	}
	return w.commit(workspaceCommand{Label: label, Edits: []documentEdit{{Target: id, After: data}}})
}

func (w *WorkspaceDocument) commit(command workspaceCommand) error {
	changed := false
	for i := range command.Edits {
		edit := &command.Edits[i]
		if edit.After != nil {
			var err error
			edit.After, err = canonicalWorkspaceJSON(edit.Target, edit.After)
			if err != nil {
				return fmt.Errorf("%s: %w", edit.Target, err)
			}
		}
		if state := w.states[edit.Target]; state != nil {
			edit.Before = state.data
		}
		changed = changed || !bytes.Equal(edit.Before, edit.After)
	}
	if !changed {
		return nil
	}
	if err := w.apply(command, false); err != nil {
		return err
	}
	w.undo = append(w.undo, command)
	clear(w.redo)
	w.redo = w.redo[:0]
	return nil
}

func (w *WorkspaceDocument) apply(command workspaceCommand, backwards bool) error {
	overrides := make(map[DocumentID][]byte, len(command.Edits))
	for _, edit := range command.Edits {
		data := edit.After
		if backwards {
			data = edit.Before
		}
		overrides[edit.Target] = data
	}
	plan, err := w.compile(overrides)
	if err != nil {
		return err
	}
	for _, mod := range plan.Mods {
		id := modDocumentID(mod.ID)
		if w.states[id] == nil && overrides[id] == nil {
			return fmt.Errorf("%s: new definition discovered; refresh files before editing", id)
		}
	}
	for _, scene := range plan.Scenes {
		id := sceneDocumentID(scene.ID)
		if w.states[id] == nil && overrides[id] == nil {
			return fmt.Errorf("%s: new scene discovered; refresh files before editing", id)
		}
	}
	for id, data := range overrides {
		state := w.states[id]
		if state == nil {
			state = &documentState{}
			w.states[id] = state
		}
		if !bytes.Equal(state.data, data) {
			state.data = data
			state.revision++
			w.markDirty(state, !bytes.Equal(state.data, state.saved))
		}
	}
	w.Revision++
	w.installPlan(plan)
	return nil
}

// markDirty keeps dirtyCount in step with the per-document dirty cache.
func (w *WorkspaceDocument) markDirty(state *documentState, dirty bool) {
	if state.dirty == dirty {
		return
	}
	state.dirty = dirty
	if dirty {
		w.dirtyCount++
	} else {
		w.dirtyCount--
	}
}

func (w *WorkspaceDocument) installPlan(plan *project.ProjectPlan) {
	w.Plan, w.Spec = plan, plan.Spec
	for id := range w.Scenes {
		if state := w.states[sceneDocumentID(id)]; state == nil || state.data == nil {
			delete(w.Scenes, id)
		}
	}
	for index, scene := range plan.Scenes {
		state := w.states[sceneDocumentID(scene.ID)]
		doc := w.Scenes[scene.ID]
		if doc == nil {
			doc = &SceneDocument{ID: scene.ID}
			w.Scenes[scene.ID] = doc
		}
		doc.Spec, doc.Dirty, doc.Revision = scene.Spec, state.dirty, state.revision
		doc.Plan = &plan.Scenes[index]
	}
	for id, mod := range w.Mods {
		mod.Deleted = true
		state := w.states[modDocumentID(id)]
		if state.data == nil {
			mod.Dirty, mod.Revision = state.dirty, state.revision
			mod.Spec, mod.Plan = project.ModSpec{}, nil
		}
	}
	for index, mod := range plan.Mods {
		state := w.states[modDocumentID(mod.ID)]
		doc := w.Mods[mod.ID]
		if doc == nil {
			doc = &ModDocument{ID: mod.ID}
			w.Mods[mod.ID] = doc
		}
		if doc.Plan == nil || doc.Revision != state.revision && !workspaceModContentEqual(doc.Spec, mod.Spec) {
			doc.contentRevision = state.revision
		}
		doc.Spec, doc.Dirty, doc.Revision, doc.Deleted = mod.Spec, state.dirty, state.revision, false
		doc.Plan = &plan.Mods[index]
	}
	if w.Scenes[w.Active] == nil {
		w.Active = w.Spec.Entry
	}
}

func workspaceModContentEqual(a, b project.ModSpec) bool {
	// Names and behavior do not enter the preview; root transforms update in place.
	a.Name, b.Name, a.Behavior, b.Behavior = "", "", "", ""
	a.Transform, b.Transform = project.ModTransformSpec{}, project.ModTransformSpec{}
	return reflect.DeepEqual(a, b)
}

func (w *WorkspaceDocument) Undo() error {
	if len(w.undo) == 0 {
		return nil
	}
	command := w.undo[len(w.undo)-1]
	if err := w.apply(command, true); err != nil {
		return err
	}
	w.undo = slices.Delete(w.undo, len(w.undo)-1, len(w.undo))
	w.redo = append(w.redo, command)
	return nil
}

func (w *WorkspaceDocument) Redo() error {
	if len(w.redo) == 0 {
		return nil
	}
	command := w.redo[len(w.redo)-1]
	if err := w.apply(command, false); err != nil {
		return err
	}
	w.redo = slices.Delete(w.redo, len(w.redo)-1, len(w.redo))
	w.undo = append(w.undo, command)
	return nil
}

func (w *WorkspaceDocument) Dirty() bool { return w.dirtyCount > 0 }

// Save writes only changed owning documents, after checking external edits.
// Module removal moves the complete private folder into recoverable editor trash.
func (w *WorkspaceDocument) Save() error {
	if _, err := w.compile(nil); err != nil {
		return err
	}
	ids := make([]DocumentID, 0, len(w.states))
	for id, state := range w.states {
		if !state.dirty {
			continue
		}
		disk, err := os.ReadFile(w.Absolute(string(id)))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if documentDisk(disk) != state.disk {
			return fmt.Errorf("%s: changed outside the editor; reopen before saving", id)
		}
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b DocumentID) int {
		if w.states[a].data == nil && w.states[b].data == nil {
			if depth := strings.Count(string(b), "/") - strings.Count(string(a), "/"); depth != 0 {
				return depth
			}
		}
		return strings.Compare(string(a), string(b))
	})
	assetFiles := make([]string, 0, len(w.assets))
	for file := range w.assets {
		if state := w.assetOwner(file); state == nil || state.data == nil || state.disk.exists {
			continue
		}
		if _, err := os.Lstat(w.Absolute(file)); err == nil {
			return fmt.Errorf("%s: copy destination appeared outside the editor", file)
		} else if !os.IsNotExist(err) {
			return err
		}
		assetFiles = append(assetFiles, file)
	}
	slices.Sort(assetFiles)
	for dir := range w.assetDirs {
		if state := w.assetOwner(dir); state == nil || state.data == nil || state.disk.exists {
			continue
		}
		if err := os.MkdirAll(w.Absolute(dir), 0o755); err != nil {
			return err
		}
	}
	for _, file := range assetFiles {
		if err := copyWorkspaceAsset(os.DirFS(filepath.Dir(w.assets[file])), filepath.Base(w.assets[file]), w.Absolute(file)); err != nil {
			return err
		}
	}
	for _, id := range ids {
		state := w.states[id]
		if state.data == nil {
			if state.saved != nil && strings.HasSuffix(string(id), "/mod.json") {
				dir := w.Absolute(path.Dir(string(id)))
				trash := w.Absolute(path.Join("resources", ".editor-trash", fmt.Sprintf("%s-%d", path.Base(path.Dir(string(id))), time.Now().UnixNano())))
				private, dirs, err := snapshotWorkspaceModule(dir, trash, path.Dir(string(id)), string(id))
				if err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(trash), 0o755); err != nil {
					return err
				}
				if err := os.Rename(dir, trash); err != nil {
					return err
				}
				for file, data := range private {
					w.assets[file] = data
				}
				for dir := range dirs {
					w.assetDirs[dir] = true
				}
			} else if state.saved != nil {
				trash := w.Absolute(path.Join("resources", ".editor-trash", fmt.Sprintf("%s-%d", path.Base(string(id)), time.Now().UnixNano())))
				if err := os.MkdirAll(filepath.Dir(trash), 0o755); err != nil {
					return err
				}
				if err := os.Rename(w.Absolute(string(id)), trash); err != nil {
					return err
				}
			}
		} else if err := writeWorkspaceFile(w.Absolute(string(id)), state.data); err != nil {
			return err
		}
		state.saved, state.disk = state.data, documentDisk(state.data)
		w.markDirty(state, false)
	}
	for name := range w.assets {
		if state := w.assetOwner(name); state != nil && state.disk.exists {
			delete(w.assets, name)
		}
	}
	for name := range w.assetDirs {
		if state := w.assetOwner(name); state != nil && state.disk.exists {
			delete(w.assetDirs, name)
		}
	}
	w.installPlan(w.Plan)
	return w.RefreshFiles()
}

func snapshotWorkspaceModule(directory, storage, prefix, definition string) (map[string]string, map[string]bool, error) {
	files, dirs := make(map[string]string), make(map[string]bool)
	source := os.DirFS(directory)
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		file := path.Join(prefix, name)
		if entry.IsDir() {
			dirs[file] = true
			return nil
		}
		if file == definition {
			return nil
		}
		files[file] = filepath.Join(storage, filepath.FromSlash(name))
		return nil
	})
	return files, dirs, err
}

// Refresh discovers new definitions while preserving all open document drafts.
// Existing external document edits remain conflicts until the project is reopened.
func (w *WorkspaceDocument) Refresh() error {
	if err := w.RefreshFiles(); err != nil {
		return err
	}
	plan, err := w.compile(nil)
	if err != nil {
		return err
	}
	newStates := make(map[DocumentID]*documentState)
	for _, mod := range plan.Mods {
		id := modDocumentID(mod.ID)
		if w.states[id] != nil {
			continue
		}
		data, err := documentJSON(mod.Spec)
		if err != nil {
			return err
		}
		disk, err := os.ReadFile(w.Absolute(string(id)))
		if err != nil {
			return err
		}
		newStates[id] = &documentState{data: data, saved: data, disk: documentDisk(disk), revision: 1}
	}
	for _, scene := range plan.Scenes {
		id := sceneDocumentID(scene.ID)
		if w.states[id] != nil {
			continue
		}
		data, err := documentJSON(scene.Spec)
		if err != nil {
			return err
		}
		disk, err := os.ReadFile(w.Absolute(string(id)))
		if err != nil {
			return err
		}
		newStates[id] = &documentState{data: data, saved: data, disk: documentDisk(disk), revision: 1}
	}
	for id, state := range newStates {
		w.states[id] = state
	}
	w.Revision++
	w.installPlan(plan)
	return nil
}

func (w *WorkspaceDocument) assetOwner(file string) *documentState {
	for owner := file; owner != "."; owner = path.Dir(owner) {
		if state := w.states[modDocumentID(project.ModID(owner))]; state != nil {
			return state
		}
	}
	return nil
}

func (w *WorkspaceDocument) compile(overrides map[DocumentID][]byte) (*project.ProjectPlan, error) {
	definitions := make(map[project.ModID]bool, len(w.Mods))
	for id, state := range w.states {
		if strings.HasSuffix(string(id), "/mod.json") && state.data != nil {
			definitions[project.ModID(path.Dir(string(id)))] = true
		}
	}
	for id, data := range overrides {
		if !strings.HasSuffix(string(id), "/mod.json") {
			continue
		}
		mod := project.ModID(path.Dir(string(id)))
		if data == nil {
			delete(definitions, mod)
		} else {
			definitions[mod] = true
		}
	}
	return w.compiler.Compile(workspaceFS{w: w, overrides: overrides}, ".", sortedWorkspaceKeys(definitions))
}

func canonicalWorkspaceJSON(id DocumentID, data []byte) ([]byte, error) {
	if id == "resources/packages.json" {
		value, err := decodeDocument[resourcePackageConfig](data)
		if err != nil {
			return nil, err
		}
		if err := validateResourcePackageConfig(value); err != nil {
			return nil, err
		}
		return documentJSON(value)
	}
	if id == "resources/project.json" {
		value, err := decodeDocument[project.ProjectSpec](data)
		if err != nil {
			return nil, err
		}
		return documentJSON(value)
	}
	if strings.HasSuffix(string(id), "/mod.json") {
		value, err := decodeDocument[project.ModSpec](data)
		if err != nil {
			return nil, err
		}
		return documentJSON(value)
	}
	value, err := decodeDocument[project.SceneSpec](data)
	if err != nil {
		return nil, err
	}
	return documentJSON(value)
}

func writeWorkspaceFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".onix-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, name)
}

func (w *WorkspaceDocument) RefreshFiles() error {
	files := make([]WorkspaceFile, 0, len(w.Files))
	err := fs.WalkDir(os.DirFS(w.ProjectDir), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".git" || name == "resources/.editor-trash" || name == workspaceStagingDirectory) {
			return fs.SkipDir
		}
		files = append(files, WorkspaceFile{Path: name, IsDir: entry.IsDir()})
		return nil
	})
	if err != nil {
		return err
	}
	w.Files = files
	return nil
}

// ModuleFiles uses the cached inventory and includes unsaved definitions/copies.
func (w *WorkspaceDocument) ModuleFiles(id project.ModID) []WorkspaceFile {
	prefix := ""
	if id != "" {
		prefix = string(id) + "/"
	}
	files := make(map[string]WorkspaceFile)
	add := func(file WorkspaceFile) {
		if !strings.HasPrefix(file.Path, prefix) {
			return
		}
		if state := w.states[DocumentID(file.Path)]; state != nil && state.data == nil {
			return
		}
		for owner := file.Path; owner != "."; owner = path.Dir(owner) {
			if mod := w.Mods[project.ModID(owner)]; mod != nil && mod.Deleted {
				return
			}
		}
		files[file.Path] = file
	}
	for _, file := range w.Files {
		add(file)
	}
	for name := range w.assets {
		add(WorkspaceFile{Path: name})
	}
	for name := range w.assetDirs {
		add(WorkspaceFile{Path: name, IsDir: true})
	}
	for name, state := range w.states {
		if state.data != nil {
			add(WorkspaceFile{Path: string(name)})
		}
	}
	result := make([]WorkspaceFile, 0, len(files))
	for _, file := range files {
		result = append(result, file)
	}
	slices.SortFunc(result, func(a, b WorkspaceFile) int { return strings.Compare(a.Path, b.Path) })
	return result
}

// workspaceFS overlays authoring documents without modifying the disk or
// projecting them into the removed node/prefab data model.
type workspaceFS struct {
	w         *WorkspaceDocument
	overrides map[DocumentID][]byte
}

func (s workspaceFS) data(name string) ([]byte, bool) {
	if data, ok := s.overrides[DocumentID(name)]; ok {
		return data, true
	}
	if state := s.w.states[DocumentID(name)]; state != nil {
		return state.data, true
	}
	return nil, false
}

func (s workspaceFS) Open(name string) (fs.File, error) {
	if data, ok := s.data(name); ok {
		if data == nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return &workspaceMemoryFile{Reader: *bytes.NewReader(data), name: path.Base(name), size: int64(len(data))}, nil
	}
	if file, ok := s.w.assets[name]; ok {
		return os.Open(file)
	}
	return os.DirFS(s.w.ProjectDir).Open(name)
}

func (s workspaceFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == project.SceneDirectory {
		entries, err := fs.ReadDir(os.DirFS(s.w.ProjectDir), name)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		files := make(map[string]fs.DirEntry, len(entries))
		for _, entry := range entries {
			files[entry.Name()] = entry
		}
		include := func(id DocumentID, data []byte) {
			if path.Dir(string(id)) != name {
				return
			}
			base := path.Base(string(id))
			if data == nil {
				delete(files, base)
			} else {
				files[base] = workspaceFileInfo{name: base, size: int64(len(data))}
			}
		}
		for id, state := range s.w.states {
			include(id, state.data)
		}
		for id, data := range s.overrides {
			include(id, data)
		}
		entries = entries[:0]
		for _, entry := range files {
			entries = append(entries, entry)
		}
		slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
		return entries, nil
	}
	if name != "." {
		return fs.ReadDir(os.DirFS(s.w.ProjectDir), name)
	}
	// Compilation needs only root module directories. The inventory was read once.
	names := make(map[string]bool)
	for _, file := range s.w.Files {
		if file.IsDir && !strings.Contains(file.Path, "/") {
			names[file.Path] = true
		}
	}
	for id, state := range s.w.states {
		if strings.HasSuffix(string(id), "/mod.json") && state.data != nil {
			names[strings.SplitN(string(id), "/", 2)[0]] = true
		}
	}
	for id, data := range s.overrides {
		if strings.HasSuffix(string(id), "/mod.json") && data != nil {
			names[strings.SplitN(string(id), "/", 2)[0]] = true
		}
	}
	entries := make([]fs.DirEntry, 0, len(names))
	for name := range names {
		entries = append(entries, workspaceFileInfo{name: name, dir: true})
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

type workspaceMemoryFile struct {
	bytes.Reader
	name string
	size int64
}

func (f *workspaceMemoryFile) Close() error { return nil }
func (f *workspaceMemoryFile) Stat() (fs.FileInfo, error) {
	return workspaceFileInfo{name: f.name, size: f.size}, nil
}

type workspaceFileInfo struct {
	name string
	size int64
	dir  bool
}

func (f workspaceFileInfo) Name() string { return f.name }
func (f workspaceFileInfo) Size() int64  { return f.size }
func (f workspaceFileInfo) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (f workspaceFileInfo) ModTime() time.Time         { return time.Time{} }
func (f workspaceFileInfo) IsDir() bool                { return f.dir }
func (f workspaceFileInfo) Sys() any                   { return nil }
func (f workspaceFileInfo) Type() fs.FileMode          { return f.Mode().Type() }
func (f workspaceFileInfo) Info() (fs.FileInfo, error) { return f, nil }
