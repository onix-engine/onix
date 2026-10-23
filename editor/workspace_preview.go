package editor

import (
	"slices"

	"github.com/diiyw/onix/project"
	"github.com/go-gl/mathgl/mgl32"
)

// PreviewItem is pure authoring data. No module registry, events, requests or Go
// behavior is installed by the editor. Placement ancestors exclude mod locals.
type PreviewItem struct {
	ID              string
	Mod             project.ModID
	Parent          int32
	Placement       mgl32.Mat4
	Content         mgl32.Mat4
	Visible         bool
	Enabled         bool
	Revision        uint64
	ContentRevision uint64
}

type DataPreview struct {
	Scene          project.SceneID
	Local          project.ModID
	Items          []PreviewItem
	Revision       uint64
	sourceRevision uint64
	source         *WorkspaceDocument
	structure      uint64
}

func previewTransformMatrix(t project.ModTransform, space project.CoordinateSpace) mgl32.Mat4 {
	if space != project.CoordinateSpaceWorld {
		return mgl32.Translate3D(t.Position[0], t.Position[1], 0).Mul4(mgl32.HomogRotate3DZ(t.Rotation[2])).Mul4(mgl32.Scale3D(t.Scale[0], t.Scale[1], 1))
	}
	rotation := mgl32.QuatRotate(t.Rotation[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(t.Rotation[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(t.Rotation[0], mgl32.Vec3{1, 0, 0}))
	return mgl32.Translate3D(t.Position[0], t.Position[1], t.Position[2]).
		Mul4(rotation.Mat4()).
		Mul4(mgl32.Scale3D(t.Scale[0], t.Scale[1], t.Scale[2]))
}

func (p *DataPreview) Sync(w *WorkspaceDocument, local project.ModID) {
	if w == nil {
		return
	}
	if p.source == w && p.Scene == w.Active && p.Local == local && p.sourceRevision == w.Revision {
		return
	}
	reset := p.Scene != w.Active || p.Local != local
	p.Scene, p.Local, p.sourceRevision = w.Active, local, w.Revision
	p.source = w
	if local != "" {
		for _, mod := range w.Plan.Mods {
			if mod.ID != local {
				continue
			}
			if len(p.Items) != 1 || reset {
				p.Items = make([]PreviewItem, 1)
				p.structure++
			}
			p.update(0, PreviewItem{ID: "local", Mod: local, Parent: -1, Placement: mgl32.Ident4(), Content: previewTransformMatrix(mod.Transform, mod.Spec.Space), Visible: true, Enabled: true, ContentRevision: w.Mods[local].contentRevision})
			p.Revision++
			return
		}
		p.Items = p.Items[:0]
		p.structure++
		p.Revision++
		return
	}
	for _, scene := range w.Plan.Scenes {
		if scene.ID != w.Active {
			continue
		}
		if !reset && len(p.Items) == len(scene.Instances) {
			for i, instance := range scene.Instances {
				if p.Items[i].ID != instance.ID || p.Items[i].Parent != instance.Parent {
					reset = true
					break
				}
			}
		}
		if len(p.Items) != len(scene.Instances) || reset {
			p.Items = slices.Grow(p.Items[:0], len(scene.Instances))[:len(scene.Instances)]
			clear(p.Items)
			p.structure++
		}
		for i, instance := range scene.Instances {
			mod := &w.Plan.Mods[instance.Mod]
			placement, visible := previewTransformMatrix(scene.Transform, mod.Spec.Space), instance.Visible
			if instance.Parent >= 0 {
				placement = p.Items[instance.Parent].Placement
				visible = visible && p.Items[instance.Parent].Visible
			}
			placement = placement.Mul4(previewTransformMatrix(instance.Placement, mod.Spec.Space))
			p.update(i, PreviewItem{ID: instance.ID, Mod: mod.ID, Parent: instance.Parent, Placement: placement, Content: placement.Mul4(previewTransformMatrix(mod.Transform, mod.Spec.Space)), Visible: visible, Enabled: instance.Enabled, ContentRevision: w.Mods[mod.ID].contentRevision})
		}
		p.Revision++
		return
	}
}

func (p *DataPreview) update(index int, item PreviewItem) {
	previous := p.Items[index]
	item.Revision = previous.Revision
	if item != previous {
		item.Revision++
		p.Items[index] = item
	}
}
