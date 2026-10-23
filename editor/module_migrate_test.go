package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diiyw/onix/project"
)

func migrationFixture(t *testing.T, scene, main string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                           "module example.com/migrated\ngo 1.26.0\n",
		"resources/project.json":           `{"window":{"width":320,"height":240}}`,
		"resources/scenes.json":            `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`,
		"resources/resources.json":         `{}`,
		"resources/scenes/main.scene.json": scene,
		"main.go":                          main,
	}
	for name, data := range files {
		if err := migrationWrite(dir, name, []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestModuleMigrationPreservesOrderAndCoordinateBoundaries(t *testing.T) {
	source := migrationFixture(t, `{"nodes":[{"id":"group","coordinateSpace":"ui","children":[{"id":"world","order":1},{"id":"ui","coordinateSpace":"ui","order":2}]},{"id":"first","order":-1}]}`, "package main\nfunc main() {}\n")
	output := filepath.Join(t.TempDir(), "draft")
	report, err := MigrateModuleProject(source, output)
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || len(report.Issues) == 0 {
		t.Fatal("cross-space binding was silently migrated")
	}
	plan, err := project.CompileProjectDirectory(output)
	if err != nil {
		t.Fatal(err)
	}
	var ui, world *project.ModPlan
	for i := range plan.Mods {
		mod := &plan.Mods[i]
		if mod.Spec.Space == project.CoordinateSpaceUI {
			ui = mod
		} else {
			world = mod
		}
	}
	if ui == nil || world == nil || len(ui.Elements) != 2 || len(world.Elements) != 2 {
		t.Fatal("private content was lost")
	}
	if ui.Elements[1].Spec.Parent != "group" || ui.Elements[1].Parent != 0 {
		t.Fatal("space boundary changed the following sibling's parent")
	}
	if world.Elements[0].Spec.ID != "first" {
		t.Fatal("stable authored order was lost")
	}
	if _, err := os.Stat(filepath.Join(output, "resources/migration-report.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := project.CompileProjectDirectory(source); err == nil {
		t.Fatal("new runtime accepted old project")
	}
}

func TestModuleMigrationReportsCustomEntryCallsAndDoesNotOverwrite(t *testing.T) {
	source := migrationFixture(t, `{"nodes":[]}`, "package main\nfunc main() { app := makeGame(); app.Run() }\n")
	before, err := os.ReadFile(filepath.Join(source, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "draft")
	report, err := MigrateModuleProject(source, output)
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || !strings.Contains(report.Issues[0].Reason, "custom entry call") {
		t.Fatal("custom main was discarded without an issue")
	}
	after, err := os.ReadFile(filepath.Join(source, "main.go"))
	if err != nil || string(before) != string(after) {
		t.Fatal("source was changed")
	}
	if _, err := MigrateModuleProject(source, output); err == nil {
		t.Fatal("existing draft was overwritten")
	}
	if _, err := MigrateModuleProject(source, filepath.Join(source, "draft")); err == nil {
		t.Fatal("nested migration output was accepted")
	}
}

func TestModuleMigrationRejectsAmbiguousSceneNamesBeforeCreatingDraft(t *testing.T) {
	source := migrationFixture(t, `{"nodes":[]}`, "package main\nfunc main() {}\n")
	if err := migrationWrite(source, "resources/scenes.json", []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"},{"id":"other","path":"other/Main.scene.json"}]}`)); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "draft")
	if _, err := MigrateModuleProject(source, output); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal("ambiguous scene basenames were accepted", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("invalid migration created a draft")
	}
}

func TestModuleMigrationBoundsFileAccessAndReportsEntryState(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../outside.json", "a/../../outside.json", `a\file.json`, "C:/outside.json"} {
		if _, err := migrationBytes(root, name); err == nil {
			t.Fatal("unsafe read", name)
		}
		if err := migrationWrite(root, name, nil); err == nil {
			t.Fatal("unsafe write", name)
		}
	}
	for _, code := range []string{"x := 1; _ = x", "for {}", "defer app.Run()", "app.Config.Value = true"} {
		source := migrationFixture(t, `{"nodes":[]}`, "package main\nfunc main() { "+code+" }\n")
		report, err := MigrateModuleProject(source, filepath.Join(t.TempDir(), "draft"))
		if err != nil || report.Ready || len(report.Issues) == 0 {
			t.Fatal("entry state was silently discarded", code, report, err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.json")); err != nil {
		t.Skip("symlink creation unavailable", err)
	}
	if _, err := migrationBytes(root, "link.json"); err == nil {
		t.Fatal("symlink escaped the migration root")
	}
}
