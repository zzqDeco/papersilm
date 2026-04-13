# projector.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/taskboard/projector.go`
- 文档文件: `doc/src/internal/taskboard/projector.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `taskboard`

## 2. 核心职责
- 从 `PlanResult`、`ExecutionState`、`SessionMeta`、`ArtifactManifest` 和 `PaperWorkspace` 水合出公开 `TaskBoard`。
- 负责把底层节点状态转换成用户可理解的 task 状态、分组和可用动作。

## 3. 输入与输出
- 输入来源: 已存在的 plan/execution 快照、artifact 列表和 workspace 列表。
- 输出结果: 供 CLI、JSON 和未来 GUI 共用的 `TaskBoard` 视图。

## 4. 关键实现细节
- 关键函数/方法: `Build`、`deriveTaskStatus`、`availableActions`、`artifactIDsForNode`、`taskTitle`、`taskDescription`。
- `Build()` 不额外持久化任何 task board 文件，而是每次按需从真实会话状态投影。
- `deriveTaskStatus()` 会把 `NodeStatus + stale_node_ids + pending_node_ids + SessionState` 组合成 `blocked/ready/awaiting_approval/running/completed/failed/stale/skipped`。
- `taskGroupID()` 把单篇节点固定投影到 `paper:<paper_id>` 分组，把 compare / synthesis 节点投影到 `comparison` 分组。
- `availableActions()` 在 approval gate 打开时只允许当前 `pending_node_ids` 内的 task 暴露 `approve/reject`；其他 task 即使底层已 ready，也只保留 `inspect`，防止 UI/CLI 暴露不可执行动作。
- `artifactIDsForNode()` 会优先读取 node outputs，再回退到当前 artifact manifests 补齐 merge/final synthesis 的持久化产物引用。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `sort`、`strings`、`time`

## 6. 变更影响面
- 这里的投影规则定义了 CLI 和未来 GUI 看到的 task board 契约，状态映射变化会直接改变用户理解的执行状态。
- 分组、标题和动作映射如果不稳定，会造成同一个底层 DAG 在不同入口下呈现不一致。

## 7. 维护建议
- 新增 node kind 或审批状态时，先补这里的标题、状态和动作投影，再改 CLI 展示。
- 修改 `internal/taskboard/projector.go` 后，同步更新 `doc/src/internal/taskboard/projector.go.plan.md`。
