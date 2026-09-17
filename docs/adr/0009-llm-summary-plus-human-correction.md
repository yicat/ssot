# 0009 多来源描述：照 LightRAG 合成 + 人机纠错优先

状态：已定（**修订 [0004](0004-conflicts-in-parallel.md)**）
关联：`docs/specs/derived.spec.md` §九.2、`docs/specs/vault.spec.md` §6

## 背景

同一个实体在不同文档里有不同描述时，派生层存什么？
0004 定的是「并列不裁决」，而 LightRAG 的做法是 `_handle_entity_relation_summary` 用 LLM
**map-reduce 总结成一段**（描述数 ≥8 强制、`summary_max_tokens = 1200`）。
用户拍板：**照 LightRAG 的机制来，但纠错权留给人和 agent**。

## 决策

- **合成照 LightRAG**：`description_summary` 用它的 map-reduce 机制与阈值；
- **纠错是一等数据且优先级更高**：人和 agent 一起改过的描述标 `authority: corrected`，
  抽取来的标 `authority: derived`；合成提示词里明确 **`corrected` 不得被 `derived` 覆盖**，
  有分歧时保留 corrected 并把其他说法以「另有说法」并列附上；
- **纠正必须落在文件里**（文档正文的纠正块 + front matter 记一笔），派生层只是引用它。

## 为什么

- 用户要的是「能一起纠正」，而不是「系统把两边都摊在那儿不管」；
- LightRAG 自己的提示词里其实写着「冲突时**调和，或并列呈现并标注不确定性**」——
  **不是抹掉一边**，所以我们采纳它的机制并不违背 0004 的立场；
- ⚠️ **纠正不能只存在派生层**：派生层可重建，重建一次人的工作就没了
  （这是 0007 的直接推论）。

## 否掉的选项

- **纯并列、不合成**（原 0004 的派生层版本）：用户否掉——那样等于不服侍「一起纠正」这件事。
- **把纠正存在 `.data/` 里**：重建即丢，不可接受。
- **让 LLM 顺手把冲突一并裁决**：等于放弃 0004 的立场。

## 后果

- **文件层的并列立场不变**：文档里的两种说法一个字都不会被改掉（0004 仍然成立）。
- 需要一个「纠正块」的书写语法（callout 关键字 + front matter 字段）——
  ⚠️ **还没定**，见 `docs/OPEN.md`（阻塞 P3）。
