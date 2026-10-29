package scene

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
)

func TestAnimationTaskCountTracksCompletionPauseAndScope(t *testing.T) {
	w := ecs.NewWorld()
	clock := &Time{Delta: 2 * time.Second}
	w.InsertResource(clock)
	parents := w.NewSpawner1[Activity]()
	parent := parents.Spawn(Activity{Active: true})
	animators := w.NewSpawner2[Animator, Parent]()
	entity := animators.Spawn(NewAnimator(&AnimationClip{Duration: 1}, false), Parent{Entity: parent})
	check := func(want int) {
		t.Helper()
		if got := AnimationTaskCount(w); got != want {
			t.Fatalf("animation tasks=%d want=%d", got, want)
		}
	}
	check(1)
	if err := w.Insert(parent, Activity{}); err != nil {
		t.Fatal(err)
	}
	check(0)
	if err := w.Insert(parent, Activity{Active: true}); err != nil {
		t.Fatal(err)
	}
	check(1)
	if err := UpdateWorldAnimatorSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	check(0)
	w.GetMut[Animator](entity).Play()
	check(1)
	var scope ecs.QueryScope
	scope.Reset(nil)
	w.SetQueryScope(&scope)
	check(0)
	scope.Reset([]ecs.Entity{entity})
	check(1)
	w.SetQueryScope(nil)
	check(1)
	w.GetMut[Animator](entity).Pause()
	check(0)
	if allocations := testing.AllocsPerRun(100, func() {
		if AnimationTaskCount(w) != 0 {
			panic("paused animation changed")
		}
	}); allocations != 0 {
		t.Fatalf("stable task count allocated %g times", allocations)
	}
}

func TestEffectTaskCountsDrainAndRestart(t *testing.T) {
	w := ecs.NewWorld()
	clock := &Time{Delta: 50 * time.Millisecond}
	w.InsertResource(clock)
	emitter := NewParticleEmitter2D()
	emitter.Loop, emitter.Count, emitter.Lifetime = false, 1, .1
	emitters := w.NewSpawner1[ParticleEmitter2D]()
	entity := emitters.Spawn(emitter)
	if ParticleTaskCount(w) != 1 {
		t.Fatal("new burst allowed suspension")
	}
	if err := UpdateParticleEmitter2DSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if ParticleTaskCount(w) != 1 {
		t.Fatal("living particles allowed suspension")
	}
	clock.Delta = time.Second
	if err := UpdateParticleEmitter2DSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if ParticleTaskCount(w) != 0 {
		t.Fatal("finished emitter prevented suspension")
	}
	w.GetMut[ParticleEmitter2D](entity).Restart()
	if ParticleTaskCount(w) != 1 {
		t.Fatal("restarted burst did not become active")
	}
	trail := NewSpriteTrail2D()
	trail.samples = []spriteTrailSample2D{{}}
	trails := w.NewSpawner1[SpriteTrail2D]()
	trailEntity := trails.Spawn(trail)
	if SpriteTrailTaskCount(w) != 1 {
		t.Fatal("fading trail allowed suspension")
	}
	w.GetMut[SpriteTrail2D](trailEntity).advance(1)
	if SpriteTrailTaskCount(w) != 0 {
		t.Fatal("empty trail prevented suspension")
	}
}

func TestTaskCountReusesSemanticCache(t *testing.T) {
	type tag struct{}
	w := ecs.NewWorld()
	spawner := w.NewSpawner1[int]()
	e := spawner.Spawn(1)
	calls := 0
	count := func() int {
		return ActiveTaskCount[tag, int](w, 0, func(_ *ecs.World, _ ecs.Entity, value *int) bool {
			calls++
			return *value > 0
		})
	}
	if count() != 1 || count() != 1 || calls != 1 {
		t.Fatal("unchanged tasks were scanned again")
	}
	if err := w.Insert(e, "unrelated component"); err != nil {
		t.Fatal(err)
	}
	if count() != 1 || calls != 1 {
		t.Fatal("row migration invalidated task count")
	}
	*w.GetMut[int](e) = 0
	if count() != 0 || calls != 2 {
		t.Fatal("semantic write failed to invalidate task count")
	}
}
