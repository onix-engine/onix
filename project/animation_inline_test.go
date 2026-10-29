package project

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
)

func TestInlineAnimationBindsDescendantsAndTweens(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [{
    "id": "actor",
    "components": {"animation": {"clip": {
      "duration": 1,
      "floatTracks": [
        {"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0,"tween":"easeIn"},{"time":1,"value":10}]},
        {"binding":{"target":"transform.rotation.x"},"keyframes":[{"time":0,"value":0.1},{"time":1,"value":0.3}]},
        {"binding":{"target":"transform.rotation.y"},"keyframes":[{"time":0,"value":0.2},{"time":1,"value":0.4}]},
        {"binding":{"target":"transform.rotation.z"},"keyframes":[{"time":0,"value":0.3},{"time":1,"value":0.5}]},
        {"binding":{"path":"arm/hand","target":"transform.position.z"},"keyframes":[{"time":0,"value":2},{"time":1,"value":12}]},
        {"binding":{"path":"arm","target":"transform.scale.x"},"keyframes":[{"time":0,"value":1},{"time":1,"value":3}]}
      ],
      "boolTracks": [{"binding":{"path":"arm/hand","target":"visibility.visible"},"keyframes":[{"time":0,"value":true},{"time":0.5,"value":false}]}]
    }}},
    "children": [{"id":"arm","children":[{"id":"hand"}]}]
  }]
}`)

	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	animator := world.GetMut[sceneapi.Animator](scene.MustEntity("actor"))
	if animator == nil || !animator.IsBound() {
		t.Fatal("inline animator was not bound")
	}
	animator.Seek(.5)
	animator.Pause()
	if err := sceneapi.UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	root, _ := world.Get[sceneapi.Transform](scene.MustEntity("actor"))
	if root.Translation.X() != 2.5 {
		t.Fatalf("ease-in position x = %v, want 2.5", root.Translation.X())
	}
	hand, _ := world.Get[sceneapi.Transform](scene.MustEntity("hand"))
	if hand.Translation.Z() != 7 {
		t.Fatalf("descendant position z = %v, want 7", hand.Translation.Z())
	}
	arm, _ := world.Get[sceneapi.Transform](scene.MustEntity("arm"))
	if arm.Scale.X() != 2 {
		t.Fatalf("descendant scale x = %v, want 2", arm.Scale.X())
	}
	visible, _ := world.Get[sceneapi.Visibility](scene.MustEntity("hand"))
	if visible.Visible {
		t.Fatal("descendant visibility should be false at the midpoint")
	}
	if world.Has[sceneapi.Sprite2D](scene.MustEntity("actor")) {
		t.Fatal("child-only clip should not add a root Sprite2D")
	}
}

func TestAnimationComponentRejectsResourceField(t *testing.T) {
	var component AnimationComponentSpec
	if err := json.Unmarshal([]byte(`{"resource":"legacy"}`), &component); err == nil {
		t.Fatal("named animation resource fields should not decode")
	}
}

func TestBundleDisposeClearsInlineAnimationClipCache(t *testing.T) {
	cache := newResourceCache[string, *sceneapi.AnimationClip](1)
	if err := cache.replace("inline", &sceneapi.AnimationClip{Duration: 1}, nil); err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{inlineAnimationClips: cache}
	if err := bundle.Dispose(); err != nil {
		t.Fatal(err)
	}
	if bundle.inlineAnimationClips.len() != 0 {
		t.Fatalf("inline animation cache retained %d clips after dispose", bundle.inlineAnimationClips.len())
	}
}

func TestPrefabInlineAnimationSharesClipAndIsolatesPlayback(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{"prefabs":[{"id":"pulse","path":"resources/pulse.prefab.json"}]}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"host"}]}`)
	writeTestFile(t, dir, "resources/pulse.prefab.json", `{"root":{"id":"pulse","components":{"animation":{"clip":{"duration":1,"loop":true,"floatTracks":[{"binding":{"target":"transform.position.x"},"keyframes":[{"time":0,"value":0},{"time":1,"value":10}]}]}}}}}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	first, err := scene.MountPrefab(scene.MustEntity("host"), "pulse", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := scene.MountPrefab(scene.MustEntity("host"), "pulse", "second")
	if err != nil {
		t.Fatal(err)
	}
	firstEntity, _ := first.Entity("pulse")
	secondEntity, _ := second.Entity("pulse")
	firstAnimator := world.GetMut[sceneapi.Animator](firstEntity)
	secondAnimator := world.GetMut[sceneapi.Animator](secondEntity)
	if firstAnimator.Clip == nil || firstAnimator.Clip != secondAnimator.Clip {
		t.Fatal("prefab instances did not share immutable clip")
	}
	firstAnimator.Seek(.25)
	firstAnimator.Pause()
	secondAnimator.Seek(.75)
	secondAnimator.Pause()
	if err := sceneapi.UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	firstTransform, _ := world.Get[sceneapi.Transform](firstEntity)
	secondTransform, _ := world.Get[sceneapi.Transform](secondEntity)
	if firstTransform.Translation.X() != 2.5 || secondTransform.Translation.X() != 7.5 {
		t.Fatalf("prefab playback = %v/%v, want 2.5/7.5", firstTransform.Translation.X(), secondTransform.Translation.X())
	}
}

func TestInlineAnimationRejectsAnimatorOwnershipConflict(t *testing.T) {
	animation := &AnimationComponentSpec{Clip: AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{
			{
				Binding:   sceneapi.AnimationBinding{Path: "child", Target: sceneapi.AnimationTargetTransformPositionX},
				Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}},
			},
		},
	}}
	_, err := compileSceneSpawnPlan([]SceneNodeSpec{{
		ID: "root", Components: SceneComponents{Animation: animation},
		Children: []SceneNodeSpec{{ID: "child", Components: SceneComponents{Animation: animation}}},
	}})
	if err == nil {
		t.Fatal("expected parent/child animator ownership conflict")
	}
}

func TestInlineAnimationRejectsSplitRotationOwnership(t *testing.T) {
	clip := func(path string, target sceneapi.AnimationTarget) *AnimationComponentSpec {
		return &AnimationComponentSpec{Clip: AnimationClipSpec{
			Duration: 1,
			FloatTracks: []AnimationTrackSpec[float32]{{
				Binding:   sceneapi.AnimationBinding{Path: path, Target: target},
				Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}},
			}},
		}}
	}
	_, err := compileSceneSpawnPlan([]SceneNodeSpec{{
		ID:         "root",
		Components: SceneComponents{Animation: clip("child", sceneapi.AnimationTargetTransformRotationX)},
		Children: []SceneNodeSpec{{
			ID:         "child",
			Components: SceneComponents{Animation: clip("", sceneapi.AnimationTargetTransformRotationY)},
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "already animated") {
		t.Fatalf("split rotation ownership error = %v", err)
	}
}

func TestInlineAnimationsAllowIndependentParentAndChildProperties(t *testing.T) {
	parentClip := AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   sceneapi.AnimationBinding{Path: "child", Target: sceneapi.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 10}},
		}},
	}
	childClip := AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   sceneapi.AnimationBinding{Target: sceneapi.AnimationTargetTransformScaleX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}, {Time: 1, Value: 2}},
		}},
	}
	plan, err := compileSceneSpawnPlan([]SceneNodeSpec{{
		ID: "root", Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: parentClip}},
		Children: []SceneNodeSpec{{ID: "child", Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: childClip}}}},
	}})
	if err != nil {
		t.Fatalf("independent parent/child properties were rejected: %v", err)
	}
	if len(plan.Nodes) != 2 || len(plan.Nodes[0].AnimationFloatTargets) != 1 || len(plan.Nodes[1].AnimationFloatTargets) != 1 {
		t.Fatalf("animation target plan = %+v", plan.Nodes)
	}
}

func TestInlineAlphaAnimationAllowsRootHostButRequiresDescendantSprite(t *testing.T) {
	alpha := AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   sceneapi.AnimationBinding{Target: sceneapi.AnimationTargetSpriteAlpha},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}},
		}},
	}
	if _, err := compileSceneSpawnPlan([]SceneNodeSpec{{
		ID: "actor", Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: alpha}},
	}}); err != nil {
		t.Fatalf("root alpha animation was rejected: %v", err)
	}

	alpha.FloatTracks[0].Binding.Path = "child"
	if _, err := compileSceneSpawnPlan([]SceneNodeSpec{{
		ID:         "actor",
		Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: alpha}},
		Children:   []SceneNodeSpec{{ID: "child"}},
	}}); err == nil || !strings.Contains(err.Error(), "without a sprite component") {
		t.Fatalf("descendant alpha animation error = %v", err)
	}
}

func TestDynamicAnimationRejectsTargetOwnershipConflict(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	clip := func(path string, target sceneapi.AnimationTarget) AnimationClipSpec {
		return AnimationClipSpec{
			Duration: 1,
			FloatTracks: []AnimationTrackSpec[float32]{{
				Binding:   sceneapi.AnimationBinding{Path: path, Target: target},
				Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}},
			}},
		}
	}
	root := SceneNodeSpec{
		ID:         "owner",
		Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: clip("child", sceneapi.AnimationTargetTransformRotationX)}},
		Children: []SceneNodeSpec{{
			ID:         "child",
			Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: clip("", sceneapi.AnimationTargetTransformRotationY)}},
		}},
	}
	if err := scene.ApplyNode(root); err == nil || !strings.Contains(err.Error(), "already animated") {
		t.Fatalf("dynamic ownership conflict error = %v", err)
	}
}

func TestRuntimeAnimationRejectsDuplicateTargetWithinOwner(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner1[sceneapi.Transform]()
	entity := spawner.Spawn(sceneapi.NewTransform())
	scene := &SceneInstance{World: world, EntityIDs: map[ecs.Entity]string{entity: "target"}}
	claims := make(map[runtimeAnimationClaim]ecs.Entity)
	seen := make(map[runtimeAnimationSeen]struct{})
	if err := claimRuntimeAnimationBinding(scene, claims, seen, entity, entity, sceneapi.AnimationTargetTransformPositionX); err != nil {
		t.Fatal(err)
	}
	if err := claimRuntimeAnimationBinding(scene, claims, seen, entity, entity, sceneapi.AnimationTargetTransformPositionX); err == nil || !strings.Contains(err.Error(), "bound more than once") {
		t.Fatalf("duplicate target error = %v", err)
	}
}

func TestRuntimeAnimationAllowsMultipleRotationAxesWithinOwner(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	clip := AnimationClipSpec{Duration: 1, FloatTracks: []AnimationTrackSpec[float32]{
		{Binding: sceneapi.AnimationBinding{Target: sceneapi.AnimationTargetTransformRotationX}, Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: .1}}},
		{Binding: sceneapi.AnimationBinding{Target: sceneapi.AnimationTargetTransformRotationY}, Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: .2}}},
		{Binding: sceneapi.AnimationBinding{Target: sceneapi.AnimationTargetTransformRotationZ}, Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: .3}}},
	}}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "actor", Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: clip}}}); err != nil {
		t.Fatalf("multiple rotation axes were rejected: %v", err)
	}
}

func TestApplyNodeRollsBackFailedAnimationBinding(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	before := world.EntityCount()
	err = scene.ApplyNode(SceneNodeSpec{
		ID: "broken",
		Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: AnimationClipSpec{
			Duration: 1,
			FloatTracks: []AnimationTrackSpec[float32]{
				{
					Binding:   sceneapi.AnimationBinding{Path: "missing", Target: sceneapi.AnimationTargetTransformPositionX},
					Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}},
				},
			},
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not name a descendant") {
		t.Fatalf("failed animation binding error = %v", err)
	}
	if world.EntityCount() != before {
		t.Fatalf("failed ApplyNode leaked %d entities", world.EntityCount()-before)
	}
	if _, exists := scene.Entities["broken"]; exists {
		t.Fatal("failed ApplyNode left an entity index")
	}
	if len(scene.Roots) != 0 {
		t.Fatalf("failed ApplyNode left roots: %v", scene.Roots)
	}
}

func TestApplyNodePreservesExistingNodeOnAnimationBindingFailure(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	valid := SceneNodeSpec{ID: "actor", Components: SceneComponents{Sprite: &SpriteComponentSpec{Texture: &TextureSpec{Color: "#ffffff"}}}}
	if err := scene.ApplyNode(valid); err != nil {
		t.Fatal(err)
	}
	entity := scene.MustEntity("actor")
	bad := valid
	bad.Components = SceneComponents{Animation: &AnimationComponentSpec{Clip: AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   sceneapi.AnimationBinding{Path: "missing", Target: sceneapi.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}},
		}},
	}}}
	if err := scene.ApplyNode(bad); err == nil || !strings.Contains(err.Error(), "does not name a descendant") {
		t.Fatalf("failed animation update error = %v", err)
	}
	if !scene.World.Has[sceneapi.Sprite2D](entity) || scene.World.Has[sceneapi.Animator](entity) {
		t.Fatal("failed animation update changed the existing node composition")
	}
	if got := scene.ID(entity); got != "actor" {
		t.Fatalf("failed animation update changed entity id to %q", got)
	}
}

func TestApplyNodePreservesControlWhenAnimationUpdateFails(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	button := SceneNodeSpec{ID: "button", CoordinateSpace: CoordinateSpaceUI, Width: 80, Height: 24, Control: &ControlSpec{Text: "Keep"}}
	if err := scene.ApplyNode(button); err != nil {
		t.Fatal(err)
	}
	if err := scene.BindControl("button", engineui.Button{}); err != nil {
		t.Fatal(err)
	}
	bad := SceneNodeSpec{ID: "button", CoordinateSpace: CoordinateSpaceUI, Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   sceneapi.AnimationBinding{Path: "missing", Target: sceneapi.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}},
		}},
	}}}}
	if err := scene.ApplyNode(bad); err == nil || !strings.Contains(err.Error(), "does not name a descendant") {
		t.Fatalf("failed control animation update error = %v", err)
	}
	if !scene.World.Has[engineui.Control](scene.MustEntity("button")) {
		t.Fatal("failed animation update removed the existing control")
	}
}

func TestApplyNodeRevalidatesAncestorAnimationTargets(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes":[{
    "id":"owner",
    "components":{"animation":{"clip":{"duration":1,"floatTracks":[{
      "binding":{"path":"child","target":"sprite.alpha"},
      "keyframes":[{"time":0,"value":1}]
    }]}}},
    "children":[{"id":"child","components":{"sprite":{"texture":{"color":"#ffffff"}}}}]
  }]
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()

	if err := scene.ApplyNode(SceneNodeSpec{ID: "child"}); err == nil || !strings.Contains(err.Error(), "requires Sprite2D") {
		t.Fatalf("ancestor target revalidation error = %v", err)
	}
}

func TestRuntimeAnimationTargetValidationEnumeratesFloatTargets(t *testing.T) {
	world := ecs.NewWorld()
	transformSpawner := world.NewSpawner1[sceneapi.Transform]()
	transformEntity := transformSpawner.Spawn(sceneapi.NewTransform())
	if err := validateRuntimeAnimationTarget(world, transformEntity, sceneapi.AnimationTargetTransformScaleX, animationTrackFloatTarget); err != nil {
		t.Fatalf("scale X on Transform was rejected: %v", err)
	}
	if err := validateRuntimeAnimationTarget(world, transformEntity, sceneapi.AnimationTargetVisibility, animationTrackFloatTarget); err == nil {
		t.Fatal("visibility target was accepted as a float track")
	}
	uiSpawner := world.NewSpawner1[sceneapi.UITransform]()
	uiEntity := uiSpawner.Spawn(sceneapi.NewUITransform())
	if err := validateRuntimeAnimationTarget(world, uiEntity, sceneapi.AnimationTargetTransformScaleY, animationTrackFloatTarget); err != nil {
		t.Fatalf("scale Y on UITransform was rejected: %v", err)
	}
	if err := validateRuntimeAnimationTarget(world, uiEntity, sceneapi.AnimationTargetTransformScaleZ, animationTrackFloatTarget); err == nil {
		t.Fatal("scale Z on UITransform was accepted")
	}
}

func TestEmptyAnimationComponentCompilesForSceneAndDynamicNode(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"authored","x":7,"components":{"animation":{}}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	assertEmptyAnimation := func(entity ecs.Entity, wantX float32) {
		t.Helper()
		animator := world.GetMut[sceneapi.Animator](entity)
		ok := animator != nil
		if !ok || animator == nil || !animator.IsBound() || animator.Clip == nil || animator.Clip.Duration != 1 || len(animator.Clip.FloatTracks) != 1 {
			t.Fatalf("empty animation = ok:%v animator:%+v", ok, animator)
		}
		if animator.Clip.FloatTracks[0].Binding.Target != sceneapi.AnimationTargetTransformPositionX {
			t.Fatalf("default target = %+v", animator.Clip.FloatTracks[0].Binding)
		}
		transform, ok := world.Get[sceneapi.Transform](entity)
		if !ok || transform.Translation.X() != wantX {
			t.Fatalf("authored transform = %+v, want x=%v", transform, wantX)
		}
	}
	assertEmptyAnimation(scene.MustEntity("authored"), 7)

	if err := scene.ApplyNode(SceneNodeSpec{ID: "dynamic", X: 11, Components: SceneComponents{Animation: &AnimationComponentSpec{}}}); err != nil {
		t.Fatal(err)
	}
	assertEmptyAnimation(scene.MustEntity("dynamic"), 11)
}

func TestEmptyAnimationComponentCompilesForMountedPrefab(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{"prefabs":[{"id":"pulse","path":"resources/pulse.prefab.json"}]}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"host"}]}`)
	writeTestFile(t, dir, "resources/pulse.prefab.json", `{"root":{"id":"pulse","x":5,"components":{"animation":{}}}}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	first, err := scene.MountPrefab(scene.MustEntity("host"), "pulse", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := scene.MountPrefab(scene.MustEntity("host"), "pulse", "second")
	if err != nil {
		t.Fatal(err)
	}
	firstEntity, _ := first.Entity("pulse")
	secondEntity, _ := second.Entity("pulse")
	firstAnimator := world.GetMut[sceneapi.Animator](firstEntity)
	ok := firstAnimator != nil
	if !ok || firstAnimator == nil || !firstAnimator.IsBound() || firstAnimator.Clip == nil {
		t.Fatal("mounted prefab did not initialize a bound empty animation")
	}
	secondAnimator := world.GetMut[sceneapi.Animator](secondEntity)
	ok = secondAnimator != nil
	if !ok || secondAnimator == nil || firstAnimator.Clip != secondAnimator.Clip {
		t.Fatal("mounted prefab instances did not share the compiled default clip")
	}
}

func TestReplaceNodesBindsInlineAnimationWithoutBundleCacheKey(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	root := SceneNodeSpec{
		ID: "actor",
		Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: AnimationClipSpec{
			Duration: 1,
			FloatTracks: []AnimationTrackSpec[float32]{{
				Binding:   sceneapi.AnimationBinding{Path: "child", Target: sceneapi.AnimationTargetTransformPositionX},
				Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 8}},
			}}},
		}},
		Children: []SceneNodeSpec{{ID: "child"}},
	}
	if err := scene.ReplaceNodes([]SceneNodeSpec{root}); err != nil {
		t.Fatal(err)
	}
	animator := world.GetMut[sceneapi.Animator](scene.MustEntity("actor"))
	if animator == nil || !animator.IsBound() {
		t.Fatal("ReplaceNodes did not bind inline animator")
	}
	animator.Seek(.5)
	animator.Pause()
	if err := sceneapi.UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	child, _ := world.Get[sceneapi.Transform](scene.MustEntity("child"))
	if child.Translation.X() != 4 {
		t.Fatalf("replaced child position = %v, want 4", child.Translation.X())
	}
}

func TestInlineRootTextureTrackCanMaterializeSpriteHost(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{
  "nodes": [{"id":"actor","components":{"animation":{"clip":{
    "duration":1,
    "textureTracks":[{"binding":{"target":"sprite.texture"},"keyframes":[{"time":0,"value":{"color":"#ff0000"}}]}]
  }}}}]
}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	if !world.Has[sceneapi.Sprite2D](scene.MustEntity("actor")) {
		t.Fatal("root texture animation did not materialize Sprite2D")
	}
}

func TestClonedInlineAnimationResolvesLocalChildPath(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"host"},{"id":"template","components":{"animation":{"clip":{"duration":1,"floatTracks":[{"binding":{"path":"hand","target":"transform.position.x"},"keyframes":[{"time":0,"value":0},{"time":1,"value":6}]}]}}},"children":[{"id":"hand"}]}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	clock := sceneapi.NewTime()
	world.InsertResource(clock)
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	clones, err := scene.CloneNodeUnder(scene.MustEntity("host"), "template", "copy")
	if err != nil {
		t.Fatal(err)
	}
	cloneRoot := clones["copy-template"]
	animator := world.GetMut[sceneapi.Animator](cloneRoot)
	if animator == nil || !animator.IsBound() {
		t.Fatal("cloned inline animator was not bound")
	}
	animator.Seek(.5)
	animator.Pause()
	if err := sceneapi.UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	child, _ := world.Get[sceneapi.Transform](clones["copy-hand"])
	if child.Translation.X() != 3 {
		t.Fatalf("cloned child position = %v, want 3", child.Translation.X())
	}
}

func TestCloneNodeRollsBackFailedAnimationBinding(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	world := ecs.NewWorld()
	scene, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	if err := scene.ApplyNode(SceneNodeSpec{ID: "host"}); err != nil {
		t.Fatal(err)
	}
	if err := scene.ApplyNode(SceneNodeSpec{ID: "template", Components: SceneComponents{Animation: &AnimationComponentSpec{Clip: AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   sceneapi.AnimationBinding{Target: sceneapi.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}},
		}},
	}}}}); err != nil {
		t.Fatal(err)
	}
	templateEntity := scene.MustEntity("template")
	templateNode := scene.nodes[templateEntity]
	templateNode.Spec.Components.Animation.Clip.FloatTracks[0].Binding.Path = "missing"
	scene.nodes[templateEntity] = templateNode
	before := world.EntityCount()
	if _, err := scene.CloneNodeUnder(scene.MustEntity("host"), "template", "copy"); err == nil || !strings.Contains(err.Error(), "does not name a descendant") {
		t.Fatalf("failed clone animation binding error = %v", err)
	}
	if world.EntityCount() != before {
		t.Fatalf("failed clone leaked %d entities", world.EntityCount()-before)
	}
	for id := range scene.Entities {
		if strings.HasPrefix(id, "copy-") {
			t.Fatalf("failed clone left entity %q", id)
		}
	}
}
