# service.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `pkg/core/service.go`
- 文档文件: `doc/src/pkg/core/service.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `core`

## 2. 核心职责
- 提供核心服务门面，把存储层、Agent 和事件 sink 组装成会话级公共操作接口。
- 负责新建会话、加载会话、执行任务、审批、运行已规划任务和附加来源。

## 3. 输入与输出
- 输入来源: 上层 CLI/REPL 传入的 `ClientRequest`、会话 ID、来源列表与审批结果。
- 输出结果: `RunResult`、`SessionMeta`、`SessionSnapshot` 以及追加到存储层的事件。

## 4. 关键实现细节
- 主要类型: `EventSink`、`Service`。
- 关键函数/方法: `New`、`NewSession`、`LoadSession`、`LatestSession`、`Execute`、`RunPlanned`、`Approve`、`AttachSources` 等。
- `NewSession()` 负责生成 session ID、写入初始元数据并发送初始化事件。
- `Execute()` 会在缺少 session ID 时先创建会话，再把请求交给 Agent。
- `emit()` 同时向 sink 和 session event log 写入事件。

## 5. 依赖关系
- 内部依赖: `internal/agent`、`internal/storage`、`pkg/protocol`
- 外部依赖: `context`、`crypto/rand`、`encoding/base32`、`fmt`、`time`

## 6. 变更影响面
- 这里是上层调用的稳定入口，方法签名变化会同时影响 CLI 和未来外部接入层。
- 事件写入策略变化会影响会话回放和调试体验。

## 7. 维护建议
- 保持门面层简单，把复杂的计划/执行编排继续放在 Agent 层。
- 修改 `pkg/core/service.go` 后，同步更新 `doc/src/pkg/core/service.go.plan.md`。
