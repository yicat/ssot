# 0006 后端工具集：读放开、写收口

状态：已定（含一次自我纠正）
关联：`docs/specs/dsh.spec.md`「后端工具集必须收紧」、`internal/infrastructure/appconfig`、`.dsh/extraction.patch.yml`

## 背景

ACP profile 继承 `dsh-base`，里面带着一整套编码 agent 工具：`tool-pwsh`、`tool-bash`、
`tool-fs`、`tool-fs-search`、`tool-str-replace-editor`。留着它们，agent 能直接改 vault 文件、
直接 `git commit`、直接改 front matter 里的 `status` —— 0003 那道门就白设了。

## 决策

**只有「能写」的能力必须走能力层；读放开。**

| 工具行 | 处理 | 理由 |
|---|---|---|
| `tool-pwsh` / `tool-bash` | 禁用 | 能跑任意命令 → 能改文件、能 commit |
| `tool-fs` | 禁用 | `read` 与 `write`/`edit` 绑在同一个插件里，没法只要读 → 读由我们自己的 `file_read` 补 |
| `tool-str-replace-editor` | 禁用 | 同上（`view` 是读，`create`/`str_replace` 是写） |
| `tool-fs-search`（glob/grep） | **保留** | 纯只读（走打包的 ripgrep，写不了一个字节） |
| MCP 的九个工具 | 保留 | 这就是能力层 |
| `tool-skill`、`tool-subagent`、`tool-todo`、`tool-web` | 保留 | 角色分发与调度；不碰文件 |

## 为什么

第一版我**一刀切把读也砍了**（连 `fs-search` 一起禁用），结果 agent 连找文件都不会——
「理论上门更严」换来的是「agent 不好用」。**砍掉读不增加安全性，只损失能力**。

## 否掉的选项

- **全禁（含 fs-search）**：见上，已纠正。
- **全留（含 pwsh/fs）**：0003 的门被绕过，不可接受。
- **给 `tool-fs` 加只读配置**：该插件没有这个开关（读到的 README 里只有 `readLimit` 之类的
  读限制，没有关写的能力），所以只能在「整块关」与「整块留」之间选。

## 后果

- 能力层要提供**只读**的文件读取：`file_read`（按行分页、限定 vault 内、拒绝绝对路径与 `..`、
  非 UTF-8 明确报错）。
- 抽取/后台任务同样跑在受限工具集上（0011）。
- 每次调用的「工具定义」体积也小了：实测从 7,863 → 5,220 token（`notes/extraction-spike.md`）。
