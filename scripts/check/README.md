# scripts/check/ —— 检查类脚本

这一组是**只读探针**：不改状态，只把「看不见的东西」变成文本证据。

| 脚本 | 一句话 |
|---|---|
| `nonclient-probe.ps1` | 探自绘标题栏：原生标题栏是否已去掉、哪一段 x 是可拖区、哪一段是窗口按钮、bar 多高 |
| `ui-dump.mjs` | 把窗口里**渲染出来的东西读成文本**（WebView2 远程调试端口 + CDP），看不了屏幕时用它验证界面 |

什么时候用它：

- `nonclient-probe.ps1`：改动窗口外壳（`main.go` 的窗口选项、`AppTitleBar` 的 `--wails-non-client-region` 标记）之后，
  需要在不看屏幕的情况下确认非客户区机制真的生效。约定见 `docs/specs/shell.spec.md`。
- `ui-dump.mjs`：改界面之后，确认**真的渲染出了数据**（而不是只看日志没报错就以为好了）。
  需要应用带 `SSOT_WEBVIEW_DEBUG_PORT=9222` 启动；⚠️ 用不了 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`，
  Wails 会覆盖它（原因写在脚本头部）。

每个脚本的详细边界写在各自文件头部（做什么 / 适用范围 / 什么时候不该用）。
