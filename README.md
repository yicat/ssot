# ssot

单一事实源（Single Source of Truth）工具。Go + Wails v3 + Vite/React/shadcn。

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
wails3 task build
```

> `task` 无需单独安装——Wails v3 自带 `wails3 task`。

## 结构

```
├─ cmd/ssot/                # CLI 入口（MVP 阶段用它验证机制）
├─ main.go                  # Wails 桌面应用入口
├─ internal/
│  ├─ domain/               # 领域层：值/量纲/元模型/schema/校验/表达式/断言
│  ├─ application/          # 用例：接入、准入、公式、派生、场景
│  ├─ infrastructure/       # 适配器：SQLite 断言库、YAML 加载、原件读取
│  └─ api/                  # wails3 bindings
├─ projects/onmyoji/        # 一个项目 = 一个领域
└─ docs/
   ├─ specs/                # 规格（唯一事实源）
   ├─ notes/                # 调研与实测记录
   └─ mvp-scope.md          # MVP 实施范围
```

## CLI（MVP）

```powershell
# 校验项目的 schema 与单位表
go run ./cmd/ssot schema projects/onmyoji

# 接入 + 准入 + 原子应用（数据来自本地已存档的原件）
go run ./cmd/ssot sync projects/onmyoji

# 按公式派生 L3 断言（会先验证公式的算例）
go run ./cmd/ssot derive projects/onmyoji crit_factor

# 查看断言库状态
go run ./cmd/ssot status projects/onmyoji

# 核验队列（按优先级排序，强制含抽检项）
go run ./cmd/ssot review projects/onmyoji queue --limit 20 --sample 0.05

# 冲突成组呈现（系统不裁决）
go run ./cmd/ssot review projects/onmyoji conflicts

# 批量核验（默认只预览，加 --yes 才执行）
go run ./cmd/ssot review projects/onmyoji batch approve `
  --where "entity=skill" --where "predicate=cost" `
  --by "你的名字" --reason "与原件逐字比对一致"

# 单条核验
go run ./cmd/ssot review projects/onmyoji approve <断言ID> --by "你的名字" --reason "..."

# 运行场景
go run ./cmd/ssot run projects/onmyoji damage-calc 262 `
  --ref "ratio=skill:262_01" --bind "def_reduction=0.5 fraction"
```

**核验优先级**说明：`争议 > 影响面（被派生引用次数）> 分级（L4 最需核验）`，
并强制包含随机抽检项——否则人会只核验「显眼」的部分，系统性错误永远发现不了。
抽检用固定种子，**「随机」不等于「不可复现」**，出问题能重放同一批。

开发规范见 `AGENTS.md`，规格见 `docs/specs/`。

## 核验工作台（GUI）

数据由程序接入与准入，**人只在核验环节介入**。

```powershell
wails3 task dev            # 开发模式（热重载）
go build -o bin/ssot-gui.exe . && ./bin/ssot-gui.exe --project projects/onmyoji
```

界面遵循三条规格要求：

- **一切默认未核验**——队列里每一条都带着状态与分级，未核验不会被藏起来
- **核验必须能回答「谁、何时、凭什么」**——批准者与理由为必填，且批准者必须是人
- **溯源可见**——每条断言都能看到来自哪个原件、哪个位置、哪个修订
