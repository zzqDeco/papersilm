# local_model.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/providers/local_model.go`
- 文档文件: `doc/src/internal/providers/local_model.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `providers`

## 2. 核心职责
- 提供一个本地 deterministic tool-calling chat model，用于在无外部 provider 时模拟 planner、executor 和 replanner。
- 把 JSON envelope 转成工具调用消息，使计划、执行和重规划链路能在离线模式下跑通。

## 3. 输入与输出
- 输入来源: 模型消息列表、工具定义、planner/executor/replanner JSON 负载。
- 输出结果: assistant message 或 tool call，包含计划步骤、执行请求或完成响应。

## 4. 关键实现细节
- 主要类型: `LocalToolCallingChatModel`、`localEnvelope`、`localPlannerInput`、`localExecutorInput`、`localPlanEnvelope`、`localReplannerInput`。
- 关键函数/方法: `NewLocalToolCallingChatModel`、`WithTools`、`Generate`、`Stream`、`generate`、`generatePlanner`、`generateExecutor`、`generateReplanner` 等。
- `WithTools()` 复制当前可用工具信息，供后续 `assistantToolCall` 选择名称。
- `generate()` 根据 envelope 的 `mode` 分派到 planner / executor / replanner。
- `buildLocalPlanSteps()` 为离线模式生成最小可执行计划，并在多篇论文时追加 compare 步骤。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `context`、`encoding/json`、`fmt`、`strings`、`time`、`github.com/cloudwego/eino/components/model`、`github.com/cloudwego/eino/schema`

## 6. 变更影响面
- 这是无 provider 配置时 plan/confirm/auto 仍能端到端运行的关键兜底。
- 离线模式输出格式若与真实工具表不一致，会直接破坏执行链路。

## 7. 维护建议
- 新增工具或 envelope mode 时，保持本地回退输出与真实运行时的工具契约一致。
- 修改 `internal/providers/local_model.go` 后，同步更新 `doc/src/internal/providers/local_model.go.plan.md`。
