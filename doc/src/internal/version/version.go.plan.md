# version.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/version/version.go`
- 文档文件: `doc/src/internal/version/version.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `version`

## 2. 核心职责
- 持有构建时注入的版本、提交和日期元数据，并提供统一的多行文本输出。
- 为 `papersilm version` 和发布构建追踪提供最小公共接口。

## 3. 输入与输出
- 输入来源: 链接阶段注入的 `Version`、`Commit`、`Date` 变量。
- 输出结果: `version=<...>
commit=<...>
date=<...>` 格式的文本。

## 4. 关键实现细节
- 关键函数/方法: `Lines`。
- `Lines()` 固定了对外展示格式，CLI 子命令直接复用该格式。

## 5. 依赖关系
- 内部依赖: 无直接内部包依赖。
- 外部依赖: `fmt`

## 6. 变更影响面
- 字段名或输出格式变化会影响构建追踪、发布说明和可能的脚本解析。

## 7. 维护建议
- 保持这里与发布脚本注入的 linker flag 字段名一致。
- 修改 `internal/version/version.go` 后，同步更新 `doc/src/internal/version/version.go.plan.md`。
