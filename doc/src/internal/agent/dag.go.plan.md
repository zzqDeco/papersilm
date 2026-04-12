# dag.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/agent/dag.go`
- 文档文件: `doc/src/internal/agent/dag.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `agent`

## 2. 核心职责
- 把论文处理目标和已检查来源编译成显式 DAG，定义单篇摘要、实验、数学、网页研究和多论文对比节点。
- 维护 DAG 节点就绪状态、拓扑顺序投影和执行状态的初始快照。

## 3. 输入与输出
- 输入来源: 用户目标文本、已检查的 `PaperRef` 列表，以及中间的 `taskSpec`。
- 输出结果: `PlanDAG`、投影后的 `PlanStep` 序列和初始 `ExecutionState`。

## 4. 关键实现细节
- 主要类型: `taskKind`、`taskSpec`。
- 关键函数/方法: `buildTaskSpecs`、`compileDAG`、`newNode`、`projectSteps`、`firstProduce`、`topoSortedNodeIDs`、`buildExecutionState`、`refreshReadyNodes` 等。
- `buildTaskSpecs()` 会先过滤出可提取文本的来源，再决定是否追加 compare 任务。
- `compileDAG()` 依据目标内容动态插入 `math_reasoner` / `web_research` 等可选节点。
- `refreshReadyNodes()` 与 `topoSortedNodeIDs()` 负责把依赖关系转成可执行状态。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `fmt`、`sort`、`strings`、`time`

## 6. 变更影响面
- 修改节点构成或依赖边会直接影响计划可视化、批次选择和最终执行顺序。
- 这里的节点 ID 和 kind 也会影响执行器与结果持久化的匹配逻辑。

## 7. 维护建议
- 新增 worker 类型时，先在协议类型中补齐枚举，再同步这里的 DAG 编译逻辑。
- 修改 `internal/agent/dag.go` 后，同步更新 `doc/src/internal/agent/dag.go.plan.md`。
