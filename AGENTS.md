# AGENTS.md

## 构建与测试

- 开发运行：`wails3 task dev`
- 构建：`wails3 task build`；服务模式：`wails3 task build:server` / `wails3 task run:server`
- 测试：`wails3 task test`（= `go test ./...` + 前端 `npm run test`（vitest））
- **提交前全量检查**：`wails3 task check`（= `go vet ./...` + 全部测试 + 前端构建）
- 前端单独：`cd frontend && npm run dev` / `npm run build`

## 目录约定

```
├─ main.go                  # 入口（Wails 桌面应用）
├─ cmd/ssot/                # CLI 入口（MVP 用它验证机制）
├─ internal/
│  ├─ compose/              # 组合根：唯一允许同时依赖各层的包
│  ├─ domain/               # 领域层：纯规则，只允许标准库
│  │  ├─ value/             # 值的三态（有值/未知/空/缺失）
│  │  ├─ unit/              # 量纲与换算
│  │  ├─ metamodel/         # 类型与约束原语
│  │  ├─ schema/            # 实体集合与双向漂移检测
│  │  ├─ validate/          # 六类校验
│  │  ├─ expr/              # 表达式求值器（含量纲检查）
│  │  ├─ assertion/         # 断言模型与变更集（含解析方式→分级上限）
│  │  ├─ verification/      # 核验记录：四级方法、参与者、责任归属
│  │  └─ decision/          # 待判定：歧义事项、候选与裁决规则
│  ├─ application/          # 应用层：用例编排
│  │  ├─ ingest/            # 接入：原件 → 候选
│  │  ├─ admit/             # 准入：候选 → 变更集
│  │  ├─ formula/           # 公式加载、算例验证与求值
│  │  ├─ derive/            # 派生：由断言产出 L3 断言
│  │  ├─ review/            # 核验编排：优先级、冲突、批量
│  │  ├─ disambig/          # 待判定编排：歧义登记、排队、裁决
│  │  └─ scenario/          # 场景：requires 检查与运行
│  ├─ infrastructure/       # 基础设施：外部适配
│  │  ├─ store/             # SQLite 断言库、核验记录与待判定事项（原子应用）
│  │  ├─ schemafile/        # YAML schema 与单位表加载
│  │  └─ artifact/          # 原件存档读取
│  └─ api/                  # 接口层：核验工作台后端（wails3 bindings）
├─ projects/onmyoji/        # 一个项目 = 一个领域（阴阳师）
│  ├─ project.yml
│  ├─ units.yml
│  ├─ schema/               # 定义：可版本控制、可分享
│  ├─ formulas/             # 公式与算例
│  ├─ scenarios/            # 场景：一个用途一个目录
│  └─ .data/                # 实例：断言库（gitignore，可重建）
├─ frontend/
│  ├─ src/components/ui/        # shadcn 生成，勿手改
│  ├─ src/components/custom/    # 自研业务组件：index.tsx + useXxx.ts + store.ts
│  ├─ src/pages/                # 页面 = 纯编排，无交互逻辑
│  └─ bindings/                 # wails3 generate bindings 生成，勿手改
├─ tools/wiki/              # 灰机 wiki 数据同步与体检（Node，需可见 Chrome）
├─ docs/
│  ├─ specs/                # 规格文档（SDD 唯一事实源）
│  └─ notes/                # 调研与实测记录（非规范，可能过期）
└─ .huiji/                  # wiki 抓取缓存与凭据（gitignore，勿提交）
```

## 分层铁律

1. 依赖方向：`api → application → domain ← infrastructure`，禁止反向
2. `domain` 只允许 Go 标准库，禁止 import 本仓库其他层
3. 新业务组件必须落在 `components/custom/<Name>/`；页面不做交互逻辑
4. 生成目录（`components/ui/`、`frontend/bindings/`）不手改
5. 动 `internal/` 之前加载 `layering-guard` skill

## SDD 流程（本项目开发规范）

Spec 先行：每个特性先写 `docs/specs/<feature>.spec.md`（固定模板：目的 / 领域规则 Given-When-Then / 验收标准 / 边界与异常 / 前后端契约），流程为 `Spec → 验收测试(红) → 领域端口接口 → 实现(绿) → 重构`。规格即唯一事实源，实现变化时**先改规格再改代码**。写 spec 前加载 `writing-spec` skill。

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

### 灰机 wiki 抓取

该站是 Cloudflare **指纹门**（不下发 `cf_clearance`），cookie 复用无效，
唯一可行通道是**可见窗口 Chrome + CDP**。详见 `docs/notes/wiki-data-source.md` 与 `tools/wiki/README.md`。

## dsh 协作约定

- **项目 skill** 放 `.dsh/skills/<name>/SKILL.md`，随仓库提交、团队共享。已建：`writing-spec`、`layering-guard`
- ⚠️ **不要**在 `.dsh/skills/` 下放 `README.md`：平铺 `<name>.md` 也会被当成 skill 发现，会产生幽灵条目
- skill 的 `description` 是**唯一路由键**（模型目录不渲染 `whenToUse`），务必写清"什么时候用它"
- 已加载的 skill 正文**没有大小上限**，保持精简
- 分层/契约类改动先走 plan mode，用 `exit_plan_mode` 出完整方案再动手
- 个人本地覆盖写 `AGENTS.local.md`（已在 `.gitignore`），不要改 `AGENTS.md`
- 子目录局部规则写在各自的 `AGENTS.md`（`internal/`、`frontend/`、`docs/specs/` 已有）
