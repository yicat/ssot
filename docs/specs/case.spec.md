# Case：项目下的一个用途

## 目的

上一版把「场景」写成一份配置：requires / inputs / outputs。那不够。
你要的是一个**有上下文的子 Agent**：它知道自己在干什么、核心数据与核心文档是哪些、
正常情况下怎么用——然后才开始分析，按给入的条件给出不同的答案。

因此 Case 由三部分组成，而且这三部分**写入的人不同**：

| 组成 | 是什么 | 谁写 | 要不要拍板 |
|---|---|---|---|
| **上下文** | 干什么事、核心数据、核心文档、正常情况下怎么用 | **主对话 Agent** 协助建立 | 要（它是项目级登记的内容） |
| **experience** | 这个用途下积累的判断与经验 | Case 子 Agent 与人 | 不要（Case 内自主） |
| **CaseData** | 这个 Case 自己的数据表 | Case 子 Agent 与人 | 不要 |

这条分工是整个 Case 层的骨架：**上下文由主对话与人对好，经验由 Case 自己长**。
Case 不生产项目级事实——它只生产「在这个用途下怎么用这些事实」。

## 术语

- **Case**：项目下的一个用途。每个 Case 对应一个子 Agent。
- **上下文（context）**：Case 的立身之本，四项——干什么事、核心数据、核心文档、正常情况下怎么用。
- **核心数据 / 核心文档**：这个 Case 离不开的数据定义与文档。**是引用，不是复制。**
- **条件（input）**：调用时给入的取值（含偏好类的条件）。条件不同，答案不同。
- **答案（answer）**：Case 在一次给定条件下给出的结论，含它用了哪些数据与文档。
- **偏好（preference）**：一类条件。未声明时 Case **不替人做取舍**，给出多解。
- **依据已变**：Case 引用的项目级内容变了，因此产物需要复核的标注。

## 领域规则（Given-When-Then）

### 建立与上下文

- Given 一个项目 When 建立 Case Then 必须给出四项上下文：
  **干什么事**（一句话说清用途）、**核心数据**（引用哪些数据定义与字段）、
  **核心文档**（引用哪些文档）、**正常情况下怎么用**（步骤或判据）
- Given 一个 Case When 建立 Then 上下文**由主对话 Agent 协助建立**，
  并需人的拍板——它是项目级登记的内容，下游所有答案都建立在它上面
- Given 一个 Case 的上下文 When 缺任一项 Then **明确标出缺哪一项**；
  四项不齐时 Case 标为「未就绪」，不得当作可用
- Given 上下文里引用的数据定义、文档或词条 When 不存在 Then 加载时报出**全部**失效引用，
  不得只报第一个
- Given 一个 Case When 引用核心数据与核心文档 Then 是**引用**：
  同一项目内的多个 Case **共享**项目级内容，不复制、不各存一份
- Given Case 需要覆盖某条项目级计算方式或默认取值 When 声明 Then
  必须在上下文里**显式声明覆盖**，并说明理由；**不得静默覆盖**
- Given 两个 Case 声明了同名但内容不同的私有计算方式 When 检查 Then 报冲突，
  不得其中一个静默生效

### 只读边界

- Given 一个 Case When 运行 Then 只能**读**项目级六部件
  （主对话 Agent、工具、文档、词表、数据定义、项目基础数据）
- Given Case 的产物 When 需要落到项目级 Then **不得直接写**；
  写进自己的 experience，由主对话汇总后统一改（见 `agent.spec.md`）
- Given 一个 Case When 写 experience 或 CaseData Then 允许——它们是 Case 作用域
- Given 一个 Case 的 experience 与 CaseData When 查询 Then
  **其他 Case 看不到**；项目级内容才是它们唯一共享的东西
- Given Case 需要取数 When 执行工具 Then **允许执行**，但不得新增、修改或删除工具；
  产出留在 Case 作用域，不得写入项目级

### 条件与答案

- Given 一个 Case When 调用 Then 需要给出它声明的条件；
  条件不同则答案不同
- Given 一个未声明的条件名 When 传入 Then **拒绝**，并列出可用条件名
- Given 一个条件缺单位 When 传入 Then 按上下文声明补全；
  与声明不一致时**拒绝**，不做隐式换算
- Given 缺少必需条件 When 调用 Then 报告**需要用户提供什么**，不降级、不猜
- Given 一个条件声明了范围 When 越界 Then 拒绝并指出范围
- Given 一个答案 When 给出 Then 必须说明：用了哪些数据、哪些文档、
  **未核验比例**，以及所依据内容的**版本标识**
- Given 一个答案 When 给出 Then 只包含上下文中声明过的产出项，不得多给
- Given 一次调用失败 When 处理 Then **保留已填的条件**——
  那正是人最需要保留刚才填了什么的时候
- Given Case 引用的核心数据缺失 When 调用 Then 标记为「不可运行」而非「错误」，
  并逐项列出缺什么
- Given 核心数据的来源暂不可达 When 调用 Then 标为「**不可用**」，
  与「缺失」（数据不存在）**分开报告**——前者是环境问题，后者是数据问题

### 多解与不替人取舍

- Given 未声明偏好类的条件 When 调用 Then **不得**给出单一推荐答案，
  而给出一组**标注了各自假设**的备选
- Given 一组备选 When 给出 Then 每个都声明：目标、约束、代价（含机会成本）、
  以及自身的未核验比例
- Given 结果含随机因素 When 呈现 Then 用区间或期望，**不得**给出单点结论
- Given 一个备选 When 被另一个支配 Then 剪枝，并说明被谁支配
- Given 两个备选的差异低于阈值 When 处理 Then 合并，避免**伪多样性**——
  一排看着不同的方案、选哪个都一样
- Given 全部备选都被剪枝 When 处理 Then 输出的是**互相打架的约束清单**，
  而不是一个空结果
- Given 从历史选择推测出偏好 When 呈现 Then 标为**建议**，
  **必须经显式确认才生效**；确认前它不改变任何输出
- Given 人选定一个备选 When 完成 Then 其余备选**仍然可访问**——
  偏好只影响顺序与推荐，不删除任何东西
- Given 一个假设失效 When 检测到 Then 受影响的备选被标出，不静默沿用

### 答案与经验

- Given 一次分析得到一个值得留下的结论 When 记录 Then 写进 **experience**，
  并写明**当时的条件**与依据——这样重新给同样的条件就能复现
- Given 一次分析的答案 When 未记录 Then 它是一次性的：
  答案本身**不单独持久化**，要留下来就进 experience。
  理由：多一个「方案库」就多一套版本与失效规则，而 experience 已经能做这件事
- Given 一条经验 When 记录 Then 可以引用 CaseData 里的表，
  并**写清这张表怎么用**（见 `casedata.spec.md`）
- Given Case 引用项目级内容 When 该内容发生变更 Then
  Case 的产物与经验标为「**依据已变**」，并保留原依据标识
- Given 一条依据被否决 When 处理 Then 依赖它的经验标为失效并指出失效依据
- Given 一个答案 When 含未核验或不适用内容 Then 在答案上标注，
  包括「关键假设已失效」

### 生命周期

- Given 一个 Case When 启用 Then 参与就绪检查与调用
- Given 一个 Case When 禁用 Then 不参与检查，但其 experience 与 CaseData **仍可访问**
- Given 一个 Case When 不可运行 Then 标为「不可运行」并说明原因，
  **不得**呈现为「错误」——它可能只是数据还没接入
- Given Case 的上下文变更 When 应用 Then 走演进流程（见 `dataschema.spec.md`），
  并报告影响面
- Given 一个项目 When 没有任何 Case Then 允许；
  界面说明这不是错误，并给出建立入口

## 验收标准

- [ ] Case 建立时必须给出四项上下文，缺项被标出
- [ ] 上下文由主对话协助建立并需人拍板
- [ ] 上下文引用的定义、文档、词条不存在时全部列出
- [ ] 同一项目内多个 Case 共享项目级内容，不复制
- [ ] Case 覆盖项目级计算方式时必须显式声明并给出理由
- [ ] 两个 Case 的同名私有计算方式冲突时报出
- [ ] Case 无法写入项目级六部件的任何一处
- [ ] Case 的 experience 与 CaseData 对其他 Case 不可见
- [ ] Case 可以执行工具，但不能增删改工具
- [ ] 条件不同时答案不同
- [ ] 未声明的条件名被拒绝并列出可用条件名
- [ ] 条件缺单位时按声明补全；与声明不一致时拒绝，不做隐式换算
- [ ] 缺少必需条件时报告需要用户提供什么
- [ ] 条件越界时拒绝并指出范围
- [ ] 答案说明用了哪些数据、哪些文档、未核验比例与版本标识
- [ ] 答案只含声明过的产出项
- [ ] 调用失败时保留已填条件
- [ ] 核心数据缺失时标为「不可运行」并逐项列出
- [ ] 「不可用」与「缺失」分开报告
- [ ] 未声明偏好时不给出单一答案，而给出标注各自假设的备选
- [ ] 每个备选声明目标、约束、代价与未核验比例
- [ ] 含随机因素的结果以区间或期望呈现
- [ ] 被支配的备选被剪枝并说明被谁支配
- [ ] 差异低于阈值的备选被合并
- [ ] 全部备选被剪枝时输出冲突的约束清单，而非空结果
- [ ] 偏好推断标为建议且需显式确认才生效
- [ ] 选定后其余备选仍可访问
- [ ] 假设失效时受影响的备选被标出
- [ ] 值得留下的结论写进 experience 并写明条件与依据
- [ ] 答案本身不单独持久化
- [ ] 项目级内容变更后 Case 产物标为「依据已变」并保留原依据标识
- [ ] 依据被否决时依赖它的经验标为失效并指出失效依据
- [ ] 禁用的 Case 不参与检查但 experience 与 CaseData 仍可访问
- [ ] 不可运行的 Case 标为「不可运行」而非「错误」
- [ ] 项目没有 Case 时界面说明这不是错误

## 边界与异常

- Case 名为空或与项目内既有 Case 重名：拒绝
- 上下文四项中有项是占位文本（如「待补充」）：视为缺项
- 核心数据引用了另一个项目的定义：拒绝（项目间不建立运行时依赖）
- 核心文档被标记为「依据不可达」：Case 仍可运行，但答案标注该文档不可达
- 条件值类型与声明不符：拒绝，指出期望类型
- 条件为必需但传空字符串：拒绝并说明（空字符串不是缺省值）
- 一次调用没有给出任何条件而 Case 需要条件：报告需要哪些，不尝试默认值
- 答案为空（没有任何产出）：允许，但必须说明为什么为空，
  不得返回一个空壳让人以为坏了
- 备选数量很多：按差异与支配关系收拢，不得原样倒出几十个
- 同一个 Case 被并发调用：允许，两次调用的答案各自独立
- CaseData 被删除而 experience 仍引用它：experience 标为「依据不可达」，不删除经验
- Case 的上下文引用了自己的 CaseData：拒绝——上下文只引用项目级内容

## 前后端契约

`CaseService`（作用域：项目与 Case）：

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `List` | — | `CaseView[]` | 项目内全部 Case 及就绪情况 |
| `Get` | `name` | `CaseView` | |
| `Put` | `CaseInput, by, reason` | `CaseView` | 建立或改上下文；需人拍板 |
| `Context` | `name` | `ContextView` | 四项上下文与逐项就绪情况 |
| `Check` | `name` | `CheckView` | 引用检查：核心数据/文档/词条是否存在与可用 |
| `Inputs` | `name` | `PortView[]` | 需要哪些条件（名、类型、量纲、范围、是否必填） |
| `Enable` / `Disable` | `name, by, reason` | `CaseView` | |
| `Ask` | `name, args` | `Answer` | 给条件要答案；**不持久化答案** |
| `Impact` | `name` | `ImpactView[]` | 项目级变更对它有什么影响 |
| `Overrides` | `name` | `OverrideView[]` | 显式声明的覆盖 |

`CaseView`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `name` | string | 否 | |
| `title` | string | 否 | 中文名；无则界面显示 name |
| `purpose` | string | 否 | 干什么事（一句话） |
| `ready` | bool | 否 | 四项上下文齐备且引用无失效 |
| `readyText` | string | 否 | 不就绪时说明缺什么 |
| `enabled` | bool | 否 | |
| `inputs` | int | 否 | 条件个数 |
| `experiences` | int | 否 | 经验条数 |
| `tables` | int | 否 | CaseData 表数 |
| `stale` | int | 否 | 依据已变或失效的产物数 |
| `signedBy` | `ActorView \| null` | 是 | |

`ContextView`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `purpose` | string | 否 | 干什么事 |
| `data` | `RefView[]` | 否 | 核心数据：数据定义与字段 |
| `docs` | `RefView[]` | 否 | 核心文档 |
| `terms` | `RefView[]` | 否 | 核心词条（可选） |
| `how` | string | 否 | 正常情况下怎么用 |
| `overrides` | `OverrideView[]` | 否 | 显式覆盖声明 |
| `missing` | `string[]` | 否 | 缺项与失效引用，逐项可读 |

`RefView`：`{ kind, ref, label, note, ok, why }`
——`kind` 为 `schema` / `field` / `doc` / `term`；`ok` 为 false 时 `why` 说明原因。

`OverrideView`：`{ target, kind, detail, reason, signedBy }`
——`target` 是被覆盖的项目级对象标识。

`Answer`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `ok` | bool | 否 | |
| `message` | string | 否 | 失败原因，或为空的说明 |
| `summary` | string | 否 | 结论摘要 |
| `options` | `OptionView[]` | 否 | 未声明偏好时可能有多个；单解时一项 |
| `used` | `RefView[]` | 否 | 用了哪些数据与文档 |
| `unverified` | float | 否 | 未核验比例 |
| `basisVersions` | `string[]` | 否 | 所依据内容的版本标识 |
| `warnings` | string[] | 否 | 关键假设失效、公式未验证、依据已变等 |
| `kept` | `Record<string,string>` | 否 | 回显本次给入的条件（失败时也回显） |

`OptionView`：`{ title, objective, constraints, costs, opportunity, unverified, dominatedBy, warnings }`

约定：

- `Ask` **没有**持久化答案的副作用；要留下结论必须显式写 experience
- `Inputs` 必须给全量声明，使界面能生成表单；缺单位与范围条件的界面**必须**标出，
  否则填出来的值不可比
- `Answer.warnings` 不得为空数组以外的方式吞掉问题：有「依据已变」「关键假设失效」
  「公式未验证」时必须出现在这里，并按严重程度排序
- `Context` 与 `Check` 分成两个接口：前者给人看「这个 Case 是干什么的」，
  后者给人看「它现在能不能用」——混在一起会让两者都看不清

## 待确认

1. 备选的差异阈值与数量上限
2. 偏好推断的确认方式（逐条确认还是批量）
3. 「正常情况下怎么用」是自由文本还是结构化步骤（当前为自由文本 + 可选步骤列表）
4. Case 是否需要声明产出项的 schema（当前只声明产出项名字）
5. 答案是否需要临时保留一段时间以便「刚才那个结果」可回看
