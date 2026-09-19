package mcp

import (
	"encoding/json"
	"testing"
)

// `args.num` 的边界逐个钉住。
//
// 为什么专门测它：#33 那场「limit 不生效」的争议就卡在这一层——模型发来的数字
// 未必是整数（`"50"`、`50.0` 都见过），而这一层只要返回默认值或报错，
// 上层看起来就像「limit 被忽略了」。所以「认什么、不认什么」必须写死。
func TestArgsNum(t *testing.T) {
	cases := []struct {
		name string
		raw  string // 原始 JSON；空串表示**没有这个参数**
		want int
		bad  bool
	}{
		{"没给就吃默认", "", 10, false},
		{"整数", `50`, 50, false},
		{"小数形式的整数", `50.0`, 50, false},
		{"带引号的整数", `"50"`, 50, false},
		{"字符串两端有空格", `" 50 "`, 50, false},
		{"负数也照收（怎么解释由调用方定）", `-1`, -1, false},
		{"带小数的非整数", `1.5`, 0, true},
		{"非数字字符串", `"abc"`, 0, true},
		{"空字符串", `""`, 0, true},
		{"布尔", `true`, 0, true},
		{"数组", `[50]`, 0, true},
		{"对象", `{"n":50}`, 0, true},
	}
	for _, c := range cases {
		a := args{}
		if c.raw != "" {
			a["limit"] = json.RawMessage(c.raw)
		}
		got, err := a.num("limit", 10)
		if c.bad {
			if err == nil {
				t.Errorf("%s：该报错，却拿到 %d", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s：不该报错：%v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s：拿到 %d，想要 %d", c.name, got, c.want)
		}
	}
}

// 字符串参数的边界：缺了要说清缺哪个（模型最容易漏参数）。
func TestArgsStr(t *testing.T) {
	a := args{"query": json.RawMessage(`"增益"`)}
	if got, err := a.str("query"); err != nil || got != "增益" {
		t.Errorf("该取到「增益」：%q %v", got, err)
	}
	if _, err := a.str("path"); err == nil {
		t.Error("缺参数该报错")
	}
	// 类型不对时报错，而不是当成空字符串悄悄过。
	if _, err := a.str("query"); err != nil {
		_ = err
	}
	b := args{"query": json.RawMessage(`{"x":1}`)}
	if _, err := b.str("query"); err == nil {
		t.Error("类型不对该报错")
	}
}
