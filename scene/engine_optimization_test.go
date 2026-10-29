package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestTransformSyncSkipsStaticAndUnrelatedSubtrees(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner3[Transform, GlobalTransform, Parent]()
	root := spawner.Spawn(TransformFromXYZ(10, 0, 0), GlobalTransform{}, Parent{})
	child := spawner.Spawn(TransformFromXYZ(2, 0, 0), GlobalTransform{}, Parent{root})
	leaf := spawner.Spawn(TransformFromXYZ(3, 0, 0), GlobalTransform{}, Parent{child})
	other := spawner.Spawn(TransformFromXYZ(100, 0, 0), GlobalTransform{}, Parent{})
	sync := SyncGlobalTransformSystem("")
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	workspace := world.FrameValue[transformWorkspaceTag, transformWorkspace]()
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	if workspace.recomputed != 0 {
		t.Fatal("static transforms were recomputed")
	}
	unrelated := world.NewSpawner1[testPosition]()
	unrelated.Spawn(testPosition{})
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	if workspace.recomputed != 0 {
		t.Fatal("unrelated entity rebuilt transform hierarchy")
	}
	pose := world.GetMut[Transform](child)
	pose.Translation[0] = 5
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	if workspace.recomputed != 2 {
		t.Fatalf("recomputed=%d, want affected child and leaf", workspace.recomputed)
	}
	global, _ := world.Get[GlobalTransform](leaf)
	if global.Matrix[12] != 18 {
		t.Fatalf("descendant x=%v, want 18", global.Matrix[12])
	}
	parent := world.GetMut[Parent](child)
	parent.Entity = other
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	global, _ = world.Get[GlobalTransform](leaf)
	if global.Matrix[12] != 108 {
		t.Fatalf("reparented x=%v, want 108", global.Matrix[12])
	}
	world.Despawn(other)
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	global, _ = world.Get[GlobalTransform](leaf)
	if global.Matrix[12] != 8 {
		t.Fatalf("removed ancestor x=%v, want 8", global.Matrix[12])
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocs != 0 {
		t.Fatalf("static allocations=%v", allocs)
	}
}

func TestUITransformIncrementalInputsAndEntityReuse(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(ViewportSize{Width: 200, Height: 100, ScaleFactor: 1})
	spawner := world.NewSpawner4[UITransform, UIGlobalTransform, Parent, Size2D]()
	root := spawner.Spawn(UITransformFromXY(10, 0), UIGlobalTransform{}, Parent{}, Size2D{Width: 100, Height: 100})
	local := NewUITransform()
	local.Translation = Percent2(50, 0)
	child := spawner.Spawn(local, UIGlobalTransform{}, Parent{root}, Size2D{Width: 20, Height: 20})
	spawner.Spawn(UITransformFromXY(100, 0), UIGlobalTransform{}, Parent{}, Size2D{})
	sync := SyncUIGlobalTransformSystem("")
	run := func() {
		t.Helper()
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	workspace := world.FrameValue[uiTransformWorkspaceScratch, uiTransformWorkspace]()
	if workspace.recomputed != 0 {
		t.Fatal("static UI transforms recomputed")
	}
	size := world.GetMut[Size2D](child)
	size.Width = 40
	run()
	global, _ := world.Get[UIGlobalTransform](child)
	if global.Matrix[12] != 30 || workspace.recomputed != 1 {
		t.Fatalf("size x=%v recomputed=%d", global.Matrix[12], workspace.recomputed)
	}
	if err := world.Insert(root, ClipRect2D{Width: 50, Height: 50}); err != nil {
		t.Fatal(err)
	}
	run()
	global, _ = world.Get[UIGlobalTransform](child)
	if !global.clip.valid || workspace.recomputed != 2 {
		t.Fatal("parent clip did not reach descendant")
	}
	world.Despawn(root)
	run()
	global, _ = world.Get[UIGlobalTransform](child)
	if global.Matrix[12] != 20 || global.clip.valid {
		t.Fatal("removed ancestor retained transform or clip")
	}
	reused := spawner.Spawn(UITransformFromXY(70, 0), UIGlobalTransform{}, Parent{}, Size2D{})
	run()
	global, _ = world.Get[UIGlobalTransform](child)
	if global.Matrix[12] != 20 {
		t.Fatal("stale parent resolved to reused entity slot")
	}
	pose := world.GetMut[UITransform](reused)
	pose.Translation.X = Vw(10)
	run()
	world.InsertResource(ViewportSize{Width: 400, Height: 100, ScaleFactor: 1})
	run()
	global, _ = world.Get[UIGlobalTransform](reused)
	if global.Matrix[12] != 40 {
		t.Fatalf("viewport-relative x=%v", global.Matrix[12])
	}
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("static UI allocations=%v", allocations)
	}
}
