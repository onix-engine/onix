# Virtual Joystick

`ui.VirtualJoystick` is a UI widget with a circular touch track, draggable
thumb, dead-zone filtering, horizontal or vertical axis constraints, and ECS
messages. It is included by the `ui-widgets` engine feature.

```go
joystick := ui.SpawnVirtualJoystick(app.World, ui.VirtualJoystickOptions{
    X: 28, Y: 446, Size: 148, KnobSize: 58,
    DeadZone: .16,
})

value, _ := app.World.Get[ui.VirtualJoystick](joystick)
movement := value.Value // normalized mgl32.Vec2 in [-1, 1]
```

`Value` has the configured dead zone removed and is scaled back to the full
`[0, 1]` range. `RawValue` is the physical deflection after the axis constraint
and unit-circle clamp. `Active` is true from pointer press until release or
cancel.

Use `VirtualJoystickHorizontal` or `VirtualJoystickVertical` to lock one
axis. `ChangedEvent[mgl32.Vec2]` is emitted only when `Value` changes, and
`VirtualJoystickActiveEvent` reports press and release transitions. Call
`SetVirtualJoystickRawValue` for programmatic input or `ResetVirtualJoystick`
to clear it.

For a scene-authored UI root, bind the same behavior after loading it:

```go
err := scene.BindControl("move-stick", ui.VirtualJoystick{
    DeadZone: .16,
    Track: scene.MustEntity("move-stick-track"),
    Knob: scene.MustEntity("move-stick-knob"),
})
```

The root declares `"control": {}` and its visual parts are ordinary child
nodes in the scene JSON. This keeps the track and knob textures, colors,
dimensions, and render order authorable without runtime-generated visuals.
Parts may use `scene.Size2D` or an authored `Sprite2D` for their dimensions.

Set `Floating: true` on a joystick bound to a larger UI root to hide its parts
until press. The track is centered at the press position, dragging produces
the same normalized output, and release or cancel hides the parts again.
The root must cover the intended input region, such as the full viewport, and
must not set `passThrough`.

Keep application scene JSON under its own `resources/scenes/` directory.
Runnable applications are maintained separately from the engine checkout.
