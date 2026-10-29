package ecs

import "testing"

func TestValueRevisionsIgnoreStorageMigration(t *testing.T) {
	w := NewWorld()
	s := w.NewSpawner1[int]()
	e := s.Spawn(1)
	entities := w.EntityVersion()
	values, members := w.ComponentValueRevision[int]()
	storage, _ := w.ComponentRevision[int]()
	if err := w.Insert(e, "unrelated"); err != nil {
		t.Fatal(err)
	}
	if got, count := w.ComponentValueRevision[int](); got != values || count != members || w.EntityVersion() != entities {
		t.Fatal("migration changed value or entity membership revisions")
	}
	if got, _ := w.ComponentRevision[int](); got == storage {
		t.Fatal("migration did not invalidate storage references")
	}
	*w.GetMut[int](e) = 2
	if got, count := w.ComponentValueRevision[int](); got != values+1 || count != members {
		t.Fatal("write did not change only the value revision")
	}
	w.Remove[int](e)
	if _, count := w.ComponentValueRevision[int](); count != members+1 || w.EntityVersion() != entities {
		t.Fatal("removal did not change only component membership")
	}
	w.Despawn(e)
	if w.EntityVersion() != entities+1 {
		t.Fatal("despawn did not change entity membership")
	}
}

func TestUnscopedQueryInsideScopedIteration(t *testing.T) {
	w := NewWorld()
	s := w.NewSpawner1[int]()
	a, b := s.Spawn(1), s.Spawn(2)
	var scope QueryScope
	scope.Reset([]Entity{a})
	w.SetQueryScope(&scope)
	for e := range w.Query[int]().All {
		if e != a {
			t.Fatal("outer query escaped its scope")
		}
		count := 0
		for range w.Query[int](Unscoped()).All {
			count++
		}
		if count != 2 || w.QueryScope() != &scope {
			t.Fatal("unscoped query changed execution scope")
		}
	}
	count := 0
	for e := range w.Query[int](Unscoped()).In([]Entity{b}).All {
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
	s := w.NewSpawner2[int, bool]()
	e := s.Spawn(1, true)
	values, members := w.ComponentValueRevision[int]()
	unchanged, retained := w.ComponentValueRevision[bool]()
	p := w.NewPatch2[int, string]()
	if err := p.Apply(e, 2, "added"); err != nil {
		t.Fatal(err)
	}
	if got, count := w.ComponentValueRevision[int](); got != values+1 || count != members {
		t.Fatal("overwriting a migrated value changed its membership")
	}
	if got, count := w.ComponentValueRevision[bool](); got != unchanged || count != retained {
		t.Fatal("patch invalidated an unchanged migrated value")
	}
	if got, count := w.ComponentValueRevision[string](); got != 1 || count != 1 {
		t.Fatal("patch insertion did not record value membership")
	}
}
