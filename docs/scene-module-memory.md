# Scene / module 内存优化

2026-10-25，Linux amd64，Go 1.27.3。

## 已完成

- `module` 消息与 slot 的已用存储列表按注册类型数预留，最多预留事件队列容量。预留发生在类型注册时，派发时不增加辅助存储分配。
- 场景准备工作区延迟到第一次实际准备时分配，后续复用，保留前台与准备阶段独立的请求表、事件队列和 payload banks。
- 场景准备失败后，通过现有请求链表释放未完成请求，及时清理场景引用；取消下一次准备时对整个请求表的清空。
- 执行列表缩短时清理 Update / FrameUpdate 列表尾部，避免已销毁 Context 和所属 Scene 被底层数组保留。
- slot buffer 只复制类型和字段结构，不保留实例当前值中的字符串等数据；写入仍复用各条目的独立字段存储。
- 2D retained cache 完整重建时重新确定动态成员。实体销毁、动态组件删除、Visibility 组件删除后，旧动态标记不会阻止节点归还。
- 粒子快照重建后清理缩短部分的 appearance / particle 纹理引用，保留当前快照和可复用容量。

## 测量

`BenchmarkRuntimeMemory` 使用默认 Options 与空场景，包含创建 World、Runtime 和关闭操作；prepared 分支另包含 StartEntry。B/op 是每次操作累计分配量，不是进程 RSS 或 GC 后存活堆大小。

| 场景 | 修改前 B/op（约） | 修改后 B/op（约） | 变化 | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 新建 Runtime | 301,979 | 163,093 | -46.0% | 55 → 43 |
| 启动空场景 | 329,528 | 300,913 | -8.7% | 388 → 384 |

辅助存储逻辑预留量 `aux-reserved-B` 只统计已用存储列表和准备工作区的底层数组，不含分配器取整、map、对象头及其他 Runtime 字段：

- 新建 Runtime：119,808 → 0 B。
- 启动空场景：119,808 → 95,296 B。

消息派发、结构化 slot 事件、请求回复，以及 1,000 / 10,000 实体的稳定帧、单项颜色更新、全量更新基准，修改前后均为 0 B/op、0 allocs/op。运行耗时受并行构建和系统负载影响，本轮不据此宣称 CPU 提速。最终基准使用 `-p 1` 串行执行包。

原始结果：[修改前](scene-module-memory-before.txt)、[修改后](scene-module-memory-after.txt)。

## 验证

```sh
go test ./scene ./module ./scene/render3d ./module/audio
go test -p 1 ./scene ./module -run '^$' -bench 'BenchmarkRuntimeMemory|BenchmarkDispatch$|BenchmarkStructuredSlotEvent$|BenchmarkRequestReply$|BenchmarkIncrementalRender2D/1000/(stable|color|all)$' -benchmem -count=3 -benchtime=200ms
```

新增回归覆盖：执行列表缩短、slot 结构模板数据清理、队列已有消息时注册新类型、准备失败后请求引用清理、粒子快照缩短、动态实体和组件反复删除后的节点复用。引用残留测试均先确认失败，再验证修复通过。已有预加载提交/回滚隔离、热路径零分配测试同时通过。

本轮保留节点池与快照数组的可复用容量。释放无效引用不等于立即归还全部峰值容量；大场景切到小场景后的池容量收缩仍可作为后续测量方向。未启动编辑器，未进行视觉验收。
