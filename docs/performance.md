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
device and resize/recreation is a cold path. See [rendering and lighting](render-lighting.md).

Retained HTML layout should avoid rebuilding unaffected subtrees. Correctness
comes first: local updates must preserve browser order, clipping, inherited
styles and event behavior. Compare HTML/CSS changes with the
[Chromium compatibility fixtures](../tests/htmlcompat/README.md).

## Measurement

Use targeted package benchmarks with `-benchmem`, warm the path being measured,
and state the platform, workload, capacity and included setup cost. Report
allocations separately from latency. A microbenchmark, pixel test or reduced
upload count is not a whole-frame FPS claim.
