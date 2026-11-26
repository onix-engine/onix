# 性能指南

[English](performance.md) | [简体中文](performance.zh-CN.md)

## 通用原则

性能结论适用于输入稳定且容量已预热的引擎路径。初始化、资源创建、首次使用、
容量增长、设备重建和负载变化都可能产生分配。Go 分配计数不包含 GPU 工作、
浏览器/JavaScript 对象、驱动内部开销或进程 RSS。

## ECS 与 UI

重复访问组件时使用类型化查询。优先使用只读引用，仅在确实要修改值时获取可变
访问，因为可变访问会标记组件变更。使用变更过滤时复用已创建的查询视图；过滤
条件较多的新视图可能分配。预热后的查询迭代、命令刷新和预备 bundle 操作会在
容量范围内复用存储，详见 [ECS API 设计](ecs-api-design.md)。

变更跟踪可以使用保守摘要跳过未变化的数据块，但必须保留写入、原型迁移和行移动
时的 tick 语义。热路径分配断言应在未开启 race 检测时测量。

## 渲染与 HTML

在尺寸和容量稳定时复用渲染资源与临时缓冲。上传和读回路径可以复用 staging
存储；GPU fence 仍决定何时可以重用 GPU 可见内存。资源驻留属于特定设备，尺寸
变化和设备重建属于冷路径。[渲染流程](../rendering/render-path.zh-CN.md)

保留式 HTML 布局应避免重建未受影响的子树。正确性优先：局部更新必须保持浏览器
顺序、裁剪、继承样式和事件行为。HTML/CSS 修改应对照
[Chromium 兼容性用例](../../tests/htmlcompat/README.md)。

## 缓存生命周期

- 借用结束或提交完成后清理失效引用。例如，`input.State.DrainEvents` 返回的切片可借用至
  下次入队、调用 `ReleaseEvents` 或帧结束。`ReleaseEvents` 会保留新入队的事件。
  [输入队列](../../input/state.go)
- 复用工作区并记录帧内最大批次。共用的容量维护逻辑会回收长期闲置的容量。缩容可能触发
  一次分配和存储搬迁，维护后需重新获取借用视图。
  [容量回收](../../internal/reuse/retention.go) · [ECS 帧末维护](ecs-api-design.md#frame-maintenance-and-query-scopes)
- 按所有者管理缓存，对长期存活的缓存限制条目数和载荷。消费者的生命周期一致时可共享
  不可变载荷。待绘制安全完成后，由所属设备回收 GPU 资源。

## 测量方法

使用带 `-benchmem` 的定向包基准，预热被测路径，并说明平台、负载、容量和是否
包含初始化。分开报告分配量和延迟。比较内存保留量时，分别测量存活堆、进程 RSS 和显存。
帧率需要在目标后端测量完整帧。

```sh
go test ./ecs -run '^$' -bench '^BenchmarkChangedQuerySparse$' -benchmem
go test ./ui/html -run '^$' -bench '^BenchmarkHTMLPaintCull' -benchmem
```
