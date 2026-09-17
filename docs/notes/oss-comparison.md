# 三个开源项目的对照：WeKnora / MaxKB / LightRAG

日期：2026-09-17。结论一句话：**三家都不做「事实核验」；要贴只能分两条轴借。**

材料来源：三个仓库浅克隆到本地后逐项读代码（`--depth 1`），
WeKnora `3911` 文件 / MaxKB `2197` / LightRAG `1308`。
**证据可信度**：MaxKB 与 LightRAG 的关键结论我逐条抽查核对过原文；
WeKnora 的 schema 与 types 是我自己独立挖的，与分头读的结论一致。

## 一、定位对照

| 维度 | 我们 | WeKnora | MaxKB | LightRAG |
|---|---|---|---|---|
| 输入 | 社区 wiki / 攻略 / 实测（**数值与口径为主**） | PDF/Word/网页 → 问答 | 文档 → 智能体 | 文档 → 知识图谱 |
| 输出 | 文档 + 断言 + 未核验可见 | 带引用的自然语言回答 | 智能体回答/工作流 | 检索片段（喂给 LLM） |
| 真值模型 | 文档正文 + `status` 发布态 + 块级溯源 | chunk + 引用片段 | 段落 + 引用 | 实体/关系图 + 向量 |
| 冲突 | **并列 + 双链相连，系统不裁决** | 无（只有一个可选的 issue 标签） | 无 | **LLM 合并成一段话**（反面） |
| 人的角色 | **裁决者**（发布必须是人） | 提问者 | 配置者 | 调用方 |

**共同空缺**：三家都没有「事实级核验」——没有核验记录、没有方法分级、
没有「未核验可见」、没有责任归属。`unverified` 在 WeKnora 全仓只出现 1 次，还是写在提示词里。

## 二、WeKnora（Tencent，MIT，Go + Wails v2 + Vue3 + Python docreader）

**它是三家离我们最近的**，但近的是「内容治理」，不是「事实核验」。

能借（都要照抄改造，不能直接 import）：

| 借什么 | 证据 |
|---|---|
| 抓取管线：HTTP 先行 → 内容不足/403 才退 Chromium；403 不可重试、429/5xx 可重试 | `internal/infrastructure/web_fetch/fetcher.go:155-217`、`:405-416`、`:359-371` |
| 浏览器加固：`host-resolver-rules` 钉 IP（防 DNS rebinding）、`disable-blink-features=AutomationControlled`、完整浏览器头 | `fetcher.go:331-338`、`:387-402` |
| 正文链：readability 抽取 → goquery 清理 → html-to-markdown（**带 table 插件**）→ 前置标题 | `markdown.go:18-97` |
| **双粒度溯源**：文档级 `source_refs`＝`"<knowledge_id>\|<doc_title>"` + 分块级 `chunk_refs`＝chunk UUID | `internal/types/wiki_page.go:235-247` |
| **改动的四种来源**：`agent` / `user` / `revert` / `pipeline`，逐版本留痕 | `types/wiki_page.go:288-304`、`application/service/wiki_page.go:86,171` |
| 内容 lint 的种类（含 `stale_ref`）：孤儿页 / 死链 / 引用失效 / 缺交叉引用 / 空内容 / 重名 | `application/service/wiki_lint.go:13-22` |
| **取代而非删除 + 有效期**：`valid_from` / `invalid_at` / `superseded_by`；「只有模型能判同一性，否则停下来报告原因」 | `types/memory.go:288-318`、`memory/consolidate.go:256-296` |

不能借 / 要警惕：

- **它没有「待核验」这道门**：页面状态只有 `draft / published / archived`（`types/wiki_page.go:155-181`），
  而 **agent 能直接写** wiki 页面（agent 工具会把自己标成 `agent`，但不阻断发布）。
  我们的做法相反：**发布必须是人**。
- `stale_ref` **没有时间维度**，只等于「来源被软删」（`wiki_lint.go:192-221`），
  且只在按需调 lint 时检测、**没有定时任务**。「来源过期」这件事它只覆盖「来源没了」。
- 值冲突**没有机制**：`contradictory_facts` 全仓唯一命中是 agent 工具的一个**标签**
  （`internal/agent/tools/wiki_flag_issue.go:43`），没有自动比对、没有候选并列。
- 人工批准闸门确实有（`internal/agent/approval/gate.go`：fail-close、超时即拒、批准可改参数），
  但**批准的对象是工具调用，不是数据**。
- 无量纲/公式/派生：`metamodel` 0 命中，`unit`/`measure` 命中全是散文或嵌入维度。
- ⚠️ **它的桌面端主动禁用了 Wails 内建 CSS 变量拖拽检测**，理由是 `getComputedStyle`
  在动态 SPA 下有「时序/继承问题」（`cmd/desktop/main.go:41-66`、`:99-116`）。
  我们的自绘标题栏用的是同一套 `getComputedStyle` 量矩形（`@wailsio/runtime/dist/appregion.js`）
  ——**同类风险，我们的动态增删内容后需要复验**。

## 三、LightRAG（HKUDS，MIT，Python）

**概念上最接近的是它的「抽取」那一段，但语义差得远。**

- 抽取产物：实体 4 段（name/type/description）、关系 5 段（source/target/**keywords**/description）
  （`lightrag/prompt.py:83-84`）。**关系只有自由关键词，没有谓词**；**没有值、没有量纲**——
  「1.5 倍」「20% 暴击」只会落进一段描述文本，我们要的「谓词＋值＋单位」它给不了。
- 溯源是 chunk 级：`source_id`＝chunk id（`operate.py:757`），多来源用分隔符拼成一个字符串
  （`:2574`），且**会被上限截断**（`lightrag.py:1205,1212`）。
- ❌ **冲突处理与我们的立场相反**：同名实体合并后把描述列表交给 LLM 再总结
  （`operate.py:578-618` + `prompt.py:297-328`），冲突只在 prompt 里要求「reconcile 或并列标注」
  （`prompt.py:311-314`）——**没有冲突状态、没有并列候选，谁赢由 LLM 非确定决定，不可复现、不留痕**。
  直接入库正好踩我们最忌的那条，只能当**候选证据**。
- 工程上可用：核心依赖只有 `networkx` + `nano-vectordb` + `tiktoken`，有 `/openapi.json`
  → Go 侧可生成 client；**强制 LLM + embedding**（抽取与总结都要调模型）。
- 结论：**可作为 `infrastructure` 层的候选抽取器**，但只用于「找候选」，不碰核验。

## 四、MaxKB（1Panel，**GPL-3.0**，Django + Vue）

**两条轴都不贴**，但留下一条最该记的反面教训。

- 数据模型是纯文本切片：`Paragraph{content, title, position(段落序号), chunks}`
  （`apps/knowledge/models/knowledge.py:251-266`）；引用只到段落，**没有页码/偏移**，
  所以它的模型**先天做不了「回原文核对」**。
- ❌ **反面教训（最该记）**：LLM 的答案会**直接落成新段落并进检索库**
  （`apps/application/serializers/application_chat_record.py:368-375`）——
  无来源、无责任人，却与原始文档**同权**。这正是我们最忌的：未核验内容以同等地位进库。
- 许可：标准 GPLv3 全文、无附加例外、无 NOTICE（`LICENSE`、`README_CN.md:82-90`），`ui/` 同受约束。
  **借鉴思路/重写实现不受约束；复制代码（含改写）会传染。**

## 五、对我们的结论

1. **不采用任何一家的定位**（问答/RAG/智能体平台），也不做它们的检索-生成链。
2. **借机制、不借架构**。
3. **明确拒绝**：LLM 合并冲突（LightRAG）、未验证内容同权进库（MaxKB）。
4. 我们要的那条缝：**把来源整理成文档，冲突并列不裁决，发布必须是人，未核验可见**。

### 落位表（2026-09-17 补：下面这句原来只写了「要落进 spec」，实际没落，已核对）

⚠️ 教训：这条笔记一度写着「借机制…→ 落进 `vault.spec.md` 与 `agent.spec.md`」，
但**根本没落**——spec 里 grep 这些关键词全是 0 命中。写着「已落进」而没人核对，
等于给自己签了张空头支票。所以这里改成逐条落位：

| 借来的机制 | 落在哪 | 状态 |
|---|---|---|
| **LightRAG 的 KV / 向量 / 图 / 文档状态四层存储** | `derived.spec.md`（整份都是） | **本轮定方向**：照搬机制、沉到水下、自动维护；用户只看文档与表 |
| LightRAG 的**混合检索**（local/global/hybrid → 我们只借「混合」这一个概念） | `derived.spec.md` §二 | 方向已定，具体排序待做 |
| LightRAG 的「抽取→候选」与「谓词＋值＋单位」目标形态 | `derived.spec.md` §二（实体/关系字段） | 借字段；**不做**实体/关系图那套问答模式 |
| LightRAG 的 LLM 合并冲突 | 无（`vault.spec.md` §6 反面参照） | **明确拒绝** |
| WeKnora 的取代不删除 + 有效期（`superseded_by` / `valid_from`） | `derived.spec.md` §五.4（作为**图上的边**） | 待定：不新增用户可见概念，长在派生层里 |
| WeKnora 的改动来源四分类（agent/user/revert/pipeline） | 未落 | 只有 human/agent 两类（`agent.spec.md` §3） |
| WeKnora 的抓取管线（HTTP 先行→403 退 Chromium、钉 IP、readability+table） | 未落（`vault.spec.md` 未定 #1、`agent.spec.md` §5「未做」里有 `raw_refresh`） | 口径只在本文里，做抓取时以本文为准 |
| WeKnora 的双粒度溯源（文档级 + 块级 refs） | `vault.spec.md` §3（块级） | 只做了更紧的那半 |
| WeKnora 的内容 lint 六类 | `vault.spec.md` §3（断链/歧义两类） | 其余（孤儿页/引用失效/缺交叉引用/空内容）未做，也没写「未做」 |
| 向量检索（`embedding-spike.md` 的实测） | `vault.spec.md` 未定 #4 + `derived.spec.md` §五.2 | 已验证可行；先上不上未定 |

## 六、什么时候不该用这份笔记

- 三个项目都在活跃开发（对照的提交都在 2026-09）：**行号会过期**，重读时以代码为准。
- 我读的是文档、`go.mod`、`LICENSE`、schema 与关键实现点，**没有通读源码**；
  「能不能借」是基于依赖与技术栈的判断，不是通读后的结论。
