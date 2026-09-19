# AGENTS.md

## 最高优先级：与人协作的六条

> 这一节**优先于本文其余所有内容**。与下面的任何约定冲突时，以这一节为准。

1. **任务不清晰就先问清楚，不要猜着开工。** 不确定的都要问；**提问必须带上下文**——
   说清发生了什么、我看到了什么、卡在哪一步。**不要自己造概念和词语**，
   用我们已经确认过的说法。我们是平等的交流，一切以我们确认过的事实为准。
2. **不要过度思考。** 出错是正常的，不必为了不出错而反复推演。
   要善于利用工具、整理工具、编写工具；好用的工具**写成文档记下来，并写明它的适用范围**。
3. **想不明白、反复出错的，不要钻牛角尖。** 停下来问——但提问之前先看看**提问的艺术**：
   这个问题值不值得问、够不够具体、对方要回答它需要哪些信息。**问题要有质量**，
   不要把「我没想清楚」直接丢过去。
4. **尽量带着有效的方案来讨论。** 不用搜得特别多；必要时可以去 GitHub 找找最佳实践
   （找的是可参照的做法，不是照抄）。
5. **凡是要改到代码的，先讨论、形成 spec，再动手。** 已经定下的规范**不得擅自更改**，
   要改先提出来一起定。
6. **我们确认过的事实放进 spec 管理起来**；脚本单独找个地方放，
   每个脚本写明**做什么、适用范围、什么时候不该用**。

## 构建与测试

- 开发运行：`wails3 task dev`（会**先编 CLI**：App 起 MCP 服务器用的就是它）
- CLI 单独：`wails3 task build:cli` → `bin/ssot-cli.exe`
  ⚠️ **两个二进制不能同名**：`bin/ssot.exe` 是 Wails 的 GUI（`main.go`），
  `bin/ssot-cli.exe` 是 CLI（`cmd/ssot`，同时也是 MCP 服务端）。写成同一个路径会互相覆盖——
  症状是「界面起不来」或「MCP 工具调的是一份老代码」，都很难查（踩过）。
- 构建：`wails3 task build`；服务模式：`wails3 task build:server` / `wails3 task run:server`
- 测试：`wails3 task test`（= `go test ./...` + 前端 `npm run test`（vitest））
- 界面测试（要应用带 `SSOT_WEBVIEW_DEBUG_PORT=9222` 跑着）：
  - `node scripts/check/ui-test.mjs`：文档渲染、文件树、检索——**内容依赖当前打开的 vault**
  - `node scripts/check/agent-ui-test.mjs`：聊天界面与配置页——**不依赖 vault 内容**
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
├─ projects/                # 项目根目录（当前为空，未提交）：一个项目 = 一个含 project.yml 的目录
├─ docs/specs/              # **我们确认过的事实与规范**（改代码前先改这里；见「开发流程」）
└─ scripts/                 # 脚本单独放这里；每个脚本写明做什么、适用范围、什么时候不该用
```

## 分层铁律

1. 依赖方向：`api → application → domain ← infrastructure`，禁止反向
2. `domain` 只允许 Go 标准库，禁止 import 本仓库其他层
3. 新业务组件必须落在 `components/custom/<Name>/`；页面不做交互逻辑
4. 生成目录（`components/ui/`、`frontend/bindings/`）不手改
5. 动 `internal/` 之前先想清落位（换掉外部系统后还需要吗？需要 → `domain`）

## 开发流程

**讨论 → 形成 spec → 改代码**，顺序不颠倒：

1. 事情不清楚，先问清楚（见「最高优先级」第 1 条）
2. 凡是要改到代码的，先在 `docs/specs/` 里把**我们确认过的事实与规范**写下来，一起定
3. spec 定了才动代码；发现实现与 spec 不一致时**先改 spec**，不许绕过去改实现
4. 已定下的规范要改，先提出来讨论，**不擅自改**
5. 过程中确认的新事实、新约定，回头补进对应的 spec（放在 `docs/specs/`）

> 旧流程（固定五段模板 + `Spec → 验收测试(红) → 领域端口 → 实现(绿) → 重构`）随旧方案作废。
> 新方案定下来后，再补这里的细节（要不要固定模板、要不要先写失败测试等）。

## 提问的格式

提问前先过一遍「提问的艺术」：**这个问题值不值得问、够不够具体、对方回答它需要哪些信息**。
带上下文的提问包含四样（不必逐字照抄）：

1. **发生了什么**：我在做什么、进行到哪一步
2. **我看到的**：具体证据——报错原文、文件路径、我实际跑的命令与输出
3. **我卡在哪**：具体到哪一句、哪个决定
4. **需要你定什么**：给出选项，以及我的倾向（若有）

反例：「这个怎么办？」「好像有问题吧？」——没有上下文的问题只会换来一轮来回。

## 技术栈

Go + Wails v3 + Vite/React/shadcn。构建编排走 Taskfile（`wails3 task ...`）。

## 环境与已知坑

### 脚本与文本处理

- **改源码只用编辑工具**（`read` → `edit`/`write`）。**不要**用 PowerShell 脚本去 patch 源码：
  在 Windows 上会被 CRLF（锚点对不上）、反引号（被当转义吃掉）、`-replace`/`.Replace` 的重载
  这些坑咬住，而且报错往往看不出真正原因（实测一个下午翻车四次）。
- **写脚本就进 `scripts/`**，并在文件头写清「做什么 / 适用范围 / 什么时候不该用」（见本文开头第 6 条）。
  临时文件里的一次性补丁脚本**不算脚本**，它既没出处也不可复用。
- **运行时优先 Deno**（本机 2.8.2）或 Node（v24.9，仓库现有 `scripts/check/*.mjs` 都是它）；
  TypeScript / 正则 / JSON / 文件遍历这类活别用 PowerShell 干。
- PowerShell 只用来**跑命令**：构建、测试、git、只读探查（`netstat`、`Get-Process` 这类）。

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

### 读 DSH 的 `--dump-config`（后端工具集怎么验）

1. **必须显式给 profile**：App 的后端是 `ssot-agent`，而 `scripts\dsh\dsh-ssot.ps1` 起的是 `web`——
   照抄脚本的 `-DumpConfig` 验的是**写代码那个会话**，不是后端。
2. **`--dump-config` 只能写在 app 名之前**：写在 `acp` 之后会被 acp 当未知选项拒掉
   （`web` 那侧恰好容忍，所以看起来「通用」，其实不是）。
3. **输出必须走管道**：`DSH Desktop.exe` 是 GUI 子系统进程，`> dump.yml` 会得到 **0 字节**。
   管道 + `Out-String` 才拿得到 892 行那种完整 dump。
4. **别用正则「上一个 `id:` 配下一个 `disabled:`」**：`disabled: true` 排在
   `__dshPluginOwner:` 块之后，配对必然错位（我因此写出过假清单）。
   **按条目边界切**（每个 `- id:` 到下一个 `- id:` 之间）。

完整命令与实测结果见 `docs/specs/dsh.spec.md`「能验到哪一步」。

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
  现在是**四个角色**（见 `docs/specs/agent.spec.md` §6）：`vault-organize`（整理）、
  `vault-find`（查找）、`vault-verify`（核验）、`vault-rewrite`（优化重写）。
  skill 名必须 kebab-case（发现规则限制），中文写在 `description` 与正文里
- **把能力层接给 agent**：MCP 服务端是 `ssot mcp`（stdio），overlay 在 `.dsh/mcp.patch.yml`，
  启动用 `pwsh -File scripts\dsh\dsh-ssot.ps1`。**不许写进全局 profile**
  （`~/.dsh/profiles/web/cordis.patch.yml`）——那会污染写代码的会话。形状与理由见 `docs/specs/dsh.spec.md`
- ⚠️ **不要**在 `.dsh/skills/` 下放 `README.md`：平铺 `<name>.md` 也会被当成 skill 发现，会产生幽灵条目
- skill 的 `description` 是**唯一路由键**（模型目录不渲染 `whenToUse`），务必写清"什么时候用它"
- 已加载的 skill 正文**没有大小上限**，保持精简
- 分层/契约类改动先走 plan mode，用 `exit_plan_mode` 出完整方案再动手
- 个人本地覆盖写 `AGENTS.local.md`（已在 `.gitignore`），不要改 `AGENTS.md`
- 子目录局部规则写在各自的 `AGENTS.md`（`internal/`、`frontend/` 已有）
