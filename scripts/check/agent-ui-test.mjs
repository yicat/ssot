/**
 * agent-ui-test.mjs —— 验「聊天界面 + 配置页」这一层，**不依赖 vault 里的具体内容**。
 *
 * 做什么：连上正在跑的应用（CDP），断言
 *   1. 主体区有「文档 / 数据表 / Agent」三个模式，能切；
 *   2. 切到 Agent 后面板在、没起后端时说清了怎么起；
 *   3. 配置弹窗能打开，三节都在，**后端检查逐条列出来**（缺什么报什么），能保存；
 *   4. 全程没有页面异常。
 *
 * 适用范围：改 AgentPane / SettingsModal / 工具条模式切换时跑它。
 *   它**不假装**能验真后端：不发话、不花模型调用（那是人自己跑的事）。
 *
 * 什么时候不该用：
 *   - 不要拿它替代 `ui-test.mjs`：那份测的是文档渲染与树，内容依赖它当前的 vault。
 *   - 它也不测 ACP 协议本身：协议由 `go test ./internal/infrastructure/acp` 用假后端验。
 *
 * 需求：应用带 `SSOT_WEBVIEW_DEBUG_PORT=9222` 启动（见 main.go 的 debugBrowserArgs）。
 * 用法：node scripts/check/agent-ui-test.mjs
 */
import { chromium } from "playwright-core";

const CDP = process.env.SSOT_CDP || "http://127.0.0.1:9222";
const APP_URL_RE = /wails\.localhost|127\.0\.0\.1:9245|localhost:9245/;

let passed = 0;
const failures = [];
function check(name, ok, detail = "") {
  if (ok) {
    passed++;
    console.log(`  ✓ ${name}${detail ? `  —— ${detail}` : ""}`);
  } else {
    failures.push(name);
    console.log(`  ✗ ${name}${detail ? `  —— ${detail}` : ""}`);
  }
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

  console.log("\n== 模式切换 ==");
  const tabs = page.locator("section, div").filter({ hasText: /^文档数据表Agent$/ }).first();
  const tabButtons = page.getByRole("button", { name: /^(文档|数据表|Agent)$/ });
  check("工具条上有三个模式按钮", (await tabButtons.count()) >= 3, `count=${await tabButtons.count()}`);
  check("默认在「文档」模式", (await page.getByRole("button", { name: "文档" }).first().getAttribute("aria-pressed")) === "true");

  await page.getByRole("button", { name: "Agent" }).first().click();
  await page.waitForTimeout(300);
  const asideVisible = await page.locator("aside").first().isVisible();
  check("切到 Agent 之后左栏还在（聊天不占导航）", asideVisible);
  // ⚠️ 主体区是 section.min-w-0；页面上还有别的 section，用 .last() 会取到空的那个（踩过）。
  const mainText = await page.locator("section.min-w-0").first().innerText();
  check("Agent 面板出现了", /后端|agent|启动后端|说点什么/i.test(mainText), mainText.split("\n").slice(0, 3).join(" / "));
  // 没起后端时：必须说清「怎么起」，而不是一片空白
  check(
    "没起后端时说清了怎么起",
    /启动后端|后端未启动|先启动后端/.test(mainText),
    mainText.split("\n").filter((l) => l.trim()).slice(0, 4).join(" / "),
  );
  check("面板里写了「agent 不能发布」这条边界", /不能发布|发布只能/.test(mainText));

  console.log("\n== 配置弹窗 ==");
  await page.locator("button[title*='配置']").first().click();
  await page.waitForSelector("[role=dialog][aria-label=配置]", { timeout: 8000 });
  const dialog = page.locator("[role=dialog][aria-label=配置]");
  const dialogText = await dialog.innerText();
  check("配置弹窗打开了", true);
  check("有「项目」一节", dialogText.includes("项目"));
  check("有「Agent 后端」一节", dialogText.includes("Agent 后端"));
  check("有「外观」一节", dialogText.includes("外观"));
  check("写了模型与 key 不在这里", /模型|key|密钥/.test(dialogText), "（口径见 settings.spec.md §2）");

  // 后端检查：逐条列出来（缺什么报什么），并且要说清「怎么补」
  const rows = await dialog.locator("li").allInnerTexts();
  const checkRows = rows.filter((t) => /DSH|profile|ssot CLI|harness|dsh 入口/.test(t));
  check("逐条列出后端检查项", checkRows.length >= 4, checkRows.length + " 条");
  check(
    "检查项给出了路径（能照着去查）",
    checkRows.every((t) => /[\\/]/.test(t)),
    checkRows[0]?.slice(0, 60) ?? "",
  );

  // 设置文件路径要显示出来：读不动的时候人得知道去哪改
  check("显示了设置文件路径", /settings\.json/.test(dialogText), dialogText.match(/[^\s]*settings\.json/)?.[0] ?? "");

  // 保存：改一个字段再存，重新打开要还在（走真后端读写用户级配置）
  const profileInput = dialog.locator("input").nth(1); // 0=projectsRoot, 1=profile
  const before = await profileInput.inputValue();
  await profileInput.fill("acp");
  await dialog.getByRole("button", { name: /保存/ }).click();
  await page.waitForTimeout(600);
  await dialog.getByRole("button", { name: "关闭配置" }).click();
  await page.waitForSelector("[role=dialog][aria-label=配置]", { state: "detached", timeout: 8000 });
  // ⚠️ 用 title 定位工具条上那个「配置」：Agent 面板里也有一个同名按钮，
  // 只按名字取 first 会赌 DOM 顺序，而且弹窗还没卸干净时点击会被背景盖住（踩过，超时 30s）。
  await page.locator("button[title*='配置']").first().click();
  await page.waitForSelector("[role=dialog][aria-label=配置]", { timeout: 8000 });
  const after = await page.locator("[role=dialog][aria-label=配置]").locator("input").nth(1).inputValue();
  check("保存后能读回（配置真的落盘了）", after === "acp", `before=${before} after=${after}`);
  await page.locator("[role=dialog][aria-label=配置]").getByRole("button", { name: "关闭配置" }).click();

  console.log("\n== 回到文档模式 ==");
  await page.getByRole("button", { name: "文档" }).first().click();
  await page.waitForTimeout(300);
  check("切回文档模式后正文回来了", (await page.locator("article, .md-body, section").count()) > 0);

  check("期间没有页面异常", errors.length === 0, errors.slice(0, 2).join(" | "));

  await browser.close();
  const total = passed + failures.length;
  console.log(`\n${passed}/${total} 通过`);
  if (failures.length) {
    console.log("失败：" + failures.join("、"));
    process.exit(1);
  }
}

main().catch((e) => {
  console.error("测试脚本自身出错：" + e.message);
  process.exit(1);
});


