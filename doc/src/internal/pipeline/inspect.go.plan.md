# inspect.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/pipeline/inspect.go`
- 文档文件: `doc/src/internal/pipeline/inspect.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `pipeline`

## 2. 核心职责
- 检查来源可读性，优先尝试 AlphaXiv markdown，失败时再下载/读取 PDF 并提取页面文本。
- 生成 `SourceInspection` 元数据，包括页数、标题、章节提示、可比较性和简介片段，并把页面缓存到会话目录。

## 3. 输入与输出
- 输入来源: `PaperRef`、会话 ID、HTTP/PDF 内容和缓存目录。
- 输出结果: 更新后的 `PaperRef`、页面切片缓存 `[]Page`，以及失败原因。

## 4. 关键实现细节
- 主要类型: `Page`。
- 关键函数/方法: `InspectSource`、`inspectMarkdownContent`、`ensureLocalPDF`、`loadPages`、`cleanText`、`hasEnoughText`、`extractTitle`、`extractSectionHints` 等。
- `InspectSource()` 体现 AlphaXiv overview -> full text -> arXiv PDF fallback 的整体检查顺序。
- `ensureLocalPDF()` 负责把 arXiv PDF 下载到会话 cache。
- `loadPages()`、`cleanText()` 和多组启发式函数决定文本是否足够可提取。
- `writePagesCache()` 会把页面结果落盘，供后续蒸馏阶段复用。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `context`、`encoding/json`、`fmt`、`io`、`net/http`、`os`、`path/filepath`、`regexp`、`strings`、`github.com/cloudwego/eino-ext/components/document/loader/file`、`github.com/cloudwego/eino-ext/components/document/parser/pdf`、`github.com/cloudwego/eino/components/document`、`github.com/cloudwego/eino/schema`

## 6. 变更影响面
- 检查失败或阈值过严会直接让来源失去可蒸馏资格。
- 页面缓存格式若变化，会影响后续材料加载与会话重用。

## 7. 维护建议
- 修改文本提取阈值时，需同时评估 scanned PDF 与 AlphaXiv markdown 的误判率。
- 修改 `internal/pipeline/inspect.go` 后，同步更新 `doc/src/internal/pipeline/inspect.go.plan.md`。
