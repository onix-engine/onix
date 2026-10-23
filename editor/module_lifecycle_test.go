package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleGeneratorBindsSceneLifetimeHooksAndRejectsBadSignature(t *testing.T) {
	directory := moduleBuildFixture(t)
	moduleBuildFile(t, directory, "resources/scenes/game.json", `{"behavior":"Game","modules":[]}`)
	moduleBuildFile(t, directory, "game.go", `package main
import m "github.com/diiyw/onix/module"
type Game struct{}
func (*Game) Enter(*m.Context, m.SceneStartup) error { return nil }
func (*Game) Exit(*m.Context) error { return nil }
func (*Game) Suspend(*m.Context) error { return nil }
func (*Game) Resume(*m.Context, m.SceneStartup) error { return nil }
`)
	if _, err := GenerateModuleProject(directory); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "onix_generated.go")
	generated, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range []string{"Enter", "Exit", "Suspend", "Resume"} {
		if !strings.Contains(string(generated), "return m."+hook+"(c") {
			t.Fatalf("missing generated %s binding", hook)
		}
	}
	moduleBuildFile(t, directory, "game.go", `package main
import m "github.com/diiyw/onix/module"
type Game struct{}
func (*Game) Resume(*m.Context, string) error { return nil }
`)
	if _, err := GenerateModuleProject(directory); err == nil || !strings.Contains(err.Error(), "Resume signature") {
		t.Fatalf("bad resume signature: %v", err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(generated, after) {
		t.Fatal("failed lifetime validation rewrote the registry")
	}
}
