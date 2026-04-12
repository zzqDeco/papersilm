# research skills v1 计划

## 目标与收敛范围

本计划的目标，是把 `papersilm` 里的高频研究型工作流从“固定 prompt + 固定工具组合”升级为显式、可复用、可扩展的 skills。

一期收敛以下 skill 方向：

- reviewer 视角精读
- equation explain
- related work map
- compare refinement

## 当前现状与问题

当前系统已有一些接近 skill 的基础，但还不够显式：

- `internal/agent/dag.go` 已根据目标内容动态插入不同 worker。
- `internal/tools/registry.go` 提供固定工具集合。
- `internal/cli/repl.go` 支持 `/style <distill|ultra|reviewer>`，说明系统已经存在“输出风格切换”的需求。

当前问题在于：

- “reviewer” 仍更像风格选项，而不是完整工作流。
- 公式解释、related work 梳理、对比精炼等研究行为没有稳定命名和稳定输入输出。
- 新能力如果继续直接塞进固定工具和 planner，很快会让系统边界变得混乱。

## 目标体验 / 工作流

目标工作流如下：

1. 用户在已有 paper workspace 上选择某个 skill，而不是反复手写相似 prompt。
2. skill 拥有稳定的输入约束、执行步骤和产物类型。
3. skill 产物会回写到 workspace 和 task board，而不是只返回一段临时文本。
4. skill 既可以 inline 执行，也可以扩展为额外的 DAG / task。
5. 后续新增 skill 不必修改所有 CLI 主流程。

## 数据模型 / 协议 / 接口影响

建议新增以下概念：

- `SkillDescriptor`
  描述 skill 的名称、用途、输入要求、输出类型和默认执行方式。
- `SkillRunRequest`
  表示一次 skill 调用，绑定会话、目标论文或目标比较对象。
- `SkillArtifact`
  表示 skill 结果，允许回写到 workspace 或 task board。

实现边界：

- 一期不做任意用户脚本执行。
- 一期 skill 仍由仓库内置实现和注册，不做远程分发。
- skill 不替代底层工具；skill 应编排工具、任务和产物，而不是直接吞掉所有细节。

CLI 接口建议：

- `/skill list`
- `/skill run <name>`
- `/skill show <run-id>`

## 分阶段实施步骤

### 阶段 1：建立 skill 描述与注册机制

- 定义 `SkillDescriptor` 与 skill registry。
- 为 skill 约定稳定的输入输出结构和 artifact 类型。
- 明确 skill 与现有 style、tool、task 的关系。

### 阶段 2：落地首批内置 skills

- `reviewer`
- `equation-explain`
- `related-work-map`
- `compare-refinement`

每个 skill 都应有明确输入边界和产物落点。

### 阶段 3：接入 workspace 与 task board

- skill 运行结果回写到 `PaperWorkspace` 或 comparison workspace。
- skill 执行过程在 task board 中可见，可查看状态和输出。

### 阶段 4：与 dynamic tool surface 对齐

- skill 不直接持有全部工具实例。
- skill 通过 tool descriptor / tool search 按需拿到执行能力。

## 验收标准

- 至少四个研究型 skill 以显式对象存在，而不是继续以 prompt 魔法散落在系统里。
- skill 运行有稳定输入输出，不需要修改所有 CLI 路径才能接入。
- skill 产物可以挂接到 workspace 或 task board。
- `reviewer` 从 style 升级为完整 workflow 的方向清晰可落地。

## 明确不做

- 不做通用插件市场或第三方脚本执行平台。
- 不在一期引入远程 skill 下载、签名或权限沙箱。
- 不把所有 prompt 差异都硬塞成 skill。
- 不做与论文研究无关的通用 agent skill catalog。
