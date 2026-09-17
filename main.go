package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ngnl5/ssot/internal/api"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// debugBrowserArgs 给 WebView2 的额外启动参数。
//
// 设了 SSOT_WEBVIEW_DEBUG_PORT 才返回 `--remote-debugging-port=<端口>`——
// 有了它，`scripts/check/ui-dump.mjs` 能把窗口里渲染出来的东西读成文本
// （看不了屏幕时用这个验证界面，而不是只靠「日志里没报错」猜）。
func debugBrowserArgs() []string {
	port := os.Getenv("SSOT_WEBVIEW_DEBUG_PORT")
	if port == "" {
		return nil
	}
	return []string{"--remote-debugging-port=" + port}
}

// Wails 用 Go 的 embed 把前端产物打进二进制。
//
// 因此 frontend/dist 必须在编译前存在——先跑 `npm run build`（或 `wails3 task build`）。
// 注意：显式列出要打进二进制的后端包，否则 bindings 可能因「没有引用」而被裁掉。
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	projectsRoot := flag.String("projects", "projects", "项目根目录（扫它来发现项目）")
	projectDir := flag.String("project", "", "初始项目目录；留空则自动取第一个")
	flag.Parse()

	// 当前项目是会话状态：界面可以在运行时切换，不必重启进程。
	session := compose.NewSession(*projectsRoot, *projectDir)
	defer func() { _ = session.Close() }()

	// 聊天服务：起后端、开会话、推流式更新。设置读不动也要能起界面
	// （配置页正是用来告诉人「哪里不对」的），所以这里错了就把错误带在身上，不 panic。
	agentSvc, agentErr := api.NewAgentService(session)
	if agentErr != nil {
		fmt.Fprintln(os.Stderr, "聊天服务不可用："+agentErr.Error())
	}

	services := []application.Service{
		application.NewService(api.NewProjectService(session)),
		application.NewService(api.NewVaultService(session)),
	}
	if agentSvc != nil {
		services = append(services, application.NewService(agentSvc))
	}

	app := application.New(application.Options{
		Name:        "ssot",
		Description: "单一事实源工具",
		// 服务按设计逐个加回来——分层不变：api → application → domain ← infrastructure。
		// 界面与 CLI 走同一套用例层，所以「谁能发布」这类规则只有一份实现。
		Services: services,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Windows: application.WindowsOptions{
			// 调试用：只有显式设了 SSOT_WEBVIEW_DEBUG_PORT 才开远程调试端口。
			//
			// ⚠️ 不要用 WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS 那个环境变量：Wails 在
			// preventEnvAndRegistryOverrides 里会 os.Setenv 成自己的值把它盖掉
			// （internal/webview2/webviewloader/native_module.go），从外面设没用。
			AdditionalBrowserArgs: debugBrowserArgs(),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		// Title 从此只是窗口标识（任务栏、Alt-Tab 上显示的名字）：
		// 原生标题栏已被下面的 Frameless 去掉，界面上那条是自绘的
		// （frontend/src/components/custom/AppTitleBar/，见 docs/specs/shell.spec.md）。
		Title:  "SSOT 工作台",
		Width:  1440,
		Height: 900,
		// 去掉原生标题栏：整窗只保留自绘的那一条，免得出现两条 titlebar。
		Frameless: true,
		Windows: application.WindowsWindow{
			// 打开「非客户区」机制：前端用 CSS 变量 --wails-non-client-region
			// 标出的区域，由 WM_NCHITTEST 回 HTCAPTION/HTMINBUTTON/HTMAXBUTTON/HTCLOSE，
			// 于是拖动、双击最大化、右键系统菜单、Snap Layouts 都是 Windows 原生行为。
			// 注意开关是这一项，不是 NonClientRegionSupport——那只喂 WebView2 自带的 app-region，
			// 而前端上报与 WM_NCHITTEST 这条路只在 WebView2CompositionHosting 下才走。
			WebView2CompositionHosting: true,
		},
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		// 浅色主题对应浅色底：留一个深色底会在启动瞬间闪一下黑，
		// 而那一瞬间正好是窗口刚出现、人最注意它的时候。
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
