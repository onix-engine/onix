package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diiyw/onix/window"
)

// CreateProjectOptions configures the authoring scaffold. Module folders and
// their private resource layouts are created by the module author.
type CreateProjectOptions struct {
	Width  int
	Height int
	Title  string
	// Used when there is no enclosing go.mod. EngineDirectory selects a local
	// engine checkout; EngineVersion selects a published module version.
	ModulePath      string
	EngineDirectory string
	EngineVersion   string
}

// CreateProjectDirectory creates the module-system project entry.
// Existing entry files are never overwritten, including user main.go.
// It returns the same compiled representation used by the authoring validator.
func CreateProjectDirectory(directory string, options CreateProjectOptions) (*ProjectPlan, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("project directory is required")
	}
	if options.Width <= 0 || options.Height <= 0 {
		return nil, fmt.Errorf("project width and height must be positive")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if options.Title == "" {
		options.Title = filepath.Base(absolute)
	}
	spec := ProjectSpec{
		Entry:      SceneID(DefaultSceneFile),
		Scenes:     []SceneID{SceneID(DefaultSceneFile)},
		ClearColor: "#14161b",
		Window:     WindowConfig{Width: options.Width, Height: options.Height, Title: options.Title},
		Viewport:   SizeConfig{Width: options.Width, Height: options.Height, Mode: window.ViewportModeContain},
	}
	if err := ValidateProjectSpec(spec); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, err
	}
	files := []struct {
		name string
		data []byte
	}{
		{"resources/project.json", append(manifest, '\n')},
		{ScenePath(SceneID(DefaultSceneFile)), []byte("{\n  \"modules\": []\n}\n")},
		{"main.go", []byte(moduleProjectMain)},
		{"onix_generated.go", []byte(moduleProjectRegistry)},
	}
	hasModule := false
	for current := absolute; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			hasModule = true
			break
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	if !hasModule {
		name := options.ModulePath
		if name == "" {
			name = "game.local/" + strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
					return r
				}
				return '-'
			}, filepath.Base(absolute))
		}
		if strings.ContainsAny(name, " \t\r\n\\\"") || strings.Contains(name, "..") {
			return nil, fmt.Errorf("invalid Go module path %q", name)
		}
		version := options.EngineVersion
		if version == "" {
			version = "v0.0.0"
		}
		if strings.ContainsAny(version, " \t\r\n\"") {
			return nil, fmt.Errorf("invalid engine version %q", version)
		}
		text := fmt.Sprintf("module %s\n\ngo 1.27.0\n\nrequire github.com/diiyw/onix %s\n", name, version)
		if options.EngineDirectory != "" {
			engine, err := filepath.Abs(options.EngineDirectory)
			if err != nil {
				return nil, err
			}
			if _, err := os.Stat(filepath.Join(engine, "go.mod")); err != nil {
				return nil, fmt.Errorf("local engine: %w", err)
			}
			text += fmt.Sprintf("\nreplace github.com/diiyw/onix => %q\n", filepath.ToSlash(engine))
			if sums, err := os.ReadFile(filepath.Join(engine, "go.sum")); err == nil {
				files = append(files, struct {
					name string
					data []byte
				}{"go.sum", sums})
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		files = append(files, struct {
			name string
			data []byte
		}{"go.mod", []byte(text)})
	}
	// Preflight every destination before writing any project file. Lstat also
	// prevents replacing a dangling link at a reserved entry path.
	for _, file := range files {
		name := filepath.Join(absolute, filepath.FromSlash(file.name))
		if _, err := os.Lstat(name); err == nil {
			return nil, fmt.Errorf("project entry already exists: %s", name)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Join(absolute, filepath.FromSlash(SceneDirectory)), 0o755); err != nil {
		return nil, err
	}
	created := make([]string, 0, len(files))
	rollback := func() {
		for _, name := range created {
			_ = os.Remove(name)
		}
	}
	for _, file := range files {
		name := filepath.Join(absolute, filepath.FromSlash(file.name))
		output, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			rollback()
			return nil, err
		}
		created = append(created, name)
		_, writeErr := output.Write(file.data)
		closeErr := output.Close()
		if writeErr != nil {
			rollback()
			return nil, writeErr
		}
		if closeErr != nil {
			rollback()
			return nil, closeErr
		}
	}
	plan, err := CompileProjectDirectory(absolute)
	if err != nil {
		rollback()
		return nil, err
	}
	return plan, nil
}

// Generated behavior registration lives in a separate file; subsequent
// authoring operations do not rewrite this user-owned entry.
const moduleProjectMain = `package main

import (
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/project"
)

func main() {
	plan, source, err := project.OpenModuleProject()
	if err != nil {
		panic(err)
	}
	app := onix.NewApp()
	registry, err := OnixRegistry()
	if err != nil { panic(err) }
	if err := OnixInstallSystems(app); err != nil { panic(err) }
	if err := OnixOpenWindow(app, plan.Spec, source); err != nil { panic(err) }
	if _, err := app.LoadModules[OnixContentProfile](plan, registry, OnixModuleOptions(source)); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
`

const moduleProjectRegistry = `// Code generated by Onix; DO NOT EDIT.
package main

import (
	"io/fs"
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
)

type OnixContentProfile = module.Render2D

func OnixRegistry() (module.Registry, error) { return module.Registry{}, nil }
func OnixInstallSystems(*onix.App) error { return nil }
func OnixModuleOptions(source fs.FS) module.Options { return module.Options{Source: source} }
func OnixOpenWindow(app *onix.App,spec project.ProjectSpec,source fs.FS) error { return app.OpenModuleWindow(spec,source,nil) }
`
