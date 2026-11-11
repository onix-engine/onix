# Map 存储优化与对比

日期：2026-11-11。基线是本轮开始时的工作区快照，包含当时已有的未提交修改。DOM 属性存储不在本轮修改范围内。

## 实现范围

| 路径 | 调整 |
| --- | --- |
| `ui/html/component.go` | VNode 状态从 8 个 map 合并为 5 个；属性值与删除标记共存，子节点列表与活动标记共存，继续复用列表容量 |
| `ui/tree.go` | 4 个逐节点 map 合并为 1 个 ID→槽位索引；可见性、递归标记和行偏移保存在已有状态切片 |
| `input/state.go` | 键盘／鼠标状态使用泛型紧凑活动列表，标志位合并；组合输入按键复用切片，快照直接复制连续存储 |
| `ecs/scratch.go` | 临时 map 接入已有容量回收机制，记录同一帧多次调用的峰值；持续低使用量后缩容，闲置时清除引用，30 次闲置维护后释放存储 |
| `html/style` | 已支持的 CSS 属性在声明准备时转换为紧凑 ID；级联使用定长槽位索引及可复用值切片，删除声明中的冗余属性字符串 |
| `ui/layout.go` | 深度缓存与递归访问标记合并，负深度表示正在解析 |
| `onix.go`、`app_ecs.go` | 四个输入阶段的 handler 桶改为定长数组，保留顺序和事件消费语义 |
| `ecs/archetype.go`、`ecs/world.go` | 转换边使用可选的小型有序切片；少量项顺序查找，大集合优先直接偏移，否则二分查找；首次预留 4 项 |
| `scene/render_hierarchy2d.go` | 密集实体复用 `internal/entitycache.Store`；少量高 ID 实体使用 map，随实际密度切换，保留 generation 校验及容量维护 |

输入活动列表支持负数及任意大按键码、额外鼠标按键；查找成本随同时活动的按键数量增长。自定义调度阶段仍允许任意字符串，因此保留原来的阶段名称索引。

`FrameScratchMap` 返回值借用至下一次同标签调用或 `World.Maintain`；调用方不应跨维护持有它。维护允许一次缩容分配，稳定使用时继续复用。

## 测量方法

Linux amd64，Go 1.27.3，Intel Core i5-11500。通过 `GOMAXPROCS=1 taskset -c 2` 固定单线程及 CPU，每项运行 300 ms、重复 3 次，下表取中位数。优化前通过 Go overlay 恢复任务开始时的源码，使用相同基准；转换边在最后调整预留容量后单独重测。

`B/op` 表示每次操作的分配字节数，`allocs/op` 表示分配次数，不是存活堆、RSS 或显存。暖路径的零分配数据表示已预热并复用容量。这些 CPU 基准不能直接换算成应用 FPS；三次短基准也不足以确认几个百分点差异的统计显著性。

## 耗时

| 基准场景 | 优化前 | 优化后 | 耗时变化 |
| --- | ---: | ---: | ---: |
| 输入状态：创建并记录 Ctrl+A、鼠标左键 | 747.6 ns | 300.7 ns | −59.8% |
| 输入状态：同一组合的按下／释放／帧结束 | 399.1 ns | 76.5 ns | −80.8% |
| 输入状态：复制快照 | 382.6 ns | 9.4 ns | −97.6% |
| 树同步：128 节点 | 42.95 μs | 14.14 μs | −67.1% |
| 树同步：1,024 节点 | 351.61 μs | 107.60 μs | −69.4% |
| UI 布局层级：1,024 节点 | 164.56 μs | 132.26 μs | −19.6% |
| VNode 状态创建 | 300.1 ns | 191.5 ns | −36.2% |
| CSS 候选级联：8 项 | 875.4 ns | 661.7 ns | −24.4% |
| CSS 候选级联：64 项 | 5.85 μs | 4.28 μs | −26.8% |
| CSS 候选级联：256 项 | 23.84 μs | 17.65 μs | −26.0% |
| CSS 工作区首次建立 | 1.93 μs | 1.15 μs | −40.6% |
| 完整样式计算：缓存已建立 | 1.50 ms | 1.40 ms | −6.6% |
| 完整样式计算：重新建立 | 1.57 ms | 1.43 ms | −9.0% |
| 密集渲染层级缓存：4,096 次命中 | 90.10 μs | 28.20 μs | −68.7% |
| 渲染层级缓存：首次填充 1,024 项 | 286.23 μs | 190.46 μs | −33.5% |
| 输入 handler 阶段分发：32 个事件 | 9.07 μs | 5.96 μs | −34.2% |

转换边的分布对比：命中每次查询一条边；冷建立每次重新填充指定数量的边，目标 archetype 已存在。

| 边数 | 命中：前→后 | 命中耗时变化 | 冷建立：前→后 | 冷建立耗时变化 |
| --- | ---: | ---: | ---: | ---: |
| 1 | 6.761→5.393 ns | −20.2% | 157.8→122.7 ns | −22.2% |
| 4 | 7.178→6.715 ns | −6.5% | 358.5→282.6 ns | −21.2% |
| 16 | 10.890→5.825 ns | −46.5% | 2,310→1,214 ns | −47.4% |
| 64 | 11.140→5.603 ns | −49.7% | 9,495→5,773 ns | −39.2% |

## 分配量

| 场景 | B/op：前→后 | 分配字节变化 | allocs/op：前→后 |
| --- | ---: | ---: | ---: |
| 输入状态创建 | 1,248→632 | −49.4% | 12→4 |
| VNode 状态创建 | 384→240 | −37.5% | 8→5 |
| CSS 工作区首次建立 | 4,712→2,832 | −39.9% | 6→3 |
| 渲染层级缓存首次填充 1,024 项 | 430,440→249,752 | −42.0% | 22→22 |
| 转换边冷建立：1 条 | 132→60 | −54.5% | 3→3 |
| 转换边冷建立：4 条 | 144→72 | −50.0% | 6→6 |
| 转换边冷建立：16 条 | 696→312 | −55.2% | 23→20 |
| 转换边冷建立：64 条 | 2,680→1,272 | −52.5% | 75→70 |

树同步、层级深度、输入帧处理及快照、缓存 CSS 计算、暖 VNode diff、临时 map、暖转换边和渲染层级命中均保持 `0 B/op、0 allocs/op`。输入 handler 分发仍为 `5,120 B/op、32 allocs/op`，本轮只减少阶段桶查找成本。

## 持平及略慢的场景

| 场景 | 优化前→后 | 分配变化 | 解释 |
| --- | ---: | --- | --- |
| VNode child slot 更新 | 5.66→5.68 μs | 均为零 | 基本持平 |
| VNode 首次 diff | 149.69→147.17 μs | 均为 74,456 B、22 次 | 基本持平，合并 map 的主要收益是状态创建 |
| VNode 暖 diff | 89.29→90.69 μs | 均为零 | +1.6%，没有测得加速 |
| 临时 map 填充和维护 | 1.640→1.674 μs | 均为零 | +2.1%；收益是峰值缩容及闲置回收 |
| ECS 组件插入／移除完整循环 | 160.5→165.2 ns | 均为零 | +2.9%；单独边查询的收益未体现在这个小循环中 |
| 稀疏访问渲染层级缓存 | 118.3→117.3 ns | 均为零 | 基本持平；该基准有祖先继承，另用下行验证真正稀疏的首次建立 |
| 65,536 实体中仅填充最后 8 项 | 1.678→1.730 μs | 均为 944 B、2 次 | +3.1%；稀疏 map 避免按最高实体 ID 扩大密集索引 |

## 验证

相关包完整测试通过：`input`、`ecs`、`html/dom`、`html/css`、`html/style`、`html/layout`、`ui`、`ui/html`、`scene`、`project`、`module` 和根包。最后的输入、临时 map 和转换边调整后，重新运行了所有直接相关包测试。

定向竞态测试通过，覆盖输入标志、临时 map 峰值／闲置回收、稀疏转换边、CSS 候选遍历及 inline cache、渲染层级。回归测试覆盖树循环及折叠、子节点槽位生命周期、CSS 继承与级联顺序、输入同帧重复转换和快照隔离、渲染缓存密集／稀疏切换及实体 generation。

`fieldalignment ./input ./ecs ./ui ./ui/html ./html/style ./scene .` 无诊断，`git diff --check` 通过。WebAssembly 输入测试在 Node 下通过；Android arm64 输入测试二进制编译通过。`adb connect 10.0.0.174:16384` 返回连接被拒绝，因此未完成本轮 Android 真机运行。未启动编辑器。

## 复现与原始记录

新增基准保存在各包的 `map_bench_test.go`、`ui/html/component_map_bench_test.go` 和根目录 `app_map_bench_test.go`；同时复用已有的 CSS、VNode 基准。

```sh
GOMAXPROCS=1 taskset -c 2 go test -p 1 -run '^$' \
  -bench '^(InputStateMaps|FrameScratchMap|ArchetypeEdges|ArchetypeEdgeDistribution|TreeMaps|UILayoutHierarchyMaps|HTMLComponentStateCreate|HTMLComponentChildSlot|HTMLComponentDiff|CandidateCascade|CascadeWorkspacePopulation|ComputeFullStyles|RenderHierarchyMaps|RenderHierarchyPopulation|RenderHierarchySparsePopulation|InputHandlerPhases)$' \
  -benchmem -benchtime=300ms -count=3 \
  ./input ./ecs ./ui ./ui/html ./html/style ./scene .

go test -p 2 ./input ./ecs ./html/dom ./html/css ./html/style ./html/layout ./ui ./ui/html ./scene ./project ./module .
go test -race ./input ./ecs ./html/style -run 'Test(FrameScratchMap|ArchetypeEdge|ButtonStorage|CandidateTraversal|InlineStyleCache)' -count=1
go test -race ./scene -run '^TestRenderHierarchy' -count=1
fieldalignment ./input ./ecs ./ui ./ui/html ./html/style ./scene .
```

本次会话的本地原始记录在 `/tmp/onix-map-opt-73omdm3u`：`before/` 为原始 Go 源码快照，`before-overlay.json` 为恢复基线的 overlay，`changes.diff` 为相对该快照的本轮 Go 改动。基准原始输出为 `controlled-before.txt`、`controlled-after.txt`、`controlled-edges.txt`，合并后的中位数是 `controlled-comparison.json`；转换边以最后单独重测的文件为准。为前述基准命令增加 `-overlay /tmp/onix-map-opt-73omdm3u/before-overlay.json` 可重测基线。这些临时文件随环境清理可能失效，报告中的结果与仓库中的基准源文件长期保留。
