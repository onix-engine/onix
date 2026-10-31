# 五项性能优化实施报告

日期：2026-10-31。五项优化均已实现。收益主要来自消除重复扫描、分段字符串拼接和静态效果的重复 GPU 计算；首次布局、结构变化和效果失效仍然需要正常重建。

比较基线是本任务开始时的工作区代码，包含当时已有的未提交修改，并非 Git HEAD。原有修改已保留。基线源码与 Go overlay 位于 `/tmp/onix-perf-five-baseline-24_9e5i9`；永久保留的原始测量及验证记录见文末。

## 实测结果

以下为每组三次测量的中位数。耗时对应指定函数或测试操作，不代表完整应用的帧耗时。

| 项目与负载 | 优化前 | 优化后 | 结果 |
| --- | ---: | ---: | --- |
| 分段文本顺序读取，约 72 万字节 | 7.356 ms | 2.936 ms | 约 2.5 倍 |
| 1024 词换行，640px 宽，编辑后的分段文本 | 6.795 ms | 0.793 ms | 约 8.6 倍 |
| 1024 词换行，极宽单行压力场景 | 367.972 ms | 0.754 ms | 约 488 倍 |
| 100 行文档，生成 20 行可见选区 | 174.136 µs | 1.018 µs | 约 171 倍 |
| 10000 行文档底部，生成 20 行可见选区 | 16.425 ms | 0.820 µs | 消除了按槽位反复扫描文档的开销 |
| 编辑后的 8192 字节行，整行选区 | 8.325 µs | 0.802 µs | 约 10.4 倍 |
| 10000 个静态刚体，只改一个刚体 Transform | 3.984 ms | 0.628 ms | 约 6.3 倍 |
| 同一物理场景，只改一个无关 Transform | 4.005 ms | 0.743 ms | 约 5.4 倍 |
| 同一物理场景，只改刚体属性 | 0.627 ms | 0.633 ms | 耗时基本持平，消除了分配 |
| 同一物理场景，无输入变化 | 0.671 ms | 0.632 ms | 小幅波动，不作为主要收益 |
| GPU blur 静态缓存，256×256、σ=4 | 15.367 ms | 1.070 ms | 受控软件渲染环境下约 14.4 倍 |
| 相同 GPU blur，每次都使缓存失效 | 31.780 ms | 31.616 ms | 基本持平 |

CPU 环境为 Linux/amd64、Go 1.27.3、Intel i5-11500。文本/选区使用 `-cpu=1 -benchtime=200ms -count=3`。物理表格采用延长后的 750ms 测量，固定 CPU 4、`GOMAXPROCS=1`；测量包含系统同步、模拟步进和写回。

GPU 使用 Xvfb 下的 Vulkan + llvmpipe 软件渲染，包含绘制、Present 和同步单像素读回。默认线程测量出现较大调度波动，完整数据均已保留；表格使用固定 CPU 4、5，`LP_NUM_THREADS=1`、`GOMAXPROCS=1`、500ms × 3 的对比。这些数值不能外推为真实显卡或 Android 的 FPS 提升。

选区计时已排除首次文本布局，比较的是生成相同 20 行可见矩形的总成本。极宽换行和文档底部选区是暴露原有重复遍历的压力场景；正常 640px 段落结果另列，避免将压力场景倍数当作普遍收益。

| 热路径分配 | 优化前 | 优化后 |
| --- | ---: | ---: |
| 640px 段落换行 | 880 B，14 次/op | 0 B，0 次/op |
| 极宽分段文本换行 | 2,639,744 B，768 次/op | 0 B，0 次/op |
| 10000 行文档可见选区 | 2,560 B，160 次/op | 0 B，0 次/op |
| 编辑后长行选区 | 16,384 B，2 次/op | 0 B，0 次/op |
| 物理刚体属性编辑 | 32 B，1 次/op | 0 B，0 次/op |

顺序游标及物理 Transform 场景优化前后均为零分配。GPU 基准中的约 4 B、1 次/op 来自测试里的读回缓冲，未将其归为渲染缓存分配。

## 1. 批量生成可见选区

实现位于 [text_selection.go](/home/zhuoh/workspace/onix/scene/text_selection.go)，并接入两种 UI 选区表现方式。

- 通过二分定位选区起点及视口起点，只访问相交的可见行。
- 一次迭代提供所有矩形，不再为每个选区槽位从头查找。
- 缓存行的排版 advance，完整选中一行时直接取值。
- 部分行通过游标测量端点，保留 tab、字距、kerning 和原子内联对象的几何语义。
- 传统独立 Surface 选区共用按 owner 缓存的临时结果；容量预热后复用。

实现过程中修复了传递 Transform 地址造成的闭包逃逸。常规输入框、密码输入、textarea、IME 组合文本及 authored 控件的原有预热零分配测试均通过。

## 2. 2D/3D 物理 Transform 增量同步

共享逻辑位于 [workload.go](/home/zhuoh/workspace/onix/physics/workload.go)。

- 普通刚体属性和 Transform 值变化只同步相关实体。
- 祖先 Transform 变化沿层级访问受影响子树，即使祖先没有刚体。
- epoch 标记复用内存，合并重叠子树及刚体属性变化。
- 复用 `SystemContext`，消除原有属性编辑时的堆分配。
- 组件成员、层级、活动状态、查询范围等结构变化，以及失效刚体句柄，仍触发完整同步和所有权清理。

补充 [InQueryScope](/home/zhuoh/workspace/onix/ecs/query_scope.go) 查询范围成员检查，使范围外祖先的变化能够影响范围内子刚体，同时排除范围外刚体。该检查利用已有范围位图，预热后不扫描整个范围、不分配。

新增测试覆盖 2D/3D、无刚体祖先、重叠修改、无关实体、非法缩放、补加 Transform、范围外祖先和零分配。物理步进与写回本身仍有成本，本次没有把整个物理系统变成常数时间。

## 3. 分段文本顺序游标

实现位于 [cursor.go](/home/zhuoh/workspace/onix/text/piecetree/cursor.go)。

`Snapshot.Cursor` 保存当前位置及当前连续片段，跨片段时才重新定位树节点；支持值拷贝、字节读取、UTF-8 解码和前移。新增 `EqualRange` 直接比较分段范围。

文本布局、可见字形重建、caret 命中和换行已使用游标或范围比较，减少逐字符树查找及 `Slice` 拼接。测试包含 Unicode、无效 UTF-8、编辑后跨片段重新组成的 UTF-8、独立游标拷贝及零分配。

现有 `Chunks` 整块遍历仍比逐 rune 的通用游标快，基准保留了这组参照；本次游标用于需要逐步前视、移动和维护排版状态的路径。

## 4. 线性 word wrapping

实现位于 [render2d.go](/home/zhuoh/workspace/onix/scene/render2d.go)。

原实现反复测量从行首到当前候选词的前缀；现在向前遍历，每个词测量一次。换行时扣除前缀及边界字距/kerning，仅重放依赖新行原点的尾部空白与 tab。

用旧实现记录的换行范围摘要验证 24 组确定性文本、6 种宽度、3 种字距，覆盖 Unicode、空白、tab、CR 和内联对象。该测试固定 fixture 字体，避免操作系统 emoji 回退字体改变预期；桌面与 Android 均通过。已有字符换行、token 换行、caret 和布局测试也通过。

## 5. 缓存完整 GPU blur

实现位于 [gpu_blur.go](/home/zhuoh/workspace/onix/ui/html/gpu_blur.go) 及对应生成 shader。

原来只缓存横向模糊，静态绘制仍执行纵向卷积。现在每个效果保留横纵两轮完成后的纹理，静态绘制只采样一次；阴影 knockout 也烘焙到缓存。

- 源版本、源纹理、设备和完整参数共同决定缓存有效性。
- 文档内效果共享一个可调整尺寸的横向中间目标。
- 更新后的 shader 处理预乘 alpha 及渲染目标采样方向；已生成 SPIR-V、GLSL、Metal 版本。
- 源纹理在输出使用期间保持存活；效果全部移除时释放中间目标，文档销毁时释放全部资源。

缓存命中测试确认不提交额外 GPU 工作；参数失效测试确认重新执行两轮模糊。像素读回测试覆盖非对称图案、半透明颜色、阴影 knockout、仅 σ 改变、共享目标尺寸调整及资源清理。DOM 测试覆盖内容更新及只改 opacity 的情况。

效果每帧变化时仍需要横纵模糊，另有一次最终合成，因此该场景没有稳定提速；主要收益来自静态或低频变化的效果。缓存保留最终纹理，并额外占用一张文档共享的中间纹理，新增 GPU 内存约为当前中间目标的宽 × 高 × 4 字节，不含驱动内部开销。

## 验证及已知限制

通过的包测试：`ecs`、`text/piecetree`、`scene`、`ui`（排除下述已有失败）、`physics`、`graphics`、`render`、`ui/html`、`font`、`html/layout`、`input`。

通过的补充检查：

- 游标、选区、换行、查询范围和物理增量同步的定向 race 测试。
- Xvfb 下三个 native GPU 测试，包含实际绘制和像素读回。
- 已连接的 Android x86_64 设备上运行游标、选区、word/token 换行、2D/3D 物理及控件预热零分配测试。
- `go vet` 检查 `ecs`、`scene`、`ui`、`physics`、`ui/html`，无诊断；`git diff --check` 通过。

已有问题及覆盖边界：

- `ui.TestHTMLHoverRebuildsMixedBorderSurfaces` 失败：hover 后左侧混合边框 Surface 没有重建。使用任务开始时的源码 overlay 单独复现，同样失败；未将其混入本次性能修改。
- 包含 `text/piecetree` 的完整 `go vet` 仍报告已有测试中的 7 处未使用 `Tree.String()` 返回值；本次未修改这些测试。
- GPU 运行验证覆盖桌面 Vulkan；GL/Metal/WebGPU shader 已生成，但这些后端和 Android GPU 未做实际像素验收。Android 验证是逻辑测试，不是移动渲染帧率测试。
- 未启动编辑器、未进行截图验收、未使用子代理，也未运行 `go test ./...`。

## 原始记录与复跑

- [CPU 优化前](/home/zhuoh/workspace/onix/docs/performance-five-cpu-before.txt)、[CPU 优化后](/home/zhuoh/workspace/onix/docs/performance-five-cpu-after.txt)：包含常规基准及延长的物理测量。
- [GPU 优化前](/home/zhuoh/workspace/onix/docs/performance-five-gpu-before.txt)、[GPU 优化后](/home/zhuoh/workspace/onix/docs/performance-five-gpu-after.txt)：包含受控线程结果、默认线程结果及发生大幅波动的延长测量。
- [验证记录](/home/zhuoh/workspace/onix/docs/performance-five-validation.txt)：包测试、race、native GPU、Android、已有失败对照与 vet 诊断。

在仓库根目录复跑当前实现的主要基准：

```sh
GOMAXPROCS=2 go test -p 1 ./scene ./ui ./physics -run '^$' \
  -bench '^Benchmark(Review|TextParagraphWordWrap)' -benchmem \
  -benchtime=200ms -count=3 -cpu=1

GOMAXPROCS=2 go test -p 1 -c ./ui/html -o /tmp/onix-html-perf.test
LP_NUM_THREADS=1 GOMAXPROCS=1 taskset -c 4,5 xvfb-run -a \
  /tmp/onix-html-perf.test -test.run '^$' \
  -test.bench '^BenchmarkNativeHTMLGPUBlurCache$' \
  -test.benchtime=500ms -test.count=3 -test.cpu=1 -test.v
```

CPU 编号需按目标机器调整。首次缓存建立或容量增长会分配；报告中的零分配均指测试覆盖的预热路径。
