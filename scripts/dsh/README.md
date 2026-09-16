# scripts/dsh/ —— 把能力层接到 DSH 上的启动脚本

这一组只管**怎么起**：把仓库里的 overlay 传给 DSH，让某一次会话带上 `mcp__ssot__*` 工具。
接入的**形状与理由**在 `docs/specs/dsh.spec.md`；角色与权限在 `docs/specs/agent.spec.md`。

| 脚本 | 一句话 | 依赖 |
|---|---|---|
| `dsh-ssot.ps1` | 起一个会话级 DSH：设好环境变量 + `--patch .dsh\mcp.patch.yml`，不碰你的全局 profile | DSH Desktop、Go（首次会编译 `bin\ssot.exe`） |

```powershell
pwsh -File scripts\dsh\dsh-ssot.ps1                    # 用 projects\demo
pwsh -File scripts\dsh\dsh-ssot.ps1 -Vault D:\myvault   # 指定 vault
pwsh -File scripts\dsh\dsh-ssot.ps1 -DumpConfig         # 只打印组装结果（自检，零副作用）
```

## 为什么要脚本，不能直接 `dsh`

- 本机**没有独立的 `node.exe`**：DSH 桌面版是拿 Electron 当 node 跑 harness
  （`DSH Desktop.exe --expose-internals <harness-node-entry.mjs> <dsh/lib/bin.js> web …`）。
  不设 `ELECTRON_RUN_AS_NODE=1`，子进程会当成 Electron 应用启动，参数错位后只报
  「--profile <name> is required」——那个报错完全看不出真实原因。
- overlay 里的 `command` / `args` 用 `!!js process.env.…` 取动态值，所以要有人先把
  `SSOT_MCP_BIN` 与 `SSOT_MCP_VAULT` 设好；脚本还负责在源码比二进制新时重新编译。

## 什么时候不该用

- **别拿它跑日常写代码的会话**：那些会话不需要 vault 工具，多出来的工具只会占上下文。
- **别把 overlay 抄进 `~/.dsh/profiles/web/cordis.patch.yml`**：那份是全局的，会污染所有会话
  （理由写在 `docs/specs/dsh.spec.md` §1）。
- 桌面 GUI 里加不了这个 overlay（桌面版写死只叠加它自己的 patch），
  所以想要「带 ssot 工具的界面」只能用这个脚本起的那个地址。

## 卡住时先看这三处

1. `-DumpConfig` 里有没有 `# == <仓库>\.dsh\mcp.patch.yml` 那一段——没有就是 overlay 没被读到。
2. harness 日志里 `patch: entry "mcp-ssot" not found` → 新增插件行必须包在 `insert:` 里。
3. `unknown tag !<tag:yaml.org,2002:js>` → `!!js` 写到了序列上（它只能用在**标量**上）。
4. `unknown option '--dump-config'` 或 `config dumps take no app arguments` →
   启动器自己的选项要排在 `--host` / `--port` **之前**，而且 dump 模式**不收任何 app 参数**
   （脚本里已经按这个顺序拼好了，改脚本时别调换）。
