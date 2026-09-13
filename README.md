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

# 运行场景
go run ./cmd/ssot run projects/onmyoji damage-calc 262 `
  --bind "ratio=80 percent" --bind "def_reduction=0.5 fraction"
```

开发规范见 `AGENTS.md`，规格见 `docs/specs/`。
