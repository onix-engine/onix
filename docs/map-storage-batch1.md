# Map 存储优化：第一批

日期：2026-11-12。基线为本轮开始时的工作区源码，包含当时已有的未提交修改。

## 实现

- HTML 组件的绑定状态使用零值初始化，首次写入时创建对应 map，随后继续复用。实例的事件、keyed 列表及子组件容器也按需创建；只有模板声明了 ref 才建立实例 ref 索引。删除了只负责预建五张 map 的状态构造函数。
- 组件 CSS 的样式文本和引用计数合并为一张 map，保留首次挂载的样式顺序。同一定义增减非最后一份引用时只更新计数，首次挂载和最后一次卸载才重新拼接页面 CSS；`SetBaseCSS` 继续保留组件样式。
- UI 布局深度、布局运行状态、Widget 对齐状态和部件索引接入 `World.Maintain`，通过现有 `reuse.MapRetention` 在持续低使用量后重建较小的 map。清空、删除前记录使用量，保留同一帧多次使用的峰值；重建保留布局状态对象和 Widget 的一次对齐状态。部件索引连续 30 次维护未使用后释放，下次查询重新建立。
- 新增 ECS 转换索引对比基准，比较当前 map 与泛型有序紧凑索引。紧凑索引对密集键直接偏移访问，小集合顺序查找，较大集合二分查找。原型仅存在于基准文件中；Patch/Removal 的生产存储仍保留 map。

## 组件基准

Linux amd64，Go 1.27.3，Intel Core i5-11500。使用 `GOMAXPROCS=1 taskset -c 2`，每项运行 250 ms、重复三次，以下取中位数。前后运行同一份基准，基线通过 Go overlay 替换生产源码。

挂载基准包含挂载、首次绑定渲染、卸载及清理变更批次；复用已经建立的 World 和组件定义，不包含样式计算或实际绘制。B/op 是每次操作累计分配量，不是存活堆或 RSS。

| 场景 | 耗时：前→后 | B/op：前→后 | allocs/op：前→后 |
| --- | ---: | ---: | ---: |
| 静态组件挂载／卸载 | 1,663→1,293 ns | 1,256→536 | 23→8 |
| 单个文本绑定组件挂载／卸载 | 2,024→1,741 ns | 2,040→1,464 | 26→14 |
| 已挂载 CSS 定义的引用增加／减少 | 210.10→92.51 ns | 128→0 | 2→0 |
| 暖子组件槽位写入 | 4,081→4,174 ns | 0→0 | 0→0 |
| VNode 首次 diff | 114,976→117,152 ns | 74,456→74,456 | 22→22 |
| 暖 VNode diff | 68,459→68,141 ns | 0→0 | 0→0 |

静态实例少分配 720 B、15 次；文本绑定实例少分配 576 B、12 次。约两个百分点的耗时变化不作为加速或回退的统计结论；本轮主要改善初始化分配、CSS 引用更新和历史容量回收。

## ECS 转换索引对比

相同 CPU 设置，每项 200 ms、重复三次，以下取中位数。记录数为 1、4、16、64；密集键相邻，稀疏键间隔 4093。命中轮换访问所有键；冷建立按置换顺序插入。Transition 对象提前创建，不计入容器分配量。

| 场景 | map | 紧凑索引 |
| --- | ---: | ---: |
| 稀疏 1 项：命中 | 4.15 ns | 2.83 ns |
| 稀疏 4 项：命中 | 4.68 ns | 3.98 ns |
| 稀疏 16 项：命中 | 6.97 ns | 7.16 ns |
| 稀疏 64 项：命中 | 7.47 ns | 9.16 ns |
| 密集 64 项：命中 | 7.29 ns | 2.84 ns |
| 稀疏 64 项：冷建立 | 4.06 μs | 6.77 μs |
| 1／4 项：冷建立分配 | 192 B、2 次 | 64 B、1 次 |
| 16 项：冷建立分配 | 1,128 B、7 次 | 448 B、3 次 |
| 64 项：冷建立分配 | 4,648 B、11 次 | 2,112 B、5 次 |

紧凑索引节省冷建立分配，但在 64 个稀疏键的命中和乱序建立场景较慢，因此本轮未替换生产 map。基准只隔离容器成本，不代表完整 Apply 或游戏帧率。

## 验证与复现

完整测试覆盖 `internal/reuse`、`ecs`、`ui`、`ui/html`、`module`、`scene` 和根包。新增回归覆盖绑定从静态状态启用、首次 patch、绑定移除、CSS 引用与重挂载顺序、基础样式、缓存缩容后存活状态、同帧峰值与闲置索引重建，并检查稳定路径零分配。

```sh
GOMAXPROCS=2 go test -p 1 ./internal/reuse ./ecs ./ui ./ui/html ./module ./scene .
GOMAXPROCS=2 go test -race -p 1 \
  -run 'Test(UILayoutMapRetention|PartIndexMapRetention|HTMLComponentBindingsActivate|HTMLComponentCSSReferences|ScopedHTMLUnmountRemovesCSS|HTMLComponentChildSlot|HTMLAppendComponentChildrenReconcile|FindPartRefreshes)' \
  ./ui ./ui/html
fieldalignment ./ui ./ui/html ./ecs
git diff --check

GOMAXPROCS=1 taskset -c 2 go test -p 1 -run '^$' \
  -bench '^(BenchmarkHTMLComponentMountStorage|BenchmarkHTMLComponentCSSReferences|BenchmarkHTMLComponentChildSlot|BenchmarkHTMLComponentDiff)$' \
  -benchmem -benchtime=250ms -count=3 ./ui/html
GOMAXPROCS=1 taskset -c 2 go test -p 1 -run '^$' \
  -bench '^BenchmarkPreparedTransitionIndex$' \
  -benchmem -benchtime=200ms -count=3 ./ecs
```

未启动编辑器。`adb connect 10.0.0.174:16384` 返回连接被拒绝，未进行 Android 真机验证。

本地原始记录保存在 `/tmp/onix-map-batch1-erqnys_t`：`before/` 和 `before-overlay.json` 是生产源码基线，`components-before.txt`、`components-after.txt`、`transition-index-controlled.txt` 是上述基准原始输出。目录可能随环境清理而失效。
