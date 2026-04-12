# task board 与 DAG execution 计划

## 目标与收敛范围

本计划的目标，是把 `papersilm` 当前已经存在的 DAG 和执行状态，提升为用户可理解、可操作、可恢复的任务系统。

本计划聚焦：

- task view model
- execution board
- approval 与 rerun 语义
- CLI-first 的任务操作入口

不在本计划里重写执行引擎本身。

## 当前现状与问题

当前系统已有明确执行骨架：

- `internal/agent/dag.go` 能把目标编译成 `PlanDAG` 并生成 `ExecutionState`。
- `pkg/protocol/types.go` 已有 `PlanNode`、`PlanEdge`、`ExecutionBatch`、`NodeExecutionState` 等协议结构。
- `internal/tools/registry.go` 能根据审批与执行模式构建相关工具。
- `internal/cli/repl.go` 已提供 `/plan`、`/run`、`/approve` 等命令。

但这些能力还主要停留在系统内部：

- DAG 节点仍偏向底层 worker 视角，用户很难直接理解其职责和依赖。
- 审批和执行更多是“整个计划”粒度，缺少按任务理解和选择的体验。
- rerun、partial rerun、invalidated state 还没有以用户可读形式呈现。
- 当前 JSON 输出适合机器消费，但不等于人类友好的任务板。

## 目标体验 / 工作流

目标工作流如下：

1. 用户执行 `/plan` 后，不只是看到原始 DAG，而是看到按论文和任务类型组织的 task board。
2. 用户可以查看某个任务的输入来源、依赖关系、预期产物和当前状态。
3. 用户可以对需要审批的任务进行确认，对失败或过期任务进行单独 rerun。
4. 用户恢复会话时，可以直接看到哪些任务已完成、哪些待执行、哪些需要审批。
5. skills 和 workspace 后续都基于 task board 挂接，而不是重新发明另一套执行语义。

## 数据模型 / 协议 / 接口影响

建议在现有 DAG 协议上增加一层稳定的任务视图模型：

- `TaskCard`
  面向用户的任务对象，包含标题、描述、状态、所属论文、依赖摘要、产物摘要。
- `TaskGroup`
  用于把任务按论文、阶段或主题分组。
- `TaskAction`
  明确可执行动作，例如 approve、run、rerun、skip、inspect output。

实现原则：

- 现有 `PlanNode` / `ExecutionState` 仍然保留，作为底层执行真实来源。
- `TaskCard` 只是把底层节点投影成稳定的人类界面，不与执行引擎脱节。
- 审批请求应能映射回具体 task，而不是只映射到整个批次。

CLI 接口建议：

- `/tasks`
- `/task show <id>`
- `/task run <id>`
- `/task approve <id>`

## 分阶段实施步骤

### 阶段 1：建立 task projection

- 从现有 `PlanDAG` 与 `ExecutionState` 投影出稳定的 `TaskCard` 列表。
- 给当前节点补齐用户可读标题、描述和产物摘要生成规则。
- 明确 task ID 与底层 node ID 的映射关系。

### 阶段 2：暴露 CLI 任务入口

- 在 REPL 中增加任务查看与单任务操作命令。
- 让 `/plan` 的默认展示更偏任务板，而不是只打印底层执行结构。

### 阶段 3：补齐 rerun / invalidate 语义

- 将来源变更导致的 plan invalidation 映射到 task 级状态变化。
- 支持按任务查看为什么需要重跑、会影响哪些下游任务。

### 阶段 4：与后续 plans 对齐

- 让 `research-skills-v1.md` 新增的 skill 执行也以 task 形式出现。
- 让 `session-memory-compact-and-resume.md` 可以直接基于 task 状态生成 resume 摘要。

## 验收标准

- 同一个计划可以以任务板形式稳定展示，而不是要求用户理解底层 worker 命名。
- 用户可以查看单个任务的状态、依赖和产物信息。
- 审批和 rerun 至少能在协议和 CLI 层定位到具体任务。
- task board 不要求重写底层执行器，仍复用现有 `PlanDAG` 与 `ExecutionState`。

## 明确不做

- 不做可视化拖拽式 DAG 编辑器。
- 不做分布式调度、远程 worker 或后台常驻任务服务。
- 不做完整的项目管理看板系统。
- 不在本计划里重写 DAG 编译逻辑和核心执行器。
