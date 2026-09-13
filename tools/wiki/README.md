# tools/wiki — 灰机 wiki 数据同步与体检

主源：`yys.huijiwiki.com`（阴阳师wiki）。完整背景见 [`docs/notes/wiki-data-source.md`](../../docs/notes/wiki-data-source.md)。

## 为什么需要浏览器

该站是 **Cloudflare 指纹门**，不下发 `cf_clearance`。以下都已实测失败：
纯 Node 带浏览器请求头、导出 cookie 后直连、headless Chrome、headless 复用已过挑战的 profile。

**唯一可行通道：可见窗口的 Chrome。** 抓取是批量同步而非实时访问，
所以**每次同步开一个窗口**即可，同步过程本身全自动。

## 用法

### 1. 启动可见 Chrome

```powershell
& "C:\Program Files\Google\Chrome\Application\chrome.exe" `
  --remote-debugging-port=9226 `
  --user-data-dir="<仓库路径>\.huiji-profile" `
  --no-first-run --no-default-browser-check
```

⚠️ 必须是**可见窗口**，headless 一定被挑战拦下。
⚠️ Chrome 在受限文件沙箱下无法启动（Mojo 需创建命名管道被拒），需要 `danger-full-access`。

### 2. 全量同步

```powershell
node tools/wiki/sync.mjs          # 默认端口 9226
node tools/wiki/sync.mjs 9227     # 指定其他端口
```

窗口会自动打开 wiki 并等待挑战通过，然后分批抓取。结束时自动关闭它启动的浏览器实例。

### 3. 数据体检（离线，不联网）

```powershell
node tools/wiki/verify.mjs
```

检查五项，退出码 1 表示存在 ERROR：

| # | 检查 | 说明 |
|---|---|---|
| 1 | 清单概览 | 页面数、缺失内容、按前缀分组 |
| 2 | 穷尽性 | `CharacterIndex` / `Attribute` / `Character` 文件三方 id 对齐，并对多出的 id **逐一归因** |
| 3 | schema 漂移 | **双向**：数据中未声明的字段、schema 中从未出现的字段 |
| 4 | 字段覆盖率 | 低于 100% 的字段列出，交由人工判定是合法缺失还是漏录 |
| 5 | 跨源一致性 | `Character.stats` vs `Attribute`，**做量纲归一后再比** |

## 缓存位置

```
.huiji/
├─ raw/            423 个页面正文（带 revid 的原始 wikitext / JSON）
├─ manifest.json   抓取清单：每个页面的 revid / parentid / timestamp / 字节数
└─ cookies.json    会话 cookie（**属凭据**，已 gitignore，勿提交）
```

`.huiji/` 与 `.huiji-profile/` 均在 `.gitignore` 中。

## 已知限制

- 技能倍率尚未抽取：1 级在 `description` 自由文本，2–5 级在 `upgrades[].effect`，两处格式不同
- 伤害计算公式尚无权威来源（wiki 数据层中不存在）
- `Data:Character.tabx` 未解析
- `Attribute.json` 的数值口径未验证（是否满级 / 是否含觉醒加成）
