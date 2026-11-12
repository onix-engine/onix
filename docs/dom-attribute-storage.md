# DOM 属性存储与对比

日期：2026-11-12。基线是本轮开始时的干净工作区 `9f1e2e0`，接续前一轮 map 优化。

## 存储和复用

节点仅保存一个可空的属性存储指针。首次出现属性时分配存储；最多 8 个属性使用紧凑有序列表，更多属性使用 map。大集合不再保留列表副本，也不增加第二个名称索引。逐项创建列表首次预留 4 项；解析和批量构造直接按输入大小分配。

纯列表方案在大属性集合上的查询明显退化，因此最终保留大集合的 map。晋升为 map 后，删除到阈值以下继续复用它，避免在 8／9 项之间反复切换和分配。解析器清除引用并保留容量，生命周期规则与原来一致；释放解析器本身才能释放全部历史容量。

模板克隆共享整个属性存储。源节点或任何克隆首次实际修改时复制自己的存储；相同值赋值、删除不存在的属性不复制。共享标记使用原子操作，支持独立目标并发克隆同一个只读源；同一个节点的并发修改仍不受支持。共享不会延长解析器借用字符串的有效期。

诊断输出、HTML 序列化和文档模板作用域哈希仍按属性名称排序。小列表无需临时名称缓冲；大 map 按需建立排序缓冲。解析时重复属性仍保留首次值，保留 ASCII 名称折叠、SVG／MathML 名称修正及限定名称。这与 [HTML 属性名解析规则](https://html.spec.whatwg.org/multipage/parsing.html#attribute-name-state) 一致；本轮不修改其他 HTML 语义。

Linux amd64 下，`Node` 本体由 96 B 降到 88 B，`Token` 保持 32 B；独立节点的 Go 堆分配仍落在 96 B 档。属性存储头为 40 B、每个属性对为 32 B，不包含切片容量、map 桶及分配器对齐。空节点不分配属性存储。

## API 迁移

删除 `Node.Attributes` 和 `Token.Attributes` 公开 map；仓库内的 DOM、CSS、布局、HTML 组件、Markdown 和编辑器测试调用均已迁移。`NewElement` 接受属性对并取得独立副本，不再借用可写 map：

```go
node := dom.NewElement("div",
    dom.Attribute{Name: "id", Value: "target"},
    dom.Attribute{Name: "class", Value: "card"},
)
node.SetAttr("hidden", "")
value, present := node.Attr("hidden") // 空值与不存在仍可区分
_ = value
_ = present
node.RemoveAttr("hidden")
```

只需要字符串时可用 `AttrValue`，数量用 `AttrCount`；`SetAttributes(attrs...)` 复用容量替换整组属性，重复名称取首次值。`Token` 提供相同的读写方法，已经返回的 token 不会被下一次分词覆盖。

```go
for name, value := range node.Attributes() {
    // 无序、零分配，可修改或移除当前这一项。
    if name == "data-old" {
        node.RemoveAttr(name)
    }
    _ = value
}
for name, value := range node.SortedAttributes() {
    // 按名称排序；小列表不分配，大 map 分配名称缓冲。
    // 此遍历期间不修改属性。
    _, _ = name, value
}
clone := dom.NewElement("div")
clone.ShareAttributes(node)
clone.SetAttr("class", "changed") // node 保持原值
```

## 基准结果

Linux amd64、Go 1.27.3、Intel Core i5-11500。固定 `GOMAXPROCS=1` 和 CPU 2，每项 300 ms、重复 3 次取中位数；按包依次测量基线与新实现。基线通过 overlay 恢复原始源码，两边使用相同基准驱动。

解析／分词场景包含 1,000 个四属性 `div` 和 1,000 个二属性 `input`；克隆／序列化场景包含 1,000 个四属性 `div` 及其文本节点。属性查询一轮查首项、末项和不存在的名称；属性修改一轮更新两次已有值，再添加并删除 `hidden`。逐项创建包含新节点及全部 `SetAttr`，因此 B/op 包含增长过程中已经退休的数组或 map 桶。

| 场景 | 耗时：前→后 | 耗时变化 | B/op：前→后 | allocs/op：前→后 |
| --- | ---: | ---: | ---: | ---: |
| 空节点创建 | 35.21→35.26 ns | +0.1% | 96→96 | 1→1 |
| 逐项创建 1 个属性 | 144.8→111.7 ns | −22.9% | 432→272 | 3→3 |
| 逐项创建 3 个属性 | 200.2→158.2 ns | −21.0% | 432→272 | 3→3 |
| 查询 1 个属性的节点 | 26.19→10.28 ns | −60.7% | 0→0 | 0→0 |
| 查询 3 个属性的节点 | 26.40→16.71 ns | −36.7% | 0→0 | 0→0 |
| 修改 3 个属性的节点 | 86.90→71.40 ns | −17.8% | 0→0 | 0→0 |
| 首次解析 | 1.756→1.634 ms | −7.0% | 960,577→560,160 | 5,047→5,047 |
| 复用解析器 | 990.31→948.45 μs | −4.2% | 0→0 | 0→0 |
| 模板克隆 | 365.09→104.69 μs | −71.3% | 544,288→208,288 | 5,002→3,002 |
| HTML 序列化 | 329.94→202.49 μs | −38.6% | 349,432→285,432 | 1,022→22 |

首次解析分配字节减少 **41.7%**；模板克隆减少 **61.7%**；1／3 个属性的逐项创建减少 **37.0%**。首次解析和逐项创建的分配次数没有减少；收益主要来自更小的存储。暖解析、查询、修改、组件 diff 和缓存样式计算仍为零分配。

以下成本保留在最终结果中：

| 场景 | 耗时：前→后 | 耗时变化 | 分配量变化 |
| --- | ---: | ---: | --- |
| 逐项创建 8 个属性 | 337.6→427.5 ns | +26.6% | 432→528 B，3→4 次 |
| 查询 8 个属性 | 28.59→31.22 ns | +9.2% | 均为零 |
| 修改 8 个属性 | 84.93→92.60 ns | +9.0% | 均为零 |
| 逐项创建 32 个属性 | 3.125→3.228 μs | +3.3% | 4,568→4,712 B，10→12 次 |
| 查询 32 个属性 | 23.53→23.91 ns | +1.6% | 均为零 |
| 修改 32 个属性 | 84.86→91.90 ns | +8.3% | 均为零 |
| 逐项创建 128 个属性 | 11.989→12.023 μs | +0.3% | 18,968→19,112 B，14→16 次 |
| 查询 128 个属性 | 22.99→23.93 ns | +4.1% | 均为零 |
| 修改 128 个属性 | 86.42→91.74 ns | +6.2% | 均为零 |
| 独立 token 分词 | 1.190→1.259 ms | +5.9% | 680,394→296,392 B，次数均为 4,018 |
| VNode 首次 diff | 114.11→116.83 μs | +2.4% | 均为 74,456 B、22 次 |
| VNode 暖 diff | 65.07→68.23 μs | +4.8% | 均为零 |

8 项逐个添加的额外分配来自列表增长；生产解析和批量构造直接预留全部属性。大集合保留 map 查找速度，增加存储头和晋升期间的少量分配。独立分词器需要为返回的每个 token 保存独立属性，因此在减少分配字节的同时略慢。child slot 更新均为 4.116 μs；完整样式计算首次／缓存路径耗时分别约 −0.8%／−1.1%，视为基本持平。

`B/op` 是累计分配字节数，不是存活堆、RSS 或显存；上述微基准不代表完整应用 FPS。少数百分点的耗时差异没有做统计显著性判断。

## 验证与复现

相关包测试通过：`html/dom`、`html/css`、`html/style`、`html/layout`、`ui/html`、`ui`、`module`、`editor` 和根包。`ui/markdown` 的 `TestMarkdownFractionRendersHorizontalBar` 在改动前及 overlay 恢复基线后都失败（未找到横线 shape），本轮未修改它的渲染逻辑；排除这一已有失败后，Markdown 包其他测试通过。

定向 race 检查通过，覆盖属性操作、共享源／克隆隔离、并发只读模板克隆、解析器重用及 HTML 组件作用域。最终自适应实现的解析模糊测试约执行 30 万个输入，无失败。WebAssembly DOM 测试通过，Android arm64 DOM 测试二进制编译通过。ADB 地址 `10.0.0.174:16384` 连接被拒绝，未做本轮真机运行。没有启动编辑器。

相关七个包的 `go vet`、`fieldalignment` 均无诊断，`git diff --check` 通过。

```sh
GOMAXPROCS=1 taskset -c 2 go test -p 1 ./html/dom ./ui/html ./html/style \
  -run '^$' \
  -bench '^Benchmark(DOMAttributes|DOMAttributeParsing|HTMLAttributeTemplates|HTMLComponentChildSlot|HTMLComponentDiff|ComputeFullStyles)$' \
  -benchmem -benchtime=300ms -count=3

go test -p 2 ./html/dom ./html/css ./html/style ./html/layout ./ui/html ./ui/markdown ./ui ./module ./editor . \
  -skip '^TestMarkdownFractionRendersHorizontalBar$'
go test -race ./html/dom ./ui/html \
  -run 'Test(Attributes|SharedAttributes|Parser|Tokenizer|HTMLAttribute|HTMLComponentScope)' -count=1
go test ./html/dom -run '^$' -fuzz '^FuzzParse$' -fuzztime=15s -parallel=2
go vet ./html/dom ./html/css ./html/style ./html/layout ./ui/html ./ui/markdown ./editor
fieldalignment ./html/dom ./html/css ./html/style ./html/layout ./ui/html ./ui/markdown ./editor
```

新增基准在 `html/dom/attributes_bench_test.go` 和 `ui/html/attributes_bench_test.go`。本次会话原始记录在 `/tmp/onix-dom-attrs-sz2hruoq`：`before/`、`before-overlay.json` 保存基线，`final-before.txt`／`final-after.txt` 为最终原始数据，`comparison.json` 为中位数，`markdown-baseline-failure.txt` 记录原有失败。给基准命令加 `-overlay /tmp/onix-dom-attrs-sz2hruoq/before-overlay.json` 可重测本地基线；临时目录可能随环境清理而失效。
