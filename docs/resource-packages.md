# Resource packaging and named packages

The builder collects resources reachable from project manifests, preprocesses them for the target and writes encoded blobs into indexed `.dat` files. Packages are neither ZIP nor an additional compression layer: `ReaderAt` and read-only mmap can address individual ranges directly.

Default output is `resources/catalog.bin` plus `resources/base.dat`. `ONIXCAT2` owns logical paths and records package names, files, size, SHA-256, dependencies, startup flags and resource size, CRC32 and 128-bit ContentID. `ONIXDAT5` indexes entry order, data ranges and alias flags without repeating logical paths. Matching ContentID and size share one blob. Sorted inputs produce reproducible output.

## Build preprocessing

Manifest decoding rejects unknown fields and invalid paths. Statically compilable structures become typed Onix binaries, avoiding runtime JSON field scanning and normalization. Scene spawn plans resolve hierarchy, camera references and animation bindings to indexes and precompute component presence, base transforms, UI layout, colors and static rendering parameters. Numeric animation tracks validate on bundle load; texture keys resolve on scene preload or first instantiation.

Tilemap RLE is used only if smaller. Spine skeletons/atlases link into one IR containing mipmap options; source atlases are omitted. TexturePacker metadata becomes `ONIXATL1`. Release PNG recompression must preserve pixels and reduce size. Pure Go `github.com/gogpu/naga` compiles WGSL into Vulkan, Metal or WebGL2 target forms without CGO or external tools. Android retains Vulkan and GLSL ES fallback artifacts; WebGPU retains WGSL. Package configuration is build-only. Encoded media and platform shaders retain their formats; Wasm Brotli compression is unrelated to `.dat`.

## Incremental builds

Cache output is `<project>/dist/.onix-resource-cache/<platform>-<mode>/`. Fingerprints cover reachable input content, package rules, target, mode, pipeline version and catalog/package versions. Reuse verifies size and SHA-256 before skipping preprocessing and packaging. Cache metadata reports fingerprints, files and bytes per package, stored blobs, deduplication savings and format versions; it is excluded from published output.

## Paths and configuration

Logical paths use `/`. Absolute paths, backslashes and escapes outside the repository resource root are rejected. Project resource references use the project root; permitted `../` references must remain inside that root boundary. Scene-list paths are relative to `resources/`; prefab paths are project-relative without parent traversal. Internal atlas, glTF, OBJ, MTL and JSON references resolve only relative to the referring file. glTF/GLB dependencies come from `buffers[].uri` and `images[].uri`, never names, descriptions or extras.

Resources unmatched by named packages enter `base.dat`. Configure `resources/packages.json`, also editable through Project Settings → Resource Packages. `*`, `?` and character classes match one segment; `**` spans levels. Every include must match a file, and a file cannot belong to two named packages. Named packages automatically depend on `base`.

`keep` retains untraceable dynamic paths. `name` accepts lowercase letters, digits, `-` and `_`. `include` is required and both collects files and assigns ownership; `exclude` subtracts matches. `dependencies` must load first. `required` defaults to false and marks startup packages. Dynamic files already included by a package need no duplicate keep rule. Startup manifests and package configuration are assigned to base.

## Runtime ownership

Desktop uses read-only mmap; other targets use `ReaderAt`. Opening, reading and validation do not hold a global Manager lock. Independent packages load concurrently; duplicate loads coalesce into one I/O. Mounting checks size, headers and index bounds without scanning the package. First `Blob.Bytes`, `Blob.Open` or `Blob.Validate` validates CRC32/ContentID; later access reuses validation state.

`Blob.Bytes` borrows immutable bytes until close; `ReadAll` returns an owned mutable copy; `Open` returns a bounded seekable reader. Live blobs pin packages. Unloading a pinned package returns `ErrPackageInUse`; required packages and dependencies with live dependents cannot unload. `Manager.Statistics` reports bytes, pins, load/unload counts and duration. Project `Resources.Statistics` reports generic cache hits, misses and resident objects.

Textures cache by ContentID and `TextureOptions`, sharing decode/upload across paths, images, atlases and Spine when options match. Per-key synchronization allows unrelated first loads concurrently. Fonts and Vulkan shaders retain blob pins until `Resources.Dispose`. Video is read-only. Streaming clips retain reopenable sources; each MP3 player has a section reader without a cached encoded copy, and `Clip.Close` releases its last pin after readers close. Typed loaders pair with disposers. Bundle disposal releases shaders, meshes, clips, fonts, video, shader blobs and shared textures. Unloaded optional-package errors do not poison the cache; retry after loading.

Package SHA-256 supports build/transport integrity, CRC32 detects corruption and ContentID identifies/deduplicates content. They provide neither signatures nor encryption. WOFF2 Brotli, media coding and `main.wasm.br` are separate compression mechanisms.

## Examples and commands

```text
resources/
  catalog.bin
  base.dat
```

```text
<project>/dist/.onix-resource-cache/<platform>-<mode>/
```

```json
{
  "keep": ["resources/runtime/**"],
  "packages": [
    {
      "name": "common",
      "include": ["resources/common/**"],
      "required": true
    },
    {
      "name": "chapter-1",
      "include": ["resources/chapters/01/**", "../shared/chapter-1/**"],
      "exclude": ["resources/chapters/01/source/**"],
      "dependencies": ["common"]
    }
  ]
}
```

```go
manager, err := resource.OpenManager("resources")
if err != nil {
	return err
}
defer manager.Close()

if err := manager.Load("chapter-1"); err != nil {
	return err
}
blob, err := manager.OpenBlob("resources/chapters/01/map.bin")
if err != nil {
	return err
}
defer blob.Close()
data, err := blob.Bytes() // Immutable borrowed view; mmap does not copy
```
