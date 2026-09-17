package agentapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/infrastructure/acp"
)

// fakeClient 顶掉真后端：只记「上层让它做了什么」，并按剧本回话。
type fakeClient struct {
	cwd      string
	mcp      []acp.MCPServer
	session  string
	stop     bool
	prompted []string
	resumed  string
	model    any
	stopped  bool
	agent    acp.AgentInfo
}

func (f *fakeClient) Initialize(context.Context) (acp.Caps, error) {
	f.agent = acp.AgentInfo{Name: "fake-acp", Version: "1.0"}
	return acp.Caps{ProtocolVersion: 1, Agent: f.agent, CanList: true, CanResume: true, CanClose: true}, nil
}

func (f *fakeClient) NewSession(_ context.Context, cwd string, mcp []acp.MCPServer) (acp.Session, error) {
	f.cwd, f.mcp, f.session = cwd, mcp, "sess-1"
	return acp.Session{ID: "sess-1", ConfigOptions: []acp.ConfigOption{{
		ID: "model", CurrentValue: []any{"fake-provider", "fake-model"},
	}}}, nil
}

func (f *fakeClient) ResumeSession(_ context.Context, id, cwd string, mcp []acp.MCPServer) (acp.Session, error) {
	f.resumed, f.cwd, f.mcp = id, cwd, mcp
	f.session = id
	return acp.Session{ID: id, ConfigOptions: []acp.ConfigOption{{ID: "model", CurrentValue: "restored"}}}, nil
}

func (f *fakeClient) Prompt(_ context.Context, _, text string) (string, error) {
	f.prompted = append(f.prompted, text)
	return "end_turn", nil
}

func (f *fakeClient) Cancel(context.Context, string) error { f.stop = true; return nil }

func (f *fakeClient) ListSessions(_ context.Context, cwd string) ([]acp.Summary, error) {
	if cwd != f.cwd {
		return nil, errors.New("列会话没带对 cwd")
	}
	return []acp.Summary{{ID: "sess-0", Cwd: cwd, Title: "上次整理"}}, nil
}

func (f *fakeClient) CloseSession(context.Context, string) error { return nil }

func (f *fakeClient) SetConfigOption(_ context.Context, _, _ string, v any) ([]acp.ConfigOption, error) {
	f.model = v
	return []acp.ConfigOption{{ID: "model", CurrentValue: v}}, nil
}

func (f *fakeClient) Stop() error { f.stopped = true; return nil }

func (f *fakeClient) Agent() acp.AgentInfo { return f.agent }

func newTestService(t *testing.T, cfg Config) (*Service, *fakeClient) {
	t.Helper()
	fake := &fakeClient{}
	if cfg.Vault == "" {
		cfg.Vault = `C:\vault`
	}
	cfg.BackendCommand = "fake.exe"
	cfg.MCPServer = acp.MCPServer{Name: "ssot", Command: `C:\bin\ssot-cli.exe`, Args: []string{"mcp", "--root", cfg.Vault}, Type: "stdio"}
	cfg.newClient = func(acp.Config) (client, error) { return fake, nil }
	return New(cfg), fake
}

func TestStartMountsMCPPerSession(t *testing.T) {
	s, fake := newTestService(t, Config{Actor: "agent:dsh"})
	st, err := s.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.SessionID != "sess-1" {
		t.Errorf("状态不对：%+v", st)
	}
	// 最关键的一条：MCP 是**随会话挂**的，而且 command 用绝对路径。
	if len(fake.mcp) != 1 || fake.mcp[0].Command != `C:\bin\ssot-cli.exe` {
		t.Fatalf("MCP 没按会话挂上：%+v", fake.mcp)
	}
	if fake.cwd != `C:\vault` {
		t.Errorf("工作区该是 vault：%q", fake.cwd)
	}
	// 会话级配置（模型）要能读出来给界面显示。
	if st.Model != "fake-provider / fake-model" {
		t.Errorf("模型该从 configOptions 里取出来：%q", st.Model)
	}
	if st.Agent.Name != "fake-acp" {
		t.Errorf("该报出后端身份：%+v", st.Agent)
	}
}

func TestStartTwiceReusesBackend(t *testing.T) {
	s, fake := newTestService(t, Config{})
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.stopped {
		t.Error("第二次 Start 不该把后端停掉重来")
	}
}

func TestStartRefusesWithoutVaultOrCLI(t *testing.T) {
	// 没 vault：报错要说清是「没打开项目」，不是含糊的失败。
	s, _ := newTestService(t, Config{Vault: ""})
	s.cfg.Vault = ""
	if _, err := s.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "打开的项目") {
		t.Errorf("没 vault 该明确报出来：%v", err)
	}
	// 没 CLI：MCP 起不来，也要说清怎么补。
	s2, _ := newTestService(t, Config{})
	s2.cfg.MCPServer = acp.MCPServer{}
	if _, err := s2.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "build:cli") {
		t.Errorf("没 CLI 该告诉人怎么补：%v", err)
	}
}

func TestSendAndBusyGuard(t *testing.T) {
	s, fake := newTestService(t, Config{})
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stop, err := s.Send(context.Background(), "整理一下茨木童子")
	if err != nil || stop != "end_turn" {
		t.Fatalf("发送失败：%v %q", err, stop)
	}
	if len(fake.prompted) != 1 || fake.prompted[0] != "整理一下茨木童子" {
		t.Errorf("提示词没送到：%v", fake.prompted)
	}
	// 一轮跑完后 busy 要落回去（不然界面会一直显示「正在跑」）。
	if s.Status().Busy {
		t.Error("跑完之后 busy 该是 false")
	}
	// 没起后端就发 → 明确报错。
	s3, _ := newTestService(t, Config{})
	if _, err := s3.Send(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "还没有起后端") {
		t.Errorf("没起后端该明确报：%v", err)
	}
}

func TestSessionsFiltersByVaultAndResumeSwitches(t *testing.T) {
	s, fake := newTestService(t, Config{})
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	list, err := s.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Cwd != `C:\vault` {
		t.Errorf("会话列表不对：%+v", list)
	}
	// 恢复历史会话：MCP 要跟着重新挂，工作区不变。
	st, err := s.Resume(context.Background(), "sess-0")
	if err != nil {
		t.Fatal(err)
	}
	if fake.resumed != "sess-0" || len(fake.mcp) != 1 {
		t.Errorf("恢复会话时该重新挂 MCP：resumed=%q mcp=%+v", fake.resumed, fake.mcp)
	}
	if st.SessionID != "sess-0" || st.Model != "restored" {
		t.Errorf("恢复后状态不对：%+v", st)
	}
	// 恢复成当前会话 = 什么都不做。
	if _, err := s.Resume(context.Background(), "sess-0"); err != nil {
		t.Errorf("恢复当前会话不该报错：%v", err)
	}
}

func TestSetModelIsSessionScoped(t *testing.T) {
	s, fake := newTestService(t, Config{})
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := s.SetModel(context.Background(), `["fake-provider","fake-pro"]`)
	if err != nil {
		t.Fatal(err)
	}
	if fake.model != `["fake-provider","fake-pro"]` || !strings.Contains(st.Model, "fake-pro") {
		t.Errorf("模型没改到：%+v", st)
	}
}

func TestStopAndShutdown(t *testing.T) {
	s, fake := newTestService(t, Config{})
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Shutdown(2 * time.Second)
	if !fake.stopped {
		t.Error("Shutdown 该把后端停掉")
	}
	if s.Status().Running {
		t.Error("停掉之后状态该是没在跑")
	}
	// 停掉之后再 Start 要能重新起来（不残留旧状态）。
	if _, err := s.Start(context.Background()); err != nil {
		t.Errorf("停掉之后该能重新开始：%v", err)
	}
}
