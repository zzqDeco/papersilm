# chatmodel.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/providers/chatmodel.go`
- 文档文件: `doc/src/internal/providers/chatmodel.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `providers`

## 2. 核心职责
- 根据配置选择真实 provider chat model 或本地 deterministic tool-calling fallback。
- 把统一的 provider 配置映射成不同 Eino 扩展模型的构造参数。

## 3. 输入与输出
- 输入来源: `ProviderConfig`、超时时间和上下文。
- 输出结果: 满足 `model.ToolCallingChatModel` 的具体实现。

## 4. 关键实现细节
- 关键函数/方法: `BuildChatModel`、`useLocalAgentModel`。
- `BuildChatModel()` 负责 provider 分支与不支持 provider 的错误返回。
- `useLocalAgentModel()` 根据模型名、API key 或 Ollama base URL 决定是否启用本地回退模型。

## 5. 依赖关系
- 内部依赖: `internal/config`
- 外部依赖: `context`、`fmt`、`strings`、`time`、`github.com/cloudwego/eino-ext/components/model/ark`、`github.com/cloudwego/eino-ext/components/model/deepseek`、`github.com/cloudwego/eino-ext/components/model/ollama`、`github.com/cloudwego/eino-ext/components/model/openai`、`github.com/cloudwego/eino-ext/components/model/qwen`、`github.com/cloudwego/eino/components/model`

## 6. 变更影响面
- provider 选择逻辑会直接影响线上模型调用与无外部配置时的本地降级路径。
- 超时或鉴权参数映射不正确会造成模型层不可用。

## 7. 维护建议
- 新增 provider 时，同时补齐配置枚举、构造参数映射和回退判断逻辑。
- 修改 `internal/providers/chatmodel.go` 后，同步更新 `doc/src/internal/providers/chatmodel.go.plan.md`。
