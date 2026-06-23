# papersilm Technical Docs

本目录用于存放 `papersilm` 的技术文档。当前采用与 `starxo` 相同的两层组织方式：

- `doc/README.md`：项目级文档入口与维护约定
- `doc/workspace-tools.md`：Eino-native workspace tool 的用户可见契约、审批语义与 release smoke 清单
- `doc/src/<源文件相对路径>.plan.md`：与源码路径镜像的一对一文件级技术说明

## 当前覆盖范围

- 当前已覆盖 `cmd/`、`internal/`、`pkg/` 下全部非测试 Go 文件
- 当前专题文档覆盖 workspace read/search/edit/shell tool 的稳定行为契约
- `*_test.go` 暂不纳入本轮文件级技术文档
- 若新增、移动或删除非测试 Go 文件，应同步增删对应的 `doc/src/*.plan.md`

## 命名与结构约定

- 源文件 `internal/pipeline/service.go` 对应文档 `doc/src/internal/pipeline/service.go.plan.md`
- 文档采用统一 7 段结构：文件定位、核心职责、输入与输出、关键实现细节、依赖关系、变更影响面、维护建议
- 文件级文档描述当前实现，不在这里预埋未来设计或未落地接口

## 维护原则

- 修改源码后，同步更新对应文件级文档
- 对公共行为、目录约定或跨模块设计的变更，优先先更新根 `README.md` 与本目录入口文档
- 当前会话目录除 `digests/`、`artifacts/`、`checkpoints/` 外，还包含 `workspaces/`，用于保存人工维护的 paper workspace 状态
- `task_board` 是从 `plan + execution + workspaces + artifacts` 水合出来的公开视图，不单独持久化为额外文件
- 若未来引入项目级专题文档，可直接放在 `doc/` 根目录下
