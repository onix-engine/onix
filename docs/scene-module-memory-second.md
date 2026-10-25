# Scene / module 第二批内存优化

2026-10-25，Linux amd64，Go 1.27.3。基线包含[第一批优化](scene-module-memory.md)。

## 改动

### 3D 渲染

资源准备时在快照数组内完成不透明 / 透明分区，两条队列直接借用互不重叠的分区。原来每条队列通过 append 再保存一份完整网格、材质、纹理与变形引用；现在只保留一份快照。仍按原有规则分别排序，拾取视图全部进入不透明队列，阴影仍直接使用其提取结果。

快照重建时记录旧长度，并在现有视图清理遍历中清除缩短部分，释放已移除网格、材质和变形的引用。SSAO / SSR 借用的旧队列引用同步清理。错误路径也执行尾部清理，允许阴影视图尚未完成初始化。

阴影数量缩减时丢弃不再使用的视图及其 CPU 缓冲；剩余视图继续复用。绘制变形列表缩短时清理尾部的关节矩阵和 morph 数据引用。准备阶段的网格、shader、纹理临时表在调用结束时清空，保留 map 容量，避免最后一帧的资源被临时表长期持有。

### 模块

Content 的输入快照在首次读取平台输入时创建；私有状态 map 在首次 ContentStore 时创建。没有用到这些功能的 Content 不再承担输入状态及其七个 map 的常驻分配。

销毁模块时释放私有 Content 状态、输入快照、元素索引、暂停可见性表及预备节点 bundle。旧 Content 句柄不会重新写入状态或创建存储；Destroy 回调执行期间仍可读取和更新自身状态，清理发生在销毁回调及实体释放之后。

## 测量结果

| 基准 | 第一批完成后 | 第二批完成后 | 变化 |
| --- | ---: | ---: | ---: |
| 2048 个网格、透明与不透明各半：快照和队列数组预留 | 2,702,592 B | 1,179,648 B | -56.4% |
| 创建 Content 并存入一个 int：分配量 | 1,624 B/op | 776 B/op | -52.2% |
| 创建 Content 并存入一个 int：分配次数 | 13 allocs/op | 5 allocs/op | -8 次 |

`snapshot-reserved-B` 统计单个视图提取快照和两条队列的独立底层数组容量，借用同一数组只计一次；不含 map、GPU 数据、其他视图及分配器取整。创建 Content 的 B/op 是累计分配量。以上均不代表整个引擎的 RSS 降幅。

100 / 1000 网格、两个摄像机的准备整帧基准，以及重复准备 2048 个混合材质网格的基准，预热后保持 0 B/op、0 allocs/op。模块消息派发、结构化 slot 事件、请求回复基准也保持零分配。

原始数据：[优化前](scene-module-memory-second-before.txt)、[优化后](scene-module-memory-second-after.txt)。

## 验证

```sh
go test -p 1 ./scene ./module ./scene/render3d ./module/audio
go test -p 1 .
go test -p 1 ./module ./scene/render3d -run '^$' -bench 'BenchmarkContentCreation|BenchmarkPreparedSnapshotMemory|BenchmarkRender3DPreparedFrame|BenchmarkDispatch$|BenchmarkStructuredSlotEvent$|BenchmarkRequestReply$' -benchmem -count=3 -benchtime=150ms
```

新增测试覆盖：分区正确性与共享存储、网格删除后的尾部引用、阴影视图 6 → 1 的回收、变形绘制数据清理、资源准备临时表清理、提取失败时部分初始化阴影的清理、模块销毁时的私有状态释放、输入快照隔离与预热后零分配，以及正常销毁 / Runtime.Close 中 Destroy 回调的状态访问。

继续保留活动视图快照数组的峰值容量，避免波动场景反复分配。本轮重点是去掉重复数组和无效资源引用；未启动编辑器，视觉验收仍由人工完成。
