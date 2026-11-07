# 字符、组件和全局缓存内存优化

日期：2026-11-07。本轮完成无用 cmap 数组删除、组件差分索引合并、布局及祖先工作区缩容、二分字符查询，以及全局缓存生命周期调整。

## 删除与合并

- `font/cmap.go` 删除 format 4 的 `GlyphIDArray`。间接 glyph 查询原本就从 `Data` 读取，无须再复制一份 `[]uint16`；每个 format 4 省去一个切片头和整个重复数组。
- `font/face.go` 删除非 ASCII 的 `glyphIndexCache`。ASCII 仍使用固定数组，其他字符直接查询二分 cmap，不再按输入字符持续扩充 map。关闭字体时将 outline、bitmap map 置空，释放桶和缓存值。
- `ui/html/component.go` 用一张 `map[uint64]uint64` 编码旧位置、输入已见和旧节点实际复用状态，替代组件的两张索引及 VNode 的三张索引。两个状态位区分重复输入与相同 key 更换标签；key 为零或最大 uint64 仍有效。
- 小列表只保存索引；超过 64 项才按需分配缩容计数器。amd64 上 VNode 子列表本体由 80 B 降至 72 B，组件子列表维持 72 B；这些数字不包含 map 桶、切片底层数组及大列表的 32 B 计数器。

## 峰值回收

复用现有的 30 次低使用量观察窗口。工作区持续只使用容量的四分之一或更少时缩容，保留近期需求的两倍与小容量下限；完全空闲的大切片直接释放。

本轮新增维护：

- 布局 `Cache.nodes`，Changed、Removed、Reordered、localFlexRoots，以及主层和嵌套文字布局的行项、内联片段、控件 token、长词 glyph 工作区。
- HTML 祖先路径与结果 map；超过小缓存阈值后才分配计数器。
- World 共享的 2D 渲染层级结果 map，通过已有 `World.Maintain` 维护。
- 纯色纹理登记表的 map 和密集槽位数组。

布局更新结束时维护工作区；HTML 同步没有执行布局时也继续维护，避免必须重新排版才能缩容。祖先缓存按操作清理、组件按差分操作观察，观察次数不一定等于屏幕帧数。缩容会偶尔分配更小存储，稳定热路径保持零分配。没有为此增加整个布局树或场景的遍历。

大文档缩小的回归用例中，行项／内联片段／长词 glyph 的总容量由 `1331 / 1609 / 4096` 降至 `64 / 64 / 0`。存活布局与重新构建结果一致，之后重复布局零分配。另验证了祖先、组件索引、World 层级和纯色纹理登记表确实更换历史 map，而非仅清空内容。

## 字符映射算法

format 4 和 format 12 根据有序区间终点执行二分，查询由 O(区间数) 降为 O(log 区间数)，无需额外字符 map。解析时验证区间顺序及非重叠性，format 12 的长度和计数校验避免 32 位整数溢出。format 4 保留 glyph 数组偏移、delta、零 glyph 和越界保护语义。

区间排序依据 [OpenType cmap 规范](https://learn.microsoft.com/en-us/typography/opentype/spec/cmap)。回归覆盖区间端点、空洞、补充平面、负 rune、间接 glyph 数组，以及无序、重叠和溢出的输入。

## 全局缓存生命周期

- 文字 shaping 缓存使用字体的弱身份，保留实例隔离，同时允许不用的字体及其文件映射回收。维持 8,192 项／4 MiB 载荷上限，达到压力阈值时先清理失效字体条目；并发同 key 插入不会重复记账。
- convenience 编译样式缓存对 stylesheet 和 matcher 均使用弱引用。文档的 `MutationCache` 仍强持有自己的 matcher；不再由全局单项缓存留住最后一个已关闭文档。没有文档持有的 matcher 可在 GC 后重新编译。
- 每种字体 fallback 样式的字符查询采用 4,096 项 FIFO，成功和失败均计入预算。被淘汰的字符再次出现会重新查找；共享系统 fallback 字体本身仍按原有设计由进程持有。
- 纯色纹理缓存弱引用 CPU Texture，独立登记每个设备的 GPU 句柄。GC 只使纹理失效，不调用图形驱动；`Canvas2D.FinishFrame` 在所属设备线程中每次检查最多 64 个登记项，其他设备的句柄等待对应设备维护。删除失败保留句柄重试，旧记录清理不会删除同色替代纹理。
- Canvas 将直接使用及通过子纹理视图使用的纯色底图保留到提交完成；驱动继续负责已提交资源的 GPU 完成时序。纯 CPU 创建时只进行有限的无 GPU 清理。

弱引用释放依赖 GC，GPU 回收还依赖后续对应设备帧；这是延迟回收机制。测试验证 CPU 对象可回收、多设备独立释放、驱动失败重试、设备已销毁、同色替换、排队绘制保活和热路径零分配。

## 测量

Linux amd64、Go 1.27.3、Intel i5-11500。相同基准按修改前、修改后依次运行，每项三次，`-benchtime=300ms -benchmem`，表中耗时为中位数。修改前通过 Go overlay 恢复对应实现文件，其余工作区和测试夹具一致。

| 基准 | 修改前 | 修改后 | 分配变化 |
| --- | ---: | ---: | --- |
| Face 查询，循环 8,192 个中文字符 | 20.27 ns | 19.28 ns | 均为 0 B/op、0 allocs/op |
| 直接 cmap 查询，同样字符 | 321.4 ns | 18.43 ns | 均为 0 B/op、0 allocs/op |
| 1,024 项 VNode 差分，首次建立索引 | 313.82 µs | 147.15 µs | 223,369 → 74,456 B/op；66 → 22 allocs/op |
| 1,024 项 VNode 差分，复用索引 | 112.44 µs | 84.36 µs | 均为 0 B/op、0 allocs/op |

组件首次建立索引的基准预建并共享 DOM、VNode 和输出切片，测量差分索引建立开销；B/op 是分配量，不代表整个应用的常驻内存。字符 map 删除主要减少按字符积累的存储，其热查询耗时基本持平。上述结果不能换算为编辑器整体 RSS、帧率或驱动显存收益。

## 验证

```sh
go test ./font ./graphics ./html/layout ./html/style ./ui/html ./scene ./render -count=1
go test -race ./font ./html/style ./html/layout ./graphics \
  -run 'Test(GlyphFallback.*(Concurrent|Bounds)|CompiledMatcherCacheDoesNotRetainStylesheet|ShapeTokenCacheDoesNotRetainFont|ShapeTokenCachesShapedResult|ColorTexture)' -count=1
go test ./font ./ui/html -run '^$' \
  -bench 'Benchmark(CharacterMap|HTMLComponentDiff)$' -benchtime=300ms -count=3 -benchmem
fieldalignment ./font ./graphics ./html/layout ./html/style ./ui/html ./scene ./render
```

`fieldalignment` 的项目源码检查无诊断；工具仍报告 Go build cache 中三处已有 CGo 生成结构，未修改生成文件。ADB 已连接 `10.0.0.174:16384`；本轮验证为主机测试，未启动编辑器或执行 Android 应用验收。
