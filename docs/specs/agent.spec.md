# Agent：主对话与 Case 子 Agent

## 目的

上一版没有任何位置放 Agent：系统假定内容由人写、由程序抽，然后排队等人核验。
可真正把一堆来源整理成可用 SSOT 的主体是 **Agent**，核验只是它会做的一个动作。

本规格定义两种 Agent 的能力与边界：

| | 主对话 Agent | Case 子 Agent |
|---|---|---|
| 作用域 | 项目 | Case |
| 职责 | 建立与维护项目级六部件 | 在给定上下文里按条件给答案 |
| 能写 | 项目级六部件（需人拍板） | 自己的 experience 与 CaseData |
| 会话 | 支持多个 | 支持 |

一句话概括两者的分工：**主对话搞大部分建设，Case 负责用。**

## 术语

- **会话（session）**：一次连续的工作对话。会话是**工作记录**，不是事实源。
- **主对话 Agent**：项目级 Agent。可读全部内容；它的产出落到项目级六部件。
- **Case 子 Agent**：Case 的代理人。它的上下文由 Case 定义（见 `case.spec.md`）。
- **拍板（sign-off）**：人对「某内容进入项目级事实源」的确认。见下文「人拍板的是哪一步」。
- **交叉校验**：就同一件事比对多个来源，找出矛盾或一致，并给出可追溯的结论。
- **提议（proposal）**：Agent 想改动项目级内容时给出的差异，尚未生效。

## 领域规则（Given-When-Then）

### 主对话 Agent 能做什么

- Given 一个项目 When 使用主对话 Then 可以建立与维护**全部六个部件**：
  建立 Case、写工具、导入与整理文档、维护词表、写 DataSchema、写项目基础数据
- Given 一批来源散落在各处 When 要求整理 Then 主对话把它们落地到具体部件里
  （文档 / 词表 / DataSchema / 基础数据），**不得只留在会话里**
- Given 需要判断来源是否可信 When 要求交叉校验 Then 主对话比对多个来源，
  给出「一致 / 矛盾 / 单源」的结论并附上每一条的来源
- Given 需要数据分析 When 要求分析 Then 主对话通过工具完成（含分析型查询），
  见 `tools.spec.md`
- Given 主对话完成一次整理 When 结束 Then 必须报告：动了哪些部件、每处改动凭什么、
  以及有多少 Case 产物因此标为「依据已变」

### 主对话 Agent 不能做什么

- Given 主对话想改动项目级内容 When 该改动会进入事实源 Then **必须取得人的拍板**，
  不得自行生效
- Given 主对话产生的是推断而非直引 When 记录 Then 必须标为**生成**并指出依据，
  不得把自己的推断伪装成来源
- Given 主对话与项目级已有内容矛盾 When 处理 Then 以**提议**形式摆出差异，
  不得直接覆盖
- Given 一个 Case 的 experience When 主对话读取 Then **可以读**（汇总上报需要），
  但**不直接改写**——Case 级内容的写入属于该 Case

### Case 子 Agent 能做什么、不能做什么

- Given 一个 Case When 运行 Then 只能读项目级六部件；只能写自己的 experience 与 CaseData
- Given Case 需要取数 When 运行工具 Then **可以执行**（读工具的声明、运行它），
  但**不能新增、修改或删除**工具
- Given Case 执行工具产生结果 When 处理 Then 结果留在 Case 作用域内或仅作为分析输入；
  **不得写入项目级**——要落项目级必须写进 experience 上报
- Given Case 发现项目级内容有误（词表指代错了、DataSchema 缺字段、基础数据不对、文档不该这么总结）
  When 记录 Then 写进自己的 experience，由主对话汇总后统一改
- Given Case 按给入条件给答案 When 条件不同 Then 答案不同；
  每个答案必须说明它用了哪些数据与哪些文档
- Given Case 给出答案 When 展示 Then 必须带上它依据的项目级内容的**版本标识**，
  使用户能判断「依据是否已经变了」

### 人拍板的是哪一步

上一版把人固定成流程里的一环（排队核验），这一版的规则是：
**人只在内容进入项目级事实源时拍板。**

- Given Agent 想在 Case 作用域内记录 When 记录 Then 不需要逐条拍板——
  experience 与 CaseData 是探索区，改错了代价小
- Given 内容要进入项目级六部件 When 生效 Then **必须有人拍板**，
  且必须记录拍板者与理由
- Given 人拍板 When 记录 Then 拍板针对的是**具体哪一版内容**；
  此后内容再变，需要重新拍板
- Given 一条已拍板的内容 When 被改动 Then 状态回到「未拍板」，
  不得沿用旧的拍板结论
- 理由：项目级是事实源，下游所有 Case 都依赖它。事实源没有人的背书，
  「社区内容可信」这件事就没有落点

### 会话

- Given 一个项目或 Case When 使用 Then 支持**多个会话**；会话可命名、可继续、可归档
- Given 会话 When 记录 Then 只追加：对话不可改写（改写对话等于伪造工作记录）
- Given 会话 When 引用 Then 它**不是事实源**：结论要生效必须落到部件或 experience 里
- Given 会话记录过长 When 裁剪 Then 可以裁过程，**不得裁掉**「谁改了什么、凭什么改」
- Given 会话中产生了可利用的结论 When 落地 Then 必须写明它出自哪个会话的哪一处，
  使结论可回溯

### Agent 不可用时

- Given Agent 不可用（无网络、无凭据、无额度） Then 项目仍**可读**；
  已有内容、文档、词表、定义、基础数据都不得因此不可访问
- Given Agent 不可用 When 界面展示 Then 明确说明哪些动作不可用及原因，
  不得让按钮沉默地失效
- Given Agent 的一次动作中途失败 When 处理 Then 不得留下半完成的改动；
  已产生的部分必须显式标出或回滚

## 验收标准

- [ ] 主对话可建立与维护全部六个部件，且每次改动可回答「凭什么」
- [ ] 主对话的整理结果落到具体部件，而不是只留在会话里
- [ ] 主对话改动项目级内容前必须取得人的拍板，未拍板不生效
- [ ] Agent 生成的推断标为「生成」并指出依据，不伪装成来源
- [ ] Agent 产出与既有内容矛盾时以提议形式给出差异，不直接覆盖
- [ ] Case 无法新增、修改或删除工具
- [ ] Case 可以执行工具，但结果不得写入项目级
- [ ] Case 无法写入项目级六部件的任何一处
- [ ] Case 上报的问题出现在自己的 experience 中，并能被主对话汇总
- [ ] Case 的答案在不同条件下不同，且说明用了哪些数据与哪些文档
- [ ] 答案带上所依据内容的版本标识
- [ ] 项目与 Case 都支持多个会话，会话可继续
- [ ] 会话记录不可改写
- [ ] 会话裁剪不丢失改动记录与结论
- [ ] 内容改动后旧拍板结论失效，需重新拍板
- [ ] Agent 不可用时项目仍可读，不可用的动作有明确说明

## 边界与异常

- 会话为空：允许；界面说明「还没有对话」，不显示成错误
- 一个项目没有任何会话：允许——项目可以完全只由人来建立
- Agent 给出互相矛盾的两次结论：两条都留，标出矛盾，不自动择一
- Agent 的结论无法回溯到会话位置或来源：标为不可回溯并降级展示，不丢弃
- Agent 尝试写项目级而无人在场：动作挂起为提议，不静默失败也不静默生效
- 一个 Case 的上报与项目级既有内容重复：合并提示，不产生重复条目
- 一个 Case 的上报与另一个 Case 的上报重复：主对话汇总时合并，
  但保留两个来源（两个 Case 各自遇到过这个问题，这本身是信息）
- 拍板者不是人（是 Agent）：拒绝
- 会话中的凭据或密钥：写入时即拒绝，不依赖事后清理
- Agent 动作超时：报告超时并列出已完成的部分，不假装成功

## 前后端契约

`AgentService`（作用域：项目与 Case）：

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `Sessions` | `case string` | `Session[]` | 空 case 表示主对话会话 |
| `OpenSession` | `case, id` | `Session` | `id` 为空则新建 |
| `Send` | `case, id, text` | `Turn` | 追加一轮；返回 Agent 的回复与它**动了什么** |
| `Archive` | `case, id` | — | 归档会话（保留，不可继续） |
| `Proposals` | — | `Proposal[]` | 待拍板的项目级改动 |
| `SignOff` | `proposalId, by, reason` | `Proposal` | `by` 必须是人 |
| `Reject` | `proposalId, by, reason` | `Proposal` | 条目保留，作为审计轨迹 |
| `Availability` | — | `AgentStatus` | Agent 是否可用；不可用时给出原因 |

`Turn`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `role` | string | 否 | `user` / `agent` |
| `text` | string | 否 | |
| `at` | string | 否 | RFC3339 |
| `touched` | `Touched[]` | 否 | 这一轮动了哪些部件、哪些条目；可为空数组 |
| `proposal` | `Proposal \| null` | 是 | 需要拍板时给出 |

`Proposal`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `id` | string | 否 | |
| `part` | string | 否 | `tools` / `documents` / `dict` / `schemas` / `basicdata` / `case` |
| `summary` | string | 否 | 一句话说清要改什么 |
| `diff` | string | 否 | 改动前后的差异，人要能读懂 |
| `basis` | `string[]` | 否 | 凭什么改：来源标识或既有内容标识 |
| `fromSession` | string | 否 | 出自哪个会话的哪一处，使结论可回溯 |
| `status` | string | 否 | `open` / `signed` / `rejected` |
| `signedBy` | `ActorView \| null` | 是 | null 表示未拍板 |
| `reason` | string | 否 | 拍板或驳回的理由 |

约定：

- `Send` 必须返回 `touched`：**Agent 每次回答都要能说明自己动了什么**，
  否则人无法判断它是真改了还是只是说了说
- `SignOff` 的 `by` 在服务层校验必须是人；Agent 身份一律拒绝
- 任何写操作返回受影响的范围，以便界面提示「有多少 Case 产物将标为依据已变」
- Agent 不可用时，除 `Send` 与 `Proposals` 外的接口全部正常可用

## 待确认

1. Case 与用户的对话是否也保留为会话记录（本规格按**保留**写），
   还是 Case 只有 experience、对话即用即弃
2. 拍板能否批量（一次确认一组同源改动）
3. 会话保留期限与自动归档策略
4. 主对话是否可以对 Case 的 experience 提出修改建议（当前：可读、不写）
