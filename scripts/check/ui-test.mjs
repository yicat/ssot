/**
 * 界面测试：用 Playwright 连上**正在运行的 ssot 窗口**跑断言。
 *
 * 做什么
 *   连 WebView2 的远程调试端口（CDP），把界面当普通网页测：
 *   点文档树、看渲染结果、填检索框、勾任务、点断链，逐条断言。失败时退出码非 0。
 *
 * 适用范围
 *   - Windows，应用跑着，并且**启动时带了** `SSOT_WEBVIEW_DEBUG_PORT=9222`
 *     （`main.go` 的 `debugBrowserArgs`；⚠️ 不能用 WEBVIEW2_ADDITIONAL_BROWSER_ARGS，
 *     Wails 会覆盖它）。
 *   - 测的是**真实窗口**：bindings 走真正的 Go 用例层，不是 mock。
 *   - 断言写死在 `projects/demo` 那几篇示例文档上——它验的是「界面通不通」，
 *     不是通用回归。换 vault 要改断言。
 *
 * 什么时候不该用
 *   - 没带调试端口启动：连不上，脚本会明确报错。
 *   - 想断言「好不好看」：它只断言文本与结构，颜色/间距/手感得人看。
 *
 * 用法
 *   $env:SSOT_WEBVIEW_DEBUG_PORT="9222"; wails3 task dev     # 先起应用
 *   node scripts/check/ui-test.mjs
 *
 * 为什么用 Playwright 而不是手搓 CDP
 *   手搓 CDP 填输入框要么用 insertText（不触发 React 认的事件）、要么自己造 keydown，
 *   很容易出现「DOM 有值、React 状态是空的」这种假象，把好功能判成坏的。
 *   Playwright 的 fill/press/click 按真实输入路径走，还自带等待。
 */
import { chromium } from "playwright-core";

const CDP = "http://127.0.0.1:9222";
const APP_URL_RE = /9245/;

const results = [];
function check(name, ok, detail = "") {
  results.push({ name, ok, detail });
  console.log(`${ok ? "  ✓" : "  ✗"} ${name}${detail ? "  —— " + detail : ""}`);
}

async function main() {
  let browser;
  try {
    browser = await chromium.connectOverCDP(CDP);
  } catch (e) {
    console.error(`连不上 ${CDP}：${e.message}`);
    console.error("（应用是不是没带 SSOT_WEBVIEW_DEBUG_PORT=9222 启动？）");
    process.exit(1);
  }

  const pages = browser.contexts().flatMap((c) => c.pages());
  const page = pages.find((p) => APP_URL_RE.test(p.url())) ?? pages[0];
  if (!page) {
    console.error("调试端口上没有页面（窗口还没加载出来？）");
    process.exit(1);
  }
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  console.log(`已连上：${page.url()}`);

  await page.reload();
  await page.waitForSelector("aside", { timeout: 15000 });

  // ── 文件树（层级、排序、标记）─────────────────────────────
  const aside = page.locator("aside").first();
  const treeText = await aside.innerText();
  check("文件树用中文分组标题", treeText.includes("整理层") && treeText.includes("原始层"));
  // 状态在行尾：用位置断言（chip 在标题右边、且贴着行的右边缘）
  const layout = await page.evaluate(() => {
    const row = [...document.querySelectorAll("aside button")].find(
      (b) => b.innerText.includes("茨木童子") && b.innerText.includes("未核验"),
    );
    if (!row) return null;
    const spans = [...row.querySelectorAll("span")];
    const title = spans.find((s) => s.innerText.includes("茨木童子"));
    const chip = spans.find((s) => s.innerText.trim() === "未核验");
    if (!title || !chip) return null;
    const rb = row.getBoundingClientRect();
    const tb = title.getBoundingClientRect();
    const cb = chip.getBoundingClientRect();
    return { titleX: Math.round(tb.left - rb.left), chipX: Math.round(cb.left - rb.left), chipRightGap: Math.round(rb.right - cb.right) };
  });
  check(
    "状态标记在标题右边、贴行尾",
    !!layout && layout.chipX > layout.titleX && layout.chipRightGap <= 12,
    layout ? JSON.stringify(layout) : "没找到那一行",
  );
  check("文件树显示文件夹", treeText.includes("式神") && treeText.includes("机制"));
  // 树用次要字号 + 文件夹/文档一眼分得开（document.spec.md 第二节）
  const treeStyle = await page.evaluate(() => {
    const row = [...document.querySelectorAll("aside button")].find((b) => b.innerText.includes("茨木童子"));
    return {
      font: row ? getComputedStyle(row).fontSize : null,
      folders: document.querySelectorAll("aside .lucide-folder, aside .lucide-folder-open").length,
      files: document.querySelectorAll("aside .lucide-file-text").length,
    };
  });
  check("树用 12px 次要字号", treeStyle.font === "12px", String(treeStyle.font));
  // 树的语义色：分组最淡、文件夹浅蓝、文件名比正文淡，且**图标与文字同色**
  const treeColors = await page.evaluate(() => {
    const color = (el) => (el ? getComputedStyle(el).color : null);
    const folderBtn = [...document.querySelectorAll("aside button")].find((b) =>
      b.querySelector(".lucide-folder, .lucide-folder-open"),
    );
    const fileBtn = [...document.querySelectorAll("aside button")].find((b) => b.querySelector(".lucide-file-text"));
    const group = document.querySelector("aside .tree-group");
    const h1 = document.querySelector("article h1");
    return {
      // 显式查图标：按钮里第一个 svg 是**折叠箭头**，不是文件夹图标（踩过）
      folderIcon: color(folderBtn?.querySelector(".lucide-folder, .lucide-folder-open")),
      folderText: color(folderBtn?.querySelector(".tree-folder")),
      fileIcon: color(fileBtn?.querySelector(".lucide-file-text")),
      fileText: color(fileBtn?.querySelector(".tree-file")),
      group: color(group),
      body: color(h1),
    };
  });
  const oklch = (s) => {
    const m = (s ?? "").match(/oklch\(([\d.]+)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+))?/);
    return m ? { l: +m[1], c: +m[2], h: +m[3], a: m[4] ? +m[4] : 1 } : null;
  };
  const fc = oklch(treeColors.folderText);
  const gc = oklch(treeColors.group);
  const bc = oklch(treeColors.body);
  const fl = oklch(treeColors.fileText);
  // 比**解析后的值**而不是原始字符串：同一个颜色可能被序列化成 `oklch(…)` 或 `oklch(… / 1)`。
  const sameColor = (x, y) => {
    const a = oklch(x);
    const b = oklch(y);
    if (!a || !b) return x === y;
    return Math.abs(a.l - b.l) < 0.01 && Math.abs(a.c - b.c) < 0.01 && Math.abs(a.h - b.h) < 1 && Math.abs(a.a - b.a) < 0.01;
  };
  check(
    "文件夹图标与文字同色",
    sameColor(treeColors.folderIcon, treeColors.folderText),
    `icon=${treeColors.folderIcon} text=${treeColors.folderText}`,
  );
  check("文件夹是浅蓝（有彩度）", !!fc && fc.c > 0.03, `chroma=${fc?.c}`);
  check("文档图标与文字同色", sameColor(treeColors.fileIcon, treeColors.fileText), `icon=${treeColors.fileIcon} text=${treeColors.fileText}`);
  check("文件名比正文淡", !!fl && !!bc && fl.l >= bc.l && fl.a < bc.a, `file=${treeColors.fileText} body=${treeColors.body}`);
  check("分组标题最淡", !!gc && !!fl && gc.l > fl.l, `group=${treeColors.group}`);
  const groupWeight = await page.evaluate(() => {
    const g = document.querySelector("aside .tree-group");
    return g ? getComputedStyle(g).fontWeight : null;
  });
  check("分组标题不加粗", groupWeight === "400", String(groupWeight));
  check("文件夹与文档图标不同", treeStyle.folders > 0 && treeStyle.files > 0, `folder=${treeStyle.folders} file=${treeStyle.files}`);
  check("示例 vault 没有顶层文档（合规）", !treeText.includes("直接放在顶层"));
  check("树里未核验有标记", treeText.includes("未核验"));
  check("数据表列在左栏", treeText.includes("数据表"));

  // ── 文档渲染（切到语法示例那篇）───────────────────────────
  await page.locator("aside button", { hasText: "语法示例" }).first().click();
  await page.waitForSelector("article h1", { timeout: 8000 });
  const body = page.locator(".md-body");
  check("markdown 标题渲染成 h2", (await body.locator("h2").count()) > 0);
  check("GFM 表格渲染成 table", (await body.locator("table").count()) > 0);
  check("LaTeX 公式渲染成 KaTeX", (await body.locator(".katex").count()) > 0);
  check("==高亮== 渲染成 mark", (await body.locator("mark").count()) > 0);
  check("callout 渲染", (await body.locator(".md-callout").count()) >= 2);
  check("双链渲染成可点链接", (await body.locator("a.md-wikilink").count()) >= 4);
  check("断链有醒目样式", (await body.locator("a.md-wikilink.md-broken").count()) >= 1);
  check("块锚点渲染", (await body.locator("a.md-blockref").count()) >= 1);
  check("嵌入数据表渲染成表", (await body.locator(".md-embed-table table").count()) > 0);

  // 注释：默认隐藏（留在 DOM 里），点「显示注释」才看得见
  const comment = body.locator(".md-comment").first();
  check("注释默认不显示", (await comment.count()) > 0 && !(await comment.isVisible()));
  await page.locator("button:has-text('显示注释')").first().click();
  await page.waitForTimeout(200);
  check("点「显示注释」后可见", await comment.isVisible());
  await page.locator("button:has-text('隐藏注释')").first().click();

  // 任务列表：勾一下要写回文件（走真后端）
  const task = body.locator("input.md-task").first();
  check("任务列表渲染成勾选框", (await body.locator("input.md-task").count()) >= 2);
  await task.click();
  await page.waitForSelector("text=/行已(勾选|取消勾选)/", { timeout: 8000 });
  const taskNotice = await page.locator("text=/行已(勾选|取消勾选)/").first().innerText();
  check("勾任务写回文件并如实报告", /行已(勾选|取消勾选)/.test(taskNotice), taskNotice.slice(0, 40));
  await page.locator("button[aria-label='关闭提示']").first().click();

  // ── 双链跳转 ─────────────────────────────────────────────
  await page.locator(".md-body a.md-wikilink", { hasText: "伤害计算" }).first().click();
  await page.waitForSelector("text=防御减免", { timeout: 8000 });
  check("点双链能跳过去", (await page.locator("article h1").first().innerText()).includes("伤害计算"));
  check("跳过去后嵌入表还在", (await page.locator(".md-embed-table table").count()) > 0);

  // ── 断链如实报错 ─────────────────────────────────────────
  await page.locator(".md-body a.md-wikilink.md-broken").first().click();
  await page.waitForSelector("text=/找不到双链目标/", { timeout: 8000 });
  const notice = await page.locator("div[class*='rose-50']").first().innerText();
  check("断链被如实报出来", notice.includes("御魂套装效果"), notice.slice(0, 40));

  // ── 字号与视觉重心（用计算样式断言，不靠眼看）──────────
  const sizes = await page.evaluate(() => {
    const px = (sel) => {
      const el = document.querySelector(sel);
      return el ? getComputedStyle(el) : null;
    };
    const body = px(".md-body");
    const h1 = px("article h1");
    const tag = px("article button[title^='按标签检索']");
    return {
      bodyFont: body?.fontSize,
      bodyLine: body?.lineHeight,
      h1Font: h1?.fontSize,
      tagFont: tag?.fontSize,
      tagColor: tag?.color,
      tagBg: tag?.backgroundColor,
    };
  });
  check("文档正文是 13/21 那一档", sizes.bodyFont === "13px" && sizes.bodyLine === "21px", `${sizes.bodyFont}/${sizes.bodyLine}`);
  check("标题仍是最大字号", parseFloat(sizes.h1Font) > parseFloat(sizes.tagFont), `h1=${sizes.h1Font} tag=${sizes.tagFont}`);
  check("标签比标题轻（小一号 + 无底色）", parseFloat(sizes.tagFont) <= 11 && /rgba\(0, 0, 0, 0\)|transparent/.test(sizes.tagBg), `tag=${sizes.tagFont} bg=${sizes.tagBg}`);

  // ── 检索弹窗（命令面板式）──────────────────────────────
  await page.keyboard.press("/");
  await page.waitForSelector("[role=dialog]", { timeout: 8000 });
  check("按 / 能打开检索弹窗", true);
  await page.locator("[role=dialog] input").fill("伤害");
  await page.waitForSelector("[role=dialog] button:has-text('伤害计算')", { timeout: 8000 });
  const hitsText = await page.locator("[role=dialog]").innerText();
  check("弹窗里出结果", hitsText.includes("伤害计算") && hitsText.includes("未核验"));
  await page.keyboard.press("Enter");
  // 断言要精确：不能用「防御减免」这类文本——它也会出现在**弹窗的片段**里，会误判成"已打开"。
  await page.waitForSelector("[role=dialog]", { state: "detached", timeout: 8000 });
  check(
    "回车打开选中的结果并关弹窗",
    (await page.locator("article h1").first().innerText()).includes("伤害计算"),
    await page.locator("article h1").first().innerText(),
  );

  // ── 数据表能打开 ────────────────────────────────────────
  await page.locator("aside button", { hasText: "技能倍率" }).first().click();
  await page.waitForSelector("text=查询示例", { timeout: 8000 });
  const tablePane = await page.locator("section").first().innerText();
  check("点数据表能打开", tablePane.includes("技能倍率") && tablePane.includes("数据表"));
  check("表里是真实数据", tablePane.includes("罗生门") && tablePane.includes("2.63"));
  check("给出查询示例", tablePane.includes("SELECT * FROM"));

  check("期间没有页面异常", errors.length === 0, errors.slice(0, 2).join(" | "));

  await browser.close();

  const failed = results.filter((r) => !r.ok);
  console.log(`\n${results.length - failed.length}/${results.length} 通过`);
  process.exit(failed.length === 0 ? 0 : 1);
}

main().catch((e) => {
  console.error("测试脚本自身出错：" + e.message);
  process.exit(1);
});











