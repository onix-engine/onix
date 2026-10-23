# Module system refactor plan (historical design)

The original JSON events/routes design was superseded by unified Go OnMessage handlers.
Current documents do not accept those old fields. Use [module authoring](module-system-authoring.md#typed-messages-and-input)
for supported configuration, API names and dispatch rules. This record preserves the
architectural decisions and staged implementation; application-specific migration
notes belong with the independent projects.

## Goals and ownership

Scenes orchestrate module definitions and independent module instances. Modules own
content, resources, state and behavior: visual elements, font/audio services, animation,
cameras and game logic can all be modules. Scenes select definitions, place instances,
order them, initialize public slots and coordinate communication. A module must not
reach another module's files, resources, entities or implementation types. Shared
contracts contain data only. ECS/rendering/resource caches remain underlying machinery;
a module may contain several entities/files.

A reused definition is immutable; every placement gets independent slots, subscriptions,
behavior, animation and requests. Shared content caches do not make behavior a singleton.
Cross-module state changes use declared messages, including external slot writes. Remove
old runtime entry/fallback formats directly rather than maintain compatibility branches.

Project settings and scene JSON live under resources, with resources/project.json
holding entry/scenes and resources/scenes/main.json as the default empty modules scene.
Only mod.json is required for a module; an empty object is valid. Module identity is
its project-relative directory; mod.go is optional and resources are private paths.
The implementation allows module-local resource directories. Follow repository guidance
to place authored resource JSON under resources; required mod.json is the module marker.
Unknown fields, duplicate instances, missing definitions/slots and path escapes must
produce file/field diagnostics. Resources cannot cross module or symlink/embed boundaries.

## State and coordinate separation

Store scene root, scene placement hierarchy, instance placement, module-local transform
and content/slot transforms separately. Rendering multiplies them; editing placement
does not rewrite definitions and local animation does not rewrite scene placement.
Scene children inherit placement proxies rather than a parent's module-local animation.
Declare UI/world spaces explicitly. Do not silently mix pixels and meters.

Array order determines sibling paint order, flattened by stable depth-first traversal.
Batching preserves UI occlusion. 3D keeps depth/transparent sorting within scene/camera
priorities. Private module content stays inside its instance's paint scope even after
entity IDs are recycled.

## Communication and lifecycle

The early proposal used typed ports and JSON routes on two scopes: module messages
within a scene instance and scene messages within an application. That schema is
historical. Current scenes coordinate typed OnMessage delivery explicitly; do not copy
the old events/routes format into projects.

The persistent principles are single-threaded deterministic queues, a frozen batch per
logical tick, and callback-generated messages delivered in the next batch. Never
recursively invoke recipients. Resolve identities/type storage at registration or load;
no JSON encoding or reflection in dispatch. Capacity limits return errors or declared
coalescing behavior, never silently drop essential messages. Delivery scales with actual
targets rather than scanning every module. Structural changes commit at batch boundaries.

Requests carry unique IDs, source generations and response addresses. Timeouts,
cancellation and destruction invalidate outstanding work; late replies cannot land on
reused instances. Slot revisions reject stale text results. Startup can advance a
bounded number of preparation batches before Active; rendering frames and logical ticks
remain distinct. Services without Update do not enter update lists; pending events/
requests prevent idle scheduling from stalling progress.

Context limits access to owned entities/resources/slots. Go code remains trusted and
this is an architectural boundary rather than a process sandbox. Build validation checks
direct module implementation imports, resource traversal, embed and symlink escapes.
Generated registration goes in onix_generated.go with stable import aliases; user main.go
is created initially and preserved afterward. Pure-data modules need no Go registration.

## Visual service outputs

A consumer requests text using shared TextRenderRequest/Result data. The font service
owns decoding/layout/glyph preparation and returns an immutable RenderOutputHandle and
logical size. The consumer checks generation/revisions and installs it in a visual slot.
No font path, provider entity or mutable rendering pointer crosses the boundary. Reuse
text geometry/atlases rather than create one texture per frame.

Outputs have explicit leases. Successful Reply transfers ownership to the queue; callbacks
borrow, slots retain independently, and replacing/clearing/destroying releases. Unused,
cancelled, expired and stale responses release their output. Delivered output can outlive
the provider until the final consumer lease releases resources. GPU retirement uses the
existing deferred resource lifecycle. Output never includes a provider transform or
mutable callback; consumers apply local placement, clipping and paint order.

## Scene management

SceneManager separates Loading, Ready, Active, Suspended and Disposed. Prepare a target
before committing a switch; failure preserves the foreground scene and reports an error.
Startup payloads are typed. Default switching disposes the source; explicit retention
keeps state while pausing content, update/input and module messages. Later refinements
add preload, Enter/Resume/Exit/Suspend and overlays, as documented in the current guide.
Do not interpret the original stage proposal as the complete current behavior.

## Editor documents

SceneDocument and ModDocument keep stable identities, dirty/revision state and independent
saved baselines. Undo commands identify owning documents; placement writes scene JSON,
definition edits write mod.json and refresh references. Multi-document edits validate
before committing. Invalid forms retain drafts and precise diagnostics.

Scenes shows scene/instance/placement hierarchy. Selecting an instance shows its private
file tree. Mod separates instance overrides/placement/state from definition transform,
content/resources/slots/behavior. Copy instance reuses the definition; copy as a new
module duplicates private files. Deleting definitions refuses dangling references.
Reordering modifies arrays; preview drag forms one undo operation. Saves write changed
documents only. UI labels/tree selection and accessibility follow W3C/Chromium semantics.

Data preview does not execute mod.go, service requests or game logic: placeholders must
remain explicit. Play saves/validates/generates/builds/runs. Visual acceptance is manual;
automated tests inspect documents, component state and headless preview data.

## Build and performance

Discover modules/scenes/behavior sources in one directory pass. Combine validation,
resource registration, local indexing and compilation where practical; resolve forward
references through pending tables. Analyze all registered scenes, including services
without visible content; topics cannot substitute for explicit resource reachability.
Collection follows declarations and local dependencies regardless of folder names.
Reuse resource packages, ContentID deduplication, typed loading and feature pruning.

Release uses one preprocessed ONIXBIN1 ProjectPlan with scene plans, definitions,
instance/slot indexes and behavior identities; it does not load raw source JSON as a
fallback. Dynamically used resources require explicit keep/include declarations. Plans
enter the base package and registered scenes' dependencies are startup requirements.
Desktop uses overlays/external resources, Web preloads packages, mobile stages entries
and embeds equivalent packages. Independent project module paths differ from engine imports.

Reserve typed event/request/command buffers, resolve handlers once, update text only
on input/style/size changes, and respect borrowed payload lifetimes. Warm fixed-capacity
delivery, existing slot updates, Update and stable text should allocate zero. First
load/glyphs, new instances, growth and arbitrary editing are cold work and measured
separately. Avoid duplicate traversals and per-frame callbacks for empty services.

## Staged implementation record

A established strict ModSpec/SceneSpec schemas, project creation and compilation.
Acceptance covered empty projects/modules, repeated independent instances and precise
unknown-field/resource/slot diagnostics.

B added module.Runtime/Scene/Context, private ownership, typed dispatch, double buffers,
slot revisions, deferred destruction, App.LoadModules and fixed-tick ModuleEvents.
Tests covered isolation, stable order, stale requests and warm allocation reuse. Early
solid-sprite/input content preceded complete resource installers in F.

C implemented font services, leased visual outputs and transactional scene preparation.
Tests covered lease reclamation, provider exit, stale replies, pressure, bounded caches,
coordinates/clipping, budgets, failed switching and resume. CachedFontRequest was about
827 ns/op and VisualReply about 840 ns/op in the recorded Windows amd64 run, both warm
0 B/op and 0 allocs/op. These exclude first font/glyph/scene preparation.

D replaced editor node/prefab workflows with scene/module documents, reference-aware
copy/delete, separate placement/definition forms, preview, save and undo. Headless state
regressions verified behavior; interactive visuals remained manual.

E added source discovery, concrete method/type checks, generated contracts/registration,
import/embed/resource diagnostics, immutable release plans, resource collection, capability
selection and desktop/Web/mobile builders. JSON-only modules and duplicate Mod names
are valid. Generated lifecycle/dispatch warm allocation tests passed. Recorded Windows
Release ZIP, Web Wasm/Brotli and Android libonix.so exports succeeded. APK packaging failed
with Java 26/Gradle 8.9; supported JDK 17–22 and a reachable device were still needed.
The Android connection timed out, so compilation did not establish installed app behavior.

F migrated content installers for sprites/text, images/atlases/SVG, animation, tilemaps,
particles/trails, rigs/poses, HTML, cameras/lights, physics, audio and Spine. Collision
messages expose private element identities rather than ECS/body/collider handles. HTML
handles stay surface-owned and route ID/type/value, not foreign entities. Audio owns
pause/resume/disposal through module lifecycle. Physics currently requires final world
transforms with unit scale and no shear/reflection; 2D remains in the XY plane. Put
size in collider shapes. Application-specific migration reports live outside the engine.
Historical source migrations were completed, while some legacy test helpers/editor
compatibility cleanup remained follow-up items in the original record; do not interpret
those stage notes as proof every old subsystem was deleted.

## One-time migration and acceptance

Generate independent drafts/reports before replacing old content; never overwrite first
and rely on rollback. Move scenes.json registration into project.json, node/prefab trees
into meaningful module ownership, resources into private declarations and behavior into
module code. Cross-boundary camera/widget/animation references become typed messages or
engine service protocols. A wrapper around every old node is not adequate architectural
migration. Flag ambiguous control flow/resources/references for explicit rewriting.

Run bounded targeted package tests and benchmarks for isolation, transform composition,
paint order, routing, slots, cancellation/late replies, lease release, failed switching,
undo, collection and release loading. Measure allocations and growth against recipient
counts. No whole-repository test command, editor screenshot tests or subagents. Mobile
validation distinguishes build success from connection/install/runtime success.

The final architecture requires correct independent project entry, reusable isolated
modules, owned coordinates/resources, explicit communication, visual outputs through
slots, editor/build support and direct cleanup of superseded formats. App-wide permanent
modules, arbitrary user channels and a free-canvas event editor were outside the initial
completion criteria.
