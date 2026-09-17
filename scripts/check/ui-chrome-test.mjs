/**
 * ui-chrome-test.mjs —— 验界面外壳那些「一眼看不出对错」的口径（**不依赖 vault 内容**）。
 *
 * 做什么：连上正在跑的应用，用**计算样式**断言外壳口径——目前是滚动条
 *   （细 / 半透明 / 轨道透明 / 悬停变实，见 docs/specs/shell.spec.md §7）。
 *   我读不了截图，所以「看着像不像」只能靠量出来的数字验。
 *
 * 适用范围：改 style.css 里的全局样式、或改外壳（标题栏、滚动条、字体栈）时跑。
 *   它不碰 vault，也不启动 Agent 后端，所以换项目、换数据都不会假红。
 *
 * 什么时候不该用：
 *   - 不要拿它验文档渲染或文件树（那是 ui-test.mjs，内容依赖当前 vault）；
 *   - 不要拿它验聊天界面（那是 agent-ui-test.mjs）。
 *
 * 需求：应用带 `SSOT_WEBVIEW_DEBUG_PORT=9222` 启动。
 * 用法：node scripts/check/ui-chrome-test.mjs
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

/** 解析 oklch/rgb 颜色里的 alpha；解析不出来返回 null。 */
function alphaOf(color) {
  if (!color) return null;
  const oklch = color.match(/oklch\(([^)]*)\)/);
  if (oklch) {
    const parts = oklch[1].split("/");
    return parts.length > 1 ? parseFloat(parts[1]) : 1;
  }
  const rgba = color.match(/rgba?\(([^)]*)\)/);
  if (rgba) {
    const p = rgba[1].split(",").map((s) => s.trim());
    return p.length > 3 ? parseFloat(p[3]) : 1;
  }
  if (color === "transparent") return 0;
  return null;
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
    console.error("调试端口上没有页面");
    process.exit(1);
  }
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  console.log(`已连上：${page.url()}`);

  await page.reload();
  await page.waitForSelector("aside .group-title", { timeout: 20000 });

  console.log("\n== 滚动条（shell.spec.md §7）==");
  // 挑一个**真的能滚**的容器（左栏文档树），量它的伪元素。
  const read = () =>
    page.evaluate(() => {
      const el = document.querySelector("aside");
      if (!el) return null;
      const bar = getComputedStyle(el, "::-webkit-scrollbar");
      const thumb = getComputedStyle(el, "::-webkit-scrollbar-thumb");
      const track = getComputedStyle(el, "::-webkit-scrollbar-track");
      const own = getComputedStyle(el);
      return {
        width: bar.width,
        thumbBg: thumb.backgroundColor,
        thumbRadius: thumb.borderRadius,
        thumbBorder: thumb.borderTopWidth,
        trackBg: track.backgroundColor,
        scrollbarWidth: own.scrollbarWidth,
        scrollbarColor: own.scrollbarColor,
        canScroll: el.scrollHeight > el.clientHeight,
      };
    });

  // ⚠️ 先把鼠标移出左栏再读「默认值」：鼠标一开始可能就停在左栏上，
  // 那样读到的是 hover 后的颜色，于是「默认 0.34 → 悬停 0.34」看着像规则没生效（踩过）。
  const middle = await page.locator("section.min-w-0").first().boundingBox();
  await page.mouse.move(middle.x + middle.width / 2, middle.y + middle.height / 2);
  await page.waitForTimeout(250);

  const s = await read();
  if (!s) {
    console.error("左栏没找到（aside 不在？）");
    process.exit(1);
  }
  check("热区 10px（细）", s.width === "10px", s.width);
  // ⚠️ 别断言 border 的字面值：Chromium 会把滚动条伪元素的边框按比例缩放
  // （写 3px，算出来是 2.4px）。要断言的是**可见宽度**——那才是「细」这件事本身。
  const visible = 10 - 2 * parseFloat(s.thumbBorder);
  check("可见部分细（约 4–5px）", visible >= 2 && visible <= 6, `border=${s.thumbBorder} → 可见 ${visible.toFixed(1)}px`);
  check("滑块圆角（不是方块）", parseFloat(s.thumbRadius) >= 100, s.thumbRadius);

  const thumbAlpha = alphaOf(s.thumbBg);
  check("滑块是半透明（不是实心黑）", thumbAlpha !== null && thumbAlpha > 0.05 && thumbAlpha < 0.5, `${s.thumbBg} alpha=${thumbAlpha}`);

  const trackAlpha = alphaOf(s.trackBg);
  check("轨道全透明（看起来像浮在内容上）", trackAlpha === 0, `${s.trackBg}`);

  check("标准属性也写了（只写一套会不生效）", s.scrollbarWidth === "thin", `scrollbar-width=${s.scrollbarWidth}`);

  // ⚠️ 这里**没有**「悬停变实」的断言：实测这个 WebView2 不会因为 hover 变化重算
  // `::-webkit-scrollbar*` 的样式（`aside:hover` 会翻转，滑块颜色不动）。
  // 所以那层没做，也就没有可断言的东西——写一条「悬停不生效」的断言只会把 bug 钉成规范。
  // 想知道细节看 docs/specs/shell.spec.md §7。

  // 「全应用一套」：换一个滚动宿主量一遍，风格必须与左栏一致（防止有人某处又写了一套）。
  const pre = await page.evaluate(() => {
    const el = document.querySelector(".md-body pre") || document.querySelector("section.min-w-0");
    if (!el) return null;
    const h = getComputedStyle(el, "::-webkit-scrollbar-thumb");
    const own = getComputedStyle(el);
    return { thumbBg: h.backgroundColor, width: getComputedStyle(el, "::-webkit-scrollbar").width, scrollbarWidth: own.scrollbarWidth };
  });
  check(
    "另一处滚动宿主也是同一套（不是某处单独写的）",
    !!pre && pre.thumbBg === s.thumbBg && pre.width === s.width && pre.scrollbarWidth === s.scrollbarWidth,
    pre ? `${pre.thumbBg} / ${pre.width}` : "没找到第二个宿主",
  );
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



