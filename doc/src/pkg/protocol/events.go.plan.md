# events.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `pkg/protocol/events.go`
- 文档文件: `doc/src/pkg/protocol/events.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `protocol`

## 2. 核心职责
- 定义运行时流式事件类型以及统一事件 envelope。
- 为 CLI 输出、会话事件日志和未来 GUI/流式消费提供公共协议。

## 3. 输入与输出
- 输入来源: 各层在运行期发出的状态、消息和 payload。
- 输出结果: 可序列化的 `StreamEventType` 和 `StreamEvent` 结构。

## 4. 关键实现细节
- 主要类型: `StreamEventType`、`StreamEvent`。
- 事件类型覆盖初始化、会话加载、来源挂载、分析、计划、审批、进度、结果和错误等关键阶段。

## 5. 依赖关系
- 内部依赖: 无直接内部包依赖。
- 外部依赖: `time`

## 6. 变更影响面
- 事件枚举和值是前后端或脚本消费的直接协议，变更需要审慎处理兼容性。

## 7. 维护建议
- 新增事件类型时，需同步检查输出层、存储层以及调用方是否需要显式分支处理。
- 修改 `pkg/protocol/events.go` 后，同步更新 `doc/src/pkg/protocol/events.go.plan.md`。
