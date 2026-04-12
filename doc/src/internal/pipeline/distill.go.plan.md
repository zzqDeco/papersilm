# distill.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/pipeline/distill.go`
- 文档文件: `doc/src/internal/pipeline/distill.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `pipeline`

## 2. 核心职责
- 实现单篇论文蒸馏主流程，把来源材料切块、分类、抽句并合成为结构化 `PaperDigest`。
- 在 AlphaXiv overview、full text 与 PDF 切块之间选择合适文本来源，并用启发式方法提取问题、方法、实验和结论。

## 3. 输入与输出
- 输入来源: 来源引用、目标文本、语言/风格、页面缓存或 AlphaXiv markdown。
- 输出结果: 结构化 `PaperDigest`、中间 `chunk` 切块，以及供 markdown 渲染的摘要字段。

## 4. 关键实现细节
- 主要类型: `chunk`。
- 关键函数/方法: `Distill`、`distillFromAlphaXiv`、`digestFromChunks`、`buildChunks`、`pageFallbackChunks`、`preferredContent`、`classifyChunk`、`topSentences` 等。
- `Distill()` 先加载 `SourceMaterial`，再组合 paper summary、experiment output 和可选数学输出。
- `digestFromChunks()` 负责从切块中提取问题、方法、实验、关键结果、结论和局限。
- `buildChunks()` 在递归切分器失败时回退到按页切块。
- 多个辅助函数用启发式句子筛选与 section 分类来避免把背景内容误当结论。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `context`、`fmt`、`regexp`、`sort`、`strings`、`time`、`unicode`、`github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive`、`github.com/cloudwego/eino/schema`

## 6. 变更影响面
- 该文件的规则直接决定单篇摘要质量、背景裁剪策略和 key result 提取结果。
- 切块策略或启发式条件的改动会连带影响 compare 阶段的输入质量。

## 7. 维护建议
- 若引入真正的模型抽取或引用对齐逻辑，应优先保持现有 `PaperDigest` 字段契约稳定。
- 修改 `internal/pipeline/distill.go` 后，同步更新 `doc/src/internal/pipeline/distill.go.plan.md`。
