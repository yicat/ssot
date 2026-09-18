# STATUS.md —— 现在做到哪、下一步、卡在哪

> 这份文件的**唯一职责**是让「进度」不再靠翻对话。规范在 `docs/specs/`，方案在 `docs/plans/`，
> 决策在 `docs/adr/`，问题在 `docs/OPEN.md`。**每次收尾（见 ADR 0013）都更新这里。**

**上次整理：2026-09-18（P3 第三轮：抽取产物入库并真跑 3 篇；产出质量要调提示词）**

## 整理台账

| 日期 | 范围 | 结论 | 关联 |
|---|---|---|---|
| 2026-09-17 | 建立治理体系；补录已发生的决策 | 决策/进度/问题/反例从「散在各文件边角」改为各自有家 | ADR 0001–0013 |
| 2026-09-18 | 派生层 P0 收尾 | 切块进索引（`chunk`/`extract_chunk`/`sync` 三表 + 读取口）；发现 spike 的 JS 切块脚本与 Go 实现差 5%，已对拍定位并记账 | `embedding-spike.md` §六.4、plan §3 |
| 2026-09-18 | P1 地基验证 | 纯 Go（免 cgo）加载 ONNX Runtime 跑通 bge-small-zh int8；依赖离线可装；新增探针 `scripts/check/onnx-probe/` | `embedding-spike.md` §六.5 |
| 2026-09-18 | P1 主体完成 | 自研 WordPiece 分词 + ONNX 推理 + `embedding` 表 + 暴力扫；对拍逐条一致；量出两个坑（落盘放大 2.29×、每查一次读 25MB → P2 必须缓存向量） | `embedding-spike.md` §六.6、`OPEN.md` #17–19 |
| 2026-09-18 | P2 第一轮 | 混合检索（内存 Searcher + 标题项融合）落地；**验收未达标**：Go 纯向量 48.3% 低于 JS 65.9%（嵌入已排除，嫌疑在块边界） | `embedding-spike.md` §六.7、`OPEN.md` #19–22 |
| 2026-09-18 | P2 第二轮（收口） | **找到并修掉元凶：切块的打包顺序**（长段的句子被摊平后与相邻段拼块）。改「段内先打包」后纯向量 **61.6%/80.1%**，head/mid/tail 与 spike 表逐字相同 → 基线复现、验收达标（实体名式 R@1 +3.0pp） | `embedding-spike.md` §六.8、`chunk.go`、`derived.spec.md` |
| 2026-09-18 | P3 第一轮 | 抽取**内核**落地：提示词（LightRAG 结构 + doc/行号要求）+ 回包解析（能从噪声/围栏里挑 JSON）+ 块级溯源校验（越界丢、缺行号退回区间并记说明、悬空关系丢）+ 补抽轮；7 条测试**不花钱**跑通。接线（谁跑调用）撞上 Windows 命令行长度上限 → 计划走 ACP | `vextract/`、`extraction-spike.md`「命令行长度」、`OPEN.md` #23/#24 |
| 2026-09-18 | P3 第二轮 | **ACP 接线跑通并真跑**：`ssot vault extract`（后端 `deepseek-harness-acp`、0 权限提示）；一篇 8 块 → 44 实体/38 关系、**0 丢弃**、60 秒；量出**固定开销 ≈13k token/次调用**（2 块 13～15k、8 块 21.2k）→ 批越大越省 | `extraction-spike.md`「P3 生产实测」、`OPEN.md` #23/#25 |
| 2026-09-18 | P3 第三轮 | **入库**：`entity`/`relation` 两表（结构版本→3）+ `-store` 写入口；来源各占一行、归并只合条目、纠错优先、**不复制文档状态**（读时 join）；3 篇真跑 23 块 → **92 实体 / 65 关系 / 1 丢弃**，入库 92 行 / 89 个实体 / 65 行关系；产出里 `Other` 最多（26/92）且有表名被当实体 → 提示词要调 | `vaultindex/graph.go`、`extraction-spike.md`「P3 第三轮」 |

## 现在在哪

**已完成**
- 能力层（CLI + MCP）：vault 读写、双链、状态机、SQLite 查询、git 留痕（写入即 commit）
- 界面：三栏文档浏览器（文档 / 数据表 / Agent 三个模式）+ 配置页（含后端逐条检查）
- Agent 接入：ACP 后端（`ssot-agent` profile）+ 会话级 MCP 挂载 + 四个角色 skill
- 派生层**方案定稿**：`docs/specs/derived.spec.md` + `docs/plans/derived-layer.md`
- 实测：嵌入选型（B）、召回基线、抽取成本（关推理 + 收工具集）
- **P0 完成**：切块（`domain/vault/chunk.go`，512/2000 两套 + 行号区间）→
  索引三表（`chunk` / `extract_chunk` / `sync`，`Rebuild` 时按篇一个事务写入）
  + 读取口（`ChunkStat` / `ChunksOf` / `ExtractChunksOf` / `SyncOf` / `StaleDocs`）
  + `ssot vault index` 打印块统计；demo 上 12,512 / 7,147 块，全量重建 127～192 秒
- **P1 完成**：`internal/infrastructure/vembed`（自研 WordPiece 分词 + purego 跑 ORT + CLS 池化 + L2 归一）
  + `embedding` 表与暴力扫（`PendingChunks`/`PutEmbeddings`/`SearchVector`/`ScanVectors`）
  + CLI `vault embed` / `vault vector` + `SchemaVersion` 自动重建旧索引
  + 对拍参照 `scripts/check/embed-ref.mjs` → `vembed/testdata/parity.json`
- **P2 完成**：`vaultindex.Searcher`（内存 37.9 MB / 13,181 块 / 装载 647 ms，检索 **9 ms/次**）
  + 融合规则（`domain/vault/rank.go`，默认 β_title=0.05）+ CLI `ssot vault find`
  + 评测工具 `scripts/check/hybrid-bench`
  + **验收**：正文句查询 61.6%/80.1%（基线 61.6%/79.8%，不降）；实体名式查询 R@1 **34.3% → 37.3%**、R@5 70.6% → 73.5%
  + 顺带修掉切块打包顺序（`chunk.go`）——它让 R@1 少了 13.3pp（`OPEN.md` #20）

**进行中**
- **P3 第三轮：抽取→入库全线通了；下一步是质量与成本**
  - ✅ `entity` / `relation` 两表（结构版本 → 3，旧索引自动重建）+ `ssot vault extract -store`
  - ✅ **3 篇真跑入库**：23 块 → 92 实体 / 65 关系 / **1 丢弃**；库里 92 行来源 / 89 个实体 / 65 行关系 / 3 篇；
    **行号退回 0 条**、跨批去重生效
  - ⚠️ 产出质量要调：`Other` 最多（26/92），还有把表名（`增益减益.csv`）当实体的 → `OPEN.md` #24
- **P3 第四轮（进行中）：按坏例加固提示词，跑 20 篇看质量**
  - ✅ 提示词写进坏例：文件名/表名/路径、命令与用法示例里的词、章节序号与年份、空占位**都不算实体**；
    并要求「尽量用具体类型，Other 太多等于没分类」（测试钉住）
  - 🔎 成本探针撞墙：`--profile headless` **不是 ACP profile**（一次性任务，握手即关）→
    13k 固定开销的对比要换能挂 ACP 的 profile 或直接数工具定义（`OPEN.md` #25）
  - ❌ **20 篇跑批完成 33/37 批后被一个真 bug 打断**（模型连着吐两个 JSON 对象，
    解析器取「首 `{` 到末 `}`」→ 解析失败）；**已修**（改成扫所有括号配平的对象再合并，测试钉住）；
    34 分钟、约 264 块、约 18 篇
  - ✅ 落**确定性过滤**（`vextract/filter.go`）：文件名/路径/占位符/命令名/纯数字一律丢并记原因
    （`Other` 之前那 28%→5.6% 的「改善」是把坏例换进了具体类型，靠提示词拦不住）
  - ✅ **重抽完成（干净版）**：37/37 批 / 290 块 → 1,312 实体 / 1,318 关系 / 3 丢弃，38 分钟；
    入库 1,298 行 / 765 个 / 1,318 / 20 篇；**`Other` 4.0%**（旧提示词 28%）、
    **坏例形状实体 0 行**；3 条丢弃暴露「端点按批校验」的新问题（`OPEN.md` #26）
  - **中途读数**：
     1,358 实体来源行 / 776 个实体 / 1,349 关系行；Other 从 28% 降到 5.6%——
    ⚠️ **但复查发现坏例只是换了类型**（`docs/式神/<名字>.md` 被标成「物品」、
    `tables/*.csv` 也是「物品」、`ssot vault` 落 Other）→ **要在代码里加确定性过滤**，
    在那之前**库里的图不能当图用**
  - ⚠️ 成本：一次调用固定开销 ≈13k（`OPEN.md` #25）；全库外推**还不能算**，先查这 13k
- **P3 第二轮（已完成）：抽取走 ACP 真跑通**
  - ✅ `internal/infrastructure/vextract`：提示词（LightRAG 结构 + doc/行号要求 + 9 类词表）
    + 回包解析（从推理痕迹/围栏里挑 JSON）+ **块级溯源校验**（越界丢、缺行号退回区间并记说明、
    悬空关系丢）+ 补抽轮 + 7 条**不花钱**的测试
  - ✅ `vextract.ACPCompleter`（每批新会话、不给 MCP、权限一律拒绝）+ `ssot vault extract`
    （`-docs / -batch / -gleaning / -out`；`SSOT_EXTRACTION_PATCH` 指 overlay）
  - ✅ **真跑通了**：后端 `deepseek-harness-acp`、0 权限提示；一篇 8 块 → 44 实体 / 38 关系 /
    **0 丢弃**，60 秒；行号全部落在块区间内
  - ⚠️ **固定开销 ≈13k token/次调用**（2 块一次 13～15k、8 块一次 21.2k）——比 spike 的 headless
    高一倍多，原因未查；**批越大越省**（`OPEN.md` #25）
  - ❌ 未做：结果**不入库**（entity/relation 表与归并还没做）；20 篇质量看还没跑

**下一步**
1. **P3 第四**：调提示词（别把命令示例/文件名当实体；`Other` 太多）→ 跑 **20 篇**看质量，
   带上人工抽查（`OPEN.md` #24）
2. **P3 成本**：查那 13k 固定开销（试 `headless` profile / 关多余插件），再定批量与成本外推（`OPEN.md` #25）
3. **P3 尾巴**：触发方式等三个阻塞项（`OPEN.md` #1–3）拍板后，再把抽取接到界面/后台
4. P4：图检索并入（local/global/hybrid/mix）
5. P5：增量与索引状态接到界面

## 卡在哪

- **3 个阻塞项挡着 P3**（抽取）：纠正块语法、抽取触发方式、`agent.spec` 工具清单更新 —— 见 `docs/OPEN.md`。
  **不挡 P0–P2**。
- **1 个阻塞项挡着 P1 定稿**（不是挡开发）：模型与 ONNX 运行时怎么分发（`OPEN.md` #15）——
  现在只在本机临时目录里。
- 待用户决策：ADR 0013（定期整理）状态还是「提议」。

## 最近一次验证（都是跑出来的）

```
go vet ./...                           clean
go test ./...                          ok（vembed 对拍 3 条、vaultindex 7 条、domain 规则 2 条、vextract 7 条）
go test ./...                          ok（vembed：算法 1 条 + 对拍 2 条（分词 12 例、向量 12 条）；vaultindex：块 4 条 + 向量 3 条）
SSOT_EMBED_DIR=%TEMP%\embed-spike go test ./internal/infrastructure/vembed/
                                        对拍 12 条：最小余弦 1.0000000，最大分量差 5.96e-08
bin/ssot-cli.exe vault index -root projects/demo
                                        410 篇 / 13 表 / 12,512 嵌入块 / 7,147 抽取块
bin/ssot-cli.exe vault embed -root <副本>  12,512 块 205 秒（58.8 块/秒）；索引 43.9 → 99.7 MB
bin/ssot-cli.exe vault vector -root <副本> "暴击伤害怎么算"
                                        5 次 222～241 ms（含模型加载）；命中带行号区间与状态
前端 npm run build                      ✓ built（492ms）
wails3 task check                      exit 0
scripts/check/mcp-smoke.mjs            31/31
scripts/check/agent-ui-test.mjs        22/22
scripts/check/ui-chrome-test.mjs       8/8
scripts/check/agent-e2e.mjs（可选）     从界面起后端 → deepseek-harness-acp + 会话 + 9 个 MCP 工具
```

⚠️ **已知红**：`scripts/check/ui-test.mjs`（46 条）**依赖旧示例 vault 的内容**（`语法示例`/`罗生门`/第 47 行任务），
那个 vault 现已挪到 `.tmp/backup-demo`，所以它对当前 vault 是红的。**不是界面坏了**，
修法两条：加一个随仓库提交的 fixture vault 并让套件切过去，或把套件改成不依赖内容（见 `docs/OPEN.md`）。
