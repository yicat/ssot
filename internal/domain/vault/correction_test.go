package vault

import (
	"strings"
	"testing"
)

// 纠正块的语法：docs/specs/document.spec.md「纠正块的写法」。
func TestParseCorrectionsReadsTheBlock(t *testing.T) {
	body := strings.Join([]string{
		"# 茨木童子",                          // 1
		"",                                    // 2
		"鬼手是左手。",                          // 3
		"",                                    // 4
		"> [!correction] 茨木童子：鬼手是右手",  // 5
		"> 类型：式神",                          // 6
		"> 说明：旧版设定里写成左手。",           // 7
		"",                                    // 8
		"后面还有正文。",                        // 9
	}, "\n")

	cors, bad := ParseCorrections(body, 1)
	if len(bad) != 0 {
		t.Fatalf("这条写对了，不该报没生效：%+v", bad)
	}
	if len(cors) != 1 {
		t.Fatalf("该认出 1 条纠正，实际 %d：%+v", len(cors), cors)
	}
	c := cors[0]
	if c.Name != "茨木童子" || c.Type != "式神" || c.Description != "鬼手是右手" {
		t.Errorf("名字/类型/说法读错了：%+v", c)
	}
	if c.FromLine != 5 || c.ToLine != 7 {
		t.Errorf("行号该覆盖整个块（5-7），实际 %d-%d", c.FromLine, c.ToLine)
	}
	if !strings.Contains(c.Note, "旧版设定") {
		t.Errorf("其余行该留给「给人看」：%q", c.Note)
	}
	if !strings.Contains(c.Note, "说明") {
		t.Errorf("其余行该原样保留：%q", c.Note)
	}
}

// 行号是**文件行号**：bodyOffset 不是 1 的时候要加上它（块的行号要能直接跳转）。
func TestParseCorrectionsUsesFileLineNumbers(t *testing.T) {
	body := "前言。\n\n> [!correction] A：改过的说法\n> 类型：T\n"
	cors, _ := ParseCorrections(body, 7) // 正文第一行是文件第 7 行
	if len(cors) != 1 {
		t.Fatalf("该认出 1 条：%+v", cors)
	}
	// 正文第一行是文件第 7 行；块是正文第 3-4 行 → 文件第 9-10 行（与 ChunkBody 一个算法：
	// 文件行 = bodyOffset + 正文行 - 1）。
	if cors[0].FromLine != 9 || cors[0].ToLine != 10 {
		t.Errorf("该换算成文件行号（9-10），实际 %d-%d", cors[0].FromLine, cors[0].ToLine)
	}
}

// 没写对的要**逐条报出来**，不能静默（人改了没反应，会以为系统坏了）。
func TestParseCorrectionsReportsWhatIsWrong(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺类型行", "> [!correction] 茨木童子：鬼手是右手\n", "类型"},
		{"类型写成空", "> [!correction] 茨木童子：鬼手是右手\n> 类型：\n", "类型"},
		{"标题行没有冒号", "> [!correction] 茨木童子鬼手是右手\n> 类型：式神\n", "标题行"},
		{"名字为空", "> [!correction] ：鬼手是右手\n> 类型：式神\n", "标题行"},
		{"说法为空", "> [!correction] 茨木童子：\n> 类型：式神\n", "标题行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cors, bad := ParseCorrections(tc.body, 1)
			if len(cors) != 0 {
				t.Fatalf("这条不该生效：%+v", cors)
			}
			if len(bad) != 1 {
				t.Fatalf("该报 1 条没生效：%+v", bad)
			}
			if bad[0].Line != 1 {
				t.Errorf("要报块的第一行行号，实际 %d", bad[0].Line)
			}
			if !strings.Contains(bad[0].Reason, tc.want) {
				t.Errorf("原因里该提到 %q，实际 %q", tc.want, bad[0].Reason)
			}
		})
	}
}

// 别的东西不许被当成纠正块。
func TestParseCorrectionsIgnoresOtherBlocks(t *testing.T) {
	body := strings.Join([]string{
		"> [!note] 这是一条普通 callout",
		"> 类型：不是纠正",
		"",
		"> 没有关键字的引用块",
		"",
		"普通正文里的 [!correction] 字样不算（不在引用行里）",
	}, "\n")
	cors, bad := ParseCorrections(body, 1)
	if len(cors) != 0 || len(bad) != 0 {
		t.Errorf("这些都不该被当成纠正块：%+v %+v", cors, bad)
	}
	if HasCorrections(body) {
		t.Error("HasCorrections 也不该认")
	}
}

// 关键字大小写、半角冒号都认（人写东西不该为一个字母白撞一次）。
func TestParseCorrectionsAcceptsVariants(t *testing.T) {
	body := "> [!Correction] 茨木童子: 鬼手是右手\n> 类型: 式神\n"
	cors, bad := ParseCorrections(body, 1)
	if len(bad) != 0 || len(cors) != 1 {
		t.Fatalf("大小写与半角冒号都该认：%+v %+v", cors, bad)
	}
	if cors[0].Name != "茨木童子" || cors[0].Type != "式神" || cors[0].Description != "鬼手是右手" {
		t.Errorf("读出来的内容不对：%+v", cors[0])
	}
	if !HasCorrections(body) {
		t.Error("HasCorrections 该认出来")
	}
}

// 块到**第一个不是 `>` 的行**就结束：块外的行不能被卷进来。
func TestParseCorrectionsBlockEndsAtFirstNonQuoteLine(t *testing.T) {
	body := "> [!correction] 茨木童子：鬼手是右手\n> 类型：式神\n正文这一行不是引用\n> 类型：别的\n"
	cors, bad := ParseCorrections(body, 1)
	if len(bad) != 0 || len(cors) != 1 {
		t.Fatalf("该是一条生效的纠正：%+v %+v", cors, bad)
	}
	if cors[0].ToLine != 2 {
		t.Errorf("块该在第 2 行结束，实际到 %d", cors[0].ToLine)
	}
	if cors[0].Type != "式神" {
		t.Errorf("块外那行不该影响类型：%q", cors[0].Type)
	}
}

// BlankCorrections 是给抽取块用的：文本抹掉，**行数一个不变**（否则行号全错位）。
func TestBlankCorrectionsKeepsLineCount(t *testing.T) {
	body := strings.Join([]string{
		"第一段。",
		"",
		"> [!correction] 茨木童子：鬼手是右手",
		"> 类型：式神",
		"",
		"第二段。",
	}, "\n")
	out := BlankCorrections(body)
	if strings.Contains(out, "鬼手是右手") || strings.Contains(out, "[!correction]") {
		t.Errorf("纠正块该被抹掉：%q", out)
	}
	if !strings.Contains(out, "第一段。") || !strings.Contains(out, "第二段。") {
		t.Errorf("别的行该原样留着：%q", out)
	}
	inLines := strings.Split(body, "\n")
	outLines := strings.Split(out, "\n")
	if len(inLines) != len(outLines) {
		t.Fatalf("行数必须不变（%d → %d）：抹行会让后面所有块的行号错位", len(inLines), len(outLines))
	}
	for i := range inLines {
		// 引用行（`>` 开头）可能就是块的一部分，会被抹掉；**非引用行一个都不许动**。
		if strings.HasPrefix(strings.TrimSpace(inLines[i]), ">") {
			continue
		}
		if outLines[i] != inLines[i] {
			t.Errorf("第 %d 行被动过了：%q → %q", i+1, inLines[i], outLines[i])
		}
	}
	// 没写对的纠正块也照样抹掉：它是给派生层的指令，不该进抽取块。
	badBlock := "正文。\n> [!correction] 没写类型\n"
	if strings.Contains(BlankCorrections(badBlock), "[!correction]") {
		t.Errorf("没写对的块也不该喂给模型：%q", BlankCorrections(badBlock))
	}
}
