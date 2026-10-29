package ecs

import "testing"

func TestComponentBindingRowsAndChangeTracking(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[int]()
	e := spawn.Spawn(1)
	bound := w.BindComponent[int]()
	missing := w.BindComponent[bool]()
	row := w.Row(e)
	version, tick := w.ComponentVersion[int](), w.ChangeTick()
	access := bound.Access(row)
	if access.Get() != 1 || missing.Access(row).Ref() != nil || bound.Access(w.Row(0)).Ref() != nil {
		t.Fatal("incorrect row lookup")
	}
	if CommitComponent(access, 1) || w.ComponentVersion[int]() != version || w.ChangeTick() != tick {
		t.Fatal("read/equal commit changed tracking")
	}
	if !CommitComponent(access, 2) || w.ComponentVersion[int]() != version+1 || w.ChangeTick() == tick {
		t.Fatal("write did not mark change")
	}
	if err := w.Insert(e, true); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expired row was accepted")
			}
		}()
		bound.Access(row)
	}()
	if value, ok := bound.Get(w.Row(e)); !ok || value != 2 {
		t.Fatal("binding did not survive migration")
	}
	other := NewWorld()
	foreignSpawn := other.NewSpawner1[int]()
	foreign := foreignSpawn.Spawn(3)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("foreign row was accepted")
			}
		}()
		bound.Access(other.Row(foreign))
	}()
	w.Despawn(e)
	if bound.Access(w.Row(e)).Ref() != nil {
		t.Fatal("retired entity was accepted")
	}
}

func BenchmarkBoundRowComponents(b *testing.B) {
	w := NewWorld()
	spawn := w.NewSpawner4[int, int32, int64, bool]()
	e := spawn.Spawn(1, 2, 3, true)
	a, c, d, f := w.BindComponent[int](), w.BindComponent[int32](), w.BindComponent[int64](), w.BindComponent[bool]()
	b.Run("WorldLookups", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			x, _ := w.Get[int](e)
			y, _ := w.Get[int32](e)
			z, _ := w.Get[int64](e)
			v, _ := w.Get[bool](e)
			if !v {
				b.Fatal("value")
			}
			benchmarkTotal = float32(x) + float32(y) + float32(z)
		}
	})
	b.Run("BoundRow", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			row := w.Row(e)
			x, _ := a.Get(row)
			y, _ := c.Get(row)
			z, _ := d.Get(row)
			v, _ := f.Get(row)
			if !v {
				b.Fatal("value")
			}
			benchmarkTotal = float32(x) + float32(y) + float32(z)
		}
	})
}
