# STATUS.md —— 现在做到哪、下一步、卡在哪

> 这份文件的**唯一职责**是让「进度」不再靠翻对话。规范在 `docs/specs/`，方案在 `docs/plans/`，
> 决策在 `docs/adr/`，问题在 `docs/OPEN.md`。**每次收尾（见 ADR 0013）都更新这里。**

**上次整理：2026-09-18（P2 完成：**打包顺序**是 17.6pp 的元凶，修完基线复现、验收达标）**

## 整理台账

| 日期 | 范围 | 结论 | 关联 |
|---|---|---|---|
| 2026-09-17 | 建立治理体系；补录已发生的决策 | 决策/进度/问题/反例从「散在各文件边角」改为各自有家 | ADR 0001–0013 |
| 2026-09-18 | 派生层 P0 收尾 | 切块进索引（`chunk`/`extract_chunk`/`sync` 三表 + 读取口）；发现 spike 的 JS 切块脚本与 Go 实现差 5%，已对拍定位并记账 | `embedding-spike.md` §六.4、plan §3 |
| 2026-09-18 | P1 地基验证 | 纯 Go（免 cgo）加载 ONNX Runtime 跑通 bge-small-zh int8；依赖离线可装；新增探针 `scripts/check/onnx-probe/` | `embedding-spike.md` §六.5 |
| 2026-09-18 | P1 主体完成 | 自研 WordPiece 分词 + ONNX 推理 + `embedding` 表 + 暴力扫；对拍逐条一致；量出两个坑（落盘放大 2.29×、每查一次读 25MB → P2 必须缓存向量） | `embedding-spike.md` §六.6、`OPEN.md` #17–19 |
| 2026-09-18 | P2 第一轮 | 混合检索（内存 Searcher + 标题项融合）落地；**验收未达标**：Go 纯向量 48.3% 低于 JS 65.9%（嵌入已排除，嫌疑在块边界） | `embedding-spike.md` §六.7、`OPEN.md` #19–22 |
| 2026-09-18 | P2 第二轮（收口） | **找到并修掉元凶：切块的打包顺序**（长段的句子被摊平后与相邻段拼块）。改「段内先打包」后纯向量 **61.6%/80.1%**，head/mid/tail 与 spike 表逐字相同 → 基线复现、验收达标（实体名式 R@1 +3.0pp） | `embedding-spike.md` §六.8、`chunk.go`、`derived.spec.md` |

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
- 无（P2 已收口；P3 未开工）

**下一步**
1. **P3**：抽取（`vextract`）→ 需先解 `docs/OPEN.md` 的 3 个阻塞项（纠正块语法、抽取触发方式、`agent.spec` 工具清单）
2. P2 尾巴（不挡 P3）：实体名式查询只 +3.0pp（n=102），要更硬得靠 P4 图检索；真人问法的查询集（正文项才有意义，`OPEN.md` #22）
3. P4：图检索并入（local/global/hybrid/mix）
4. P5：增量与索引状态接到界面

## 卡在哪

- **3 个阻塞项挡着 P3**（抽取）：纠正块语法、抽取触发方式、`agent.spec` 工具清单更新 —— 见 `docs/OPEN.md`。
  **不挡 P0–P2**。
- **1 个阻塞项挡着 P2 验收（`OPEN.md` #20）**：Go 侧纯向量 48.3% vs JS 路径 65.9%，
  差 17.6pp 没定位——**在这条查清之前，P2 的验收线没有意义**（比的是不同的池）。
- **1 个阻塞项挡着 P1 定稿**（不是挡开发）：模型与 ONNX 运行时怎么分发（`OPEN.md` #15）——
  现在只在本机临时目录里。
- 待用户决策：ADR 0013（定期整理）状态还是「提议」。

## 最近一次验证（都是跑出来的）

```
go vet ./...                           clean
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
