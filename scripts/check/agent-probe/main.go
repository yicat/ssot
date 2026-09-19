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
//   - 不带 `-ask` 时**不发提示词**（不花额度）；要验「AI 能不能干活」得另跑真任务；
//   - 它会在 DSH 里**建一个会话**（`Start` / `-ask` 都会）——不想留就手动删，或用 `-no-start` 只列会话。
//
// 用法：
//
//	go run ./scripts/check/agent-probe -root projects/demo            # 起后端 + 列会话
//	go run ./scripts/check/agent-probe -root projects/demo -no-start # 不起后端，只看配置与存储
//	go run ./scripts/check/agent-probe -root projects/demo -ask "在库里找出提到「增益」的文档"  # 真发一轮，打工具名
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	ask := flag.String("ask", "", "真发一句话（**会花额度**）：起后端 → 建会话 → 发送，把这一轮模型实际调用的工具名打出来")
	rmSession := flag.String("rm-session", "", "删掉**这一个**会话（给完整 id；测试留下的用这个清，别用 -delete 把本 vault 的会话一把全删）")
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

	if *rmSession != "" {
		fmt.Printf("\n=== 删这一个会话：%s ===\n", *rmSession)
		deleteSessions(agent.DSHHome, []string{*rmSession}, abs)
		return
	}

	if *noStart {
		fmt.Println("\n(-no-start：不起后端)")
		return
	}

	// -ensure-only：验「起后端会不会新开会话」——数 DSH 里 sessions/ 的目录数（前后各一次）。
	// 数文件系统而不是数 `Sessions()`：后者要求后端已经在跑，那就成了先用被测对象来造条件。
	if *ensureOnly {
		before := countSessions(agent.DSHHome)
		fmt.Printf("\n=== EnsureBackend 前：真会话（所有 vault 加起来）%d 个 ===\n", before)
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
		after := countSessions(agent.DSHHome)
		fmt.Printf("=== EnsureBackend 后：真会话（所有 vault 加起来）%d 个 ===\n", after)
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

	svc := newService(agent, abs)

	// -ask：真跑一轮。**为什么要它**：`--dump-config` 只能证明「配置层把这个插件禁了」，
	// 证明不了「模型手上真的没有 grep」——ACP 不暴露工具清单，工具名只有真跑一轮才看得见。
	// 走法跟 App 一致：EnsureBackend 不建会话，会话由 NewSession 显式开。
	if *ask != "" {
		runAsk(svc, *ask)
		return
	}

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
	// 用 Delete 逐条删，不用 Prune：「keep 为空 = 什么都不删」那种语义凑一份名单太容易写错。
	for _, id := range ids {
		if err := sessionstore.Delete(vaultRoot, id); err != nil {
			fmt.Printf("清 %s 的条目失败：%v\n", short(id), err)
		}
	}
	fmt.Printf("（%s 里对应的条目已删）\n", sessionstore.Path(vaultRoot))
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// countSessions 数 DSH 里**真正的会话**有多少个（**全部 vault**，不只本 vault）。
//
// ⚠️ 别数 `sessions/` 下面那一层：那是**按 cwd 分的桶**（`--C-Users-...-demo--`），
// 桶的数量跟你开过几个会话无关。第一版就是这么数的，于是「前后都是 3」看着像通过，
// 其实什么也没验到（每个桶里才是 `<sessionId>/session.jsonl.zstd`）。
// 所以这里数到**两层**：桶的下一层目录。
//
// 为什么不按 vault 过滤：`EnsureBackend` 只可能动本 vault 的桶，所以「全部桶的总数不变」
// 已经足够回答问题，而且不依赖 DSH 把 cwd 编码成目录名的那套内部规则。
func countSessions(dshHome string) int {
	buckets, err := os.ReadDir(filepath.Join(dshHome, "sessions"))
	if err != nil {
		return -1
	}
	n := 0
	for _, b := range buckets {
		if !b.IsDir() {
			continue
		}
		kids, err := os.ReadDir(filepath.Join(dshHome, "sessions", b.Name()))
		if err != nil {
			continue
		}
		for _, k := range kids {
			if k.IsDir() {
				n++
			}
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
		OnUpdate: func(u acp.Update) {
			noteUpdate(u)
		},
	})
}

// —— `-ask` 用的一点点收集状态。探针是单进程一次性跑，不用上锁也够用；
// 锁着只是因为 ACP 的回调在客户端自己的 goroutine 里。

var (
	noteMu    sync.Mutex
	toolOrder []string          // 工具名，按第一次出现排序
	toolSeen  map[string]string // 工具名 → 最后一次状态
	toolCalls int               // 这一轮的工具调用**次数**（同一个工具可能调多次）
	oursCalls int               // 其中走我们能力层的次数（名字里带 ssot）
	answer    strings.Builder
)

// noteUpdate 从一条 ACP 更新里挑出「用了什么工具」和「最后答了什么」。
//
// 为什么看 `title`：ACP 的 `tool_call` 只保证有 toolCallId/title/status，
// **工具名就在 title 里**（界面那侧也是拿它当标签显示的，见 `internal/api/agent.go` 的 emitUpdate）。
// 所以这里打印的与用户在界面上看到的是同一个字符串——两边不会各说各话。
func noteUpdate(u acp.Update) {
	var body struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		Text       string `json:"text"`
		Content    struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(u.Raw, &body); err != nil {
		return
	}
	switch u.Kind() {
	case "tool_call", "tool_call_update":
		if body.Title == "" {
			return
		}
		noteMu.Lock()
		defer noteMu.Unlock()
		if toolSeen == nil {
			toolSeen = map[string]string{}
		}
		if _, ok := toolSeen[body.Title]; !ok {
			toolOrder = append(toolOrder, body.Title)
		}
		toolSeen[body.Title] = body.Status
		toolCalls++
		if strings.Contains(strings.ToLower(body.Title), "ssot") {
			oursCalls++
		}
		fmt.Printf("  [工具] %-12s %s\n", body.Status, body.Title)
	case "agent_message_chunk", "agent_message":
		t := body.Text
		if t == "" {
			t = body.Content.Text
		}
		noteMu.Lock()
		answer.WriteString(t)
		noteMu.Unlock()
	}
}

// runAsk 起后端 → 建一个会话 → 发一句 → 把这一轮的工具调用与结论打出来。
func runAsk(svc *agentapp.Service, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("\n=== 起后端（不建会话）===")
	start := time.Now()
	st, err := svc.EnsureBackend(ctx)
	if err != nil {
		fmt.Printf("起后端失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("起来了：agent=%s version=%s（%.1f 秒）\n", st.Agent.Name, st.Agent.Version, time.Since(start).Seconds())

	fmt.Println("\n=== 建会话 ===")
	st, err = svc.NewSession(ctx)
	if err != nil {
		fmt.Printf("建会话失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("会话 %s\n", short(st.SessionID))

	fmt.Printf("\n=== 发送（这一轮会花额度）===\n%s\n\n", prompt)
	start = time.Now()
	stop, err := svc.Send(ctx, prompt)
	if err != nil {
		fmt.Printf("发送失败：%v\n", err)
		_ = svc.Stop()
		os.Exit(1)
	}
	fmt.Printf("\n这一轮结束：stop=%q（%.1f 秒）\n", stop, time.Since(start).Seconds())
	reportTools()

	if err := svc.Stop(); err != nil {
		fmt.Printf("停后端失败：%v\n", err)
	}
}

// reportTools 打这一轮的工具清单与结论。
//
// ⚠️ 结论只对**这一轮**成立：模型这一轮没调工具，就什么也证明不了（如实说出来，
// 别把「没出现 grep」当成「grep 没有了」）。
func reportTools() {
	noteMu.Lock()
	names := append([]string(nil), toolOrder...)
	statuses := map[string]string{}
	for k, v := range toolSeen {
		statuses[k] = v
	}
	n := toolCalls
	ours := oursCalls
	text := strings.TrimSpace(answer.String())
	noteMu.Unlock()

	fmt.Println("\n=== 这一轮用了哪些工具 ===")
	if len(names) == 0 {
		fmt.Println("  （一个工具都没调）")
	} else {
		sort.Strings(names) // 去重后的清单按名字排，出现顺序不好读
		for _, name := range names {
			fmt.Printf("  %-40s %s\n", name, statuses[name])
		}
	}
	if text != "" {
		runes := []rune(text)
		if len(runes) > 300 {
			text = string(runes[:300]) + "…"
		}
		fmt.Printf("\n=== 最后的回答（截 300 字）===\n%s\n", text)
	}

	fmt.Println("\n=== 结论 ===")
	if len(names) == 0 {
		fmt.Println("  这一轮没调工具 —— **证明不了工具集**，得换个非用工具不可的问法再跑一次")
		return
	}
	// DSH 自带的那批（写与搜索）只要冒出来一个，就说明没堵住。
	banned := []string{"pwsh", "bash", "grep", "glob", "str_replace", "str-replace", "read_file", "write_file"}
	var hit []string
	for _, name := range names {
		low := strings.ToLower(name)
		for _, w := range banned {
			if strings.Contains(low, w) {
				hit = append(hit, name)
				break
			}
		}
	}
	if len(hit) > 0 {
		fmt.Printf("  ✗ 出现了 DSH 自带的工具：%s —— 后端 profile 没堵住\n", strings.Join(hit, "、"))
		return
	}
	fmt.Printf("  ✓ 没出现 DSH 自带的写/搜索工具：%d 次调用里 %d 次走的是我们自己的工具（其余是后端自带的非文件工具）\n",
		n, ours)
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
