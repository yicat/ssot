# 0005 接 DSH 用会话级 overlay，不写全局 profile

状态：已定
关联：`docs/specs/dsh.spec.md` §1/§6、`docs/specs/agent.spec.md` §7、`.dsh/mcp.patch.yml`、`scripts/dsh/`

## 背景

要让 agent 能用我们的能力层（MCP），就得让 DSH 知道这个 MCP 服务器。能配的地方有三处：

1. profile 的用户 patch：`~/.dsh/profiles/<profile>/cordis.patch.yml`（**全局**）；
2. 启动参数 `--patch <file>`（每次会话叠加，可重复）；
3. ACP 的 `session/new { mcpServers: [...] }`（**按会话挂**，我们自己起后端时用这条）。

## 决策

- **命令行/GUI 场景**：把 MCP 条目放仓库内 `.dsh/mcp.patch.yml`，启动时 `--patch` 带上；
- **App 内置聊天场景**：走 `session/new` 的 `mcpServers`（连 overlay 都不需要）；
- **绝不写进全局 profile**。

## 为什么

写全局会让**每一次** DSH 会话（包括用户写代码那些）都多出我们的工具，白吃 token，
还有被误调的风险。会话级叠加则「谁需要谁带上」。

## 否掉的选项

- **全局 profile**：见上（用户明确担心过「跟写代码的 dsh 冲突」）。
- **让桌面 GUI 用户自己加**：桌面版起 harness 时**写死**只叠加它自己的 `dsh-desktop.patch.yml`
  （在 `out/main/index.js` 的 `buildHarnessArguments` 里读到的），**没有给用户加 patch 的位置**——
  所以 GUI 这条路走不通，只能另起一个会话（`scripts/dsh/dsh-ssot.ps1`）。
- **streamable-http**：要多管端口、生命周期与鉴权，现在没这需求。

## 后果

- 需要一个自定义 profile（`ssot-agent`）与一份 overlay；两者都要能自检（见配置页的「后端检查」）。
- 踩过的坑写在 `dsh.spec.md` §6：新增插件行必须包在 `insert:` 里；`!!js` 只能用在**标量**上；
  启动器自己的选项要排在 app 参数之前。
