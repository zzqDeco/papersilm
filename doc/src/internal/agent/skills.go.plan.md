# skills.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/agent/skills.go`
- 文档文件: `doc/src/internal/agent/skills.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `agent`

## 2. 核心职责
- 定义 research-skills-v1 的内置 skill registry，并实现 skill 级目标解析、同步执行、artifact 落盘和 skill run 持久化。
- 把 `reviewer`、`equation-explain`、`related-work-map`、`compare-refinement` 四个内置能力变成会话内可重复执行的独立入口。

## 3. 输入与输出
- 输入来源: 当前 `SessionSnapshot`、skill 名称、可选 target、当前会话语言，以及 store/pipeline 提供的 source、digest、comparison 数据。
- 输出结果: `SkillRunResult`、独立 `skill-runs/*.json` 记录、`skill-artifacts/*` manifest/markdown/json 文件，以及写入 event log 的 skill 事件。

## 4. 关键实现细节
- 主要类型: `reviewerSkillArtifact`、`equationExplainSkillArtifact`、`relatedWorkMapSkillArtifact`、`compareRefinementSkillArtifact`。
- 关键函数/方法: `ListSkills`、`RunSkill`、`lookupSkillDescriptor`、`resolveSkillTarget`、`executeSkill`、`ensureDigestForSkill`、`persistSkillArtifact`。
- 内置 skill registry 会按当前 session language 生成 descriptor 标题、摘要、run title 和 artifact markdown 标题，不做远程注册、插件发现或动态下载。
- `RunSkill()` 先读取当前 snapshot，再解析目标、持久化 running run record，记录绑定的 `paper_ids`，随后同步执行 skill 并把结果写入独立 skill artifact 目录。
- paper 级 skill 在缺少持久化 digest 时，会临时从当前 source material 构造一个只用于技能分析的 digest，但不会回写到常规 `digests/` 目录。
- `compare-refinement` 只消费现有 comparison digest 与 paper digests；如果 session 里还没有 comparison，就直接报错。
- skill artifact 沿用 `ArtifactManifest`，但走 `skill-artifacts/` 独立目录，并把 `paper_ids` 复制进 manifest metadata，因此 `InvalidatePlanState()` 不会误删 skill 结果，旧 run 也能靠 metadata / JSON 懒兼容恢复比较论文集合。

## 5. 依赖关系
- 内部依赖: `internal/pipeline`、`internal/storage`、`pkg/protocol`
- 外部依赖: `context`、`encoding/json`、`fmt`、`os`、`path/filepath`、`sort`、`strings`、`time`

## 6. 变更影响面
- 这里直接定义了 v1 research skills 的产品边界；新增或收缩 skill 都会影响 CLI 命令、session snapshot、task board 和 skill artifact 目录结构。
- target 解析或 artifact 落盘规则变化，会直接影响 `/skill run` 的可用性和未来 GUI 对 skill runs 的消费方式。

## 7. 维护建议
- 新增 skill 时，先补 `SkillDescriptor`、task board kind、artifact markdown 模板和 snapshot 可见性规则，再补 CLI 帮助与测试。
- 修改 `internal/agent/skills.go` 后，同步更新 `doc/src/internal/agent/skills.go.plan.md`。
