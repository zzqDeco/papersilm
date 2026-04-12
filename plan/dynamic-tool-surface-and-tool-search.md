# dynamic tool surface 与 tool search 计划

## 目标与收敛范围

本计划的目标，是把 `papersilm` 当前固定、预先注册的工具集合，演进为“轻量工具索引 + 按需加载执行工具”的动态工具表面。

本计划聚焦：

- tool descriptor
- tool search
- lazy schema / lazy construction
- 与 skill 的衔接

## 当前现状与问题

当前系统的工具组织方式主要集中在 `internal/tools/registry.go`：

- 工具列表相对固定。
- `BuildExecutionTools()` 会按当前模式构建执行所需工具。
- 工具输入输出结构由注册表直接定义和暴露。

这在当前规模下足够简单，但一旦继续扩能力，会出现明显问题：

- 工具数量增加后，全部 schema 一次性暴露会推高 prompt 和 tool-calling 负担。
- 研究型能力、资源型能力、导出型能力会混在同一层，边界越来越差。
- skill 如果继续直接依赖全量工具集合，会丧失组合弹性。

## 目标体验 / 工作流

目标工作流如下：

1. Agent 默认只看到轻量工具索引，而不是完整工具集合。
2. 当用户目标指向某类能力时，Agent 先通过 tool search 找到候选工具。
3. 只有在决定真正调用时，系统才物化该工具的完整 schema 与执行实例。
4. skills 与 MCP resources 也通过统一的 descriptor 层接入，而不是再开一套发现机制。

## 数据模型 / 协议 / 接口影响

建议引入：

- `ToolDescriptor`
  描述工具名、用途、类别、输入摘要、输出摘要、成本、风险和依赖条件。
- `ToolSearchResult`
  表示搜索结果，至少包含候选工具列表和命中原因。
- `ToolFactory`
  从 descriptor 构建真实执行工具的工厂层。

设计约束：

- 保留当前固定工具调用能力，避免一次性重写全部执行逻辑。
- 先把“描述”和“执行构建”拆开，再做延迟 schema 暴露。
- descriptor 层要能服务 skill，也要能服务未来的资源工具与 GUI 提示。

## 分阶段实施步骤

### 阶段 1：拆分 descriptor 与执行实现

- 把当前 `Registry` 中的工具元信息抽离为 descriptor。
- 保持现有工具行为不变，只调整组织方式。

### 阶段 2：增加 tool search

- 建立最小工具索引。
- 允许 Agent 按能力、主题或对象类型搜索工具。
- 让 tool search 返回候选工具和理由，而不是直接执行。

### 阶段 3：引入按需加载 schema

- 只有在真正需要执行时才构建完整工具实例和 schema。
- 避免把未来大量低频工具一次性暴露给模型。

### 阶段 4：与 skills / MCP 对齐

- 让 skill 通过 descriptor 层发现所需工具。
- 为 `mcp-resources-and-binary-artifacts.md` 中的资源读取能力预留同一入口。

## 验收标准

- 当前已有工具仍能正常执行，功能不回退。
- 工具元信息与工具构建逻辑明确分层。
- 系统具备工具搜索和按需实例化能力，不再要求所有工具预先暴露。
- 本计划与 MCP resources 计划边界清晰，没有把资源引用和工具发现混写在一起。

## 明确不做

- 不做外部工具市场或任意运行时代码加载。
- 不在本计划里引入复杂安全策略系统。
- 不把所有能力都抽象成不可理解的元编程层。
- 不以牺牲现有简单执行路径为代价强推过度设计。
