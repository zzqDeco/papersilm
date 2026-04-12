# tasks.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/agent/tasks.go`
- 文档文件: `doc/src/internal/agent/tasks.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `agent`

## 2. 核心职责
- 在现有 batch 执行引擎之上补充 task 级读取、单任务执行、单任务审批和 rerun 级联失效语义。
- 把 `PlanNode -> TaskCard` 的用户动作映射回真实的 DAG / execution state 变更。

## 3. 输入与输出
- 输入来源: 会话 ID、task ID、当前 `PlanResult` / `ExecutionState`、语言/风格配置与审批结果。
- 输出结果: 更新后的 `RunResult`、按 task 范围执行后的计划状态、审批请求和被清理/重建的产物。

## 4. 关键实现细节
- 关键函数/方法: `RunTask`、`ApproveTask`、`runScopedExecution`、`finishTaskApproval`、`dependencyClosure`、`cascadeRerun`、`deleteArtifactsForNodes`。
- `RunTask()` 会根据 task 状态决定是首次执行、失败重试还是 completed rerun，并在 rerun 时把目标及其下游 descendants 标记为 stale/pending。
- `runScopedExecution()` 只执行目标 task 所需的未完成依赖闭包，不会自动拉起下游节点。
- `ApproveTask()` 在 confirm 模式下只批准并执行指定 pending task，其他同批任务保持 `awaiting_approval`。
- `deleteArtifactsForNodes()` 负责把 merge/comparison 节点对应的 digest、comparison 和 artifact 文件做级联清理。

## 5. 依赖关系
- 内部依赖: `internal/storage`、`pkg/protocol`
- 外部依赖: `context`、`fmt`、`sort`、`time`

## 6. 变更影响面
- 这里直接定义 `/task run` 和 `/task approve` 的用户语义，错误地放宽或收紧条件都会影响审批流和 rerun 行为。
- 级联清理逻辑如果不完整，会让 task board 状态和磁盘上的 artifact 真实状态脱节。

## 7. 维护建议
- 新增 task 动作或 rerun 语义时，优先先补 service / storage / CLI 的集成测试，再改这里的状态流转。
- 修改 `internal/agent/tasks.go` 后，同步更新 `doc/src/internal/agent/tasks.go.plan.md`。
