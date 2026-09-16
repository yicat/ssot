/**
 * 把正在运行的 ssot 窗口**读成文本**，必要时**真的操作一次**再读。
 *
 * 做什么
 *   通过 WebView2 的远程调试端口（CDP）连上去：
 *     - 默认：取回窗口里渲染出来的文字（document.body.innerText）
 *     - `--select`：只看某个选择器命中的元素文本
 *     - `--eval` / `--eval-file`：执行一段 JS 并按需读结果
 *     - `--focus` / `--type` / `--key` / `--wait`：**真的在界面里打字、按回车**再读
 *   用来在「看不了屏幕」（比如读不了截图的模型）时**验证界面真的渲染出/响应了**，
 *   而不是只看日志里没报错就猜。
 *
 * 适用范围
 *   - Windows，应用以 dev 或 build 起在跑，并且**启动时带了**环境变量
 *     `SSOT_WEBVIEW_DEBUG_PORT=9222`（`main.go` 的 `debugBrowserArgs` 读它，
 *     再用 `application.Options.Windows.AdditionalBrowserArgs` 开端口）。
 *   - ⚠️ **不要**改用 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`：Wails 在
 *     `preventEnvAndRegistryOverrides` 里会 `os.Setenv` 成自己的值把它盖掉
 *     （`internal/webview2/webviewloader/native_module.go`），从外面设不管用——
 *     这条是实测踩出来的，不是猜的。
 *   - 默认只读；`--eval` / `--click` / `--keys` **会改界面状态**，看懂再跑。
 *
 * ⚠️ 验**交互**（填输入框、点按钮、断言结果）请用 `ui-test.mjs`（Playwright）。
 *    手搓 CDP 的输入路径有坑：`Input.insertText` 走编辑器命令，**不触发 React 认的 onChange**
 *    ——DOM 里看着有字、React 状态还是空的，很容易把好功能判成坏的。
 *    这个脚本留着是为了**零依赖**地读文本：不用装任何东西就能看一眼窗口里是什么。
 *
 * 什么时候不该用
 *   - 没带 `SSOT_WEBVIEW_DEBUG_PORT` 启动：9222 端口不会开，脚本会明确报「连不上调试端口」。
 *   - 想验证**样式/配色**：它读的是文本与属性，读不出「好不好看」。
 *   - 别拿它当爬虫反复刷：每次求值都排在界面线程上。
 *
 * 用法
 *   # 先带端口起应用（dev 模式）
 *   $env:SSOT_WEBVIEW_DEBUG_PORT="9222"; wails3 task dev
 *
 *   node scripts/check/ui-dump.mjs                                     # 打印整窗文本
 *   node scripts/check/ui-dump.mjs --select "aside"                    # 只看左栏
 *   node scripts/check/ui-dump.mjs --eval "document.querySelectorAll('button').length"
 *   node scripts/check/ui-dump.mjs --eval-file probe.js                # 多行 JS 走文件
 */
const CDP = "http://127.0.0.1:9222";

function fail(msg) {
  console.error("错误：" + msg);
  process.exit(1);
}

function arg(name) {
  const i = process.argv.indexOf(name);
  return i >= 0 ? process.argv[i + 1] : null;
}

async function main() {
  const { readFileSync } = await import("node:fs");
  let evalJS = arg("--eval");
  const evalFile = arg("--eval-file");
  if (evalFile) evalJS = readFileSync(evalFile, "utf8");
  const select = arg("--select");
  const focus = arg("--focus");
  const clickSel = arg("--click");
  const clickText = arg("--click-text");
  const typeText = arg("--type");
  const keysText = arg("--keys");
  const key = arg("--key");
  const wait = Number(arg("--wait") ?? 0);

  let targets;
  try {
    const res = await fetch(CDP + "/json/list");
    targets = await res.json();
  } catch (e) {
    fail(`连不上调试端口 ${CDP}（应用是不是没带 SSOT_WEBVIEW_DEBUG_PORT=9222 启动？）：${e.message}`);
  }
  const page = (targets || []).find((t) => t.type === "page" && t.webSocketDebuggerUrl);
  if (!page) fail("调试端口上没有页面目标（窗口还没加载出来？）");

  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = () => reject(new Error("WebSocket 连接失败"));
  });

  let nextID = 1;
  const pending = new Map();
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    const p = pending.get(msg.id);
    if (!p) return;
    pending.delete(msg.id);
    if (msg.error) p.reject(new Error(msg.error.message));
    else p.resolve(msg.result);
  };
  const send = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = nextID++;
      pending.set(id, { resolve, reject });
      ws.send(JSON.stringify({ id, method, params }));
    });

  const evaluate = async (expression) => {
    const r = await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
    if (r.exceptionDetails) {
      throw new Error(r.exceptionDetails.text + " " + (r.exceptionDetails.exception?.description ?? ""));
    }
    const v = r.result?.value;
    return typeof v === "string" ? v : JSON.stringify(v ?? r.result?.description ?? "");
  };

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  if (focus) {
    const ok = await evaluate(`(() => { const el = document.querySelector(${JSON.stringify(focus)});
      if (!el) return false; el.focus(); return true; })()`);
    if (ok !== "true") fail(`--focus 找不到元素：${focus}`);
  }
  // 真鼠标点击（不是 JS 的 el.click()）：先量中心点，再派发 mousePressed/mouseReleased。
  if (clickSel || clickText) {
    const finder = clickSel
      ? `document.querySelector(${JSON.stringify(clickSel)})`
      : `[...document.querySelectorAll('button,a,[role=button]')].find(el => el.innerText.includes(${JSON.stringify(clickText)}))`;
    const box = await evaluate(`(() => { const el = ${finder};
      if (!el) return '';
      const r = el.getBoundingClientRect();
      return JSON.stringify({ x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2), t: el.innerText.slice(0, 20) }); })()`);
    if (!box) fail(`找不到要点的元素：${clickSel ?? clickText}`);
    const { x, y, t } = JSON.parse(box);
    for (const type of ["mousePressed", "mouseReleased"]) {
      await send("Input.dispatchMouseEvent", { type, x, y, button: "left", buttons: 1, clickCount: 1 });
    }
    console.error(`（点了「${t}」 at ${x},${y}）`);
    await sleep(200);
  }
  if (typeText) {
    // ⚠️ insertText 走的是编辑器命令，**不触发 React 认的 input 事件**：
    // 实测 DOM 里有字、React 状态还是空的，按回车当然没反应。
    // 要验输入框就用 --keys（逐字派发真实按键）。
    await send("Input.insertText", { text: typeText });
    await sleep(120);
  }
  if (keysText) {
    for (const ch of keysText) {
      await send("Input.dispatchKeyEvent", { type: "keyDown", text: ch, unmodifiedText: ch, key: ch });
      await send("Input.dispatchKeyEvent", { type: "keyUp", key: ch });
      await sleep(40);
    }
  }
  if (key) {
    const codes = { Enter: 13, Tab: 9, Escape: 27, Backspace: 8 };
    const common = { key, code: key, windowsVirtualKeyCode: codes[key] ?? 0, nativeVirtualKeyCode: codes[key] ?? 0 };
    await send("Input.dispatchKeyEvent", { type: "keyDown", text: key === "Enter" ? "\r" : undefined, ...common });
    await send("Input.dispatchKeyEvent", { type: "keyUp", ...common });
  }
  if (wait > 0) await sleep(wait);

  const expression = evalJS
    ? evalJS
    : select
      ? `(() => { const els = [...document.querySelectorAll(${JSON.stringify(select)})];
         return els.length + " 个命中\\n" + els.map(e => e.innerText).join("\\n---\\n"); })()`
      : `(() => document.title + "\\n" + document.body.innerText)()`;

  console.log(await evaluate(expression));
  ws.close();
}

main().catch((e) => fail(e.message));
