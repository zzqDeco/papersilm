# repl.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/cli/repl.go`
- 文档文件: `doc/src/internal/cli/repl.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `cli`

## 2. 核心职责
- 实现交互式 REPL，会话级读取用户输入并把自然语言任务或 slash command 分发给核心服务。
- 维护当前会话快照，并在 REPL 内支持语言、风格、来源、workspace、task board、审批、运行和导出等命令。

## 3. 输入与输出
- 输入来源: 标准输入行、当前 `SessionSnapshot`、核心服务实例与存储层。
- 输出结果: 写到标准输出的提示/结果、workspace 视图，以及更新后的当前会话状态。

## 4. 关键实现细节
- 关键函数/方法: `RunREPL`、`handleSlash`、`handleSourceCommand`、`handleWorkspaceCommand`、`handleTaskCommand`。
- `RunREPL()` 负责创建默认会话、循环读取输入并区分自然语言与 slash command。
- `handleSlash()` 处理 `/plan`、`/run`、`/approve`、`/tasks`、`/task ...`、`/lang`、`/style`、`/workspace` 等顶层命令。
- `handleSourceCommand()` 实现来源列表、增删替换与对应的计划失效，并在 remove/replace 时同步清理对应 workspace。
- `handleWorkspaceCommand()` 负责解析 `::` 分隔的自由文本正文，并把 note/annotation 写入委托给 core service。
- `handleTaskCommand()` 负责 `/task show|run|approve` 的参数路由，把单任务读取、执行和审批交给 core service。

## 5. 依赖关系
- 内部依赖: `internal/storage`、`pkg/core`、`pkg/protocol`
- 外部依赖: `bufio`、`context`、`fmt`、`io`、`os`、`strings`

## 6. 变更影响面
- 这里的命令语义就是 CLI 交互模式的用户契约，改动会直接影响文档与使用习惯。
- 会话对象更新如果遗漏，REPL 会出现显示状态与真实存储状态不一致的问题，尤其会影响 workspace 视图、task board 和人工批注写回。

## 7. 维护建议
- 新增 slash command 时，同时更新 `/help` 输出和相关会话持久化逻辑。
- 修改 `internal/cli/repl.go` 后，同步更新 `doc/src/internal/cli/repl.go.plan.md`。
