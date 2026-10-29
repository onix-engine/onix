package editor

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
)

func TestWorkspace2DNameAndRootTransformRetainContent(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Private child", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "child", Components: project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{10, 10}}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport2D
	root, child, slot := v.rows[0].root, v.rows[0].parts[0], v.rows[0].slots[0]
	resources := v.mods["button"].resources
	contentRevision := a.Document.Mods["button"].contentRevision
	sync := func() {
		t.Helper()
		a.syncDraftPreview()
		a.Preview.Sync(a.previewDocument(), a.Local)
		if err := a.syncViewport2D(); err != nil {
			t.Fatal(err)
		}
	}
	a.fields["name"] = workspaceField{Target: "button/mod.json", Pointer: "/name", Kind: "string", Axis: -1, Value: "Button"}
	for _, name := range []string{"Renamed", "Again"} {
		a.drafts["name"] = name
		sync()
		if a.previewDocument().Mods["button"].Spec.Name != name || a.previewDocument().Mods["button"].contentRevision != contentRevision {
			t.Fatal("name draft did not update metadata independently of preview content")
		}
	}
	a.fields["position"] = workspaceField{Target: "button/mod.json", Pointer: "/transform/position/0", Kind: "number", Axis: -1, Value: "5"}
	a.drafts["position"] = "45"
	sync()
	assertRetained := func() {
		t.Helper()
		if v.rows[0].root != root || v.rows[0].parts[0] != child || v.rows[0].slots[0] != slot || v.mods["button"].resources != resources {
			t.Fatal("name/root transform edit rebuilt private entities or resources")
		}
	}
	assertRetained()
	if transform, _ := v.world.Get[scene.UIGlobalTransform](child); transform.Matrix[12] != 85 {
		t.Fatalf("retained child did not follow module transform: %v", transform.Matrix[12])
	}
	// Alternating validated transforms exercises the render update independently
	// of form parsing and compilation. Warm retained rendering must allocate zero.
	documents := [2]*WorkspaceDocument{a.Document, a.previewDocument()}
	next := 0
	step := func() {
		next ^= 1
		a.draftPreview.document = documents[next]
		a.Preview.Sync(a.previewDocument(), a.Local)
		if err := a.syncViewport2D(); err != nil {
			panic(err)
		}
	}
	for range 4 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("retained 2D transform update allocated %g", allocations)
	}
	a.draftPreview.document = documents[1]
	if err := a.applyDrafts(); err != nil {
		t.Fatal(err)
	}
	sync()
	assertRetained()
	if err := a.Document.Undo(); err != nil {
		t.Fatal(err)
	}
	sync()
	assertRetained()
	if transform, _ := v.world.Get[scene.UIGlobalTransform](child); transform.Matrix[12] != 45 {
		t.Fatal("undo did not restore the retained module transform")
	}
	a.fields["color"] = workspaceField{Target: "button/mod.json", Pointer: "/components/sprite/color", Kind: "string", Axis: -1, Value: "#3264d9"}
	a.drafts["color"] = "#ff0000"
	sync()
	if v.rows[0].root == root || v.mods["button"].resources == resources {
		t.Fatal("changed visual content reused stale entities/resources")
	}
	if a.Document.Mods["button"].Spec.Components.Sprite.Color != "#3264d9" {
		t.Fatal("preview edit mutated committed component data")
	}
}

func TestWorkspace2DRenamePreservesAnimationState(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Animation", func(mod *project.ModSpec) error {
		mod.Components.Animation = &project.AnimationComponentSpec{Clip: project.AnimationClipSpec{
			Duration: 1, Loop: true,
			FloatTracks: []project.AnimationTrackSpec[float32]{{Binding: scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX}, Keyframes: []project.AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 100}}}},
		}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	v := &a.viewport2D
	clock, _ := v.world.GetResource[*scene.Time]()
	clock.Set(500*time.Millisecond, 500*time.Millisecond, 1)
	if err := v.syncVisuals(true); err != nil {
		t.Fatal(err)
	}
	root := v.rows[0].root
	before, _ := v.world.Get[scene.Animator](root)
	transform, _ := v.world.Get[scene.UITransform](root)
	if err := a.Document.EditMod("button", "Rename", func(mod *project.ModSpec) error { mod.Name = "Renamed"; return nil }); err != nil {
		t.Fatal(err)
	}
	a.changed()
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	after, _ := v.world.Get[scene.Animator](root)
	current, _ := v.world.Get[scene.UITransform](root)
	if v.rows[0].root != root || before.Clip != after.Clip || transform != current {
		t.Fatal("renaming reset the animator or its sampled transform")
	}
}

func TestWorkspace3DNameAndRootTransformRetainGeometryAndCamera(t *testing.T) {
	a, _ := workspace3DEditorFixture(t)
	v := &a.viewport3D
	row := &v.rows[len(v.rows)-1]
	root, child := row.parts[0].entity, row.parts[2].entity
	mesh, _ := v.world.Get[render3d.Mesh](root)
	resources := v.mods["model"].resources
	if _, _, err := a.viewport3DEnvironmentSource(); err != nil {
		t.Fatal(err)
	}
	camera := row.parts[0].cameraSpec
	sync := func() {
		t.Helper()
		a.syncDraftPreview()
		a.Preview.Sync(a.previewDocument(), a.Local)
		if err := a.syncViewport3D(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := a.viewport3DEnvironmentSource(); err != nil {
			t.Fatal(err)
		}
	}
	a.fields["name"] = workspaceField{Target: "model/mod.json", Pointer: "/name", Kind: "string", Axis: -1}
	a.fields["position"] = workspaceField{Target: "model/mod.json", Pointer: "/transform/position/0", Kind: "number", Axis: -1, Value: "2"}
	for _, position := range []string{"6", "8", "10"} {
		a.drafts["name"], a.drafts["position"] = "Renamed", position
		sync()
		current, _ := v.world.Get[render3d.Mesh](root)
		if row.parts[0].entity != root || row.parts[2].entity != child || current.Mesh != mesh.Mesh || v.mods["model"].resources != resources {
			t.Fatal("name/root transform edit recreated model geometry")
		}
		if row.parts[0].cameraSpec != camera || len(v.mods["model"].cameras) != 1 {
			t.Fatal("equivalent camera specs accumulated in the environment cache")
		}
	}
	if transform, _ := v.world.Get[scene.GlobalTransform](child); transform.Matrix[12] != 20 {
		t.Fatal("retained model did not follow the updated module transform")
	}
	documents := [2]*WorkspaceDocument{a.Document, a.previewDocument()}
	next := 0
	step := func() {
		next ^= 1
		a.draftPreview.document = documents[next]
		a.Preview.Sync(a.previewDocument(), a.Local)
		if err := a.syncViewport3D(); err != nil {
			panic(err)
		}
		if _, _, err := a.viewport3DEnvironmentSource(); err != nil {
			panic(err)
		}
	}
	for range 4 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("retained 3D transform/environment update allocated %g", allocations)
	}
	a.draftPreview.document = documents[1]
	a.fields["size"] = workspaceField{Target: "model/mod.json", Pointer: "/components/model/mesh/size", Kind: "number", Axis: -1, Value: "2"}
	a.drafts["size"] = "4"
	sync()
	if row.parts[0].entity == root || v.mods["model"].resources == resources {
		t.Fatal("geometry change reused an outdated resource store")
	}
	current, _ := v.world.Get[render3d.Mesh](row.parts[0].entity)
	if current.Mesh == mesh.Mesh || current.Mesh.Bounds.Radius <= mesh.Mesh.Bounds.Radius {
		t.Fatal("geometry change did not rebuild the model")
	}
}
