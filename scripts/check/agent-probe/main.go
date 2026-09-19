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
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/agentapp"
	"github.com/ngnl5/ssot/internal/infrastructure/acp"
	"github.com/ngnl5/ssot/internal/infrastructure/appconfig"
	"github.com/ngnl5/ssot/internal/infrastructure/dshstore"
	"github.com/ngnl5/ssot/internal/infrastructure/sessionstore"
)

func main() {
	root := flag.String("root", "projects/demo", "vault 目录")
	noStart := flag.Bool("no-start", false, "不起后端，只看配置与 DSH 存储里的会话标题")
	del := flag.Bool("delete", false, "把**本 vault** 的会话连同文件删掉（按 id 精确定位；会打印每个被删的文件）")
	ensureOnly := flag.Bool("ensure-only", false, "只调 EnsureBackend（起后端但**不建会话**），前后各数一次 DSH 里的会话目录数")
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

	// -ensure-only：验「起后端会不会新开会话」——数 DSH 里 sessions/ 的目录数（前后各一次）。
	// 数文件系统而不是数 `Sessions()`：后者要求后端已经在跑，那就成了先用被测对象来造条件。
	if *ensureOnly {
		before := countSessionDirs(agent.DSHHome)
		fmt.Printf("\n=== EnsureBackend 前：sessions/ 目录 %d 个 ===\n", before)
		svc := newService(agent, abs)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		start := time.Now()
		st, err := svc.EnsureBackend(ctx)
		if err != nil {
			fmt.Printf("EnsureBackend 失败：%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("EnsureBackend 好了：running=%v session=%q（%.1f 秒）\n",
			st.Running, short(st.SessionID), time.Since(start).Seconds())
		after := countSessionDirs(agent.DSHHome)
		fmt.Printf("=== EnsureBackend 后：sessions/ 目录 %d 个 ===\n", after)
		if after == before {
			fmt.Println("结论：**没有新开会话** ✓")
		} else {
			fmt.Printf("结论：多了 %d 个 —— 还会新开，得继续查\n", after-before)
		}
		if err := svc.Stop(); err != nil {
			fmt.Printf("停后端失败：%v\n", err)
		}
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
		if *del {
			ids := make([]string, 0, len(list))
			for _, s := range list {
				ids = append(ids, s.ID)
			}
			fmt.Printf("\n=== 删这 %d 个会话（本 vault 的）===\n", len(ids))
			deleteSessions(agent.DSHHome, ids, abs)
		}
	}

	fmt.Println("\n=== 停后端 ===")
	if err := svc.Stop(); err != nil {
		fmt.Printf("停后端失败：%v\n", err)
	} else {
		fmt.Println("已停")
	}
}

// deleteSessions 删掉**这些会话**在 DSH 存储里的文件。
//
// 判据是**精确 id**（UUID），不再是模糊匹配：上次用「文件名里含这个 id」的办法，
// 把标题记录（`storages/.../<id>.json`）一起删了——会话还在、标题全没，列表里一堆「没有标题」。
// 现在每删一个文件都打出来，删了什么一清二楚。
func deleteSessions(dshHome string, ids []string, vaultRoot string) {
	if dshHome == "" || len(ids) == 0 {
		fmt.Println("没有可删的会话（或不知道 DSH_HOME）")
		return
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	deleted := 0
	_ = filepath.WalkDir(dshHome, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		// ⚠️ 匹配**完整路径**、并且**目录也要删**：会话日志放在以 id 命名的目录里
		// （`sessions/<id>/session.jsonl.zstd`）。只看文件名会一个都匹配不到
		// （踩过：删了 0 个，列表里还从 39 涨到 65——每次起后端都会新建会话）。
		for id := range set {
			if strings.Contains(path, id) {
				if d.IsDir() {
					if rmErr := os.RemoveAll(path); rmErr == nil {
						deleted++
						fmt.Printf("  删目录 %s\n", strings.TrimPrefix(path, dshHome))
					}
				} else if rmErr := os.Remove(path); rmErr == nil {
					deleted++
					fmt.Printf("  删文件 %s\n", strings.TrimPrefix(path, dshHome))
				}
				break
			}
		}
		return nil
	})
	fmt.Printf("共删 %d 项\n", deleted)

	// 我们自己那份元数据也清干净（会话没了，条目不该留着）。
	if err := sessionstore.Prune(vaultRoot, nil); err != nil {
		fmt.Printf("清理 %s 失败：%v\n", sessionstore.Path(vaultRoot), err)
	} else {
		fmt.Printf("（%s 里的条目：留着不动——Prune 不传 keep 不删；等有确定名单再清）\n", sessionstore.Path(vaultRoot))
	}
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// countSessionDirs 数 DSH 里 `sessions/` 下的目录数（一个会话一个目录）。
func countSessionDirs(dshHome string) int {
	dir := filepath.Join(dshHome, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return -1
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

// newService 按配置造一个 agentapp（探针与主流程共用，免得两处写法漂移）。
func newService(agent appconfig.Agent, vaultAbs string) *agentapp.Service {
	return agentapp.New(agentapp.Config{
		Vault:          vaultAbs,
		BackendCommand: agent.DSHExe(),
		BackendArgs:    agent.Args(),
		BackendEnv:     agent.Env(),
		Actor:          "probe",
		MCPServer: acp.MCPServer{
			Name:    "ssot",
			Command: agent.CLIBin,
			Args:    []string{"mcp", "-root", vaultAbs},
		},
		OnLog: func(s string) { fmt.Printf("[后端] %s\n", s) },
	})
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
