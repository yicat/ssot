// Package acp 是 Agent Client Protocol（v1）的客户端：起一个 ACP 后端进程，
// 用换行分隔的 JSON-RPC 跟它说话。
//
// 为什么是 ACP：它是「可替换后端」这条线的落地处（docs/specs/agent.spec.md §7）——
// 换后端只换这一层，能力层（application/vaultapp）与四个 skill 都不动。
//
// 形状都是在本机对着真后端（`dsh --profile acp`）探出来的，不是照记忆写的：
//
//	initialize              → {protocolVersion:1, agentInfo, agentCapabilities{sessionCapabilities{list,resume,close}}}
//	session/new {cwd,mcpServers} → {sessionId, configOptions[...]}   ← MCP 服务器按会话挂
//	session/list {cwd?}     → {sessions:[{sessionId,cwd,...}]}       ← 列表是共享的，必须按 cwd 过滤
//	session/prompt {sessionId,prompt:[{type:"text",text}]} → {stopReason}
//	session/update          ← 服务端推的语义更新（消息块 / thought / 工具生命周期 / 用量 / 配置变化）
//	session/request_permission ← 服务端**反过来问我们**的请求，必须回一个 optionId
//
// ⚠️ 两个坑（都实测过）：
//  1. 真后端的 **stdout 里混着 `[harness-node] …` 诊断行**（甚至夹在两条协议消息之间）。
//     客户端必须跳过非 JSON 行并记下来，不能当成协议错误——否则一连就炸。
//  2. 会话列表是**全机器共享**的（同一个 DSH_HOME）。列会话一定要带 cwd，
//     不然会把用户自己在别处写代码的会话一起列进来。
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProtocolVersion 是我们说的 ACP 协议版本（真后端回的就是 1）。
const ProtocolVersion = 1

// Config 是起后端需要的东西。
type Config struct {
	// Command 与 Args 是后端进程的命令行（Windows 上是 DSH Desktop.exe + harness 入口 + dsh bin）。
	Command string
	Args    []string
	// Env 是追加给子进程的环境变量（例如 ELECTRON_RUN_AS_NODE=1）。
	Env []string
	// Dir 是子进程的工作目录（空 = 继承）。
	Dir string

	// OnUpdate 收流式更新（在客户端自己的 goroutine 里被调用，实现里别做重活）。
	OnUpdate func(Update)
	// OnPermission 是权限提示：返回 optionId（如 "allow-once"）；返回空字符串表示取消。
	// 由界面弹窗问人——**这就是「批准必须是人」在聊天界面里的落地**。
	//   - 为 nil 时一律拒绝（安全默认：没人能回答，就当不许）。
	//   - 它可能被阻塞很久（人在看），所以调用在独立 goroutine 里，不挡协议流。
	OnPermission func(Request) string
	// OnLog 收 stdout 上那些**不是协议**的行（`[harness-node] …` 诊断）。为 nil 时丢弃。
	OnLog func(string)
}

// Client 是一个跑起来的 ACP 客户端。
type Client struct {
	cfg Config

	cmd   *exec.Cmd
	stdin io.WriteCloser

	mu      sync.Mutex
	nextID  int
	waiters map[int]chan rpcReply
	closed  bool

	// agentInfo 存 initialize 的结果，供上层显示（谁在跟我说话）。
	agentInfo AgentInfo
}

type rpcReply struct {
	result json.RawMessage
	err    *rpcError
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("后端错误 %d：%s", e.Code, e.Message) }

type serverMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// AgentInfo 是后端自报的名字与版本。
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Start 起后端进程并开始读协议流。
func Start(cfg Config) (*Client, error) {
	if cfg.Command == "" {
		return nil, errors.New("acp: 必须给后端命令（配置页里的 DSH 入口）")
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = append(cmd.Environ(), cfg.Env...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// stderr 单独收：后端报错多半在这儿，混进 stdout 会污染协议流。
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: 起后端失败（%s）：%w", cfg.Command, err)
	}
	c := &Client{cfg: cfg, cmd: cmd, stdin: stdin, waiters: map[int]chan rpcReply{}}
	go c.readLoop(stdout)
	go c.logLoop(stderr, "stderr")
	return c, nil
}

// readLoop 一行一条消息，直到进程退出。
func (c *Client) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg serverMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// 真后端会在 stdout 上夹诊断行——跳过并记下来，不当协议错误（踩过）。
			c.log("stdout 非协议行：" + line)
			continue
		}
		c.dispatch(msg)
	}
	c.failAll(errors.New("acp: 后端连接已关闭"))
}

func (c *Client) logLoop(r io.Reader, tag string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			c.log(tag + "：" + line)
		}
	}
}

func (c *Client) log(s string) {
	if c.cfg.OnLog != nil {
		c.cfg.OnLog(s)
	}
}

func (c *Client) dispatch(msg serverMessage) {
	switch {
	case msg.Method != "" && len(msg.ID) > 0:
		// 服务端反过来问我们（权限提示）。**不能挡住读循环**：人在看弹窗可能要很久。
		go c.answer(msg)
	case msg.Method != "":
		// 通知：session/update
		if msg.Method == "session/update" {
			var p struct {
				SessionID string          `json:"sessionId"`
				Update    json.RawMessage `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &p); err == nil && c.cfg.OnUpdate != nil {
				c.cfg.OnUpdate(Update{SessionID: p.SessionID, Raw: p.Update})
			}
		}
	case len(msg.ID) > 0:
		var id int
		if err := json.Unmarshal(msg.ID, &id); err != nil {
			return
		}
		c.mu.Lock()
		w := c.waiters[id]
		delete(c.waiters, id)
		c.mu.Unlock()
		if w != nil {
			w <- rpcReply{result: msg.Result, err: msg.Error}
		}
	}
}

// answer 回一个服务端请求（权限提示）。
func (c *Client) answer(msg serverMessage) {
	var id int
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		return
	}
	optionID := ""
	if msg.Method == "session/request_permission" && c.cfg.OnPermission != nil {
		var req Request
		if err := json.Unmarshal(msg.Params, &req); err == nil {
			optionID = c.cfg.OnPermission(req)
		}
	}
	// 没人回答 / 没有可选项 → 明确回 cancelled，别让后端悬着。
	var result any
	if optionID == "" {
		result = map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	} else {
		result = map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
	}
	_ = c.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (c *Client) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("acp: 连接已关闭")
	}
	if _, err := c.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// call 发一个请求并等回应。timeout<=0 表示不设超时（prompt 可能跑几分钟，不能掐）。
func (c *Client) call(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("acp: 连接已关闭")
	}
	c.nextID++
	id := c.nextID
	ch := make(chan rpcReply, 1)
	c.waiters[id] = ch
	c.mu.Unlock()

	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := c.write(req); err != nil {
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return nil, err
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timeoutCh = t.C
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return r.result, nil
	case <-timeoutCh:
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("acp: %s 超时（%s）", method, timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) failAll(err error) {
	c.mu.Lock()
	waiters := c.waiters
	c.waiters = map[int]chan rpcReply{}
	c.closed = true
	c.mu.Unlock()
	for _, w := range waiters {
		w <- rpcReply{err: &rpcError{Code: -32000, Message: err.Error()}}
	}
}

// Stop 关掉连接与后端进程。
func (c *Client) Stop() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}

// ── 协议方法 ──────────────────────────────────────────────────────────────

// Initialize 握手。后端会自报能力（能不能 list/resume/close 会话、支持哪些提示词类型）。
func (c *Client) Initialize(ctx context.Context) (Caps, error) {
	raw, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion":    ProtocolVersion,
		"clientCapabilities": map[string]any{},
	}, 30*time.Second)
	if err != nil {
		return Caps{}, err
	}
	var res struct {
		ProtocolVersion   int       `json:"protocolVersion"`
		AgentInfo         AgentInfo `json:"agentInfo"`
		AgentCapabilities struct {
			MCPCapabilities struct {
				HTTP bool `json:"http"`
			} `json:"mcpCapabilities"`
			SessionCapabilities struct {
				List   *struct{} `json:"list"`
				Resume *struct{} `json:"resume"`
				Close  *struct{} `json:"close"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return Caps{}, err
	}
	c.mu.Lock()
	c.agentInfo = res.AgentInfo
	c.mu.Unlock()
	return Caps{
		ProtocolVersion: res.ProtocolVersion,
		Agent:           res.AgentInfo,
		CanList:         res.AgentCapabilities.SessionCapabilities.List != nil,
		CanResume:       res.AgentCapabilities.SessionCapabilities.Resume != nil,
		CanClose:        res.AgentCapabilities.SessionCapabilities.Close != nil,
	}, nil
}

// Caps 是后端自报的能力。
type Caps struct {
	ProtocolVersion int
	Agent           AgentInfo
	CanList         bool
	CanResume       bool
	CanClose        bool
}

// MCPServer 是**按会话挂**的一个 MCP 服务器。
//
// 这是 App 内置聊天不需要任何全局配置的原因：MCP 条目随 session/new 一起送过去
// （docs/specs/agent.spec.md §7）。command 必须是**绝对路径**，后端会校验。
type MCPServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	Type    string   `json:"type"` // 只用 "stdio"
}

// Session 是一个新开的会话（含后端给的配置选项，如模型）。
type Session struct {
	ID            string         `json:"sessionId"`
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// ConfigOption 是会话级配置（模型、推理强度）。**不进配置页**——它是这一次会话的选择。
type ConfigOption struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Type         string `json:"type"`
	CurrentValue any    `json:"currentValue"`
	Options      []struct {
		Group   string `json:"group"`
		Name    string `json:"name"`
		Options []struct {
			Value       any    `json:"value"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"options"`
}

// NewSession 开一个新会话：cwd 是工作区（我们用 vault 根），mcp 是随会话挂的 MCP 服务器。
func (c *Client) NewSession(ctx context.Context, cwd string, mcp []MCPServer) (Session, error) {
	if mcp == nil {
		mcp = []MCPServer{}
	}
	raw, err := c.call(ctx, "session/new", map[string]any{"cwd": cwd, "mcpServers": mcp}, 60*time.Second)
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Summary 是会话列表里的一条。
type Summary struct {
	ID        string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
}

// ListSessions 列会话。**一定带 cwd**：会话是全机器共享的，
// 不带筛选会把用户自己在别处写代码的会话一起列进来（踩过）。
func (c *Client) ListSessions(ctx context.Context, cwd string) ([]Summary, error) {
	params := map[string]any{}
	if cwd != "" {
		params["cwd"] = cwd
	}
	raw, err := c.call(ctx, "session/list", params, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var res struct {
		Sessions []Summary `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Sessions, nil
}

// Prompt 发一句提示词，**阻塞到这一轮结束**，返回 stopReason。
//
// 不设超时：一轮里可能有多次模型调用与工具调用，掐超时会把正常的长回答判成失败。
// 要停就 Cancel（那是协议里的正规路径）。
func (c *Client) Prompt(ctx context.Context, sessionID, text string) (string, error) {
	raw, err := c.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}, 0)
	if err != nil {
		return "", err
	}
	var res struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	return res.StopReason, nil
}

// Cancel 取消当前这一轮。
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	_, err := c.call(ctx, "session/cancel", map[string]any{"sessionId": sessionID}, 15*time.Second)
	return err
}

// CloseSession 关掉一个会话（不影响别的会话）。
func (c *Client) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.call(ctx, "session/close", map[string]any{"sessionId": sessionID}, 30*time.Second)
	return err
}

// SetConfigOption 改会话级配置（例如模型）。value 用 Options 里给的原值。
func (c *Client) SetConfigOption(ctx context.Context, sessionID, optionID string, value any) ([]ConfigOption, error) {
	raw, err := c.call(ctx, "session/set_config_option", map[string]any{
		"sessionId": sessionID, "configId": optionID, "value": value,
	}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var res struct {
		ConfigOptions []ConfigOption `json:"configOptions"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.ConfigOptions, nil
}

// Agent 返回后端自报的身份（Initialize 之后才有值）。
func (c *Client) Agent() AgentInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentInfo
}

// Request 是服务端反过来问我们的请求（现在只有权限提示）。
type Request struct {
	SessionID string `json:"sessionId"`
	ToolCall  struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
	} `json:"toolCall"`
	Options []Option `json:"options"`
}

// Option 是权限提示里的一个选项（allow-once / reject-once）。
type Option struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// Update 是一条语义更新。Raw 原样留着：具体有哪些字段由后端决定，
// 上层按 sessionUpdate 取用（agent_message_chunk / agent_thought_chunk / tool_call / …）。
type Update struct {
	SessionID string
	Raw       json.RawMessage
}

// Kind 返回这条更新是什么（agent_message_chunk 等），解析不出来时为空。
func (u Update) Kind() string {
	var probe struct {
		SessionUpdate string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(u.Raw, &probe); err != nil {
		return ""
	}
	return probe.SessionUpdate
}
