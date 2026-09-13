# MVP 实施范围

> 从 13 份规格（1835 行）中抽取**最小可运行集**，用于验证架构是否成立。
> 原则：跑通一条端到端切片，比实现十份规格的一半更有价值。

## 一、MVP 的目标

**证明这条链路成立**：

```
本地已存档的 wiki 原件
  → 接入（解析）
  → 准入（schema 校验 + 分级 + 冲突检测）→ 变更集
  → 断言入库
  → 派生（满级面板，L3）
  → 表达式求值（含量纲检查）
  → 场景 requires 检查 → 方案输出
```

**不做 GUI**。核验工作台是后期的事；MVP 用 CLI 验证机制。

## 二、范围

| 项 | 值 |
|---|---|
| 项目 | `onmyoji`（阴阳师） |
| 场景 | `damage-calc`（伤害计算） |
| 数据源 | 本地 `.huiji/raw/` 中已同步的 423 个页面 |
| 主数据 | `Data:Attribute.json`（275 个式神属性）、`Data:CharacterIndex.json`、`Data:Character/<id>.json` |
| 存储 | SQLite 单文件（`modernc.org/sqlite`，已实测可离线用） |
| 入口 | Go CLI |

## 三、实现清单（按层）

### `internal/domain`

| 包 | 内容 | MVP 范围 |
|---|---|---|
| `value` | 值的三态 | `present` / `unknown` / `null` / `absent` |
| `unit` | 量纲与换算 | 单位 + 维度 + 换算表；加减要求同量纲、乘除组合单位 |
| `metamodel` | 类型与约束原语 | 类型：`text` `number` `bool` `enum` `ref` `object` `list`；约束：`required` `identity` `unique` `min` `max` `unit` `values` `items` `fields` `target` |
| `schema` | schema 结构 | 加载、身份字段、**双向漂移检测** |
| `validate` | 六类校验 | 必填、类型、范围、枚举、唯一、引用完整性 |
| `expr` | 表达式求值 | lexer + parser + eval；算术、比较、逻辑、条件、字段访问；**单位检查** |
| `assertion` | 断言模型 | subject / predicate / value / qualifiers / source / provenance / confidence / status |
| `confidence` | 分级 | L1 / L2 / L3 / L4 |

**不做**：`opaque`、`longtext`、`date`、记录级约束（`atLeastOne` 等）、枚举开放性策略、经验模型。

### `internal/application`

| 用例 | MVP 范围 |
|---|---|
| 接入 | 从本地原件目录读取 + JSON 解析器 → 候选 |
| 准入 | schema 校验 + 分级 + 冲突检测 → **变更集**（不直接写库） |
| 派生 | 满级面板 → L3 断言（上下文中立，项目级） |
| 场景 | `requires` 检查 + 运行 + 输出 |

**不做**：重放、别名表、经验准入、核验队列、多解方案。

### `internal/infrastructure`

| 适配器 | MVP 范围 |
|---|---|
| `artifact` | 原件存档读写（只读本地已有目录） |
| `store` | SQLite：断言表、变更集原子应用、唯一约束 |
| `schemafile` | YAML schema 加载 |

**不做**：缓存、迁移引擎、网络抓取（CDP）、导出器。

### `internal/api`

CLI 子命令：`schema`（校验 schema）、`sync`（接入+准入+应用）、`derive`（派生）、`run`（场景运行）、`verify`（数据体检）。

## 四、项目文件

```
projects/onmyoji/
├─ project.yml
├─ units.yml
├─ schema/
│  ├─ shikigami.schema.yml
│  └─ skill.schema.yml
├─ formulas/
│  └─ damage.formula.yml
└─ scenarios/
   └─ damage-calc/
      └─ scenario.yml
```

## 五、伤害公式的诚实范围

**关键约束：防御减免公式我们没有权威来源**（见 `docs/notes/case-requirements.md`）。

因此 MVP **不编造公式**，而是把防御减免作为**外部输入参数**，并在输出中标注它未核验：

```
伤害 = 面板攻击 × 技能倍率 × 暴击期望系数 × 防御减免
        └─ 派生 L3    └─ 数据 L2     └─ 计算        └─ 输入，未核验
```

这本身就是设计的一次演示：**缺的部分被显式标注，而不是被猜出来。**

## 六、验收标准（可执行）

- [ ] `go vet ./...` 与 `go test ./...` 全绿
- [ ] schema 加载：合法通过，非法（未知类型/未知约束/缺单位）失败并指出位置
- [ ] 六类校验各有正反用例
- [ ] 双向漂移检测：能报出「数据有而 schema 无」与「schema 有而数据从无」
- [ ] 量纲：`percent + point` 报错；`50%` 与 `0.5` 归一等价
- [ ] 表达式：字段引用、算术、条件、单位检查；引用不存在字段时**撰写期**报错
- [ ] 准入：冲突断言**并存并标记**，不覆盖
- [ ] 准入产出变更集；应用原子（失败全回滚）
- [ ] 派生：满级面板生成为 L3 断言并带推导链
- [ ] 场景：`requires` 缺失时**拒绝运行**并列出缺失项
- [ ] 端到端：用真实 `Data:Attribute.json` 跑通，产出 275 个式神断言与一份伤害输出
- [ ] 数据体检：穷尽性（式神数 = 索引声明数）与字段覆盖率可报告
- [ ] `docs/` 中的规格与实际实现一致（发现偏差先改规格）

## 七、实施顺序

```
1. domain/value + domain/unit
2. domain/metamodel + domain/schema（含漂移检测）
3. domain/validate（六类）
4. domain/expr（lexer → parser → eval → 单位检查）
5. domain/assertion + confidence
6. infrastructure/schemafile + store（SQLite）
7. application/ingest + admit
8. application/derive
9. application/scenario
10. api/cli
11. 端到端跑真实数据
```

每层**先写测试**（SDD：验收测试红 → 端口接口 → 实现绿）。

## 八、明确不做（MVP 外）

`migration` / `verification` 队列 / `experience` / `alternatives` 多解 / `player-profile` 归档 /
`derivation` 缓存与失效传导 / `admission` 别名与重放 / `ingestion` 网络与 CDP / 全部 GUI。

它们在规格里已定义，MVP 之后按需增厚。
