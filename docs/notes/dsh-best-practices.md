# DeepSeek Harness (dsh) 最佳实践 — 写代码与项目

> 版本基准：本机安装的官方包 `@deepseek-ai/*` **0.1.5-rc.2**（npx 缓存目录）。
> 来源说明：本文所有「机制类」结论均已对照本机官方包 README 与随附 preset / skill 源码核实；
> 社区文章的**正文**因当前网络策略（web_fetch 被 DNS 拦截）无法抓取，仅按标题/URL 收录在文末资源区，
> 未对其内容做背书。机制部分以官方包为准，社区部分仅供参考。

---

## 0. 十条速览

1. **三层指令链**：`~/.dsh/AGENTS.md`（全局）→ 项目根 `AGENTS.md` → 嵌套 `packages/x/AGENTS.md`。越具体优先级越高。
2. **机器本地规则用 `AGENTS.local.md`**（overlay，进 `.gitignore`），而不是改 `AGENTS.md`。
3. **能力写成 Skill**：`.dsh/skills/<name>/SKILL.md`（项目，rank 100）或 `~/.dsh/skills/<name>/SKILL.md`（用户，rank 400）。
4. **Skill 只做「按需加载的流程知识」**，`description` 写清「什么时候用」，模型才选得中。
5. **选 preset**：`standard`（默认全功能）/ `ptc`（`run_code` 编排，省往返）/ `cordis`（造插件）/ `minimal`（单 shell）。
6. **preset 只能在空会话切换**——先定模式再开聊。
7. **非平凡改动先走 plan mode**，用 `exit_plan_mode` 交付决策完整的计划，再动代码。
8. **长任务挂 goal**，别用一轮对话硬扛；跨轮自动续跑。
9. **委派有讲究**：`subagent`（干净上下文、能选模型）/ `subagent_fork`（继承历史、吃 KV cache）/ `workflow`（扇出编排）/ `ralph`（全新 agent 迭代）。
10. **上下文要主动管**：`/compact` + tool-result pruner（>8192 字符结果自动裁头尾），大文件别整份读进上下文。

---

## 1. 指令文件：AGENTS.md 三层链

### 机制（已核实）

- 注入顺序：用户全局 `$DSH_HOME/AGENTS.md` → 项目指令链（项目根到 cwd，逐目录，宽泛→具体）。
- 候选文件名默认 `['AGENTS.md', 'CLAUDE.md']`，本地 overlay `['AGENTS.local.md', 'CLAUDE.local.md']`。
- **项目根 = 最近的含 `.git` 的祖先目录**；不存在则用 cwd。
- 同目录内容相同的同级候选只渲染一次（`CLAUDE.md` 复制 `AGENTS.md` 不会重复占用预算）。
- 预算 `maxBytes: 65536`（默认）：**先整份丢弃宽泛文件，最后才截断最具体的文件**，并发出 `Workspace instruction budget ...` 通知。
- **没有 watcher**：嵌套目录的指令靠成功的 `read` / `write` / `edit` 触碰（touch）才被发现；
  `cd` 到别的目录（shell 导航）**不会**触发发现。
- 用户全局 scope **没有** `.local.md` overlay；项目 scope 才有。
- 超预算的宽泛文件在刷新期被视为「暂时不可用」，不会误报为「已移除」。

### 实践

```text
~/.dsh/AGENTS.md                 # 跨项目铁律：语言、提交规范、禁区
<repo>/AGENTS.md                 # 项目地图、构建/测试命令、架构约束
<repo>/AGENTS.local.md           # 我的机器专属路径/端口/密钥引用（gitignore）
<repo>/packages/api/AGENTS.md    # 该子包特有约定（被 read/write 触达后自动加载）
```

- **「命令」写进 AGENTS.md 收益最高**：构建、测试、lint、跑单测的精确命令，模型不用猜。
- 子目录规则放子目录，别全塞根文件——预算被宽泛文件吃掉时，**先牺牲的正是根文件**。
- 根 `AGENTS.md` 保持精简（建议 < 4–8 KB），把细节下沉到子目录或 Skill。
- 写「不要做什么」比写「要做什么」更省 token：一句 `禁止改 package-lock.json` 抵一段解释。

### AGENTS.md 模板（可直接抄）

```markdown
# <项目名>

## 技术栈
<语言/框架/包管理器/运行时版本>

## 常用命令
- 安装：`pnpm install`
- 开发：`pnpm dev`
- 测试：`pnpm test -- <path>`
- 单测：`pnpm vitest run src/foo.test.ts`
- 类型检查：`pnpm tsc --noEmit`
- Lint/格式化：`pnpm lint --fix`

## 架构
- `apps/` 可部署入口；`packages/` 库；`packages/core` 不得依赖 `apps/`
- 数据流：<一句话说明>

## 约定
- 提交信息：Conventional Commits，标题 ≤ 72 字符
- 新代码必须有测试；bug 修复先补失败测试
- 不引入新依赖前先问

## 禁区
- 不改 `package-lock.json` / 生成目录 `dist/` `gen/`
- 不在源码里写死密钥
```

---

## 2. Skills：把「怎么做事」资产化

### 机制（已核实）

- 两种形态：目录 bundle `<root>/<name>/SKILL.md`，或平铺 `<root>/<name>.md`。
- **发现只看一层**：嵌套 `**/SKILL.md` 刻意不支持；bundle 内 `references/`、`scripts/`、`assets/` 是资源，不参与发现。
- frontmatter：必填 `name`（**kebab-case**）与 `description`；可选 `whenToUse`、`metadata`、`disable-model-invocation`、`user-invocable`。
  - 布尔值支持 `true/false`、`yes/no`、`on/off`、`1/0`；**拼错会让整个 skill 被静默丢弃**（只在宿主警告里可见）。
- 扫描根与优先级：

  | Rank | 来源 | 路径 |
  |---|---|---|
  | 100 | project-dsh | `<projectRoot>/.dsh/skills` |
  | 200 | project-agents | `<projectRoot>/.agents/skills` |
  | 300 | custom | `Config.customSkillDirs` |
  | 400 | user-dsh | `<dshHome>/skills` |
  | 500 | user-agents | `<agentsHome>/skills` |

- **热更新**：新增/改名/删除 skill 或改 frontmatter → 下一个模型步骤刷新目录；
  但**只改正文不刷新目录**（正文每次加载都重新读取，所以正文编辑无需重启）。
- 目录与正文分离：正文按需加载 → **description 就是唯一的检索键**。

### 路由机制（决定你怎么写）

- 模型在**首次请求前**收到一条持久目录消息（`<available_skills>`），只有 `name` + 有长度上限的 `description`。
- **`whenToUse` 不参与路由**——它是提供方元数据，目录不渲染它。想被选中，**信息必须写进 `description`**。
- 描述上限 `catalogDescriptionMaxLength` 默认 **500** 字符（可配，最小 3）；超了会被规范化截断。
- ⚠️ **已加载的正文没有大小上限**——一个胖 skill 足以吃掉下一步的上下文。自己写时保持精简。
- 目录变更（增删/改名/改描述/可见性）会**追加一份全量替换目录**，token 成本与目录规模成正比 → skill 别建太多太啰嗦。
- 同名 skill 按提供方/根 rank 合并，项目级（100/200）先于用户级（400/500）。
- 用户可直接用 `/name` 调用 skill，把相同指令注入当步。

### 实践

- 项目 skill 提交进仓库（`.dsh/skills/`），团队共享；个人 skill 放 `~/.dsh/skills/`。
- `description` 用「**做 X 时用它**」句式，包含触发词（技术名、文件类型、任务动词），并把原本想写进 `whenToUse` 的条件**挪进 description**。
- 一个 skill 一个流程：`SKILL.md` 正文写清 步骤 → 检查项 → 常见失败；长资料放 `references/`，脚本放 `scripts/`。
- 需要用户手动触发的放 `user-invocable`；纯模型自动路由的保持默认。
- 想让它**只在特定场景**被模型看到：`disable-model-invocation: true` 让它只走用户命令。

### skill 从哪来

| 途径 | 位置 / 方式 | 适用 |
|---|---|---|
| 自己写（项目） | `<repo>/.dsh/skills/<name>/SKILL.md` | 团队共享，随仓库版本化 |
| 自己写（个人） | `~/.dsh/skills/<name>/SKILL.md` | 跨项目个人习惯 |
| 跨工具共享 | `<repo>/.agents/skills/`、`~/.agents/skills/` | **官方刻意扫描的兼容根**，同一批 skill 给别的 agent 工具复用 |
| 随附插件 / preset | preset 私有 `skills/`（如 `cordis` preset 的 2 个） | 只在该 preset 的会话里可见 |
| 第三方 / 社区 | 装成插件或复制进上面的根目录 | 见文末资源区 |

### SKILL.md 模板

```markdown
---
name: repo-migration
description: 当需要新增数据库迁移、修改表结构、或修复迁移冲突时使用。覆盖本仓库的迁移命名、回滚与上线顺序。
metadata:
  owner: platform-team
---

# 数据库迁移流程

## 步骤
1. 读 `packages/db/schema/*.ts`，确认改动落在哪张表
2. `pnpm db:gen --name <snake_case>` 生成迁移
3. 检查生成的 SQL：是否有锁表风险、是否补了索引
4. `pnpm db:migrate` 本地验证，再 `pnpm db:rollback` 验证可回滚
5. 更新 `docs/schema.md`

## 检查项
- [ ] 大表加列有默认值且非 null 时不阻塞？
- [ ] 迁移可回滚，且回滚脚本已测试？
- [ ] 是否影响 `packages/api` 的查询类型？

## 常见失败
- 忘记同步 `schema.md` → CI 的 docs 检查会失败
- 直接改历史迁移文件 → 拒绝，必须新增迁移
```

---

## 3. Preset：先选模式，再开新会话

### 官方随附四种（已核实）

| id | 名称 | 说明 |
|---|---|---|
| `standard` | 标准模式 | 全功能：文件编辑、Shell、检索、Skills、计划、目标、子代理、工作流 |
| `ptc` | PTC 模式 | 同上，但**不提供 `workflow` 工具**，其余工具通过 PTC SDK 呈现，模型用**一个 TypeScript 程序**组合多步操作 |
| `cordis` | 创造模式 | standard + 运行时检查、插件实验、preset 创作指导 |
| `minimal` | 极简模式 | 只有持久 shell 的单工具 agent |

**PTC 模式的本质**：把「5 次往返」压成「1 段程序」——即社区那篇《换了个模式，性能提升 40%》讲的东西在官方架构里的位置（`dsh-agent-tool-presentation` 的 `mode: ptc`）。

### 选定规则

- **只有空会话能切 preset**（没有消息、没有工具调用）。中途换工具会让已记录的工具调用无法执行。
- 选择是会话级的：切换会写入 `agent-preset/selected` 事件，恢复/fork 后按当时的组装重建。
- 子 agent 加入父方的组装 → 父子工具集一致。

### 自建 preset

- 用户的 preset 放 `~/.dsh/.agent-presets/<id>/`，id 必须匹配 `[a-z0-9][a-z0-9-]*`。
- **创作 = 复制**：把随附 preset 整个目录复制过来改，不手写组装文本。
- 一个 preset 就是一个 `agent.cordis.yml` 的插件行列表；每行给工具、提示词段落、skill。
- ⚠️ 你在 preset 里写的每一行都在授予能力——**把自写 preset 当受信任配置对待**。
- ⚠️ 已知限制：复制是快照，升级 dsh **不会**更新你的副本；「standard 加一处改动」的 patch 语义**不存在**，只能整份复制后编辑。

### 默认值配置

```yaml
# ~/.dsh/settings.yaml
agent-presets:
  default: standard
```

---

## 4. 会话内工作流：从需求到交付

这套顺序与 dsh 的提示词机制吻合（plan mode 明确要求「先用非变更手段勘察」）：

1. **勘察**：`grep` / `glob` / `read` 定位真实代码，别问用户「代码在哪」。
2. **计划**（非平凡改动）：plan mode → `exit_plan_mode` 提交**决策完整**的计划
   （目标与验收标准、按子系统分组、公共 API/schema/数据流变化、边界与失败模式、测试与假设）。
   - plan mode 下**工具目录刻意保持不变**（为请求缓存稳定），所以工具还在，只是不许用。
   - 计划要「另一个工程师不需要再做设计决定就能写」。
3. **拆解**：`todo_write` 一次性写全清单，每步开工前标 `in_progress`，完成立刻标 `completed`。
4. **实现**：改文件前先 `read`（fs-observation 策略要求）；用 `edit` 做定点改动，别整文件重写。
5. **验证**：跑测试/类型检查/lint。**非零退出必须查清**再继续。
   - 长命令：`run_in_background` 起后台任务，别阻塞；用 `job_output` 收结果。
   - Windows 上被强杀的命令会以 `[exit code: 1]` 无信号标记出现——按「被中断」而不是「命令失败」理解。
6. **交付**：产物文件用 `present` 声明，正文里用反引号写路径。

**并行**：真正独立的步骤在**同一条 assistant 消息里**发多个工具调用；有依赖才串行。

---

## 5. 委派与规模化

| 场景 | 用什么 | 关键点 |
|---|---|---|
| 独立、自包含的子任务 | `subagent` | 默认后台；子 agent **看不到**本对话 → 提示词必须自带全部上下文 |
| 建立在当前对话上的分析/评审/续写 | `subagent_fork` | 继承已完成轮次；父子同模型 → 历史可复用 KV cache |
| 一次性扇出几十个独立单元 | `workflow` | 写 JS 编排脚本：`pipeline`（无栅栏，首选）/ `parallel`（栅栏）/ `agent()`；**只有用户明确要工作流时才用** |
| 同一目标、跨轮迭代、每轮全新 agent | `ralph` | **仅当用户明说要 Ralph**；工作区当长期记忆 |
| 单会话长目标（跨轮自动续跑） | goal 工具 | `create_goal` 建目标；`complete` 只在真正达成时；`blocked` 需同一阻塞连续 ≥3 轮 |
| 用户自己该拍的板 | `ask_user_question` | 不要拿「代码在哪 / 现在怎么实现的」去问——那些自己能查 |

**官方默认值（standard preset，已核实）**：
- `subagent`：`provider: spawn`、`backgroundMode: continuable`、`modelSelectionSettings: true`（子代理可另选模型/推理档）。
- `subagent_fork`：`provider: fork`、`backgroundMode: continuable`，**刻意不给模型选择**（保持 provider/model 与父一致，历史才够格吃 KV cache）。
- `tool-ralph`：`maxRounds: 64`。
- `workflow` 在 `ptc` preset 中被**显式禁用**（`run_code` 已占据「模型自编编排面」这个位置，避免两个编排入口并存）。

**实践**：
- 委派是**省上下文**手段，不是省时间手段：把「读 20 个文件写摘要」这类噪音活丢出去。
- fork 适合「继续我这条线」，spawn 适合「另起一条线」。
- 委派出去的验证**不是独立评估**——关键结论自己复核。

---

## 6. 上下文与成本控制

已核实的默认装配（standard preset）：

```yaml
- id: compaction-basic                    # 自动压缩
- id: command-compact                     # /compact 手动压缩
- id: tool-result-pruner
  config: { thresholdChars: 8192, headChars: 4096, tailChars: 1024 }
- id: agent-instructions
  config: { maxBytes: 65536 }
```

**KV cache 友好的设计（都是官方刻意的）**：
- AGENTS.md 注入**只追加**（追加在可复用前缀之后），改动指令文件不会让既有前缀失效——但会追加一条完整的替代基线。
- plan mode **不改变工具目录**，正是为了请求缓存稳定。
- preset 组装**只在 agent 发布前装载一次**，整个生命周期前缀稳定。
- 子代理目录刷新会追加「替换目录」，正文编辑不改变目录 digest。

**实践**：
- 大输出别整份进上下文：用 `head`/`Select-String`/脚本先过滤，或落盘再按需 `read offset/limit`。
- 超过 8192 字符的工具结果会被自动裁成「头 4096 + 尾 1024」——**关键信息别放在中段**。
- 长会话主动 `/compact`，别等自动压缩把细节丢了。
- 会话内纠偏用一句话，别重述上下文；需要长期权威信息就写进 `AGENTS.md`（下一轮才生效，不是即刻）。

---

## 7. 项目脚手架建议

```text
<repo>/
├─ AGENTS.md                  # 项目地图 + 命令 + 禁区（提交）
├─ AGENTS.local.md            # 本机专属（.gitignore）
├─ .dsh/
│  └─ skills/
│     ├─ repo-migration/SKILL.md
│     ├─ release/SKILL.md
│     └─ debugging-ci.md           # 平铺写法同样可用
├─ .agents/skills/            # 兼容其他 agent 工具的同一批 skill（rank 200）
└─ packages/api/AGENTS.md     # 子包约定，被触达后自动加载
```

`.gitignore` 追加：

```gitignore
AGENTS.local.md
CLAUDE.local.md
```

**逐步落地顺序（收益从高到低）**：
1. 项目根 `AGENTS.md` 写「构建/测试/lint 命令 + 禁区」。
2. 把重复解释过两次以上的流程写成 skill。
3. 子包/子目录各写 3–10 行 `AGENTS.md`。
4. 个人偏好放 `~/.dsh/AGENTS.md`，不污染仓库。
5. 需要固定工具组合时，才复制一个 preset 出来改。

---

## 8. 写 dsh 插件（cordis）

`cordis` preset 随附 `cordis-plugin-development` 与 `editing-cordis-compositions` 两个官方 skill。核心纪律：

**标准流程**：`cordis_inspect_list` → `cordis_inspect_query`（按需查精确签名）→ `cordis_define` → `cordis_run` → 看 Run 卡片/diagnostics 修 → `cordis_stop`（临时停）/ `cordis_undefine`（永久删）。

**平台选择**：文件/命令/进程/网络/agent/会话数据 → Host；主题/布局/页面状态/设置页/侧栏/工具卡片 → Client；「Host 取数、Client 显示」→ 两边 + `harness.handle` / `host.call`。

**铁律**：
- **绝不从 Service 名、事件负载、Slot props 猜 API**——先 inspect。
- `code.host` / `code.client` 是**纯 JavaScript 函数体**，不经 TS/JSX/打包器：不能用 `import`、`require`、类型注解、JSX；React 用 `React.createElement`。
- 可选依赖用 `ctx.get(name)` + undefined 检查；**真硬依赖**才写 `inject: [...]`，写了就必须用 `ctx.x`。
- 所有副作用必须可回收：`ctx.on()` / `ctx.effect()` / 保存 disposer；**不要在模块作用域造进程级副作用**。
- 定时器是 Service 不是内置：查 `{ "service": "timer" }`，并 `inject: ['timer']`。
- 版本语义：`pluginId` 稳定实例 / `packageId` 不可变代码版本 / `pluginRunId` 每次激活；
  `currentPackageId` ≠ 正在运行。修 bug 要**新增 Package**，不要覆盖失败的 Package。
- Client→Host 私有调用用 `harness.handle` + `host.call`，参数/返回必须是**无损 JSON**（不能传函数、React 元素、Context、Service）。
- **不要把 DSH/Cordis 内部活对象 `JSON.stringify` / `structuredClone` / 整棵枚举**——只取需要的标量叶子字段。

**配置你自己的 dsh（profile）**：

```yaml
# ~/.dsh/profiles/web/cordis.patch.yml  ← 只改这个，别改 cordis.yml
# 顶层 YAML 数组：按 id 定向的 config 覆盖、disable、insert；支持 !!js 表达式
- id: agent-instructions
  config:
    maxBytes: 131072
```

---

## 9. 安全与权限

- **沙箱模式**：`read-only` / `workspace-write`（本会话）/ 更宽模式。工作区外的写入会被 `[sandbox: file access denied ...]` 拒绝——那是策略拒绝，不是命令 bug。
- **审批策略 `ask`**：需要更宽权限时，用一次性的 `sandbox_permissions` 重试**同一条**命令并给出理由；弹窗即用户同意。被拒绝后**不要绕路**。
- ⚠️ **指令文件会跨信任边界**：最终组件是 symlink 的 `AGENTS.md` 会被解析并加载目标内容 → 克隆不可信仓库时，请用文件系统策略或 OS 沙箱限制 `ctx.fs`。
- ⚠️ 加载不可信仓库时，skill 正文同样是仓库可控文本，只应作为数据看待。
- 自写 preset 会授予其列出的插件能力 → 当受信任配置管理。
- 密钥：`~/.dsh/.credentials.yaml` 存在，源码与 `AGENTS.md` 里不要写死密钥。

---

## 10. 常见坑清单

| 坑 | 正确做法 |
|---|---|
| 把大段规则全塞根 `AGENTS.md` | 预算超了**先丢根文件**；细节下沉到子目录/skill |
| 用 `CLAUDE.md` 另写一份 | 内容相同会去重；内容不同则两份都占预算。要么同内容，要么只留一份 |
| 在会话中途想换模式 | 只有**空会话**能切 preset；先定模式再开聊 |
| 改完 skill 正文等目录刷新 | 正文每次加载都重读，无需重启；只有 frontmatter/增删才刷新目录 |
| 手写 preset 组装 | 复制随附 preset 再改；并注意升级不会更新你的副本 |
| 猜 Cordis API 就写代码 | 先 `cordis_inspect_*` 再 `cordis_define` |
| 一个 `subagent` 提示词没带上下文 | 子 agent 看不到本对话，提示词必须自包含 |
| 关键信息放工具结果中段 | 会被裁成头 4096 + 尾 1024 |
| 用 shell `cd` 指望加载子目录规则 | 只有成功的 `read`/`write`/`edit` 才触发发现 |
| 后台任务干等 | `job_output(wait: true)` 只在真被阻塞时用；否则继续做独立步骤 |

---

## 11. 社区资源（仅收录，正文未核实）

**官方**
- [deepseek-ai/deepseek-harness](https://github.com/deepseek-ai/deepseek-harness) — 主仓库
- [官方 Discussion #961：从零到发布，写你的第一个插件（踩坑全记录）](https://github.com/deepseek-ai/deepseek-harness/discussions/961)
- [官方 Discussion #316：长时间任务 npx 后台崩溃、WebUI 假装活着](https://github.com/deepseek-ai/deepseek-harness/discussions/316)

**手册/教程**
- [Electricitysheep/dsh-handbook](https://github.com/Electricitysheep/dsh-handbook) — 从 0 到 1 深度手册：安装/插件开发/性能调优/实测（中英 PDF）
- [sandbaseai/deepseek-harness-handbook](https://github.com/sandbaseai/deepseek-harness-handbook) — `agents-md-scope.md`、`skills.md` 等 agent-patterns 文档
- [warmsum/deepseek-harness-python-tutorial](https://github.com/warmsum/deepseek-harness-python-tutorial)
- [DeepSeek Harness 设计解析：六个关键决策](https://cloud.tencent.cn/developer/article/2728774)
- [cpolar：把 AI 训练成会干活的智能体（保姆级教程）](https://www.cpolar.com/blog/deepseek-harness-nanny-level-tutorial-transform-ai-from-only-chatting-to-a-capable-intelligent-agent)

**生态/插件**
- [fendouai/awesome-deepseek-harness](https://github.com/fendouai/awesome-deepseek-harness)
- [awesome-deepseekharness/awesome-deepseek-harness](https://github.com/awesome-deepseekharness/awesome-deepseek-harness) — plugins / tools / skills 合集
- [NanmiCoder/dsh-agent-teams](https://github.com/NanmiCoder/dsh-agent-teams) — 多代理 + 官方风格 skill（含 `dsh-plugin-development/SKILL.md`）
- [xiagaogaozi/dsh-subagent-pool](https://github.com/xiagaogaozi/dsh-subagent-pool) — 命名子代理池（模型/推理/预设可复用、按名调用）
- [wig123/dsh-thread-tools](https://github.com/wig123/dsh-thread-tools) — 跨会话工具
- [HYBB-rash/dsh-plugins](https://github.com/HYBB-rash/dsh-plugins) — Telegram/调度/X feed 等个人插件
- [Practice019/agent-skills](https://github.com/Practice019/agent-skills)

**PTC / 性能**
- [我给 DeepSeek Harness 换了个模式，性能提升 40%](https://developer.aliyun.com/article/1756859)
- [3 大 DeepSeek Harness 进阶玩法](https://developer.aliyun.com/article/1758406)
- [DeepSeek Harness vs Claude Code（DataCamp）](https://www.datacamp.com/zh/blog/deepseek-harness-vs-claude-code)

**说明**：上表中若干仓库（如多个 `deepseek-harness` 同名 fork、`awesome-*` 重复条目）在搜索结果中并存，
选择时请以 `deepseek-ai/` 官方组织与 star/更新时间为准，谨防抢注或仿冒仓库。
