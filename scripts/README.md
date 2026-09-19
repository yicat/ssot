# scripts/ —— 脚本单独放这里

一次性或重复用的小工具都放这儿，**不要散落在各处**。每个脚本都要有一段说明，
写在文件顶部注释里，包含三件事：

1. **做什么**：一句话
2. **适用范围**：什么时候用它、在什么前提下成立
3. **什么时候不该用**：边界在哪，什么时候它会给错的结果

## 目录

| 子目录 | 放什么 |
|---|---|
| `check/` | 只读探针：把看不见的东西变成文本证据（见 `check/README.md`） |
| `ingest/` | 导入：外部数据 → vault（**会写文件**，见 `ingest/README.md`） |
| `dsh/` | 起 DSH 会话：把 MCP overlay 拼进参数、用 Electron 当 node（见 `dsh/README.md`） |

需要时再按用途分子目录（例如 `scripts/fetch/`），子目录里放一个 `README.md` 说明这一组是干什么的。

## 运行约定（本机环境）

- **要联网抓 HTTPS 的脚本用 Node，不要用 curl / PowerShell**（Windows 上 curl/.NET 走 schannel，
  报 `SEC_E_NO_CREDENTIALS`；Node 走 OpenSSL 可以直接用）
- Go 相关脚本依赖本机模块缓存：`GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod`
- 需要构造缓存的命令记得带 `$env:GOCACHE="<仓库>\.gocache"`（受限沙箱下否则会被拒）

更多环境坑见根 `AGENTS.md`。
