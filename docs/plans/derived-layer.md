# derived-layer.md —— 派生层技术方案（水面下的 KV / 图 / 向量 / 检索）

主题：**怎么实现** `docs/specs/derived.spec.md` 那一层。规范在 spec 里，这里只解决
「按什么顺序、动哪些包、每步怎么验」。

> 本文同时是**整体复核**：§7 逐条检查现有 spec 之间有没有矛盾、哪些是阻塞项。
> 复核结论：**方案本身没有推翻性冲突**，但有 **3 个阻塞项**（§7.2）必须先在 spec 里定下来才能动代码。

## 1. 已定的关键数字（都是实测，别再拍脑袋）

| 项 | 已定值 | 依据 |
|---|---|---|
| 嵌入块 | **512 token 子块** + `bge-small-zh-v1.5` int8（22.9MB、512 维） | 召回实验：B 与 C 在整体 R@1 上 p≈0.30 无差异，B 便宜 18 倍（`derived.spec.md` §九.1） |
| 抽取块 | LightRAG 的 **2000 token**（`DEFAULT_CHUNK_P_SIZE`、标题/段落语义合并、重叠 100） | 抽取要上下文，砍块会抽碎（同上） |
| 检索 | **FTS + 向量 + 图邻居**，五种模式照 LightRAG（默认 `mix`） | 纯向量在实体名式查询上 R@1 只有 **21.6%**，字面加权把它拉起来（补偿实验 A） |
| 抽取任务 | **关推理** + **受限工具集**（pwsh/fs 禁用） | 同任务 run1→run3：输入 −57%、输出 −51%、耗时 −66% |
| 调用方式 | **多块/多篇合进一次调用**；单次固定开销 ~5.2–7.9k token | 逐块抽 ≈ 1.9 亿输入 token，纯浪费（`extraction-spike.md`） |
| 向量检索 | SQLite BLOB + 进程内暴力扫（10k 条 ≈ 2ms） | 不上 ANN：它破坏「同一次查询顺序永远一样」 |

## 2. 模块落位（守分层铁律）

```
internal/
├─ domain/vault/            chunk、entity、relation、检索排序的**纯规则**（只 stdlib）
│    ├─ chunk.go            切块口径（标题→段落→句子）与行号区间换算
│    └─ retrieve.go         混合排序（FTS / 向量 / 图邻居的融合、确定性）
├─ application/derivedapp/  用例编排：重建、增量、抽取任务派发、检索入口
├─ infrastructure/
│    ├─ vaultindex/         （现有）SQLite：新增 chunk / embedding / entity / relation / sync 表
│    ├─ vembed/             进程内 ONNX 嵌入（bingen 绑定 + ort dll，见 embedding-spike）
│    └─ vextract/           抽取任务的落地：走 acp/headless 后端，产出结构化 JSON
└─ api/                     （现有）vault_search 改为走混合检索；新增「索引状态」查询
```

**为什么切块与排序放 `domain`**：换掉 SQLite / ONNX / 后端之后它们仍然需要（落位判断那条）。
**为什么抽取不放 domain**：它依赖外部模型，是外部适配。

## 3. 数据模型（`.data/index.db`，全派生、可重建、不进 git）

沿用 LightRAG 的四类存储抽象，实现都放一个 SQLite：

```sql
chunk(id, doc, ord, from_line, to_line, text, status, hash)        -- 嵌入单位（512）
extract_chunk(id, doc, ord, from_line, to_line, text)              -- 抽取单位（2000）
embedding(owner_kind, owner_id, dim, vec BLOB)                     -- chunk / entity / relation
entity(id, name, type, description, authority, status, doc, line)  
relation(id, src, dst, keywords, description, authority, status, doc, line)
sync(doc, hash, mtime, built_at, stale, reason)
```

- 唯一键：`entity` 用 (name, type) 归并；`relation` 用 (src, dst, keywords)。
  **归并只合并条目、不丢来源**：每条来源各占一行（`doc`+`line`），合成描述另存
  （`description_summary`，见 spec §九.2）；`authority: corrected` 优先且不被 `derived` 覆盖。
- `sync.stale` + `reason` 是「不静默」的落地：失败、半成品、嵌入缺失都写在这儿。
- `chunk.status` 只有两个值：`fresh` / `stale`——它是**派生条目的新鲜度**，
  不是文档的 `draft/published/archived`（那是文件层的事，别混）。
  刚重建出来一律 `fresh`；文件变了或嵌入失败，对应行改 `stale` 并把原因写进 `sync.reason`。
- `sync.hash` 是**正文（已剥掉 front matter）的 sha256**，`sync.mtime` 是文件修改时间：
  两者一起用来判「这篇要不要重算」——只看 mtime 会被 `git checkout` 骗，
  只看 hash 则每次都要读全文。
- 实现落位（P0 已完成）：三张表在 `infrastructure/vaultindex` 的 schema 里，
  `Rebuild` 时按「一篇一个事务」写入；读取口是 `ChunkStat/ChunksOf/ExtractChunksOf/SyncOf/StaleDocs`。
  实测 `projects/demo`（410 篇）：512 口径 12,512 块、2000 口径 7,147 块、全量重建 **127 秒**。

## 4. 流程

**建库/增量**（后台任务，不挡人）：

```
文件变 → sync 标 stale → 攒一批（默认 20 篇或 200 块）
      → 切块（2000 抽取块 / 512 嵌入子块）
      → 一次调用抽多块（受限工具集 + 关推理）→ JSON → 写 entity/relation
      → 嵌入子块与实体/关系 → 写 embedding
      → sync 标 fresh
```

**检索**（`vault_search` 的新实现，五种模式）：

```
查询 → 关键词抽取（低层实体 / 高层主题；关推理的小调用）
     → 三路并行：
         FTS（现有 LIKE→升级为带字段加权的匹配）
         向量（暴力扫 + 阈值）
         图（实体→邻居关系→相关块）
     → 融合排序（权重写死、稳定排序）→ 返回 {doc, 行号区间, 片段, status}
```

## 5. 实施顺序（每步都能单独验，不许一次做完再验）

| 阶段 | 做什么 | 怎么验 |
|---|---|---|
| **P0** | `chunk` 表 + 切块（512/2000 两套）+ `sync` | `go test`：切块行号与文件对得上；`chunk-sizing.mjs` 数字可复现 |
| **P1** | 嵌入（`vembed`）+ `embedding` 表 + 向量检索 | 与 transformers.js 逐位对拍（照 `embedding-spike.md`）；暴力扫耗时复现 |
| **P2** | **FTS + 向量的混合检索**（先不做图） | 用现有 302 条查询跑同一套指标，**必须 ≥ 纯向量基线 61.6%/79.8%**，且实体名式查询 R@1 明显上升 |
| **P3** | 抽取（`vextract`，一次多块 + 关推理 + 受限工具集）→ entity/relation | 小批 3 篇复现 run3 的 token/格式；再跑 20 篇看质量 |
| **P4** | 图检索并入（local/global/hybrid/mix）+ 关键词抽取 | **对照实验**：同 200 篇 / 302 条查询，比 P2 基线 |
| **P5** | 增量与状态（stale/进度/失败原因）接到界面 | 改一篇 → 只有它进队列；失败留原因可见 |

**P2 是分水岭**：它不花 LLM 钱，却已经把最要命的问题（实体名式查询 R@1 21.6%）解决了大半。
P3/P4 要花额度，所以放在 P2 之后。

## 6. 风险与对策

| 风险 | 对策 |
|---|---|
| 首次建库慢（抽取 ≈ 1 小时单线程 + 嵌入 79 秒） | 后台跑 + 进度可见；先只建嵌入与 FTS（P1/P2），抽取(P3)按需排队 |
| 抽取质量不稳定（小模型 / 长文档） | 产出全部带 `doc`+行号，可人工抽查；质量评估单独做（还没做） |
| 实体归并把不同东西并成一个 | 只有 (name,type) 完全一致才归并；描述并列保留、纠错优先 |
| 「关推理是否真生效」没证实 | 阶段 P3 里顺手验一次 wire 层；不生效就换模型或调 prompt |
| 542MB 的 bge-m3 诱惑 | 已实测收益不显著（p≈0.30），**不做默认**；留配置项 |

## 7. 整体复核

### 7.1 一致性检查（逐条对照，没有推翻性冲突）

| 检查 | 结论 |
|---|---|
| 分层铁律（domain 只 stdlib） | ✅ 切块/排序放 domain，ONNX 与后端放 infrastructure |
| 「事实只在文件里」 | ✅ 派生层可重建、不进 git；抽取产物只进 `.data/` |
| 「冲突并列不裁决」 | ⚠️ 已按用户拍板改为 **LightRAG 合成 + 人机纠错优先**（spec §九.2）；文件层的并列仍然成立 |
| 「发布必须是人」 | ✅ 不受影响：抽取不写 `docs/`，不碰 `status` |
| 「未核验可见」 | ✅ 派生条目带 `status`（但**界面上怎么显示**还是未定，见 7.2） |
| 块级溯源 | ✅ 比 LightRAG 更细（`doc` + 文件行号） |
| 确定性（同查询同顺序） | ✅ 权重写死 + 稳定排序；不上 ANN |
| 与 `document.spec`（界面口径） | ⚠️ 检索结果要显示「来源状态」与「命中行号」，这条要补进界面 spec |

### 7.2 阻塞项（动代码之前必须在 spec 里定下来）

1. **纠错的落盘语法**：`> [!纠正] …` 的 callout 关键字与 front matter 字段名（spec §十 待确认）。
   没定就没法让「人机纠错」在重建后活下来。
2. **抽取的触发方式**：写入即排队 / 空闲批量 / 显式重建（spec §六.1 未定）。
   它决定 P3 的任务模型与界面上的进度语义。
3. **`agent.spec.md` 的工具清单要更新**：检索升级为混合之后，`vault_search` 的语义与返回
   会变（多出命中行号与来源状态）；另外要定「后台抽取任务」算不算一种工具/怎么暴露。

### 7.3 非阻塞但要在 P 阶段里补的

- 界面上「索引状态」给多少信息（一行 / 可展开）——P5 定。
- 向量先上不上：本方案的顺序已经回答了它（**先 FTS+向量=P2，图=P4**）。
- 实体 identity 的细节（同名不同类型是否算两个）——P3 定，先按 (name,type)。

## 8. 不做

- 不做 ANN、不引外部向量库服务、不引 Python 运行时。
- 不做问答/RAG 生成链（检索结果给人或给 agent 用）。
- 不让 LLM 裁决冲突；不让未核验内容洗白。
- 不把抽取产物写进 `docs/`（它就是派生数据，重建即重算）。
