# Engine Design

[English](design.md) | [简体中文](design.zh-CN.md)

## Ownership and boundaries

- The application loop owns ECS mutation, scheduling and rendering. Worker
  goroutines may request a frame, but must not mutate ECS state.
- A `World` owns component storage, resources, change ticks and query state.
  Structural edits go through World APIs and are deferred while queries borrow
  storage; cached entity handles must be checked against lifetime and ownership.
- Modules own their runtime state and communicate through typed messages,
  slots and explicit lifecycle transitions. Scenes own prepared scene content.
- A window owns its render device and canvas. GPU resources keep residency per
  device; CPU-backed resources can be prepared before upload. GPU-only and
  external resources cannot be recreated on a different device without an
  importer or CPU data.

## Scheduling and preparation

Continuous updates are the default. On-demand scheduling is opt-in and requires
all time-dependent work to report activity or schedule a deadline. Input,
explicit render requests, tracked ECS changes and active effects wake the loop.
CPU resource preparation may happen off the render thread; GPU upload is queued
to the owning device and is bounded by the caller's upload budget.

## Rendering and HTML

Transforms are synchronized before camera extraction. Extraction determines
stable draw order; later camera filtering and drawing preserve that order.
Rendering APIs use an explicit window/device context rather than a process-wide
implicit device. Resource disposal runs through the owning driver.

The HTML subsystem keeps DOM/layout state separate from emitted UI/render state.
Local mutations should update affected nodes and ancestors while preserving
document order and browser behavior. CSS and HTML changes must follow W3C rules
and match Chromium fixtures; unsupported behavior should be explicit rather
than silently approximated.

## Further reading

- [ECS API design](ecs-api-design.md)
- [Module authoring and lifecycle](module-system-authoring.md)
- [Resource package ownership](resource-packages.md)
- [Rendering and lighting](render-lighting.md)
- [HTML compatibility fixtures](../tests/htmlcompat/README.md)
- [Performance contracts](performance.md)
