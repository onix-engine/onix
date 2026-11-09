package project

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func TestSceneCloneEntryPointsPreserveSelectedHierarchy(t *testing.T) {
	for _, kind := range []string{"node", "children", "roots"} {
		t.Run(kind, func(t *testing.T) {
			s := traversalScene(3)
			parent := s.Entities["host"]
			var clones map[string]ecs.Entity
			var err error
			want := 3
			switch kind {
			case "node":
				clones, err = s.CloneNodeUnder(parent, " node0 ", " --copy-- ")
			case "children":
				clones, err = s.CloneChildrenUnder(parent, " node0 ", " --copy-- ")
				want = 2
			case "roots":
				clones, err = s.CloneRootsUnder(parent, " --copy-- ")
				want = 4
			}
			if err != nil || len(clones) != want {
				t.Fatalf("cloned %d nodes, want %d: %v", len(clones), want, err)
			}
			for id, entity := range clones {
				source := s.Entities[strings.TrimPrefix(id, "copy-")]
				sourceParent := s.World.GetRef[scene.Parent](source).Entity
				wantParent := clones["copy-"+s.EntityIDs[sourceParent]]
				if wantParent == 0 {
					wantParent = parent
				}
				if relation := s.World.GetRef[scene.Parent](entity); relation == nil || relation.Entity != wantParent {
					t.Fatalf("clone %q parent = %v, want %d", id, relation, wantParent)
				}
			}
		})
	}
}
