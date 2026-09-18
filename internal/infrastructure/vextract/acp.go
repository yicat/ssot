// ACP 实现：抽取调用走我们自己的 ACP 客户端（提示词放在协议消息里）。
//
// 为什么不用 spike 那条 headless one-shot：那条路的任务文本是**命令行参数**，
// Windows 上限 32,767 字符，一次只放得下 5～6 个 2000 token 的块——
// 与 spec §三.1「必须多块合一次」直接冲突。理由与数字见
// docs/notes/extraction-spike.md 的「命令行长度」一节。
//
// 为什么**每批新开一个会话**：会话历史会把输入越滚越大（spike 里「关推理省下 47% 输入」
// 就是这个道理的极端例子）。抽取每批都是独立的一问一答，不该背上一批的上下文。
package vextract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ngnl5/ssot/internal/infrastructure/acp"
)

// ACPOptions 是「用 ACP 跑抽取」需要的参数。
type ACPOptions struct {
	// Command / Args / Env 与起聊天后端时同一套（见 appconfig.Agent.Args/Env）：
	// Args 里可以再追加 `--patch <抽取 overlay>`（关推理 + 收工具集）。
	Command string
	Args    []string
	Env     []string
	// Dir 是会话的工作目录（必须是绝对路径，后端会校验）。
	Dir string
	// Timeout 是**每次调用**的上限（0 = 不掐，靠 ctx 取消）。
	Timeout time.Duration
	// OnLog 收后端 stdout 上不是协议的行（诊断）。
	OnLog func(string)
	// OnUsage 收 usage_update 的原始 JSON（token 用量）。字段形状由后端定，
	// 所以这里**原样**交出去，让调用方先打出来看，而不是猜着解析。
	OnUsage func(raw json.RawMessage)
}

// ACPCompleter 用 ACP 完成一次「提示词 → 正文」。
//
// ⚠️ 只能**顺序**使用（同一时刻只允许一个调用在跑）：它共享一个后端进程与一段收集缓冲。
type ACPCompleter struct {
	opt ACPOptions

	mu     sync.Mutex
	client *acp.Client
	buf    *strings.Builder // 当前这一轮收集到的正文
	usage  json.RawMessage  // 最后一次 usage_update（原样）
	agent  string           // 握手后知道是谁在回答
}

// NewACPCompleter 构造（不启动进程，第一次 Complete 时才启动）。
func NewACPCompleter(opt ACPOptions) *ACPCompleter { return &ACPCompleter{opt: opt} }

// Agent 返回握手得到的后端名字（没握过手时为空）。
func (a *ACPCompleter) Agent() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agent
}

// LastUsage 返回最后一次 usage_update 的原始 JSON（没有就是 nil）。
func (a *ACPCompleter) LastUsage() json.RawMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.usage
}

// Complete 发一次提示词，返回助手这一轮的**正文**（不是 stopReason）。
//
// 正文是从流式更新里收集的（ACP 的 session/prompt 只回 stopReason）。
func (a *ACPCompleter) Complete(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("提示词是空的")
	}
	a.mu.Lock()
	if a.buf != nil {
		a.mu.Unlock()
		return "", fmt.Errorf("上一次调用还没结束——这个 Completer 只能顺序用")
	}
	a.buf = &strings.Builder{}
	a.buf.Grow(len(prompt) / 2)
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.buf = nil
		a.mu.Unlock()
	}()

	client, err := a.ensureClient(ctx)
	if err != nil {
		return "", err
	}

	callCtx := ctx
	if a.opt.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, a.opt.Timeout)
		defer cancel()
	}

	sess, err := client.NewSession(callCtx, a.opt.Dir, nil) // 不给 MCP：提示词里已经有正文，不需要工具
	if err != nil {
		return "", fmt.Errorf("开抽取会话失败：%w", err)
	}
	// 关会话用**不随 ctx 取消**的上下文：主流程超时了也要把会话收干净。
	defer func() {
		_ = client.CloseSession(context.WithoutCancel(ctx), sess.ID)
	}()

	if _, err := client.Prompt(callCtx, sess.ID, prompt); err != nil {
		return "", fmt.Errorf("抽取调用失败：%w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.buf == nil {
		return "", nil
	}
	return a.buf.String(), nil
}

// Close 停掉后端进程（用完一定要调，否则会留一个后台进程）。
func (a *ACPCompleter) Close() error {
	a.mu.Lock()
	client := a.client
	a.client = nil
	a.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Stop()
}

// ensureClient 懒启动后端并握手（只做一次）。
func (a *ACPCompleter) ensureClient(ctx context.Context) (*acp.Client, error) {
	a.mu.Lock()
	if a.client != nil {
		c := a.client
		a.mu.Unlock()
		return c, nil
	}
	a.mu.Unlock()

	if a.opt.Command == "" {
		return nil, fmt.Errorf("没给后端命令（抽取要调模型：见 appconfig 的 agent 配置）")
	}
	client, err := acp.Start(acp.Config{
		Command: a.opt.Command,
		Args:    a.opt.Args,
		Env:     a.opt.Env,
		Dir:     a.opt.Dir,
		OnUpdate: func(u acp.Update) {
			a.onUpdate(u)
		},
		// 权限一律拒绝：抽取不需要工具，也不该在无人值守时被批准任何事。
		OnPermission: nil,
		OnLog:        a.opt.OnLog,
	})
	if err != nil {
		return nil, fmt.Errorf("起后端失败：%w", err)
	}
	caps, err := client.Initialize(ctx)
	if err != nil {
		_ = client.Stop()
		return nil, fmt.Errorf("握手失败：%w", err)
	}
	_ = caps
	a.mu.Lock()
	a.client = client
	a.agent = client.Agent().Name
	a.mu.Unlock()
	return client, nil
}

// onUpdate 在客户端的读循环里被调用：只做「攒正文 / 记用量」这点轻活。
func (a *ACPCompleter) onUpdate(u acp.Update) {
	switch u.Kind() {
	case "agent_message_chunk":
		var body struct {
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(u.Raw, &body) != nil || body.Content.Text == "" {
			return
		}
		a.mu.Lock()
		if a.buf != nil {
			a.buf.WriteString(body.Content.Text)
		}
		a.mu.Unlock()
	case "usage_update":
		raw := append(json.RawMessage(nil), u.Raw...)
		a.mu.Lock()
		a.usage = raw
		a.mu.Unlock()
		if a.opt.OnUsage != nil {
			a.opt.OnUsage(raw)
		}
	}
}
