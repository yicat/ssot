# vault.spec.md —— 项目 vault：两层文档 + 数据表

主题：**一个项目（vault）内部长什么样**——目录、两层内容、状态、双链、表格、版本。

> 这是简化后的核心方案。旧方案（六部件 + 断言库 + 核验流程，984 行规格）已整体作废，
> 作废的真实原因是**太复杂**：治理层设计到极致，却算不出一个数、也跑不通一个案例。
> 那套实现留在分支 `legacy/mvp-v1` 备查，本文**不沿用**它的分层与部件。

## 定位

把散在会过期、会互相抄、无法验证的来源里的事实，变成**可追溯、可复用**的本地文档库。

- **以文档为中心**：`docs/` 里的 markdown 是主产物；数据表是辅助。
- **工具只提供机制，不替人裁决**：冲突并列、未核验可见。
- **不做**：问答 / RAG 生成链；wiki 阅读器；断言库；准入与变更集；迁移引擎；
  四级核验与抽检；元模型原语集；场景包分发。这些随旧方案作废。

## 目录

```
projects/<领域>/              ← 一个项目 = 一个 vault（沿用 workspace.spec.md 的发现规则）
├─ project.yml               ← 项目元信息
├─ raw/                      ← 原始层：抓来的原文与原始数据，原样留存
│   └─ 灰机wiki/茨木童子.md
├─ docs/                     ← 整理层：整理好的文档（人 + agent 都能改）
│   ├─ 式神/茨木童子.md
│   └─ 机制/伤害计算.md
├─ tables/                   ← 数据表：一表一文件
│   ├─ 技能倍率.csv
│   └─ README.md             ← 字段含义与用途
└─ .data/                    ← 派生索引（SQLite），可重建、不进 git
```

## 已确认的约定

### 1. 两层：原始层与整理层

- `raw/` 是**原样留存**：抓来什么样就什么样。front matter 至少记
  `source_url` / `revid` / `fetched_at` / `sha256`（抓取来源、远端版本、抓取时间、内容指纹）。
- `docs/` 是**整理层**：人和 agent 都可编辑，表达"我们认定的说法"。
- 两层的关系靠**块级双链**（见 3）：整理层的每条主张能指回原始层的具体段落。

### 2. 状态：照 wiki 的发布态

- front matter 里 `status: draft | published | archived`。
- **未核验 = `draft`，界面必须把它标出来**——「默认一切未核验，但必须可见」是这套东西的
  核心立场，不是可选项。
- 不记核验方法、不记抽检、不做四级核验：**状态就够了**（与单人/少人执行能力匹配）。
- **只有人能发布**（`draft → published`）；agent 改过的文档一律回落 `draft`。
  这条的落地方式（审批门在能力层、`actor` 区分人与 agent）见 `agent.spec.md`。

### 3. 双链（Obsidian 风格）

- 文档之间：`[[docs/式神/茨木童子]]`；带标题：`[[docs/机制/伤害计算#公式]]`。
- 指向原始层：`[[raw/灰机wiki/茨木童子#^段落id]]` —— **块级锚点就是主张级溯源**。
  （对照：WeKnora 只到 chunk、LightRAG 只到 chunk、MaxKB 只到段落，三家都做不到主张级。）
- 反向链接**由索引算**，不写进文件（文件里只存正链）。

### 4. 表格

- `tables/` 下一表一文件，格式按用途自选：`csv`（行列数据）、`json` / `yaml`（嵌套结构）。
- 每个表旁边有说明（字段含义、用途、什么时候不该用它），照 `scripts/README.md` 的做法。
- 查询走 **SQLite**：把 `tables/` 与各文档的 front matter 载进 `.data/index.db`，
  它是**派生索引**，删掉可重建、不进版本控制。

### 5. 版本与 diff：用 git

- vault 本身就是 git 仓库：**改动记录、diff、回滚全用 git**，应用只做薄封装
  （读 `git log` / `git diff`；回滚 = checkout 指定版本）。
- **不**自建修订存储（这是简化里砍掉的一层）。
- 改动要能看出**是人还是 agent**：靠 commit trailer（如 `Edited-By: agent`）区分——
  否则「agent 可以操作」就会和「批准必须是人」冲突。
- **vault 是独立 git 仓库，工具仓库不跟踪它**（已定）：`.gitignore` 里 `projects/` **整段忽略**。
  理由：外层若跟踪，git 会把 vault 记成 gitlink（嵌套仓库），两边版本互相搅。
  代价：`projects/demo` 这类示例 vault 也不进工具仓库——要看示例就本地留着（或另立仓库）。
- **谁提交：能力层代跑**（`agent.spec.md` §3）。写入成功后：

  1. 只 `git add` **改动的那个文件**（不用 `add -A`——不然会把用户手边未完成的改动一起卷进来，
     也会把 `.data/` 的派生文件带进去）；
  2. commit，信息两段：第一段说清做了什么，第二段是 trailer `Edited-By: agent:<名字>` / `human:<名字>`
     （git 只认**最后一段**里的 trailer，所以 trailer 必须独占最后一段）；
  3. **`.data/` 永不提交**：它是可重建的派生索引，vault 根要有 `.gitignore` 忽略它
     （`git init` 一个 vault 时就把这条写上）。
- **vault 不是 git 仓库时**：写入照样成功，但结果里**明确报告「本次未留痕」**并提示 `git init`——
  不静默跳过（静默最坏：人以为有历史，其实没有），也不因此拒绝写入（没仓库是常见起步状态）。
- `projects/demo` 按本节**应该是 git 仓库**（现在还不是）——示例 vault 也要能演示「有记录、有 diff」。

### 6. 冲突：并列，不裁决

- 两份文档或两个来源说法不同 → 用双链把两边连起来**并列呈现**，系统不选一个。
  （反面参照：LightRAG 把矛盾交给 LLM 合并成一段话，不可复现、不留痕。）

## 未定

1. `raw/` 的刷新策略：全量重抓，还是按 `revid` 增量。
2. `.data/index.db` 什么时候重建：启动时、手动、还是监听文件变更。
3. 界面形态（文档列表 + 双链面板 + 查询台）——等壳长出来再定。
4. **向量检索**：已验证可行（bge-small-zh-v1.5 int8 22.9MB + ONNX Runtime 15.7MB，
  进程内、无 cgo、0.8ms/条、与 transformers.js 逐位一致；实测数字与三个坑见
  `docs/notes/embedding-spike.md`），但**暂不引入**——
   双链 + 标签 + SQLite 全文检索已能覆盖召回；真要做就按端口接上。

## 怎么验证

能力层（CLI）已落地，可直接跑（示例 vault 在 `projects/demo`）：

```powershell
# 列文档与表：draft 标 !（未核验可见）
go run ./cmd/ssot vault -root projects/demo list

# 主张级溯源：块锚点解析到原文段落；行号是**文件行号**，能直接跳过去
go run ./cmd/ssot vault -root projects/demo resolve "[[raw/灰机wiki/茨木童子#^伤害系数]]"

# 反链 + 问题链接（「断链」与「指不清」分开报，不是一类）
go run ./cmd/ssot vault -root projects/demo backlinks docs/机制/伤害计算.md

# 硬规则一：agent 不能发布（exit 1，且文件不被改动）
go run ./cmd/ssot vault -root projects/demo status -actor agent:核验 docs/式神/茨木童子.md published

# 硬规则二：agent 写入后状态回落 draft
"新正文" | go run ./cmd/ssot vault write -root projects/demo -actor agent:优化 docs/式神/茨木童子.md

# 人发布：只改 status 那一行，tags / 注释 / 正文都原样
go run ./cmd/ssot vault status -actor human:我 docs/式神/茨木童子.md published
```

`go test ./internal/...` 覆盖同一批规则：域规则（双链、反链、状态与 actor）、
front matter 边界（无 front matter / 没结束 / BOM / status 写错）、审批门与回落。

**还没落地、所以还验不了**：`.data/index.db`（SQLite 派生索引）、界面里的 draft 标记、
`raw/` 四字段的**强制**校验（现在只是约定，没有校验）。
