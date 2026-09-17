// Agent 接口：把聊天能力暴露给界面，并把流式更新推过去。
//
// 形状与范围见 docs/specs/agent.spec.md §7 与 docs/specs/settings.spec.md。
// 这一层只做三件事：参数转换、事件转发、把「人在界面上点的东西」接回能力层。
// 协议在 infrastructure/acp，编排在 application/agentapp，这里不写业务规则。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ngnl5/ssot/internal/application/agentapp"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/infrastructure/acp"
	"github.com/ngnl5/ssot/internal/infrastructure/appconfig"
)

// 事件名（前端按名字订阅）。
const (
	// EventAgentUpdate 是一条流式更新（消息块 / thought / 工具调用 / 用量 / 配置变化）。
	EventAgentUpdate = "agent:update"
	// EventAgentTurn 是一轮结束（带 stopReason 或错误）。
	EventAgentTurn = "agent:turn"
	// EventAgentPermission 是权限提示：界面要弹窗，然后用 AnswerPermission 回。
	EventAgentPermission = "agent:permission"
)

// permissionTimeout 是等人在界面上点「允许 / 拒绝」的上限。
//
// 有上限是因为后端在等这个回答：无人应答就超时按拒绝处理，
// 免得一个没人看的弹窗把整轮对话挂死。
const permissionTimeout = 5 * time.Minute

// AgentService 暴露聊天。
type AgentService struct {
	session *compose.Session
	store   *appconfig.Store

	mu     sync.Mutex
	svc    *agentapp.Service
	svcKey string // 这个 Service 是按哪份配置 + 哪个 vault 建的（配置或项目变了就重建）
	notes  []string

	logMu   sync.Mutex
	permMu  sync.Mutex
	permSeq int
	pending map[string]chan string
}

// NewAgentService 构造服务。
func NewAgentService(s *compose.Session) (*AgentService, error) {
	store, err := appconfig.Open()
	if err != nil {
		return nil, err
	}
	return &AgentService{session: s, store: store, pending: map[string]chan string{}}, nil
}

// ── 面向界面的模型 ────────────────────────────────────────────────────────

// AgentModelOption 是模型下拉里的一个选项。
type AgentModelOption struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// AgentStatus 是聊天面板的状态。
type AgentStatus struct {
	Running bool   `json:"running"`
	Busy    bool   `json:"busy"`
	Vault   string `json:"vault"`
	Actor   string `json:"actor"`
	Agent   string `json:"agent"`
	Version string `json:"version"`
	// SessionID 是当前会话（空 = 还没开会话）。
	SessionID string `json:"sessionId"`
	// Model 是当前模型；Models 是可选清单（来自会话的 configOptions）。
	Model  string             `json:"model"`
	Models []AgentModelOption `json:"models"`
	// Ready 表示可以发话（起了后端 + 有会话）。
	Ready bool `json:"ready"`
	// Error 是上一次失败的说明（例如后端起不来），空表示没有。
	Error string `json:"error"`
}

// AgentUpdate 是一条流式更新（面向界面）。
type AgentUpdate struct {
	SessionID string `json:"sessionId"`
	// Kind 是 agent_message_chunk / agent_thought_chunk / tool_call / tool_call_update / usage_update / …
	Kind string `json:"kind"`
	// Text 是消息块/思考块里的文本（其余类型为空）。
	Text string `json:"text"`
	// Tool 是工具调用信息（tool_call / tool_call_update 时才有）。
	Tool *AgentToolCall `json:"tool,omitempty"`
	// Raw 是原始更新（界面要显示没解析过的东西时用）。
	Raw string `json:"raw"`
}

// AgentToolCall 是一次工具调用（轨迹用）。
type AgentToolCall struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Kind   string `json:"kind"`
}

// AgentPermissionPrompt 是问人的权限提示。
type AgentPermissionPrompt struct {
	// ID 用它调 AnswerPermission。
	ID        string              `json:"id"`
	SessionID string              `json:"sessionId"`
	Tool      string              `json:"tool"`
	Options   []AgentPermissionOp `json:"options"`
}

// AgentPermissionOp 是权限提示里的一个按钮。
type AgentPermissionOp struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// AgentTurnDone 是一轮结束。
type AgentTurnDone struct {
	StopReason string `json:"stopReason"`
	Error      string `json:"error"`
}

// AgentSessionRef 是历史会话列表里的一条。
type AgentSessionRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Cwd   string `json:"cwd"`
	// Current 标记它是不是当前会话。
	Current bool `json:"current"`
}

// AgentCheckItem 是后端检查里的一条。
type AgentCheckItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint"`
}

// AppSettingsView 是设置页要的东西：值 + 路径 + 逐条检查 + 说明。
type AppSettingsView struct {
	ProjectsRoot string `json:"projectsRoot"`
	// ProjectRoot 是当前实际用的项目根（来自会话，不是设置里的值）。
	ProjectRoot string `json:"projectRoot"`
	DSHInstall  string `json:"dshInstall"`
	// DSHHome 是 harness 的配置根（profile / 会话 / 凭据都在这儿）。显式钉住，见 appconfig 里的注释。
	DSHHome string `json:"dshHome"`
	Profile string `json:"profile"`
	CLIBin  string `json:"cliBin"`
	Actor   string `json:"actor"`
	Theme   string `json:"theme"`
	// SettingsPath 是设置文件在哪（要能告诉人去哪改）。
	SettingsPath string `json:"settingsPath"`
	// HarnessLog / MCPLog 是两份诊断日志：后端 stderr 与 MCP 服务端的留痕。
	// 出问题时（比如「agent 手上有没有工具」）只能靠它们。
	HarnessLog string `json:"harnessLog"`
	MCPLog     string `json:"mcpLog"`
	ProfileDir string `json:"profileDir"`
	// Checks 是逐条后端检查；AllOK 是它们的汇总。
	Checks []AgentCheckItem `json:"checks"`
	AllOK  bool             `json:"allOk"`
	// Notes 是「用的是默认值」这类说明（不静默，spec §3）。
	Notes []string `json:"notes"`
}

// ── 设置 ──────────────────────────────────────────────────────────────────

// view 把设置与检查结果拼成界面要的形状。
func (s *AgentService) view(cfg appconfig.Settings, notes []string) AppSettingsView {
	checks := cfg.Check()
	items := make([]AgentCheckItem, 0, len(checks))
	for _, c := range checks {
		items = append(items, AgentCheckItem{Name: c.Name, Path: c.Path, OK: c.OK, Hint: c.Hint})
	}
	return AppSettingsView{
		ProjectsRoot: cfg.ProjectsRoot,
		ProjectRoot:  s.session.Root(),
		DSHInstall:   cfg.Agent.DSHInstall,
		DSHHome:      cfg.Agent.DSHHome,
		Profile:      cfg.Agent.Profile,
		CLIBin:       cfg.Agent.CLIBin,
		Actor:        cfg.Agent.Actor,
		Theme:        cfg.Appearance.Theme,
		SettingsPath: s.store.Path(),
		ProfileDir:   cfg.Agent.ProfileDir(),
		HarnessLog:   s.HarnessLogPath(),
		MCPLog:       filepath.Join(filepath.Dir(s.store.Path()), "mcp.log"),
		Checks:       items,
		AllOK:        cfg.AllOK(),
		Notes:        notes,
	}
}

// Settings 读设置（含逐条后端检查）。
func (s *AgentService) Settings() (AppSettingsView, error) {
	cfg, notes, err := s.store.Load()
	if err != nil {
		// 读不动就**明确报错**，但界面仍要能看到路径与检查结果，所以把错误放 notes 里一起回。
		def := appconfig.Defaults()
		v := s.view(def, []string{err.Error()})
		return v, nil
	}
	return s.view(cfg, notes), nil
}

// SaveSettings 存设置。存完立刻重新检查一次（让人当场看到「补齐了没有」）。
func (s *AgentService) SaveSettings(v AppSettingsView) (AppSettingsView, error) {
	cfg := appconfig.Defaults()
	cfg.ProjectsRoot = v.ProjectsRoot
	cfg.Agent.DSHInstall = v.DSHInstall
	cfg.Agent.DSHHome = v.DSHHome
	cfg.Agent.Profile = v.Profile
	cfg.Agent.CLIBin = v.CLIBin
	cfg.Agent.Actor = v.Actor
	cfg.Appearance.Theme = v.Theme
	if err := s.store.Save(cfg); err != nil {
		return AppSettingsView{}, err
	}
	out := s.view(cfg, nil)
	return out, nil
}

// ── 聊天 ──────────────────────────────────────────────────────────────────

// service 按需装配 agentapp.Service。
//
// 两条都必须重建，不能只看 vault：
//   - **换项目**：会话的工作区是 vault，沿用旧会话会把另一个 vault 当成工作区；
//   - **改配置**：后端命令/参数、CLI 路径、actor 变了，旧 Service 里存的是旧值——
//     按 vault 缓存会导致「配置页改了没用」（踩过：救回一个写坏的 DSH 路径之后，
//     界面仍拿旧值去起进程，报的还是旧的错）。
//
// 所以用指纹：vault + 后端命令 + 参数 + CLI + actor，任一变化就重建。
func (s *AgentService) service() (*agentapp.Service, error) {
	proj, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	// ⚠️ ACP 要求 `cwd` 是**绝对路径**（后端会校验：cwd must be an absolute path）。
	// 项目根平时是相对路径（projects/demo），而相对路径又依赖 App 的当前工作目录——
	// 拿它当会话工作区既会被后端直接拒绝，也不稳。所以这里先转绝对路径。
	vault, err := filepath.Abs(proj.Dir)
	if err != nil {
		return nil, fmt.Errorf("算不出 vault 的绝对路径（%s）：%w", proj.Dir, err)
	}
	cfg, notes, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	key := strings.Join([]string{
		vault, cfg.Agent.DSHExe(), strings.Join(cfg.Agent.Args(), "\x00"),
		strings.Join(cfg.Agent.Env(), "\x00"), cfg.Agent.CLIBin, cfg.Agent.Actor,
	}, "\x01")

	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = notes
	if s.svc != nil && s.svcKey == key {
		return s.svc, nil
	}
	if s.svc != nil {
		s.svc.Shutdown(2 * time.Second) // 换 vault 或改了配置：把旧后端的会话收掉
	}
	s.svc = agentapp.New(agentapp.Config{
		Vault:          vault,
		BackendCommand: cfg.Agent.DSHExe(),
		BackendArgs:    cfg.Agent.Args(),
		BackendEnv:     cfg.Agent.Env(),
		Actor:          cfg.Agent.Actor,
		MCPServer: acp.MCPServer{
			Name: "ssot", Command: cfg.Agent.CLIBin,
			Args: []string{"mcp", "--root", vault}, Type: "stdio",
		},
		OnUpdate:      s.emitUpdate,
		AskPermission: s.askPermission,
		// ⚠️ 后端日志**不能丢**：MCP 有没有挂上、注册了哪些工具，只有它说得清。
		// 第一版我写成 `func(string){}` 把日志扔了，结果「agent 手上到底有几个工具」
		// 这个问题无从回答（只能靠猜）。现在落到文件里，出问题有据可查。
		OnLog: s.logHarness,
	})
	s.svcKey = key
	return s.svc, nil
}

// Status 返回聊天面板状态。
func (s *AgentService) Status() (AgentStatus, error) {
	svc, err := s.service()
	if err != nil {
		return AgentStatus{}, err
	}
	st := svc.Status()
	return s.toStatus(st), nil
}

func (s *AgentService) toStatus(st agentapp.Status) AgentStatus {
	out := AgentStatus{
		Running: st.Running, Busy: st.Busy, Vault: st.Vault, Actor: st.Actor,
		Agent: st.Agent.Name, Version: st.Agent.Version,
		SessionID: st.SessionID, Model: st.Model, Ready: st.Running && st.SessionID != "",
	}
	for _, o := range st.ConfigOptions {
		if o.ID != "model" {
			continue
		}
		for _, g := range o.Options {
			for _, item := range g.Options {
				out.Models = append(out.Models, AgentModelOption{
					Value:       toValueString(item.Value),
					Name:        item.Name,
					Description: item.Description,
					Group:       g.Name,
				})
			}
		}
	}
	return out
}

// toValueString 把 configOptions 里的值统一成字符串给界面（它给的是 JSON 元组或字符串）。
func toValueString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		out := ""
		for i, p := range t {
			if i > 0 {
				out += ","
			}
			out += fmt.Sprint(p)
		}
		return "[" + out + "]"
	default:
		return fmt.Sprint(t)
	}
}

// Start 起后端并开会话。
func (s *AgentService) Start() (AgentStatus, error) {
	svc, err := s.service()
	if err != nil {
		return AgentStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	st, err := svc.Start(ctx)
	if err != nil {
		return AgentStatus{}, err
	}
	return s.toStatus(st), nil
}

// Send 发一句。
//
// **立刻返回**：一轮可能跑几分钟，界面靠事件拿更新；一轮结束会发 EventAgentTurn。
func (s *AgentService) Send(text string) error {
	if text == "" {
		return fmt.Errorf("先写点什么再发")
	}
	svc, err := s.service()
	if err != nil {
		return err
	}
	if !svc.Status().Running {
		return fmt.Errorf("还没有起后端——先点「开始」")
	}
	go func() {
		// 不设超时：一轮里可能有多次模型与工具调用，掐超时会把正常的长回答判成失败。
		stop, err := svc.Send(context.Background(), text)
		done := AgentTurnDone{StopReason: stop}
		if err != nil {
			done.Error = err.Error()
		}
		application.Get().Event.Emit(EventAgentTurn, done)
	}()
	return nil
}

// Cancel 停当前这一轮。
func (s *AgentService) Cancel() error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return svc.Cancel(ctx)
}

// Sessions 列当前 vault 的历史会话（已按 cwd 过滤，不会串到别的项目）。
func (s *AgentService) Sessions() ([]AgentSessionRef, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	if !svc.Status().Running {
		return nil, nil // 没起后端就没法列（列表在后端那边）——不当错误，界面显示空即可
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := svc.Sessions(ctx)
	if err != nil {
		return nil, err
	}
	cur := svc.Status().SessionID
	out := make([]AgentSessionRef, 0, len(list))
	for _, it := range list {
		out = append(out, AgentSessionRef{ID: it.ID, Title: it.Title, Cwd: it.Cwd, Current: it.ID == cur})
	}
	return out, nil
}

// Resume 恢复历史会话。
func (s *AgentService) Resume(sessionID string) (AgentStatus, error) {
	svc, err := s.service()
	if err != nil {
		return AgentStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	st, err := svc.Resume(ctx, sessionID)
	if err != nil {
		return AgentStatus{}, err
	}
	return s.toStatus(st), nil
}

// SetModel 改当前会话的模型（会话级，不是全局配置）。
func (s *AgentService) SetModel(value string) (AgentStatus, error) {
	svc, err := s.service()
	if err != nil {
		return AgentStatus{}, err
	}
	var v any = value
	// 后端给的模型值是 JSON 元组（`["provider","model"]`），原样送回去最稳：
	// 拆成字符串再拼容易跟它的期望错开一格。
	if len(value) > 1 && value[0] == '[' {
		var arr []string
		if err := json.Unmarshal([]byte(value), &arr); err == nil {
			v = arr
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := svc.SetModel(ctx, v)
	if err != nil {
		return AgentStatus{}, err
	}
	return s.toStatus(st), nil
}

// Stop 关掉后端。
func (s *AgentService) Stop() error {
	s.mu.Lock()
	svc := s.svc
	s.svc = nil
	s.mu.Unlock()
	if svc == nil {
		return nil
	}
	return svc.Stop()
}

// logHarness 把后端的 stdout/stderr 诊断行落到文件里。
//
// 为什么要留：这些行里会有 MCP 客户端的连接结果与工具注册情况——
// 「agent 手上有没有 ssot 的工具」这个问题只能从这里看出，界面与 ACP 协议都不暴露工具清单。
// 落文件而不是内存：出问题往往是在事后，事后还能翻。
//
// 上限 2MB 之后截断重来（只是诊断，不做轮转）。
func (s *AgentService) logHarness(line string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "ssot", "harness.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > 2<<20 {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05"), line)
}

// HarnessLogPath 是后端日志的位置（配置页要能告诉人去哪看）。
func (s *AgentService) HarnessLogPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ssot", "harness.log")
}

// emitUpdate 把 ACP 更新转成界面好用的形状并推过去。
func (s *AgentService) emitUpdate(u acp.Update) {
	out := AgentUpdate{SessionID: u.SessionID, Kind: u.Kind(), Raw: string(u.Raw)}
	var body struct {
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		Kind       string `json:"kind"`
	}
	_ = json.Unmarshal(u.Raw, &body)
	out.Text = body.Content.Text
	if body.ToolCallID != "" {
		out.Tool = &AgentToolCall{ID: body.ToolCallID, Title: body.Title, Status: body.Status, Kind: body.Kind}
	}
	application.Get().Event.Emit(EventAgentUpdate, out)
}

// askPermission 是「批准必须是人」在聊天里的落地：把问题推给界面，等人在弹窗上点。
func (s *AgentService) askPermission(req acp.Request) string {
	s.permMu.Lock()
	s.permSeq++
	id := fmt.Sprintf("perm-%d", s.permSeq)
	ch := make(chan string, 1)
	s.pending[id] = ch
	s.permMu.Unlock()

	ops := make([]AgentPermissionOp, 0, len(req.Options))
	for _, o := range req.Options {
		ops = append(ops, AgentPermissionOp{OptionID: o.OptionID, Name: o.Name, Kind: o.Kind})
	}
	application.Get().Event.Emit(EventAgentPermission, AgentPermissionPrompt{
		ID: id, SessionID: req.SessionID, Tool: req.ToolCall.Title, Options: ops,
	})

	defer func() {
		s.permMu.Lock()
		delete(s.pending, id)
		s.permMu.Unlock()
	}()
	select {
	case opt := <-ch:
		return opt
	case <-time.After(permissionTimeout):
		// 没人回答就当拒绝：不能让一个没人看的弹窗把这一轮挂死。
		return ""
	}
}

// AnswerPermission 由界面调用：把人在弹窗上的选择回给后端。
func (s *AgentService) AnswerPermission(id, optionID string) error {
	s.permMu.Lock()
	ch, ok := s.pending[id]
	s.permMu.Unlock()
	if !ok {
		return fmt.Errorf("这个权限请求已经不在了（可能已超时）")
	}
	select {
	case ch <- optionID:
		return nil
	default:
		return fmt.Errorf("这个权限请求已经被回答过了")
	}
}
