# papersilm Plan Index

本目录用于存放 `papersilm` 当前有效、可直接实施的计划文档。

这些文档的定位不是背景综述，而是后续实现工作的直接输入。若某份计划已经完成且不再作为当前实施依据，应通过 Git 历史追溯，而不是长期堆积在工作区。

## 当前权威计划

1. `papersilm-roadmap-overview.md`
   作用：定义项目接下来一段时间的产品收敛方向、阶段顺序和专题边界。
   依赖：无。新增或删除专题计划时，先更新这份总览。
2. `paper-workspace-v1.md`
   作用：把单篇论文处理结果升级为可持续积累的 paper workspace。
   依赖：依赖当前会话、artifact 与 citation 协议；为后续 memory、resource、skills 提供承载面。
3. `task-board-and-dag-execution.md`
   作用：把现有 DAG 与执行状态前台化为用户可理解的任务系统。
   依赖：依赖当前 `PlanDAG`、`ExecutionState` 与审批流；为 skills 和 resume 提供任务语义。
4. `research-skills-v1.md`
   作用：把高频研究型工作流从固定 prompt/style 提升为显式 skills。
   依赖：优先建立在 workspace 与 task board 的稳定语义之上。
5. `dynamic-tool-surface-and-tool-search.md`
   作用：把固定工具注册表拆成轻量索引与按需加载的执行工具。
   依赖：与 skills 并行推进，但应先于大量新工具接入落地。
6. `session-memory-compact-and-resume.md`
   作用：把现有会话持久化提升为可恢复、可压缩、可持续研究的长期记忆机制。
   依赖：依赖 workspace 和 task board 的稳定数据语义。
7. `mcp-resources-and-binary-artifacts.md`
   作用：补齐外部资源读取与大对象产物管理能力。
   依赖：依赖动态工具表面和存储约定，不与 tool search 混写。

## 推荐实施顺序

1. 先按 `papersilm-roadmap-overview.md` 锁定阶段目标和依赖关系。
2. 第一阶段优先做 `paper-workspace-v1.md` 与 `task-board-and-dag-execution.md`。
3. 第二阶段推进 `research-skills-v1.md` 与 `dynamic-tool-surface-and-tool-search.md`。
4. 第三阶段推进 `session-memory-compact-and-resume.md` 与 `mcp-resources-and-binary-artifacts.md`。

## 维护约定

- `plan/` 只保留当前仍然生效的实施计划。
- `doc/` 与 `doc/src/` 负责记录当前实现，不承担未来路线图职责。
- 每份计划都应明确写出目标收敛范围、现状问题、目标工作流、接口影响、实施步骤、验收标准和明确不做。
- 若计划间依赖发生变化，应同时更新本文件和 `papersilm-roadmap-overview.md`。
