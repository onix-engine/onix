# UI and renderer nested-traversal review

The review covers UI/HTML/markdown and renderer Go/C/Objective-C/shaders. Button groups build width prefixes during layout so dividers cost O(buttons+dividers), replacing quadratic sums. HTML offset/scroll bindings build registration-order indexes and shared ancestor-route caches, O(B+V+E) instead of O(E×B×H), preserving last-offset/first-scroll priority. Scroll-membership sets prepare before emission and refresh live viewports afterward.

Animation matching indexes old tracks by name and consumes in reverse, preserving duplicate-name time matching in O(new+old). Hidden-ancestor results share generic caches, O(K+V), retaining hide cancellation/show restart. Mutation roots collect candidates before resolving ownership/selected ancestors, avoiding repeated pair comparisons. Reorder/clip roots share coverage caches; nearest-box/document-membership caches avoid repeated style lookup/cleanup walks and clear at operation boundaries.

Attribute synchronization indexes normalized new names, preserving scope classes/update order. Keyed deletion collects removed nodes then filters siblings once, O(K+S), preserving static siblings/parents/nested cleanup. Multi-root unmount groups by parent and synchronizes text once, with lifecycle callbacks before removal; resource classification still inspects ancestors/subtrees. Saved EventNodes restrict event unbinding to registered nodes while preserving within-node order.

Metal validates the entire texture-update batch and records each texture's final update index, generating mipmaps there in O(K) without temporary allocation. Generic ancestor caches reuse maps/path slices, clear references immediately after fill and clear transient state after operations/source replacement. More retained cache capacity replaces repeated walks; first growth still allocates.

Go 1.26/Linux amd64/i5-11500 uses an overlay baseline, three sequential 200ms runs. Listed optimized cases are 0 B/op/0 allocs/op; 100→1,000 items scales approximately tenfold versus roughly hundredfold for previous quadratic cases. Metal, track matching and batch removal have no independent timing; matching has warm allocation assertions.

UI/HTML/markdown tests passed. Historical renderer statistics expected 1600/164 upload bytes but observed 1664/228; HEAD overlay reproduced it and Linux does not compile the only changed Metal file. Objective-C extracted production structures/update code passed strict syntax checks with declaration stubs, not Apple SDK/hardware validation.

Fixed texture/matrix/influence loops, disjoint child/row/pixel work, progressing GLES draw indexes, filter clamping and Gaussian kernels remain necessary. Hydration root matching still allows wildcard refs and is worst-case children×template roots; ordinary equal-key hashing/KMP cannot preserve that predicate. Full animated transform/color/opacity ancestor composition remains separate work. Editor/screenshots and all-package tests were not run.

## Examples and commands

```sh
go test ./ui/html ./ui -run '^$' -bench 'BenchmarkHTML(NodeBinding|MutationRoot|AnimationVisibility|ComponentAttribute)Traversal|BenchmarkButtonGroupDividerTraversal' -benchtime=200ms -count=3 -benchmem
```

## Recorded measurements

| 1,000-item workload | Before | After | Speedup |
| --- | ---: | ---: | ---: |
| 1,000 nodes with 1,000 offset bindings | 1447.81 μs | 159.14 μs | 9.10× |
| 1,000 independent mutation roots | 1404.23 μs | 71.04 μs | 19.77× |
| Hidden ancestors on 1,000-node chain | 4629.78 μs | 47.63 μs | 97.20× |
| Match 1,000 retained attributes | 6736.92 μs | 43.42 μs | 155.14× |
| 1,000 buttons and 999 dividers | 40135.25 μs | 573.38 μs | 70.00× |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
