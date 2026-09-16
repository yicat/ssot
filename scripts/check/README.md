# scripts/check/ —— 检查与探针

这一组是**只读/验证**用的工具：把看不见的东西变成文本证据，或直接对界面跑断言。

| 脚本 | 一句话 | 依赖 |
|---|---|---|
| `nonclient-probe.ps1` | 探自绘标题栏：原生标题栏是否已去掉、哪一段 x 是可拖区、哪一段是窗口按钮、bar 多高 | PowerShell |
| `ui-dump.mjs` | 把窗口里**渲染出来的东西读成文本**（WebView2 远程调试端口 + CDP） | **零依赖**（Node 内置） |
| `ui-test.mjs` | **界面测试**：连上真实窗口，填输入框、点按钮、断言文本 | `playwright-core`（`scripts/package.json`） |

什么时候用哪个：

- 改窗口外壳（`main.go` 的窗口选项、`AppTitleBar` 的 `--wails-non-client-region`）→ `nonclient-probe.ps1`。
- 改界面、只想**看一眼**窗口里是什么 → `ui-dump.mjs`（不用装东西）。
- 改界面、要**断言行为**（检索出结果、点结果能切、断链会报错）→ `ui-test.mjs`。

⚠️ 验交互**别**手搓 CDP：`Input.insertText` 走编辑器命令，不触发 React 认的 onChange，
会出现「DOM 有字、状态是空的」，很容易把好功能判成坏的。Playwright 的 fill/press 按真实路径走。

`ui-test.mjs` 与 `ui-dump.mjs` 都要求应用带 `SSOT_WEBVIEW_DEBUG_PORT=9222` 启动
（见 `main.go` 的 `debugBrowserArgs`；**不能**用 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`，
Wails 会覆盖它）。

```powershell
$env:SSOT_WEBVIEW_DEBUG_PORT="9222"; wails3 task dev   # 另开一个终端
node scripts/check/ui-dump.mjs
node scripts/check/ui-test.mjs                          # 需要先 cd scripts && npm install
```

## 界面行为与源码不符时的排查顺序（踩过）

1. **先确认 dev 服务器提供的是新模块**，别急着怀疑代码。源码里刚加的东西在下面这条命令里搜不到，
   就说明页面拿到的还是旧模块：
   ```powershell
   (curl.exe -s http://127.0.0.1:9245/src/components/custom/VaultBrowser/useVaultBrowser.ts) -join "`n"
   ```
   ⚠️ **必须先 `-join`**：PowerShell 接 `curl.exe` 的输出拿到的是**字符串数组**，
   数组的 `.Contains('...')` 是在找元素，永远是 false——我因此得出过「Vite 给了旧模块」的**假结论**，
   白折腾一轮。
2. 还是旧的（或页面报 `xxx is not a function`）：重启 `wails3 task dev`，
   必要时删 `frontend/node_modules/.vite`。
3. **抓页面异常**：Playwright 的 `page.on("pageerror", …)` 能直接看到未捕获错误
   （`search is not a function` 就是这么抓出来的，比读日志快得多）。

每个脚本的详细边界写在各自文件头部（做什么 / 适用范围 / 什么时候不该用）。
