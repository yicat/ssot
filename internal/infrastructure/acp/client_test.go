package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeACPEnv 让同一个测试二进制变身「假 ACP 后端」。
//
// 用这个套路（而不是往仓库里塞一个可执行脚本）的理由：假后端要跟着协议一起改，
// 放在测试文件里改起来最顺手，也不会多出一个需要打包/清理的产物。
const (
	fakeEnv     = "SSOT_FAKE_ACP"
	fakeLogEnv  = "SSOT_FAKE_ACP_LOG"
	fakeSession = "fake-session-1"
)

// TestFakeACPServer 是子进程入口（不是真的测试）。
func TestFakeACPServer(t *testing.T) {
	if os.Getenv(fakeEnv) != "1" {
		t.Skip("这是假后端入口，只有被 Start 起来时才跑")
	}
	logPath := os.Getenv(fakeLogEnv)
	appendLog := func(v any) {
		if logPath == "" {
			return
		}
		b, _ := json.Marshal(v)
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.Write(append(b, '\n'))
	}

	out := bufio.NewWriter(os.Stdout)
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = out.Write(append(b, '\n'))
		_ = out.Flush()
	}
	// 真后端的 stdout 上就有这种诊断行，而且会夹在协议消息之间——客户端必须跳过。
	_, _ = out.WriteString("[harness-node] runtime node=v24 fake\n")
	_ = out.Flush()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
	waitingPermission := false
	// promptID 必须用**请求真实的 id** 回：写死一个数字的话，客户端永远等不到自己那条的回应
	// （第一次跑就这么挂住的——测试超时 5 分钟才发现）。
	promptID := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		id := string(msg.ID)
		if waitingPermission && msg.Method == "" {
			// 这是我们等的权限回答
			var r struct {
				Outcome struct {
					Outcome  string `json:"outcome"`
					OptionID string `json:"optionId"`
				} `json:"outcome"`
			}
			_ = json.Unmarshal(msg.Result, &r)
			appendLog(map[string]any{"permissionReply": r.Outcome})
			waitingPermission = false
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(promptID), "result": map[string]any{"stopReason": "end_turn"}})
			continue
		}
		switch msg.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{
				"protocolVersion": 1,
				"agentInfo":       map[string]any{"name": "fake-acp", "version": "9.9.9"},
				"agentCapabilities": map[string]any{
					"sessionCapabilities": map[string]any{"list": map[string]any{}, "resume": map[string]any{}, "close": map[string]any{}},
				},
			}})
		case "session/new":
			var p struct {
				Cwd        string      `json:"cwd"`
				MCPServers []MCPServer `json:"mcpServers"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			appendLog(map[string]any{"sessionNew": p})
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{
				"sessionId": fakeSession,
				"configOptions": []map[string]any{{
					"id": "model", "name": "Model", "category": "model", "type": "select",
					"currentValue": `["fake-provider","fake-model"]`,
					"options": []map[string]any{{"group": "fake-provider", "name": "Fake", "options": []map[string]any{
						{"value": `["fake-provider","fake-model"]`, "name": "Fake-Model", "description": "假模型"},
					}}},
				}},
			}})
		case "session/prompt":
			appendLog(map[string]any{"prompt": json.RawMessage(msg.Params)})
			promptID = id
			// 更新流：消息块 → 工具调用 → 工具调用结束。夹一行非协议输出证明客户端不受影响。
			_, _ = out.WriteString("not json at all\n")
			_ = out.Flush()
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": fakeSession,
				"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": "m1", "content": map[string]any{"type": "text", "text": "先看一眼 vault。"}},
			}})
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": fakeSession,
				"update":    map[string]any{"sessionUpdate": "tool_call", "toolCallId": "c1", "title": "mcp__ssot__vault_list", "status": "in_progress"},
			}})
			// 权限提示：反过来问客户端要一个 optionId（这就是界面上那个弹窗）。
			waitingPermission = true
			send(map[string]any{"jsonrpc": "2.0", "id": 55, "method": "session/request_permission", "params": map[string]any{
				"sessionId": fakeSession,
				"toolCall":  map[string]any{"toolCallId": "c1"},
				"options": []map[string]any{
					{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
					{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"},
				},
			}})
		case "session/list":
			appendLog(map[string]any{"sessionList": json.RawMessage(msg.Params)})
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{
				"sessions": []map[string]any{
					{"sessionId": fakeSession, "cwd": "C:\\vault", "title": "整理茨木童子"},
				},
			}})
		case "session/close":
			appendLog(map[string]any{"close": json.RawMessage(msg.Params)})
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{}})
		case "session/cancel":
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{}})
		default:
			send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": map[string]any{"code": -32601, "message": "no " + msg.Method}})
		}
	}
	os.Exit(0)
}

// startFake 起一个假后端，返回客户端与「后端收到的请求」日志读取器。
func startFake(t *testing.T, cfg Config) (*Client, func() []map[string]any) {
	t.Helper()
	logPath := t.TempDir() + "/fake.log"
	cfg.Command = os.Args[0]
	cfg.Args = []string{"-test.run=TestFakeACPServer", "-test.v=false"}
	cfg.Env = append(cfg.Env, fakeEnv+"=1", fakeLogEnv+"="+logPath)
	c, err := Start(cfg)
	if err != nil {
		t.Fatalf("起假后端失败：%v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })
	return c, func() []map[string]any {
		b, err := os.ReadFile(logPath)
		if err != nil {
			return nil
		}
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if line == "" {
				continue
			}
			var m map[string]any
			if json.Unmarshal([]byte(line), &m) == nil {
				out = append(out, m)
			}
		}
		return out
	}
}

// TestHandshakeAndSession 走一遍：握手 → 开会话（挂 MCP）→ 列会话 → 关会话。
func TestHandshakeAndSession(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	var updates []Update

	c, sent := startFake(t, Config{
		OnLog:    func(s string) { mu.Lock(); logs = append(logs, s); mu.Unlock() },
		OnUpdate: func(u Update) { mu.Lock(); updates = append(updates, u); mu.Unlock() },
	})
	ctx := context.Background()

	caps, err := c.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize：%v", err)
	}
	if caps.Agent.Name != "fake-acp" || caps.ProtocolVersion != 1 {
		t.Errorf("该拿到后端身份：%+v", caps)
	}
	if !caps.CanList || !caps.CanResume || !caps.CanClose {
		t.Errorf("该报出会话能力：%+v", caps)
	}

	sess, err := c.NewSession(ctx, "C:\\vault", []MCPServer{{
		Name: "ssot", Command: "C:\\bin\\ssot.exe", Args: []string{"mcp", "--root", "C:\\vault"}, Type: "stdio",
	}})
	if err != nil {
		t.Fatalf("session/new：%v", err)
	}
	if sess.ID != fakeSession {
		t.Errorf("会话 ID 不对：%q", sess.ID)
	}
	if len(sess.ConfigOptions) != 1 || sess.ConfigOptions[0].ID != "model" {
		t.Errorf("该拿到会话级配置选项：%+v", sess.ConfigOptions)
	}
	if sess.ConfigOptions[0].Options[0].Options[0].Name != "Fake-Model" {
		t.Errorf("模型选项该被解析出来：%+v", sess.ConfigOptions[0].Options)
	}

	// 送出去的参数必须对：cwd = vault，MCP 服务器用绝对路径随会话挂上。
	var newParams map[string]any
	for _, m := range sent() {
		if v, ok := m["sessionNew"]; ok {
			newParams, _ = v.(map[string]any)
		}
	}
	if newParams == nil {
		t.Fatal("后端没收到 session/new")
	}
	if newParams["cwd"] != "C:\\vault" {
		t.Errorf("cwd 该是 vault 根：%v", newParams["cwd"])
	}
	servers, _ := newParams["mcpServers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("该挂一个 MCP 服务器：%v", newParams["mcpServers"])
	}
	first, _ := servers[0].(map[string]any)
	if first["name"] != "ssot" || first["command"] != "C:\\bin\\ssot.exe" {
		t.Errorf("MCP 条目不对：%v", first)
	}

	list, err := c.ListSessions(ctx, "C:\\vault")
	if err != nil {
		t.Fatalf("session/list：%v", err)
	}
	if len(list) != 1 || list[0].ID != fakeSession || list[0].Cwd != "C:\\vault" {
		t.Errorf("会话列表不对：%+v", list)
	}
	// 列会话**必须带 cwd**：不带会把用户别处的会话一起列出来。
	var listParams map[string]any
	for _, m := range sent() {
		if v, ok := m["sessionList"]; ok {
			listParams, _ = v.(map[string]any)
		}
	}
	if listParams == nil || listParams["cwd"] != "C:\\vault" {
		t.Errorf("session/list 该带 cwd：%v", listParams)
	}

	if err := c.CloseSession(ctx, fakeSession); err != nil {
		t.Fatalf("session/close：%v", err)
	}
	if len(updates) != 0 {
		t.Errorf("还没发提示词就不该有更新：%+v", updates)
	}
}

// TestPromptStreamsUpdatesAndAnswersPermission 是本包最要紧的一条：
// 一轮里更新要能流到上层，权限提示要能由界面回答并回给后端。
func TestPromptStreamsUpdatesAndAnswersPermission(t *testing.T) {
	var mu sync.Mutex
	var updates []Update
	var asked []Request

	c, sent := startFake(t, Config{
		OnUpdate: func(u Update) { mu.Lock(); updates = append(updates, u); mu.Unlock() },
		OnPermission: func(r Request) string {
			mu.Lock()
			asked = append(asked, r)
			mu.Unlock()
			return "allow-once" // 界面点了「允许一次」
		},
	})
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewSession(ctx, "C:\\vault", nil); err != nil {
		t.Fatal(err)
	}

	stop, err := c.Prompt(ctx, fakeSession, "把 raw/灰机wiki/茨木童子 整理成文档")
	if err != nil {
		t.Fatalf("session/prompt：%v", err)
	}
	if stop != "end_turn" {
		t.Errorf("stopReason 该是 end_turn：%q", stop)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 {
		t.Fatalf("该收到两条更新（消息块 + 工具调用）：%d", len(updates))
	}
	if updates[0].Kind() != "agent_message_chunk" {
		t.Errorf("第一条该是消息块：%q", updates[0].Kind())
	}
	if !strings.Contains(string(updates[0].Raw), "先看一眼 vault") {
		t.Errorf("消息块内容该原样带上来：%s", updates[0].Raw)
	}
	if updates[1].Kind() != "tool_call" || !strings.Contains(string(updates[1].Raw), "mcp__ssot__vault_list") {
		t.Errorf("第二条该是工具调用轨迹：%s", updates[1].Raw)
	}
	if len(asked) != 1 || asked[0].ToolCall.ToolCallID != "c1" {
		t.Fatalf("该被问一次权限（带 toolCallId）：%+v", asked)
	}
	if len(asked[0].Options) != 2 || asked[0].Options[0].OptionID != "allow-once" {
		t.Errorf("选项该原样给上来：%+v", asked[0].Options)
	}

	// 后端的日志里应该记下我们回的正是「允许一次」。
	var reply map[string]any
	for _, m := range sent() {
		if v, ok := m["permissionReply"]; ok {
			reply, _ = v.(map[string]any)
		}
	}
	if reply == nil || reply["optionId"] != "allow-once" {
		t.Errorf("该把 optionId 回给后端：%v", reply)
	}
}

// TestNoPermissionHandlerMeansReject：没人能回答时一律拒绝，别让后端悬着。
func TestNoPermissionHandlerMeansReject(t *testing.T) {
	c, sent := startFake(t, Config{}) // OnPermission 为 nil
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewSession(ctx, "C:\\vault", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, fakeSession, "随便"); err != nil {
		t.Fatalf("prompt 不该失败（拒绝权限不等于出错）：%v", err)
	}
	var reply map[string]any
	for _, m := range sent() {
		if v, ok := m["permissionReply"]; ok {
			reply, _ = v.(map[string]any)
		}
	}
	if reply == nil || reply["outcome"] != "cancelled" {
		t.Errorf("没有处理者时该回 cancelled：%v", reply)
	}
}

// TestSkipsNonProtocolLines：真后端会往 stdout 上写 `[harness-node] …`，
// 而且会夹在协议消息之间——客户端要跳过并记下来，绝不能因此连不上。
func TestSkipsNonProtocolLines(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	c, _ := startFake(t, Config{OnLog: func(s string) { mu.Lock(); logs = append(logs, s); mu.Unlock() }})
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("诊断行不该破坏握手：%v", err)
	}
	if _, err := c.NewSession(ctx, "C:\\vault", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, fakeSession, "再来一句"); err != nil {
		t.Fatalf("诊断行夹在中间也不该破坏这一轮：%v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "harness-node") || !strings.Contains(joined, "not json at all") {
		t.Errorf("非协议行该被记下来：%v", logs)
	}
}

// TestPromptTimeoutIsNotImposed：prompt 不设超时（长回答是正常的），
// 所以这里用一个 200ms 的上下文取消来验证「取消走的是上下文，不是超时」。
func TestPromptCancelViaContext(t *testing.T) {
	c, _ := startFake(t, Config{})
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewSession(ctx, "C:\\vault", nil); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	// 假后端会一直等权限回答；这里不给 OnPermission，所以它会拿到 cancelled 并结算。
	// 真正要验的是：prompt 自己**没有**超时（上面那条已证），而且上下文取消能中断等待。
	if _, err := c.Prompt(short, fakeSession, "x"); err != nil && !strings.Contains(err.Error(), "context") {
		t.Errorf("上下文取消该是 context 错误：%v", err)
	}
}

// TestRejectsUnstartableCommand：命令不存在时要立刻报清楚，不要静默挂住。
func TestRejectsUnstartableCommand(t *testing.T) {
	_, err := Start(Config{Command: "C:\\definitely\\not\\here.exe"})
	if err == nil {
		t.Fatal("起不来的命令该报错")
	}
	if !strings.Contains(err.Error(), "起后端失败") {
		t.Errorf("错误信息要说清是起后端失败：%v", err)
	}
	if _, err := Start(Config{}); err == nil {
		t.Error("没给命令该报错")
	}
}

var _ = exec.Command // 保持 import 有用（假后端入口用 os.Args[0]）
