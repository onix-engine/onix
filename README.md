# Onix

[English](README.md) | [简体中文](README.zh-CN.md)

Onix is a game and application engine written in Go. It includes a typed ECS,
2D and 3D rendering and physics, HTML/CSS UI, audio, resource packaging, and a
project editor.

The Go module is `github.com/diiyw/onix`; the root package is `onix`.

## Prerequisites

- **Go 1.26 or newer**, with Git to clone the repository.
- **A C/C++ toolchain for native builds**, with CGO enabled and the development
  libraries required by the host platform. Linux windowing uses X11, Xext,
  Xrender, Xi, Wayland, wayland-cursor, and xkbcommon; native macOS development
  requires Xcode Command Line Tools.
- **Target-specific tools for packaging or cross-compilation.** See
  [desktop builds](docs/desktop-builds.md) and
  [Android builds](docs/android-builds.md) for toolchain and packaging requirements.

## Quick start

Run these commands from the engine checkout. Keep game and application projects
outside this repository:

```sh
go run ./editor/cmd/onix new ../my-game
go run ./editor/cmd/onix check ../my-game
go run ./editor/cmd/onix ../my-game
```

The last command opens the editor for the new project. Use **Play** to build and
run it, or **Build Project** to package it. Running `go run ./editor/cmd/onix`
without arguments opens the editor's New/Open workflow.

When the destination has no enclosing Go module, project creation generates a
`go.mod` with a local `replace` pointing to this engine checkout. To set the
application module path and initial viewport explicitly:

```sh
go run ./editor/cmd/onix new -module example.com/my-game -width 1280 -height 720 ../my-game
```

### Project layout

```text
my-game/
  go.mod
  go.sum
  main.go
  onix_generated.go
  resources/
    project.json
    scenes/
      main.json
```

`main.go` belongs to the application. `onix_generated.go` contains generated
registration and engine setup through `OnixRegistry`, `OnixInstallSystems`,
`OnixModuleOptions`, and `OnixOpenWindow`; regenerate it instead of editing it.
Project creation starts with an empty scene. Keep project JSON and assets in
`resources/`, and add module definitions and behavior using the
[module authoring guide](docs/module-system-authoring.md).

### Command reference

All commands below run from the engine checkout. Options precede the project
directory.

| Task | Command |
| --- | --- |
| Create a project | `go run ./editor/cmd/onix new ../my-game` |
| Validate a project | `go run ./editor/cmd/onix check ../my-game` |
| Regenerate registration | `go run ./editor/cmd/onix generate ../my-game` |
| Open a project in the editor | `go run ./editor/cmd/onix ../my-game` |
| Build a desktop release for the host platform | `go run ./editor/cmd/onix build -mode release ../my-game` |
| Build a Web release | `go run ./editor/cmd/onix build -platform web -mode release ../my-game` |

`generate` preserves the application's `main.go`. Builds collect and preprocess
resources, then produce platform-specific artifacts under the project's `dist/`.
Release builds use the `onix_release` tag and binary resource packages; use the
build command or editor to prepare them. Build targets are `windows`, `macos`,
`linux`, `web`, `android`, `ios`, and `wechat`, subject to their host and toolchain
requirements.

## Source map

| Path | Responsibility |
| --- | --- |
| Root package `onix` | Application lifecycle, scheduling, and engine integration |
| [`ecs/`](ecs/) | Typed components, queries, systems, and structural commands |
| [`module/`](module/) | Module lifecycles, typed messages, slots, and scene management |
| [`project/`](project/) | Project validation, compilation, and resource preprocessing |
| [`editor/`](editor/) | Project editor, code generation, build pipeline, and CLI |
| [`scene/`](scene/), [`camera/`](camera/), [`physics/`](physics/) | Scene components, cameras, and 2D/3D physics |
| [`render/`](render/), [`graphics/`](graphics/) | Rendering backends, graphics data, and image codecs |
| [`ui/`](ui/), [`text/`](text/), [`font/`](font/) | HTML/Markdown UI, text layout, and fonts |
| [`resource/`](resource/), [`fs/`](fs/) | Resource packages and file access |
| [`window/`](window/), [`input/`](input/), [`audio/`](audio/) | Platform windows, input, and audio |
| [`tests/`](tests/), [`testdata/`](testdata/) | Integration checks and test fixtures |

Platform bridge names use the `Onix` prefix; build tags and environment variables
use `onix_` and `ONIX_` respectively.

## Documentation

Start at the [documentation index](docs/README.md) for usage, design and
performance guides, each with a Simplified Chinese version. Focused API and
platform references are listed in the [usage guide](docs/usage.md).

## Engine development

Read [AGENTS.md](AGENTS.md) before changing the engine. Prefer concise generic
implementations, minimize allocations on warm paths, and combine traversals when
possible. HTML rendering changes must follow W3C standards and align with Chromium.

Run tests for the packages affected by a change. For example, for ECS work:

```sh
go test ./ecs
go test ./ecs -run TestName -count=1
```

Replace `TestName` with the relevant test name or pattern. **Do not run
`go test ./...`**; the repository-wide suite can freeze the machine. Visual
acceptance is manual; do not launch the editor for automated screenshot testing.

## License

Onix is licensed under the [MIT License](LICENSE). You may use, modify, compile,
and distribute the engine, including in commercial games and software, subject
to the license's notice requirements. Third-party code and assets retain their
own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md).
