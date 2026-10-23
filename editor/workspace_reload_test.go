package editor

import (
	"testing"

	"github.com/diiyw/onix/project"
)

func TestWorkspaceReloadKeepsModuleEditingAndSelection(t *testing.T) {
	for _, pending := range []string{"", "discard-pending", "save-pending"} {
		t.Run(pending, func(t *testing.T) {
			a := contentEditorFixture(t, project.ModComponents{})
			if err := a.Document.CreateScene("secondary.json"); err != nil {
				t.Fatal(err)
			}
			if err := a.Document.EditScene("secondary.json", "Fixture", func(spec *project.SceneSpec) error {
				spec.Modules = []project.ModInstanceSpec{{ID: "secondary", Mod: "button"}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := a.Document.Save(); err != nil {
				t.Fatal(err)
			}
			a.enterModuleEdit("secondary.json", "secondary")
			a.selectElement("secondary", "face")
			a.zoom, a.viewport2D.offset = 2, [2]float32{17, 23}
			if pending != "" {
				if err := a.Document.EditMod("button", "Rename", func(spec *project.ModSpec) error {
					spec.Name = "Changed"
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			} else {
				// A real disk change must appear in the reloaded definition.
				spec := a.Document.Mods["button"].Spec
				spec.Name = "From disk"
				data, err := documentJSON(spec)
				if err != nil {
					t.Fatal(err)
				}
				if err := writeWorkspaceFile(a.Document.Absolute(string(modDocumentID("button"))), data); err != nil {
					t.Fatal(err)
				}
			}
			a.action("file.reload")
			if pending != "" {
				if a.pendingAction != "navigate:reload" {
					t.Fatal("reload bypassed unsaved changes confirmation")
				}
				a.action(pending)
			}
			if a.Document.Active != "secondary.json" || a.editingModule != (workspaceModuleKey{Scene: "secondary.json", ID: "secondary"}) || a.Selected != "secondary" || a.Local != "button" || a.selectedElement != "face" {
				t.Fatalf("reload lost editing position: scene %q, module %+v, selection %q/%q, local %q", a.Document.Active, a.editingModule, a.Selected, a.selectedElement, a.Local)
			}
			if a.zoom != 2 || a.viewport2D.offset != ([2]float32{17, 23}) || a.Document.Dirty() || a.hasDrafts() {
				t.Fatal("reload changed the viewport or retained unsaved edits")
			}
			wantName := "Button"
			if pending == "" {
				wantName = "From disk"
			} else if pending == "save-pending" {
				wantName = "Changed"
			}
			if a.Document.Mods["button"].Spec.Name != wantName {
				t.Fatalf("reload did not use disk contents: %q", a.Document.Mods["button"].Spec.Name)
			}
		})
	}
}

func TestWorkspaceReloadClearsDeletedModuleInstance(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	if err := writeWorkspaceFile(a.Document.Absolute(string(sceneDocumentID("main.json"))), []byte(`{"modules":[]}`)); err != nil {
		t.Fatal(err)
	}
	a.action("file.reload")
	if a.editingModule.Scene != "" || a.Selected != "" || a.Local != "" || a.selectedElement != "" {
		t.Fatal("reload retained a deleted instance's editing state")
	}
}

func TestWorkspaceReloadFailureKeepsDocumentAndEditingState(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	doc := a.Document
	if err := writeWorkspaceFile(doc.Absolute(string(modDocumentID("button"))), []byte(`{`)); err != nil {
		t.Fatal(err)
	}
	if err := a.navigateWorkspace("reload"); err == nil {
		t.Fatal("invalid disk contents were accepted")
	}
	if a.Document != doc || a.editingModule.ID != "start" || a.selectedElement != "face" {
		t.Fatal("failed reload destroyed the current editing state")
	}
}
