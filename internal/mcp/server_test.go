package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

// newVault 造一个最小 vault（与 vaultapp 的测试同一套素材，便于对照）。
func newVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs/式神/茨木童子.md":    "---\ntitle: 茨木童子\ntags: [式神, SSR]\nstatus: published\n---\n\n整理后的正文。\n",
		"docs/机制/伤害计算.md":    "---\ntitle: 伤害计算\nstatus: draft\n---\n\n公式见下。\n依据 [[raw/灰机wiki/茨木童子#^第3段]]。\n",
		"raw/灰机wiki/茨木童子.md": "---\ntitle: 灰机wiki：茨木童子\nsource_url: https://example.com/x\n---\n\n第一段。\n伤害系数 1.5。 ^第3段\n",
		"tables/技能倍率.csv":    "技能,倍率\na,1.5\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// session 是一个跟服务端对话的客户端：喂一行，读一行。
type session struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Reader
}

// start 起一个内存里的服务端（不经过进程，测试跑得快）。
func start(t *testing.T, root string) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	actor, err := vault.ParseActor("agent:整理")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = New(vaultapp.New(root), actor).Serve(inR, outW)
		_ = outW.Close()
	}()
	return &session{t: t, in: inW, out: bufio.NewReader(outR)}
}

// call 发一条请求并读回响应；id 固定成 1。
func (s *session) call(method string, params any) map[string]any {
	s.t.Helper()
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, err := json.Marshal(req)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.in.Write(append(b, '\n')); err != nil {
		s.t.Fatal(err)
	}
	line, err := s.out.ReadString('\n')
	if err != nil {
		s.t.Fatalf("读响应失败（%s 没有回包？）：%v", method, err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		s.t.Fatalf("响应不是 JSON：%q", line)
	}
	return resp
}

// notify 发一条通知（不该有回包）。
func (s *session) notify(method string) {
	s.t.Helper()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	if _, err := s.in.Write(append(b, '\n')); err != nil {
		s.t.Fatal(err)
	}
}

// callTool 调一个工具，返回解出来的结果体（content[0].text 已按 JSON 解析）。
func (s *session) callTool(name string, args map[string]any) (map[string]any, bool) {
	s.t.Helper()
	resp := s.call("tools/call", map[string]any{"name": name, "arguments": args})
	if errObj, ok := resp["error"].(map[string]any); ok {
		s.t.Fatalf("%s 返回协议错误：%v", name, errObj)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		s.t.Fatalf("%s 没有 result：%v", name, resp)
	}
	if result["isError"] == true {
		return result, true
	}
	content := result["content"].([]any)[0].(map[string]any)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content["text"].(string)), &parsed); err != nil {
		s.t.Fatalf("%s 的 text 不是 JSON：%q", name, content["text"])
	}
	return parsed, false
}

func TestInitializeNegotiatesProtocolVersion(t *testing.T) {
	s := start(t, newVault(t))
	// 客户端要一个我们支持的版本 → 照它来（协商的全部含义）。
	resp := s.call("initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}})
	result := resp["result"].(map[string]any)
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("该回客户端要的版本：%v", result["protocolVersion"])
	}
	info := result["serverInfo"].(map[string]any)
	if info["name"] != "ssot" {
		t.Errorf("serverInfo.name 该是 ssot：%v", info)
	}
	// 客户端要一个我们不认识的版本 → 回我们自己最新的那一版。
	resp = s.call("initialize", map[string]any{"protocolVersion": "1999-01-01"})
	if got := resp["result"].(map[string]any)["protocolVersion"]; got != ProtocolVersion {
		t.Errorf("不认识的版本该回 %s，实际 %v", ProtocolVersion, got)
	}
}

func TestToolsListIsSnakeCaseAndHidesStatusSet(t *testing.T) {
	s := start(t, newVault(t))
	resp := s.call("tools/list", nil)
	list := resp["result"].(map[string]any)["tools"].([]any)
	if len(list) != 9 {
		t.Fatalf("工具该有 9 个（含只读的 file_read），实际 %d", len(list))
	}
	names := map[string]bool{}
	for _, raw := range list {
		tool := raw.(map[string]any)
		name := tool["name"].(string)
		names[name] = true
		// DSH 只接受 [A-Za-z0-9_-]：有别的字符就会被换成 _ 加 hash，模型看到的名字会变。
		for _, r := range name {
			if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
				t.Errorf("工具名 %q 里有 DSH 不接受的字符 %q", name, r)
			}
		}
		if tool["description"] == "" || tool["inputSchema"] == nil {
			t.Errorf("%s 缺 description 或 inputSchema", name)
		}
	}
	for _, want := range []string{"vault_list", "doc_read", "doc_write", "vault_search", "link_backlinks", "link_resolve", "file_read", "table_infos", "table_query"} {
		if !names[want] {
			t.Errorf("少了工具 %s", want)
		}
	}
	if names["status_set"] {
		t.Error("status_set 不该暴露：MCP 侧 actor 恒为 agent，它永远失败，只会白占上下文")
	}
}

func TestReadWriteAndGate(t *testing.T) {
	root := newVault(t)
	s := start(t, root)

	// 只写文件名、而库里有两篇同名（docs/ 与 raw/ 各一篇）→ **不猜**，如实报候选。
	ambiguous, isErr := s.callTool("doc_read", map[string]any{"path": "茨木童子"})
	if !isErr {
		t.Error("同名两篇时该是 isError 结果（系统不裁决）")
	}
	if text := ambiguous["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, "同名") {
		t.Errorf("歧义信息该说清是同名：%q", text)
	}

	// 读：正文与 front matter 字段都在。
	doc, isErr := s.callTool("doc_read", map[string]any{"path": "docs/式神/茨木童子.md"})
	if isErr {
		t.Fatalf("doc_read 不该失败：%v", doc)
	}
	if doc["status"] != "published" || !strings.Contains(doc["body"].(string), "整理后的正文") {
		t.Errorf("doc_read 该带状态与正文：%v", doc)
	}

	// 写：agent 写已发布的文档 → 回落 draft，并且如实报告没留痕（这个 vault 不是 git 仓库）。
	ch, isErr := s.callTool("doc_write", map[string]any{"path": "docs/式神/茨木童子.md", "body": "新正文。\n"})
	if isErr {
		t.Fatal("doc_write 不该失败")
	}
	if ch["from"] != "published" || ch["to"] != "draft" {
		t.Errorf("agent 写已发布文档该回落 draft：%v", ch)
	}
	if ch["committed"] != false || ch["versionNote"] == nil {
		t.Errorf("没留痕必须说出来：%v", ch)
	}
	if !strings.Contains(ch["versionNote"].(string), "git init") {
		t.Errorf("该告诉人怎么补：%v", ch["versionNote"])
	}
	if b, err := os.ReadFile(filepath.Join(root, "docs", "式神", "茨木童子.md")); err != nil || !strings.Contains(string(b), "新正文") {
		t.Errorf("文件该真被改了：%v %s", err, b)
	}

	// 反链 + 问题链接（断链要如实报出来）。
	back, isErr := s.callTool("link_backlinks", map[string]any{"path": "raw/灰机wiki/茨木童子.md"})
	if isErr {
		t.Fatal("link_backlinks 不该失败")
	}
	if len(back["backlinks"].([]any)) != 1 {
		t.Errorf("该有一条反链：%v", back)
	}
}

func TestResolveQueryAndErrors(t *testing.T) {
	s := start(t, newVault(t))

	// 块锚点解析到文件行号（主张级溯源）。
	res, isErr := s.callTool("link_resolve", map[string]any{"ref": "[[raw/灰机wiki/茨木童子#^第3段]]"})
	if isErr {
		t.Fatal("link_resolve 不该失败")
	}
	if res["path"] != "raw/灰机wiki/茨木童子.md" || res["blockLine"].(float64) <= 0 {
		t.Errorf("该解析到文档与文件行号：%v", res)
	}

	// 数据表：先看有什么表，再查。
	infos, isErr := s.callTool("table_infos", map[string]any{})
	if isErr {
		t.Fatal("table_infos 不该失败")
	}
	if len(infos["tables"].([]any)) == 0 {
		t.Fatal("该看到 tables/技能倍率.csv")
	}
	q, isErr := s.callTool("table_query", map[string]any{"sql": "SELECT 技能, 倍率 FROM 技能倍率"})
	if isErr {
		t.Fatal("table_query 不该失败")
	}
	if len(q["rows"].([]any)) != 1 {
		t.Errorf("该查到一行：%v", q)
	}

	// 工具跑起来失败 → isError 结果（模型能读到原因），不是协议错误。
	if _, isErr := s.callTool("doc_read", map[string]any{"path": "不存在"}); !isErr {
		t.Error("读不存在的文档该是 isError 结果")
	}
	// 参数缺失 → isError 结果，且说清缺哪个。
	res2, isErr := s.callTool("doc_read", map[string]any{})
	if !isErr {
		t.Error("缺 path 该是 isError 结果")
	}
	if text := res2["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, "path") {
		t.Errorf("错误信息该说清缺哪个参数：%q", text)
	}
	// 工具名不认识 → 协议错误（-32602）。
	resp := s.call("tools/call", map[string]any{"name": "vault_nope", "arguments": map[string]any{}})
	if errObj, ok := resp["error"].(map[string]any); !ok || errObj["code"].(float64) != -32602 {
		t.Errorf("不认识的工具该回 -32602：%v", resp)
	}
	// 不认识的方法 → -32601。
	resp = s.call("resources/list", nil)
	if errObj, ok := resp["error"].(map[string]any); !ok || errObj["code"].(float64) != -32601 {
		t.Errorf("不认识的方法该回 -32601：%v", resp)
	}
}

// TestNotificationsGetNoReplyAndStreamStaysClean 盯两件容易翻车的事：
// 通知不回包、坏消息之后流还能继续用（stdout 里不许混进任何非协议内容）。
func TestNotificationsGetNoReplyAndStreamStaysClean(t *testing.T) {
	s := start(t, newVault(t))
	s.notify("notifications/initialized")
	// 坏 JSON 一行 → 必须回一条 parse error，而且**不能**把后面的解析带歪。
	if _, err := s.in.Write([]byte("{这不是 JSON}\n")); err != nil {
		t.Fatal(err)
	}
	line, err := s.out.ReadString('\n')
	if err != nil {
		t.Fatalf("坏消息该回一条 parse error：%v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("parse error 的响应本身要是合法 JSON：%q", line)
	}
	if errObj, ok := resp["error"].(map[string]any); !ok || errObj["code"].(float64) != -32700 {
		t.Fatalf("该回 -32700：%q", line)
	}
	// 之后一切照常。
	s.call("ping", nil)
	if _, isErr := s.callTool("vault_list", map[string]any{}); isErr {
		t.Error("坏消息之后服务端该继续可用")
	}
}

// TestServeViaRealBinary 走真进程真 stdio：内存测试证明不了「CLI 接线对不对」，
// 而 DSH 起的就是这个进程。
func TestServeViaRealBinary(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("没有 go，跳过（这条要编译真二进制）")
	}
	root := newVault(t)
	bin := filepath.Join(t.TempDir(), "ssot.exe")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/ssot")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编译失败：%v\n%s", err, out)
	}

	cmd := exec.Command(bin, "mcp", "-root", root, "-actor", "agent:dsh")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	send := func(v any) {
		t.Helper()
		b, _ := json.Marshal(v)
		if _, err := stdin.Write(append(b, '\n')); err != nil {
			t.Fatalf("写 stdin 失败：%v（stderr=%s）", err, stderr.String())
		}
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "vault_list", "arguments": map[string]any{}}})
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("进程退出异常：%v\nstderr=%s", err, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("该回 3 条响应，实际 %d 条：\n%s", len(lines), stdout.String())
	}
	// stdout 里**只许**有协议消息：每一行都必须是能解析的 JSON-RPC 响应。
	for _, line := range lines {
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("stdout 里混进了非协议内容：%q", line)
		}
		if resp["jsonrpc"] != "2.0" {
			t.Fatalf("不是 JSON-RPC 2.0 响应：%q", line)
		}
	}
	if !strings.Contains(lines[2], "documents") {
		t.Errorf("vault_list 该给出文档列表：%q", lines[2])
	}
}

// TestRejectsHumanActor：MCP 这条路上没有 human——发布只能走界面或 CLI
// （docs/specs/dsh.spec.md §4）。这是编译真二进制跑一遍 CLI 的拒绝路径。
func TestRejectsHumanActor(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("没有 go，跳过")
	}
	root := newVault(t)
	bin := filepath.Join(t.TempDir(), "ssot.exe")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/ssot").CombinedOutput(); err != nil {
		t.Fatalf("编译失败：%v\n%s", err, out)
	}
	out, err := exec.Command(bin, "mcp", "-root", root, "-actor", "human:我").CombinedOutput()
	if err == nil {
		t.Fatalf("human actor 该被拒绝，输出：%s", out)
	}
	if !strings.Contains(string(out), "只能是 agent") {
		t.Errorf("拒绝信息该说清原因：%s", out)
	}
}
