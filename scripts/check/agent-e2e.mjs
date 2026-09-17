/**
 * agent-e2e.mjs —— 点界面上的「启动后端」，验这条真路径能起来（默认**跳过**）。
 *
 * 做什么：连上应用，切到 Agent 模式，点「启动后端」，等状态栏报出后端身份与会话 id，
 *   并断言没有出现错误提示。这一步只做 ACP 握手 + 开会话（含按会话挂 MCP），
 *   **不发提示词**，所以不花模型调用。
 *
 * 适用范围：改 Agent 后端接线（appconfig / agentapp / api / AgentPane）之后想验真路径时跑。
 *   **默认跳过**：它依赖本机装了 DSH 并建好了 `acp` profile，别人机器上跑会假红。
 *
 * 什么时候不该用：
 *   - 不要放进日常回归（要 `SSOT_E2E_AGENT=1` 才跑）。
 *   - 它不验「回答得对不对」——那要发提示词、花额度，得人自己跑。
 *
 * 用法：$env:SSOT_E2E_AGENT="1"; node scripts/check/agent-e2e.mjs
 */
import { chromium } from "playwright-core";

if (process.env.SSOT_E2E_AGENT !== "1") {
  console.log("跳过（要真起后端：设 SSOT_E2E_AGENT=1 再跑）");
  process.exit(0);
}

const CDP = process.env.SSOT_CDP || "http://127.0.0.1:9222";
const b = await chromium.connectOverCDP(CDP);
const page = b.contexts().flatMap((c) => c.pages()).find((p) => /9245/.test(p.url()));
if (!page) {
  console.error("调试端口上没有页面");
  process.exit(1);
}
await page.reload();
await page.waitForSelector("aside .group-title", { timeout: 20000 });

await page.getByRole("button", { name: "Agent" }).first().click();
await page.waitForTimeout(300);

const main = page.locator("section.min-w-0").first();
await main.getByRole("button", { name: "启动后端" }).first().click();

// 起 harness 要几秒；最多等 90 秒，期间把状态栏的变化打出来。
let text = "";
for (let i = 0; i < 45; i++) {
  await page.waitForTimeout(2000);
  text = await main.innerText();
  if (/deepseek-harness-acp|后端就绪/.test(text)) break;
  if (/RuntimeError|起后端失败|开会话失败|握手失败/.test(text)) break;
}
console.log("状态栏与消息：");
console.log(text.split("\n").slice(0, 12).join("\n"));

const ok = /deepseek-harness-acp/.test(text) && !/RuntimeError|失败/.test(text);
console.log(ok ? "\n✓ 后端起来了（会话已开，MCP 按会话挂上）" : "\n✗ 后端没起来（上面是界面里的原文）");
await b.close();
process.exit(ok ? 0 : 1);
