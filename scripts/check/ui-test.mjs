/**
 * 界面测试：用 Playwright 连上**正在运行的 ssot 窗口**跑断言。
 *
 * 做什么
 *   连 WebView2 的远程调试端口（CDP），把界面当普通网页测：
 *   填输入框、按回车、点按钮、断言文本。跑完打印每条的通过/失败，失败时退出码非 0。
 *
 * 适用范围
 *   - Windows，应用跑着，并且**启动时带了** `SSOT_WEBVIEW_DEBUG_PORT=9222`
 *     （`main.go` 的 `debugBrowserArgs`；⚠️ 不能用 WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS，
 *     Wails 会覆盖它）。
 *   - 测的是**真实窗口**：bindings 走真正的 Go 用例层，不是 mock。
 *   - 需要 vault 里有测试数据；默认用 `projects/demo`（断言就写死在里面那三篇文档上）。
 *
 * 什么时候不该用
 *   - 没带调试端口启动：连不上，脚本会明确报错。
 *   - 想断言「好不好看」：它只断言文本与结构，颜色/间距得人看。
 *   - 断言写死了 demo 数据：换 vault 要改断言——它验的是「界面通不通」，不是通用回归。
 *
 * 用法
 *   $env:SSOT_WEBVIEW_DEBUG_PORT="9222"; wails3 task dev     # 先起应用
 *   node scripts/check/ui-test.mjs
 *
 * 为什么用 Playwright 而不是手搓 CDP
 *   手搓 CDP 时「填输入框」要么用 insertText（不触发 React 认的事件）、
 *   要么自己造 keydown，很容易出现「DOM 有值、React 状态是空的」这种假象，
 *   把好功能判成坏的。Playwright 的 fill/press 按真实输入路径走，还自带等待。
 */
import { chromium } from "playwright-core";

const CDP = "http://127.0.0.1:9222";
const APP_URL_RE = /127\.0\.0\.1:9245|localhost:9245|wails\.localhost:9245/;

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
  console.log(`已连上：${page.url()}`);

  // 从头开始：刷新一次，避免上一次操作的残留状态影响断言。
  await page.reload();
  await page.waitForSelector("aside", { timeout: 15000 });

  // 1) 左栏按两层列出文档，未核验有标记
  const aside = await page.locator("aside").innerText();
  check("左栏列出整理层", aside.includes("整理层 docs/（2）"), aside.split("\n")[0]);
  check("左栏列出原始层", aside.includes("原始层 raw/（1）"));
  check("未核验有标记", aside.includes("未核验"));
  check("已发布有标记", aside.includes("已发布"));

  // 2) 数据表进了左栏（含推断出来的列）
  check("数据表列出（可 SQL 查）", aside.includes("数据表（可 SQL 查）") && aside.includes("技能倍率"));

  // 3) 右栏打开第一篇：正文带文件行号、元信息齐
  const section = await page.locator("section").first().innerText();
  check("右栏显示文档标题与状态", section.includes("茨木童子") && section.includes("未核验"));
  check("右栏显示标签与来源", section.includes("标签：式神、SSR") && section.includes("来源："));
  check("正文带文件行号（第 16 行那条双链）", section.includes("16"));

  // 4) 检索：Playwright 的 fill/press 按真实输入路径走
  await page.fill("input", "伤害");
  await page.press("input", "Enter");
  await page.waitForSelector("text=/检索「伤害」/", { timeout: 8000 });
  const asideAfter = await page.locator("aside").innerText();
  check("检索出结果并把检索词显示出来", asideAfter.includes("检索「伤害」（3）"), firstLine(asideAfter));
  check("检索结果带未核验标记", asideAfter.includes("未核验"));

  // 5) 点检索结果 → 右栏切过去
  await page.locator("aside button", { hasText: "伤害计算" }).first().click();
  await page.waitForSelector("text=最终伤害", { timeout: 8000 });
  const section2 = await page.locator("section").first().innerText();
  check("点结果能切到那篇文档", section2.includes("伤害计算") && section2.includes("最终伤害"));

  // 6) 点正文里的断链 → 如实报错（这正是这套东西该有的样子）
  await page.locator("section button", { hasText: "御魂套装效果" }).first().click();
  await page.waitForSelector("text=/找不到双链目标/", { timeout: 8000 });
  const notice = await page.locator("text=/找不到双链目标/").first().innerText();
  check("断链被如实报出来", notice.includes("御魂套装效果"), notice.slice(0, 40));

  // 7) 提示条能关掉
  //    注意：不能拿「找不到双链目标」这段文字判有没有关掉——右栏的「问题链接」面板里
  //    也有同样的理由文字。要按**提示条本身**（那个红色容器）判。
  const banner = page.locator('div[class*="rose-50"]');
  const before = await banner.count();
  await page.locator("button[aria-label='关闭提示']").first().click();
  await page.waitForTimeout(300);
  const after = await banner.count();
  check("提示条能关闭", before > 0 && after === 0, `关前 ${before} 个 → 关后 ${after} 个`);

  await browser.close();

  const failed = results.filter((r) => !r.ok);
  console.log(`\n${results.length - failed.length}/${results.length} 通过`);
  process.exit(failed.length === 0 ? 0 : 1);
}

function firstLine(s) {
  return s.split("\n").filter(Boolean)[0] ?? "";
}

main().catch((e) => {
  console.error("测试脚本自身出错：" + e.message);
  process.exit(1);
});

