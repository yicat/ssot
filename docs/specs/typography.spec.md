# typography.spec.md —— 字体与字号

主题：**字用什么字体、多大、行高多少**。跨页面，所以单开一个主题。

> ⚠️ 主题怎么切、要不要固定模板还没定（见 `README.md`），这份沿用现有三段写法，**不宣布这是模板**。
> 引用第三方写「文件 + 出处位置 + 版本」；引用本仓库只写符号名，不写行号（行号会过期）。

## 已确认的约定

### 1. 字体：照 DeepSeek Harness，走系统栈，不打包 webfont

出处：DSH 桌面版 `resources/app/node_modules/@deepseek-ai/dsh-client-ui-theme/lib/client.js`
里 `:root` 的 token（我在本机 DSH 安装目录读到的原文）：

```css
--font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC",
             "Hiragino Sans GB", "Microsoft YaHei", "Helvetica Neue", Helvetica, Arial, sans-serif;
--font-mono: "SF Mono", "JetBrains Mono", "Fira Code", Consolas, "Liberation Mono",
             Menlo, Courier, "PingFang SC", "Microsoft YaHei";
```

- Windows 上 `sans` 落到 **Segoe UI**、中文落到 **微软雅黑**（`PingFang SC` / `Hiragino Sans GB` 在前面，
  但本机没有，跳过）。
- **不打包 UI 字体**：DSH 整个包里只有 KaTeX 有 `@font-face`，界面字全是系统字。
  因此原来的 `@fontsource-variable/geist` **已去掉**——Geist 只含 latin / latin-ext / cyrillic / vietnamese
  子集，**没有中文字形**，中文会掉到浏览器默认，和拉丁部分不是同一个家族体系，混排不齐。
- `--font-mono` 同时定了 `<pre>` / `<code>` 的字体：Tailwind 预检把它们指到
  `--default-mono-font-family`（默认就是 `var(--font-mono)`），所以**不要在代码块上再手写 `font-mono`**。

### 2. 字号：两档，改 token 而不是各处写 class

| 档 | 字号 / 行高 | Tailwind | 用在哪 | DSH 对应 |
|---|---|---|---|---|
| 正文 | **14px / 22px** | `text-sm` | 正文、按钮、输入、标题栏应用名 | `--dsh-content-font-size`，默认 14px |
| 次要 | **12px / 18px** | `text-xs` | 次要说明、元信息、徽标、代码块 | 组件里的 12px 元信息档（11–12px） |
| 文档正文 | **13px / 21px** | 见 `.md-body` | **只有文档正文**用这一档 | — |

**为什么文档正文单独一档**：文档是这套东西的主产物，长时间**阅读**和点按钮不是一回事——
界面控件用 14/22，文档正文用更紧的 13/21，一屏能多看几行；但**控件与说明仍守 14/12 两档**，
不许在界面里随手插 13px。这条口径只在 `.md-body` 这一处生效。

- 定义在 `frontend/src/style.css` 的 `@theme` 里：**改 token 就全局生效**，各处不要再写 `text-[13px]`
  这类字面量。
- **行高必须一起定**：Tailwind 默认 `text-sm` 是 14/20、`text-xs` 是 12/16；DSH 是 14/22、12/18，更松一点。
- `text-base`（16/24）保持 Tailwind 默认，卡片标题用它。
- ⚠️ **`@theme inline` 会把 token 内联进工具类**：产物 CSS 里是
  `.text-sm{font-size:14px;line-height:var(--tw-leading,22px)}`，**没有 `--text-sm` 这个变量**。
  所以验证要看编译后的工具类，别去找变量——找不到不代表没生效（这条踩过）。
- `--font-mono` 会被编译成 `--default-mono-font-family`，由 Tailwind 预检的
  `code,kbd,samp,pre{font-family:var(--default-mono-font-family,…)}` 消费。

### 3. 不用 shadcn `Button` 的 `size="sm"`

- `components/ui/button.tsx` 的 `sm` 档写着字面量 `text-[0.8rem]`（= **12.8px**），**不受 token 控制**，
  而且**从外面覆盖不掉**：实测生成 CSS 里 `.text-[0.8rem]{` 排在 `.text-sm{` 之后
  （offset 15306 vs 15122），同为 utilities 层、同优先级时后者胜——也就是额外加 `text-sm` 也压不住它。
- 约定：**业务代码不用 `size="sm"`**。要正常大小用默认档（`h-8` + `text-sm`，受 token 控制）；
  要小号用 `xs`（`h-6` + `text-xs`，同样受 token 控制）。
- `components/ui/` 是生成物（勿手改），**所以不去改那一行**——这条约定就是绕开它的办法。
  将来重新生成后，如果默认档或 `xs` 也长出字面量，用同一招绕开，别去改生成物。
- **反过来是安全的**：给 shadcn 组件传 `className` 覆盖**确定生效**——`Button` 的类名走
  `cn(buttonVariants({ variant, size, className }))`，而 `cn` 是 `twMerge(clsx(...))` 的替代品，
  在**类名层**就把同类冲突消解掉（后传的胜），与 CSS 顺序无关。
  例：`className="font-semibold"` 会顶掉基础类里的 `font-medium`，可以放心用。
  这就是「传 className 安全、在普通元素上叠同类工具类不安全」的分界。

## 未定

1. DSH 还有 **13px 次级正文**档（`--dsh-content-font-size-secondary`）和 **11px 极小**档。
   现在骨架用不到就先不建；真出现「次级正文」需求（比如大段说明）再加，原则不变：**档位少、改 token**。
2. DSH 的字号可整体缩放（`--dsh-content-font-size` + delta 推导标题字号）。我们要不要做这个设置项，没定。
3. 暗色模式下的字重与对比度要不要另调。

## 怎么验证

```powershell
cd frontend; npm run build
$c = Get-Content -Raw (Get-ChildItem dist\assets\*.css | Select-Object -First 1).FullName

# 字体栈生效（sans 落在 html 上，mono 通过 --default-mono-font-family 给 <pre>/<code>）
$c.Contains('Segoe UI')                  # 期望 True
$c.Contains('--default-mono-font-family:"SF Mono"')   # 期望 True
$c.Contains('Geist')                     # 期望 False——Geist 已彻底拿掉

# 字号 token 生效：看**编译后的工具类**，不要找 --text-sm 变量（inline 模式不 emit）
$c.Contains('.text-sm{font-size:14px;line-height:var(--tw-leading,22px)}')  # 期望 True
$c.Contains('.text-xs{font-size:12px;line-height:var(--tw-leading,18px)}')  # 期望 True
```

- **不该再有 `size="sm"`**（除 `components/ui/button.tsx` 里的定义本身）：
  `Select-String -Path frontend\src\components\custom\**\*.tsx -Pattern 'size="sm"'` 应为空。
- 改动标题栏后按 `shell.spec.md` 的探针确认那条 bar 没被撑坏。
- 类型检查：`cd frontend && npx tsc --noEmit`。

