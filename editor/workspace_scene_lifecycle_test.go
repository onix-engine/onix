package editor

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestWorkspaceSceneLifecycleEditsPersistPerScene(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateScene("game.json"); err != nil {
		t.Fatal(err)
	}
	a.handleEvent(htmlui.Event{ID: "scene:main.json", Kind: htmlui.EventClick})
	source := workspaceShellTest(t, a)
	id := workspaceFieldID(sceneDocumentID("main.json"), "/exitPolicy", "")
	if !strings.Contains(source, `id="scene-lifecycle-title"`) || a.fields[id].Target != sceneDocumentID("main.json") {
		t.Fatal("selecting a scene did not show its lifecycle settings")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Value: "retain"})
	a.action("save")
	if a.Document.Scenes["main.json"].Spec.ExitPolicy != project.SceneExitRetain || a.Document.Dirty() {
		t.Fatalf("scene lifecycle edit did not save: %s", a.status)
	}
	a.action("undo")
	if a.Document.Scenes["main.json"].Spec.ExitPolicy != "" {
		t.Fatal("undo did not restore the previous policy")
	}
	a.action("redo")
	if a.Document.Scenes["main.json"].Spec.ExitPolicy != project.SceneExitRetain {
		t.Fatal("redo did not restore the retained policy")
	}
	a.handleEvent(htmlui.Event{ID: "scene:game.json", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	id = workspaceFieldID(sceneDocumentID("game.json"), "/exitPolicy", "")
	if a.Document.Active != "game.json" || a.controlValues[id] != "" {
		t.Fatal("another scene inherited the selected scene's policy")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventChange, Value: "destroy"})
	a.action("save")
	reloaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reloaded.Dispose() })
	if reloaded.Scenes["main.json"].Spec.ExitPolicy != project.SceneExitRetain || reloaded.Scenes["game.json"].Spec.ExitPolicy != project.SceneExitDestroy {
		t.Fatal("scene lifecycle policies did not survive save and reload")
	}
	a.handleEvent(htmlui.Event{ID: "scene:main.json", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	id = workspaceFieldID(sceneDocumentID("main.json"), "/exitPolicy", "")
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if _, ok := a.fields[id]; ok {
		t.Fatal("module selection exposed scene lifecycle settings")
	}
}
