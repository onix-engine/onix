# 切片引用与容量回收

日期：2026-11-13。

输入、拖尾、ECS 工作区、消息、碰撞网格和文本重置现在按各自的借用期释放旧引用与历史容量。

- `State.ReleaseEvents` 在消费后清理已排出的事件，保留此后新入队的事件；应用的输入派发结束时自动调用，`EndFrame` 也会兜底。`DrainEvents` 的返回值借用到下一次入队、显式释放或帧结束。文本编辑在帧结束及焦点切换时清引用。两个队列记录实际批次需求，持续低使用量后缩容。
- `FrameScratch` 的私有持有者统一接入 `World.Maintain`，记录同帧最大批次，清理未覆盖的尾部及闲置引用，再按已有 30 次观察窗口回收峰值。2D、相机和 3D 候选切片移除各自重复的容量维护；独立驱动系统时也应在帧完成后调用 `World.Maintain`。
- 消息及其 arena 的两个缓冲区只在对应数据过期后清理、缩容。每个缓冲区独立观察 30 次低使用量，通常对应约 60 次消息轮换；消息序号及两帧借用期保持原有语义。
- 碰撞网格完成顶点去重和退化三角形过滤后，用泛型 `reuse.Exact` 收紧最终数组。没有余量时直接保留原数组；存在余量时分配精确长度的数组并复制。已有烹制工作区继续复用，不额外缓存原始大小的顶点／索引副本。
- 文本树重置释放超过 64 KiB 的旧编辑数组，并按新文档规模退掉过大的节点 arena。节点不含引用，重新索引时直接覆盖，不再预先清零整份旧 arena。`RetainedBytes` 计入编辑数组的容量，仍只统计源文本存储，不包含节点和已生成的快照。
- 拖尾以环形缓冲保存时间顺序。清空、过期和容量淘汰都清除退休样本的引用；满队列追加及前缀过期不再搬移存活样本。扩容保留已有快照及顺序。渲染按逻辑顺序读取环形数据。

缩容可能偶尔分配更小的数组；稳定需求保留暖容量。拖尾的 `Reserve` 仍用于显式预热。

## 容量测量

Linux amd64、Go 1.27.3。下表来自相同构造场景的修改前后诊断，按切片底层容量乘元素大小计算，不包含分配器取整、持有者、map、其他共享所有者或 GPU 存储，也不等于应用 RSS。

| 场景 | 修改前 | 修改后 |
| --- | ---: | ---: |
| 4096 项 Event 工作区降到 1 项，维护 64 帧 | 589,824 B | 9,216 B |
| 两次 4096 项 Event 消息峰值，空闲轮换 64 次 | 1,392,480 B | 0 B |
| 重复顶点和退化三角形：30000 项顶点／索引各剩 3 项 | 840,000 B | 84 B |
| 编辑过 1 MiB 文本后重置成 1 字节，编辑／节点数组合计 | 1,171,424 B | 176 B |

输入队列中的两份 128 KiB 旧文本，以及拖尾清空后的旧纹理引用，也在诊断中由仍被槽位持有变为已清除。纹理的实际回收还取决于其他所有者。

## 性能与验证

`BenchmarkSpriteTrailBuffer` 比较预热后向满队列追加一个样本：环形缓冲与原来的头部删除／整体搬移。i5-11500、`GOMAXPROCS=2`，每项 300 ms、三次取中位数：

| 样本数 | 整体搬移 | 环形缓冲 |
| --- | ---: | ---: |
| 24 | 61.45 ns/op | 16.03 ns/op |
| 1024 | 3247 ns/op | 17.77 ns/op |

两者均为 0 B/op、0 allocs/op。这是缓冲操作微基准，不是完整拖尾更新或应用帧率测量。没有声称网格烹制固定提速；无去重余量的现有网格基准仍为 4 allocs/op。

主机完整包测试通过：`input`、`ecs`、`text/piecetree`、`internal/reuse`、`physics`、`scene`、`scene/render3d`、`ui`、`ui/html`、`project`、`module` 和根包。新增回归覆盖借用期、入队保留、闲置引用、同帧大小批次、消息过期、精确网格容量、文本快照，以及拖尾环绕、淘汰、扩容和顺序。

相关定向 race 测试通过，`fieldalignment` 对涉及的七个包无诊断。Android x86_64 设备 `10.0.0.174:16384` 上的输入、ECS、网格、拖尾及文本重置定向测试通过；临时测试二进制和字体已移除。未启动编辑器。

```sh
GOMAXPROCS=2 go test -p 1 ./input ./ecs ./physics ./scene ./scene/render3d ./ui ./ui/html ./project ./module . -count=1 -timeout=180s
GOMAXPROCS=2 go test -p 1 ./text/piecetree ./internal/reuse -count=1 -timeout=180s
GOMAXPROCS=2 go test -race -p 1 ./input ./ecs ./physics ./scene ./text/piecetree -run 'Test(InputQueues|FrameScratch|MessageBuffers|MessageScratch|TriangleMeshOwns|CollisionCookingOwns|ResetReclaims|SpriteTrail)' -count=1
GOMAXPROCS=2 CGO_ENABLED=0 fieldalignment ./input ./ecs ./physics ./text/piecetree ./scene ./scene/render3d ./internal/reuse
GOMAXPROCS=2 go test -p 1 ./scene -run '^$' -bench '^BenchmarkSpriteTrailBuffer$' -benchmem -benchtime=300ms -count=3
```
