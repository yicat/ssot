# shell.spec.md —— 界面外壳

主题：**页面之外**的那些约定——窗口标题栏、顶栏内容、空状态。

> ⚠️ 主题怎么切、要不要固定模板还没定（见 `README.md`），所以这份按
> 「已确认的约定 / 未定 / 怎么验证」三段写，**不宣布这是模板**。
>
> 引用第三方代码时写 `文件:行` 并注明 Wails 版本（当前 `v3.0.0-beta.16`）；
> 引用本仓库代码时只写符号名，不写行号——行号会随改动过期。

## 已确认的约定

### 1. 只有一条标题栏，是自绘的那条

- **原生标题栏去掉**：`main.go` 里 `WebviewWindowOptions.Frameless = true`。
- 应用名 `SSOT 工作台` 由**自绘标题栏**承担，落位 `frontend/src/components/custom/AppTitleBar/`，
  界面上**只出现一次**。
- `WebviewWindowOptions.Title` **保留**，但它此后只作窗口标识（任务栏、Alt-Tab 的名字），
  不再是可见的那条栏。
- 由来：先是「原生标题栏与应用内顶栏都写 `SSOT 工作台`」→ 去掉应用内那条 →
  再定「原生那条整个去掉，自己画一条」，免得出现两条 titlebar。

### 2. 自绘标题栏的内容（一条，从左到右）

| 位置 | 内容 | 可拖？ |
|---|---|---|
| 左 | 应用名 `SSOT 工作台` | **可拖** |
| 次左 | 当前项目：`名称（目录）`；未打开时 `（未打开）` | 否（普通内容区） |
| 次左 | 项目切换按钮：每个项目一个，尺寸 `xs`；**当前项目** `secondary`（浅灰底）+ 加粗，**其余** `ghost` | 否 |
| 中/右 | 空白填充区（`flex-1`） | **可拖** |
| 右 | 窗口按钮：最小化 / 最大化（最大化时换成还原图标）/ 关闭 | 否（真按钮） |

标题栏里**不放实心重色块**：曾经当前项目用 `variant="default"`，而本主题的 `--primary` 是近黑
（`style.css` 的 `:root`），于是那条 bar 上杵着一个黑按钮、且它与左边「当前项目：…」那行文字说的是同一件事，
视觉重量压在重复信息上。现在当前项只做安静标记（浅灰底 + 加粗）。
尺寸用 `xs` 还有个好处：bar 高度由内容决定，`py-1` + 最高元素 24px = **32px**，与 Windows 标题栏同高
（探针竖向扫描实测：`HTMINIMIZE` 段占 y 6–32）。

### 3. 拖动与窗口按钮走 Windows「非客户区」机制，不手搓

- **可拖区**：元素上标 CSS 变量 `--wails-non-client-region: caption`。前端 runtime
  量出矩形上报给 Go（`@wailsio/runtime/dist/appregion.js`），Go 在 `WM_NCHITTEST` 里回
  `HTCAPTION`（`pkg/application/webview_window_windows_nonclient.go:146-204`）。
  于是**拖动、双击最大化、右键系统菜单、Snap Layouts 都是 Windows 原生行为**，我们一行都不用写。
- **窗口按钮**：分别标 `minimize` / `maximize` / `close`。命中后 Go 会**吞掉原生动作**，
  只把按下/抬起转发给前端（喂按下态样式，见 `webview_window_windows_nonclient.go:116-125`、
  `:268-308`）。所以**动作要我们自己调 runtime**：
  `Window.Minimise()` / `Window.ToggleMaximise()` / `Window.Close()`（`@wailsio/runtime`）。
- **没标变量的元素一律是普通内容区**，照常收点击。所以「当前项目」和项目切换按钮
  不需要任何特判——**别把 `caption` 标到它们的父元素上**（该属性会继承）。
- 前置开关是 `Windows.WebView2CompositionHosting = true`，**不是** `NonClientRegionSupport`：
  前者才打开 `WM_NCHITTEST` 这条路与前端上报（`webview_window_windows.go:1607` 与 `:2664`；
  前端 `appregion.js` 靠注入的 `_wails.flags.nonClientRegionTracking` 决定要不要启动）。
- 边缘缩放走原生 `WM_NCHITTEST`（DPI 感知；最大化/全屏时自动禁用），前端无需代码
  （`webview_window_windows_nonclient.go:206-259`）。
- 圆角与 Aero 阴影默认保留：**不要**设 `DisableFramelessWindowDecorations`。
- **不采用** `--wails-draggable: drag` + JS 拖拽那条路：`@wailsio/runtime/dist/drag.js`
  只在 macOS 处理双击标题栏，Windows 上走那条会丢双击最大化与 Snap Layouts。

### 4. 「一个项目都没有」是空状态，不是出错

- `projects/` 下没有含 `project.yml` 的目录时，后端返回的错误文本包含 `没有可用项目`。
- 界面**必须**把这种情况显示为「还没有项目」的空状态，**不能**显示成错误条。
- 理由：分不清这两件事，人会把「还没建项目」当成工具坏了。
- 出处：`App.tsx` 的 `noProjects` 判断（按错误文本区分）。
- 边界：这是**按错误文本**区分的权宜做法。后端一旦改成返回结构化的「无项目」结果，
  这里要跟着改——现在只是因为骨架期不想为这一个状态引入新契约。

#### 这条链路是怎么走到空状态的（排查时照这个顺序看）

1. `main.go` 的 `-projects` flag 默认值就是项目根目录 `projects`。
2. **`projects/` 默认不存在**：`.gitignore` 里 **`projects/` 整段被忽略**
   （vault 各自是独立 git 仓库，见 `workspace.spec.md` §7），
   所以 clone 出来不会有它——**与「空目录不提交」无关**，是整段不跟踪。
3. `projectfile.Discover`：root 不存在时**返回空列表而不是错误**——「一个都没有」不是「出错了」。
4. 于是 `Projects()` 返回空数组：**标题栏的项目按钮组为空**。
5. 而 `Current()` 走 `Session.Project()`：`dir` 为空且发现 0 个项目时报错
   `projects 下没有可用项目——一个项目就是一个含 project.yml 的目录`。
6. 前端 `App.tsx`：`Projects()` 成功（列表为空）、`Current()` 抛错；
   `error.includes("没有可用项目")` 命中 → 卡片显示「还没有项目」。
   Wails 日志里那条 `ERR Binding call failed: …` 就是第 5 步，**前端已 catch，不是崩溃**。

结论：界面空 = 正常空状态，**不是故障**。放一个 `projects/<名字>/project.yml` 进去就不再是空的。

### 5. 平台口径：只管 Windows

- 界面按 **Windows** 验收。`main.go` 里的 `MacOptions` / `MacWindow`（`MacTitleBarHiddenInset`）
  留在代码里，但**不作为口径**；将来真要支持 macOS 再单独讨论。
- 因此第 1、3 条**在 Windows 上成立即可**。

### 6. 光标：内容按钮手型，窗口按钮与可拖区保持箭头

- **内容里的按钮给手型**（`cursor: pointer`），表示「这东西能点」。
  做法是全局一条规则补在 `style.css` 的 `@layer base` 里：
  Tailwind v4 的 preflight 把 v3 有的 `button { cursor: pointer }` 去掉了
  （v4 里按钮只剩 `appearance: button`），所以是我们主动补回来的。
- **例外：窗口按钮（最小化/最大化/关闭）保持箭头**，用 `cursor-default` 显式开例外。
  理由：Windows 原生的窗口按钮就是箭头，手型是 Web 习惯不是 Windows 习惯
  （VS Code 这类 Electron 应用同样用箭头）。
- **可拖区（`caption`）也保持箭头**：那是窗口 chrome，不是按钮。
- 为什么例外一定生效：`@layer base` 里的规则压不过 `@layer utilities` 里的 `.cursor-default`
  ——这是**层序**保证的（theme → base → components → utilities），不靠 CSS 文件里的先后，
  跟前面 `hover:bg-*` 那种「同层同优先级靠顺序」的坑不是一回事。

### 7. 滚动条：细、半透明、轨道透明（对齐 macOS / DSH）

口径（用户提的，照 DSH 的观感来）：**细**、**半透明**、**轨道全透明**——看起来像浮在内容上，
而不是一条有槽的轨道。

- **全应用一套**：样式集中在 `style.css` 的变量里（`--scrollbar-thumb`），
  组件里**不要各写一套**。颜色从 `--foreground` 派生，深浅色都能用，不写死黑。
  已验证：左栏与正文里的 `pre` 两处宿主的滑块颜色/宽度/`scrollbar-width` 完全一致。
- **两套写法都要写**：标准属性 `scrollbar-width` / `scrollbar-color`（Chromium 121+ 也认）
  与 `::-webkit-scrollbar*`（WebView2 走这条）。只写一套，在某些引擎里就是完全不生效。
- **怎么做到「细」**：热区给 10px，用「透明边框 + `background-clip: padding-box`」把可见部分
  收到 **5px 左右**（Chromium 会把滚动条伪元素的边框按比例缩放：写 `3px` 算出来是 `2.4px`，
  所以别断言字面值，断言**可见宽度**）。
  ⚠️ 热区那 10px **仍然占布局宽度**；热区给太小会不好拖，所以不能靠缩热区来变细。
- ⚠️ **「鼠标进入变实」这层没做**，因为它在这个 WebView2 里**不生效**（实测）：
  `aside:hover` 会正确翻转，但 `::-webkit-scrollbar-thumb` 的 computed 颜色**一动不动**——
  刷新时鼠标恰好停在容器上才会显示出 hover 色，很容易误判成「生效了」（我就误判过一次）。
  没做的就不写进规范，也**不留一行不生效的死代码**；要真做得走这两条路之一：
  ① JS 在 `mouseenter` 时给容器加类；② 给 WebView2 加 `--enable-features=FluentOverlayScrollbars`。
- ⚠️ **这不是真正的 overlay 滚动条**：Chromium 已删掉 `overflow: overlay`，
  Windows 上也不提供 macOS 那种「浮在上面、不占位、自动淡出」的系统行为。
  这里对齐的是**观感**（细 + 半透明 + 轨道透明），占位这一点没变——写清楚，
  免得以后有人以为它是真 overlay 而去改布局。

## 未定

1. **一条 bar 将来放不下时怎么拆**：新方案加了导航之后，可能要拆成「窗口 chrome + 工具栏」
   两条（当时的备选方案 B）。现在不预先拆。
2. 标题栏还要不要别的入口（菜单、版本号、状态指示）。
3. **空状态下标题栏偏秃**：`projects/` 为空时项目按钮组为空，那一段是空的，怎么处理待设计。
4. 窗口按钮的图形（现在用 lucide 的 `Minus` / `Square` / `Copy`），要不要换成更贴近 Windows 原生的自绘图标。

## 怎么验证

**主手段是探针**：`scripts/check/nonclient-probe.ps1`（做什么、适用范围、什么时候不该用写在文件头）。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\check\nonclient-probe.ps1
```

实测期望输出（窗口宽度 1440 时；x 是距窗口左边的像素）：

```
标题：[SSOT 工作台]
客户区原点：相对窗口左上角 +0,+0
  → 客户区从窗口顶部开始：原生标题栏已去掉，顶上就是自绘的那条

横向扫描：窗口顶边往下 20 px
  x     2 -     2 : HTLEFT      边缘缩放（原生）
  x    18 -    82 : HTCAPTION   应用名：可拖
  x    98 -   370 : HTCLIENT    当前项目 + 项目按钮：普通内容区，可点
  x   386 -  1298 : HTCAPTION   拖拽空白
  x  1314 -  1346 : HTMINIMIZE  窗口按钮
  x  1362 -  1394 : HTMAXIMIZE  窗口按钮
  x  1410 -  1426 : HTCLOSE     窗口按钮

竖向扫描：在 HTMINIMIZE 的中线往下扫
  y     1 -     5 : HTTOP       缩放进来的边框
  y     7 -    31 : HTMINIMIZE  窗口按钮通高 → bar 就是这 32px
  y    33 -    79 : HTCLIENT    内容区
```

⚠️ 量 bar 高度**必须扫窗口按钮**（它们 `items-stretch`、通高）。扫可拖填充区会量小：
那是标出来的元素，被 bar 的内边距内缩过（第一次就是这么量错的）。

⚠️ **不要用 `WS_CAPTION` 判断原生标题栏在不在**。Wails 的 `Frameless` 是靠 `WM_NCCALCSIZE`
把客户区铺满整窗，样式位 `WS_CAPTION` / `WS_THICKFRAME` 是**故意保留**的
（`WS_THICKFRAME` 还要给缩放用，见 `pkg/application/webview_window_windows.go` 的
`WM_NCCALCSIZE` 注释）。判据是**客户区原点**：`+0,+0` = 原生那条没了，约 `+0,+31` = 还在。
（这条是踩过的坑：探针第一版拿 `WS_CAPTION` 判断，误报成「Frameless 没生效」。）

界面侧另有一条轻量检查（dev 模式，端口见 `Taskfile.yml` 的 `VITE_PORT`）：

```powershell
# 应用名在标题栏里只出现一次：转换后的 App.tsx 模块里不该有第二处
$c = (Invoke-WebRequest http://127.0.0.1:9245/src/App.tsx -UseBasicParsing).Content
$c.Contains("SSOT 工作台")   # 期望 False——它现在只在 AppTitleBar 里
```

注意：查的是 **Vite 转换后的模块**，所以 HMR 一到就能看出来，不必重启窗口。
类型检查：`cd frontend && npx tsc --noEmit`。
