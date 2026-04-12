# root.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/cli/root.go`
- 文档文件: `doc/src/internal/cli/root.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `cli`

## 2. 核心职责
- 定义 `papersilm` 根命令、flag、版本子命令以及单次执行与 REPL 两条运行入口。
- 负责装配运行时依赖，包括配置、存储、pipeline、tool registry、agent、core service 和输出器。

## 3. 输入与输出
- 输入来源: Cobra flag、配置文件、会话恢复参数与输出格式选择。
- 输出结果: 初始化完成的命令树；运行时返回单次 `RunResult` 或进入 REPL。

## 4. 关键实现细节
- 关键函数/方法: `NewRootCommand`、`newVersionCommand`、`buildRuntime`。
- `NewRootCommand()` 统一处理 `--print`、`--source`、`--resume`、`--permission-mode` 等入口参数。
- `buildRuntime()` 组装配置、存储、pipeline、tools、agent、core service 和输出 writer。
- `newVersionCommand()` 暴露构建元数据查询子命令。

## 5. 依赖关系
- 内部依赖: `internal/agent`、`internal/config`、`internal/pipeline`、`internal/storage`、`internal/tools`、`internal/version`、`pkg/core`、`pkg/protocol`
- 外部依赖: `context`、`errors`、`fmt`、`os`、`github.com/spf13/cobra`

## 6. 变更影响面
- 修改 flag 或默认值会直接影响 CLI 公共接口和 README 示例。
- 运行时装配顺序如果变化，可能导致默认配置、输出格式或服务依赖初始化不完整。

## 7. 维护建议
- 新增 CLI 参数时，需同步检查默认配置、文档示例和 REPL/单次模式之间的一致性。
- 修改 `internal/cli/root.go` 后，同步更新 `doc/src/internal/cli/root.go.plan.md`。
