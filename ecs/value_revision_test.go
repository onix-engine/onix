package ecs

import "testing"

func TestValueRevisionsIgnoreStorageMigration(t *testing.T) {
	w := NewWorld()
	s := NewSpawner1[int](w)
	e := s.Spawn(1)
	entities := w.EntityVersion()
	values, members := ComponentValueRevision[int](w)
	storage, _ := ComponentRevision[int](w)
	if err := Insert(w, e, "unrelated"); err != nil {
		t.Fatal(err)
	}
	if got, count := ComponentValueRevision[int](w); got != values || count != members || w.EntityVersion() != entities {
		t.Fatal("migration changed value or entity membership revisions")
	}
	if got, _ := ComponentRevision[int](w); got == storage {
		t.Fatal("migration did not invalidate storage references")
	}
	*GetMut[int](w, e) = 2
	if got, count := ComponentValueRevision[int](w); got != values+1 || count != members {
		t.Fatal("write did not change only the value revision")
	}
	Remove[int](w, e)
	if _, count := ComponentValueRevision[int](w); count != members+1 || w.EntityVersion() != entities {
		t.Fatal("removal did not change only component membership")
	}
	w.Despawn(e)
	if w.EntityVersion() != entities+1 {
		t.Fatal("despawn did not change entity membership")
	}
}

func TestUnscopedQueryInsideScopedIteration(t *testing.T) {
	w := NewWorld()
	s := NewSpawner1[int](w)
	a, b := s.Spawn(1), s.Spawn(2)
	var scope QueryScope
	scope.Reset([]Entity{a})
	w.SetQueryScope(&scope)
	for e := range Query[int](w).All {
		if e != a {
			t.Fatal("outer query escaped its scope")
		}
		count := 0
		for range Query[int](w, Unscoped()).All {
			count++
		}
		if count != 2 || w.QueryScope() != &scope {
			t.Fatal("unscoped query changed execution scope")
		}
	}
	count := 0
	for e := range Query[int](w, Unscoped()).In([]Entity{b}).All {
		if e != b {
			t.Fatal("explicit restriction was ignored")
		}
		count++
	}
	if count != 1 {
		t.Fatal("explicit restriction returned no entity")
	}
}

func TestPatchValueRevisionsSeparateWritesAndMembership(t *testing.T) {
	w := NewWorld()
	s := NewSpawner2[int, bool](w)
	e := s.Spawn(1, true)
	values, members := ComponentValueRevision[int](w)
	unchanged, retained := ComponentValueRevision[bool](w)
	p := NewPatch2[int, string](w)
	if err := p.Apply(e, 2, "added"); err != nil {
		t.Fatal(err)
	}
	if got, count := ComponentValueRevision[int](w); got != values+1 || count != members {
		t.Fatal("overwriting a migrated value changed its membership")
	}
	if got, count := ComponentValueRevision[bool](w); got != unchanged || count != retained {
		t.Fatal("patch invalidated an unchanged migrated value")
	}
	if got, count := ComponentValueRevision[string](w); got != 1 || count != 1 {
		t.Fatal("patch insertion did not record value membership")
	}
}
