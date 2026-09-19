// agent-probe —— **不开界面**，直接从 Go 侧把聊天那条链的数据跑一遍。
//
// 做什么：按配置（appconfig.Agent）起 ACP 后端 → 打印状态 → 列会话（走 `agentapp.Sessions`：
// 自己按 cwd 过滤 + 从 DSH 存储读标题）→ 试切回「上次的会话」→ 停掉。
//
// 为什么要有它：界面那半（渲染、按钮）要靠 Playwright 连窗口；但**数据那半（后端起没起、
// 会话列表对不对、标题读得到吗）全在 Go 这边**，这里能直接验，不用等界面、也不用开调试端口。
//
// 什么时候不该用：
//   - 它**不验界面渲染**（那是 `scripts/check/agent-ui-test.mjs` 的活，且不改 UI 就能过）；
//   - 它**不发提示词**（不花额度）；要验「AI 能不能干活」得另跑真任务；
//   - 它会在 DSH 里**建一个会话**（起后端就会建）——不想留就手动删，或用 `-no-start` 只列会话。
//
// 用法：
//
//	go run ./scripts/check/agent-probe -root projects/demo            # 起后端 + 列会话
//	go run ./scripts/check/agent-probe -root projects/demo -no-start # 不起后端，只看配置与存储
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ngnl5/ssot/internal/application/agentapp"
	"github.com/ngnl5/ssot/internal/infrastructure/acp"
	"github.com/ngnl5/ssot/internal/infrastructure/appconfig"
	"github.com/ngnl5/ssot/internal/infrastructure/dshstore"
)

func main() {
	root := flag.String("root", "projects/demo", "vault 目录")
	noStart := flag.Bool("no-start", false, "不起后端，只看配置与 DSH 存储里的会话标题")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	must(err)

	settings := appconfig.Defaults()
	if store, err := appconfig.Open(); err == nil {
		if loaded, _, err := store.Load(); err == nil {
			settings = loaded
		}
	}
	agent := settings.Agent
	fmt.Printf("vault      %s\n", abs)
	fmt.Printf("DSH        %s（profile=%s）\n", agent.DSHInstall, agent.Profile)
	fmt.Printf("DSH_HOME   %s\n", agent.DSHHome)
	fmt.Printf("CLI        %s（存在=%v）\n", agent.CLIBin, exists(agent.CLIBin))

	titles := dshstore.Titles(agent.DSHHome)
	fmt.Printf("\nDSH 存储里能读出 %d 条会话标题\n", len(titles))
	n := 0
	for id, t := range titles {
		if n >= 5 {
			break
		}
		fmt.Printf("  %s  %s\n", id[:8], t)
		n++
	}

	if *noStart {
		fmt.Println("\n(-no-start：不起后端)")
		return
	}

	svc := agentapp.New(agentapp.Config{
		Vault:          abs,
		BackendCommand: agent.DSHExe(),
		BackendArgs:    agent.Args(),
		BackendEnv:     agent.Env(),
		Actor:          "probe",
		MCPServer: acp.MCPServer{
			Name:    "ssot",
			Command: agent.CLIBin,
			Args:    []string{"mcp", "-root", abs},
		},
		OnLog: func(s string) { fmt.Printf("[后端] %s\n", s) },
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Println("\n=== 起后端 ===")
	start := time.Now()
	st, err := svc.Start(ctx)
	if err != nil {
		fmt.Printf("起后端失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("起来了：agent=%s version=%s session=%s（%.1f 秒）\n",
		st.Agent.Name, st.Agent.Version, short(st.SessionID), time.Since(start).Seconds())

	fmt.Println("\n=== 列会话（agentapp 自己按 cwd 过滤）===")
	list, err := svc.Sessions(ctx)
	if err != nil {
		fmt.Printf("列会话失败：%v\n", err)
	} else {
		fmt.Printf("本 vault 的会话：%d 个\n", len(list))
		for i, s := range list {
			if i >= 8 {
				break
			}
			title := s.Title
			if title == "" {
				title = "（没有标题）"
			}
			fmt.Printf("  %s  %s  ← %s\n", short(s.ID), title, s.Cwd)
		}
	}

	fmt.Println("\n=== 停后端 ===")
	if err := svc.Stop(); err != nil {
		fmt.Printf("停后端失败：%v\n", err)
	} else {
		fmt.Println("已停")
	}
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误："+err.Error())
		os.Exit(1)
	}
}
