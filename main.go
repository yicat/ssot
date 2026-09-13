package main

import (
	"embed"
	"flag"
	"log"

	"github.com/ngnl5/ssot/internal/api"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails 用 Go 的 embed 把前端产物打进二进制。
// 因此 frontend/dist 必须在编译前存在——先跑 `npm run build`（或 `wails3 task build`）。
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	projectDir := flag.String("project", "projects/onmyoji", "项目目录")
	flag.Parse()

	app := application.New(application.Options{
		Name:        "ssot",
		Description: "单一事实源工具",
		// 核验工作台的后端。所有判断都在 domain 与 application 里，
		// 这里只做暴露——CLI 与 GUI 因此看到同一份规则。
		Services: []application.Service{
			application.NewService(api.NewReviewService(*projectDir)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "SSOT 核验工作台",
		Width:  1180,
		Height: 760,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
