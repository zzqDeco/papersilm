# papersilm 渐进式路线图总览

## 目标与收敛范围

本路线图的目标，是把 `papersilm` 从“CLI-first 的论文摘要与比较工具”逐步推进为“可持续积累研究上下文的 paper workspace”。

本轮路线图只收敛以下六条主线：

- paper workspace
- task board 与 DAG execution
- research skills
- dynamic tool surface 与 tool search
- session memory / compact / resume
- MCP resources 与 binary artifacts

本路线图明确不把项目扩展为通用 coding agent，也不把产品边界拉向公开社区站点。

## 当前现状与问题

当前仓库已经具备较好的核心底座：

- `pkg/core/service.go` 提供会话级门面，负责会话创建、加载、执行与审批。
- `internal/agent/dag.go` 已能把目标编译成显式 DAG，并维护 ready node 与执行批次。
- `internal/storage/store.go` 已持久化会话、计划、执行状态、digest、comparison、artifact 与 event log。
- `internal/tools/registry.go` 已提供固定工具集合，支撑 attach / inspect / distill / compare / export 等流程。
- `internal/cli/repl.go` 已提供 `/plan`、`/run`、`/approve`、`/export` 等 CLI 入口。

但当前能力仍以“一次性执行任务”为中心，缺少以下产品面：

- 缺少一等公民的 notes / anchored annotations / resources / similar 结构。
- 缺少用户可直接操作的任务面板，DAG 仍主要停留在协议层与 JSON 输出层。
- 缺少可复用的研究型 skills，当前风格切换仍偏 prompt 选项。
- 缺少延迟工具发现，工具集合会随能力增长快速膨胀。
- 缺少显式的 session memory / compact / resume 语义。
- 缺少对外部资源与大对象产物的统一引用与存储约定。

## 目标体验 / 工作流

目标状态下，`papersilm` 的主工作流应当是：

1. 用户附加论文来源并生成初始计划。
2. 系统把计划呈现为可理解的任务板，而不是只是一组底层 DAG 节点。
3. 用户在单篇或多篇论文上下文中积累私有笔记、锚点批注、资源清单和相似论文线索。
4. 用户通过 skills 触发高频研究工作流，例如 reviewer 视角精读、公式拆解、related work map 和对比精炼。
5. Agent 不再一次性暴露全部工具，而是先通过工具索引和搜索发现所需能力，再按需加载。
6. 长会话可以恢复、压缩并延续，关键决定、关键引用与关键产物不会在上下文膨胀中丢失。
7. 外部资源和二进制产物不直接塞进会话 JSON，而是通过稳定引用和 manifest 管理。

## 数据模型 / 协议 / 接口影响

本路线图会逐步影响以下层面：

- `pkg/protocol/types.go`
  需要新增 workspace、task view、skill descriptor、memory、resource handle 等结构。
- `internal/storage/store.go`
  需要新增 workspace / memory / resource / blob manifest 等持久化布局。
- `internal/tools/registry.go`
  需要从固定工具注册逐步演进到“描述符索引 + 按需构建执行工具”。
- `internal/cli/repl.go`
  需要新增与 task、note、memory、resume、skill 相关的显式命令。
- 未来 GUI
  应继续复用相同的协议和 artifact，而不是另起一套状态模型。

总原则是：优先扩展现有协议与存储，而不是推翻当前 CLI-first、artifact-first 的核心设计。

## 分阶段实施步骤

### 第一阶段：建立产品骨架

- 落地 `paper-workspace-v1.md`，让单篇论文拥有私有 notes、anchored annotations、resources、similar。
- 落地 `task-board-and-dag-execution.md`，把已有 DAG 变成用户可直接理解和操作的任务系统。

第一阶段完成后，`papersilm` 应从“执行一次任务”过渡到“围绕论文持续工作”。

### 第二阶段：提升能力组织方式

- 落地 `research-skills-v1.md`，把高频研究流程提炼为可复用 skills。
- 落地 `dynamic-tool-surface-and-tool-search.md`，避免工具数量增长带来的 prompt 和 schema 膨胀。

第二阶段完成后，新增能力应优先通过 skill 和 tool descriptor 进入系统，而不是继续把固定工具塞进全局表面。

### 第三阶段：补齐长期会话与外部资源

- 落地 `session-memory-compact-and-resume.md`，支持长会话恢复与压缩。
- 落地 `mcp-resources-and-binary-artifacts.md`，补齐外部资源与大对象的统一管理。

第三阶段完成后，`papersilm` 应具备长期研究工作台的基本连续性。

## 验收标准

- 六条专题主线都有独立、可实施、边界清晰的计划文件。
- 各专题计划的依赖关系和阶段顺序在本文件中可直接追踪。
- 每份专题计划都明确了“做什么”和“不做什么”，没有把范围扩到通用 coding agent、公开社区或重型平台能力。
- 根 `README.md`、`plan/README.md` 与本文件对 `plan/` 的职责描述一致。

## 明确不做

- 不把 `papersilm` 扩展为 IDE bridge、voice、remote server 或通用插件市场。
- 不在当前路线图里引入公开评论、点赞、热榜、社交 feed 等 AlphaXiv 式社区层。
- 不以推翻现有协议和存储为代价重写核心架构。
- 不在本轮计划里引入多 agent 协作、分布式调度或复杂权限系统。
