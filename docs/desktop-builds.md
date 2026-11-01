# Desktop builds and installers

The Build Project panel supports Linux, Windows and macOS. Output lives under
`dist/<project-name>/<platform>/`, excluding the project's parent directories.
Every format includes binary `catalog.bin` and `.dat` resource packages. Staging,
generated Go source and `overlay.json` are removed after building. Release uses
`onix_release`; manifests, ordinary atlases and Spine require preprocessed binary
resources.

The Renderer row selects exactly one backend for the build. Defaults also apply
to direct `go build` commands without a renderer tag:

| Platform | Default | Other choices |
| --- | --- | --- |
| macOS | Metal | Vulkan, OpenGL |
| Linux | Vulkan | OpenGL |
| Windows | DirectX 11 | Vulkan, OpenGL |
| Web | WebGPU | WebGL 2 |
| WeChat | WebGL 2 | — |

For direct builds, select one of `onix_metal`, `onix_vulkan`, `onix_directx`,
`onix_opengl`, `onix_webgpu`, or `onix_webgl`. For example:

```sh
go build -tags onix_vulkan ./path/to/app
go build -tags onix_opengl ./path/to/app
GOOS=js GOARCH=wasm go build -tags onix_webgl ./path/to/app
GOOS=js GOARCH=wasm go build -tags wechat,onix_webgl ./path/to/app
```

`editor.BuildOptions.Backend` accepts the corresponding `render.Backend` constant;
an empty value uses the platform default. `render.CompiledBackend` identifies the
compiled renderer. Conflicting or unsupported renderer tags fail compilation.
Only the selected implementation, native link libraries and builtin shaders are
compiled; custom shader packages and resource caches also use the selected
backend. Initialization errors are reported without loading another backend.
WebGL output excludes the WebGPU JavaScript bridge.

Vulkan on macOS requires MoltenVK; OpenGL uses the system OpenGL framework.
Linux OpenGL uses GLX through X11/XWayland and requires OpenGL development
libraries. Windows OpenGL uses WGL. Android defaults to Vulkan and can select
OpenGL ES with `onix_opengl`; iOS uses Metal.

DirectX builtin shaders embed precompiled DXBC. After editing their WGSL sources,
regenerate HLSL with `go run ./render/internal/hlslgen`, then compile the bytecode
on Windows with `go run ./render/internal/dxbcgen`. Custom shaders compile from
their packaged HLSL when loaded.

Set App Icon in Project Settings. PNG, JPEG, GIF, WebP and SVG are supported;
paths are relative to the project's `resources/`. The default is the editor's
`resources/images/logo.svg`. SVG is rasterized directly at each target size for
ICO, ICNS or PNG packaging on Windows, macOS, Android, iOS and WeChat. Linux and
Web retain SVG; Web additionally generates an Apple touch PNG. The build panel's
fallback icon also accepts SVG.

## Linux

Portable output is `<AppName>-linux-<arch>.tar.gz`, containing the executable and
adjacent `resources/`. Directories and executables use mode `0755`, resources
`0644`. A project-root `THIRD_PARTY_NOTICES.md` is copied to the application root.

Linux builds require a Linux host and CGO development libraries for X11, Xext,
Xrender, Xi, Wayland including wayland-cursor, and xkbcommon. Output dynamically
links the target's graphics and C runtime libraries.

## macOS

App Bundle produces `<AppName>.app`. DMG first builds the app and uses `hdiutil`
to create `<AppName>.dmg` with an `/Applications` link, then removes the temporary
app. Executables live in `Contents/MacOS`, resources and third-party notices in
`Contents/Resources`, with a generated `Info.plist`.

The default Universal architecture includes Intel amd64 and Apple Silicon arm64;
select either separately to reduce size. Universal requires Xcode Command Line
Tools or osxcross `lipo`. App bundles can build on macOS or Linux with osxcross;
DMG requires macOS. Native macOS builds use ad-hoc signing. Developer ID signing,
notarization and stapling remain distribution/CI steps.

Linux cross-compilation also requires `VCPKG_ROOT` and static target `aom`,
`libvpx` and `libyuv` libraries. Default triplets are `x64-osx` / `arm64-osx`;
override with `ONIX_VCPKG_MACOS_TRIPLET`. Target include/lib directories pass to
CGO; codecs link into the app without extra codec dynamic libraries. For example:

```sh
vcpkg install --triplet x64-osx aom libvpx libyuv
```

An osxcross setup generally needs a custom triplet and chainload toolchain;
point `ONIX_VCPKG_MACOS_TRIPLET` at that triplet after installation.

## Windows

Portable produces `<AppName>-windows-<arch>.zip` with `<AppName>.exe`. MSI produces
`<AppName>.msi`, installs to 64-bit Program Files and creates a Start menu shortcut.
MSI tool lookup checks WiX v4 `wix`, then `wixl`, then WiX v3 `candle` and `light`.
Missing tooling produces an installation error.

Windows builds and Play use MinGW-w64 on both Windows and Linux hosts. When
`VCPKG_ROOT` is set, default codec triplets are `x64-mingw-static` /
`arm64-mingw-static`; override with `ONIX_VCPKG_WINDOWS_TRIPLET`. These must match
the MinGW toolchain; MSVC `*-windows-static` libraries are not suitable. Linux
cross-compilation requires `VCPKG_ROOT` and static target codecs. For amd64:

```sh
vcpkg install --triplet x64-mingw-static aom libvpx libyuv
```

Codecs link statically, so ZIP/MSI packages need no extra codec DLLs. Both copy
the project's `THIRD_PARTY_NOTICES.md` when present.
