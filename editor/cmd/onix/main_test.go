package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleSystemNewAndCheckCommands(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "demo")
	var output bytes.Buffer
	if err := run([]string{"new", "-width", "800", "-height", "600", directory}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Entry: main.json") {
		t.Fatalf("unexpected output: %s", output.String())
	}
	output.Reset()
	if err := run([]string{"check", directory}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "1 scenes, 0 module definitions") {
		t.Fatalf("unexpected check output: %s", output.String())
	}
	if err := os.WriteFile(filepath.Join(directory, "resources", "scenes", "main.json"), []byte(`{"modules":[{"id":"x","mod":"missing"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"check", directory}, &output); err == nil || !strings.Contains(err.Error(), "main.json: modules[0].mod") {
		t.Fatalf("check did not return the source diagnostic: %v", err)
	}
}

func TestModuleSystemCommandArguments(t *testing.T) {
	for _, args := range [][]string{{"new"}, {"new", "-width", "0", t.TempDir()}, {"check"}, {"check", "a", "b"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
}
