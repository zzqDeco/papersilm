# graph_tool.go 技术说明

## 1. 文件定位
- 项目: `papersilm`
- 源文件: `internal/tools/graphtool/graph_tool.go`
- 文档文件: `doc/src/internal/tools/graphtool/graph_tool.go.plan.md`
- 文件类型: Go 源码
- 所属模块: `graphtool`

## 2. 核心职责
- 把可编译的 Eino graph/workflow 包装成可调用工具，并为中断/恢复场景保存 checkpoint 状态。
- 提供泛型实例化辅助函数，用于把 map/slice/pointer 输入安全地初始化出来。

## 3. 输入与输出
- 输入来源: 工具 JSON 字符串、graph compile options、checkpoint 数据和 tool runtime option。
- 输出结果: graph 执行后的 JSON 字符串，或带复合中断状态的 tool interrupt。

## 4. 关键实现细节
- 主要类型: `graphToolInterruptState`、`graphToolOptions`、`graphToolStore`。
- 关键函数/方法: `Info`、`init`、`InvokableRun`、`WithGraphToolOption`、`newEmptyStore`、`newResumeStore`、`Get`、`Set`。
- `NewInvokableGraphTool()` 负责从输入结构推导 `ToolInfo`。
- `InvokableRun()` 是核心入口，处理初次执行与恢复执行两条路径。
- `graphToolStore` 以最小实现承接 checkpoint store 读写。
- `NewInstance()` 根据泛型类型动态创建 map/slice/pointer 的零值实例。

## 5. 依赖关系
- 内部依赖: 无直接内部包依赖。
- 外部依赖: `context`、`fmt`、`reflect`、`github.com/bytedance/sonic`、`github.com/cloudwego/eino/components/tool`、`github.com/cloudwego/eino/components/tool/utils`、`github.com/cloudwego/eino/compose`、`github.com/cloudwego/eino/schema`

## 6. 变更影响面
- 该文件直接影响图工具在审批/中断/恢复流程里的可恢复性。
- 输入反序列化或 checkpoint 传递错误会让工具在中断恢复后失去状态。

## 7. 维护建议
- 扩展 graph tool 选项时，优先保持 `InvokableRun()` 的中断恢复分支行为稳定。
- 修改 `internal/tools/graphtool/graph_tool.go` 后，同步更新 `doc/src/internal/tools/graphtool/graph_tool.go.plan.md`。
