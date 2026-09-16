/**
 * 把正在运行的 ssot 窗口**读成文本**。
 *
 * 做什么
 *   通过 WebView2 的远程调试端口（CDP）连上去，执行一段 JS 取回：
 *     - 窗口里渲染出来的文字（document.body.innerText）
 *     - 一段可选的 CSS 选择器命中的元素数量与文本
 *   用来在「看不了屏幕」（比如读不了截图的模型）时**验证界面真的渲染出了东西**，
 *   而不是只看日志里没有报错就猜。
 *
 * 适用范围
 *   - Windows，应用以 dev 或 build 起在跑，并且**启动时带了**环境变量
 *     `SSOT_WEBVIEW_DEBUG_PORT=9222`（`main.go` 的 `debugBrowserArgs` 读它，
 *     再用 `application.Options.Windows.AdditionalBrowserArgs` 开端口）。
 *   - ⚠️ **不要**改用 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`：Wails 在
 *     `preventEnvAndRegistryOverrides` 里会 `os.Setenv` 成自己的值把它盖掉
 *     （`internal/webview2/webviewloader/native_module.go`），从外面设不管用——
 *     这条是实测踩出来的，不是猜的。
 *   - 只读：只执行取文本的 JS，不改界面状态、不点按钮。
 *
 * 什么时候不该用
 *   - 没带 `SSOT_WEBVIEW_DEBUG_PORT` 启动：9222 端口不会开，脚本会明确报「连不上调试端口」。
 *   - 想验证**交互**（点按钮、切项目）：它只读快照，不能代替人点一遍。
 *   - 想验证**样式/配色**：它读的是文本与属性，读不出「好不好看」。
 *   - 别拿它当爬虫反复刷：每次求值都排在界面线程上。
 *
 * 用法
 *   # 先带端口起应用（dev 模式）
 *   $env:SSOT_WEBVIEW_DEBUG_PORT="9222"; wails3 task dev
 *
 *   node scripts/check/ui-dump.mjs                      # 打印整窗文本
 *   node scripts/check/ui-dump.mjs --select "aside button"   # 只看某个选择器的命中
 */
const CDP = "http://127.0.0.1:9222";

function fail(msg) {
  console.error("错误：" + msg);
  process.exit(1);
}

async function main() {
  const selectIdx = process.argv.indexOf("--select");
  const selector = selectIdx >= 0 ? process.argv[selectIdx + 1] : null;

  let targets;
  try {
    const res = await fetch(CDP + "/json/list");
    targets = await res.json();
  } catch (e) {
    fail(`连不上调试端口 ${CDP}（应用是不是没带 WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222 启动？）：${e.message}`);
  }
  const page = (targets || []).find((t) => t.type === "page" && t.webSocketDebuggerUrl);
  if (!page) fail("调试端口上没有页面目标（窗口还没加载出来？）");

  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = () => reject(new Error("WebSocket 连接失败"));
  });

  const expression = selector
    ? `(() => { const els = [...document.querySelectorAll(${JSON.stringify(selector)})];
         return els.length + " 个命中\\n" + els.map(e => e.innerText).join("\\n---\\n"); })()`
    : `(() => document.title + "\\n" + document.body.innerText)()`;

  const result = await new Promise((resolve, reject) => {
    ws.onmessage = (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.id !== 1) return;
      if (msg.result?.exceptionDetails) {
        reject(new Error(msg.result.exceptionDetails.text + " " + (msg.result.exceptionDetails.exception?.description ?? "")));
      } else {
        resolve(msg.result?.result?.value ?? "");
      }
    };
    ws.onerror = () => reject(new Error("求值失败"));
    ws.send(JSON.stringify({ id: 1, method: "Runtime.evaluate", params: { expression, returnByValue: true } }));
  });

  console.log(result);
  ws.close();
}

main().catch((e) => fail(e.message));
