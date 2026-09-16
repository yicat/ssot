// Command ssot 是 SSOT 工具的命令行入口。
//
// ⚠️ 当前状态：**骨架**。上一套方案（六部件 + 断言库 + 核验流程）已整体作废，
// 业务子命令与规格集一并清空，新的设计待定。
//
// 这里只保留入口本身：GUI 与 CLI 将来共用同一套用例层，
// 因此这个入口要在，但**不要**先铺一堆空壳子命令——那会让人以为功能已经存在。
//
// 已作废的那套实现在分支 `legacy/mvp-v1` 上备查。
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`ssot —— 单一事实源工具

当前是**骨架**：上一套方案已作废，业务命令与规格集已清空，新设计待定。

用法：
  ssot help          看这段说明

界面（Wails 桌面 / 服务模式）：
  wails3 task dev            开发运行
  wails3 task run:server     以服务模式跑在 localhost:8080
  wails3 task check          提交前全量检查（vet + 测试 + 前端构建）

备查：作废的那套实现（规格、领域、界面）在分支 legacy/mvp-v1 上。
`)
}
