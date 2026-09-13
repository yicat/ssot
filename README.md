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
├─ main.go                  # 入口：Wails 应用装配
├─ internal/
│  ├─ domain/               # 领域层：纯业务规则，只允许标准库
│  ├─ application/          # 应用层：用例编排
│  ├─ api/                  # 接口层：wails3 bindings 暴露给前端
│  └─ infrastructure/       # 基础设施：外部适配
├─ frontend/                # Vite + React + shadcn
└─ docs/specs/              # 规格文档（SDD 唯一事实源）
```

开发规范见 `AGENTS.md`，规格流程见 `docs/specs/`。
