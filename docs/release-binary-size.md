# Release binary size

## Flappy Bird module profiles (2026-10-29)

Measured on Linux amd64 with Go 1.27.3, the standard Portable release builder,
`onix_release`, `-trimpath`, and `-s -w`. These measurements use the current
module-based Flappy Bird example and are separate from the historical sample
below. All values are bytes; unpacked totals sum regular file lengths.

| Artifact | Before | After | Reduction |
| --- | ---: | ---: | ---: |
| Linux tar.gz | 10,782,136 | 2,971,655 | 72.44% |
| Unpacked application | 22,778,563 | 7,664,881 | 66.35% |
| Executable | 11,934,000 | 7,361,840 | 38.31% |
| base.dat | 10,841,940 | 300,420 | 97.23% |
| UI font | 10,559,284 | 17,764 | 99.83% |
| ELF .text | 5,547,420 | 3,027,100 | 45.43% |
| ELF .gopclntab | 3,943,157 | 2,451,380 | 37.83% |
| ELF .go.type | 1,512,528 | 1,067,504 | 29.42% |

At the time of these measurements, module creation selected `module.Render2D`,
an animation profile, or `module.FullContent`. The current API composes additions
with `module.WithContent[P, F]`, for example
`module.WithContent[module.Render2D, module.AnimationsContent]`, through
`module.New[P]` / `app.LoadModules[P]`. The size figures above predate that
composition change. Generation
uses the project's required runtime features for the `OnixContentProfile` alias.
The selected lifecycle includes component construction, async preload, layout,
deformation, upload collection, HTML input metadata and cleanup. Unsupported
components fail explicitly instead of falling back to the full loader.

Optional systems register their idle counters and shutdown/input callbacks when
installed. The generic app loop and Destroy no longer retain all HTML, Spine,
physics ECS and 3D implementations. Module-specific model, animation, HTML,
rig and particle caches are allocated only when used. FullContent uses the same
2D component builders as the minimal profiles.

The final Flappy Bird binary contains no ui/html, Spine or scene/render3d text
symbols. Its private simulation directly uses the physics World; approximately
58 KiB of physics machine code remains an actual game dependency. Tiny HTML
style/layout helpers and type metadata may remain through shared data types.
Package-level import membership is not a measure of linked machine code.

The example font is an ASCII/nonbreaking-space subset of the original Noto Sans
SC font, with the original license and font metadata retained. All printable
ASCII glyph outlines, advances and line metrics are compared against the source
font by a headless test. Regeneration is documented beside the font. Image and
atlas aliases remain deduplicated; the final DAT has 7 logical resources and
5 unique contents.

Validation covered the root app, project, module, module/audio, Flappy Bird and
its simulation, 2048, Pelican and Spine package tests; editor module/build and CLI
checks; profile rejection and cleanup; async preload; zero-allocation warm
Flappy Bird routing; and generated release linking with forbidden optional
implementation symbols. Packaged resource hashes/checksums passed. A diagnostic
build with the identical native resource overlay had byte-identical .text and
identical shared ELF section sizes. Visual acceptance was not run.

The WebView example's existing `TestWebViewModuleOwnsPageBoundsBridgeAndLifecycle`
fails with "suspended browser remained visible". It also fails with the original
HEAD sources restored through a read-only Go overlay; it is not attributed to
this change. Editor tests change HOME, so independent build tests were run with
explicit GOMODCACHE, GOPATH and GOCACHE pointing to the existing toolchain caches.

## Historical measurements

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
