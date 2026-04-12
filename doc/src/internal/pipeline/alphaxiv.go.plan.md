# alphaxiv.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/pipeline/alphaxiv.go`
- 文档文件: `doc/src/internal/pipeline/alphaxiv.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `pipeline`

## 2. 核心职责
- 封装 AlphaXiv markdown 获取、缓存、缺失标记和章节解析逻辑。
- 为 overview/full text 两条内容通道提供统一的读取接口，供检查与蒸馏阶段复用。

## 3. 输入与输出
- 输入来源: 论文 ID、会话 ID、目标 `ContentSource` 和 HTTP 响应体。
- 输出结果: AlphaXiv markdown 文本、缓存命中状态、标题/章节解析结果和缺失标记文件。

## 4. 关键实现细节
- 主要类型: `AlphaXivClient`、`alphaSection`。
- 关键函数/方法: `NewAlphaXivClient`、`FetchOverview`、`FetchFullText`、`fetchMarkdown`、`supportsAlphaXiv`、`LookupAlphaXivOverview`、`LookupAlphaXivFullText`、`lookupAlphaXivMarkdown` 等。
- `AlphaXivClient` 负责 HTTP 拉取 `.md` 文件并把 404 映射为 `ErrAlphaXivNotFound`。
- `lookupAlphaXivMarkdown()` 统一处理缓存读取、远端获取、missing 标记与内容写回。
- 后半部分的 markdown 解析工具会提取标题、分节和摘要化可用的语义标签。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `context`、`errors`、`fmt`、`io`、`net/http`、`os`、`path/filepath`、`regexp`、`strings`

## 6. 变更影响面
- 这里决定了 AlphaXiv-first 策略的缓存行为和回退判定。
- 解析规则变化会直接影响标题抽取、章节提示和后续摘要质量。

## 7. 维护建议
- 变更缓存文件命名或缺失标记约定时，需同时兼容既有会话缓存目录。
- 修改 `internal/pipeline/alphaxiv.go` 后，同步更新 `doc/src/internal/pipeline/alphaxiv.go.plan.md`。
