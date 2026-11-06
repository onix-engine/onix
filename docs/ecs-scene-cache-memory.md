# ECS、module、scene 缓存收敛

日期：2026-11-06。本轮按 scope 退休、空表和峰值回收、重复索引合并、scope 索引紧凑化、2D／3D 记录重构的顺序实施。以下数字均为 Linux amd64、Go 1.27.3 的 CPU 结构或切片容量，不包含 GPU 存储、分配器开销或完整应用 RSS。

## 生命周期和维护边界

`ecs.World.Maintain()` 在帧末回收缓存；`App.Step()` 已在组件使用和绘制提交结束后调用。自行驱动 World 的调用方需要每帧调用一次，调用时不能存在活动迭代器，也不能继续使用先前借用的组件引用或 EntityRow。

- `QueryScope.Reset(nil)` 立即释放成员索引和 ScopeValue 缓存。
- `QueryScope.ReleaseCache()` 显式释放缓存，保留借用的实体列表，供之后重新编译。
- scope 连续 30 次维护没有查询或 ScopeValue 使用后，自动解除 World 注册并释放缓存；再次使用时重建。缓存指针只在其所属缓存存续期间有效，重新启用 scope 时必须重新取得 ScopeValue。
- 活跃 scope 保留暖缓存，连续 30 次低使用量后收缩峰值容量。参与过但不再包含成员的表记录也会退休。
- archetype 在移除实体时加入回收候选，帧末只检查候选表。持续低使用量后收缩实体、组件、Added／Changed 和块范围数组；空表释放所有这些数组，包括小表。

表及列对象保留身份，预备 spawner、patch、removal 继续有效；组件数组搬迁时使旧 EntityRow 失效，并重建 tick 范围。记录帧内临时峰值，避免在同帧先大量使用再清空后错误缩容。仅预留、从未使用的表继续保留显式 Reserve 的容量。

## 删除和合并

| 路径 | 实现 |
| --- | --- |
| module 列表 | 无覆盖层或预备场景时，渲染直接使用 runningScope，不再构建 renderEntities；切回共享模式时释放独立渲染列表和 scope |
| module 销毁队列 | 删除按整个 World 预留的容量，按实际销毁子树增长，再延迟回收 |
| archetype 迁移边 | 合并 addEdges／removeEdges 为一个 map；已有组件的移除边和缺失组件的添加边使用互斥键域 |
| 普通／UI 变换 | 使用实体 Store 的密集记录直接遍历，删除 active 列表和节点内重复实体身份；Store 的记录和空尾索引均可缩容 |
| hierarchy／module 节点目录 | 只扩到实际活跃实体的最高槽位，剪掉空尾并延迟缩容 |
| 拾取 | 延迟缩容 targets／cameras；逆矩阵仅保存计算二维 x／y／w 所需的 9 个系数，仍使用完整矩阵求逆 |
| 2D payload | 五个互斥快照指针合成一个带类型指针，通过泛型方法读取；保持 GC 跟踪，不使用 unsafe union |
| 2D paint | 删除可从 rawBounds、fringe 和裁剪推导的 bounds／opaque 矩形，使用时计算；修改前仍记录旧 damage 范围 |
| 3D payload | 每个需要绘制的实体／submesh 共享一份材质、变换和资源记录，相机与阴影只保留轻量引用和各自的 CPU 顶点选择 |
| 3D meshKey | 从每个 view 的 map 合并到 World 的唯一索引；材质资源每次 Prepare 按批次编号准备一次 |

## Scope 索引

历史表目录为 `[]uint32`，每项 4 B，指向仅为参与过的表创建的紧凑记录。低密度表使用排序去重的 `[]uint32` 行号；高密度表使用 `[]uint64` 位图，删除原位图之外的块编号数组。重建只扫描一次实体列表，再按表选择表示；暖查询和同一表示的重建不分配。

查询游标对稀疏行号直接跳转，对位图逐字迭代，继续保持 64 行变更过滤边界、查询表顺序、重复成员去重和代际校验。显式 Query.In 的顺序语义保持不变。

## 内存测量

| 测量对象 | 改造前 | 改造后 |
| --- | ---: | ---: |
| 4,097 个历史表、单成员 scope 的目录和表记录容量 | 237,552 B | 18,520 B |
| 单个第 65,536 行成员的行索引容量 | 8,200 B | 8 B |
| 10,000 个 256 B 组件迁出后，空源表底层数组 | 2,730,496 B | 0 B |
| 无覆盖层的模块渲染列表容量，10,000 实体 | 98,304 B | 0 B，使用 runningScope |
| 尚未使用的模块销毁队列，同一场景 | 81,920 B | 0 B |
| 普通／UI 变换节点，不含 Store 项中的实体身份 | 128／336 B | 120／328 B |
| 2D renderItem／paint | 440／104 B | 416／40 B |
| inputPickTarget | 232 B | 200 B |
| 3D 提取候选 | 496 B | 192 B |
| 每个视图的 3D 引用 | 560 B 完整记录 | 40 B 引用，另共享一份 544 B payload |

`TestTransformHierarchyAndPickReclaimPeaks` 同时创建普通／UI 变换和拾取目标，将 10,000 个实体缩到 100 个并运行 64 帧：普通变换 Store 的记录及索引由 1,531,904 B 降至 26,400 B，拾取目标数组由 5,038,000 B 降至 80,000 B。拾取同时保存普通和 UI 两种目标，因此这组容量不能与仅含 UI 目标的测试直接比较。

`BenchmarkPreparedSnapshotMemory` 使用 2,048 个 payload，包含共享块和块目录容量：单视图为 1,196,920 B，两个视图为 1,278,840 B，五个视图为 1,524,600 B。按原来每视图一份完整 560 B 记录计算，两个和五个视图分别为 2,293,760 B 和 5,734,400 B，减少约 44% 和 73%。

单视图的 payload 加引用比原完整记录略大；候选记录更小，且完全被相机和阴影剔除的网格不会创建 payload。某个 submesh 被使用后，同一实体的 submesh payload 一次构建完毕，以便后续视图共享。固定 64 项块保持增长期间的引用稳定；删除记录清尾，持续闲置时释放尾块，完全空闲时也释放最后一个块。

## 验证

回归覆盖 scope 自动退休和重新启用、跨 World 重绑、稀疏／位图切换、重复成员、行迁移、代际复用、预备计划复用、Added／Changed 保留、瞬时峰值、迭代期回收保护、覆盖层切换、列表缩容重绑，以及多相机／阴影共享和 CPU／GPU 选择隔离。稳定维护、scope 重建、2D 和 3D 准备均有零分配检查。

固定 CPU 2、GOMAXPROCS=1 的独立进程基准：10,000 成员 scope 重建原实现约 66–78 μs，当前约 64–68 μs；双成员稀疏重建原实现约 146 ns，当前约 130–139 ns。这是短基准范围，受调频和系统负载影响，不作为稳定加速比例。scope 查询、2D 稳定／单项改色／全量修改及 3D 准备基准均为 0 B/op、0 allocs/op。渲染使用测试驱动，未测真实 GPU 性能。

相关包完整测试：ecs、internal/entitycache、internal/reuse、module、scene、scene/render3d、physics、project、graphics、ui、ui/html 和根包。上述新回收路径的定向 race 检查通过；fieldalignment 检查相关五个包无诊断。没有运行 `go test ./...`，没有启动编辑器。

已连接 Android x86_64 设备，使用 NDK 27 交叉编译并通过 ECS、module、scene 和 render3d 的上述定向回归。场景测试需要额外复制测试字体；通过临时 overlay 指向设备字体目录，仅调整测试夹具的资源路径。设备渲染测试同样使用测试驱动，真实画面仍需人工验收。

复现主要回归和容量测量：

```sh
go test ./ecs -run 'Test(ScopeCache|CompactScope|ScopeRepresentation|TableCompaction|EmptyLargeComponent|SmallEmpty|Maintenance)' -count=1 -v
go test ./module ./scene ./scene/render3d -run 'Test(ScopeLists|TransformHierarchy|SharedSnapshotsKeep|CulledCandidates)' -count=1 -v
go test ./scene/render3d -run '^$' -bench '^BenchmarkPreparedSnapshotMemory$' -benchmem
fieldalignment ./ecs ./module ./scene ./scene/render3d ./internal/entitycache
```
