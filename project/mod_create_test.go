package project

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestModuleSystemCreateRootEntryAndReload(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "demo")
	plan, err := CreateProjectDirectory(directory, CreateProjectOptions{Width: 750, Height: 1624})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Spec.Entry != "main.json" || plan.Spec.Window.Title != "demo" || len(plan.Scenes) != 1 || len(plan.Scenes[0].Instances) != 0 {
		t.Fatalf("unexpected scaffold: %+v", plan)
	}
	for _, file := range []string{"main.go", "resources/scenes/main.json", "resources/project.json"} {
		if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(file))); err != nil {
			t.Fatal(err)
		}
	}
	for _, old := range []string{"resources/scenes.json", "resources/resources.json", "resources/scenes/main.scene.json"} {
		if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(old))); !os.IsNotExist(err) {
			t.Fatalf("unexpected old entry %s: %v", old, err)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, "main.go"), nil, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
	loaded, err := CompileProjectDirectory(directory)
	if err != nil || loaded.Spec.Entry != plan.Spec.Entry {
		t.Fatalf("reload scaffold: %v", err)
	}
	// This guard protects main.go when the user has edited their bootstrap.
	main := filepath.Join(directory, "main.go")
	userSource := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(main, userSource, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateProjectDirectory(directory, CreateProjectOptions{Width: 100, Height: 100}); err == nil {
		t.Fatal("existing project was overwritten")
	}
	data, err := os.ReadFile(main)
	if err != nil || string(data) != string(userSource) {
		t.Fatal("user main.go was overwritten")
	}
}

func TestModuleSystemCreateDoesNotPartiallyOverwriteEntries(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := CreateProjectDirectory(directory, CreateProjectOptions{Width: 750, Height: 1624})
	if err == nil || !strings.Contains(err.Error(), "main.go") {
		t.Fatalf("expected existing main.go diagnostic, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "resources", "project.json")); !os.IsNotExist(err) {
		t.Fatal("project.json was written before the entry conflict was found")
	}
	if _, err := CreateProjectDirectory(filepath.Join(directory, "invalid"), CreateProjectOptions{}); err == nil {
		t.Fatal("invalid dimensions accepted")
	}
}

func TestModuleSystemGeneratedMainCompiles(t *testing.T) {
	directory := t.TempDir()
	if _, err := CreateProjectDirectory(directory, CreateProjectOptions{Width: 750, Height: 1624}); err != nil {
		t.Fatal(err)
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	engineRoot, err := moduleRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	module := "module example.com/onix-scaffold\n\ngo 1.26.0\n\nrequire github.com/diiyw/onix v0.0.0\n\nreplace github.com/diiyw/onix => " + strconv.Quote(filepath.ToSlash(engineRoot)) + "\n"
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(engineRoot, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "go.sum"), checksums, 0o644); err != nil {
		t.Fatal(err)
	}
	// Compile the generated entry without executing main or opening a window.
	command := exec.Command("go", "test", "-mod=mod", ".")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated main.go does not compile: %v\n%s", err, output)
	}
}
