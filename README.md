# ssot

单一事实源（Single Source of Truth）工具。Go + Wails v3 + Vite/React/shadcn。

回答的问题：当数据散在会过期、会互相抄、无法验证的来源里（社区 wiki、攻略、实测），
**如何让它可信、可追溯、可复用**。

## 快速开始

```powershell
# 前端依赖（首次）
cd frontend; npm install

# 开发运行（热重载）
wails3 task dev

# 测试
wails3 task test          # go test ./... + 前端 vitest
wails3 task check         # go vet + 全部测试 + 前端构建（提交前跑）

# 构建
wails3 task build         # 产物 bin/ssot.exe（GUI）
```

> `task` 无需单独安装——Wails v3 自带 `wails3 task`。

## 三层模型

```
引擎   ── 机制：值的三态、量纲、元模型、断言、核验、经验、备选方案
  └ 项目 ── 一个领域（阴阳师）：schema、公式、场景、数据源。**隔离边界**
      └ 场景 ── 一个用途（伤害计算）：只声明自己需要什么。**用途边界，不拥有数据**
```

界面按这三层下钻：顶栏选项目与场景，左栏按「项目 / 场景 / 核验」分区。

## 界面

| 分区 | 页面 | 做什么 |
|---|---|---|
| 项目 | 概览 | 定义、规模、状态与分级分布、质量缺口 |
| 项目 | 数据 | **定义 / 断言库 / 数据质量**三个视图（见下） |
| 项目 | 公式 | 公式原文、算例、三级验证状态 |
| 项目 | 核验 | 队列（按优先级 + 抽检）/ 冲突 / 待判定 |
| 场景 | 概览 | requires 逐项：需要什么、覆盖率、能不能跑 |
| 场景 | 运行 | 填外部输入 → 方案 + **未核验比例** |
| 场景 | 经验 | 判断、责任级别、产生它的会话记录 |
| 场景 | 备选方案 | 多解并排，**系统不裁决** |

### 数据页的三个视图是什么关系

```
定义        应该有什么    人写的，进 git，可评审、可分享
断言库      实际有什么    程序写的，可重建
数据质量    差多少        没人写，每次算
```

第三行是关键：**数据质量不是第三种数据，是一次比较的结果**，不落在任何表里。

三者必须分开，因为**合起来会丢掉最重要的那个信号**：「声明了但是空的」会与
「根本没定义」长得一样——而前者是缺口，后者只是没定义。场景能不能跑，
恰恰取决于能不能区分这两者。

详见 `docs/specs/data.spec.md`。

## 核心立场

- **默认一切未核验。** 未核验内容允许使用，但必须可见——而不是「核验通过才能用」，
  那在人力上不现实，结果是工具直接不可用。
- **系统不替人做取舍。** 冲突并存并标记；歧义交给人裁决；备选方案并排呈现。
- **agent 可以提出与取证，批准必须是人。** 无追责的核验等于没有核验。
- **缺的部分显式标注，不猜。** 「没有这个数据」与「文本里有但无法确定」是两回事。
- **不确定就报告，不降级。** 拿不准的解析结果不得标成直引。

## 结构

```
├─ cmd/ssot/                # CLI 入口
├─ main.go                  # Wails 桌面应用入口
├─ internal/
│  ├─ domain/               # 领域层：纯规则，只允许标准库
│  │  ├─ value/ unit/ metamodel/ schema/ validate/ expr/
│  │  ├─ assertion/         # 断言与变更集
│  │  ├─ verification/      # 核验记录：四级方法、责任归属
│  │  ├─ decision/          # 待判定：歧义事项与裁决
│  │  ├─ experience/        # 经验：责任级别、会话依据
│  │  └─ alternatives/      # 备选方案：多解、剪枝、取舍
│  ├─ application/          # 用例编排：接入、准入、公式、派生、核验、待判定、经验、场景
│  ├─ infrastructure/       # 适配器：SQLite、YAML、原件存档
│  └─ api/                  # 接口层：wails3 bindings
├─ frontend/
│  ├─ src/pages/            # 页面 = 纯编排
│  ├─ src/components/ui/    # shadcn 生成，勿手改
│  └─ src/components/custom/# 业务组件：index.tsx + useXxx.ts + store.ts
├─ projects/onmyoji/        # 一个项目 = 一个领域
└─ docs/
   ├─ specs/                # 规格（唯一事实源）
   ├─ notes/                # 调研与实测记录（非规范，可能过期）
   └─ mvp-scope.md          # MVP 实施范围
```

## CLI

```powershell
go run ./cmd/ssot schema   projects/onmyoji   # 校验 schema 与单位表
go run ./cmd/ssot sync     projects/onmyoji   # 接入 + 准入 + 原子应用
go run ./cmd/ssot derive   projects/onmyoji crit_factor   # 派生 L3（先验算例）
go run ./cmd/ssot status   projects/onmyoji   # 断言库状态
go run ./cmd/ssot decision projects/onmyoji list          # 待判定队列

# 核验（队列按优先级 + 强制抽检）
go run ./cmd/ssot review projects/onmyoji queue --limit 20 --sample 0.05
go run ./cmd/ssot review projects/onmyoji conflicts        # 冲突成组，系统不裁决
go run ./cmd/ssot review projects/onmyoji batch approve `
  --where "entity=skill" --where "predicate=cost" `
  --by "你的名字" --reason "与原件逐字比对一致"           # 默认只预览，加 --yes 才执行

# 运行场景
go run ./cmd/ssot run projects/onmyoji damage-calc 262 `
  --ref "ratio=skill:262_01" --bind "def_reduction=0.5 fraction"
```

**核验优先级**：`争议 > 影响面（被派生引用次数）> 分级（L4 最需核验）`，
并强制包含随机抽检项——否则人会只核验「显眼」的部分，系统性错误永远发现不了。
抽检用固定种子，**「随机」不等于「不可复现」**。

## 开发

开发规范见 `AGENTS.md`。**规格先行**：每个特性先写 `docs/specs/<feature>.spec.md`，
流程为 `Spec → 验收测试(红) → 端口接口 → 实现(绿) → 重构`。
规格即唯一事实源，**实现变化时先改规格再改代码**。
