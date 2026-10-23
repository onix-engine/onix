package editor

import (
	"bytes"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestWorkspaceDetailsVisibilityLivePreview(t *testing.T) {
	for _, threeD := range []bool{false, true} {
		name := "2D"
		if threeD {
			name = "3D"
		}
		t.Run(name, func(t *testing.T) {
			var a *ModuleEditor
			var context *ecs.SystemContext
			id := "start"
			if threeD {
				a, context = workspace3DEditorFixture(t)
				id = "model"
				if err := a.Document.EditMod("button", "Use world coordinates", func(mod *project.ModSpec) error {
					mod.Space = project.CoordinateSpaceWorld
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			} else {
				a = workspaceEditorFixture(t)
				context = &ecs.SystemContext{World: ecs.NewWorld()}
			}
			if err := a.Document.MoveInstance("again", id, 0); err != nil {
				t.Fatal(err)
			}
			a.selectInstance(id)
			if err := a.sync(context); err != nil {
				t.Fatal(err)
			}
			_, pointer := locateWorkspaceInstance(a.Document.Scene().Spec.Modules, id)
			field := workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules"+pointer+"/visible", "")
			revision, history := a.Document.Revision, len(a.Document.undo)
			disk := readWorkspaceTestFile(t, a.Document, string(sceneDocumentID(a.Document.Active)))
			assertVisible := func(want bool) {
				t.Helper()
				for i, item := range a.Preview.Items {
					if item.ID != id && item.ID != "again" {
						continue
					}
					if item.Visible != want {
						t.Fatalf("%s visibility = %t, want %t", item.ID, item.Visible, want)
					}
					if threeD {
						for _, part := range a.viewport3D.rows[i].parts {
							if part.entity != 0 && ecs.GetRef[scene.Visibility](a.viewport3D.world, part.entity).Visible != want {
								t.Fatal("3D canvas did not follow Details visibility")
							}
						}
					} else if viewportContentVisible(a.viewport2D.world, a.viewport2D.rows[i].root) != want {
						t.Fatal("2D canvas did not follow Details visibility")
					}
				}
			}
			for _, checked := range []bool{false, true, false} {
				a.handleEvent(htmlui.Event{ID: field, Kind: htmlui.EventChange, Checked: checked})
				if !a.dirty {
					t.Fatal("Details did not request a redraw")
				}
				if err := a.sync(context); err != nil {
					t.Fatal(err)
				}
				assertVisible(checked)
			}
			if a.Document.Revision != revision || len(a.Document.undo) != history || a.Document.Instance(id).Visible != nil || !bytes.Equal(disk, readWorkspaceTestFile(t, a.Document, string(sceneDocumentID(a.Document.Active)))) {
				t.Fatal("preview committed draft data or changed disk/history")
			}
			if allocs := testing.AllocsPerRun(100, a.syncDraftPreview); allocs != 0 {
				t.Fatalf("unchanged draft preview allocated %g", allocs)
			}
			clear(a.drafts)
			a.changed()
			if err := a.sync(context); err != nil {
				t.Fatal(err)
			}
			assertVisible(true)
		})
	}
}

func TestWorkspaceDetailsTransformsAndComponentsLivePreview(t *testing.T) {
	a := workspaceEditorFixture(t)
	a.selectInstance("start")
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	sync := func() {
		t.Helper()
		if err := a.sync(context); err != nil {
			t.Fatal(err)
		}
		if a.viewport2D.err != nil {
			t.Fatal(a.viewport2D.err)
		}
	}
	sync()
	x := workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules/0/placement/position", "X")
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventInput, Value: "90"})
	sync()
	row := &a.viewport2D.rows[0]
	if a.Preview.Items[0].Content[12] != 105 || ecs.GetRef[scene.UITransform](a.viewport2D.world, row.placement).Translation.X.Value != 90 {
		t.Fatal("placement draft did not update canvas transform")
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventInput, Value: "-"})
	sync()
	if a.Preview.Items[0].Content[12] != 105 || a.Document.Instance("start").Placement.Position[0] != 30 {
		t.Fatal("incomplete input lost valid preview or modified document")
	}
	a.handleEvent(htmlui.Event{ID: x, Kind: htmlui.EventChange, Value: "120"})
	sync()
	if a.Preview.Items[0].Content[12] != 135 {
		t.Fatal("corrected input did not resume live preview")
	}
	color := workspaceFieldID(modDocumentID("button"), "/components/sprite/color", "")
	a.handleEvent(htmlui.Event{ID: color, Kind: htmlui.EventChange, Value: "#ff0000"})
	sync()
	for _, index := range []int{0, 1} {
		root := a.viewport2D.rows[index].root
		if ecs.GetRef[scene.Sprite2D](a.viewport2D.world, root).Tint.R != 1 {
			t.Fatal("shared component draft did not update every module instance")
		}
	}
	if a.Document.Mods["button"].Spec.Components.Sprite.Color != "#3264d9" {
		t.Fatal("component preview changed the authored document")
	}
	if err := a.removeField(string(modDocumentID("button")) + "|/components/sprite"); err != nil {
		t.Fatal(err)
	}
	a.changed()
	sync()
	if ecs.GetRef[scene.Sprite2D](a.viewport2D.world, a.viewport2D.rows[0].root) != nil {
		t.Fatal("staged component removal did not update canvas")
	}
	if err := a.applyDrafts(); err != nil {
		t.Fatal(err)
	}
	a.changed()
	sync()
	if a.Document.Instance("start").Placement.Position[0] != 120 || a.Document.Mods["button"].Spec.Components.Sprite != nil {
		t.Fatal("applying preview drafts lost edits")
	}
	if err := a.Document.Undo(); err != nil {
		t.Fatal(err)
	}
	a.changed()
	sync()
	if a.Preview.Items[0].Content[12] != 45 || ecs.GetRef[scene.Sprite2D](a.viewport2D.world, a.viewport2D.rows[0].root) == nil {
		t.Fatal("undo did not restore canvas")
	}
}

func TestWorkspaceDetails3DDefinitionLivePreview(t *testing.T) {
	a, context := workspace3DEditorFixture(t)
	a.enterModuleEdit(a.Document.Active, "model")
	a.selectElement("model", "parent")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	field := workspaceFieldID(modDocumentID("model"), "/elements/0/transform/position", "X")
	a.handleEvent(htmlui.Event{ID: field, Kind: htmlui.EventInput, Value: "8"})
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	row := &a.viewport3D.rows[len(a.viewport3D.rows)-1]
	if a.viewport3D.err != nil || ecs.GetRef[scene.GlobalTransform](a.viewport3D.world, row.parts[2].entity).Matrix[12] != 10 {
		t.Fatalf("3D definition draft did not update canvas: %v", a.viewport3D.err)
	}
	if a.Document.Mods["model"].Spec.Elements[0].Transform.Position[0] != 0 {
		t.Fatal("3D preview committed draft")
	}
}
