// Package agentapp 是「聊天」这件事的用例编排：起后端、开会话、发话、看更新、停。
//
// 它把 acp 客户端（infrastructure）与界面之间那层缝起来，并且**只做编排**：
// 协议形状在 acp 包里，界面形态在 docs/specs/agent.spec.md §7 里。
//
// 三条规矩落在这里：
//  1. **MCP 按会话挂**：起会话时把 `<CLIBin> mcp --root <vault>` 作为 mcpServers 送过去
//     ——App 内置聊天不需要 overlay，也不需要碰用户 profile。
//  2. **一次一个会话**：同时开多个会话会让「现在在跟谁说话」说不清（第一版不做，见 spec）。
//  3. **权限问人**：后端的 session/request_permission 交给 AskPermission（界面弹窗），
//     没人回答就一律拒绝——这是「批准必须是人」在聊天里的落地。
package agentapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ngnl5/ssot/internal/infrastructure/acp"
	"github.com/ngnl5/ssot/internal/infrastructure/dshstore"
)

// client 是本层用到的 ACP 能力。
//
// 抽成接口只有一个目的：**测试时不真起后端**（假后端在测试文件里，几行就够），
// 而生产路径就是 acp.Client。
type client interface {
	Initialize(ctx context.Context) (acp.Caps, error)
	NewSession(ctx context.Context, cwd string, mcp []acp.MCPServer) (acp.Session, error)
	ResumeSession(ctx context.Context, sessionID, cwd string, mcp []acp.MCPServer) (acp.Session, error)
	Prompt(ctx context.Context, sessionID, text string) (string, error)
	Cancel(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context, cwd string) ([]acp.Summary, error)
	CloseSession(ctx context.Context, sessionID string) error
	SetConfigOption(ctx context.Context, sessionID, optionID string, value any) ([]acp.ConfigOption, error)
	Stop() error
	Agent() acp.AgentInfo
}

// Config 是起服务需要的全部输入。
type Config struct {
	// Vault 是 session 的工作区（绝对路径）。
	Vault string
	// BackendCommand / BackendArgs / BackendEnv 是后端进程的命令行。
	BackendCommand string
	BackendArgs    []string
	BackendEnv     []string
	// MCPServer 是随会话挂上的 MCP 服务器（通常就是 `ssot-cli mcp --root <vault>`）。
	MCPServer acp.MCPServer
	// Actor 只是记给人看的（写进 git 的是 MCP 服务器那侧的 actor）。
	Actor string

	// OnUpdate 收流式更新（转发给界面）。
	OnUpdate func(acp.Update)
	// AskPermission 是权限提示：返回 optionId；返回空表示拒绝/取消。
	// 实现里通常要**阻塞等人在界面点**——所以它在独立 goroutine 里被调用，不挡协议流。
	AskPermission func(acp.Request) string
	// OnLog 收后端 stdout/stderr 上的非协议行（诊断）。
	OnLog func(string)

	// newClient 只给测试用（默认 acp.Start）。
	newClient func(acp.Config) (client, error)
}

// Status 是当前状态，给界面用。
type Status struct {
	Running bool   `json:"running"`
	Vault   string `json:"vault"`
	Actor   string `json:"actor"`
	// Agent 是后端自报的身份（起来了才有）。
	Agent acp.AgentInfo `json:"agent"`
	// SessionID 是当前会话。
	SessionID string `json:"sessionId"`
	// ConfigOptions 是会话级配置（模型、推理强度）。**不是**全局配置。
	ConfigOptions []acp.ConfigOption `json:"configOptions"`
	// Busy 表示正在跑一轮（这时只能停，不能再发）。
	Busy bool `json:"busy"`
	// Model 是当前选中的模型（从 ConfigOptions 里取出来，省得界面自己翻）。
	Model string `json:"model"`
}

// Service 是一个 vault 上的聊天会话。
type Service struct {
	cfg Config

	mu      sync.Mutex
	cli     client
	caps    acp.Caps
	session acp.Session
	busy    bool
	lastErr string
}

// New 构造 Service（不会立刻起后端——起后端是 Start 的事）。
func New(cfg Config) *Service {
	if cfg.newClient == nil {
		cfg.newClient = func(c acp.Config) (client, error) { return acp.Start(c) }
	}
	return &Service{cfg: cfg}
}

// Start 起后端并开一个**新**会话（已经起来了就直接返回现状，不重复起）。
func (s *Service) Start(ctx context.Context) (Status, error) {
	s.mu.Lock()
	if s.cli != nil {
		st := s.statusLocked()
		s.mu.Unlock()
		return st, nil
	}
	s.mu.Unlock()

	if s.cfg.BackendCommand == "" {
		return Status{}, errors.New("还没配 Agent 后端（设置里给 DSH 的安装目录）")
	}
	if s.cfg.Vault == "" {
		return Status{}, errors.New("还没有打开的项目——聊天要有个 vault 当工作区")
	}
	if s.cfg.MCPServer.Command == "" {
		return Status{}, errors.New("没有可用的 ssot CLI——MCP 服务器起不来（跑 wails3 task build:cli）")
	}

	cli, err := s.cfg.newClient(acp.Config{
		Command:      s.cfg.BackendCommand,
		Args:         s.cfg.BackendArgs,
		Env:          s.cfg.BackendEnv,
		OnUpdate:     s.cfg.OnUpdate,
		OnPermission: s.cfg.AskPermission,
		OnLog:        s.cfg.OnLog,
	})
	if err != nil {
		return Status{}, err
	}
	caps, err := cli.Initialize(ctx)
	if err != nil {
		_ = cli.Stop()
		return Status{}, fmt.Errorf("后端握手失败：%w", err)
	}
	// MCP 按会话挂：command 必须是绝对路径，后端会校验（acp 包里已记录）。
	sess, err := cli.NewSession(ctx, s.cfg.Vault, []acp.MCPServer{s.cfg.MCPServer})
	if err != nil {
		_ = cli.Stop()
		return Status{}, fmt.Errorf("开会话失败（工作区 %s）：%w", s.cfg.Vault, err)
	}

	s.mu.Lock()
	s.cli, s.caps, s.session, s.lastErr = cli, caps, sess, ""
	st := s.statusLocked()
	s.mu.Unlock()
	return st, nil
}

// Status 返回当前状态。
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() Status {
	st := Status{
		Running: s.cli != nil, Vault: s.cfg.Vault, Actor: s.cfg.Actor,
		SessionID: s.session.ID, ConfigOptions: s.session.ConfigOptions, Busy: s.busy,
	}
	if s.cli != nil {
		st.Agent = s.cli.Agent()
	}
	st.Model = modelOf(s.session.ConfigOptions)
	return st
}

// modelOf 从会话配置里取当前模型（值可能是字符串，也可能是 ["provider","model"] 这种元组）。
func modelOf(opts []acp.ConfigOption) string {
	for _, o := range opts {
		if o.ID != "model" {
			continue
		}
		switch v := o.CurrentValue.(type) {
		case string:
			return v
		case []any:
			parts := make([]string, 0, len(v))
			for _, p := range v {
				parts = append(parts, fmt.Sprint(p))
			}
			return fmt.Sprint(joinSlash(parts))
		default:
			if v != nil {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

func joinSlash(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " / "
		}
		out += p
	}
	return out
}

// Send 发一句，**阻塞到这一轮结束**（界面那边是异步等回调）。
func (s *Service) Send(ctx context.Context, text string) (string, error) {
	s.mu.Lock()
	cli, sid := s.cli, s.session.ID
	if cli == nil {
		s.mu.Unlock()
		return "", errors.New("还没有起后端（先 Start）")
	}
	if s.busy {
		s.mu.Unlock()
		return "", errors.New("上一轮还在跑——先停掉或者等它结束")
	}
	s.busy = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	stop, err := cli.Prompt(ctx, sid, text)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		return "", err
	}
	return stop, nil
}

// Cancel 停当前这一轮。
func (s *Service) Cancel(ctx context.Context) error {
	s.mu.Lock()
	cli, sid := s.cli, s.session.ID
	s.mu.Unlock()
	if cli == nil {
		return errors.New("还没有起后端")
	}
	return cli.Cancel(ctx, sid)
}

// Sessions 列这个 vault 的会话。
//
// **一定带 cwd**：会话是全机器共享的，不带筛选会把用户自己在别处写代码的会话列进来（踩过）。
func (s *Service) Sessions(ctx context.Context) ([]acp.Summary, error) {
	s.mu.Lock()
	cli := s.cli
	s.mu.Unlock()
	if cli == nil {
		return nil, errors.New("还没有起后端")
	}
	list, err := cli.ListSessions(ctx, s.cfg.Vault)
	if err != nil {
		return nil, err
	}
	// 标题：ACP 的 session/list **不回**，但 DSH 自己把标题落在盘上了（见 dshstore 的包注释）。
	// 读它，老会话也就有可读标题了——不用猜、不用调模型。读不到就保持空（界面显示「未命名会话」）。
	if titles := dshstore.Titles(s.dshHome()); len(titles) > 0 {
		for i := range list {
			if list[i].Title == "" {
				list[i].Title = titles[list[i].ID]
			}
		}
	}
	return list, nil
}

// dshHome 从后端环境里取 DSH_HOME（组合根就是这么把它传进来的）。
//
// 为什么不加 Config 字段：那要改组合根与调用方；这里只是**读一个已经在环境里的值**，
// 取不到就当没有（标题是便利信息）。将来 Config 有专门的字段再换。
func (s *Service) dshHome() string {
	for _, kv := range s.cfg.BackendEnv {
		if v, ok := strings.CutPrefix(kv, "DSH_HOME="); ok {
			return v
		}
	}
	return ""
}

// ensureBackend 起后端进程并握手，**不建会话**（会话由 Start 或 Resume 负责）。
func (s *Service) ensureBackend(ctx context.Context) error {
	s.mu.Lock()
	if s.cli != nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if s.cfg.BackendCommand == "" {
		return errors.New("还没配 Agent 后端（设置里给 DSH 的安装目录）")
	}
	if s.cfg.Vault == "" {
		return errors.New("还没有打开的项目——聊天要有个 vault 当工作区")
	}
	if s.cfg.MCPServer.Command == "" {
		return errors.New("没有可用的 ssot CLI——MCP 服务器起不来（跑 wails3 task build:cli）")
	}
	cli, err := s.cfg.newClient(acp.Config{
		Command:      s.cfg.BackendCommand,
		Args:         s.cfg.BackendArgs,
		Env:          s.cfg.BackendEnv,
		OnUpdate:     s.cfg.OnUpdate,
		OnPermission: s.cfg.AskPermission,
		OnLog:        s.cfg.OnLog,
	})
	if err != nil {
		return err
	}
	caps, err := cli.Initialize(ctx)
	if err != nil {
		_ = cli.Stop()
		return fmt.Errorf("后端握手失败：%w", err)
	}
	s.mu.Lock()
	s.cli, s.caps = cli, caps
	s.mu.Unlock()
	return nil
}

// Resume 恢复一个历史会话：换掉当前会话，后端进程不重开。
//
// 为什么不做成「并行开多个会话」：同时跟两个会话说话会让「现在在跟谁说话」说不清
// （spec §7 第一版：会话只列与恢复，不做并行）。
func (s *Service) Resume(ctx context.Context, sessionID string) (Status, error) {
	s.mu.Lock()
	cli, current := s.cli, s.session.ID
	s.mu.Unlock()
	if cli == nil {
		// **顺手把后端拉起来**：界面「打开应用就落回上次那个会话」靠的就是这条——
		// 走 Start() 会顺带建一个**新**会话（于是每次进来都多一个空会话，实测攒过一堆）；
		// 走 Resume 则只把后端拉起来、把会话切过去，不产生任何新会话。
		if err := s.ensureBackend(ctx); err != nil {
			return Status{}, err
		}
		s.mu.Lock()
		cli = s.cli
		s.mu.Unlock()
	}
	if cli == nil {
		return Status{}, errors.New("还没有起后端")
	}
	if sessionID == current {
		return s.Status(), nil
	}
	// MCP 要跟新会话重新挂一遍：resume 走的是同一套校验（command 必须绝对路径）。
	sess, err := cli.ResumeSession(ctx, sessionID, s.cfg.Vault, []acp.MCPServer{s.cfg.MCPServer})
	if err != nil {
		return Status{}, fmt.Errorf("恢复会话 %s 失败：%w", sessionID, err)
	}
	s.mu.Lock()
	s.session = sess
	st := s.statusLocked()
	s.mu.Unlock()
	return st, nil
}

// SetModel 改会话级模型（值是 configOptions 里给的原值）。
func (s *Service) SetModel(ctx context.Context, value any) (Status, error) {
	s.mu.Lock()
	cli, sid := s.cli, s.session.ID
	s.mu.Unlock()
	if cli == nil {
		return Status{}, errors.New("还没有起后端")
	}
	opts, err := cli.SetConfigOption(ctx, sid, "model", value)
	if err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	s.session.ConfigOptions = opts
	st := s.statusLocked()
	s.mu.Unlock()
	return st, nil
}

// Stop 关掉后端（下次 Start 会重新起）。
func (s *Service) Stop() error {
	s.mu.Lock()
	cli := s.cli
	s.cli, s.session = nil, acp.Session{}
	s.mu.Unlock()
	if cli == nil {
		return nil
	}
	return cli.Stop()
}

// Shutdown 是给「窗口关了」用的：给一点时间收尾，但不无限等。
func (s *Service) Shutdown(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		_ = s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
