# 数据：定义、断言库与数据质量

## 目的

回答一个必须说清楚、否则界面会被误解的问题：**同一批数据的三个视图是什么关系。**

```
定义（schema）      应该有什么     人写的，进 git，可评审、可分享
断言库（store）     实际有什么     程序写的，可重建
数据质量（quality） 差多少         没人写，每次算
```

第三行是关键：**数据质量不是第三种数据，是一次比较的结果。** 它不落在任何表里。
删掉所有断言，它不会变成空表，它会实时变成「每个字段覆盖 0%」。

三者必须分开，因为**它们回答的问题不同，而合起来会丢掉最重要的那个信号**：

| | 定义 | 断言库 | 数据质量 |
|---|---|---|---|
| 能告诉你 | 应该有哪些字段 | 有哪些事实 | 差在哪里 |
| **不能**告诉你 | 数据长什么样 | **少了什么** | 单个事实是什么 |

「少了什么」只能由前两者相减得出。把三者合并成一张「字段 + 值 + 覆盖率」的大表，
**「声明了但是空的」就与「根本没定义」长得一样**——而前者是缺口，后者只是没定义。
场景能不能跑，恰恰取决于能不能区分这两者。

## 术语

- **定义（schema）**：实体类型、字段、类型、量纲、必填 / 身份 / 唯一、引用目标。
  **它是唯一能声明「意图」的地方**——没有声明就没有「缺」。
- **断言库（store）**：一条条事实。它只能说明「有什么」。
- **数据质量（quality）**：定义与断言库的差集与覆盖率，**读时算出**。
- **未声明字段**：数据里出现而定义里没有 → 定义落后于数据。
- **从未出现的字段**：定义里有而数据里从未出现 → 数据落后于定义。
- **覆盖率**：有该谓词的**主体数** ÷ 该实体的主体总数。

## 领域规则（Given-When-Then）

### 三个视图的边界

- Given 数据页 When 显示「定义」 Then 只呈现 schema，**不混入覆盖率或数据现状**——
  定义是「应该」，把它与「实际」画在一起会让人以为它是从数据里推出来的
- Given 数据页 When 显示「断言库」 Then 只呈现事实本身，**不显示覆盖率或漂移**
- Given 数据页 When 显示「数据质量」 Then 必须**逐字段**给出覆盖率与两个方向的漂移，
  且必须能追溯到具体字段名
- Given 一条断言的核验状态 When 在数据页显示 Then 呈现状态与分级，
  但**核验动作不在这一页**——核验是另一件事，有它自己的队列与责任记录

### 覆盖率的分母

- Given 某主体某谓词有多条限定条件不同的断言 When 统计覆盖率
  Then **只计一个主体**，不得按断言数累加——否则会算出大于 100% 的覆盖率
- Given 字段存在但内容为空（`unknown` / `null`） When 统计覆盖
  Then **不计为已填充**——「字段存在」不等于「已被填充」
- Given 某实体一个主体都没有 When 显示覆盖率 Then 显示为 0，**不得显示 100% 或留空**

### 以定义为准，而不是以库里有数据的为准

- Given 一个在 schema 中声明、但库里一条数据都没有的实体
  When 显示质量 Then **必须列出来**，并标为「无数据」
- Given 一个库里完全没有的实体 When 遍历 When 不出现
  Then 那是最该被看见的缺口被藏起来了——**遍历必须以定义为准**
- Given 数据里出现而定义未声明的字段 When 统计 Then 标为未声明，并说明它的后果：
  这些字段会被准入层过滤掉，也就是**「接进来了但没入库」**

### 词表（术语呈现）

界面上不得只出现原始标识符。名称的**来源必须是指定义或数据本身**，不得另起一套：

| 呈现什么 | 名称从哪来 |
|---|---|
| 字段 | schema 的 `description` |
| 实体 | schema 的 `description` |
| 单位 | 单位表 |
| 字段类型 | 元模型的类型枚举 |
| 分级 / 状态 / 参与者 | 领域层的枚举中文名 |
| 主体 | **数据里的 `name` 断言** |
| 依赖 | `assert:` / `formula:` 前缀的解析 |
| 修订 | `revid:` 前缀的解析 |

- Given 一个标识符 When 显示 Then **中文名与标识符都要在**（中文在前、标识符在后）——
  只给中文，人无法与数据、CLI、规格里的标识符对上；只给标识符，人看不懂
- Given 定义里没有中文名 When 显示 Then **退回显示标识符，不得编一个名字**：
  一个听起来合理的错名字比看不懂更糟
- Given 词表 When 提供 Then 它随项目加载，换项目换一套词
- Given 同一套枚举 When CLI 与界面显示 Then 必须用**同一处定义**的中文名，
  不得各自维护一份

## 验收标准

- [ ] 定义、断言库、质量三个视图各自只呈现自己该呈现的，不互相混入
- [ ] 质量逐字段给出覆盖率，且能追溯到字段名
- [ ] 覆盖率的分母是主体数；同一谓词多条断言不重复计入
- [ ] 空值（unknown / null）不计为已填充
- [ ] 没有数据的实体也被列出并标为「无数据」
- [ ] 未声明字段被列出，并说明它会被准入层过滤
- [ ] 从未出现的字段被列出
- [ ] 必填但无数据的字段被单独标出
- [ ] 界面上的字段、实体、单位、类型、分级、状态、参与者、主体、依赖、修订都有中文名
- [ ] 中文名与原始标识符同时显示
- [ ] 定义里没有中文名时退回显示标识符，不编造
- [ ] 词表随项目切换而更新
- [ ] CLI 与界面使用同一套中文名

## 边界与异常

- schema 里一个实体都没声明：质量页说明「还没有声明任何实体」，而不是留白
- 某实体声明了字段但主体数为 0：覆盖率全部显示 0%，并说明「无数据」
- 某谓词在库里存在但 schema 未声明：**它一定是从绕过准入的路径进来的**
  （例如派生直接写库），报告中必须指出这一点，而不是只列个字段名
- 词表加载失败：界面退回显示标识符，**不得因此白屏或挡住数据**——
  名称是辅助，数据是主体
- 同一个单位在项目单位表里被重新定义：以项目定义为准，并保留引擎默认名作为回退
- 主体没有 `name` 断言：显示「实体 主体标识」，**不得显示空字符串**
- 一个字段既是未声明又在必填清单里：不可能同时成立（必填来自定义），
  若出现说明数据被外部改过，报告异常

## 前后端契约

### `DataService`

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `Schema` | — | `EntitySchema[]` | 定义 |
| `Quality` | — | `EntityQuality[]` | 质量，以 schema 声明为准遍历 |
| `Assertions` | `FilterInput, limit, offset` | `AssertionPage` | 断言库分页 |
| `SubjectDetail` | `entity, subject` | `Item[]` | 某主体上的全部断言 |
| `Subjects` | `entity` | `string[]` | 主体列表 |
| `EntityNames` | — | `string[]` | schema 声明的实体名 |

`EntitySchema`：`{ entity, description, schemaRev, fields: FieldSchema[] }`
`FieldSchema`：`{ key, description, type, unit, required, identity, unique, target, values, min, max }`

`EntityQuality`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `entity` | string | 否 | |
| `subjects` / `assertions` | int | 否 | 主体数与断言数 |
| `fields` | `FieldQuality[]` | 否 | `{ key, declared, required, coverage, subjects }` |
| `undeclared` | string[] | 否 | 数据里有而定义没有 |
| `unused` | string[] | 否 | 定义里有而数据从未出现 |
| `requiredMissing` | string[] | 否 | 必填却一条数据都没有 |

`AssertionPage`：`{ items: Item[], total, offset, limit, where }`
——`where` 是筛选条件的人话描述，**无条件时必须显式提示**它一次选中了全部。

### `FormulaService`

| 方法 | 参数 | 返回 |
|---|---|---|
| `List` | — | `FormulaView[]` |

`FormulaView`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `name` / `version` / `description` | string | 否 | |
| `result` | string | 否 | 结果表达式**原文**；不透明的公式没法被审阅 |
| `params` | `FormulaParamView[]` | 否 | `{ name, type, unit, note }` |
| `bindings` | `BindingView[]` | 否 | `{ name, path }` |
| `status` | string | 否 | `verified` / `unverified` / `failed` / `unreadable` |
| `cases` | `FormulaCaseView[]` | 否 | `{ name, passed, got, want, detail }` |
| `error` | string | 否 | 仅读不出来时非空 |
| `file` | string | 否 | 来自哪个文件，便于去改 |
| `usedBy` | string[] | 否 | 哪些场景在用它 |

- 逐个文件加载：**一条写坏的公式不该让整页打不开**，它应该被单独指出
- 三级状态必须一眼可分：算例全过 / 无算例（未验证）/ 算例未过（拒绝使用）。
  把「无算例」显示成绿色，等于把未验证的公式伪装成验证过的

### 词表

词表挂在 `ProjectService` 上（它随项目变化）：`Glossary()` → `Glossary`。

`Glossary` 的每个映射都是 `标识符 → { key, label, note, extra }`：

| 字段 | 内容 | 名从哪来 |
|---|---|---|
| `entities` | `shikigami` → 式神 | schema |
| `fields` | `shikigami.atk` → 攻击 | schema |
| `units` | `percent` → 百分比 | 单位表 |
| `types` | `number` → 数值 | 元模型 |
| `subjects` | `skill/262_03` → 天翔鹤斩 | **数据里的 name 断言** |
| `confidences` / `statuses` / `actorKinds` | L2 → 结构化 / pending → 待核验 / human → 人 | 领域枚举 |
| `expKinds` / `expStatuses` / `decStatuses` / `planStatuses` | 经验、待判定、方案的状态 | 领域枚举 |

可空性约定：`subjects` 是 `map[string]string`；查不到的主体**不出现在映射里**，
界面那时显示「实体 主体标识」，而不是空字符串。
`label` 为空串表示定义里没有中文名——界面退回显示 `key`。
