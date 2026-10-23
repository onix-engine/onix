# Desktop builds and installers

The Build Project panel supports Linux, Windows and macOS. Output lives under
`dist/<project-name>/<platform>/`, excluding the project's parent directories.
Every format includes binary `catalog.bin` and `.dat` resource packages. Staging,
generated Go source and `overlay.json` are removed after building. Release uses
`onix_release`; manifests, ordinary atlases and Spine require preprocessed binary
resources. See [release binary size](release-binary-size.md) for linking boundaries.

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

Linux cross-compilation requires MinGW-w64, `VCPKG_ROOT` and static target codecs.
Default triplets are `x64-mingw-static` / `arm64-mingw-static`; override with
`ONIX_VCPKG_WINDOWS_TRIPLET`. For amd64:

```sh
vcpkg install --triplet x64-mingw-static aom libvpx libyuv
```

Codecs link statically, so ZIP/MSI packages need no extra codec DLLs. Both copy
the project's `THIRD_PARTY_NOTICES.md` when present.
