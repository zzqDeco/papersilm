# store.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/storage/store.go`
- 文档文件: `doc/src/internal/storage/store.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `storage`

## 2. 核心职责
- 提供基于文件系统的会话存储，持久化 session 元数据、sources、plan、execution state、digests、artifacts、skill-runs、skill-artifacts、workspaces、events 和 checkpoints。
- 负责生成完整 `SessionSnapshot`，并在来源变化或配置变化时失效旧计划与产物。

## 3. 输入与输出
- 输入来源: `SessionMeta`、计划/执行状态、digest/comparison/artifact 数据、workspace 状态，以及会话 ID。
- 输出结果: JSON 文件、events.jsonl、checkpoint 二进制文件和聚合后的 `SessionSnapshot`。

## 4. 关键实现细节
- 主要类型: `Store`、`candidate`、`fileCheckpointStore`。
- 关键函数/方法: `New`、`BaseDir`、`Ensure`、`SessionsDir`、`SessionDir`、`sessionPath`、`sourcesPath`、`planPath`、`SaveWorkspaceState`、`LoadWorkspaceStates`、`LoadWorkspaces` 等。
- `Ensure()` 和 `CreateSession()` 创建工作目录树，其中 skill 结果独立落在 `skill-runs/` 与 `skill-artifacts/`，不会和 plan artifacts 混用。
- `Snapshot()` 聚合读取 meta、sources、plan、execution、digests、comparison、artifact、skill runs 和 hydration 后的 workspaces，再进一步投影 `task_board`。
- `Snapshot()` 会只暴露当前 attached sources 可见的 paper-level skill runs，以及当前 comparison 仍存在时才可见的 comparison-level skill runs。
- `LoadWorkspaces()` 会把持久化的人工状态与当前 sources、digests、artifacts、paper-level skill runs 组合成公开 `PaperWorkspace`。
- `DeleteDigest()`、`DeleteArtifact()`、`DeletePaperDigestArtifacts()`、`DeleteComparisonArtifacts()` 为 task rerun 的级联失效提供按产物清理入口。
- `InvalidatePlanState()` 只删除旧计划、执行状态和 plan artifacts，不会删除 `workspaces/` 下的人工状态，也不会删除独立 skill 结果目录。
- `CheckPointStore()` 与 `fileCheckpointStore` 为 Eino ADK 提供文件化 checkpoint 存储。
- `legacyStepsToDAG()` 保留旧 plan step 结构到 DAG 的兼容转换。

## 5. 依赖关系
- 内部依赖: `pkg/protocol`
- 外部依赖: `context`、`encoding/json`、`errors`、`fmt`、`os`、`path/filepath`、`sort`、`time`、`github.com/cloudwego/eino/adk`

## 6. 变更影响面
- 目录布局或文件名变化会直接影响会话恢复、artifact 导出、task rerun 清理、skill 结果可见性和向后兼容。
- 失效逻辑如果不完整，旧 digest/artifact 可能污染新计划结果，或错误清掉人工 notes / annotations。

## 7. 维护建议
- 调整存储格式时，优先考虑旧会话目录的兼容读取，而不是只依赖全量重建。
- 修改 `internal/storage/store.go` 后，同步更新 `doc/src/internal/storage/store.go.plan.md`。
