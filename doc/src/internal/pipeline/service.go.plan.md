# service.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/pipeline/service.go`
- 文档文件: `doc/src/internal/pipeline/service.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `pipeline`

## 2. 核心职责
- 定义 pipeline 服务对象，持有配置、HTTP 客户端、arXiv 基址和 AlphaXiv 客户端。
- 负责把原始来源字符串标准化为带类型和偏好内容来源的 `PaperRef`。

## 3. 输入与输出
- 输入来源: 配置对象、原始来源字符串列表、会话 ID 和 URL/路径文本。
- 输出结果: `Service` 实例、规范化后的 `PaperRef` 列表，以及 arXiv/AlphaXiv 相关辅助判断结果。

## 4. 关键实现细节
- 主要类型: `Service`。
- 关键函数/方法: `New`、`NormalizeSources`、`normalizeSource`、`buildPaperID`、`defaultLabel`、`isPaperID`、`isArxivAbs`、`isArxivPDF` 等。
- `normalizeSource()` 支持本地 PDF、原始 paper ID、arXiv abs/pdf 和 AlphaXiv overview/abs。
- `buildPaperID()` 根据 session 片段和来源字符串构造稳定但可读的 paper ID。
- `canonicalArxivPDF()` 统一把 arXiv-compatible 来源映射到 PDF 下载地址。

## 5. 依赖关系
- 内部依赖: `internal/config`、`pkg/protocol`
- 外部依赖: `context`、`fmt`、`net/http`、`os`、`path/filepath`、`regexp`、`sort`、`strings`

## 6. 变更影响面
- 来源归一化规则变化会直接影响缓存命名、paper ID 稳定性和后续 AlphaXiv/PDF 回退路径。
- 这里也是新来源类型接入的第一入口。

## 7. 维护建议
- 新增来源类型时，先补协议枚举，再同步更新识别正则、默认标签和内容来源偏好。
- 修改 `internal/pipeline/service.go` 后，同步更新 `doc/src/internal/pipeline/service.go.plan.md`。
