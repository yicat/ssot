# workspace.spec.md —— 项目根目录与项目发现

主题：**什么算一个项目、怎么被找到**。落地处是 `internal/infrastructure/projectfile/`。

> ⚠️ 主题怎么切、要不要固定模板还没定（见 `README.md`），这份沿用 `shell.spec.md` 的三段写法，
> **不宣布这是模板**。

## 已确认的约定

### 1. 项目根目录

- 默认 `projects`，来自 `main.go` 的 `-projects` flag（可覆盖；另有 `-project` 指定初始项目）。
- 项目根目录下**只扫一层**：`root/*/project.yml`，**不递归**。

### 2. 一个项目 = 一个含 `project.yml` 的目录

判定只认这个文件（`projectfile.Name = "project.yml"`）。以下都**不是**项目：

| 形态 | 结论 |
|---|---|
| 空目录 | 不是项目 |
| 有别的文件（如 `units.yml`）但没有 `project.yml` | 不是项目（半成品） |
| 根目录下混着普通文件（如 `README.md`） | 不是项目 |

验收：`projectfile_test.go:21` `TestDiscoverOnlyListsRealProjects`。

### 3. 根目录不存在 ≠ 出错

- `root` 不存在时 `Discover` **返回空列表而不是错误**：第一次使用时一个项目都没有，
  那是「一个都没有」，不是「出错了」。
- 验收：`projectfile_test.go:49` `TestDiscoverMissingRootIsEmptyNotError`。
- 但**需要「当前项目」时**一个都没有是明确报错（`internal/compose/compose_session.go:59-62`），
  界面据此显示空状态——因果链见 `shell.spec.md`。

### 4. 有 `project.yml` 却读不出来：必须报错，不得静默跳过

- 缺 `project` 名 → 拒绝（`projectfile.Load`）。
- YAML 解析失败 → 报错。
- 理由：这不是「不是项目」，是**配置写错了**。静默跳过会让人以为项目不存在，去别处找问题。
- 验收：`projectfile_test.go:61` `TestDiscoverReportsBrokenProjectFile`、`:69` `TestLoadRequiresProjectName`。

### 5. 项目名必填，不用目录名凑

- `project:` 为空则整个项目被拒绝：没有名字的项目在界面上无法区分，
  而「拿目录名凑一个」会让 `project.yml` 里的名字与界面显示悄悄不一致。
- 注：`Ref.Display()` 里有一条「退回目录名」的分支，但经 `Load` 的内容不可能缺名，
  那个分支只是防御；正常路径上名字一定来自 `project.yml`。

### 6. 发现按**目录**，不按名字；顺序按目录名排序

- 两个不同目录里的 `project.yml` 可以写同一个 `project` 名，所以 **`ProjectRef` 带完整 `Path`**，
  界面要显示路径才能区分（`internal/api/project.go:19-21`）。
- 结果按目录名排序（`Discover` 末尾的 `sort.Slice`），界面按钮顺序才是稳定的。

### 7. 项目实例数据不进版本控制

- `.gitignore` 里 **`projects/` 整段被忽略**：vault 各自是独立 git 仓库，工具仓库不跟踪它
  （见 `vault.spec.md` §5）。`.data/` 的忽略规则已经包含在这一整段里。
- 由此推出一件事：clone 出来**不会有 `projects/`**（因为它整段不进版本控制，
  不是因为「空目录 git 不提交」），界面必然是空状态——因果链见 `shell.spec.md` §4。

## 未定

1. `project.yml` 还允许哪些字段：现在只有 `project` / `description` / `metamodelVersion`
   （`projectfile.File`），其中 `metamodelVersion` 当前**没有任何代码读它**——留着还是去掉，等新方案定。
2. 是否支持多个项目根目录（`-projects` 传多个）或嵌套子项目。
3. 项目列表要不要缓存：现在每次 `Projects()` 都重新扫盘。

## 怎么验证

```powershell
# 发现规则的四条验收测试
go test ./internal/infrastructure/projectfile/

# 全量（Go + 前端）
wails3 task test
```

界面侧的验证（窗口标题、顶栏、空状态）见 `shell.spec.md`。

## 与别处的分工

- **本文件**：项目**怎么被找到**、什么算项目（发现规则）
- `vault.spec.md`：项目**内部长什么样**（两层文档、状态、双链、表格、git 版本）
- `shell.spec.md`：窗口标题、顶栏、空状态**在界面上**怎么表现
- `typography.spec.md`：字体与字号
