# dsh.spec.md —— 把能力层接到 DSH 上

主题：**怎么接**——MCP 服务端长什么样、配在哪、叫什么名、skill 放哪、怎么启动。
角色分工与权限边界在 `agent.spec.md`；本文只管「接入形状」。

> 本文里的 DSH 事实都是**在机器上查出来的**（随包文档 + 桌面版自己的启动代码），
> 不是照记忆写的。出处见「DSH 侧已核实的事实」。

## DSH 侧已核实的事实

| 事实 | 出处 |
|---|---|
| MCP 客户端是 `@deepseek-ai/dsh-mcp-client`，**只桥接 tools**；resources / prompts 不支持 | 该包的 `README.zh.md` |
| 配置层次：bundle 层 → profile 的用户 patch（`~/.dsh/profiles/<profile>/cordis.patch.yml`）→ `--patch` overlay（**可重复**） | `@deepseek-ai/dsh/lib/bin.js` 的 `--patch` 说明与 `cordis.yml` 头注释 |
| 传输两种：`stdio`（`command`/`args`/`env`/`cwd`）与 `streamable-http`（`url`/`headers`） | mcp-client README |
| 工具公开名 `mcp__<serverName>__<rawName>`；**raw 名只允许 `[A-Za-z0-9_-]`**，其它字符被换成 `_` 并追加 12 位 hash | mcp-client `lib/index.js` 的 `INVALID_NAME_CHARS` |
| `serverName` 必须 `[A-Za-z0-9_-]{1,32}`，同一作用域内唯一 | 同上 `SERVER_NAME_PATTERN` |
| 单次调用超时 `toolCallTimeoutMs` 默认 **60s**；断线重连默认开（500ms 起翻倍，上限 30s，连续 10 次放弃） | mcp-client README |
| 协议版本：SDK 1.30.0，支持到 `2025-11-25`，默认协商 `2025-03-26` | `@modelcontextprotocol/sdk` 的 `types.js` |
| 桌面 GUI 起 harness 是**写死一条 patch**（它自己的 `dsh-desktop.patch.yml`），**没有给用户加 patch 的位置** | `out/main/index.js` 的 `buildHarnessArguments` |
| 项目内 `.dsh/` **只被指令与 skill 用**，没有项目级 MCP 配置位 | `dsh-skill-filesystem` / `dsh-agent-instructions` |
| skill 发现根 `<项目根>/.dsh/skills`（项目根 = 最近含 `.git` 的祖先）；形式 `<name>/SKILL.md` 或平铺 `<name>.md`；**只发现一层**（不认嵌套 `**/SKILL.md`）；frontmatter 必填 `name`（**kebab-case**）与 `description` | `dsh-skill-filesystem` README / `lib` |

## 已确认的约定

### 1. **不写全局 profile**，用会话级 overlay

写进 `~/.dsh/profiles/web/cordis.patch.yml` 是**全局生效**的：之后每次开 DSH——包括写代码那次——
工具列表里都会多出八个 `mcp__ssot__*`，白吃 token，还有被误调的风险。**明确不采用。**

改用 overlay：条目放仓库内 `.dsh/mcp.patch.yml`，启动时显式带上：

```powershell
dsh web --patch <仓库>\.dsh\mcp.patch.yml      # 只有这次会话有这些工具
```

将来 App 内置聊天壳（`agent.spec.md` 未定 #5）也走同一条路：App 自己起后端时把 overlay 拼进参数，
所以现在这条路不是临时方案。

### 2. 传输用 **stdio**，不用 HTTP

`command` 指向 ssot 可执行文件，`args` 是 `["mcp", "--root", "<vault>"]`。
理由：stdio 不用占端口、不用管生命周期与鉴权，进程随 harness 起落；HTTP 那套要额外解决
「谁先起、端口给谁、进程活着吗」，现在没有这个需求。

### 3. `serverName` = `ssot`，工具名一律 **snake_case**

模型看到的是 `mcp__ssot__doc_read` 这种。**不许用点号**（`doc.read` 会被替换成 `doc_read_<hash>`，
难看且不稳）——所以 spec 里写工具名也用下划线。

### 4. MCP 侧 `actor` 恒为 `agent:<名字>`：**MCP 表达不出 human**

`--actor` 默认 `agent:dsh`，MCP 层不接受调用方传 actor。于是：

- 发布/归档只能走界面或 CLI（人的入口）——门仍然在能力层（`agent.spec.md` §1），MCP 绕不过去；
- `status_set` **不暴露**：一个永远失败的工具只会白占上下文（见 `agent.spec.md` §5）。

### 5. 四个角色 skill 用 kebab-case 名，中文放正文

`name` 必须 kebab-case，所以：`vault-organize`（整理）、`vault-find`（查找）、
`vault-verify`（核验）、`vault-rewrite`（优化重写）。`description` 与正文用中文。
它们随仓库提交、随项目根被发现——**在别的仓库里写代码时不会被加载**（项目级作用域天然隔离）。

### 6. 写 overlay 时踩过的三个坑（都记下来，别再踩）

1. **新增插件行必须包在 `insert:` 里**。只写 `- id: mcp-ssot` 是「改**已存在**的那一行」，
   加载器只警告 `patch: entry "mcp-ssot" not found` 然后什么都不做——看起来像配好了，其实没有。
2. **`!!js` 只能用在标量上**。`args: !!js ['mcp', …]` 是给整个序列打标签，
   js-yaml 报 `unknown tag !<tag:yaml.org,2002:js>`，而这个报错完全看不出真实原因。
   要动态值就逐项标：`- !!js process.env.SSOT_MCP_VAULT`。
3. **启动器自己的选项要排在 app 参数之前**：`--patch` / `--dump-config` 必须写在
   `--host` / `--port` 前面（后者不是启动器选项，`passThroughOptions` 一遇到就把后面全透传给 app）；
   而且 **dump 模式不收任何 app 参数**，多一个 `--host` 就报 `config dumps take no app arguments`。

## ACP 后端（App 内置聊天用）

聊天界面走 ACP（`agent.spec.md` §7）。这里只记**在本机探出来的事实**，形状与理由在那份 spec 里。

### 已核实的事实

| 事实 | 怎么知道的 |
|---|---|
| ACP 后端 = `dsh --profile acp`，说 ACP v1（`protocolVersion: 1`），agent 自称 `deepseek-harness-acp` | 真起了一次，`initialize` 的返回 |
| `session/new {cwd, mcpServers}` → `{sessionId, configOptions}`：**MCP 服务器按会话挂**，`command` 必须是绝对路径 | 同上，`session/new` 返回 + `dsh-acp` 的 `resolveMcpConfigs` 校验 |
| `configOptions` 里是模型目录（分组、`value` 是 `["provider","model"]` 这样的 JSON 元组） | 同上 |
| 会话**持久化且全机器共享**：`session/list` 能看到用户自己在别处写的会话；它支持按绝对 `cwd` 过滤 | 探针的输出里出现了 cwd = 仓库根的会话，那是用户自己的 |
| `DSH_HOME` 是 `%APPDATA%\dsh-desktop\harness`，**不是** `~/.dsh` | harness 启动时打在 stdout 上的那行 |
| ⚠️ 真后端的 **stdout 上混着 `[harness-node] …` 诊断行**，而且夹在协议消息之间 | 探针把这些行判成了「非 JSON」 |
| ⚠️ `mcpServers[]` 里的 **`args` 与 `env` 必须显式给**（空数组也要给）。省略 `env` 时 `session/new` **仍然成功**，但服务器**根本不会被拉起**——静默不挂载 | 实测对比：带 `env: []` → 子进程 0→1；不带 → 0→0。表现是「agent 手上一个 ssot 工具都没有」 |
| 挂载失败本身**不是静默的**：命令路径不存在时 `session/new` 直接失败（`mcp-client(x): initial connection or tool synchronization failed`） | 故意给错命令验过 |
| `acp` profile 本机原本不存在，已新建 `~/.dsh/profiles/acp/package.json`（bundles：`dsh-base` + `dsh-acp-app`）；**不用装东西**——`~/.dsh/profiles/node_modules/@deepseek-ai/` 下这些包本来就有 | 建完 `--dump-config` 无错误、`--help` 能起来、真连一次成功 |

### 由此定下的三条实现要求

1. **跳过非协议行**：客户端遇到 stdout 上不是 JSON 的行要**记下来继续读**，
   不能当协议错误（`internal/infrastructure/acp` 里就是这么做的，并有测试守这条）。
2. **列会话必须带 `cwd`**：不带筛选会把用户写代码的会话一起列进我们界面的会话列表里。
3. **`cwd` 必须是绝对路径**：后端会直接拒绝 `cwd must be an absolute path`。
   项目根平时是相对的（`projects/demo`），所以交给 ACP 之前必须先转绝对路径——
   相对路径还依赖 App 的当前工作目录，本来就不该当会话工作区（踩过）。

### 改配置要立刻生效（踩过的坑）

`api.AgentService` 里那个 `agentapp.Service` **不能只按 vault 缓存**：
后端命令、CLI 路径、actor 都是从设置里读的，只按 vault 缓存会让「配置页改了没用」——
表现为配置改对了、界面仍拿旧值去起进程，报的还是旧的错。
缓存键要包含 vault + 后端命令 + 参数 + CLI + actor 的指纹。

### 后端工具集必须收紧：**只给能力层的工具**

**为什么**：ACP profile 继承 `dsh-base`，而 `dsh-base` 带着一整套编码 agent 工具——
`tool-pwsh`、`tool-bash`、`tool-fs`、`tool-fs-search`、`tool-str-replace-editor`
（在本机 `--dump-config` 里逐个查过）。留着它们，agent 可以直接改 vault 文件、
直接 `git commit`、直接改 front matter 里的 `status`——**整条门就绕过去了**。
这直接违反 `agent.spec.md` §1「门在能力层，后端可替换也绕不过」，
所以我们自己的后端 profile 必须把它们关掉。

**约定**（我们的 profile = `dsh-base` + `dsh-acp-app` + 一份禁用 patch）：

**原则：读放开、写收口。** 只有「能写」的能力才必须走能力层——
砍掉读只会让 agent 变瞎，而它瞎了照样改不了文件，纯粹是白损失能力（第一版就是一刀切砍过头了）。

| 处理 | 行 id | 为什么 |
|---|---|---|
| **禁用** | `tool-pwsh`、`tool-bash` | 能跑任意命令 → 能改文件、能 commit |
| **禁用** | `tool-fs` | 它把 `read` 与 `write`/`edit` 绑在同一个插件里，没法只要读；整块关掉，**读用我们自己的 `file_read` 补** |
| **禁用** | `tool-str-replace-editor` | 同上（`view` 是读，但 `create`/`str_replace`/`insert` 是写） |
| **禁用** | `tool-fs-search`（`glob` / `grep`） | 只读，但**不受 vault 约束**，且会绕开我们的检索（见下） |
| 保留 | MCP（`mcp__ssot__*` 十二个工具） | **这就是能力层**，vault 的读/写/查都在里面 |
| 保留 | `tool-skill`、`dsh-skill-filesystem` | 四个角色靠它分发（`agent.spec.md` §6） |
| 保留 | `tool-subagent`、`tool-todo` | 主 Agent 的调度用，**不碰文件** |
| 保留 | `tool-web` | 只读外部；要落进 vault 仍得走 `doc_write`，绕不过门 |

**为什么连只读的 `tool-fs-search` 也关掉**（2026-09-20 定的，推翻了第一版「读放开」里的例外）：
它的只读是事实，但**只读不等于受约束**——

1. **没有 vault 边界**。插件 `@deepseek-ai/dsh-tool-fs-search` 的 `toWorkdirRelative` 只做一件事：
   在 workdir 内就转相对路径，**在 workdir 外的绝对路径原样放行**（`lib/index.js` 附近）；
   它的 `Config` 全是**输出与结果上限**（`rawOutputMaxBytes`、`searchMetaMaxBytes`、结果条数这类），
   **没有任何 root / workdir 限制项**。也就是说 agent 可以 `grep C:\Users\...` 翻整个磁盘。
2. **它会绕开我们的检索**。「哪篇文档说过 X」正是 `vault_search`（分块 + 向量 + 图）要回答的问题；
   留着 `grep`，agent 必然改用关键词匹配，于是**收录范围（`derived.spec.md` §4）
   白定**——范围之外的文件照样能被 grep 出来、被当成事实写进整理稿，而我们在派生层做的
   一切（范围、留痕、可核验）全被旁路。当时用户的判断很直接：**有 grep 还要 RAG 干什么。**

**代价**：agent 失去「按文件名找文件」的能力。补法是能力层给**受约束的只读检索**：
「这个地方有哪些文档」用 `vault_list`，「哪篇说过 X」用 `vault_search`。
代价是明确的：agent 找不到「vault 外的东西」——**而这正是我们要的**。

**能力层因此要提供只读的文件读取**：`file_read`（带行号分页、限定在 vault 内、拒绝绝对路径与
`..`、非 UTF-8 明确报错）。这样 agent 能读原文/JSON 导出/`project.yml`/表说明，
但**写**仍然只有 `doc_write` 那条路——「写入回落 draft + 留痕」「agent 不能发布」两条规则都还在。

**怎么验「工具真的到 agent 手上了」**：ACP 协议不暴露工具清单，harness 日志里也没有 MCP 客户端的
注册记录。所以**服务端自己留痕**：`ssot-cli mcp` 在 `initialize` 与 `tools/list` 时写一行到
stderr 与 `<用户配置目录>/ssot/mcp.log`。看到

```
initialize 客户端=dsh-mcp-client 0.0.1 协议=2025-11-25
tools/list 被调用，返回 12 个工具
```

就说明客户端连上了并取走了工具清单。⚠️ 这**证明到「工具被取走」为止**；
模型最终看到的那份清单只有真跑一轮才看得见（界面里的工具调用行会显示工具名——
出现 `pwsh`/`read`/`write` 就说明没堵住或工具集变了）。

**落地形态**：专属 profile `ssot-agent`（不是通用的 `acp`）——名字就说明它是给谁用的。
App 默认指向它（`appconfig.DefaultProfile = "ssot-agent"`，起后端时参数里带
`--profile ssot-agent`）；配置页的「后端检查」要能看出**这个 profile 到底堵没堵住**，
而不是只看目录在不在。

**能验到哪一步**：dump 出**后端真正用的那个 profile**，再按**条目**看 `disabled`。
两个细节都会把人带沟里，先写清楚：

```powershell
# profile 要显式给（App 用的是 ssot-agent；scripts\dsh\dsh-ssot.ps1 用的是 web，两者不同！）
# --dump-config 只能写在 app 名**之前**：写在 `acp` 之后会被 acp 当成未知选项拒掉
# （`web` 那侧恰好容忍，所以照抄脚本会以为通用）
# ⚠️ 必须用管道取输出：DSH Desktop.exe 是 GUI 子系统进程，`> 文件` 会得到 **0 字节**
$env:ELECTRON_RUN_AS_NODE="1"
& "$env:LOCALAPPDATA\Programs\DSH Desktop\DSH Desktop.exe" --expose-internals `
  "$env:LOCALAPPDATA\Programs\DSH Desktop\resources\harness-node-entry.mjs" `
  "$env:LOCALAPPDATA\Programs\DSH Desktop\resources\app\node_modules\@deepseek-ai\dsh\lib\bin.js" `
  --profile ssot-agent --dump-config 2>&1 | Out-String
```

2026-09-20 实测（本机，892 行 dump）：**被禁的只有这 5 个 + bundle 自带的 `skill-badge`**

```
tool-bash  tool-pwsh  tool-fs  tool-fs-search  tool-str-replace-editor   （+ skill-badge）
```

其余照常开着（**不碰文件**的那批）：`tool-todo`、`tool-skill`、`skill-filesystem`、
`tool-subagent*`、`tool-goal`、`tool-ralph`、`tool-workflow`、`plan-mode`、`compaction-basic`、
`command-compact`、`tool-result-pruner`、`tool-web`、`fs-observation-policy` 等。
它们不给写能力，所以与 0003 的门不冲突（要不要再收，见 `OPEN.md` #30）。

⚠️ **踩过的坑（两次，都是读数方法错，不是配置错）**：
1. **别用「上一个 `- id:` 配下一个 `disabled:`」这种正则**。dump 的每个条目里
   `disabled: true` 排在 `__dshPluginOwner:` 块**之后**，配对必然错位——我因此写出过一份
   「27 个插件全被禁」的假清单（把保留的 `tool-todo`/`tool-web` 也算成禁用了）。
   **要按条目边界切**（每个 `- id:` 到下一个 `- id:` 之间）再看块里有没有 `disabled:`。
2. **别读输出文件**。GUI 子系统进程不往文件句柄写，`> dump.yml` 得到的是 0 字节——
   我当时读到空文件，误判成「patch 静默失效」，还据此怀疑了半天配置文件。

**「模型手上真有什么工具」也能自动验了**（2026-09-20 起，不用再靠人看界面）：
`scripts/check/agent-probe` 加了 `-ask`——起后端 → 建会话 → **真发一句话**（花额度）→
把这一轮模型**实际调用的工具名**逐条打出来。工具名取自 ACP `tool_call` 的 `title`，
与界面显示的是同一个字符串（`internal/api/agent.go` 的 emitUpdate），两边不会各说各话。

实测（demo vault，问法「在库里找出提到「增益」的文档，只列路径、不要改文件」）：

```
  [工具] in_progress  mcp__ssot__vault_search
  [工具] in_progress  mcp__ssot__table_infos
  [工具] in_progress  mcp__ssot__vault_list
  [工具] in_progress  mcp__ssot__table_query      ← 后面还有 9 次
  …
这一轮结束：stop="end_turn"（23.0 秒）
  ✓ 没出现 DSH 自带的写/搜索工具：13 次调用里 13 次走的是我们自己的工具
```

**结论：`grep`/`glob` 确实没到模型手上**（这个问法有 grep 的话多半会去 grep）。
⚠️ 一条限制：**这一轮没调工具就什么都证明不了**，探针会把这种情况如实打出来
（「换个非用工具不可的问法再跑一次」）——别把「没出现 grep」当成「grep 没有了」。

顺带：这次真跑还暴露了两个与工具集无关、但更要紧的问题——
`table_query` 能直接查派生层的表、以及 agent 手上的 `vault_search` 还是关键词检索
（见 `OPEN.md` #31 / #32）。**验证工具的副产品就是这种**，所以它值得常跑。

## 不做

- 不做 `streamable-http`；不做 MCP resources / prompts（DSH 也不桥接）。
- 不把 MCP 条目写进全局 profile（正是为了不跟「写代码的 DSH」冲突）。
- 不做 App 内置聊天壳之外的第二套后端接入（`agent.spec.md` §7 的第一版范围）。
- 不做 `raw.refresh`（抓取）：远超 MCP 默认 60s 超时，要单独设计。

## 怎么验证

```powershell
# 1) overlay 真能组装进去（只打印、不启动，零副作用）
pwsh -File scripts\dsh\dsh-ssot.ps1 -DumpConfig      # 看有没有 "# == <仓库>\.dsh\mcp.patch.yml" 那一段

# 2) 服务端行为：自写 JSON-RPC 客户端驱动真二进制（协议、八个工具、读写、门、留痕）
node scripts/check/mcp-smoke.mjs

# 3) 同一条权限规则在 CLI 与 MCP 上一致（agent 不能发布、写入回落 draft）
go test ./internal/...
```

**已实测过的证据**（2026-09-17，本机）：

- `-DumpConfig` 输出里有 `# == C:\Users\ngnl5\workspace\ssot\.dsh\mcp.patch.yml` 那一段，
  内容是我们的 `mcp-ssot` 行，**无警告无错误**；`args` / `command` 里的 `!!js` 原样打印（未求值）。
- `mcp-smoke.mjs` 29/29，其中包含真二进制的 stdio 会话、git trailer、以及「stdout 里只有协议消息」。
- 四个 skill 被真实会话目录**当场发现**（写进 `.dsh/skills/` 后，会话的 skill 目录立刻列出
  `vault-find` / `vault-organize` / `vault-rewrite` / `vault-verify`）——发现链路不用猜。
- `internal/mcp` 的 Go 测试里也有一条走**真进程真 stdio** 的（内存管道证不了 CLI 接线对不对）。

`dsh` 本机没有独立 node，是 Electron 当 node 用（`ELECTRON_RUN_AS_NODE=1`）——
启动脚本里已封好，见 `scripts/dsh/README.md`。
