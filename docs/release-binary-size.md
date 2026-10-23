# Release binary size

Historical measurements from 2026-09-30 used Go 1.26.0 on Linux amd64 and MinGW-w64
for Windows amd64. A small independent 2D game used the same resources, toolchain,
`onix_release`, `-trimpath`, and `-s -w` throughout. Windows measurements include
the default icon in the actual Release Portable executable. Sizes are bytes;
sections come from `objdump -h`.

| Windows artifact | Before | Round 1 | Round 2 | Total reduction |
| --- | ---: | ---: | ---: | ---: |
| Executable | 6,903,296 | 6,591,488 | 6,154,752 | 748,544 (10.84%) |
| `.text` | 2,868,096 | 2,714,976 | 2,526,080 | 342,016 (11.92%) |
| `.rdata` | 3,592,464 | 3,457,168 | 3,220,304 | 372,160 (10.36%) |
| `.data` | 251,200 | 233,760 | 233,760 | 17,440 (6.94%) |
| `.pdata` | 65,256 | 62,040 | 58,584 | 6,672 (10.22%) |

Round 2 removed another 436,736 bytes (6.63%) from Windows, including 236,864 bytes
of rdata (6.85%). Disabling runtime scene/prefab source compilation alone produced
6,426,112 bytes; profile-specific UI pruning produced 6,159,360; removal of source-only
JSON methods yielded the final result above.

Direct Linux release compilation fell from 6,504,824 to 5,921,336 bytes (8.97%).
Actual packaged Linux executables later fell from 6,060,696 to 5,638,808 (6.96%).
Keep these comparisons separate: packaging generates resource initialization code.

## Linking boundaries

- ImageFeature and IconFeature live in project. Image/icon-only applications avoid
  aggregate project/assets audio initialization; full resources still use
  project/assets.Feature or DefaultResourceFeature.
- resource.Size reads signatures and metadata. JPEG/GIF avoid pixel decoders; PNG
  uses the engine decoder; images/webp/config handles VP8/VP8L/VP8X/animation
  dimensions without global decoder registration. GIF validates palette integrity;
  JPEG skips application/comment segments with a fixed streaming buffer.
- Release scene/prefab manifests require preprocessed spawn plans. The compile-time
  sourcejson.Enabled constant removes source compilation fallback. Development
  loads source formats; explicit scene editing APIs may still link the compiler.
- Load profiles select layout, widget target binding and control cleanup.
  Render2DProfile and Render2DAnimationsProfile exclude these UI operations while
  retaining world/UI hierarchy and animation refresh, without extra traversal or
  closure allocation. Full/configured loaders retain UI support.
- Widget.UnmarshalJSON and ModelMaterialSpec.UnmarshalJSON compile only outside
  onix_release. Preprocessing preserves defaults and explicit-field metadata;
  binary schemas stay the same.

The measured minimal application no longer linked audio, Oto, PureGo, MP3/FLAC/
Vorbis, WOFF2/Brotli, standard GIF/JPEG/PNG or full WebP decoders. Diagnostic builds
also excluded source scene/prefab compilation, ECS UI layout synchronization,
control composition cleanup and the two JSON methods. External catalog.bin/.dat
resources were byte-identical between rounds.

## Interpreting sizes

Windows rdata includes Go type descriptors, function metadata, names, line tables
and constants, even after DWARF/symbol stripping. Before optimization, diagnostic
runtime.pclntab was about 1.76 MiB and go:func.* about 346 KiB. Further reductions
require reducing linked types/functions, source reflection and backend features.

The roughly 32.3 MiB bss is zero-initialized virtual memory rather than file payload;
32 MiB came from Go 1.26 crypto/internal/fips140/drbg.memory. Measure external fonts,
atlases and resource packages separately.

## Recorded validation

Targeted resource metadata, asset, project, script generation, resource packaging,
prepared-plan, UI cleanup and release-runtime checks passed, along with Windows
and Linux Release Portable builds. Packaged resources loaded and instantiated a
scene with generated bindings without opening an editor/window. Source widget JSON
and release ECS layout tests passed.

The historical complete release-mode project test package did not compile because
model_resource_test.go referenced development-only gltfDocument types. Selected
release regression files and real packaged resources validated the relevant paths.
Some game script tests required a display; only compilation was checked there.
These are historical results, not a claim that every platform suite passes today.

Inspect an independent application's executable and dependencies from its own module:

```sh
objdump -h app.exe
go list -tags onix_release -deps .
go build -p 2 -buildvcs=false -trimpath -tags onix_release -o /tmp/app-symbols .
go tool nm -size -sort size /tmp/app-symbols
```
