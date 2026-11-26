# Performance Guide

[English](performance.md) | [简体中文](performance.zh-CN.md)

## General rule

Performance claims apply to warmed engine paths with stable inputs and retained
capacity. Initialization, resource creation, first use, capacity growth, device
recreation and workload changes may allocate. Go allocation counts do not
include GPU work, browser/JavaScript objects, driver internals or process RSS.

## ECS and UI

Use typed queries for repeated component access. Prefer read-only references and
request mutable access only when a value will change, because mutable access
marks component change state. Reuse saved query views when applying change
filters; a newly constructed view with many filters may allocate. Warm query
iteration, command flushing and prepared bundle operations are designed to reuse
storage within capacity. See [ECS API design](ecs-api-design.md).

Change tracking may use conservative summaries to skip unchanged groups. It must
preserve tick semantics across writes, archetype migration and row movement.
Warm-path allocation assertions should be measured without race instrumentation.

## Rendering and HTML

Reuse render resources and scratch buffers while dimensions and capacity remain
stable. Upload and readback paths may reuse staging storage; frame fences still
govern when GPU-visible memory can be reused. Resource residency belongs to a
device and resize/recreation is a cold path.
[Rendering pipeline (简体中文)](../rendering/render-path.zh-CN.md)

Retained HTML layout should avoid rebuilding unaffected subtrees. Correctness
comes first: local updates must preserve browser order, clipping, inherited
styles and event behavior. Compare HTML/CSS changes with the
[Chromium compatibility fixtures](../../tests/htmlcompat/README.md).

## Cache lifetime

- Clear retired references after the borrow or submission ends. For example,
  `input.State.DrainEvents` borrows its slice until the next enqueue,
  `ReleaseEvents`, or frame end. `ReleaseEvents` preserves newly queued events.
  [Input queue](../../input/state.go)
- Reuse scratch storage across calls and record the largest batch in a frame.
  The shared retention policy trims sustained excess capacity. Compaction may
  allocate once and move backing storage, so reacquire borrowed views after
  maintenance. [Retention policy](../../internal/reuse/retention.go) · [ECS frame maintenance](ecs-api-design.md#frame-maintenance-and-query-scopes)
- Tie caches to their owner and bound long-lived caches by entries and payload.
  Share immutable payloads across consumers when their lifetimes agree. Retire
  GPU resources through the owning device after pending draws are safe.

## Measurement

Use targeted package benchmarks with `-benchmem`. Warm the path and report the
platform, workload, capacity, setup cost, and latency separately from allocation
counts. Measure live heap, process RSS, and GPU memory separately when comparing
memory retention. Measure full frames on the target backend for FPS claims.

```sh
go test ./ecs -run '^$' -bench '^BenchmarkChangedQuerySparse$' -benchmem
go test ./ui/html -run '^$' -bench '^BenchmarkHTMLPaintCull' -benchmem
```
