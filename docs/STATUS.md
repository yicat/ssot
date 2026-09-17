# STATUS.md —— 现在做到哪、下一步、卡在哪

> 这份文件的**唯一职责**是让「进度」不再靠翻对话。规范在 `docs/specs/`，方案在 `docs/plans/`，
> 决策在 `docs/adr/`，问题在 `docs/OPEN.md`。**每次收尾（见 ADR 0013）都更新这里。**

**上次整理：2026-09-17（建立治理体系：13 条 ADR + STATUS/OPEN/ARCHITECTURE；派生层 P0 起步）**

## 整理台账

| 日期 | 范围 | 结论 | 关联 |
|---|---|---|---|
| 2026-09-17 | 建立治理体系；补录已发生的决策 | 决策/进度/问题/反例从「散在各文件边角」改为各自有家 | ADR 0001–0013 |

## 现在在哪

**已完成**
- 能力层（CLI + MCP）：vault 读写、双链、状态机、SQLite 查询、git 留痕（写入即 commit）
- 界面：三栏文档浏览器（文档 / 数据表 / Agent 三个模式）+ 配置页（含后端逐条检查）
- Agent 接入：ACP 后端（`ssot-agent` profile）+ 会话级 MCP 挂载 + 四个角色 skill
- 派生层**方案定稿**：`docs/specs/derived.spec.md` + `docs/plans/derived-layer.md`
- 实测：嵌入选型（B）、召回基线、抽取成本（关推理 + 收工具集）
- **P0 第一步**：`internal/domain/vault/chunk.go`（切块 + 行号区间 + token 估算）+ 6 条测试

**进行中**
- **P0 第二步**：`chunk` / `sync` 表进 `vaultindex` 并在 `Rebuild` 时填充（含增量用 hash）

**下一步（按 plan 的顺序）**
1. P0 收尾（上面那条）
2. **P1**：`internal/infrastructure/vembed` 嵌入 + `embedding` 表 + 向量检索（与 transformers.js 逐位对拍）
3. **P2**：FTS + 向量混合检索 ← **不花额度，却是分水岭**（纯向量在实体名式查询上 R@1 只有 21.6%）
4. P3：抽取（`vextract`）→ 需先解 `docs/OPEN.md` 的 3 个阻塞项
5. P4：图检索并入（local/global/hybrid/mix）
6. P5：增量与索引状态接到界面

## 卡在哪

- **3 个阻塞项挡着 P3**（抽取）：纠正块语法、抽取触发方式、`agent.spec` 工具清单更新 —— 见 `docs/OPEN.md`。
  **不挡 P0–P2**。
- 待用户决策：ADR 0013（定期整理）状态还是「提议」。

## 最近一次验证（都是跑出来的）

```
go test ./internal/...                 ok（含 chunk 6 条、appconfig 6 条、acp 6 条、mcp、vaultapp）
wails3 task check                      exit 0
scripts/check/mcp-smoke.mjs            31/31
scripts/check/agent-ui-test.mjs        22/22
scripts/check/ui-chrome-test.mjs       8/8
scripts/check/agent-e2e.mjs（可选）     从界面起后端 → deepseek-harness-acp + 会话 + 9 个 MCP 工具
```

⚠️ **已知红**：`scripts/check/ui-test.mjs`（46 条）**依赖旧示例 vault 的内容**（`语法示例`/`罗生门`/第 47 行任务），
那个 vault 现已挪到 `.tmp/backup-demo`，所以它对当前 vault 是红的。**不是界面坏了**，
修法两条：加一个随仓库提交的 fixture vault 并让套件切过去，或把套件改成不依赖内容（见 `docs/OPEN.md`）。
