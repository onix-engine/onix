package editor

import (
	"math"
	"testing"

	"github.com/diiyw/onix/project"
)

func TestWorkspacePreviewCoordinateIsolationAndAffectedInstances(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.MoveInstance("again", "start", 0); err != nil {
		t.Fatal(err)
	}
	var p DataPreview
	p.Sync(w, "")
	start, child, other := p.Items[0], p.Items[1], p.Items[2]
	if start.Content[12] != 45 || start.Content[13] != 67 {
		t.Fatalf("scene + placement + local = %v", start.Content)
	}
	if child.Content[12] != 245 || child.Content[13] != 167 {
		t.Fatalf("child inherited parent's module-local coordinates: %v", child.Content)
	}
	if err := w.EditMod("button", "Change local", func(mod *project.ModSpec) error { mod.Transform.Position = []float32{50, 60}; return nil }); err != nil {
		t.Fatal(err)
	}
	p.Sync(w, "")
	if p.Items[0].Content[12] != 90 || p.Items[1].Content[12] != 290 || p.Items[0].Revision == start.Revision || p.Items[1].Revision == child.Revision || p.Items[2] != other {
		t.Fatal("definition update missed reused instances or updated unrelated preview")
	}
	if p.Items[1].Placement != child.Placement {
		t.Fatal("definition moved scene-instance hierarchy")
	}
	child = p.Items[1]
	if err := w.EditInstance("again", "Move child", func(instance *project.ModInstanceSpec) error {
		instance.Placement.Position = []float32{300, 300}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	start = p.Items[0]
	p.Sync(w, "")
	if p.Items[0] != start || p.Items[1].Revision == child.Revision || p.Items[2] != other {
		t.Fatal("placement affected the wrong preview instances")
	}
	p.Sync(w, "button")
	if len(p.Items) != 1 || p.Items[0].Content[12] != 50 || p.Items[0].Content[13] != 60 || p.Items[0].Placement[12] != 0 {
		t.Fatal("local preview inherited scene coordinates")
	}
	allocations := testing.AllocsPerRun(100, func() { p.Sync(w, "button") })
	if allocations != 0 {
		t.Fatalf("stable data preview allocated %g", allocations)
	}
}

func TestWorkspacePreviewVisibilityOrderingAndUIRotation(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.MoveInstance("again", "start", 0); err != nil {
		t.Fatal(err)
	}
	if err := w.EditInstance("start", "Hide parent", func(instance *project.ModInstanceSpec) error {
		visible := false
		instance.Visible = &visible
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var p DataPreview
	p.Sync(w, "")
	if p.Items[0].Visible || p.Items[1].Visible || !p.Items[2].Visible {
		t.Fatal("visibility did not follow placement ancestors")
	}
	structure := p.structure
	if err := w.MoveInstance("other", "", 0); err != nil {
		t.Fatal(err)
	}
	p.Sync(w, "")
	if p.Items[0].ID != "other" || p.structure == structure {
		t.Fatal("array ordering did not invalidate draw structure")
	}
	transform := project.ModTransform{Position: [3]float32{3, 4, 100}, Rotation: [3]float32{1, 2, float32(math.Pi / 2)}, Scale: [3]float32{2, 3, 10}}
	matrix := previewTransformMatrix(transform, project.CoordinateSpaceUI)
	if math.Abs(float64(matrix[1]-2)) > 1e-5 || math.Abs(float64(matrix[4]+3)) > 1e-5 || matrix[14] != 0 || matrix[10] != 1 {
		t.Fatal("UI preview used world-only transform axes")
	}
}
