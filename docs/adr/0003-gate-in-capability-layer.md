# 0003 审批门在能力层：agent 不能发布

状态：已定
关联：`docs/specs/agent.spec.md` §1/§2、`docs/specs/vault.spec.md` §2、`internal/domain/vault/doc.go`

## 背景

这套东西的立场是「**agent 可以提出与取证，批准必须是人**」。问题是这道门放在哪一层：
放在 agent 后端（DSH 侧）里，还是放在我们自己的能力层里。

## 决策

**门在能力层**：每个写操作都带 `actor`（`human` / `agent:<名字>`），

- `actor=agent` 时：允许写 `docs/`（产生新版本），但 `status` **强制回落 `draft`**；
- **不许**把 `status` 改成 `published` / `archived`——只有人能改。

落地在 `domain/vault` 的 `Actor.CanChangeStatus()` 与 `StatusAfterEdit()`，
由 `application/vaultapp` 在写入路径上强制执行。

## 为什么

**后端是可替换的**（见 0005 与 `agent.spec.md` 定位）。门放在后端，换一个后端就绕过去了；
放在能力层，任何后端（DSH、别的 ACP 客户端、将来的 OpenAI 兼容 API）都绕不过。

## 否掉的选项

- **放在 agent 后端 / 靠提示词约束 agent「不要发布」**：提示词是软的，模型换一版就可能不遵守；
  而且这等于把「批准必须是人」交给被管的对象去执行。
- **不做门，靠人看 diff 追认**：默认一切可信，与「未核验可见」的立场相反。

## 后果

- 能力层必须拿到 `actor`；CLI 与 MCP 都是显式参数（MCP 侧恒为 `agent:<名字>`，见 0006）。
- 失败信息必须说清原因（「只有人能发布或归档」），而不是含糊的权限错误。

## 实测过的边界

- `go test ./internal/application/...` 里有断言：agent 调 `SetStatus(published)` 必须失败，
  且**不产生提交**、文件不被改动。
