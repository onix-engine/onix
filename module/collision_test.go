package module

import (
	"testing"

	"github.com/diiyw/onix/physics"
)

func TestModuleCollisionEventsKeepOwnershipAndBufferDelivery(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/project.json":     `{"entry":"main.json","scenes":["main.json"]}`,
		"resources/scenes/main.json": `{"modules":[{"id":"ball","mod":"ball"},{"id":"wall","mod":"wall"},{"id":"body3d","mod":"body3d"},{"id":"log","mod":"log"}]}`,
		"ball/mod.json":              `{"space":"world","elements":[{"id":"body","components":{"rigidBody2D":{"type":"dynamic"},"circleCollider2D":{"radius":1,"sensor":true,"collisionType":7},"collision":{"emit":"hit"}}}]}`,
		"wall/mod.json":              `{"space":"world","components":{"rigidBody2D":{"type":"static"},"circleCollider2D":{"radius":1,"collisionType":11}}}`,
		"body3d/mod.json":            `{"space":"world","components":{"rigidBody3D":{"type":"dynamic"},"collision":{"emit":"hit"}}}`,
		"log/mod.json":               `{}`,
	})
	var count2D, count3D int
	var value2D Collision2D
	var value3D Collision3D
	registry := Registry{}
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		if m.Topic != "hit" {
			return nil
		}
		topic := "hit"
		if m.From == "body3d" {
			topic = "hit3d"
		}
		return c.ForwardAs("log", topic, m)
	}}))
	must(t, registry.RegisterMod("log", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		if err := handleValue[Collision2D](c, "hit", func(_ *Context, v Collision2D) error { count2D++; value2D = v; return nil }); err != nil {
			return err
		}
		return handleValue[Collision3D](c, "hit3d", func(_ *Context, v Collision3D) error { count3D++; value3D = v; return nil })
	}}))
	r := newFixture(t, plan, registry, Options{Source: source})
	s, err := r.Manager().StartEntry()
	must(t, err)
	runtime := physics.NewRuntime2D()
	runtime.Simulation.Gravity = physics.Vector{}
	r.world.InsertResource(runtime)
	step := physics.StepSystem2D("")
	must(t, step.Run(r.world))
	r.world.WriteMessage(physics.CollisionMessage3D{Entity: s.modules[2].root, Other: s.modules[1].root, Phase: physics.CollisionBegin, CollisionType: 3, OtherType: 11, Point: physics.V3(1, 2, 3)})
	must(t, r.PublishCollisions())
	if count2D != 0 || count3D != 0 {
		t.Fatal("physics callbacks ran inside the solver step")
	}
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	if count2D != 1 || value2D.Element != "body" || value2D.Type != 7 || value2D.OtherType != 11 || value2D.Phase != physics.CollisionBegin {
		t.Fatal("lost 2D collision data", count2D, value2D)
	}
	if count3D != 1 || value3D.Element != "" || value3D.Point != [3]float32{1, 2, 3} || value3D.OtherType != 11 {
		t.Fatal("lost 3D collision data", count3D, value3D)
	}
	advance := func() {
		r.world.AdvanceMessages()
		if err := step.Run(r.world); err != nil {
			panic(err)
		}
		if err := r.PublishCollisions(); err != nil {
			panic(err)
		}
		if err := r.Dispatch(); err != nil {
			panic(err)
		}
		if err := r.Dispatch(); err != nil {
			panic(err)
		}
	}
	for range 20 {
		advance()
	}
	if allocs := testing.AllocsPerRun(100, advance); allocs != 0 {
		t.Fatalf("warm collision bridge allocated %g", allocs)
	}
	must(t, step.Run(r.world))
	must(t, r.PublishCollisions())
	count := count2D
	must(t, s.modules[0].Dispose())
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	if count2D != count {
		t.Fatal("disposed module published a queued collision")
	}
}
