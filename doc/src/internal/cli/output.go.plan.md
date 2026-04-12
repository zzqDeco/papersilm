# output.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/cli/output.go`
- 文档文件: `doc/src/internal/cli/output.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `cli`

## 2. 核心职责
- 根据输出格式把事件流和最终运行结果渲染为文本、JSON 或 stream-json。
- 为文本模式补齐计划、审批请求、来源列表、digest 与 comparison 的可读展示。

## 3. 输入与输出
- 输入来源: `StreamEvent` 与 `RunResult`，以及在创建时绑定的目标输出 writer。
- 输出结果: 写入终端或管道的文本/JSON 内容。

## 4. 关键实现细节
- 主要类型: `OutputWriter`。
- 关键函数/方法: `NewOutputWriter`、`Emit`、`PrintResult`、`printPlan`、`printApproval`。
- `Emit()` 只对 text 与 stream-json 做逐事件输出。
- `PrintResult()` 会根据结果内容自动选择打印计划、审批、摘要或对比结果。
- `printPlan()` 与 `printApproval()` 定义了 REPL/CLI 默认的人类可读布局。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `encoding/json`、`fmt`、`io`、`strings`

## 6. 变更影响面
- 修改字段输出顺序或文本布局会直接影响 CLI 使用体验和外部脚本解析。
- stream-json 分支若不稳定，会破坏未来 GUI / 自动化消费路径。

## 7. 维护建议
- 新增结果类型时，优先评估 text、json、stream-json 三种输出是否都需要显式覆盖。
- 修改 `internal/cli/output.go` 后，同步更新 `doc/src/internal/cli/output.go.plan.md`。
