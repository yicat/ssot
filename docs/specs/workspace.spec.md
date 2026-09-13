# 工作台：项目、场景与会话

## 目的

界面此前只呈现核验层。**项目**与**场景**这两层只活在数据目录与命令行参数里——
换项目要重启进程，看场景要敲 `ssot run`。本层把它们显式化：

**一个工作台，当前项目 → 当前场景 → 该场景的工作区。**

它同时回答一个此前没有答案的问题：**哪些东西属于项目，哪些属于场景？**
项目级的定义（schema、公式、断言库、核验）不随场景变化，因此不挂在场景下；
场景级的（requires、运行输入、经验、备选方案）随场景变化。

## 术语

- **会话（session）**：当前打开的项目 + 当前选中的场景。它是界面的上下文，不是数据。
- **项目（project）**：一个领域（如「阴阳师」）。它的边界同时是**隔离边界**与**定义边界**。
- **场景（scenario）**：项目下的一个用途（如「伤害计算」）。它**不拥有数据**，
  只声明自己需要什么。
- **外部输入**：场景运行时由使用者提供的值（如防御减免）。它与断言的区别是
  **没有来源**——因此必须始终带着「未核验」的标记。

## 领域规则（Given-When-Then）

### 项目

- Given 工作台启动且未指定项目 When 加载 Then 取项目列表的第一个；
  列表为空时**明确提示「没有可用项目」**，不得静默指向一个不存在的目录
- Given 项目列表 When 显示 Then 只列出**含 `project.yml` 的目录**——
  一个空目录或半成品目录不是项目
- Given 切换到新项目 When 加载失败（目录不存在 / 缺 `project.yml` / schema 校验不通过）
  Then **保持原项目不变**并返回失败原因；不得留下一个半加载的项目
- Given 切换项目成功 When 完成 Then 当前场景重置为该项目的第一个场景；
  该项目没有场景时当前场景为空
- Given 一个项目 When 显示概览 Then 必须给出：目录、实体类型、断言总数、按状态分布、
  按分级分布、场景数、公式数

### 场景

- Given 一个项目 When 列出场景 Then 按名称排序，每项给出名称、描述、
  是否需要外部输入、`requires` 的满足情况
- Given 切换场景 When 成功 Then **场景级视图**重新取数；
  **项目级视图（数据、公式、核验）不受影响**——它们不属于任一场景
- Given 项目没有场景 When 显示 Then 说明「该项目还没有场景」，
  而不是留白或报错
- Given 场景的 `requires` 未满足 When 显示 Then 逐项列出「需要什么 / 现有多少 /
  覆盖率 / 缺什么」，**不得只报一个总数**

### 场景运行

- Given 场景运行 When `requires` 未满足 Then **拒绝运行**并列出缺失项
  （规则本体见 `scenario.spec.md`，本层只定义呈现契约）
- Given 场景运行 When 使用者提供外部输入 Then 每个输入必须是
  **名 + 值 + 单位**三元组；缺单位时按该输入声明的单位补全，
  既没给也没声明则**拒绝**，不得臆测量纲
- Given 外部输入的名字未在场景中声明 When 提交 Then **拒绝**并列出可用的输入名
- Given 外部输入的单位与声明不一致 When 提交 Then 拒绝，
  **不得隐式换算**（除非单位表明确声明了这两个单位的换算关系）
- Given 一次运行完成 When 输出 Then 必须同时给出：结果值及其单位、
  该结果依赖的断言中**未核验的比例**、以及所有未核验/外部输入的显式标注
- Given 运行失败 When 显示 Then 原因必须可见并保留使用者已填的输入，
  不得清空表单

### 呈现的通用约束

- Given 任一列表为空 When 显示 Then 必须说明「没有」以及**这意味着什么**，
  不得留白——留白与「加载失败」在界面上无法区分
- Given 一次加载失败 When 显示 Then 错误必须可见；不得静默失败
- Given 界面上的数字 When 显示 Then 必须可追溯到它的来源（哪个项目、哪个场景、
  哪次运行）；不可追溯的数字不得出现在界面上

## 验收标准

- [ ] 没有可用项目时给出明确提示，而不是指向不存在的目录
- [ ] 项目列表只包含含 `project.yml` 的目录
- [ ] 切换项目失败时保持原项目不变，并返回失败原因
- [ ] 切换项目成功后当前场景重置为该项目的第一个场景
- [ ] 项目概览给出目录、实体类型、断言总数、状态分布、分级分布、场景数、公式数
- [ ] 场景列表按名称排序，并显示每个场景的 requires 满足情况
- [ ] 切换场景只重取场景级视图，项目级视图数据不变
- [ ] 没有场景的项目给出明确说明
- [ ] requires 未满足时逐项列出需求、现有、覆盖率
- [ ] 外部输入缺单位且无声明时被拒绝
- [ ] 外部输入名未声明时被拒绝并列出可用名
- [ ] 外部输入单位与声明不一致时被拒绝，不做隐式换算
- [ ] 运行输出带上未核验断言的比例
- [ ] 运行失败时保留已填输入
- [ ] 空列表都给出「没有 + 这意味着什么」的说明

## 边界与异常

- 项目目录不存在：拒绝切换，报目录路径
- 目录存在但没有 `project.yml`：拒绝切换，说明「这不是一个项目目录」
- `units.yml` 或 `schema/` 缺失：拒绝切换，报具体缺哪个文件
- schema 校验不通过：拒绝切换，**列出全部问题**（一次看完，而不是修一个跑一次）
- 两个项目目录同名：允许（以目录名为标识），但界面上必须显示完整路径
- 项目在会话期间被外部删除：下一次取数时报错，**不静默返回空**
- 场景目录存在但没有 `.scenario.yml`：列出时跳过并说明跳过原因，
  不得当成一个可用场景
- 同一个场景名出现两次：拒绝加载该项目并说明冲突
- 运行中切换项目/场景：本次运行结果作废，不得把 A 的结果显示在 B 之下
- 外部输入给了值但类型不是数值：拒绝
- 外部输入给了负值而该输入声明了范围约束：拒绝并说明范围

## 前后端契约

界面分两层：**会话**（当前项目 / 当前场景）与**取数**（各页面）。

### `ProjectService`

| 方法 | 参数 | 返回 | 可空 |
|---|---|---|---|
| `Projects` | — | `ProjectRef[]` | 否（空数组表示一个都没有） |
| `Open` | `dir string` | `SessionState` | 否；失败返回 error，**会话不变** |
| `Current` | — | `SessionState` | 否 |
| `Overview` | — | `ProjectOverview` | 否 |

`ProjectRef`：`{ dir string, name string, path string }`
——`name` 取自 `project.yml`，`path` 是完整路径（同名项目靠它区分）。

`SessionState`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `dir` | string | 否 | 当前项目目录 |
| `name` | string | 否 | 项目名 |
| `projectDir` | string | 否 | 完整路径 |
| `scenarios` | `ScenarioRef[]` | 否 | 该项目的场景，按名称排序 |
| `scenario` | string | **是** | 当前场景名；null 表示未选中或该项目没有场景 |

`ProjectOverview`：

| 字段 | 类型 | 说明 |
|---|---|---|
| `dir` / `name` | string | 目录与名称 |
| `entities` | `EntitySummary[]` | `{ entity, subjects, assertions }` |
| `assertions` | int | 断言总数 |
| `byStatus` | `map[string]int` | 按状态分布 |
| `byConfidence` | `map[string]int` | 按分级分布 |
| `verifications` | int | 核验记录数 |
| `conflicts` | int | 冲突组数 |
| `scenarioCount` | int | 场景数 |
| `formulaCount` | int | 公式数 |
| `decisions` | int | 待判定条数 |

`ScenarioRef`：`{ name string, description string, inputs int, requiresMet bool, requiresTotal int }`

### `ScenarioService`

| 方法 | 参数 | 返回 |
|---|---|---|
| `List` | — | `ScenarioRef[]` |
| `Overview` | `name string` | `ScenarioOverview` |
| `Select` | `name string` | `SessionState` |
| `Run` | `name string, subject string, inputs []RunInput` | `RunResult` |

`ScenarioOverview`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `name` / `description` | string | 否 | |
| `runnable` | bool | 否 | requires 是否全部满足 |
| `requires` | `Requirement[]` | 否 | `{ want, status, have, total, coverage, detail }` |
| `inputs` | `InputSpec[]` | 否 | `{ name, description, unit, required, min, max }` |
| `formulas` | `FormulaStatus[]` | 否 | `{ name, status, detail }` |
| `outputs` | string[] | 否 | 输出项名 |

`RunInput`：`{ name string, value string, unit string }`
——`unit` 为空时按 `InputSpec.unit` 补全；`InputSpec.unit` 也为空则拒绝。

`RunResult`：

| 字段 | 类型 | 可空 | 说明 |
|---|---|---|---|
| `ok` | bool | 否 | |
| `outputs` | `OutputValue[]` | 否 | `{ name, value, unit, derived, unverified }` |
| `unverifiedRatio` | float64 | 否 | 依赖断言中未核验的比例，0~1 |
| `unverified` | `ClaimRef[]` | 否 | 未核验的依赖，`{ id, subject, predicate, value, status, confidence }` |
| `external` | `ExternalInput[]` | 否 | 本次使用的外部输入，**始终标注未核验** |
| `message` | string | 否 | 人话结果或拒绝原因 |

可空性约定：Go 的 `nil` 切片序列化为 `null`，前端一律补成空数组再渲染；
`SessionState.scenario` 为 `null` 时界面必须显示「未选择场景」，不得显示空字符串。

### 数据页与公式页

它们的契约不在本规格里，见 `data.spec.md`（定义 / 断言库 / 数据质量三个视图的关系、
`DataService`、`FormulaService`、以及跨页面共用的**词表**）。

### 页面与服务的对应

本规格定的是**壳**（会话、项目、场景、导航）；每个页面自己的规则在它自己的规格里：

| 页面 | 规格 |
|---|---|
| 项目概览 | 本规格（`ProjectService.Overview`） |
| 数据：定义 / 断言库 / 数据质量 | `data.spec.md` |
| 公式 | `data.spec.md`（行为约束见 `computation.spec.md`） |
| 场景概览 / 运行 | 本规格 + `scenario.spec.md` |
| 核验：队列 / 冲突 | `verification.spec.md` |
| 待判定 | `decision.spec.md` |
| 经验 | `experience.spec.md` |
| 备选方案 | `alternatives.spec.md` |
