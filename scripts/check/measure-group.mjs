/**
 * measure-group.mjs —— 量「分组标题 + 其分隔线」的几何与颜色。
 *
 * 做什么：连到正在跑的 dev WebView（CDP 9222），把全 app 每个分组标题（`.group-title`）的
 *   位置/宽度/边框样式、以及它下面第一条目到线的距离打出来。
 * 适用范围：只有 `wails3 task dev` 在跑（且设了 SSOT_WEBVIEW_DEBUG_PORT=9222）时可用。
 * 什么时候不该用：不要用它做断言——断言写在 ui-test.mjs 里；这个只是「眼睛的替代品」。
 * 用法：node scripts/check/measure-group.mjs
 */
import { chromium } from "playwright-core";

const PORT = process.env.SSOT_WEBVIEW_DEBUG_PORT || "9222";
const URL = process.env.SSOT_URL || "http://wails.localhost:9245/";

const browser = await chromium.connectOverCDP(`http://127.0.0.1:${PORT}`);
const ctx = browser.contexts()[0];
const page = ctx.pages().find((p) => p.url().includes("wails.localhost")) ?? ctx.pages()[0];
await page.waitForSelector(".group-title", { timeout: 15000 });

const rows = await page.evaluate(() => {
  const out = [];
  for (const g of document.querySelectorAll(".group-title")) {
    const cs = getComputedStyle(g);
    const as = getComputedStyle(g, "::after");
    const r = g.getBoundingClientRect();
    const range = document.createRange();
    range.selectNodeContents(g);
    const textRight = Math.round(range.getBoundingClientRect().right - r.left);
    // 分组标题下面第一个兄弟/兄弟容器里的第一条目
    let next = g.nextElementSibling;
    while (next && !next.querySelector?.("button") && next.tagName !== "BUTTON") next = next.nextElementSibling;
    const firstBtn = next?.tagName === "BUTTON" ? next : next?.querySelector("button");
    const br = firstBtn?.getBoundingClientRect();
    out.push({
      text: g.textContent.trim(),
      font: cs.fontSize,
      color: cs.color,
      weight: cs.fontWeight,
      x: Math.round(r.x),
      w: Math.round(r.width),
      h: Math.round(r.height),
      padBottom: cs.paddingBottom,
      marginBottom: cs.marginBottom,
      textRight,
      lineW: Math.round(parseFloat(as.width)),
      lineH: as.height,
      lineBg: as.backgroundImage,
      // 线到下面第一条目的距离（线在元素底边）
      gapToFirst: br ? Math.round(br.top - r.bottom) : null,
      firstItemX: br ? Math.round(br.x) : null,
      firstItemW: br ? Math.round(br.width) : null,
    });
  }
  return out;
});

for (const r of rows) {
  console.log(`「${r.text}」 ${r.font} w=${r.w} h=${r.h} x=${r.x} 色=${r.color} 字重=${r.weight}`);
  console.log(`   线: ${r.lineH} 宽=${r.lineW}px（文字右边 ${r.textRight}px / 元素 ${r.w}px） ${r.lineBg}`);
  console.log(`   到第一条目 ${r.gapToFirst}px（条目 x=${r.firstItemX} w=${r.firstItemW}）`);
}
console.log(`共 ${rows.length} 个分组标题`);
await browser.close();

