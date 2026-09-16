package vault

import (
	"strings"
	"testing"
)

// 双链解析：四种写法都要认，且别名要先于锚点拆——顺序错了会把 `[[a|b#c]]` 解析歪。
func TestParseLinks(t *testing.T) {
	body := strings.Join([]string{
		"看 [[docs/式神/茨木童子]] 和 [[机制/伤害计算#公式]]。",
		"依据是 [[raw/灰机wiki/茨木童子#^第3段]]，别名叫 [[茨木童子|那只鬼]]。",
		"嵌入 ![[tables/技能倍率.csv]] 是嵌入。",
		"没闭合的 [[别当链接 和普通方括号 [x] 都不算。",
	}, "\n")

	links := ParseLinks(body)
	if len(links) != 5 {
		t.Fatalf("应解析出 5 条双链，实际 %d：%+v", len(links), links)
	}
	cases := []struct {
		target  string
		heading string
		block   string
		alias   string
		embed   bool
	}{
		{"docs/式神/茨木童子", "", "", "", false},
		{"机制/伤害计算", "公式", "", "", false},
		{"raw/灰机wiki/茨木童子", "", "第3段", "", false},
		{"茨木童子", "", "", "那只鬼", false},
		{"tables/技能倍率.csv", "", "", "", true},
	}
	for i, c := range cases {
		got := links[i]
		if got.Target != c.target || got.Heading != c.heading || got.Block != c.block || got.Alias != c.alias || got.Embed != c.embed {
			t.Errorf("第 %d 条解析错：%+v（期望 target=%q heading=%q block=%q alias=%q embed=%v）",
				i, got, c.target, c.heading, c.block, c.alias, c.embed)
		}
	}
	// 没闭合的那条不能算：把不完整的东西当链接，会让人以为引用已经建立。
	for _, l := range links {
		if strings.Contains(l.Target, "别当链接") {
			t.Error("没闭合的 [[ 不该被当成链接")
		}
	}
}

// 命令行里手打整串方括号很烦，两种都要能解析。
func TestParseLinkWithoutBrackets(t *testing.T) {
	l, err := ParseLink("raw/灰机wiki/茨木童子#^第3段")
	if err != nil {
		t.Fatal(err)
	}
	if l.Target != "raw/灰机wiki/茨木童子" || l.Block != "第3段" {
		t.Errorf("解析错：%+v", l)
	}
}

func fixtureDocs() []Doc {	return []Doc{
		{Path: "docs/式神/茨木童子.md", Title: "茨木童子", Status: StatusDraft,
			Body: "见 [[docs/机制/伤害计算#公式]]。"},
		{Path: "docs/机制/伤害计算.md", Title: "伤害计算", Status: StatusPublished,
			Body: "公式见下。\n依据 [[raw/灰机wiki/茨木童子#^第3段]]。\n也链 [[茨木童子]]。\n还链 [[不存在的东西]]。"},
		{Path: "raw/灰机wiki/茨木童子.md", Title: "灰机wiki：茨木童子",
			Body: "原文第一段。\n原文第二段。\n伤害系数为 1.5。 ^第3段"},
	}
}

func withLinks(docs []Doc) []Doc {
	for i := range docs {
		docs[i].Links = ParseLinks(docs[i].Body)
	}
	return docs
}

// mustLink 走 ParseLink 造链接：锚点是解析出来的，不是手塞进 Target 的。
func mustLink(t *testing.T, ref string) Link {
	t.Helper()
	l, err := ParseLink(ref)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// 目标可以只写文件名（像 Obsidian 那样），也可以写全路径。
func TestResolveByPathAndByBasename(t *testing.T) {
	docs := withLinks(fixtureDocs())

	res, err := Resolve(docs, mustLink(t, "raw/灰机wiki/茨木童子#^第3段"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "raw/灰机wiki/茨木童子.md" {
		t.Errorf("全路径应命中：%q", res.Path)
	}
	if res.BlockLine != 3 || !strings.Contains(res.BlockText, "1.5") {
		t.Errorf("块锚点该指到第 3 行：line=%d text=%q", res.BlockLine, res.BlockText)
	}

	// 名字唯一时可以只写文件名；同名多篇的情况见下面的歧义测试。
	res, err = Resolve(docs, Link{Target: "伤害计算"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "docs/机制/伤害计算.md" {
		t.Errorf("只写文件名应命中那篇：%q", res.Path)
	}
}

// 同名多篇时不猜：报出候选让调用方去问人（系统不裁决）。
func TestResolveAmbiguousReportsCandidates(t *testing.T) {
	docs := withLinks([]Doc{
		{Path: "docs/式神/茨木童子.md", Body: ""},
		{Path: "raw/灰机wiki/茨木童子.md", Body: ""},
	})
	res, err := Resolve(docs, Link{Target: "茨木童子"})
	if err == nil {
		t.Fatal("同名两篇应报歧义")
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("应给出两个候选，实际 %v", res.Candidates)
	}
	for _, c := range res.Candidates {
		if !strings.Contains(err.Error(), c) {
			t.Errorf("错误信息里该列出候选 %q：%v", c, err)
		}
	}
}

// 块锚点只在行尾才算：正文里出现 ^abc 是普通字符（否则会把普通文本当锚点）。
func TestFindBlockOnlyAtLineEnd(t *testing.T) {
	body := "这里提到 ^甲 但不在行尾。\n这一行才是锚点。 ^甲\n"
	line, text, ok := FindBlock(body, "甲")
	if !ok || line != 2 || !strings.Contains(text, "才是锚点") {
		t.Errorf("应命中第 2 行，实际 ok=%v line=%d text=%q", ok, line, text)
	}
	if _, _, ok := FindBlock(body, "乙"); ok {
		t.Error("不存在的块不该命中")
	}
}

func TestBacklinksAndBroken(t *testing.T) {
	docs := withLinks(fixtureDocs())

	back := Backlinks(docs, "raw/灰机wiki/茨木童子.md")
	if len(back) != 1 || back[0].From != "docs/机制/伤害计算.md" {
		t.Fatalf("反链应来自伤害计算那篇：%+v", back)
	}
	if back[0].Link.Block != "第3段" {
		t.Errorf("反链该带上块锚点，便于显示「他引的是哪一段」：%+v", back[0].Link)
	}

	// 两条问题链接：一条断链、一条歧义（fixture 里有两篇同名的茨木童子）。
	// 这两件事必须分开——混成一类会让人以为内容缺失，实际是命名冲突。
	issues := LinkIssues(docs, "docs/机制/伤害计算.md")
	if len(issues) != 2 {
		t.Fatalf("应报出两条问题链接：%+v", issues)
	}
	kinds := map[string]LinkIssueKind{}
	for _, is := range issues {
		kinds[is.Link.Target] = is.Kind
	}
	if kinds["不存在的东西"] != IssueBroken {
		t.Errorf("「不存在的东西」该判为断链：%v", kinds)
	}
	if kinds["茨木童子"] != IssueAmbiguous {
		t.Errorf("同名两篇该判为指不清而不是断链：%v", kinds)
	}
}

// 发布态只认三个值；打错要拒绝，不能悄悄退回 draft。
func TestParseStatusRejectsUnknown(t *testing.T) {
	if _, err := ParseStatus("publishd"); err == nil {
		t.Error("拼错的 status 必须报错")
	}
	if _, err := ParseStatus(""); err == nil {
		t.Error("空 status 必须报错（该怎么处理由调用方决定，不在这里猜）")
	}
	if st, err := ParseStatus("published"); err != nil || st != StatusPublished {
		t.Errorf("正常值应通过：%v %v", st, err)
	}
}

func TestParseActor(t *testing.T) {
	a, err := ParseActor("agent:整理")
	if err != nil || a.Kind != ActorAgent || a.Name != "整理" {
		t.Fatalf("解析 agent 失败：%+v %v", a, err)
	}
	if a.Trailer() != "agent:整理" {
		t.Errorf("trailer 该是 kind:name：%q", a.Trailer())
	}
	if _, err := ParseActor("机器"); err == nil {
		t.Error("不认识的操作者必须报错")
	}
}

// 核心规则一：只有人能发布或归档。
func TestOnlyHumanCanChangeStatus(t *testing.T) {
	agent, _ := ParseActor("agent:核验")
	human, _ := ParseActor("human:我")

	if agent.CanChangeStatus() {
		t.Error("agent 不该能改发布态")
	}
	if !human.CanChangeStatus() {
		t.Error("人应该能改发布态")
	}
}

// 核心规则二：agent 改过一律回落 draft，哪怕原来是 published。
func TestAgentEditFallsBackToDraft(t *testing.T) {
	agent, _ := ParseActor("agent:优化")
	human, _ := ParseActor("human:我")

	if got := agent.StatusAfterEdit(StatusPublished); got != StatusDraft {
		t.Errorf("agent 改过应回落 draft，实际 %q", got)
	}
	if got := human.StatusAfterEdit(StatusPublished); got != StatusPublished {
		t.Errorf("人改过应保持原状态，实际 %q", got)
	}
}
