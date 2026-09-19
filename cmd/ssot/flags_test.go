package main

import (
	"strings"
	"testing"
)

// 选项解析是**手写**的（标准库 flag 遇到第一个位置参数就停，而用法说明里教的
// 顺序恰好是 `status -actor human:我 …`），所以它必须有自己的测试——
// 它是所有 CLI 命令的入口，解析错了每个命令都跟着错。
func TestSplitCommandOptionsAnywhere(t *testing.T) {
	cases := []struct {
		name string
		args []string
		cmd  string
		pos  []string
		chk  func(t *testing.T, f vaultFlags)
	}{
		{
			name: "选项在子命令后面（用法说明里的顺序）",
			args: []string{"status", "-actor", "human:我", "-root", `C:\v`},
			cmd:  "status",
			chk: func(t *testing.T, f vaultFlags) {
				if f.actor != "human:我" || f.root != `C:\v` {
					t.Errorf("actor/root 没解析对：%+v", f)
				}
			},
		},
		{
			name: "选项在子命令前面",
			args: []string{"-root", `C:\v`, "search", "增益"},
			cmd:  "search",
			pos:  []string{"增益"},
			chk:  func(t *testing.T, f vaultFlags) { _ = f },
		},
		{
			name: "等号写法",
			args: []string{"-root=C:\\v", "search", "增益", "-limit=25"},
			cmd:  "search",
			pos:  []string{"增益"},
			chk: func(t *testing.T, f vaultFlags) {
				if f.root != `C:\v` || f.limit != 25 {
					t.Errorf("等号写法没解析对：%+v", f)
				}
			},
		},
		{
			name: "多个位置参数按顺序留下",
			args: []string{"write", "路径 带空格.md", "正文", "第二段"},
			cmd:  "write",
			pos:  []string{"路径 带空格.md", "正文", "第二段"},
			chk:  func(t *testing.T, f vaultFlags) { _ = f },
		},
		{
			name: "布尔开关",
			args: []string{"rm", "-dry", "-store", "docs/a.md"},
			cmd:  "rm",
			pos:  []string{"docs/a.md"},
			chk: func(t *testing.T, f vaultFlags) {
				if !f.dry || !f.store {
					t.Errorf("-dry/-store 该为真：%+v", f)
				}
			},
		},
		{
			name: "-n 是 -dry 的别名",
			args: []string{"rm", "-n", "x"},
			cmd:  "rm",
			pos:  []string{"x"},
			chk: func(t *testing.T, f vaultFlags) {
				if !f.dry {
					t.Error("-n 该等价于 -dry")
				}
			},
		},
		{
			name: "extract 的几个数字参数（0 也要能收）",
			args: []string{"extract", "-docs", "0", "-batch", "8", "-gleaning", "0", "-out", "d.json"},
			cmd:  "extract",
			chk: func(t *testing.T, f vaultFlags) {
				if f.docs != 0 || f.batch != 8 || f.gleaning != 0 || f.out != "d.json" {
					t.Errorf("数字/输出参数没解析对：%+v", f)
				}
			},
		},
		{
			name: "-beta 记下「用户显式给了」",
			args: []string{"find", "词", "-beta", "0.05"},
			cmd:  "find",
			pos:  []string{"词"},
			chk: func(t *testing.T, f vaultFlags) {
				if !f.betaSet || f.beta != 0.05 {
					t.Errorf("beta 该是 0.05 且标为已给：%+v", f)
				}
			},
		},
		{
			name: "beta=0 也算显式给（不能被当成「没给」）",
			args: []string{"find", "词", "-beta=0"},
			cmd:  "find",
			pos:  []string{"词"},
			chk: func(t *testing.T, f vaultFlags) {
				if !f.betaSet || f.beta != 0 {
					t.Errorf("beta=0 该标记为已给：%+v", f)
				}
			},
		},
		{
			name: "没有参数：什么都不设",
			args: nil,
			cmd:  "",
			chk:  func(t *testing.T, f vaultFlags) { _ = f },
		},
	}
	for _, c := range cases {
		cmd, pos, f, err := splitCommand(c.args)
		if err != nil {
			t.Errorf("%s：不该报错：%v", c.name, err)
			continue
		}
		if cmd != c.cmd {
			t.Errorf("%s：子命令该是 %q，拿到 %q", c.name, c.cmd, cmd)
		}
		if len(pos) != len(c.pos) {
			t.Errorf("%s：位置参数该是 %v，拿到 %v", c.name, c.pos, pos)
			continue
		}
		for i := range pos {
			if pos[i] != c.pos[i] {
				t.Errorf("%s：位置参数不同：%v vs %v", c.name, pos, c.pos)
				break
			}
		}
		if c.chk != nil {
			c.chk(t, f)
		}
	}
}

// 参数不合法时报的错要说清是哪个参数、期望什么——CLI 的错误信息就是文档。
func TestSplitCommandRejectsBadValues(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"limit 是 0", []string{"search", "词", "-limit", "0"}, "-limit 要一个正整数"},
		{"limit 是负数", []string{"search", "词", "-limit", "-3"}, "-limit 要一个正整数"},
		{"limit 不是数字", []string{"search", "词", "-limit", "abc"}, "-limit 要一个正整数"},
		{"limit= 空", []string{"search", "词", "-limit="}, "-limit 要一个正整数"},
		{"limit 后面没有值", []string{"search", "词", "-limit"}, "-limit 后面要跟一个值"},
		{"root 后面没有值", []string{"-root"}, "-root 后面要跟一个值"},
		{"docs 是负数", []string{"extract", "-docs", "-1"}, "-docs 要一个非负整数"},
		{"batch 是 0", []string{"extract", "-batch", "0"}, "-batch 要一个正整数"},
		{"gleaning 是负数", []string{"extract", "-gleaning", "-2"}, "-gleaning 要一个非负整数"},
		{"beta 不是数字", []string{"find", "词", "-beta", "x"}, "beta"},
		{"不认识的选项", []string{"status", "-nope"}, "不认识的选项"},
	}
	for _, c := range cases {
		_, _, _, err := splitCommand(c.args)
		if err == nil {
			t.Errorf("%s：该报错", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("%s：错误信息该包含 %q，实际是 %q", c.name, c.wantSub, err.Error())
		}
	}
}

// 解析不出错时，同一个选项重复给：后面的说了算（人改主意时不必删前面的）。
func TestSplitCommandLastValueWins(t *testing.T) {
	_, _, f, err := splitCommand([]string{"search", "-limit", "5", "词", "-limit", "50"})
	if err != nil {
		t.Fatal(err)
	}
	if f.limit != 50 {
		t.Errorf("重复给 -limit 该以后面的为准，拿到 %d", f.limit)
	}
}
