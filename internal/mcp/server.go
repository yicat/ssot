// Package mcp 是能力层的 MCP 服务端（stdio 传输）。
//
// 它是**接口层**：与 internal/api（wails3 绑定）平级，只做「MCP 协议 ↔ 用例调用」的转换，
// 不写业务规则、不复制权限判断——权限只有 application/vaultapp 一份实现
// （docs/specs/agent.spec.md §4：「MCP 与 CLI 走同一套权限规则」）。
//
// 形状与理由见 docs/specs/dsh.spec.md：
//   - stdio + **换行分隔**的 JSON-RPC 2.0（MCP 的 stdio 传输就是一行的 JSON，
//     不是 LSP 那种 Content-Length 头）
//   - 工具名 snake_case：DSH 只接受 `[A-Za-z0-9_-]`，点号会被换成 `_` 加 hash
//   - `actor` 由本层写死为 `agent:<名字>`——MCP 表达不出 human，发布只能走界面/CLI
//
// ⚠️ **stdout 只许走协议消息**。任何一句 `fmt.Println` 调试输出都会把流弄坏，
// 客户端会报解析错误而看不出原因。所以：日志一律 stderr（cli 层的 runMCP 也照此办）。
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

// Server 是一个 vault 上的 MCP 服务端。
type Server struct {
	svc   *vaultapp.Service
	actor vault.Actor
}

// New 构造 Server。actor 由调用方给（CLI 的 `-actor`），且必须是 agent：
// MCP 这条路上不存在 human，写代码时就把这个前提固定住。
func New(svc *vaultapp.Service, actor vault.Actor) *Server {
	return &Server{svc: svc, actor: actor}
}

// ProtocolVersion 是我们支持的 MCP 协议版本（协商时优先回客户端要的那一版）。
//
// 版本号来自机器上 DSH 实际装的 @modelcontextprotocol/sdk（1.30.0），
// 它认 2024-10-07 起、最新 2025-11-25（见 docs/specs/dsh.spec.md）。
const ProtocolVersion = "2025-06-18"

// supportedProtocols 是能原样回给客户端的版本。
var supportedProtocols = map[string]bool{
	"2024-10-07": true,
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
	"2025-11-25": true,
}

// ServerName / ServerVersion 出现在 initialize 的 serverInfo 里。
const (
	ServerName    = "ssot"
	ServerVersion = "0.1.0"
)

// maxMessage 是单条消息的上限（64MiB）。
//
// 要设得比一篇文档大：scanner 的默认 64KiB 会把长文档的写入直接判成「消息太长」，
// 而那个报错看起来像是协议错误，很容易查错方向。
const maxMessage = 64 << 20

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC 标准错误码。
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve 跑主循环：一行一条消息，直到 in 结束（客户端关掉管道）。
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), maxMessage)
	w := bufio.NewWriter(out)
	defer w.Flush()

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			// 解析失败的 id 无从得知，按协议回 null。
			if err := write(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &rpcError{Code: codeParse, Message: "parse error: " + err.Error()}}); err != nil {
				return err
			}
			continue
		}
		resp, ok := s.handle(req)
		if !ok {
			continue // 通知（notification）不回包
		}
		if err := write(w, resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func write(w *bufio.Writer, resp response) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

// handle 分派一条消息。ok=false 表示这是一条通知，不该回包。
func (s *Server) handle(req request) (response, bool) {
	notification := len(req.ID) == 0 || string(req.ID) == "null"
	reply := func(result any) (response, bool) {
		return response{JSONRPC: "2.0", ID: req.ID, Result: result}, true
	}
	fail := func(code int, msg string) (response, bool) {
		if notification {
			return response{}, false
		}
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg}}, true
	}

	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		}
		_ = json.Unmarshal(req.Params, &p)
		s.logf("initialize 客户端=%s %s 协议=%s", p.ClientInfo.Name, p.ClientInfo.Version, p.ProtocolVersion)
		v := ProtocolVersion
		if supportedProtocols[p.ProtocolVersion] {
			v = p.ProtocolVersion // 客户端要的版本我们认，就照它来
		}
		return reply(map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": ServerName, "version": ServerVersion},
			"instructions":    instructions,
		})
	case "notifications/initialized", "notifications/cancelled":
		return response{}, false
	case "ping":
		return reply(map[string]any{})
	case "tools/list":
		// 留痕：客户端来取工具清单，说明它真的把我们挂上了——
		// 「agent 手上到底有没有这些工具」只能从这里看出来（ACP 不暴露工具清单）。
		s.logf("tools/list 被调用，返回 %d 个工具", len(tools))
		return reply(map[string]any{"tools": toolList()})
	case "tools/call":
		return s.callTool(req, notification)
	default:
		return fail(codeMethodNotFound, fmt.Sprintf("不认识的方法 %q（本服务只提供 tools/list 与 tools/call）", req.Method))
	}
}

// logf 记一行诊断。
//
// 为什么要有：MCP 这条链上没有别的地方能看出「客户端到底连上了没、取没取工具清单」——
// ACP 协议不暴露工具清单，harness 的日志里也没有 MCP 客户端的注册记录。
// 所以服务端自己留痕：写 stderr（能进 harness 日志），同时落到
// `<用户配置目录>/ssot/mcp.log`（事后也能翻）。**stdout 绝不能碰**——那是协议流。
//
// 诊断而已，任何失败都不影响服务本身。
func (s *Server) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[ssot-mcp] %s\n", line)
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "ssot", "mcp.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05"), line)
}

// instructions 是给模型的短说明。
//
// 写短：它每次请求都要占上下文。只写「这里是什么、边界在哪」，
// 具体怎么干活在四个角色的 skill 里（.dsh/skills/）。
const instructions = `这是 SSOT 的 vault 能力：两层文档（raw 原始层 / docs 整理层）+ 数据表。

- 写入（doc_write）一律把文档状态回落为 draft——agent 不能发布。
- 发布 / 归档只能由人在界面或 CLI 做，这里不提供工具。
- 文档里 [[双链]] 的块级锚点（[[目标#^块]]）就是主张级溯源，写整理稿时用它指回 raw/。
- 表相关：先看 table_infos，再用 table_query 跑只读 SQL。`
