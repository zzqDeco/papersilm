# MCP resources 与 binary artifacts 计划

## 目标与收敛范围

本计划的目标，是为 `papersilm` 补齐“结构化外部资源引用”和“大对象产物管理”能力，使系统不再只围绕文本摘要和小型 JSON 产物运转。

本计划聚焦：

- 外部 resources 的统一引用模型
- MCP resources 的接入边界
- binary artifacts / large blobs 的存储约定
- 与 workspace 的资源清单衔接

## 当前现状与问题

当前系统已经具备一些与资源和产物相关的基础：

- `internal/pipeline/service.go` 支持本地 PDF、arXiv、AlphaXiv 等论文来源标准化。
- `internal/storage/store.go` 已有 artifact manifest 与文件化会话目录。
- `internal/tools/registry.go` 已支持 artifact 导出与会话资产查询。

但现状仍偏向“以论文文本和摘要为中心”：

- GitHub 仓库、项目页、博客、数据集页面、视频讲解等外部资源没有一等公民对象。
- 大对象产物目前缺少更明确的 blob / manifest 语义，未来接入图像、补充材料、二进制文件时容易混乱。
- 如果后续引入 MCP 资源，当前协议还没有稳定的 handle 层。

## 目标体验 / 工作流

目标工作流如下：

1. 用户围绕一篇论文工作时，除了 digest，还能看到结构化 resources 列表。
2. 资源既可以来自已有抓取流程，也可以来自外部连接器或 MCP。
3. 大对象内容不直接塞进会话快照，而是以稳定 handle 和 manifest 引用。
4. workspace 可以把 resource card 与 binary artifact 统一纳入资源层，但区分“引用对象”和“实际内容”。

## 数据模型 / 协议 / 接口影响

建议新增：

- `ResourceRef`
  统一表示外部资源，至少包含 `kind`、`uri`、`title`、`source_paper_id`、`provenance`。
- `ResourceHandle`
  统一抽象本地文件、远端 URL、MCP 资源和已落盘 blob。
- `BlobManifest`
  描述大对象产物的路径、类型、大小、校验信息和来源。

存储建议：

- 会话快照只保留 resource / blob 的引用与摘要。
- 大对象内容单独落盘，避免把会话 JSON 膨胀成不可维护的大文件。

接口边界：

- 资源读取能力不应与 tool search 实现混在一起。
- MCP 接入层应先服务资源读取与引用，不要求一开始就接入所有外部系统。

## 分阶段实施步骤

### 阶段 1：定义资源与 blob 契约

- 补齐 `ResourceRef`、`ResourceHandle`、`BlobManifest` 等结构。
- 明确 artifact、resource、blob 三者的关系和边界。

### 阶段 2：升级存储布局

- 为 blob 引入稳定目录与 manifest 管理。
- 调整 artifact manifest，使其可引用大对象内容而不直接内嵌。

### 阶段 3：引入资源摄取层

- 让 GitHub、项目页、博客、数据集页面等先以结构化资源对象进入系统。
- 为 MCP 资源建立最小适配层，先解决“可引用、可读取、可追溯”。

### 阶段 4：接入 workspace

- 让 `paper-workspace-v1.md` 中的 resources 直接承载这些结构化资源对象。
- 让 future export / GUI 可以直接复用统一的 resource card 与 blob manifest。

## 验收标准

- 资源与大对象在协议和存储层都有明确对象，不再混在自由文本和普通 artifact 文件里。
- session snapshot 可以稳定引用外部资源和大对象，而不会因为嵌入内容过大而失控。
- MCP resource 接入路径明确，且与 tool search 保持边界清晰。
- workspace 能挂接结构化资源卡片，而不是继续依赖散乱链接。

## 明确不做

- 不做通用网页爬虫平台或全站缓存系统。
- 不在本计划里引入海量资源离线镜像。
- 不做与论文研究无关的通用文件同步服务。
- 不把 MCP 资源接入扩展为“大而全”的外部系统集成平台。
