# session memory / compact / resume 计划

## 目标与收敛范围

本计划的目标，是把 `papersilm` 现有的“会话能持久化”提升为“会话可恢复、可压缩、可持续研究”的长期记忆能力。

本计划聚焦：

- resume entrypoint
- session memory
- compact 机制
- 关键决定与关键产物保留

## 当前现状与问题

当前仓库已经有不错的持久化基础：

- `pkg/core/service.go` 支持新建会话、加载会话和获取最近会话。
- `internal/storage/store.go` 可以持久化 meta、sources、plan、execution、digest、comparison、artifact 和 events。
- REPL 在一次会话内可以持续操作同一组来源和结果。

但现状仍缺少长期研究体验需要的几层抽象：

- “能加载会话”不等于“能快速恢复上下文”。
- 事件日志与执行状态比较底层，缺少面向人的记忆摘要。
- 会话越长，原始状态越臃肿，缺少 compact 语义来保留关键内容、压缩低价值细节。
- 目前没有显式的 `/resume`、`/memory`、`/compact` 产品面。

## 目标体验 / 工作流

目标工作流如下：

1. 用户重新进入项目时，可以直接恢复最近会话或指定会话。
2. 系统先展示一份 compact 的会话记忆摘要，包括当前论文、关键结论、未完成任务、重要笔记和主要资源。
3. 用户可以固定关键事实、关键判断和关键产物，防止后续压缩时丢失。
4. 当会话过长时，系统可以把低价值中间状态压缩为摘要，同时保留任务状态、人工决定和关键引用。

## 数据模型 / 协议 / 接口影响

建议新增：

- `SessionMemory`
  表示某个会话当前的高价值研究记忆快照。
- `MemoryEntry`
  表示具体记忆片段，例如目标、结论、待办、决策、风险、重要引用。
- `CompactionSnapshot`
  表示一次 compact 操作生成的摘要和保留边界。
- `ResumeHint`
  表示恢复会话时需要展示的简明导语。

与现有层的关系：

- `SessionSnapshot` 继续保存原始结构状态。
- `SessionMemory` 是从 snapshot、event、workspace、task board 中提炼出的高价值层。
- compact 不应破坏可审计性；必要时仍能回到完整事件和产物。

CLI 接口建议：

- `/resume [session-id]`
- `/memory`
- `/compact`

## 分阶段实施步骤

### 阶段 1：从现有会话状态中提炼 memory

- 明确哪些信息属于长期应保留的记忆。
- 从 snapshot、event、workspace、task board 中生成结构化 `SessionMemory`。

### 阶段 2：增加显式 resume 入口

- 在 CLI 中补齐恢复最近会话和指定会话的显式命令。
- 恢复时默认展示简洁摘要，而不是把原始状态整段倒给用户。

### 阶段 3：增加 compact 语义

- 允许把执行细节、重复中间结果压缩为摘要。
- 允许用户 pin 住关键事实、关键任务、关键引用和关键产物。

### 阶段 4：与 workspace / task board 对齐

- `paper-workspace-v1.md` 中的 notes、resources、similar 要成为 memory 的重要来源。
- `task-board-and-dag-execution.md` 中的任务状态要成为 resume 摘要的一部分。

## 验收标准

- 用户可以显式恢复最近会话或指定会话。
- 会话恢复后能看到结构化摘要，而不是只能重新翻原始事件和 JSON。
- compact 之后，关键决定、关键任务、关键引用和关键产物仍被保留。
- memory 设计能与 workspace 和 task board 形成稳定联动。

## 明确不做

- 不做跨用户共享记忆。
- 不强制引入向量数据库或重型检索系统。
- 不在本计划里实现主动型后台 agent。
- 不把 compact 做成不可逆、不可审计的黑箱清理。
