# ssot

单一事实源（Single Source of Truth）工具：一个**以文档为中心**的本地事实库。
把会过期、会互相抄、无法验证的来源，变成可追溯、可复用的文档与数据表——
**agent 可以提出与取证，批准必须是人**。

技术栈：Go + Wails v3 + Vite / React / shadcn，构建编排走 Taskfile（`wails3 task ...`）。

> 旧方案（六部件 + 断言库 + 核验流程）已整体作废，实现留在分支 `legacy/mvp-v1` 备查，
> 本文与 `docs/` 都不沿用它的分层与部件。

## 现在到哪了

| 层 | 状态 |
|---|---|
| **能力层**（CLI + MCP） | 能用：vault 读写、块级双链、状态机（只有人能发布）、只读 SQL、写入即 git 提交、删除/恢复/统计、收录范围声明 |
| **界面** | 能用：三栏文档浏览器（`文档` / `数据表` / `Agent`）+ 配置页（含后端逐条检查）+ 会话面板 |
| **Agent** | 能用：ACP 后端（我们的 `ssot-agent` profile）+ 会话级挂 MCP + 四个角色 skill |
| **派生层**（水面下，`.data/index.db`） | **P0–P3 完成**：切块 → 嵌入（自研 WordPiece + ONNX）→ 混合检索 → 抽取入库。P4 图检索、P5 增量接界面**未做** |

进度台账（做到哪、下一步、卡在哪）在 `docs/STATUS.md`；问题清单在 `docs/OPEN.md`。

## 快速开始

```powershell
# 前端依赖（首次）
cd frontend; npm install

# 开发运行（热重载）——会先编 CLI：App 起 MCP 服务器用的就是它
wails3 task dev

# 服务模式（浏览器打开 http://localhost:8080）
wails3 task run:server

# 测试与检查
wails3 task test          # go test ./... + 前端 vitest
wails3 task check         # go vet + 全部测试 + 前端构建（提交前跑）

# 构建
wails3 task build         # 产物 bin/ssot.exe（GUI）
wails3 task build:cli     # 产物 bin/ssot-cli.exe（CLI，同时是 MCP 服务端）

# CLI 用法
bin\ssot-cli.exe help
bin\ssot-cli.exe vault -root projects/demo list
```

> `task` 无需单独安装——Wails v3 自带 `wails3 task`。
> ⚠️ `bin/ssot.exe`（GUI）与 `bin/ssot-cli.exe`（CLI / MCP）**不能同名**，理由见 `AGENTS.md`。

## 目录

```
├─ main.go              Wails 桌面应用入口（无边框 + 自绘标题栏，见 docs/specs/shell.spec.md）
├─ cmd/ssot/            CLI 入口，同时是 MCP 服务端（`ssot mcp`）
├─ internal/
│  ├─ api/              接口层：wails3 bindings（project / vault / agent）
│  ├─ application/      用例层：vaultapp（读写/检索/删除/统计/范围/抽取接线）
│  │                             agentapp（起后端/开会话/发话/流）
│  ├─ domain/vault/     领域层（只 stdlib）：文档、双链、状态机、切块、排序、查询门
│  ├─ infrastructure/   外部适配：vaultfs · vaultgit · vaultindex(SQLite) · vembed(ONNX)
│  │                             vextract(抽取) · acp(后端协议) · appconfig · projectfile
│  │                             scopefile · sessionstore · dshstore
│  ├─ mcp/              MCP 服务端（stdio），工具表在 tools.go
│  └─ compose/          组合根：唯一允许同时依赖各层的包
├─ frontend/
│  ├─ src/components/ui/       shadcn 生成，勿手改
│  ├─ src/components/custom/   自研业务组件（AppTitleBar / DocTree / VaultBrowser /
│  │                           SearchModal / SettingsModal / AgentPane）
│  ├─ src/pages/               页面 = 纯编排，不写交互逻辑
│  └─ bindings/                wails3 generate bindings 生成，勿手改
├─ projects/            项目根目录（**整段 gitignore**）：一个项目 = 一个含 project.yml 的目录，
│                       每个项目各自是独立 git 仓库（见 docs/specs/workspace.spec.md §7）
├─ docs/                specs（已确认的规范）/ adr（决策）/ plans（方案）/ notes（实验）
│                       + STATUS.md（进度）/ OPEN.md（问题）
├─ .dsh/                接给 DSH 的东西：mcp.patch.yml（会话级 overlay）+ skills/ 四个角色
└─ scripts/             脚本：check/（只读探针）· ingest/（外部数据 → vault）· dsh/（启动）
```

一个项目 = 一个含 `project.yml` 的目录，目录里长这样（`projects/demo` 是真实数据的示范库）：

```yaml
# projects/<名字>/project.yml
project: 项目名
description: 一句话说明
```

```
projects/<名字>/
├─ raw/        原始层：抓来什么样就什么样（进 vault 自己的 git）
├─ docs/       整理层：整理好的 markdown（人和 agent 都能改）
├─ tables/     数据表：csv / json / yaml，一表一文件
└─ .data/      派生层：index.db，可重建、不进 git、不进人眼
```

## 想知道什么，看哪

| 想知道 | 看 |
|---|---|
| 整体方案：水面上下、三层结构、数据怎么流、不做什么 | `docs/ARCHITECTURE.md` |
| **我们确认过的事实与规范**（改代码前先改这里） | `docs/specs/` |
| 做过哪些决定、为什么、**否掉了什么** | `docs/adr/` |
| 技术方案怎么实现 | `docs/plans/` |
| 实验记录：数字、复现方式、局限 | `docs/notes/` |
| 现在做到哪、下一步、卡在哪 | `docs/STATUS.md` |
| 有哪些问题（阻塞 / 待定 / 观察） | `docs/OPEN.md` |
| 脚本做什么、什么时候不该用 | `scripts/README.md` 与各脚本文件头 |

## 沿用的约定

1. **分层铁律**：`api → application → domain ← infrastructure`；`domain` 只允许 Go 标准库；
   组合根 `compose` 是唯一允许同时依赖各层的包（见 `internal/AGENTS.md`）。
2. **前端落位**：`components/ui/` 与 `bindings/` 不手改；业务组件落 `components/custom/<Name>/`
   （`index.tsx` + `useXxx.ts` + `store.ts`）；`pages/` 只做编排（见 `frontend/AGENTS.md`）。
3. **改动 Go 侧导出后要重新生成 bindings**：
   `wails3 generate bindings -ts -d frontend/bindings`。
4. **提交前跑 `wails3 task check`**。

环境与已知坑（TLS、代理、沙箱）见 `AGENTS.md`。
