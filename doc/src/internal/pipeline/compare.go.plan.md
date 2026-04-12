# compare.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/pipeline/compare.go`
- 文档文件: `doc/src/internal/pipeline/compare.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `pipeline`

## 2. 核心职责
- 把多篇 `PaperDigest` 聚合成结构化对比结果，并渲染成最终 markdown。
- 提供论文 ID 收集、首项提取、对比综合结论和矩阵渲染等轻量辅助函数。

## 3. 输入与输出
- 输入来源: 用户目标和已生成的多篇 `PaperDigest`。
- 输出结果: `ComparisonDigest` 及其 markdown 渲染内容。

## 4. 关键实现细节
- 关键函数/方法: `Compare`、`collectPaperIDs`、`firstLine`、`firstResult`、`buildSynthesis`、`renderComparison`、`writeMatrix`。
- `Compare()` 是对 `BuildFinalComparison()` 的轻薄封装。
- `renderComparison()` 固化了任务目标、论文列表、逐篇总结、三类矩阵和限制说明的输出顺序。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `sort`、`strings`

## 6. 变更影响面
- 这里的渲染布局会直接影响最终 comparison artifact 的可读性。
- 综合文案或矩阵生成规则变化会改变多论文对比的默认呈现方式。

## 7. 维护建议
- 新增对比维度时，同时扩展 digest 结构、矩阵渲染与综合结论生成逻辑。
- 修改 `internal/pipeline/compare.go` 后，同步更新 `doc/src/internal/pipeline/compare.go.plan.md`。
