# workers.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/agent/workers.go`
- 文档文件: `doc/src/internal/agent/workers.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `agent`

## 2. 核心职责
- 执行具体 DAG 节点，把 pipeline 产物转换成节点输出、摘要、对比结果和最终 artifact。
- 提供节点输出查找、反序列化、筛选和 artifact 持久化等执行期辅助能力。

## 3. 输入与输出
- 输入来源: 当前会话、目标文本、语言/风格、`ExecutionState` 中的上游节点输出，以及被执行的 `PlanNode`。
- 输出结果: 节点级 `NodeOutputRef`、写入存储层的 digest/comparison/artifact，以及节点错误。

## 4. 关键实现细节
- 主要类型: `nodeResult`。
- 关键函数/方法: `executeNode`、`executeMergeDigest`、`executeCompareRow`、`executeFinalSynthesis`、`executeLegacyDistill`、`executeLegacyCompare`、`findSource`、`firstOutputByKind` 等。
- `executeNode()` 是所有 node kind 的总分发入口。
- `executeMergeDigest()` 把 summary / experiment / math / web 输出合并成最终 `PaperDigest`。
- `executeFinalSynthesis()` 读取对比矩阵输出，生成 `ComparisonDigest` 并持久化。
- `persistArtifact()` 负责统一写 markdown、json 和 manifest。

## 5. 依赖关系
- 内部依赖: `internal/pipeline`、`internal/storage`、`pkg/protocol`
- 外部依赖: `context`、`encoding/json`、`fmt`、`os`、`path/filepath`、`sort`、`strings`、`time`

## 6. 变更影响面
- 节点 kind 与输出 kind 的对应关系一旦变化，会影响执行图下游依赖和最终 artifact 结构。
- 这里的持久化约定会直接影响 `/export`、会话快照和工具调用返回。

## 7. 维护建议
- 新增节点执行逻辑时，务必同步补齐输出 kind、artifact 命名和状态查询辅助函数。
- 修改 `internal/agent/workers.go` 后，同步更新 `doc/src/internal/agent/workers.go.plan.md`。
