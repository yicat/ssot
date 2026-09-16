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

### 3. 改动归属可追：**能力层写入即提交**

- 每次写入在 git commit trailer 里记 `Edited-By: agent:<名字>` 或 `Edited-By: human:<名字>`。
- 与 `vault.spec.md` §5 一致：版本与 diff 用 git，不自建修订存储。
- **谁提交：能力层**。写入（`doc.write` / `status.set`）成功后立刻 `git add <该文件>` + commit，
  提交信息带 trailer。落地细节（只提交那一个文件、`.data/` 永不提交、不是 git 仓库时怎么办）
  见 `vault.spec.md` §5。
- ⚠️ 这里曾与实现不一致：代码里一度写「我们不代跑 git、只返回建议的提交信息」，
  那样本节「怎么验证」里的「每次写入的 commit trailer 里有 `Edited-By`」永远验不了。
  **已按本节改回：能力层代跑。** 理由不是「顺便」，是留痕不能靠 agent 记得做。

### 4. 对外接口：MCP + CLI（同一套能力）

- **MCP server**：给 agent 后端用（DSH 支持 MCP；将来其他客户端也能用）。
- **CLI**：同一套能力的命令行封装，便于脚本、调试与人工操作。
- 两者走同一个能力层，**权限规则只有一份实现**。
- 接入的具体形状（传输、命名、配置放哪、怎么启动）**单独一份**：`dsh.spec.md`。

### 5. 工具清单

**第一批（已实现，MCP 与 CLI 同名同义）**：

| 工具 | 作用 | 写？ |
|---|---|---|
| `vault_list` | 列文档 / raw / 表 | 只读 |
| `doc_read` | 读一篇文档（正文、状态、front matter、链接） | 只读 |
| `doc_write` | 写文档正文（带 `actor`，**回落 draft**） | 写 |
| `vault_search` | 全文检索（标题 + 正文） | 只读 |
| `link_backlinks` | 某文档的反链 + 它链出去的问题链接 | 只读 |
| `link_resolve` | 解析 `[[...]]`（含块级锚点）到文档与段落 | 只读 |
| `table_infos` | 有哪些数据表、列与类型 | 只读 |
| `table_query` | 对索引跑只读 SQL（表与 front matter） | 只读 |

- **`status.set` 不在第一批**：MCP 侧 `actor` 恒为 `agent`（`dsh.spec.md` §4），
  这个工具在 MCP 上永远失败；暴露它只会白占模型的上下文。发布/归档走界面与 CLI。
- 工具名用 **snake_case**，不用点号——DSH 只接受 `[A-Za-z0-9_-]`，点号会被换成 `_` 加 hash
  （`dsh.spec.md` §3）。

**未做（写下来免得被当成漏了）**：

| 工具 | 为什么还没做 |
|---|---|
| `table_write` | 要 csv / json / yaml 写回**且保住列类型**（现在只有读与类型推断），不是小活 |
| `raw_refresh` | 抓取远超 MCP 默认 60s 调用超时，要单独设计（分片或异步任务） |
| `history_log` / `history_diff` | 本轮只做写侧 git 封装；读侧等做「diff 视图」时一起 |

### 6. 四个角色的落地名（DSH 场景）

skill 名必须 kebab-case（`dsh.spec.md` §5），所以：

| 角色 | skill 名 | 目录 |
|---|---|---|
| 整理 | `vault-organize` | `.dsh/skills/vault-organize/SKILL.md` |
| 查找 | `vault-find` | `.dsh/skills/vault-find/SKILL.md` |
| 核验 | `vault-verify` | `.dsh/skills/vault-verify/SKILL.md` |
| 优化重写 | `vault-rewrite` | `.dsh/skills/vault-rewrite/SKILL.md` |

主 Agent 不需要 skill：它由后端提供，负责调度；它的约束（不直接改文档、只调子 Agent 与工具）
写在 `AGENTS.md` 里。

## 未定

1. **编排协议的具体形态**：聊天壳 ↔ 后端之间是自己定一套（消息 + 工具调用 + 流式），
   还是直接照 MCP 的客户端形状做。这决定后端能不能"即插即用"。
2. **会话与上下文存哪**：我们存（可审计）还是后端自己管（更简单）。倾向前者存一份只读记录，
   便于"这条结论是哪次会话产生的"这种追溯。
3. 子 Agent 之间能不能互相调用（现在假设只能由主 Agent 调度）。
4. 聊天壳的界面形态（并入现有标题栏那套壳，还是独立面板）。

> 「四个子 Agent 的实现形态」原列在此处，**已定**（DSH 场景 = `.dsh/skills/`，名字与目录见 §6）；
> 「换成别的后端时角色定义怎么随 App 分发」仍没定。

## 怎么验证

代码落地前只有约定；落地后至少要能验：

- `actor=agent` 调 `status.set` **必须失败**，且失败信息说明"只有人能发布"（CLI 侧能验；
  MCP 侧根本不暴露这个工具，见 §5）
- agent 改过的 `published` 文档，`status` 变回 `draft`
- 每次写入的 commit trailer 里有 `Edited-By`（能力层代跑，见 §3）
- MCP 与 CLI 走同一套权限规则（同一条规则在两个入口行为一致）：
  `scripts/check/mcp-smoke.mjs` 跑的就是「同一批断言，换一个入口」
