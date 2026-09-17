# STATUS.md —— 现在做到哪、下一步、卡在哪

> 这份文件的**唯一职责**是让「进度」不再靠翻对话。规范在 `docs/specs/`，方案在 `docs/plans/`，
> 决策在 `docs/adr/`，问题在 `docs/OPEN.md`。**每次收尾（见 ADR 0013）都更新这里。**

**上次整理：2026-09-18（派生层 P1 完成：分词/推理/向量表/暴力扫 + 逐位对拍）**

## 整理台账

| 日期 | 范围 | 结论 | 关联 |
|---|---|---|---|
| 2026-09-17 | 建立治理体系；补录已发生的决策 | 决策/进度/问题/反例从「散在各文件边角」改为各自有家 | ADR 0001–0013 |
| 2026-09-18 | 派生层 P0 收尾 | 切块进索引（`chunk`/`extract_chunk`/`sync` 三表 + 读取口）；发现 spike 的 JS 切块脚本与 Go 实现差 5%，已对拍定位并记账 | `embedding-spike.md` §六.4、plan §3 |
| 2026-09-18 | P1 地基验证 | 纯 Go（免 cgo）加载 ONNX Runtime 跑通 bge-small-zh int8；依赖离线可装；新增探针 `scripts/check/onnx-probe/` | `embedding-spike.md` §六.5 |
| 2026-09-18 | P1 主体完成 | 自研 WordPiece 分词 + ONNX 推理 + `embedding` 表 + 暴力扫；对拍逐条一致；量出两个坑（落盘放大 2.29×、每查一次读 25MB → P2 必须缓存向量） | `embedding-spike.md` §六.6、`OPEN.md` #17–19 |

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

**进行中**
- **P1 主体已完成**（`internal/infrastructure/vembed` + `embedding` 表 + 暴力扫 + CLI `embed`/`vector`）：
  分词自研、token id 与向量都跟 transformers.js **逐条对拍通过**（最小余弦 1.0000000、最大分量差 5.96e-08）；
  demo 12,512 块全量嵌入 205 秒，索引 43.9 → 99.7 MB。**P1 只剩下「向量进内存缓存 + 是否 int8 量化」，
  那两条并到 P2 一起定**（`OPEN.md` #18/#19）。

**下一步（按 plan 的顺序）**
1. **P2**：FTS + 向量混合检索 ← **不花额度，却是分水岭**（纯向量在实体名式查询上 R@1 只有 21.6%）
   - 前置：向量**读进内存**（现在每查一次读 25MB → 222～241 ms；内存点积 2 ms）
   - 顺便定 int8 量化（落盘 2.29× → 1.15×），**先测召回再决定**
   - 验收线：≥ 纯向量基线 61.6%/79.8%，且实体名式查询 R@1 明显上升
2. P3：抽取（`vextract`）→ 需先解 `docs/OPEN.md` 的 3 个阻塞项
3. P4：图检索并入（local/global/hybrid/mix）
4. P5：增量与索引状态接到界面

## 卡在哪

- **3 个阻塞项挡着 P3**（抽取）：纠正块语法、抽取触发方式、`agent.spec` 工具清单更新 —— 见 `docs/OPEN.md`。
  **不挡 P0–P2**。
- **1 个新的阻塞项挡着 P1 定稿**（不是挡开发）：模型与 ONNX 运行时怎么分发（`OPEN.md` #15）——
  现在只在本机临时目录里。开发期用本机路径，先不动。
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
