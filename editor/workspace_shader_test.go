package editor

import (
	"slices"
	"testing"

	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

func TestWorkspaceShaderComponentOffersElementSourcePickers(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Shader element", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "effect", Components: project.ModComponents{Shader: &project.ShaderComponentSpec{}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	workspaceShellTest(t, a)
	target := modDocumentID("button")
	for _, field := range a.fields {
		if field.Target == target && field.Pointer == "/components/shader" {
			t.Fatal("Shader is offered on the module root")
		}
	}
	a.selectElement("start", "effect")
	workspaceShellTest(t, a)
	base := "/elements/0/components/shader"
	for _, path := range []string{"wgslVertex", "wgslFragment"} {
		if _, exists := a.fields["content-resource:"+workspaceFieldID(target, base+"/"+path, "")]; !exists {
			t.Fatalf("shader %s has no source picker", path)
		}
	}
	for _, path := range []string{"id", "webGL2Generated", "vulkanVertex", "vulkanFragment", "metal", "webGL2Vertex", "webGL2Fragment", "webGPUVertex", "webGPUFragment", "Compiled"} {
		if _, exists := a.fields[workspaceFieldID(target, base+"/"+path, "")]; exists {
			t.Fatal("internal shader metadata is exposed in the inspector", path)
		}
	}
	file := a.Document.Absolute("button/resources/effect.wgsl")
	if err := writeWorkspaceFile(file, []byte("shader source")); err != nil {
		t.Fatal(err)
	}
	var options window.FileDialogOptions
	a.openNativeFile = func(value window.FileDialogOptions) (string, error) { options = value; return file, nil }
	id := workspaceFieldID(target, base+"/wgslFragment", "")
	a.handleEvent(htmlui.Event{ID: "content-resource:" + id, Kind: htmlui.EventClick})
	if a.drafts[id] != "resources/effect.wgsl" || options.Filters[0].Name != "Shader" || !slices.Equal(options.Filters[0].Extensions, []string{"wgsl"}) {
		t.Fatalf("shader picker authored %q with %+v", a.drafts[id], options)
	}
	a.action("save")
	if a.hasDrafts() || a.Document.Mods["button"].Spec.Elements[0].Components.Shader.WGSLFragment != "resources/effect.wgsl" {
		t.Fatal("shader source did not save on the selected element", a.status)
	}
}
