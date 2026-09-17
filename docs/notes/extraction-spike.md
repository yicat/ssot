# 抽取实测：3 篇文档跑一次（格式 / 消耗 / 耗时）

日期：2026-09-17。目的：在决定「整库抽取实体关系」之前，先量清**格式能不能稳定产出**、
**一次要花多少 token**、**多久**。结论：格式完全可用，但**消耗与耗时的外推要先解决两件事**
（批量、受限工具集），否则整库跑不划算。

## 怎么跑的

```powershell
# 一次性任务（dsh-headless），带上我们的 MCP overlay
$env:ELECTRON_RUN_AS_NODE=1; $env:DSH_HOME="$env:APPDATA\dsh-desktop\harness"
$env:SSOT_MCP_BIN=<仓库>\bin\ssot-cli.exe; $env:SSOT_MCP_VAULT=<vault>
& "…\DSH Desktop.exe" --expose-internals "…\harness-node-entry.mjs" `
  "…\node_modules\@deepseek-ai\dsh\lib\bin.js" --profile headless --patch .dsh\mcp.patch.yml "<任务文本>"
```

- `headless` profile 是**本轮新建**的（`dsh-base` + `dsh-headless`），建法与 `ssot-agent` 同：
  `~/.dsh/profiles/headless` 与 `%APPDATA%\dsh-desktop\harness\profiles\headless` 各一份。
- 任务：用 `mcp__ssot__file_read` 读 `raw/式神/{天邪鬼青,盗墓小鬼,寄生魂}.md`（各 3KB 上下），
  按 LightRAG 的字段输出一个 JSON（实体 name/type/description + 关系 source/target/keywords/description，
  每条带 `doc` + `line`）。
- ⚠️ **MCP 在 headless 里也走通了**：日志里有 `initialize 客户端=dsh-mcp-client` +
  `tools/list 被调用，返回 9 个工具`，说明 overlay 这条路与 ACP 那条是等价的。

## 结果

**格式**：完全可用。**12 个实体 / 9 条关系**，字段一个不缺，**12/12 都带合法文件行号**，
`doc` 是相对路径，description ≤40 字，keywords 是「技能/伤害」「CV/配音」这类高层词。
实体类型给的是「式神 / 技能」这种——**类型词表要我们自己定**（它默认只有 `Other` 兜底）。

**消耗与耗时**（从会话投影缓存 `storages/session_projcache/…json` 里的 `tokenUsage` 读的，不是估的）：

| 指标 | 数值 |
|---|---|
| 耗时 | **75 秒**（3 篇，一次任务） |
| 输入 · 未命中缓存 | 16,257 |
| 输入 · 命中缓存 | 38,400 |
| **输入合计** | **54,657** |
| **输出** | **13,327** |
| 单次调用的固定开销 | system 1,174 + **工具定义 7,863** + 消息 12,539 ≈ **21.5k** |

## 由此得到的三条结论（要做整库之前必须解决）

1. **输出里大头是推理**：3 篇文档产出 21 条实体/关系，却花了 13,327 输出 token
   ——CLI 打印的 `dsh: reasoning:` 说明推理也计进输出。想省钱就得**关/降推理**或换更快模型。
2. **⚠️ 绝不能「一块一次调用」**：单次调用的固定开销 ~21.5k（其中工具定义 7,863）。
   按块抽（8,667 块）≈ 8667 × 21.5k ≈ **1.9 亿输入 token**，纯浪费。
   **必须把多块/多篇合进一次调用**（比如一次 20 块 → 434 次调用 ≈ 9.3M 固定开销 + 正文）。
3. **工具集要收紧**：这次用的是 `dsh-base` 全套工具 + 我们的 9 个 MCP 工具，
   工具定义就 7,863 token/次。生产上应当用 `ssot-agent`（已禁用 pwsh/fs 那批），
   每次能省下几千 token——**这也顺便让抽取必须走能力层**。

## 外推（按本次实测，粗算，别当承诺）

- **按篇抽**：每篇 ≈ 18.2k 输入 / 4.4k 输出 → demo 的 410 篇 ≈ **7.5M 输入 / 1.8M 输出**，
  单机单线程耗时 ≈ **2.8 小时**（75 秒 × 410 / 3）。
- 并行跑多个任务能压时间，但**token 量不变**；想压 token 只能靠上面第 1、2 条。

## 关掉推理的 A/B（同一任务、3 篇、各跑一次）

旋钮：**DeepSeek 适配器只接受 `reasoningEffort ∈ {off, low, high, max}`，`off` 落成 wire 上的
`thinking: disabled`**（`dsh-llm-deepseek` 的 `reasoningEffort()` 与 resolve 处）。
落到 profile 上就是给 `agent-default-model` 那一行加 `reasoningEffort: off`——
仓库里有现成的 overlay：`.dsh/extraction.patch.yml`（与 `.dsh/mcp.patch.yml` 一起 `--patch`）。

| | run1（推理默认） | run2（`off`） |
|---|---|---|
| 输入 · 未命中 / 缓存命中 | 16,257 / 38,400 | 3,554 / 25,344 |
| **输入合计** | 54,657 | **28,898（−47%）** |
| **输出** | 13,327 | 11,975（−10%） |
| **耗时** | 75.0 s | **43.9 s（−41%）** |
| `dsh: reasoning` 段数 | 3 | 2 |
| 实体 / 关系 | 12 / 9 | 11 / 8 |
| 坏行号 / 描述超 60 字 | 0 / 0 | 0 / 0 |
| 实体类型 | 式神·技能·增益效果·妖怪·配音演员 | 式神·技能·妖怪·声优 |

**怎么读**：
- 省得最多的是**输入**（−47%）——因为推理文本会进对话历史，之后每次调用都要重发一遍；
  关掉它就等于掐掉了这份复利。时间也跟着降 41%。
- 输出只降 10%：run2 的**最后一次调用**输出了 11,806 token（run1 是 3,679）——
  推理从 reasoning 通道消失后，有一部分"想"落到了正文里。净效果仍是省。
- ⚠️ **推理没有清零**（还剩 2 段），所以「`thinking: disabled` 是否真的生效」**没被证实**：
  有可能适配器照发了但模型仍回 `reasoning_content`，也有可能是 CLI 把某种 content 块标成
  reasoning。要settle得看 wire 层的请求/响应（**没做**）。
- **质量对比只到「格式没坏」这一层**（11/8 vs 12/9 条、坏行号都是 0）：
  单次、3 篇、每臂一次，**不能据此说质量没退化**，那要正经评测。

## 什么时候不该用这份记录

- 单次运行、3 篇样本：**够回答「格式与量级」，不够回答「抽取质量好不好」**。
- 别拿它去推断「图能提升多少召回」——那要用同一批 200 篇 + 302 条查询做对照实验（还没做）。
- 模型与价格会变；这里只记 token，不记钱（价格是部署方的事）。
