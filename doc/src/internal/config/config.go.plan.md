# config.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/config/config.go`
- 文档文件: `doc/src/internal/config/config.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `config`

## 2. 核心职责
- 定义 `papersilm` 的持久化配置结构、provider 枚举、默认值、配置路径和 YAML 读写。
- 把 provider timeout 这种字符串配置解析成运行时 `time.Duration`。

## 3. 输入与输出
- 输入来源: 用户主目录、基础目录、磁盘上的 `config.yaml` 内容，以及传入的配置对象。
- 输出结果: 默认配置、实际加载的 `Config`、配置文件落盘，以及 provider 超时时间。

## 4. 关键实现细节
- 主要类型: `ProviderType`、`ProviderConfig`、`Config`。
- 关键函数/方法: `Default`、`userHomeDir`、`ConfigPath`、`Load`、`Save`、`ProviderTimeout`。
- `Default()` 给出 `~/.papersilm`、默认语言、风格、权限模式和 provider 默认值。
- `Load()` 在配置文件不存在时返回默认配置，而不是把缺失视为错误。
- `ProviderTimeout()` 会在解析失败或非法值时回退到 2 分钟。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `fmt`、`os`、`path/filepath`、`strings`、`time`、`gopkg.in/yaml.v3`

## 6. 变更影响面
- 字段变更会直接影响配置兼容性、启动默认行为和 provider 选择逻辑。
- 默认值变化需要与 README 和运行时装配保持同步。

## 7. 维护建议
- 新增配置项时，同时补齐 YAML/JSON tag、默认值和向后兼容处理。
- 修改 `internal/config/config.go` 后，同步更新 `doc/src/internal/config/config.go.plan.md`。
