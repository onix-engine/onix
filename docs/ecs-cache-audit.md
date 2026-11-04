# ECS 解析、更新与渲染缓存审计

日期：2026-11-04。范围：`ecs`、`project` 的模块编译、`module` 的执行范围，以及 `scene` 的变换、输入、2D/3D 渲染主路径。已完成审计和两批运行时优化；HTML 内部缓存不重复纳入本报告。下文 1–8 节保留优化前的诊断和候选方案，实施状态见前两节。

结论：可以减少缓存，但应先修正失效范围、合并同一派生结果、减少重复遍历。最明显的浪费是“无关组件迁移导致全量重建”，其次是跨 scope 重复的渲染继承状态，以及 2D 提取中的中间快照复制。现有静态渲染缓存收益很大，不适合整体取消。

## 已实施的第一批优化

| 路径 | 实际修改 |
| --- | --- |
| ECS 查询解析 | 简单入口复用通用入口的规范查询计划；保留简单查询 map 作为快捷索引，避免重复保存 mask、匹配表和扫描进度 |
| 输入与保留渲染 | 值快照改用 `ComponentValueRevision`；无关组件迁移不会触发全量重建，相机注册组件也使用值版本 |
| 渲染继承 | 从每个 scope 一份合并为 world 共享；用同一 map 的临时默认值终止环，删除 visiting map；移除无效的 RenderOrder 继承依赖 |
| 继承更新 | 多种继承属性先合并 Changed 根，再用现有 queue/marks 遍历一次子树；只有继承属性变化才准备遍历工作区 |
| 局部渲染顺序 | RenderOrder 只标记当前已有渲染节点；非渲染祖先的本地顺序写入不会使后代重建 |
| 拾取更新 | 相机与目标分别失效；稀疏变换更新在已有排序数组内二分定位，只重算对应逆矩阵/裁剪，不增加实体索引 map；深度顺序、成员变化或密集写入回退到完整收集 |
| 渲染提取 | retained 路径临时借用 Sprite/粒子/Tilemap/Spine 组件，直接复制到最终实体快照；文本保留每次收集的结构体状态，嵌套输入只深拷贝一次；处理后清除临时项的借用指针，去掉按全 world 实体数预留容量 |

最终对照使用同一份 `BenchmarkPickSnapshotRefresh`；优化前通过 overlay 恢复原输入刷新实现，其余代码保持相同。以下为三次短基准的中位数，全部 **0 B/op、0 allocs/op**：

| 10,000 个拾取目标 | 原刷新实现 | 优化后 |
| --- | ---: | ---: |
| 无变化 | 410 ns/op | 289 ns/op |
| 只移动一个目标 | 4.85 ms/op | 1.72 μs/op |
| 单实体添加/移除无关空组件 | 4.49 ms/op | 410 ns/op |

场景没有相机，隔离目标缓存刷新成本，不包含事件派发与 GPU。目标的深度不变；排序键变化仍可能全量收集。CPU 未隔离其他负载，数字不代表实际应用的固定加速比。

最终 `BenchmarkIncrementalRender2D/10000/stable` 中位数 **1.34 μs/op**，零分配。静态路径继续复用输出；本批没有重写其常驻实体记录。中间缓冲减少不体现在已有基准的部分 `retained-B` 计数里，因此不宣称该计数已下降。

回归覆盖：双向深度重排、同 tick 多次写入、世界/UI 双变换、同实体 target/blocker、多 scope 共享及范围外祖先、环修复、裁剪/奇异矩阵、变换成员移除重建、组件迁移后继续写入、文本每次收集的独立状态和最终快照生命周期。

第二批继续处理查询空表和 scope 失效，见下节。逐行维护 scope 索引、变换最终矩阵存储统一、3D 关节/几何缓存合并尚未实施。列定位 rank 已实测，但因小表回退暂缓，详见第 7 节。

## 已实施的第二批优化

查询计划的匹配表 ID 数组替换成稀疏的位图块：每块保存一个 word 索引及 64 个匹配位，只保留非空块。world 统一维护一份非空表位图，只在首实体进入或末实体离开时改位。查询对两者求交，再按置位枚举表，保持原 archetype/行顺序；不再逐个访问历史空表，也没有增加每查询的活跃表列表。`World.Entities()` 同样使用 world 位图跳过空表。

对于连续 4,096 张匹配表，计划的逻辑匹配存储从 16,384 B 的 ID 数据缩为至多 1,040 B 的位图块，外加 world 共享的至多 520 B 非空位图。这里统计有效元素，不含切片预留容量及对象头；极稀疏匹配时，每个孤立块比原单个 ID 更宽，但只扫描实际匹配块，不扫描位图跨度中的所有空隙。

scope 保留现有行位图与稀疏 block 跳跃，只收窄重新编译条件：archetype 保存最后一次删除行对应的 world 结构版本。其他表的结构变化和同表尾部追加不会改变已有成员的行身份，因此只需检查含成员的表；可能涉及成员的删除/迁移仍完整重编译，正确处理 swap-remove。待提交的保留实体句柄继续走保守检查，避免延迟 Spawn 被漏掉。

含成员的 scopeTable 在原记录内串联，不额外建立成员表索引切片或 map；清理、排序和失效检查都只走这条链。`count` 收窄为 uint32，与 world 的实体槽位上限一致，使 scopeTable 在 amd64 下仍为 56 B。重新排列 archetype 的布尔字段后，新增行版本也没有扩大其原来的 128 B；首轮活跃表性能回退正是由未调整布局时的对象增大造成的。

新回归测试包含空/非空表反复切换、64 位边界、嵌套查询和提前退出；以实体集合为独立 oracle，在 Patch、Removal、Insert、Commands、Despawn、槽位复用及 scope 切换后逐轮比对结果；另验证新表、保留句柄提交及跨 world 绑定。ECS race 检查通过。

第二批前后对照限制在 CPU 2，单次 300 ms、重复五次取中位数，且没有并发运行项目测试。全部场景预热后为 **0 B/op、0 allocs/op**：

| 场景 | 第二批前 | 第二批后 |
| --- | ---: | ---: |
| 4,096 张匹配表，全部为空 | 11.34 μs | 214 ns |
| 4,096 张匹配表，仅 4 张有实体 | 11.27 μs | 298 ns |
| 4,096 张表全部活跃，每表一个实体 | 80.81 μs | 75.27 μs |
| 4,096 张表中只有 4 张匹配且活跃 | 153 ns | 154 ns |
| 10,000 成员，其他表实体迁移并刷新 scope | 42.81 μs | 65.8 ns |
| 同样 scope，另有 4,096 张历史表 | 63.93 μs | 68.0 ns |
| 10,000 成员，同表外部实体追加/尾删并刷新 | 42.15 μs | 18.75 μs |
| 10,000 成员，成员自身迁移并刷新 | 43.33 μs | 38.71 μs |

最后两行同时说明边界：尾部追加可以跳过重编译，删除仍保守重编译；成员迁移没有被伪装成常数成本。稳定 scope bind 为 2.49 → 2.55 ns，几乎不变；1,000 成员迁移重编译为 4.13 → 4.01 μs。链表初次插入检查只在首次访问行块时执行，避免对每个成员重复做该检查。

计时受 CPU 调度、频率和其他机器负载影响；这里的主要收益来自跳过无关工作，不能将某个端到端应用的帧率直接按上述倍数推算。完整结果见 `isolated-before.txt`、`isolated-after.txt`；scope 最终结果为 `final-scope-after.txt`。无效混合负载对照保留用于调查，没有作为结果来源。

尚未引入结构变更日志或逐实体成员索引。成员所在表删除行时仍可能重编译整个 scope；不是完整的逐行增量算法。world 非空表位图是新增的共享结构，用于替代各查询的匹配 ID 存储及历史空表扫描。

## 审计阶段测量方法与原始结果

环境：Linux amd64，Intel i5-11500，Go 1.27.3。短基准重复三次，下表取中位数；没有隔离机器上其他负载。渲染使用现有测试驱动，结果反映 CPU 准备/提交成本，不代表 GPU 耗时。没有启动编辑器。

| 场景 | 实测 | 含义 |
| --- | ---: | --- |
| 10,000 个拾取目标，输入未变 | 423 ns/op | 当前稳定检查很便宜 |
| 同一场景，仅移动一个目标 | 5.06 ms/op | 一个变化仍重建、排序全部目标 |
| 同一场景，仅给一个目标添加/移除无关空组件 | 5.06 ms/op | 组件搬家被误当作拾取数据变化 |
| 10,000 个成员的 scope，范围外实体迁移本身 | 72 ns/op | 不涉及成员数据变化 |
| 同样迁移后调用 scope.bind | 46.1 μs/op | 全局结构版本使整个 scope 索引重编译 |
| 一个活跃实体，1 个历史匹配 archetype | 107 ns/op | 查询基线 |
| 同样只有一个活跃实体，4,096 个历史匹配 archetype | 11.6 μs/op | 每次仍遍历历史空表 |
| 10,000 个静态 2D 图元，现有完整增量帧基准 | 1.58 μs/op；9,055,024 retained-B | 稳定帧缓存非常有效，但有常驻内存成本 |

以上基准均为预热后的 **0 B/op、0 allocs/op**。零分配不意味着没有全量扫描或大块复制。`retained-B` 只统计现有基准中显式累计的部分切片/节点，未包含 map、提取临时缓冲、所有嵌套数据及 GPU 缓存，不能当作总内存。

另外三项直接观测：

- 1,000 个静态图元，稳定提取为 `Extracted:0, Reused:1000, Sorted:false`；只给一个实体添加无关空组件后变成 `Extracted:1000, Reused:0, Sorted:true`。
- `Query[int]()` 与 `Query[int](Changed[int]())` 持有不同计划；`Changed` 与 `Unscoped` 版本则共用通用计划。重复发生在简单查询和通用查询两条入口之间。
- 两个包含相同 1,000 个实体的 scope 各自建立了 1,000 项的渲染继承 map，其解析值不依赖 scope。

## 1. 优先收窄输入与渲染缓存的失效条件

位置：[input_cache2d.go](/home/zhuoh/workspace/onix/scene/input_cache2d.go:5)、[render_retained2d.go](/home/zhuoh/workspace/onix/scene/render_retained2d.go:143)、[component.go](/home/zhuoh/workspace/onix/ecs/component.go:255)。

`inputPickInputs2D` 使用 `ComponentVersion`；普通渲染输入使用 `ComponentRevision`。这些版本包含 archetype 行迁移，即使相关组件值和成员身份没有变化，版本仍会增加。输入缓存据此全部重建；渲染缓存还会因为 `structures` 不同直接走全量提取。

已有的 `ComponentValueRevision` 正好区分了这两种含义。`Hierarchy`、Transform/UITransform 已经在使用它，无需另建一套版本缓存。

建议：

1. 对只保存值或独立快照的依赖使用值/成员版本。保留资源、设备、scope、外部可变内容等真正的失效来源。
2. 输入的相机和目标分别维护依赖状态：移动相机不必重新生成目标的世界/UI 空间几何。仍保持一个输入状态对象即可，不需要再嵌套一层缓存。
3. 目标集合不变时，只更新 Changed 目标的几何；排序键不变时保持已有顺序。对于排序键变化，先采用可验证的局部调整或排序回退。
4. 检查依赖是否真的被读取。目前拾取表还监听 `SiblingOrder`、`UITransform`、`Size2D`、局部裁剪、`Text2D` 等；当前目标收集主要消费全局变换/裁剪、Anchor、目标尺寸及 Sprite/SpriteLayers。应逐项沿实际读取链确认，移除多余依赖，避免布局写入和全局变换写入各触发一次刷新。

输入目标和渲染项不能直接共用一张最终列表：输入存在不可见的 blocker、receiver、禁用状态，执行范围也可能与渲染范围不同。可以共享继承属性、空间计算等相同数据，最终消费列表仍各自保留。

渲染侧不能机械地把所有版本检查改掉：应验证文本布局借用数组、资源生命周期、同实体多图元顺序和直接绘制路径；持有 ECS 存储地址的消费者仍需要结构版本。第一批已对上述值快照路径实施替换和回归验证；借用的组件指针只在本次提取中使用，处理后从临时项和文本去重状态中清除。

## 2. 渲染继承状态适合从 scope 合并到 world

位置：[render_hierarchy2d.go](/home/zhuoh/workspace/onix/scene/render_hierarchy2d.go:24)、[hierarchy.go](/home/zhuoh/workspace/onix/scene/hierarchy.go:19)。

`renderHierarchy2DCache.resolve` 只读取 world 组件和 world 的 `Hierarchy.Parent`，并未按 scope 截断父链。因此它按 `ScopeValue` 存储会重复保存相同的 SceneLayer、PainterGroup、RenderLayers、目标相机及栈序继承结果。渲染增量提取使用的临时 dirty scope 也会拥有一份。

建议先改为 world 级共享，再将 `values` 与 `visiting` 合并为一份记录及解析状态，或使用现有 `entitycache.Store`。不要为了“合并”给所有非渲染实体强行增加一大块记录；拓扑和派生属性可以共享访问入口而分别按需存储。

还有两处可减少工作：

- 当前任意相关组件变化会 `clear(values)`，导致其他分支的继承结果也失效。可沿已有 Hierarchy 子链只失效受影响子树。
- `RenderOrder2D` 被列入继承缓存版本，但 `resolve` 不读取它，`renderEntityInfo2D` 只对当前世界实体读取其自身 order。可从继承缓存依赖中移除，并将其渲染脏标记改为实体本地更新。

## 3. 合并继承变更的子树遍历

位置：[render_retained2d.go](/home/zhuoh/workspace/onix/scene/render_retained2d.go:154)、[hierarchy.go](/home/zhuoh/workspace/onix/scene/hierarchy.go:78)。

`inheritedRetainedInput2D[T].changed` 每处理一种组件就清空 hierarchyQueue、增加 generation，并遍历对应子树。Parent、RenderLayers、UIStackIndex 等同时变化时，同一批后代可能走多遍。最终 `node.queued` 能阻止重复加入 dirty 数组，却不能阻止前面的重复树遍历。

可先把所有继承输入的 Changed 实体加入现有队列，同一轮共享 generation，然后只扩展一次子树。利用已有 queue/marks，无需新增缓存；后代传播由最多 k 次扫描变成一次去重扫描，Changed 查询本身仍按组件执行。

`Hierarchy.sync` 的成员变化回退也有类似机会：Parent/Activity/Visibility 同时发生成员变化时，合并对 `h.entities` 的全量刷新。Activity 和 Visibility 当前都会调用同时读取两者的 `hierarchyLocalFlags`。

## 4. 2D 渲染可以消除一层中间快照

位置：[render_registered.go](/home/zhuoh/workspace/onix/scene/render_registered.go:134)、[render_snapshot2d.go](/home/zhuoh/workspace/onix/scene/render_snapshot2d.go:10)、[render_retained2d.go](/home/zhuoh/workspace/onix/scene/render_retained2d.go:45)。

当前保留式提取大致经过：

```text
ECS 组件 → world 临时 snapshots + 临时 renderItems
         → 每实体 retained snapshots + retained items + paints
```

例如 Sprite 先 `snapshots.addSprite`，再 `node.snapshot`；文本 spans、粒子数组等在适用路径也会被中间快照和保留快照分别复制。临时容量随后仍驻留。稀疏更新还会调用 `scratch.Reserve(world.EntityCount())`，即使本次只提取少量 dirty 实体。

建议让 collector 接收泛型输出接收器，明确本次数据的最终拥有者：

- 普通/高频动态绘制写入帧缓冲。
- retained 路径直接写入实体拥有的快照。
- 完整提取时保留排序所需记录；稀疏提取时直接更新既有位置，图元数量或排序键变化再回退。

这能省掉中间复制和部分常驻缓冲，比把三个对象包进一个 Cache 结构更有实际收益。需要保持实体快照互不覆盖、文本借用数据的有效期，以及从动态绘制切回静态保留的正确性。

当前已有“变化密集时走顺序提取”的选择，应保留。静态帧仅约 1.58 μs 的结果也说明，不应该为了删缓存而恢复每帧全量提取。

## 5. ECS 查询计划可以共享，空表扫描可以改数据结构

位置：[query.go](/home/zhuoh/workspace/onix/ecs/query.go:392)、[world.go](/home/zhuoh/workspace/onix/ecs/world.go:27)。

`simpleQueryPlans` 和 `queryPlans` 分别创建计划。同一 required/without 条件在两个入口中会重复保存 mask、匹配 archetype 列表和扫描进度。Changed 的时间窗属于 query view，不需要独立的 archetype 匹配计划。

低风险方向是让通用入口拥有唯一的规范计划；简单查询 map 只保存指向该计划的快捷索引。简单查询热路径仍是一次 map 查找，额外查找只发生在建立快捷索引时。可节省重复计划，但不能把这描述为“两个 map 完全消失”。

另外，`queryPlan.archetypes` 永远保留历史匹配表，`nextArchetype` 每次再逐个检查空表。4,096 个历史匹配表、只有一个实体的查询实测约为单表的 108 倍。

可评估用“计划匹配位图 AND world 非空表位图”替代匹配 ID 列表：

- world 只在表从空变非空、从非空变空时改一位。
- 查询按位交集后用 trailing-zero 指令枚举表，仍可保持表 ID 顺序。
- 4,096 个全匹配表的 ID 数据是 16 KiB，位图是 512 B，另外只有一份 world 共享非空位图；不需要为每个计划再维护一张活跃表缓存。

第二批已实现稀疏位图块与 world 非空位图求交，替换原匹配 ID 数组。验证同时覆盖密集活跃表、全部空表、稀疏匹配与稀疏活跃表，详见第二批实施结果。

## 6. scope 索引的全局失效值得单独处理

位置：[query_scope.go](/home/zhuoh/workspace/onix/ecs/query_scope.go:29)、[scene_scope.go](/home/zhuoh/workspace/onix/module/scene_scope.go:19)。

模块的运行/渲染成员列表已经能忽略无关组件迁移，但 ECS 的行位图仍依据整个 world 的 `structureVersion` 重编译：清旧块、扫描 scope 全部实体、重建表内位图、排序块号。实测范围外另一张表中的实体迁移，也使 10,000 成员的 bind 花费约 46 μs。

这里不能简单改成 EntityVersion：成员自己发生迁移、或者同表其他实体删除导致 swap-remove 时，行位置确实变化。

可选方案及成本：

- 经常结构变化、范围较密集：按实体槽位/代数表示成员，查询直接验证成员身份，可取消行位置编译缓存；代价是逐行过滤。
- 稀疏 scope：保留行位图，使用结构变更批次或受影响表的更新，只处理源/目标行及 swap-remove 行。实现必须同时处理 scope 外实体引起的行交换，不应再叠加多套完整成员表。

现有稀疏 scope 的 blocks 跳跃有真实价值，应保留此场景基准作为约束。第二批保留这套行位图，仅使用源表行版本跳过无关重编译；未增加完整的逐行增量维护。

## 7. 不增缓存的列定位算法：已有位图上的 rank

位置：[archetype.go](/home/zhuoh/workspace/onix/ecs/archetype.go:121)。

`ids` 已排序，`columns` 与它对齐，`mask` 已表示同一组组件。列位置可以由已有位图中目标位之前的置位数计算：

```text
word = id / 64
bit = 1 << (id % 64)
缺少目标位 → -1
column = popcount(mask[word] & (bit - 1))
       + sum(popcount(mask[0:word]))
```

无需 `componentID → column` map，也无需额外前缀缓存。临时实验对四种 ID 间距逐项验证了存在/缺失组件的结果与当前线性实现一致。命中查询微基准如下：

| ID 分布 | 当前线性查找 | 位图 rank 原型 |
| --- | ---: | ---: |
| 8 个连续组件 | 4.60 ns | 3.70 ns |
| 32 个连续组件 | 8.89 ns | 3.83 ns |
| 64 个连续组件 | 19.52 ns | 3.69 ns |
| 256 个连续组件 | 60.45 ns | 4.38 ns |
| 8 个组件、ID 间隔 1,024 | 4.49 ns | 30.49 ns |

全部零分配。原型和线性版本通过同样的函数调用形式测量，结果是列定位微基准，不是系统更新整体加速比。高位稀疏 ID 明显退化，所以适合依据表宽/位图跨度选择，或对小表继续线性扫描。还需要覆盖缺失组件分布和实际查询构造/绑定基准。

实施阶段试用了“至少 16 个组件且位图足够密集才用 rank”的选择策略。独立的前后复测中，32/64/256 组件命中分别从约 7.94/18.8/59.0 ns 降至 3.96/4.33/5.70 ns；但 8 组件命中从 3.98 ns 退化至 5.43 ns，稀疏 8 组件也退化。编译器报告 `columnIndex` 内联成本从可内联上升到 107，超过预算 80。已撤回这项运行时修改，测试/基准原型留在临时实验目录；需要保持小表快路径的内联后再评估，不能只依据宽表微基准上线。

## 8. 变换和 3D 数据还有可收缩部分

位置：[transform.go](/home/zhuoh/workspace/onix/scene/transform.go:77)、[ui_transform.go](/home/zhuoh/workspace/onix/scene/ui_transform.go:79)、[renderer.go](/home/zhuoh/workspace/onix/scene/render3d/renderer.go:21)。

当前 amd64 记录大小：普通 transformNode 192 B，uiTransformNode 480 B，inputPickTarget2D 240 B，renderItem2D 456 B。它们不是全部“无用缓存”，但跨 scope 重复时会放大内存。

变换节点保存原始输入副本、local 矩阵和最终矩阵；最终结果又写入 GlobalTransform/UIGlobalTransform。长远可将输出组件作为唯一最终矩阵存储，在父先子后的遍历中直接写出，工作区只保留必要的局部计算、脏状态和裁剪传播信息。不要同时删除 local 矩阵缓存：父节点变化时，它能避免给所有后代重新计算旋转/缩放。必须保持缺少 Transform 的父节点、scope 边界、环检测和 UI 裁剪豁免等现有语义。

3D 有两处更具体的候选：

- `JointMatrices` 与 `gpuJoints` 都保存完整关节矩阵；update 阶段已复制一次，appendPhase 又复制。统一矩阵存储类型/消费接口，可以省去一份 64 B/关节的数组和重复转换；还需处理直接提交 JointMatrices 的调用者。
- `dynamicGeometry` 与 `deformationIndices` 使用相同 key，同一绘制起点清空，可合并为带有效标志的 geometry 记录，分别保存 CPU 索引和 GPU deformation 索引。CPU 索引 0 是有效值，不能用它兼任“未填充”。合并后值更宽，实际节省量须测量。

3D 的 opaque/transparent 阶段已经借用 `extracted` 的不重叠切片，并非两份完整快照，应保持现状。

## 应保留的缓存与边界

| 数据 | 判断 |
| --- | --- |
| project.CompileCache | 已共享不可变 plan 的底层数据；命中仍校验源内容及引用文件。主要用于编辑事务，不是每帧热点，不优先取消 |
| resourceCache | 统一的泛型实现承载资源加载、并发和销毁；字体/纹理/模型生命周期不同，合并成 any 大 map 无明显收益 |
| Added/Changed ticks 与分块范围 | 分别表示加入时间和修改时间；分块索引用于跳过未变化区域，不宜合并成单一 dirty 位 |
| 存储版本与值版本 | 表达不同失效语义，应该让消费者选对版本，而不是去掉区别 |
| FrameValue / ScopeValue / Local / MessageScratch | 前者是 world 持久值，scope 和 system 分别有独立状态，消息缓冲跨轮次有效；生命周期不能混用 |
| archetype 边、Patch/Removal 计划 | 主要绑定结构转换和列，不是每帧组件值副本；已有有序合并遍历，不优先再做全局缓存层 |
| 静态渲染输出与 GPU 合成目标 | 有明确复用收益；保留密集动态内容的顺序路径，减少中间快照更合适 |

解析侧 `CompileCache` 的 plan 与返回 plan 共享切片，不能仅按字段数量认为保存了两份深拷贝。内容变更时会先流式读取哈希、再 ReadFile 解码；减少这次额外读取会与“未变内容不分配完整 JSON 缓冲”产生取舍，优先级低于每帧路径。

`FrameScratch` 也不是帧开始时自动全部清空：切片/映射在取得时重置，`FrameValue` 持久保留。统一成自动清空的大缓存会破坏已有调用语义。

## 实施顺序与剩余工作

1. 已完成：值快照使用准确版本、移除多余依赖、world 级渲染继承共享、继承脏子树合并遍历。
2. 已完成：简单/通用查询共用规范计划。列定位 rank 因小表退化暂缓。
3. 已完成：减少 retained 中间快照复制、缩小拾取相机/目标更新范围，并实现稀疏变换更新。
4. 已完成第二批：稀疏匹配位图跳过历史空表、scope 按成员表行版本检查失效；逐行更新 scope 索引仍留待结构高频变化场景验证。
5. 后续：统一变换最终矩阵、3D 关节数据等较大数据布局。

第一批不需要增加新的通用缓存框架。后续每项同时比较稳定帧、单点变化、密集变化、组件迁移、scope 切换和实体代数复用，记录时间、分配及常驻容量。

## 验证与复现

已通过：

```sh
go test ./ecs ./scene ./module ./project ./scene/render3d
go test -race ./ecs
```

第一批新增测试见 [query_test.go](/home/zhuoh/workspace/onix/ecs/query_test.go:155) 和 [cache_optimization_test.go](/home/zhuoh/workspace/onix/scene/cache_optimization_test.go:1)。最终拾取和静态渲染基准：

```sh
go test ./scene -run '^$' -bench '^BenchmarkPickSnapshotRefresh$/^10000$' -benchmem -benchtime=150ms -count=3
go test ./scene -run '^$' -bench '^BenchmarkIncrementalRender2D$/^10000$/^stable$' -benchmem -benchtime=150ms -count=3
```

第二批测试见 [query_cache_test.go](/home/zhuoh/workspace/onix/ecs/query_cache_test.go:1)，基准见 [query_cache_bench_test.go](/home/zhuoh/workspace/onix/ecs/query_cache_bench_test.go:1)。使用 `taskset -c 2` 限制在同一 CPU，前版本通过 overlay 恢复本批开始前的四个源文件；实验与输出保存在 [第二批临时目录](/tmp/onix-ecs-cache-batch2)。

```sh
taskset -c 2 go test ./ecs -run '^$' -bench '^(BenchmarkScopeStructuralRefresh|BenchmarkQueryTableDistribution)$' -benchmem -benchtime=300ms -count=5
```

仓库现有基准：

```sh
go test ./ecs -run '^$' -bench 'Benchmark(AuditQueryHistoricalArchetypes|ChangedQuerySparse|ScopedQuery)$' -benchmem -benchtime=150ms -count=3
go test ./scene -run '^$' -bench '^BenchmarkRetainedRender2DStatic/base$' -benchmem -benchtime=200ms -count=3
```

审计阶段的补充实验使用 Go overlay 注入临时测试；实施阶段的回归测试和拾取基准已写入仓库。当前会话的实验源码和输出位于 [临时实验目录](/tmp/onix-ecs-cache-audit-i7DyCR)：`ecs_probe_test.go`、`scene_probe_test.go`、`overlay.json`、`probe-bench.txt`、`column-bench.txt`、`retained-bench.txt`。最终前后对照为 `final-pick-before.txt`、`final-pick-after.txt`、`final-render-after.txt`；rank 复测为 `column-controlled-before.txt`、`column-controlled-after.txt`。目录可能随系统清理而失效。

scope 实验：成员均为 int 组件实体，范围外 float64 实体交替插入/移除 bool；预热两个 archetype 后比较仅迁移与迁移后 bind。拾取实验：目标只有 InputTarget2D、UIGlobalTransform、Visibility，无相机，分别测稳定检查、一个目标移动、一个目标添加/移除空 marker。它隔离的是目标收集成本，不包括实际事件派发。列定位实验：轮流命中指定组件 ID，比较线性扫描与上述 rank 原型。

已执行 `adb connect 10.0.0.174:16384`，设备返回 already connected。没有运行 `go test ./...`，没有进行编辑器截图、移动端运行或真实 GPU 验证。
