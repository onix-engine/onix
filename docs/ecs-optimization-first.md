# ECS 首批优化：命令流清理与迁移绑定

2026-10-25，基于 `5e85e31`，对应 [初始审查](code-count-and-ecs-review.md) 的前两项及重复列查找。本批优化已实现；空原型查询索引、闲置期容量回收和修改行索引仍属于后续按实际负载验证的方案。

## 实现

- `FlushCommands` 在完成 nil/结构修改检查之后，对空队列直接返回。空刷新不再扫描历史命令流。
- 命令入队仅在值切片由空变为非空时记录流 ID；刷新只清理本批活跃流。回滚在原有保留长度计算中清理值，并原地压缩活跃列表。只删除组件的命令不持有值，因此无需进入活跃列表。清理成本从历史流数 O(H) 降为本批活跃流数 O(H_active)，清空实际 payload 的成本仍随值数量增长。
- 普通 Insert 与 Patch 值命令共用泛型 `commandValues.queue`，避免两份入队实现。
- Removal/Patch 首次绑定直接取对齐的源列，再以有序 ID 双指针绑定目标列；消除逐列从头查找。绑定阶段从 O(C² + K×C) 降至 O(C+K)，原型查找仍依赖现有哈希缓存。
- Patch 用一份排序后的 `{id, 参数位置}` 切片保留调用顺序，取代原来两份 ID 切片，减少一次计划构建分配及一个切片头。排序发生在计划构建时。
- 移除清理列表保存源列下标，不再复制整份 `removalColumn`。在 amd64 上，每项逻辑存储由 40 字节降为 8 字节，同时减少 GC 需要扫描的重复接口引用。钩子仍先于所有实际移除操作执行。
- 单组件 Insert/Remove 复用已找到的列下标；同步修正 `archetype` 中过期的二分查找注释。

活跃流列表为每个 World 新增一个切片头及可复用的 uint32 ID 容量；因此是用少量持久索引换取每帧清理成本下降，并非所有 World 的内存都减少。首次使用/容量增长仍可能分配，预热后的被测热路径保持零分配。

## 性能

Linux amd64，Intel i5-11500，Go 1.27.3。修改前后分别编译独立测试二进制，串行运行，`-cpu=1 -benchtime=200ms -count=5`，下表为五次中位数。原始数据：[修改前](ecs-optimization-first-before.txt)、[修改后](ecs-optimization-first-after.txt)。这些是微基准，不代表完整应用帧率。

| 负载 | 修改前 | 修改后 | 说明 |
| --- | ---: | ---: | --- |
| 空 Flush，无历史流 | 13.11 ns | 2.55 ns | 空队列快速返回 |
| 空 Flush，1,024 个历史流 | 2,700 ns | 2.467 ns | 不再随历史流数增长 |
| 一个活跃命令，无历史闲置流 | 75.58 ns | 77.26 ns | 本次约 +2.2% |
| 一个活跃命令，1,024 个闲置流 | 2,819 ns | 76.91 ns | 约 36.7 倍 |
| Removal 冷绑定，8 组件 | 404 ns | 367.6 ns | 约 -9.0% |
| Patch 冷绑定，8 组件 | 491.3 ns | 523 ns | 本次约 +6.5% |
| Removal 冷绑定，64 组件 | 4,420 ns | 1,748 ns | 约 2.5 倍 |
| Patch 冷绑定，64 组件 | 4,297 ns | 1,865 ns | 约 2.3 倍 |
| Removal 冷绑定，256 组件 | 38,645 ns | 6,202 ns | 约 6.2 倍 |
| Patch 冷绑定，256 组件 | 42,182 ns | 6,073 ns | 约 6.9 倍 |
| 1,000 条 Insert 命令 | 57,676 ns | 57,093 ns | 约 -1.0% |
| 1,000 条 Spawn 命令及回收 | 100,912 ns | 100,544 ns | 约 -0.4% |
| 1,000 条批量移除命令及创建/回收 | 98,407 ns | 100,412 ns | 本次约 +2.0% |
| 混合 Patch 命令 | 404.2 ns | 378.8 ns | 约 -6.3% |
| 8 组件的单组件往返迁移 | 392.3 ns | 390.8 ns | 大体持平 |

没有为 8 组件冷 Patch 的约 32 ns 差值增加一套特殊实现：常见少组件首次绑定略慢，多组件的平方级开销得到消除。单活跃流和批量路径中的小幅变动应保守解释，不能宣称所有路径都更快。

所有上表热路径均为 `0 B/op, 0 allocs/op`。冷绑定仍需创建过渡计划；本基准每次移除一个组件，Removal/Patch 的分配字节各减少 **40 B/op**，分配次数分别保持 4/5 次。例如 256 组件 Removal 为 `12,016 → 11,976 B/op`，Patch 为 `12,208 → 12,168 B/op`。冷绑定基准复用目标原型并重建过渡计划，数字不包括首次创建目标原型的全部成本。

## 验证

- ECS 单包测试通过；`go test -race ./ecs -count=1` 通过。
- 新增用例覆盖空 Flush 的查询/清理钩子保护、部分回滚后不同泛型流重新激活、重复入队、无值移除流、error/panic 后引用释放与再次入队、预热零分配。
- 新增 32 种源组件组合，覆盖空源、全覆盖、保留组件、调用参数顺序不同于 ID 顺序、写入优先于移除和源表 swap-remove；已有 tick/cleanup/预留容量测试一并通过。
- Android x86_64 设备 `10.0.0.174:16384`，NDK 27.2.12479018、API 24，`GOMAXPROCS=2` 运行 ECS 全包测试 PASS；设备临时文件已删除。性能表来自 Linux，不是 Android 性能测量。
- 定向依赖包 `scene`、`ui/html`、`ui/markdown` 通过。
- `ui` 的 `TestBindControlExpandsGenericTextControlMode`、`TestHTMLInputEditsAndEmitsEvents`，以及 `physics` 的 `TestDynamicPoseRetainsPrecisionBetweenECSWrites` 失败。用 Go overlay 将本次修改的五个 ECS 生产文件全部替换为 `HEAD` 原版，三个测试均复现相同失败：UI 的 Anchor/插入位置不符合期望，physics 的姿态精度不符合期望。它们是本次改动前已有的失败，不能把依赖包结果汇报为全部通过。

```sh
go test ./ecs -count=1
go test -race ./ecs -count=1
GOMAXPROCS=2 go test -p 2 ./scene ./ui ./physics ./ui/html ./ui/markdown -count=1
go test ./ecs -run '^$' -bench 'BenchmarkAudit(EmptyCommandFlush|SingleActiveCommand|TransitionBinding)$|BenchmarkCommandBatch$|BenchmarkMixedPatch$|BenchmarkTraversal(Migration|CommandTrim)$|BenchmarkCommandInsertRecycled$' -benchmem -benchtime=200ms -count=5 -cpu=1
```

未运行全仓库测试，未启动编辑器。
