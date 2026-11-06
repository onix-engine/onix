package ecs

import "testing"

func BenchmarkScopeRebuild(b *testing.B) {
	for _, sparse := range []bool{false, true} {
		name := "dense"
		if sparse {
			name = "sparse"
		}
		b.Run(name, func(b *testing.B) {
			w := NewWorld()
			spawn := w.NewSpawner1[scopeNumber]()
			members := make([]Entity, 10000)
			for i := range members {
				members[i] = spawn.Spawn(scopeNumber(i))
			}
			if sparse {
				members = []Entity{members[9999], members[0]}
			}
			var scope QueryScope
			scope.Reset(members)
			w.SetQueryScope(&scope)
			query := w.Query[scopeNumber]()
			for range query.All {
			}
			b.ReportAllocs()
			for b.Loop() {
				scope.Reset(members)
				for range query.All {
				}
			}
		})
	}
}
