# agent.spec.md —— Agent 分工与写权限

主题：**谁在操作这个 vault**——主 Agent、四个子 Agent、工具，以及它们各自的权限边界。

配套：`vault.spec.md`（vault 内部结构）、`workspace.spec.md`（项目发现）。

## 定位

App **不做 agent 框架**。它提供两样东西：

1. **能力层**（Go，UI 与 agent 共用）：vault 读写、双链索引、SQLite 查询、状态机、git 封装。
2. **聊天壳**：一个聊天入口 + **可替换的 agent 后端**（先接 DSH，将来接任意 OpenAI 兼容 API）。

编排（拆任务、调子 Agent、多轮对话）由**后端**负责；我们只定义角色与边界。

## 角色

| 角色 | 干什么 | 读写边界 |
|---|---|---|
| **主 Agent** | 跟你对话、拆任务、调度子 Agent | 只调子 Agent 与工具，**自己不直接改文档** |
| **整理** | `raw/` → `docs/`：把原文整理成文档 | 写 `docs/`，产出**必然是 `draft`**，并留下指回 `raw/` 的**块级双链** |
| **查找** | 全文 / 双链 / 表格里找东西 | **只读**，产出「哪个文档、哪一段」 |
| **核验** | 对结论取证、比对来源 | **只产出证据与建议，不改 `status`** |
| **优化重写** | 改写文档的表达 | 写新版本，`status` **回落到 `draft`** |
| **工具类** | 数据表 / json / yaml 读写与查询 | 通过能力层提供的命令，**不让 agent 手改文件** |

## 已确认的约定

### 1. 审批门在能力层，不在后端

- 能力层的每个写操作都要 **`actor`**：`human` 或 `agent:<名字>`。
- `actor=agent` 时：允许写 `docs/`（产生新版本、`status` 强制回到 `draft`）、允许写 `raw/`
  （抓取与刷新）；**不允许**把 `status` 改成 `published` / `archived`——只有人能改。
- 为什么写死这条：**agent 后端是可替换的**。门若放在后端，换一个后端就绕过去了；
  放在能力层，任何后端都绕不过。这是「agent 可以提出与取证，**批准必须是人**」的落地处。

### 2. agent 改动一律回落 `draft`

- 哪怕原文档是 `published`，agent 改过之后自动变 `draft`，等人复核再发布。
- 否则 agent 能绕过人，改掉已发布的内容。

### 3. 改动归属可追

- 每次写入在 git commit trailer 里记 `Edited-By: agent:<名字>` 或 `Edited-By: human:<名字>`。
- 与 `vault.spec.md` §5 一致：版本与 diff 用 git，不自建修订存储。

### 4. 对外接口：MCP + CLI（同一套能力）

- **MCP server**：给 agent 后端用（DSH 支持 MCP；将来其他客户端也能用）。
- **CLI**：同一套能力的命令行封装，便于脚本、调试与人工操作。
- 两者走同一个能力层，**权限规则只有一份实现**。

### 5. 工具清单（第一批）

| 工具 | 作用 | 写？ |
|---|---|---|
| `vault.list` | 列文档 / raw / 表 | 只读 |
| `doc.read` / `doc.write` | 读文档；写文档（带 `actor`，回落 draft） | 读 / 写 |
| `link.backlinks` | 某文档的反向链接 | 只读 |
| `link.resolve` | 解析 `[[...]]`（含块级锚点）到具体文档与段落 | 只读 |
| `status.set` | 改 `status`（**仅 `actor=human`**） | 写（受限） |
| `table.query` | 对 `tables/` 与 front matter 跑 SQL | 只读 |
| `table.write` | 写 csv / json / yaml 表 | 写 |
| `raw.refresh` | 按来源刷新 `raw/`（抓取） | 写 |
| `history.log` / `history.diff` | git 历史与 diff | 只读 |

## 未定

1. **编排协议的具体形态**：聊天壳 ↔ 后端之间是自己定一套（消息 + 工具调用 + 流式），
   还是直接照 MCP 的客户端形状做。这决定后端能不能"即插即用"。
2. **会话与上下文存哪**：我们存（可审计）还是后端自己管（更简单）。倾向前者存一份只读记录，
   便于"这条结论是哪次会话产生的"这种追溯。
3. **四个子 Agent 的实现形态**：DSH 场景下是 `.dsh/skills/<名字>/SKILL.md`；
   换成别的后端时，同一份角色定义怎么随 App 分发。
4. 子 Agent 之间能不能互相调用（现在假设只能由主 Agent 调度）。
5. 聊天壳的界面形态（并入现有标题栏那套壳，还是独立面板）。

## 怎么验证

代码落地前只有约定；落地后至少要能验：

- `actor=agent` 调 `status.set` **必须失败**，且失败信息说明"只有人能发布"
- agent 改过的 `published` 文档，`status` 变回 `draft`
- 每次写入的 commit trailer 里有 `Edited-By`
- MCP 与 CLI 走同一套权限规则（同一条规则在两个入口行为一致）
