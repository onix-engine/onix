# Internal temporary workspace reuse

Recorded 2026-10-17 against `a11936948c69f23b71af3e0047db542008af8951`, preserving independent ownership of returned results. typedbin pools each active map's sorted entries and contiguous encoded keys; nested maps borrow separately. Keys use offsets so growth remains safe. Return clears reflected references and caps retention at 4096 entries/256 KiB key data. Cycle maps initialize lazily, struct bitmap checks merge with encoding, and map decode reuses/zeros reflected key/value objects while preserving nil/empty/zero semantics.

BC/ASTC mip encoding shares one flow writing directly into final output. One pooled RGBA buffer shrinks in place between levels, retaining at most 4 MiB; 1×1 images need no scratch. Warm paths usually allocate only caller-owned output. GC eviction/growth/oversized textures can recreate scratch.

BuildWithFonts pools layout contexts and nested text workspaces, clearing DOM/style/font/float/error references on return. Limits are 4096 array items, 32 nested workspaces and 256 image entries. High-water used lengths clear flushed long-line references without repeatedly clearing a whole inherited capacity for short paragraphs. Control tokens clear only used ranges. Incremental Cache still owns its document workspace. Generic reuse.Reset centralizes bounded slice cleanup.

Go 1.26/Linux amd64/i5-11500 measurements alternate six 200ms CPU-0 runs, with layout retested after cleanup-range optimization. Nested map encoding bytes fall about 54%, allocation count 64%; decoding allocations about 28%. BC1/BC5 mip chains drop 18 allocations to about one, bytes about 72%/66%; ASTC bytes about 42%. Full text layout saves about 37.5 KB/20 allocations. Output/reflection allocations remain. Small time changes are noisy; controls median rises 2.2%, avoiding an earlier approximately 13% full-capacity-clear regression.

Targeted codec/layout/style/reuse/HTML and race checks passed. Regressions cover deep maps, canonical order, nil/empty distinctions, cycles, error recovery, concurrency, independent outputs, unchanged pixels and released layout references. A historical Spine fixture path correction allowed its tests to pass. No editor or screenshot checks were performed.

## Examples and commands

```sh
go test ./internal/typedbin ./internal/texcompress ./internal/layout \
  -run '^$' \
  -bench 'Benchmark(CodecScratch|EncodeChain|Build.*Document|RetainedBlockLayout)$' \
  -benchmem -benchtime=200ms -count=6 -cpu=1
```

## Recorded measurements

| Benchmark | Time µs/op, before → after | Time change | B/op, before → after | allocs/op, before → after |
| --- | ---: | ---: | ---: | ---: |
| `CodecScratch/StructSlice/Marshal` | 69.33 → 65.46 | -5.6% | 17976 → 17928 | 14 → 13 |
| `CodecScratch/StructSlice/Unmarshal` | 89.68 → 92.75 | +3.4% | 43056 → 43056 | 771 → 771 |
| `CodecScratch/NestedMaps/Marshal` | 187.05 → 177.61 | -5.0% | 101288 → 46596 | 2191 → 781 |
| `CodecScratch/NestedMaps/Unmarshal` | 153.07 → 130.86 | -14.5% | 131168 → 107760 | 1797 → 1287 |
| `EncodeChain/bc1-srgb` | 3866.89 → 3845.49 | -0.6% | 180240 → 50246 | 18 → 1 |
| `EncodeChain/bc5` | 6086.91 → 6019.42 | -1.1% | 264900 → 90116 | 18 → 1 |
| `EncodeChain/astc-4x4-srgb` | 981.78 → 944.23 | -3.8% | 155648 → 90115 | 2 → 1 |
| `BuildTextDocument` | 1217.80 → 1181.79 | -3.0% | 1363584 → 1326069 | 6311 → 6291 |
| `BuildFlexDocument` | 1020.88 → 970.25 | -5.0% | 1085738 → 1080725 | 1913 → 1900 |
| `BuildControlsDocument` | 542.08 → 554.16 | +2.2% | 852618 → 849820 | 1461 → 1450 |
| `RetainedBlockLayout/nodes=10/retained=false` | 32.96 → 31.14 | -5.5% | 38920 → 36466 | 182 → 172 |
| `RetainedBlockLayout/nodes=10/retained=true` | 3.99 → 4.15 | +4.0% | 8192 → 8192 | 1 → 1 |
| `RetainedBlockLayout/nodes=1000/retained=false` | 3270.38 → 3071.72 | -6.1% | 2668536 → 2666184 | 16923 → 16914 |
| `RetainedBlockLayout/nodes=1000/retained=true` | 60.61 → 60.18 | -0.7% | 8192 → 8192 | 1 → 1 |

Raw records: [internal-memory-reuse-after.txt](internal-memory-reuse-after.txt), [internal-memory-reuse-before.txt](internal-memory-reuse-before.txt).

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
