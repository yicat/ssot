# AGENTS.md

## 构建与测试

- 开发运行：`wails3 task dev`
- 构建：`wails3 task build`；服务模式：`wails3 task build:server` / `wails3 task run:server`
- 测试：`wails3 task test`（= `go test ./...` + 前端 `npm run test`（vitest））
- **提交前全量检查**：`wails3 task check`（= `go vet ./...` + 全部测试 + 前端构建）
- 前端单独：`cd frontend && npm run dev` / `npm run build`

## 目录约定

> ⚠️ **当前是骨架，方案重写中。** 上一套方案（六部件 + 断言库 + 核验流程）已整体作废并
> **全部清除**：业务代码、规格集、示例项目数据、抓取脚本、调研笔记与项目 skill 都已删掉
> （作废实现在分支 `legacy/mvp-v1` 备查）。下面这棵树是**现在真实存在的**；
> 新方案的结构定下来再补，不要照抄旧树。

```
├─ main.go                  # 入口（Wails 桌面应用）
├─ cmd/ssot/                # CLI 入口（当前只有 help；新命令按用例层加）
├─ internal/
│  ├─ compose/              # 组合根：唯一允许同时依赖各层的包（会话 + 项目装配）
│  ├─ api/                  # 接口层：wails3 bindings（当前只有项目列表与切换）
│  └─ infrastructure/       # 外部适配（当前只有 projectfile：project.yml 与项目发现）
├─ frontend/
│  ├─ src/components/ui/        # shadcn 生成，勿手改
│  ├─ src/components/custom/    # 自研业务组件：index.tsx + useXxx.ts + store.ts（新方案按此落位）
│  ├─ src/pages/                # 页面 = 纯编排，无交互逻辑
│  └─ bindings/                 # wails3 generate bindings 生成，勿手改
└─ projects/                # 项目根目录（当前为空，未提交）：一个项目 = 一个含 project.yml 的目录
```

## 分层铁律

1. 依赖方向：`api → application → domain ← infrastructure`，禁止反向
2. `domain` 只允许 Go 标准库，禁止 import 本仓库其他层
3. 新业务组件必须落在 `components/custom/<Name>/`；页面不做交互逻辑
4. 生成目录（`components/ui/`、`frontend/bindings/`）不手改
5. 动 `internal/` 之前先想清落位（换掉外部系统后还需要吗？需要 → `domain`）

## 开发流程

**流程本身也待重定**：旧的「Spec 先行」（`docs/specs/<feature>.spec.md` 固定五段模板 +
`Spec → 验收测试(红) → 领域端口 → 实现(绿) → 重构`）随旧方案一起作废，
对应的 `writing-spec` skill 已删。新方案定下来后在这里写清新流程。

## 技术栈

Go + Wails v3 + Vite/React/shadcn。构建编排走 Taskfile（`wails3 task ...`）。

## 环境与已知坑

### 网络与 TLS

1. **抓 HTTPS 必须用 Node，不能用 curl / PowerShell**。Windows 上 `curl` / `Invoke-WebRequest` / `.NET`
   都走 schannel，报 `schannel: AcquireCredentialsHandle failed: SEC_E_NO_CREDENTIALS`；
   Node 走 OpenSSL，同一机器同一网络直接可用。

2. **`web_fetch` 拒绝所有域名是正常的**。本机 Clash 的 fake-ip 把域名解析到 `198.18.1.x`，
   dsh 的 `web_fetch` 会以「解析到非公网 IP」拒绝——那是 SSRF 防护，**不代表网络不通**。

3. **判断网络/代理状态必须用 `netstat -ano`**。本环境下 `Get-NetAdapter` 与 `Get-NetTCPConnection`
   返回空，会把人误导成「没有网卡、没有代理」。实测 `verge-mihomo` 监听 `0.0.0.0:7897`（代理）与 `0.0.0.0:53`（DNS）。

4. **出网是通的，但包管理器源不可达**，因此：
   - Go 依赖走本机模块缓存：`GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod`
   - 前端依赖走 npm 缓存：`npm ci --offline`（registry 已配置 npmmirror）
   - `wails3 init` 不可用（需联网拉模板），骨架是从既有项目复制的

### 受限文件沙箱下的三处失败

当前会话文件策略为 `danger-full-access`，不受限；但在 `read-only` / `workspace-write` 下：

| 现象 | 原因 | 对策 |
|---|---|---|
| `go vet`/`test`/`tidy` 报 `Access is denied` | 构建缓存在工作区外 | 设 `$env:GOCACHE="<仓库>\.gocache"` |
| 前端构建 EPERM | Vite 用 Node `child_process` 管道起子进程 | 需放宽沙箱 |
| Chrome 启动即 FATAL | Mojo IPC 需创建命名管道 | 需放宽沙箱 |

⚠️ 注意 `go build` 可能因命中旧缓存而**侥幸成功**，不要因此误判沙箱没问题。

### 构建

5. **改动 Go 侧导出后要重新生成 bindings**：`wails3 generate bindings -ts -d frontend/bindings`，
   否则前端构建报 `[plugin wails-typed-events] Event bindings module not found`。

> `go` 会打印 `error acquiring upload token ... Access is denied` 遥测警告，是无害噪音。

## dsh 协作约定

- **项目 skill** 放 `.dsh/skills/<name>/SKILL.md`，随仓库提交、团队共享。
  当前**一个都没有**：旧的 `writing-spec`、`layering-guard` 随旧方案一起删了
- ⚠️ **不要**在 `.dsh/skills/` 下放 `README.md`：平铺 `<name>.md` 也会被当成 skill 发现，会产生幽灵条目
- skill 的 `description` 是**唯一路由键**（模型目录不渲染 `whenToUse`），务必写清"什么时候用它"
- 已加载的 skill 正文**没有大小上限**，保持精简
- 分层/契约类改动先走 plan mode，用 `exit_plan_mode` 出完整方案再动手
- 个人本地覆盖写 `AGENTS.local.md`（已在 `.gitignore`），不要改 `AGENTS.md`
- 子目录局部规则写在各自的 `AGENTS.md`（`internal/`、`frontend/` 已有）
