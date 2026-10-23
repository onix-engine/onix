package editor

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/window"
)

type workspaceFileBrowser struct {
	Document  *WorkspaceDocument
	Mod       project.ModID
	Directory string
}

func (a *ModuleEditor) resourceDirectory() string {
	browser, mod := &a.fileBrowser, a.selectedMod()
	if browser.Document != a.Document || browser.Mod != mod {
		*browser = workspaceFileBrowser{Document: a.Document, Mod: mod, Directory: string(mod)}
		a.lastFile = ""
	}
	return browser.Directory
}

func (a *ModuleEditor) resourceParentDirectory() {
	if a.Document == nil {
		return
	}
	directory := a.resourceDirectory()
	if directory == string(a.fileBrowser.Mod) {
		return
	}
	parent := path.Dir(directory)
	if parent == "." {
		parent = ""
	}
	a.fileBrowser.Directory, a.lastFile = parent, ""
	a.changed()
}

// fileMenuAction handles the resource browser's context menu buttons.
func (a *ModuleEditor) fileMenuAction(action string) bool {
	if !a.fileMenuOpen {
		return false
	}
	switch action {
	case "file-menu-new-folder":
		a.fileMenuOpen = false
		if err := a.applyDrafts(); err != nil {
			a.report(err)
		} else {
			a.folderDraft = ""
			a.showWorkspaceDialog("folder-create")
		}
	case "file-menu-delete":
		a.fileMenuOpen = false
		if err := a.applyDrafts(); err != nil {
			a.report(err)
		} else {
			a.report(a.deleteWorkspaceFile(a.contextFile))
		}
	default:
		return false
	}
	a.changed()
	return true
}

// deleteWorkspaceFile removes one browser entry. JSON documents are deleted
// through the undoable state (files move to editor trash on save); plain
// files and folders move to editor trash immediately.
func (a *ModuleEditor) deleteWorkspaceFile(file string) error {
	w := a.Document
	if w == nil || file == "" {
		return fmt.Errorf("select a file or folder first")
	}
	if file == "resources/project.json" {
		return fmt.Errorf("project settings cannot be deleted")
	}
	if mod := project.ModID(file); w.Mods[mod] != nil && !w.Mods[mod].Deleted {
		if err := w.DeleteMod(mod); err != nil {
			return err
		}
		a.forgetDeletedFile(file)
		a.status = "Deleted module " + file + ". Press Ctrl+S to save"
		return nil
	}
	if _, ok := w.states[DocumentID(file)]; ok {
		if err := w.DeleteDocument(file); err != nil {
			return err
		}
		a.forgetDeletedFile(file)
		a.status = "Deleted " + file + ". Press Ctrl+S to save"
		return nil
	}
	isDir := false
	for _, entry := range w.Files {
		if entry.Path == file {
			isDir = entry.IsDir
			break
		}
	}
	if isDir {
		for id := range w.states {
			if strings.HasPrefix(string(id), file+"/") {
				return fmt.Errorf("%s contains project documents; delete them individually", file)
			}
		}
	}
	w.undeclareResources(file)
	if _, err := os.Lstat(w.Absolute(file)); err == nil {
		trash := w.Absolute(path.Join("resources", ".editor-trash", fmt.Sprintf("%s-%d", path.Base(file), time.Now().UnixNano())))
		if err := os.MkdirAll(filepath.Dir(trash), 0o755); err != nil {
			return err
		}
		if err := os.Rename(w.Absolute(file), trash); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if _, staged := w.assets[file]; !staged {
		if _, staged := w.assetDirs[file]; !staged {
			return fmt.Errorf("%s is already gone", file)
		}
	}
	delete(w.assets, file)
	delete(w.assetDirs, file)
	if err := w.RefreshFiles(); err != nil {
		return err
	}
	a.forgetDeletedFile(file)
	a.status = "Moved " + file + " to editor trash"
	return nil
}

// forgetDeletedFile drops selection and browsing state for a removed path.
func (a *ModuleEditor) forgetDeletedFile(file string) {
	if a.lastFile == file {
		a.lastFile = ""
	}
	if dir := a.fileBrowser.Directory; dir == file || strings.HasPrefix(dir, file+"/") {
		parent := path.Dir(file)
		if parent == "." {
			parent = ""
		}
		a.fileBrowser.Directory = parent
	}
}

func (a *ModuleEditor) layoutFolderCreateDialog(panel rect) {
	const name = "folder-create"
	directory := a.resourceDirectory()
	if directory == "" {
		directory = "project root"
	}
	a.dialogLabel(name, name+"-title", "New Folder", 24, 22, panel.W-48, 28)
	a.dialogLabel(name, name+"-body", "Directory: "+directory, 24, 62, panel.W-48, 28)
	a.dialogLabel(name, name+"-label", "Folder name", 24, 104, panel.W-48, 24)
	a.dialogField(name, "folder-name", "text", a.folderDraft, 24, 132, panel.W-48, 32)
	a.dialogButton(name, name+"-confirm", "Create", panel.W-140, panel.H-54, 116, 32, false)
	a.dialogClose(name)
}

func (a *ModuleEditor) confirmFolderCreate() {
	name := strings.TrimSpace(a.folderDraft)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\:*?"<>|`) {
		a.report(fmt.Errorf("Enter a valid folder name"))
		return
	}
	if err := a.Document.CreateFolder(path.Join(a.resourceDirectory(), name)); err != nil {
		a.report(err)
		return
	}
	a.closeWorkspaceDialog()
	a.status = "Created folder " + name
	a.changed()
}

type workspaceResourceType struct {
	Kind   string
	Filter window.FileDialogFilter
}

var workspaceResourceTypes = [...]workspaceResourceType{
	{"font", window.FileDialogFilter{Name: "Font", Extensions: []string{"ttf", "otf", "woff", "woff2"}}},
	{"image", window.FileDialogFilter{Name: "Image", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "avif", "ktx", "ktx2", "dds", "ontex", "hdr", "exr"}}},
	{"audio", window.FileDialogFilter{Name: "Audio", Extensions: []string{"wav", "mp3", "ogg", "flac", "aac", "m4a"}}},
	{"video", window.FileDialogFilter{Name: "Video", Extensions: []string{"mp4", "webm", "mov", "avi", "mkv"}}},
	{"html", window.FileDialogFilter{Name: "HTML", Extensions: []string{"html"}}},
	{"shader", window.FileDialogFilter{Name: "Shader", Extensions: []string{"wgsl"}}},
	// Model sidecars for animations and collision meshes are JSON.
	{"model", window.FileDialogFilter{Name: "Model", Extensions: []string{"gltf", "glb", "obj", "fbx", "mqo", "stl", "blend", "json"}}},
	{"spine", window.FileDialogFilter{Name: "Spine", Extensions: []string{"skel", "json"}}},
	{"atlas", window.FileDialogFilter{Name: "Atlas", Extensions: []string{"atlas", "json"}}},
	{"animation", window.FileDialogFilter{Name: "Animation", Extensions: []string{"anim", "animation", "json"}}},
	{"tilemap", window.FileDialogFilter{Name: "Tilemap", Extensions: []string{"tile", "tilemap", "tileset", "tmx", "tmj", "tsj", "json"}}},
	{"data", window.FileDialogFilter{Name: "Data", Extensions: []string{"*"}}},
}

// pickModuleResource declares an existing module file using the menu's kind.
// The explicit kind also disambiguates JSON resources such as Spine and Atlas.
func (a *ModuleEditor) pickModuleResource(mod project.ModID, kind string) (string, error) {
	index := slices.IndexFunc(workspaceResourceTypes[:], func(value workspaceResourceType) bool { return value.Kind == kind })
	if index < 0 {
		return "", fmt.Errorf("unknown resource kind %q", kind)
	}
	filter := workspaceResourceTypes[index].Filter
	value, err := a.chooseWorkspacePath(false, "Add "+filter.Name+" resource", a.Document.Absolute(string(mod)), filter)
	if err != nil || value == "" {
		return "", err
	}
	relative, err := filepath.Rel(a.Document.Absolute(string(mod)), value)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resource %s is outside module %s", value, mod)
	}
	file := filepath.ToSlash(relative)
	if kind != "data" && !slices.Contains(filter.Extensions, strings.TrimPrefix(strings.ToLower(path.Ext(file)), ".")) {
		return "", fmt.Errorf("resource %s is not a %s file", file, filter.Name)
	}
	name, err := a.Document.AddResource(mod, kind, file)
	if err != nil {
		return "", err
	}
	a.status = "Added resource " + name
	return name, nil
}
