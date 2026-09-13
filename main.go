package main

import (
	"embed"
	"flag"
	"log"

	"github.com/ngnl5/ssot/internal/api"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails 用 Go 的 embed 把前端产物打进二进制。
// 因此 frontend/dist 必须在编译前存在——先跑 `npm run build`（或 `wails3 task build`）。
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	projectsRoot := flag.String("projects", "projects", "项目根目录（扫它来发现项目）")
	projectDir := flag.String("project", "projects/onmyoji", "初始项目目录")
	flag.Parse()

	// 当前项目是会话状态：界面可以在运行时切换，不必重启进程。
	session := compose.NewSession(*projectsRoot, *projectDir)
	defer func() { _ = session.Close() }()

	app := application.New(application.Options{
		Name:        "ssot",
		Description: "单一事实源工具",
		// 界面后端。所有判断都在 domain 与 application 里，
		// 这里只做暴露——CLI 与 GUI 因此看到同一份规则。
		Services: []application.Service{
			application.NewService(api.NewProjectService(session)),
			application.NewService(api.NewScenarioService(session)),
			application.NewService(api.NewDataService(session)),
			application.NewService(api.NewFormulaService(session)),
			application.NewService(api.NewExperienceService(session)),
			application.NewService(api.NewAlternativesService(session)),
			application.NewService(api.NewReviewService(session)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "SSOT 工作台",
		Width:  1440,
		Height: 900,
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
