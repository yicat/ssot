package api

import (
	"encoding/json"
	"testing"

	"github.com/ngnl5/ssot/internal/infrastructure/acp"
)

// 这两条映射是**界面契约**：消息气泡、思考折叠、工具行、权限弹窗全靠它们。
// 映射错了界面就悄悄少东西（不报错），所以拿真实形状的 ACP 更新逐个钉住。
func TestToAgentUpdate(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantKind string
		wantText string
		wantTool bool
		chk      func(t *testing.T, out AgentUpdate)
	}{
		{
			name:     "消息块",
			raw:      `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"给你答案。"}}`,
			wantKind: "agent_message_chunk",
			wantText: "给你答案。",
			chk: func(t *testing.T, out AgentUpdate) {
				if out.Tool != nil {
					t.Error("消息块不该带工具信息")
				}
			},
		},
		{
			name:     "思考块",
			raw:      `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"先看看。"}}`,
			wantKind: "agent_thought_chunk",
			wantText: "先看看。",
		},
		{
			name:     "工具调用开始",
			raw:      `{"sessionUpdate":"tool_call","toolCallId":"c1","title":"mcp__ssot__vault_search","status":"in_progress","kind":"search"}`,
			wantKind: "tool_call",
			wantTool: true,
			chk: func(t *testing.T, out AgentUpdate) {
				if out.Tool.ID != "c1" || out.Tool.Title != "mcp__ssot__vault_search" ||
					out.Tool.Status != "in_progress" || out.Tool.Kind != "search" {
					t.Errorf("工具字段没映射对：%+v", *out.Tool)
				}
			},
		},
		{
			name:     "工具调用结束（只有状态与 id）",
			raw:      `{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed"}`,
			wantKind: "tool_call_update",
			wantTool: true,
			chk: func(t *testing.T, out AgentUpdate) {
				if out.Tool.ID != "c1" || out.Tool.Status != "completed" {
					t.Errorf("应该仍带上 id 与状态：%+v", *out.Tool)
				}
			},
		},
		{
			name:     "用量更新（既没文本也没工具）",
			raw:      `{"sessionUpdate":"usage_update","used":1234,"size":100000}`,
			wantKind: "usage_update",
			chk: func(t *testing.T, out AgentUpdate) {
				if out.Tool != nil || out.Text != "" {
					t.Errorf("用量更新不该带文本或工具：%+v", out)
				}
			},
		},
		{
			name:     "看不懂的更新：kind 为空，但 raw 原样留着",
			raw:      `{"whatever":1,"nested":{"a":[1,2]}}`,
			wantKind: "",
			chk: func(t *testing.T, out AgentUpdate) {
				if out.Raw != `{"whatever":1,"nested":{"a":[1,2]}}` {
					t.Errorf("raw 该原样带着（界面要显示没解析过的东西）：%q", out.Raw)
				}
			},
		},
		{
			name:     "不是 JSON：不许 panic，raw 照留",
			raw:      `这不是 json`,
			wantKind: "",
			chk: func(t *testing.T, out AgentUpdate) {
				if out.Raw != `这不是 json` {
					t.Errorf("raw 该原样带着：%q", out.Raw)
				}
			},
		},
	}
	for _, c := range cases {
		u := acp.Update{SessionID: "s-1", Raw: json.RawMessage(c.raw)}
		out := toAgentUpdate(u)
		if out.SessionID != "s-1" {
			t.Errorf("%s：sessionId 该带上", c.name)
		}
		if out.Kind != c.wantKind {
			t.Errorf("%s：kind 该是 %q，拿到 %q", c.name, c.wantKind, out.Kind)
		}
		if out.Text != c.wantText {
			t.Errorf("%s：text 该是 %q，拿到 %q", c.name, c.wantText, out.Text)
		}
		if c.wantTool && out.Tool == nil {
			t.Errorf("%s：该有工具信息", c.name)
		}
		if !c.wantTool && out.Tool != nil {
			t.Errorf("%s：不该有工具信息：%+v", c.name, *out.Tool)
		}
		if c.chk != nil {
			c.chk(t, out)
		}
	}
}

// 权限提示：界面拿 Tool 当标题显示、拿 Options 画按钮——一个都不能少、顺序不能乱。
func TestToPermissionPrompt(t *testing.T) {
	var req acp.Request
	req.SessionID = "s-1"
	req.ToolCall.ToolCallID = "c9"
	req.ToolCall.Title = "doc_write"
	req.Options = []acp.Option{
		{OptionID: "allow-once", Name: "允许一次", Kind: "allow_once"},
		{OptionID: "reject-once", Name: "拒绝", Kind: "reject_once"},
	}

	out := toPermissionPrompt("perm-3", req)
	if out.ID != "perm-3" || out.SessionID != "s-1" || out.Tool != "doc_write" {
		t.Errorf("弹窗的基础字段不对：%+v", out)
	}
	if len(out.Options) != 2 {
		t.Fatalf("该有两个按钮：%+v", out.Options)
	}
	if out.Options[0].OptionID != "allow-once" || out.Options[1].OptionID != "reject-once" {
		t.Errorf("按钮顺序该与后端给的一致：%+v", out.Options)
	}
	if out.Options[0].Name != "允许一次" || out.Options[0].Kind != "allow_once" {
		t.Errorf("按钮的文字与类型要带上（界面按 kind 上色）：%+v", out.Options[0])
	}

	// 没有选项时给空切片，不是 nil——界面少一个 null 判断。
	empty := toPermissionPrompt("perm-4", acp.Request{})
	if empty.Options == nil || len(empty.Options) != 0 {
		t.Errorf("没有选项时该给空切片：%#v", empty.Options)
	}
}
