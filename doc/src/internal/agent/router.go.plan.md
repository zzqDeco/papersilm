# router.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/agent/router.go`
- 文档文件: `doc/src/internal/agent/router.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `agent`

## 2. 核心职责
- 作为 Agent 层总调度器，衔接会话元数据、来源挂载、规划、审批门控、批次执行和事件流输出。
- 在 `plan`、`confirm`、`auto` 三种权限模式之间切换，并在执行过程中支持 DAG 级重规划。

## 3. 输入与输出
- 输入来源: `ClientRequest`、会话状态、存储层快照、事件 sink 与运行时语言/风格配置。
- 输出结果: `RunResult`、更新后的计划/执行状态、会话状态变更以及流式事件。

## 4. 关键实现细节
- 主要类型: `EventSink`、`Agent`。
- 关键函数/方法: `New`、`AttachSources`、`Execute`、`RunPlanned`、`Approve`、`syncSessionConfig`、`planSession`、`startConfirmExecution` 等。
- `AttachSources()` 在 replace 模式下会先删除旧 source 对应的 workspace 状态，再清空 sources 并触发计划失效。
- `Execute()` 统一处理附带来源、任务补全、会话配置同步和模式分发。
- `planSession()` 负责来源检查、DAG 规划、风险生成和计划/执行状态持久化，并在返回前为 `PlanResult.TaskBoard` 做首次 hydration。
- `runDAGExecution()` 循环选择 ready batch，发射进度事件，处理失败、重规划与最终收尾。
- `startConfirmExecution()` 与 `Approve()` 共同实现显式审批门。

## 5. 依赖关系
- 内部依赖: `internal/config`、`internal/storage`、`internal/tools`、`pkg/protocol`
- 外部依赖: `context`、`fmt`、`sort`、`strings`、`sync`、`time`

## 6. 变更影响面
- 该文件的改动会直接改变 CLI 和未来 GUI 看到的会话生命周期与事件语义。
- 这里如果状态流转不一致，会导致计划缓存、审批流程和批处理执行互相脱节。

## 7. 维护建议
- 新增会话状态或计划进度状态时，需同时检查协议层、输出层和存储层兼容性。
- 修改 `internal/agent/router.go` 后，同步更新 `doc/src/internal/agent/router.go.plan.md`。
