# docs/adr/ —— 架构决策记录（ADR）

**这里放「做过哪些决定、为什么、否掉了什么」。** 规范（现在的样子）在 `docs/specs/`；
本文记录**决策本身与它的来路**——spec 只写「是什么」，ADR 写「为什么是它、不是别的」。

## 规则

1. **一条一文件**，`NNNN-短横线小写.md`，编号递增、不回收（作废的决策也不删，改状态）。
2. 每条必须包含：**背景 / 决策 / 为什么 / 否掉的选项（含理由）/ 后果 / 关联**。
   「否掉的选项」是 ADR 的**主要价值**——不然半年后有人会把已经否掉的方案再提一遍。
3. **决策变了不删旧条**：新开一条，并在旧条顶部写「被 NNNN 修订/取代」。
4. 状态只有三种：`已定` / `提议（待确认）` / `已废止`。
5. 有实测数据的决策，**把数字写进来**（或指到 `docs/notes/` 的那份实验记录）。

## 索引

| 编号 | 决策 | 状态 |
|---|---|---|
| [0001](0001-layering.md) | 分层铁律：`api → application → domain ← infrastructure`，domain 只 stdlib | 已定 |
| [0002](0002-git-as-version-layer.md) | git 当版本层，写入由能力层代跑提交 | 已定 |
| [0003](0003-gate-in-capability-layer.md) | 审批门在能力层：agent 不能发布 | 已定 |
| [0004](0004-conflicts-in-parallel.md) | 冲突并列、系统不裁决 | **被 0009 修订** |
| [0005](0005-dsh-session-scoped-overlay.md) | 接 DSH 用会话级 overlay，不写全局 profile | 已定 |
| [0006](0006-toolset-read-open-write-gated.md) | 后端工具集：读放开、写收口 | 已定 |
| [0007](0007-derived-layer-underwater.md) | 派生层沉到水下，用户只见文档与数据表 | 已定 |
| [0008](0008-embedding-plan-b.md) | 嵌入走 B（512 块 + bge-small-zh），C 被实测否掉 | 已定 |
| [0009](0009-llm-summary-plus-human-correction.md) | 多来源描述照 LightRAG 合成 + 人机纠错优先 | 已定（修订 0004） |
| [0010](0010-no-ann-brute-force.md) | 向量不上 ANN：暴力扫 + 确定性 | 已定 |
| [0011](0011-extraction-off-reasoning.md) | 抽取任务：关推理 + 受限工具集 + 批量调用 | 已定 |
| [0012](0012-no-rag-generation.md) | 不做问答/RAG 生成链 | 已定 |
| [0013](0013-periodic-consolidation.md) | 定期整理：触发式为主 + 台账 | **提议（待确认）** |

## 与别处的分工

| 放哪 | 放什么 |
|---|---|
| `docs/adr/` | 决策与它的来路（为什么、否掉了什么） |
| `docs/specs/` | 确认过的事实与规范（**现在的样子**，冲突时以它为准） |
| `docs/plans/` | 技术方案（怎么实现） |
| `docs/notes/` | 实验记录与对照笔记（数字、复现方式、局限） |
| `docs/STATUS.md` | 进度与整理台账 |
| `docs/OPEN.md` | 问题清单（阻塞 / 待定 / 观察） |
