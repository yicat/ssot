# internal/

> 动手写代码之前先看根 `AGENTS.md`：**要改到代码的，先讨论、形成 spec 再动手**；
> 已定下的规范不得擅自更改。

## 分层铁律

依赖方向：`api → application → domain ← infrastructure`，**禁止反向**。

- `domain/`：纯业务规则，**只允许 Go 标准库**，禁止 import 本仓库其他层
- `application/`：用例编排（先 A 后 B、失败回滚这类流程）
- `infrastructure/`：外部适配（存储/设备/日志），实现 `domain` 定义的端口
- `api/`：暴露给前端的 wails3 绑定层，只做参数转换与调用转发，不写业务规则

## 落位判断

问自己：**这段逻辑换掉外部系统后还需要吗？** 需要 → `domain`。

## 动这一层之前

先确认落位（见上）。当前各层的实际落位（改动前先看这一行，别照旧印象找）：

| 层 | 包 |
|---|---|
| `domain/` | `domain/vault`（文档、双链、状态机、切块、排序、`querygate`） |
| `application/` | `vaultapp`（读写/检索/删除/统计/范围/抽取接线）· `agentapp`（起后端/会话/流） |
| `infrastructure/` | `vaultfs` · `vaultgit` · `vaultindex`(SQLite) · `vembed`(ONNX) · `vextract` · `acp` · `appconfig` · `projectfile` · `scopefile` · `sessionstore` · `dshstore` |
| `api/` | `project` · `vault` · `agent` 三个 wails 服务 |
| （不在 `internal/` 下的适配） | `internal/mcp`（stdio MCP 服务端）、`cmd/ssot`（CLI） |

`domain/vault` 是唯一有「只 stdlib」这条硬约束的包；别的包按需引依赖，但方向不许反。
`derivedapp`（派生层专用用例包）**还没建**——现在重建/嵌入/抽取的编排落在 `vaultapp`。

依赖检查（纯手工，旧的 `layering-guard` skill 已随旧方案删除）：

```powershell
# domain 是否混入了非标准库依赖或反向引用
go list -deps ./internal/domain/... | Select-String -NotMatch '^(internal/|vendor/|errors|fmt|sort|strings|strconv|time|math|regexp|encoding/|unicode|cmp|slices|maps|iter|sync|context|os|path|io|bufio|bytes|hash|log|net|reflect|runtime|testing|gopkg.in/yaml.v3|github.com/ngnl5/ssot)'
```

## 提交前

`go vet ./...` + `wails3 task test` 通过。
