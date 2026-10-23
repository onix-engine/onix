package editor

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/diiyw/onix/project"
)

func benchmarkWorkspaceDocument(b *testing.B, count int) *WorkspaceDocument {
	b.Helper()
	directory := b.TempDir()
	if _, err := project.CreateProjectDirectory(directory, project.CreateProjectOptions{Width: 750, Height: 1624}); err != nil {
		b.Fatal(err)
	}
	mod := project.ModSpec{Elements: make([]project.ModElementSpec, 100)}
	for i := range mod.Elements {
		mod.Elements[i] = project.ModElementSpec{
			ID:         fmt.Sprintf("part%d", i),
			Transform:  project.ModTransformSpec{Position: []float32{float32(i), 0}},
			Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{10, 10}}},
		}
	}
	data, err := documentJSON(mod)
	if err != nil {
		b.Fatal(err)
	}
	for i := range count {
		if err := writeWorkspaceFile(filepath.Join(directory, fmt.Sprintf("mod%d", i), "mod.json"), data); err != nil {
			b.Fatal(err)
		}
	}
	if err := writeWorkspaceFile(filepath.Join(directory, "resources", "scenes", "main.json"), []byte(`{"modules":[{"id":"item","mod":"mod0"}]}`)); err != nil {
		b.Fatal(err)
	}
	w, err := LoadWorkspaceDocument(directory)
	if err != nil {
		b.Fatal(err)
	}
	return w
}

func BenchmarkWorkspaceDocumentDirty(b *testing.B) {
	for _, count := range []int{1, 10, 50} {
		b.Run(fmt.Sprintf("modules_%d", count), func(b *testing.B) {
			w := benchmarkWorkspaceDocument(b, count)
			if err := w.EditMod("mod0", "Rename", func(spec *project.ModSpec) error {
				spec.Name = "Renamed"
				return nil
			}); err != nil {
				b.Fatal(err)
			}
			if !w.Dirty() {
				b.Fatal("edited module is not dirty")
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = w.Dirty()
			}
		})
	}
}

func BenchmarkWorkspaceStagedViewMod(b *testing.B) {
	a := ModuleEditor{Document: benchmarkWorkspaceDocument(b, 10)}
	patches := map[DocumentID][]DocumentPatch{"mod0/mod.json": {{Pointer: "/name", Value: []byte(`"Renamed"`)}}}
	if err := a.stagePatches(patches); err != nil {
		b.Fatal(err)
	}
	if a.viewMod("mod0").Name != "Renamed" {
		b.Fatal("staged view lost the edit")
	}
	b.ReportAllocs()
	for b.Loop() {
		for range 50 {
			_ = a.viewMod("mod0")
		}
	}
}

func BenchmarkWorkspaceDraftField(b *testing.B) {
	for _, count := range []int{1, 10, 50} {
		b.Run(fmt.Sprintf("modules_%d", count), func(b *testing.B) {
			w := benchmarkWorkspaceDocument(b, count)
			field := workspaceField{Target: "mod0/mod.json", Pointer: "/name", Kind: "string", Axis: -1}
			a := ModuleEditor{Document: w, fields: map[string]workspaceField{"name": field}, drafts: map[string]string{"name": "first"}}
			a.syncDraftPreview()
			if a.draftPreview.document == nil {
				b.Fatal("draft did not compile")
			}
			b.ReportAllocs()
			for b.Loop() {
				if a.drafts["name"] == "first" {
					a.drafts["name"] = "second"
				} else {
					a.drafts["name"] = "first"
				}
				a.syncDraftPreview()
			}
		})
	}
}
