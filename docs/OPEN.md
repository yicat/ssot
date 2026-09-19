# OPEN.md —— 问题清单

**每个问题只有三种状态**：`阻塞`（挡着某一步，必须先定）、`待定`（不挡当前工作）、`观察`（记着）。

| # | 问题 | 状态 | 挡着谁 | 关联 |
|---|---|---|---|---|
| 1 | **纠正块的书写语法**：文档里怎么标记「这条被人和 agent 纠正过」（callout 关键字 + front matter 字段名） | **阻塞** | P3 抽取（人机纠错要能活过重建） | ADR 0009、`docs/specs/derived.spec.md` §十 |
| 2 | **抽取的触发方式**：写入即排队 / 空闲批量 / 显式「重建索引」 | **阻塞** | P3（决定任务模型与界面进度语义） | `derived.spec.md` §六.1、ADR 0013 |
| 3 | **`agent.spec.md` 工具清单要更新**：混合检索后 `vault_search` 的语义与返回变化（多出命中行号、来源状态）；后台抽取任务怎么暴露 | **阻塞** | P3/P4 | `agent.spec.md` §5 |
| 4 | `ui-test.mjs` 内容依赖：套件吃旧示例 vault 的内容，vault 一换就红 | 待定 | 无（但影响回归可信度） | `docs/STATUS.md`、`scripts/check/README.md` |
| 5 | 向量先上不上 | 待定（plan 已安排顺序：P2 先上，图在 P4） | — | `derived.spec.md` §六.3 |
| 6 | 界面上「索引状态」给多少信息（一行 / 可展开） | 待定 | P5 | `derived.spec.md` §六.6 |
| 7 | 抽取**质量**没测：512 上下文会不会把实体/关系抽碎 | 待定 | P3 的口径 | `docs/notes/extraction-spike.md` |
| 8 | 「关推理是否真生效」没证实：关掉后仍有 2 段 `dsh: reasoning` | 待定 | P3（要 wire 层才说得清） | ADR 0011 |
| 9 | 实体 identity 细节：同名不同类型算不算两个（现按 (name,type)） | 待定 | P3 | `docs/plans/derived-layer.md` §3 |
| 10 | 抓取管线口径（HTTP 先行→403 退 Chromium、钉 IP、readability）只在笔记里，`raw.refresh` 未做 | 待定 | 抓取相关需求 | `docs/notes/oss-comparison.md` §二 |
| 11 | WeKnora 那几条怎么落：`superseded_by`/有效期（拟长成图上的边）、改动来源四分类、lint 六类 | 待定 | — | `derived.spec.md` §六.5 |
| 12 | 仓库是 **public**，32 个文件含本机绝对路径（`C:\Users\ngnl5\…`）；build/darwin 的 5.2MB 仍在历史里 | 观察 | — | 用户已知，暂不处理 |
| 13 | `scripts/ingest/`（另一个 session 的导入脚本）未提交 | 观察 | — | 待其作者决定 |
| 14 | ADR 0013（定期整理）状态为「提议」 | 待定 | — | ADR 0013 |
| 15 | **模型与 ONNX 运行时怎么分发**：`onnxruntime.dll`（16.4MB）+ 模型（int8 22.9MB）现在只在本机临时目录里。选项：随包内置（仓库/安装包 +39MB，public 仓库不合适）／首次运行下载（要校验 + 失败提示）／只认用户指定路径 | **阻塞** | 挡住「别人也能用上向量」（开发不受影响：设 `SSOT_EMBED_DIR` 即可） | `embedding-spike.md` §六.5 |
| 16 | ~~分词器要自己写~~ → **已解决（P1）**：自己实现 BERT WordPiece，落在 `internal/infrastructure/vembed/tokenizer.go`，与 transformers.js 对拍 12 条 token id 逐条一致（见 `STATUS.md` 台账） | 已解决 | — | `embedding-spike.md` §六.6 |
| 17 | 嵌入吞吐 **59.3 块/秒**（单条喂、不开批），比 spike 外推的 6.0ms/条 慢 2.6×；全库 13,181 块约 222 秒。批推理/调线程能提上去，但当前够用 | 观察 | — | `embedding-spike.md` §六.6 |
| 18 | **向量落盘放大 2.29×**（2048B 的 BLOB 在 4096B 页里一行占一页；实测 4000 行表）。int8 量化能到 1.15× 且 4× 小，但**要先测召回掉不掉**（拿 302 条查询跑一遍），所以留到 P2 决定 | 待定 | P2（缓存/量化一起定） | `embedding-spike.md` §六.6、`derived.spec.md` §三 |
| 19 | **检索必须把向量缓存进内存**：实测端到端 222～241 ms（每查一次读 25MB），而内存点积只要 2 ms。P2 的混合检索要求 ≤ 几十毫秒，所以这是硬要求 | **已解决（P2）**：`vaultindex.Searcher` 一次装载（36.8 MB / 12,512 块 / 835 ms），检索 **8.7 ms/次** | — | `embedding-spike.md` §六.7 |
| 20 | ~~Go 侧纯向量 48.3%/71.5% vs JS 65.9%/77.8%，差 17.6pp 没定位~~ → **已解决（P2 第二）**：原因是切块的**打包顺序**（Go 把长段的句子摊平后与相邻段拼块）。改成「段内先打包」后纯向量 **61.6%/80.1%**，head/mid/tail 与 spike 表逐字相同 | 已解决 | — | `embedding-spike.md` §六.8 |
| 21 | 标题项权重：实测 **β=0.05** 最好（实体名式查询 R@1 34.3% → 37.3%、R@5 70.6% → 73.5%），β=0.2 反而掉到 26.5%——与 spike 的单调结论不一致。默认已定 0.05 | 待定（+3.0pp 只在 n=102 上量到，**算不上「明显」**；要更硬得靠 P4 图检索） | P2 定稿/P4 | `domain/vault/rank.go`、`embedding-spike.md` §六.8 |
| 22 | 正文项（β_body）在「原文整句」查询上是**评测假象**（98.3% 只是「句子在不在块里」）；真人问法下它变差。要用它必须先有一组**真人问法**的查询集 | 待定 | P2/P4 评测 | `embedding-spike.md` §六.7 |
| 23 | ~~抽取的模型调用走哪条路~~ → **已解决（P3 实测）**：走 ACP 已跑通（`ssot vault extract`，后端 `deepseek-harness-acp`，0 权限提示，提示词走协议、不受命令行长度限制）。为什么不能走 headless one-shot：任务文本是命令行参数，撞 Windows 32,767 字符上限，一次只放得下 5～6 块 | 已解决 | — | `extraction-spike.md`「命令行长度」 |
| 24 | 抽取的**提示词与实体类型词表没定稿**（spec §六.4）：现在有一版起点（LightRAG 结构 + 我们的 doc/行号要求 + 9 类词表），要用跑批产出与坏例来调。**已有证据（3 篇真跑）**：`Other` 最多（26/92 行），还有把表名（`增益减益.csv`）当实体、把命令示例（`ssot vault`）当关系端点的——提示词要明确「不要文件名/命令示例」**（已加）+ 代码里落地确定性过滤（`vextract/filter.go`）；20 篇干净版实测：坏例形状实体 0 行、`Other` 4.0%** | 待定 | P3 质量 | `internal/infrastructure/vextract/prompt.go`、`extraction-spike.md`「P3 第三轮」 |
| 25 | **抽取每次调用的固定开销 ≈13k token**（实测：2 块一次就用 13～15k，8 块一次 21.2k），比 spike 里 headless 的（工具 5,220 + system 1,174）高一倍多，原因未查。**先别拿 spike 的 3.2M 外推当承诺**；下一步：试**另一个能挂 ACP 的 profile**（`headless` 不行——它是一次性任务 profile，握手就关）或直接数配置里的工具定义 | 待定 | P3 成本 | `extraction-spike.md`「P3 生产实测」 |
| 26 | ~~关系的端点校验是按批做的~~ → **已解决**：端点校验改成「本批 + 图里已有」（`Options.Known`，由 `vaultindex.EntityNames` 提供）；测试钉住「图里已知的端点不该被丢」 | 已解决 | — | `vextract/extract.go`、`OPEN.md` 本条 |
| 27 | ~~同一份 vault 的索引没有互斥~~ → **已解决**：`.data/index.lock` 抢创建，抢不到给**人话**（「另一道进程正在重建索引…」），老锁（>10 分钟）当死锁清掉 | 已解决 | — | `vaultindex/lock.go` |
| 28 | **会话标题**：后端 `session/list` 不回 title → 现在用**第一句用户消息**（截 24 字）当初始描述，存本地（按 vault 分组）；**将来由 agent 生成**更好的一句（读第一轮对话 → 走 `renameSession` / 能力层 `set_session_title`），存法与现在一致，界面不区分谁写的 | 待定 | 会话体验 | `useAgent.ts` 的 titleKey/rememberTitle/renameSession |

| 29 | 关掉 `grep`/`glob` 之后（ADR 0014），agent「按文件名找」只能靠 `vault_list`——而它**没有参数**，一次把全部文档列出来：文档一多就把上下文顶满，也没法按模式筛。选项：给 `vault_list` 加 `prefix`（只读、限 vault 内）／给 `file_read` 加「按名找」／先这样（几百篇还塞得下） | 待定 | 关 grep 之后 | `internal/mcp/tools.go` 的 `vault_list`、ADR 0014 |
| 30 | **后端 profile 还开着 20 多个「会话机制」插件**：`tool-goal`/`tool-ralph`/`tool-workflow`/`plan-mode`/`compaction-basic`/`tool-result-pruner`/`tool-subagent*`…… 它们不给写能力（与 0003 的门不冲突），但**每个都往每次请求里塞工具定义**——与 #25 那 13k 固定开销同源。选项：按需再收一批（省 token）／保留（agent 复杂任务能自己调度子任务） | 待定 | P3 成本 | ADR 0014 的实测 dump、`OPEN.md` #25 |
| 31 | ~~`table_query` 能查派生层的表~~ → **已解决（2026-09-20）**：改成**白名单**——只放行数据表（`tables_meta` 登记的那些）+ `docs`（front matter 字段）。判定是纯规则 `vault.SQLTableRefs` / `TableQueryable`（fail closed：读不懂就拒），门落在 `vaultindex.Query`，**CLI / 界面 / agent 一视同仁**。四种绕过写法（子查询、CTE、表值函数、`sqlite_master`）都有测试钉住 | 已解决 | — | `domain/vault/querygate.go`、`vaultindex/index.go`、`derived.spec.md` §一 |
| 32 | **agent 手上的 `vault_search` 还是关键词检索**：MCP 的 `vault_search` 走 `vaultapp.Search` → `index.Search`，是 SQL `LIKE`；我们做的**向量 / 混合 / 图检索**（`vaultapp/vector.go`、`graph.go`、`Searcher`）目前只在 CLI（`vault vector`、`vault find`）与界面里用。**已定（2026-09-20）：先按 plan 做 P4 的 CLI 验收（同 200 篇 / 302 条查询比 P2 基线），达标后再把 MCP 切过去**——先接口会变成「换了引擎但没有数」 | 已定（待做） | P4 | `internal/mcp/tools.go` 的 `vault_search`、`vaultapp/vector.go`、`graph.go` |
| 33 | ~~`vault_search` 的 50 条上限把模型逼去翻表~~ → **已解决（2026-09-20），但根因不是我当初写的那个**：实测 `limit` **本来就生效**（CLI 同一条路：`-limit 10/50/500` → 10/50/**70** 条，70 就是全量）——模型那句「提到 500 还是那 50 条」是它自己说错了。真正的缺口是**结果里看不出「有没有被截断」**：返回既不给总数也不说截断，模型只能猜，猜不出来就换条路去翻派生层的表。所以补的是**可见性**：`vault_search` 现在返回 `total`/`returned`/`truncated` + 一句 `hint`（还有几篇、上限多少、怎么办），上限明确为 200 并写进工具说明；`limit` 传字符串也认（模型有时这么发）；CLI 打印「命中 N 篇，返回 M 条」 | 已解决 | — | `mcp/tools.go`、`vaultindex.CountMatches`、`cmd/ssot/main.go`、测试 `TestSearchReportsTotalAndTruncation` |

## 怎么用这份清单

- 新问题先加行，再决定状态；**阻塞项要在 `STATUS.md` 的「卡在哪」里再提一次**（免得埋掉）。
- 定了之后：把结论写进 spec / ADR，然后**删掉这一行**（或改成「已解决」并注明落到哪）。
- 巡检（ADR 0013）负责扫这张表：过期项、状态该变的项。