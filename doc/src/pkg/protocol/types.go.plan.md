# types.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `pkg/protocol/types.go`
- 文档文件: `doc/src/pkg/protocol/types.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `protocol`

## 2. 核心职责
- 定义整个系统的核心协议类型，包括来源、摘要、对比、workspace、task board、DAG、执行状态、artifact、会话和请求/结果对象。
- 集中维护各类枚举与 JSON 结构，是 CLI、Agent、pipeline、storage 之间的共享契约。

## 3. 输入与输出
- 输入来源: 各层业务逻辑构造的领域数据与状态更新。
- 输出结果: 可序列化、可持久化、可跨层传递的协议结构体与枚举值。

## 4. 关键实现细节
- 主要类型: `SourceType`、`SourceStatus`、`PermissionMode`、`OutputFormat`、`ArtifactFormat`、`ContentSource`、`WorkerProfile`、`NodeKind`、`NodeStatus`、`SourceInspection`、`PaperRef`、`AnchorKind`、`AnchorRef`、`PaperNote`、`PaperAnnotation`、`PaperResource`、`SimilarPaperRef`、`SkillName`、`SkillTargetKind`、`SkillRunStatus`、`SkillDescriptor`、`SkillRunRecord`、`SkillRunResult`、`PaperWorkspace`、`TaskStatus`、`TaskActionType`、`TaskAction`、`TaskCard`、`TaskGroup`、`TaskBoard`、`Citation`、`KeyResult`、`PaperDigest`、`ComparisonMatrixRow`、`ComparisonDigest`、`PlanNode`、`PlanEdge`、`PlanDAG`、`NodeOutputRef`、`DagPatch`、`BatchStatus`、`ExecutionBatch`、`NodeExecutionState`、`ExecutionState`、`PlanStep`、`PlanResult`、`ApprovalRequest`、`PlanProgressStatus`、`PlanProgress`、`ArtifactManifest`、`SessionState`、`SessionMeta`、`SessionSnapshot`、`ClientRequest`、`RunResult`。
- 前半部分覆盖来源、权限模式、输出格式、worker profile、node kind/status 和 workspace 相关基础对象。
- 中段定义 `PaperDigest`、`ComparisonDigest`、`SkillRunRecord`、`PaperWorkspace`、`TaskBoard`、`PlanDAG`、`ExecutionState` 等核心对象。
- `TaskActionType` 现在同时承载 `inspect/run/approve/reject`，其中 `reject` 是 task board 与 CLI/GUI 共享的显式 task-level 拒绝动作。
- `ExecutionState.StaleNodeIDs` 把 task rerun 的下游失效语义持久化为独立集合，而不是污染底层 `NodeStatus`。
- `SessionSnapshot` 现在同时公开 `skill_runs` 与 `skill_artifacts`，`PaperWorkspace` 也会附带当前 paper 的 `skill_runs`，让 CLI 和未来 GUI 共用同一条技能协议面。
- 后半部分定义 `PlanResult`、`ApprovalRequest`、`PlanProgress`、`ArtifactManifest`、`SessionMeta`、`SessionSnapshot`、`RunResult` 等高层协作结构，其中 `PlanResult.TaskBoard` 与 `SessionSnapshot.TaskBoard` 是对外公开的 task 视图入口。

## 5. 依赖关系
- 内部依赖: 无直接内部包依赖。
- 外部依赖: `time`

## 6. 变更影响面
- 这是仓库里最关键的结构契约层；字段、枚举或语义变化会向 CLI、存储、事件和 artifact 全面扩散。
- `SessionSnapshot.Workspaces`、`SessionSnapshot.SkillRuns`、`SessionSnapshot.SkillArtifacts` 和 `SessionSnapshot.TaskBoard` 一起构成研究工作台的公开 JSON 面，持久化兼容性和结构稳定性都受这里控制。

## 7. 维护建议
- 新增字段优先保持向后兼容，并同步检查存储快照、输出格式和测试覆盖。
- 修改 `pkg/protocol/types.go` 后，同步更新 `doc/src/pkg/protocol/types.go.plan.md`。
