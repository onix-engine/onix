# Project data, creation and module runtime

The engine provides strict project compilation, independent project creation, module/scenario lifecycles, typed messages, visual slots, scene management, editor workflows and generated generic registration. Editor/build entry points consume the new documents and ProjectPlan directly; legacy nodes, Prefab, scenes.json and resources.json are not fallback inputs. Application examples are maintained outside the engine checkout.

## Create and validate an independent project

From the engine checkout, use `go run ./editor/cmd/onix new -width 750 -height 1624 ../my-game`, followed by `check ../my-game`. Creation generates main.go, onix_generated.go and resources/project.json with resources/scenes/main.json containing an empty modules array. Entry and optional scene ordering live in project configuration; the compiler/editor discover additional JSON scenes in resources/scenes. Scene IDs are filenames such as main.json.

Existing entry files cause failure rather than overwriting user main.go. An independent project gets go.mod with a local engine replace and engine sums when no parent module exists. Set `-module example.com/game -engine-dir /path/to/onix` explicitly or select a published dependency with `-engine-version VERSION`. Projects inside a Go module use that module. Generated entry code opens module data, obtains OnixRegistry, installs systems, opens a window and loads modules. Generation, Save, Play and Build maintain the registration file while leaving user main.go alone.

## Editor workflow

Launch the editor manually with an external project directory, or without arguments for New/Open. New defaults to 750×1624. Scenes displays scene→instance→child trees, supports create/reuse definitions, instance placement parenting/reordering and Ctrl+S. Clicking an already-selected scene toggles expansion. Module rows show definition names; file views show private module files or project files when nothing is selected. Refresh discovers files/definitions and clears image caches; external document changes report save conflicts and reload on reopening.

Mod separates instance placement, enabled/visible flags and slot overrides (scene file) from definition dimensions, slots, identity, transform, content, resources, references and behavior (module mod.json). Blank overrides inherit defaults; visual JSON initial values must be empty. Communication stays in Go OnMessage; there are no editor message routes. Multi-document declarations can form one undo operation.

Scene preview combines scene root and placement without applying a parent's module-local transform to child placements. Local preview uses module coordinates. Dragging changes placement and commits one undo on release. Data previews support solid colors and private images; text/atlas/animation/visual service output can be placeholders and do not execute module code or font requests.

Empty definitions are valid. Copying an instance reuses its definition; copying as a new module duplicates the private directory. Instance deletion edits only the scene. Definition deletion refuses dangling references and lists locations; committed deletion moves files into resources/.editor-trash for undo restoration. Save writes changed documents only; Ctrl+Z/Y undo/redo applied commands. Selection/preview/closing prompts for pending forms and project changes.

Play saves, validates code/resource boundaries and method signatures, regenerates registration, builds and runs with project-root working directory. Errors appear in status; Stop terminates build/game processes. Build adds target/mode/name/toolchain settings and performs collection, preprocessing, packaging and export in the background, pausing document operations until completion.

EditScene, EditMod, EditInstance and PatchDocuments expose document edits. DocumentID is always the owning project-relative JSON path. Multi-file patches commit after whole-project validation. CompileProjectDefinitions validates opened but unreferenced definitions. Automated checks inspect documents, control state and headless preview data; visual acceptance stays manual.

## Module resources and data

Only mod.json is mandatory; `{}` defines an empty module. Top-level definitions are discovered/validated and referenced definitions compile once. Resource declarations are module-relative and must exist. Absolute paths, parent traversal, backslashes, symlink/embed escapes and cross-module file access are rejected. Modules organize private directories themselves; keep authored JSON under resources/ according to repository conventions.

Resource kinds are image, atlas, font, audio, video, animation, model, spine, shader, html, tilemap and data. Registered scenes determine collection, including local dependencies. Content installers support text/animation, particles, trails, tilemaps, rigs/poses, HTML, cameras, lights and physics. Audio players belong to modules. Configure custom WGSL on private elements' components.shader; roots do not expose shader components and there is no module-level shader list. See [custom shaders](custom-shaders.md).

Module-local transform and scene placement are independent. Position/scale accept two or three finite values; rotation is one Z angle or three Euler angles in radians. Defaults are zero translation/rotation and unit scale. Space defaults to ui or explicitly world; placement hierarchies cannot mix spaces.

Public slots declare string/bool/number/int/color/vec2/vec3/visual or project-defined structures. Visual initial values must be null, never cross-module paths. writable defaults true. Project types declare exact fields/types; compiled scalar/fixed-vector/continuous-field values avoid map[string]any. Instances declare id/mod/placement/slots/children. IDs are scene-wide unique but may repeat across scenes. Overrides target declared slots and own mutable storage independent from defaults/other instances. Compilation flattens stable depth-first order.

## Generation and release

Module and scene handlers use OnMessage(*module.Context,module.Message) error. Strict decoding rejects old events/routes fields. Senders address instance IDs without receiver subscriptions; missing handlers ignore messages. Module implementations cannot import each other; shared data types live in contracts or independent utility packages. Go-only message structs need neither JSON schemas nor RegisterValue.

`onix generate` scans Mod lifecycle methods and emits generic bindings in onix_generated.go. Set behavior to Mod in mod.json; packages may reuse that type name through stable import aliases. Scene behavior types live in the project root package and use identical optional lifecycle signatures. Install resolves private elements, slots and high-frequency handles. Source updates preserve user main.go; failed validation preserves previous generated files. SVG generates concrete ImageDecoderWith support, with container magic taking precedence over extensions.

Project types generate stable field-ordered structs, JSON names and SlotValue decoders in contracts/onix_generated.go. Contracts contain data protocols, not handwritten initialization, methods or package variables. Registry factories and engine-feature/image-format selection are generic and do not reflectively discover runtime methods. Unused systems/decoders stay unregistered.

Release emits resources/modules.bin containing all registered scene plans, definitions, flattened instances, defaults and behavior identities. Original scene/module JSON and Go source are not runtime assets. Release reads binary plans without working-directory JSON fallback. Collection follows declared resources, inline sprite/animation references and local atlas/glTF/HTML/CSS dependencies. Data stays raw without guessing strings as paths. Dynamically opened files must appear in resources/packages.json keep/include and remain module-owned. Plans always enter base.dat; all registered scenes' package dependencies load at startup. This does not implement scene-based package unloading. Content and plan changes invalidate build caches.

Desktop uses resource directories plus compile overlays; Web preloads catalogs/packages; Android/iOS embed the same packages in staged entry points. Project imports use its own module path; engine imports remain github.com/diiyw/onix. Targets require their platform toolchains. Gradle 8.9 validation accepts JDK 17–22. Historical validation built Windows Release ZIP, Web Wasm/Brotli and Android libonix.so; APK packaging was blocked by Java 26 and device connection timeout, so those records do not establish installed mobile or visual validation.

## Mounting and scheduling

CompileProject, CompileProjectDirectory and CreateProjectDirectory produce immutable-lifetime ProjectPlan/ScenePlan/ModPlan data. Compilation validates JSON/types/defaults and resolves indexes. App.LoadModules does not open a window. Source must match the compilation root. Headless logic can use module.New directly; normal entries configure a window and call Run. OnixOpenWindow applies project window metadata; OnixModuleOptions supplies private resources and static decoders.

App.Modules exposes the orchestration Runtime. LoadModules starts the entry through Manager.StartEntry, completing budgeted startup before Active. Manager.Switch queues work for the next Dispatch structural boundary. Direct MountEntry/Mount APIs create independently scoped/generation-tracked active scenes for tools/tests; games normally use Manager. Dispose destroys instances; App.Destroy closes the runtime.

Fixed scheduling is FixedFirst→ModuleEvents→FixedPreUpdate→FixedUpdate→FixedPostUpdate→FixedLast. ModuleEvents freezes one batch per logical tick; FixedUpdate invokes registered updates. Rendering may contain zero or multiple ticks without duplicate delivery. Tick.Delta is seconds. Services without Update get no empty callback. Updates, queued messages and pending requests keep logic advancing so idle scheduling cannot stall timeouts.

module.HTML returns an ownership-limited surface handle for node lookup/text/attributes/visibility, invalid after replacement/destruction. node.SetSelectedIndex changes live selection (`-1` clears), preserves authored defaults, updates labels/selectors on sync and emits no user change event; it works before first rendering. HTML events are routed with HTMLID/HTMLKind/Value. Collision2D/3D messages contain private element names, stage/type/normal/point/penetration, not ECS/body/collider handles. Module cameras are private; fallback Camera2D is active only without a custom 2D camera.

## Behavior ownership and deferred structure

Each mount creates independent behavior, slots and private entities. Context exposes no World or foreign module/scene objects. Root and Spawn handles are instance-owned. Get/Set/Remove/Each validate ownership; callers must check Each errors. Engine-maintained Parent/SiblingOrder and cross-module painting cannot bypass Reparent ownership/cycle checks.

Entity creation, new component structure, subtree deletion and module destruction commit after callback/query traversal. Destroying a module also exits its placement descendants. Destroy runs at most once. Child placements attach to parent placement proxies, independently from module-local roots; local animation does not move scene child placements.

OpenResource accepts a declared private resource name; OpenFile accepts a private inline/file path with the same compiler boundary. Visual sharing uses RenderOutputHandle requests instead of exposing provider paths/fonts/entities.

## Typed messages and input

Send/Emit/request/Reply always queue. Dispatch freezes a batch and callback-generated messages enter the next batch. BindEmitter[T] resolves reusable typed storage during Install. Topic names the message and From is the original instance ID, empty for scenes/untargeted keyboard input. Addressing stays within the current scene instance; unknown/destroyed targets return ErrUnavailable, disabled targets ignore delivery. Receivers handle unknown topics/type mismatches explicitly.

Scene Forward directly transfers the current borrowed payload during delivery, preserving From; ForwardAs additionally renames the topic. Module Forward queues for the next batch. Forward is valid only inside current OnMessage, and requests forward to at most one target. Message/Value borrows expire with the callback. Pointer/slice payloads must remain immutable until delivery finishes. No message JSON codec or reflective dispatch is involved. Applications send via SceneEmitter[T].

Input first reaches the owning scene's OnMessage under InputTopic with module.Input. Raw Entity/Path are cleared; Target retains ownership and Element/HTML fields preserve semantics. There is no automatic module Input hook or input.emit dispatch. Scenes explicitly forward keyboard input or translate clicks into domain messages. Collision emit names likewise route to the scene for orchestration.

## Slots and requests

ReadSlot[T] decodes registered values. Set mutates only owned slots, increments revisions and notifies only when values change. SlotValue.Fields is an immutable borrow; write with Set. Slot roots are owned private handles. UI content/slots default to top-left anchors independently from placement.

Application SlotWriter queues built-in writes for the next ModuleEvents batch, validating writable/type/liveness/capacity. Structured payloads copy into type-reserved storage, so later caller mutation cannot change queued data; alternating structure types stays warm allocation-free. Modules cannot obtain foreign writers and must ask receivers through messages.

BindRequester[Request,Result] binds types; SendTo addresses a service, Send routes through the scene. RequestValue exposes typed requests; Reply or Fail queues completion. ResultValue exposes ID/value/error through the requester's OnMessage under the original topic, even after scene ForwardAs renaming. Direct requests/replies require at least two batches. Timeout ticks default to 120; zero selects default. Cancel affects owned requests; orchestration can Runtime.Cancel. Destroying requesters releases pending records; destroying services cancels unanswered requests while already queued replies can still arrive.

Stale IDs/addresses, duplicate replies, timed-out tokens and stale revisions fail safely. Revision guards combine up to four slots belonging to one requester; ErrStaleRevision prevents overwriting newer input. Default capacities are 256 queued messages, 128 pending requests and 128 structural commands. ErrQueueFull/ErrPendingFull/ErrCommandFull require handling. Failed Reply retains reply ownership and can retry before timeout.

## Font services and visual leases

Built-in TextRenderRequest/Result, SceneSwitchRequest/Result and SceneStartup protocols need no project type/RegisterValue declarations. Visual messages carry RenderOutputHandle; JSON starts null. FontService.Load opens the service's own named font; OnMessage reads typed render requests and FontService.Render returns immutable output, and Destroy releases service state. Content-keyed caches share decoded fonts, glyph atlases and geometry. Keys include text/size/color/constraints/alignment. Width controls wrapping; positive width/height also clip output and zero is unbounded. Results include logical dimensions.

OnSlotsChange coalesces multi-slot writes to one next-batch callback, avoiding Update polling. Up to four revision guards protect font replies. SetVisual retains an independent slot lease and releases previous output; null clears. Visual returns a borrow. Successful Reply transfers sender ownership to the queue; failure leaves it with the sender. Unused/cancelled/expired/stale/unhandled/destroyed queued outputs release automatically. Copying a handle borrows; Retain creates an owned lease requiring Release.

Slots/modules release leases on destruction. Consumers supply transform, anchor, clip and scene paint order; outputs contain no provider transform/mutable callback. Delivered output survives provider exit until its last lease releases font/atlas resources; GPU retirement remains deferred. OutputCapacity defaults to 512 leases and TextCacheCapacity to 128 entries per font with LRU eviction of the cache's own lease. Live consumers remain valid. Exhaustion returns scene.ErrOutputCapacity. New glyphs/text are cold paths.

## Scene preparation, retention and overlays

Only scene handlers may c.Switch; module calls return ErrOwnership. Applications use Manager.Switch. Targets progress Loading→Ready→Active. Preparation privately delivers scene.startup plus service requests/replies while saving foreground queues/payloads/pending clocks. Target Update does not run. Default startup budgets are 32 batches/4096 events. Install/handler/budget failures cancel switching, leaving the source Active and delivering scene.switch.result; success reaches the new scene and retained sources.

Default switching destroys the source. Retain keeps behavior/slots/leases, hides content, stops logic/input/messages and cancels pending requests. Resume reuses generation without repeating Install/startup; failed resumes remain Suspended and paused messages are not replayed. Overlay implies retention and keeps the old scene rendered while pausing updates, animation, physics, managed resources and input. Scene camera/light/order isolation remains; foreground swapchain cameras preserve background, offscreen targets keep their clear rules. Backgrounds remain live render content rather than frozen screenshots.

Manager.Preload synchronously installs, dispatches startup, lays out HTML, updates transforms and warms static text without a window, leaving Ready outside normal updates/rendering. Foreground queues, requests and clock survive. Activate can start without a current scene; StartEntry reuses preloaded entry instances. Dispose discards a preload. Enter runs before first activation; Resume before restoration; their preparation messages can fail commit. Preload startup has empty From, while Enter/Resume receive the actual source/activation data. Exit then Suspend run when leaving retained foreground; Destroy runs finally. Business side effects of hooks are not transactionally rolled back.

A structural/hierarchy traversal builds logic scope, rendering scope, full scene ownership, per-module entity lists and parent adjacency, including generated HTML/scroll descendants. Ready/hidden Suspended scenes leave ordinary queries; overlay backgrounds enter render scope only. Stable scopes/caches reuse storage. ContentQuery/ContentQuery2 iterate owned lists; cached ownership avoids repeated parent walks. Direct Get/Set, explicit Query.In and Runtime.Mount are ownership/loading tools rather than replacements for Manager scheduling.

Existing component replacements are in-place. New components, Content.Spawn/SpawnNode variants, reparent/remove/destroy defer until traversal ends. Standalone queries commit on exit; nested queries share outer boundaries; queries inside Update/OnMessage commit at runtime batch boundaries. New entities can initialize within the batch but do not enter its active query. Typed command storage and prepared bundles reuse warm capacity. Command overflow returns ErrCommandFull; commit errors on query exit panic.

Foreground and preparation use independent double-buffered typed payload storage. Preparation success/failure clears its payloads/leases without consuming foreground capacity or altering request time. RegisterValue registers slot decoders only; actual bound message types preallocate shared storage, and only used stores clear per batch. Preload is synchronous and may decode, grow capacity or upload on first use; it is not asynchronous loading or a zero-cost first GPU frame.

## Validation scope

Default content covers private sprites/text, images/atlases, animation, HTML, Spine, particles, models, cameras and physics; generated Options.Resources chooses decoders. Options.Content can replace the installer. Examples are external and communicate through domain messages rather than implementation imports.

Recorded warm allocation tests cover events, ordinary/structured slots, alternating structures, requests, updates, visual leases/replacement, cached font requests and complete font/button frame chains. Loading, entity creation, type registration, reservation, first glyphs and new capacity remain cold. Use explicit package lists for headless validation; do not run all repository tests or editor screenshots.

## Examples and commands

```powershell
go run ./editor/cmd/onix new -width 750 -height 1624 ../my-game
go run ./editor/cmd/onix check ../my-game
```

```text
demo/
  main.go
  onix_generated.go
  resources/
    project.json
    scenes/
      main.json
```

```json
{
  "resources": {
    "ui": { "kind": "font", "path": "ui.ttf" },
    "metadata": { "kind": "data", "path": "custom/font-info.json" }
  }
}
```

```powershell
# check is read-only; generate, Play and Build update generated files.
go run ./editor/cmd/onix check ../my-game
go run ./editor/cmd/onix generate ../my-game
go run ./editor/cmd/onix build -platform windows -mode release ../my-game
go run ./editor/cmd/onix build -platform web -mode release ../my-game
go run ./editor/cmd/onix build -platform android -mode release ../my-game
```

```go
func (m *Mod) Install(*module.Context) error
func (m *Mod) Enter(*module.Context, module.SceneStartup) error
func (m *Mod) Exit(*module.Context) error
func (m *Mod) Suspend(*module.Context) error
func (m *Mod) Resume(*module.Context, module.SceneStartup) error
func (m *Mod) Update(*module.Context, module.Tick) error
func (m *Mod) OnMessage(*module.Context, module.Message) error
func (m *Mod) Destroy(*module.Context) error

```

```json
{"keep":["button/dynamic/**"],"packages":[{"name":"font","include":["font/**"]}]}
```

```go
plan, err := project.CompileProject(source, ".")
plan, err := project.CompileProjectDirectory(directory)
plan, err := project.CreateProjectDirectory(directory, project.CreateProjectOptions{
    Width: 750, Height: 1624,
})
```

```go
app := onix.NewApp()
entry, err := app.LoadModules(plan, registry, module.Options{
    Source: os.DirFS("."),
    EventCapacity: 256,
    RequestCapacity: 128,
    CommandCapacity: 128,
    TimeoutTicks: 120,
})
```

```go
plan, source, err := project.OpenModuleProject()
registry, err := OnixRegistry()
app := onix.NewApp()
if err := OnixInstallSystems(app); err != nil { panic(err) }
if err := OnixOpenWindow(app, plan.Spec, source); err != nil { panic(err) }
_, err = app.LoadModules(plan, registry, OnixModuleOptions(source))
```

```go
type Mod struct { text module.Slot }

func (m *Mod) Install(c *module.Context) error {
    var err error
    m.text, err = c.Slot("text")
    return err
}
// Update, Input and Destroy are optional. onix_generated.go registers the behavior.
```

```go
// Send by instance ID from any callback; module.SceneTarget addresses the current scene.
err := module.Send(c, "status", "setText", "Ready")

// Bind frequent emitters once in Install; generic double buffers store typed payloads.
changed, err := module.BindEmitter[int64](c, "changed")
err = changed.EmitTo("simulation", 1)
err = changed.Emit(1) // Deliver to the current scene.

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
    if message.Topic != "setText" { return nil }
    value, ok := module.Value[string](message)
    if !ok { return module.ErrType }
    return m.text.Set(project.SlotValue{Type: project.StringValue, String: value})
}
```

```go
func (m *Main) OnMessage(c *module.Context, message module.Message) error {
    if message.Topic != module.InputTopic { return nil }
    value, ok := module.Value[module.Input](message)
    if !ok { return module.ErrType }
    if message.From == "start" && value.Event.Kind == scene.InputEventTap {
        return module.Send(c, "simulation", "start", module.Signal{})
    }
    // The scene explicitly chooses modules that receive keyboard input.
    if message.From == "" { return c.Forward("keyboard", message) }
    return nil
}
```

```go
// Bind types in requester Install; no port, reply callback or route configuration is needed.
requester, err := module.BindRequester[string, int64](c, "length")
id, err := requester.SendTo("service", "hello", module.RequestOptions{TimeoutTicks: 120})

// Inside the service OnMessage:
if message.IsRequest() && message.Topic == "length" {
    request, ok := module.RequestValue[string, int64](message)
    if !ok { return module.ErrType }
    return request.Reply(c, int64(len(request.Value)))
}

// Inside the requester OnMessage:
if message.IsReply() && message.Topic == "length" {
    result, ok := module.ResultValue[int64](message)
    if !ok { return module.ErrType }
    if result.Err != nil { return result.Err }
    // result.ID and result.Value belong to this request.
}
```

```go
// Reply handles are borrowed; SetVisual retains an independent slot lease.
if result.Err != nil { return result.Err }
return label.SetVisual(result.Value.Output)
```

```go
manager := app.Modules().Manager()
ready, err := manager.Preload(module.ScenePreloadRequest{Scene: "game.json"})
if err != nil { return err }
// Commit activation at a safe application boundary, reusing loaded entities.
return manager.Activate(ready, module.SceneActivationOptions{Retain: true})
// The current scene can also call c.Switch; matching Ready instances are reused.
```

```powershell
# Validate without opening a window.
go run ./editor/cmd/onix check ../my-game
# Manual execution: run go run . from the application root.
```

```powershell
go test . ./module ./project ./editor/cmd/onix -count=1
go test ./scene -run 'Test(RenderOutput|Text2D|DrawScreenText2D)' -count=1
go test ./editor -run '^(TestWorkspace|TestModuleBuild)' -count=1
go test ./module -run '^$' -bench 'Benchmark(Dispatch|SlotUpdate|StructuredSlotEvent|RequestReply|VisualReply|CachedFontRequest)$' -benchtime=200ms
```
