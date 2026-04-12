# registry.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/tools/registry.go`
- 文档文件: `doc/src/internal/tools/registry.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `tools`

## 2. 核心职责
- 维护 `papersilm` 可用工具集合，把 pipeline 能力包装成 attach/inspect/distill/compare/export/list assets 等工具。
- 为审批场景、artifact 导出和会话资产查询提供统一的工具层入口。

## 3. 输入与输出
- 输入来源: `pipeline.Service`、会话存储、session ID、工具输入结构以及 tool runtime 上下文。
- 输出结果: `tool.BaseTool` 列表、来源列表、摘要/对比产物、artifact manifest 和会话资产集合。

## 4. 关键实现细节
- 主要类型: `Registry`、`DistillToolInput`、`DistillToolResult`、`CompareToolInput`、`CompareToolResult`、`ExportArtifactInput`、`ExportArtifactResult`、`SessionAssets`、`approvalToolInput`。
- 关键函数/方法: `New`、`Pipeline`、`init`、`AttachSources`、`InspectSources`、`ToolNames`、`BuildExecutionTools`、`LookupAlphaXivOverview` 等。
- `AttachSources()` / `InspectSources()` 负责来源去重、检查与计划失效。
- `BuildExecutionTools()` 根据是否需要审批，组装审批、distill、compare、export、list assets 等工具。
- `buildDistillTool()` 与 `buildCompareTool()` 用 graph tool 包装具体 workflow。
- `writeArtifact()` 在工具层也复用统一的 artifact 落盘约定。

## 5. 依赖关系
- 内部依赖: `internal/pipeline`、`internal/storage`、`internal/tools/graphtool`、`pkg/protocol`
- 外部依赖: `context`、`encoding/gob`、`encoding/json`、`fmt`、`io`、`os`、`path/filepath`、`strings`、`time`、`github.com/cloudwego/eino/components/tool`、`github.com/cloudwego/eino/components/tool/utils`、`github.com/cloudwego/eino/compose`、`github.com/cloudwego/eino/schema`

## 6. 变更影响面
- 工具名、输入结构和返回结构是 agent/tool-calling 层的直接契约。
- 这里如果与 pipeline 或 storage 契约脱节，会导致工具执行成功但会话状态不同步。

## 7. 维护建议
- 新增工具时，同时更新 `ToolNames()`、执行工具集合和相关输入输出结构定义。
- 修改 `internal/tools/registry.go` 后，同步更新 `doc/src/internal/tools/registry.go.plan.md`。
