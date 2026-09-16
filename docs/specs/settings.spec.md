# settings.spec.md —— 配置页管什么

主题：**哪些东西可以在 App 里配**，以及**哪些故意不给配**。
角色与权限在 `agent.spec.md`；聊天界面在 `agent.spec.md` §7；接入形状在 `dsh.spec.md`。

## 已确认的约定

### 1. 只管我们自己的三层（用户已定）

| 分组 | 配什么 | 为什么是我们的事 |
|---|---|---|
| **项目** | `projects/` 根目录、要打开哪个 vault（列表来自 `Discover`，只列含 `project.yml` 的目录） | 这是我们自己的概念（`workspace.spec.md`） |
| **Agent 后端** | DSH 的入口路径、profile 名、`actor` 名字（如 `agent:dsh`）、ssot 可执行文件路径 | 后端命令是我们拼的，必须能改；换机器/换安装位置就靠这里 |
| **外观** | 主题（浅/深/跟随系统） | 界面归属我们 |

### 2. **不管**模型与 API key（用户已定）

模型选择与推理强度属于**会话**，不属于 App：

- 它们来自 ACP `session/new` 返回的 `configOptions`，用 `session/set_config_option` 改
  （`agent.spec.md` §7），所以它们出现在 **Agent 面板里**，不是配置页。
- key/凭据留在 DSH 自己的配置与凭据存储里。我们**不复制一层**——复制一定会跟它打架
  （它下次启动按自己的配置覆盖，用户会以为我们写坏了）。

### 3. 配置存哪：**用户级一份，不进仓库**

`settings.json` 放系统配置目录（Go 的 `os.UserConfigDir()` 下的 `ssot/`），理由：

- 里面有**本机路径**（DSH 装在哪、ssot 可执行文件在哪），换机器不该带着走；
- 与 `.gitignore` 里那批「个人本地覆盖」同一性质（`AGENTS.local.md` 也在这条线上）；
- **不放 `project.yml`**：项目文件是 vault 的一部分、会进 vault 自己的 git，
  「DSH 装在哪」这种本机事实写进去会污染项目数据。

⚠️ 配置**坏掉时不许静默用默认值**：读不动就明确报「配置文件读不了，用的是默认值 + 路径」，
让人知道要去哪改。这条与「未核验必须可见」是同一个立场。

### 4. 配置页也是**检查后端的地方**

后端不是我们能保证存在的东西（DSH 可能没装、`acp` profile 可能没建）。
所以配置页要有一个**明确的检查**：把「DSH 入口在不在、profile 目录在不在、
ssot 可执行文件在不在」逐条列出来，缺哪条就说缺哪条、怎么补。
不做「一键自动装」——装 DSH 不是我们的活，静默改用户 home 下的东西更不行。

## 不做

- 不做模型 / API key 的配置（见 §2）。
- 不做 DSH 全量配置的镜像界面（我们会一直落后于它，且两边写同一个文件）。
- 不做「自动创建 profile / 自动安装 DSH」：那是改用户 home 下的东西，必须是人明确做的动作。
- 不做多后端并存（一次一个，`agent.spec.md` §7）。

## 怎么验证

- 配置读写：改一项 → 存 → 重启 App 仍在；文件损坏 → 明确报错而不是静默吞掉。
- 后端检查：把 DSH 路径指向一个不存在的目录 → 配置页逐条报缺什么（不崩、不假装可用）。
- 与本机真实情况对照：DSH 装在
  `%LOCALAPPDATA%\Programs\DSH Desktop`，harness 入口是
  `resources\harness-node-entry.mjs`，`dsh` 入口是
  `resources\app\node_modules\@deepseek-ai\dsh\lib\bin.js`（`dsh.spec.md` 里记着它们的来路）。
