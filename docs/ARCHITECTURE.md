# ARCHITECTURE.md —— 整体方案总览

**一句话**：一个以**文档为中心**的本地事实库工具——把会过期、会互相抄、无法验证的来源，
变成可追溯、可复用的文档与数据表；**agent 可以提出与取证，批准必须是人**。

- 规范（现在的样子）：`docs/specs/` ｜ 决策（为什么、否掉了什么）：`docs/adr/`
- 技术方案（怎么实现）：`docs/plans/` ｜ 实验记录：`docs/notes/`
- 进度：`docs/STATUS.md` ｜ 问题：`docs/OPEN.md`

## 一、水面上下（最重要的一张图）

```
用户看得见的（唯一事实源，进 git）
  docs/     整理层：人和 agent 都能改，表达「我们认定的说法」
  raw/      原始层：抓来什么样就什么样（source_url / revid / fetched_at / sha256）
  tables/   数据表：csv / json / yaml，一表一文件 + 说明
──────────────────────── 水面 ────────────────────────
派生的（.data/index.db，可重建、不进 git、不进人眼）
  FTS 索引 · 分块 · 向量 · 实体/关系图 · 同步状态
```

**推论**：删掉 `.data/` 只损失检索质量，不损失任何内容；任何「只在派生层里」的东西都不算事实。

## 二、三层结构（分层铁律：`api → application → domain ← infrastructure`）

```
              ┌─────────────── 界面（React/Wails）───────────────┐
入口          │ 文档三栏浏览 · 数据表 · Agent 面板 · 配置页        │
              └───────┬──────────────────────┬───────────────────┘
                      │ Wails bindings        │ ACP（聊天壳）
              ┌───────▼────────┐      ┌───────▼────────────────────┐
接口层        │ internal/api   │      │ internal/mcp（stdio MCP）   │
              └───────┬────────┘      └───────┬────────────────────┘
                      └────────┬───────────────┘
                      ┌────────▼─────────┐
用例层                │ application/     │  vaultapp（读写/检索/状态）
                      │                  │  agentapp（起后端/会话/流）
                      │                  │  derivedapp（重建/增量）*  * 未做
                      └────────┬─────────┘
                      ┌────────▼─────────┐
领域层                │ domain/vault     │  文档 · 双链 · 状态机（谁能发布）
（只 stdlib）         │                  │  切块 · 检索排序 · 确定性
                      └────────▲─────────┘
                      ┌────────┴─────────┐
适配层                │ infrastructure/  │  vaultfs · vaultindex(SQLite)
                      │                  │  vaultgit · projectfile · appconfig
                      │                  │  acp（客户端）· mcp · vembed* · vextract*
                      └──────────────────┘
```

「唯一入口」的意思：**规则只有一份实现**——CLI、MCP、界面都走 `application`，
所以「agent 不能发布」这类规则不可能在某个入口走样（ADR 0001/0003）。

## 三、数据怎么流

1. **人/agent 写**：`doc_write`（MCP）或界面编辑 → `vaultapp` → `vaultfs` 落盘 →
   `vaultgit` 提交（只提交那一个文件，带 `Edited-By` trailer）→ agent 写入强制回落 `draft`。
2. **检索**（现在）：SQLite `LIKE`（`vault_search`）。
   **目标**：FTS + 向量 + 图邻居的混合检索（顺序见 `docs/plans/derived-layer.md`）。
3. **派生层**（目标）：文件一变 → 标 `stale` → 后台按批重建（切块 → 抽取 → 嵌入）→ 标 `fresh`。

## 四、边界（明确不做）

- 不做问答 / RAG 生成链（ADR 0012）；不做断言库、准入与变更集（旧方案已作废）。
- 不做 ANN、不引外部向量库/图库、不引 Python 运行时（ADR 0010）。
- 不让 LLM 裁决冲突（0004 → 0009）；不让未核验内容以同等地位进库。
- 工具仓库不跟踪 vault 内容（`projects/` 整段忽略）。

## 五、可替换的东西（设计时就打算换掉、所以隔离了）

| 换掉什么 | 换的时候动哪 | 不动哪 |
|---|---|---|
| agent 后端（DSH → 别的 ACP / OpenAI 兼容） | `infrastructure/acp` + `agentapp` | 能力层、四个角色 skill、门 |
| 存储（SQLite → 别的） | `vaultindex` | `domain/vault` 与用例层 |
| 嵌入模型（bge-small-zh → 更大/多语言） | `vembed` + 配置 | 切块口径与检索排序（只换向量来源） |
| 检索实现（LIKE → 混合） | `vaultindex` + `domain/vault` 的排序 | 入口契约（工具名与返回形状保持） |
