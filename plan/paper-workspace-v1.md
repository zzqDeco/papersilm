# paper workspace v1 计划

## 目标与收敛范围

本计划的目标，是让 `papersilm` 的单篇论文处理结果从“一次性 digest”升级为“可持续积累的 paper workspace”。

一期范围只收敛四类对象：

- 私有 notes
- anchored annotations
- resources
- similar papers

这些能力都必须围绕当前会话和论文来源工作，不引入公开社区特性。

## 当前现状与问题

当前系统已经具备若干可复用基础：

- `internal/pipeline/service.go` 负责把原始来源标准化为 `PaperRef`。
- `pkg/protocol/types.go` 已定义 `PaperRef`、`Citation`、`PaperDigest`、`ComparisonDigest` 和 artifact 相关结构。
- `internal/storage/store.go` 已能把 digest、comparison、artifact 和 event 持久化到会话目录。
- `internal/cli/repl.go` 允许用户持续附加来源、规划任务、运行和导出结果。

但还缺少 workspace 语义：

- 用户无法把阅读过程中的私有结论和想法保存成一等公民对象。
- `Citation` 只用于结果引用，还不是可单独维护的批注锚点。
- 外部资源目前更多是散落在摘要或导出文本中，没有结构化资源层。
- 系统没有“相似论文”这种可积累、可回看的研究线索对象。

## 目标体验 / 工作流

目标工作流如下：

1. 用户附加一篇论文并完成 inspect / distill。
2. 会话中自动形成该论文的 workspace 视图，至少包含 digest、notes、annotations、resources、similar。
3. 用户可以基于 `Citation` 或用户指定的页码 / snippet / section label 新建私有笔记。
4. 系统可以把 AlphaXiv 页面、论文 GitHub、项目页、博客等外部链接整理为结构化 resources。
5. 系统可以根据已有 digest 或 compare 结果追加 similar paper 候选，但这些候选首先是“研究线索”，不是公共推荐流。
6. 用户在后续会话恢复时，可以继续在同一 workspace 中补充笔记和资源。

## 数据模型 / 协议 / 接口影响

本计划建议新增以下协议对象：

- `PaperWorkspace`
  以 `paper_id` 为核心，聚合 digest、notes、annotations、resources、similar。
- `AnchorRef`
  统一锚点表达，至少覆盖 `paper_id`、`page`、`snippet`、`section_label`、`figure_label`、`equation_label` 中的可用字段。
- `PaperNote`
  私有笔记对象，包含标题、正文、anchor、来源类型、创建时间和更新时间。
- `PaperResource`
  外部资源对象，至少包含 `kind`、`title`、`url`、`source_paper_id` 和可选摘要。
- `SimilarPaperRef`
  表示相似论文线索，至少包含目标论文 ID、推荐依据和当前状态。

存储层建议：

- 在会话目录下引入独立的 workspace 持久化文件，而不是把 notes 直接塞进 digest。
- 允许 digest 和 workspace 分开演进，避免摘要重跑时覆盖人工笔记。

CLI 层建议：

- 提供 workspace 读取入口，后续再增量补齐 note/resource 相关命令。

## 分阶段实施步骤

### 阶段 1：定义领域对象与存储布局

- 在协议层补齐 `PaperWorkspace`、`AnchorRef`、`PaperNote`、`PaperResource`、`SimilarPaperRef`。
- 在存储层为 workspace 建立稳定文件布局。
- 明确与现有 `PaperDigest`、`Citation`、`ArtifactManifest` 的边界。

### 阶段 2：把现有输出映射进 workspace

- 让单篇 digest 结果自动 materialize 到对应 `PaperWorkspace`。
- 把已有的引用信息转成可复用的锚点基础。
- 为外部资源预留结构化落点，而不是继续散落在自由文本中。

### 阶段 3：补齐最小 CLI 面

- 增加查看 workspace、列出 notes、列出 resources、查看 similar 的最小命令面。
- 为人工添加私有 note 和 annotation 预留稳定入口。

### 阶段 4：为后续 plans 提供承载层

- 为 `research-skills-v1.md` 提供 skills 产物落点。
- 为 `session-memory-compact-and-resume.md` 提供长期保留对象。
- 为 `mcp-resources-and-binary-artifacts.md` 提供资源清单挂接点。

## 验收标准

- 同一会话下，单篇论文可以拥有独立的 workspace 对象。
- 私有 notes 与人工 annotations 不会因 digest 重跑而丢失。
- resources 与 similar 以结构化对象存在，而不是只出现在自然语言摘要中。
- workspace 设计可以直接被未来 GUI 复用，不需要另起一套状态模型。
- 计划文档中明确排除了公共评论、点赞、热榜和 feed。

## 明确不做

- 不做公开评论区、用户互评、点赞、热榜和个性化 feed。
- 不做多人协作编辑或实时协同。
- 不把“similar”做成平台级推荐系统，一期只作为单篇研究线索。
- 不在本计划里引入 OCR、PDF viewer 或重型前端设计问题。
