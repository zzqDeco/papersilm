# workers.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/pipeline/workers.go`
- 文档文件: `doc/src/internal/pipeline/workers.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `pipeline`

## 2. 核心职责
- 定义 worker 级中间输出结构，并把来源材料转成 paper summary、experiment、math、web research 和 comparison matrix。
- 承担最终 digest / comparison 之前的结构化生产步骤，是 Agent worker 层调用的核心业务接口。

## 3. 输入与输出
- 输入来源: `SourceMaterial`、会话缓存、用户目标、语言/风格和已生成的 `PaperDigest`。
- 输出结果: 各类 worker output 结构、方法/实验/结果矩阵和最终 `ComparisonDigest`。

## 4. 关键实现细节
- 主要类型: `SourceMaterial`、`PaperSummaryOutput`、`ExperimentOutput`、`MathReasoningOutput`、`WebResearchOutput`。
- 关键函数/方法: `LoadSourceMaterial`、`loadAlphaXivMaterial`、`BuildPaperSummary`、`BuildExperimentOutput`、`BuildMathReasoning`、`BuildWebResearch`、`MergePaperDigest`、`BuildMethodMatrix` 等。
- `LoadSourceMaterial()` 统一从 AlphaXiv 或页面缓存中恢复可蒸馏材料。
- `BuildPaperSummary()` / `BuildExperimentOutput()` 拆分单篇摘要的两个主 worker 产物。
- `BuildMathReasoning()` 与 `BuildWebResearch()` 提供可选增强 worker。
- `MergePaperDigest()` 与三类 `Build*Matrix()` 负责把结构化中间产物拼装成最终对象。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `context`、`fmt`、`strings`、`time`

## 6. 变更影响面
- 这些结构体字段是 Agent 节点输出和最终 digest/comparison 之间的契约。
- 若中间输出变化，需同步更新 worker 执行层的解码和 merge 逻辑。

## 7. 维护建议
- 新增 worker output 时，保持输出字段可序列化，并同步更新节点执行与持久化路径。
- 修改 `internal/pipeline/workers.go` 后，同步更新 `doc/src/internal/pipeline/workers.go.plan.md`。
