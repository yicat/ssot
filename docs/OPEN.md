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
| 17 | 嵌入吞吐 **64.4 块/秒**（单条喂、不开批），比 spike 外推的 6.0ms/条 慢 2.6×；全库 12,512 块约 3.4 分钟。批推理/调线程能提上去，但当前够用 | 观察 | — | `embedding-spike.md` §六.6 |
| 18 | **向量落盘放大 2.29×**（2048B 的 BLOB 在 4096B 页里一行占一页；实测 4000 行表）。int8 量化能到 1.15× 且 4× 小，但**要先测召回掉不掉**（拿 302 条查询跑一遍），所以留到 P2 决定 | 待定 | P2（缓存/量化一起定） | `embedding-spike.md` §六.6、`derived.spec.md` §三 |
| 19 | **检索必须把向量缓存进内存**：实测端到端 222～241 ms（每查一次读 25MB），而内存点积只要 2 ms。P2 的混合检索要求 ≤ 几十毫秒，所以这是硬要求 | 待定 | P2 | `derived.spec.md` §三 |

## 怎么用这份清单

- 新问题先加行，再决定状态；**阻塞项要在 `STATUS.md` 的「卡在哪」里再提一次**（免得埋掉）。
- 定了之后：把结论写进 spec / ADR，然后**删掉这一行**（或改成「已解决」并注明落到哪）。
- 巡检（ADR 0013）负责扫这张表：过期项、状态该变的项。
