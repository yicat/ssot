# 0001 分层铁律：api → application → domain ← infrastructure

状态：已定（2026-09 起）
关联：`AGENTS.md`、`internal/AGENTS.md`、`docs/plans/derived-layer.md` §2

## 背景

工具是 Go + Wails 桌面应用：既有给界面用的绑定层，也有给 agent/脚本用的能力层，还有对外部
系统（文件、SQLite、git、DSH 后端、ONNX）的适配。上一套方案在这件事上没定死，结果是
业务规则散落在各层、换一个入口就要重写一遍。

## 决策

依赖方向固定为 `api → application → domain ← infrastructure`，且：

- `domain/`：**只允许 Go 标准库**，禁止 import 本仓库其他层；
- `application/`：用例编排（先 A 后 B、失败怎么办）；
- `infrastructure/`：外部适配（存储、进程、模型）；
- `api/`：只做参数转换与转发，不写业务规则。

## 为什么

**落位判断只有一句**：这段逻辑换掉外部系统之后还需要吗？需要 → `domain`。
这条判断让「该放哪」不再靠讨论。

## 否掉的选项

- **扁平包结构（按功能分 `vault/`、`agent/` 各自带全套）**：小项目看着省事，但规则会被复制到
  多个入口（界面一套、CLI 一套、MCP 一套），而「agent 不能发布」这种规则复制三份必然走样。
- **在 `domain` 里直接用 `os` / `database/sql`**：省一层转换，代价是没法单测规则、也没法换存储。
  （`domain` 允许 `os`/`path` 这类标准库，但**不许**碰 SQL 与进程。）

## 后果

- 规则只有一份实现（例如「agent 改过的文档回落 draft」在 `domain/vault` 里），三个入口共用。
- 多写一层转换代码；新功能要先想落位。

## 反例（我们踩过的）

`internal/mcp` 与 `internal/api` 都**只能**调 `application`；一旦有人图省事在 MCP 里直接读文件，
「门在能力层」就破了（见 0003、0006）。
