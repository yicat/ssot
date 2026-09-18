package vextract

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeCompleter 按顺序回包，用来在**不花钱**的前提下验提示词、解析、校验、补抽。
type fakeCompleter struct {
	replies []string
	prompts []string
	err     error
}

func (f *fakeCompleter) Complete(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	if len(f.replies) == 0 {
		return `{"entities":[],"relations":[]}`, nil
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r, nil
}

func testChunks() []Chunk {
	return []Chunk{
		{Doc: "raw/式神/茨木.md", Ord: 0, FromLine: 10, ToLine: 30,
			Text: "茨木童子靠鬼手打伤害。\n三技能「罗生门」系数 263%。"},
		{Doc: "docs/机制/伤害.md", Ord: 1, FromLine: 40, ToLine: 60,
			Text: "最终伤害 = 攻击 × 系数。"},
	}
}

func TestBuildPromptCarriesDocAndLineRange(t *testing.T) {
	p := BuildPrompt(testChunks())
	for _, want := range []string{"raw/式神/茨木.md", "行号: 10-30", "docs/机制/伤害.md", "行号: 40-60",
		"共 2 块", "Other", "source 与 target"} {
		if !strings.Contains(p, want) {
			t.Errorf("提示词里该有 %q", want)
		}
	}
	// 实测踩过的坏例必须写进提示词（否则模型还会把文件名/命令示例当实体）。
	for _, want := range []string{"文件名、表名、路径", "命令与用法示例", "增益减益.csv", "ssot vault",
		"Other 太多等于没分类"} {
		if !strings.Contains(p, want) {
			t.Errorf("提示词里该有防坏例的一条：%q", want)
		}
	}
	if strings.Count(p, "--- 块 ") != 2 {
		t.Errorf("两块该有两个块头：\n%s", p)
	}
}

func TestParseResponseToleratesNoiseAndFences(t *testing.T) {
	noisy := "dsh: reasoning: 我先看看块里有什么…\n```json\n" +
		`{"entities":[{"name":"茨木童子","type":"式神","description":"靠鬼手输出","doc":"raw/式神/茨木.md","line":11}],` +
		`"relations":[]}` + "\n```\n以上。"
	raw, err := ParseResponse(noisy)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Entities) != 1 || raw.Entities[0].Name != "茨木童子" {
		t.Fatalf("没从噪声里挑出 JSON：%+v", raw)
	}

	if _, err := ParseResponse("什么都没有"); err == nil {
		t.Error("没有 JSON 时该报错")
	}
	if _, err := ParseResponse(`{"entities":[{"name":"x"`); err == nil {
		t.Error("JSON 坏了该报错")
	} else if !strings.Contains(err.Error(), "开头") {
		t.Errorf("报错要带回包开头，便于排查：%v", err)
	}
	// 实测踩过：模型把答案拆成两个 JSON 对象连着吐（20 篇跑批第 34 批挂在这）。
	two := `{"entities":[{"name":"甲","type":"式神","doc":"d","line":1}]}` +
		`{"entities":[{"name":"乙","type":"技能","doc":"d","line":1}],"relations":[]}`
	merged, err := ParseResponse(two)
	if err != nil {
		t.Fatalf("连着两个 JSON 对象该能解析：%v", err)
	}
	if len(merged.Entities) != 2 {
		t.Errorf("两个对象该合并成 2 个实体：%+v", merged.Entities)
	}
	if _, err := ParseResponse("   "); err == nil {
		t.Error("空回包该报错")
	}
}

func TestExtractKeepsProvenanceAndDropsBadLines(t *testing.T) {
	f := &fakeCompleter{replies: []string{`{"entities":[
	  {"name":"茨木童子","type":"式神","description":"靠鬼手输出","doc":"raw/式神/茨木.md","line":11},
	  {"name":"罗生门","type":"技能","description":"三技能","doc":"raw/式神/茨木.md","line":25},
	  {"name":"越界的","type":"技能","description":"行号不在块里","doc":"raw/式神/茨木.md","line":999},
	  {"name":"别处的","type":"物品","description":"doc 不在这批里","doc":"raw/别的.md","line":5},
	  {"name":"","type":"技能","description":"没名字"}
	],"relations":[
	  {"source":"茨木童子","target":"罗生门","keywords":"技能/伤害","description":"拥有","doc":"raw/式神/茨木.md","line":25},
	  {"source":"茨木童子","target":"不存在的东西","keywords":"x","description":"悬空","doc":"raw/式神/茨木.md","line":25}
	]}`}}

	res, err := Extract(context.Background(), f, testChunks(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entities) != 2 {
		t.Fatalf("该只留 2 个实体（越界/doc 不对/没名字的都丢掉）：%+v", res.Entities)
	}
	byName := map[string]Entity{}
	for _, e := range res.Entities {
		byName[e.Name] = e
	}
	ci := byName["茨木童子"]
	if ci.FromLine != 10 || ci.ToLine != 30 || ci.Line != 11 {
		t.Errorf("溯源不对：%+v", ci.Provenance)
	}
	if ci.Note != "" {
		t.Errorf("行号合法时不该有说明：%q", ci.Note)
	}
	if byName["罗生门"].Type != "技能" {
		t.Errorf("类型该按词表收敛：%+v", byName["罗生门"])
	}
	if len(res.Relations) != 1 {
		t.Fatalf("端点悬空的关系该丢掉：%+v", res.Relations)
	}
	if res.Relations[0].FromLine != 10 || res.Relations[0].ChunkOrd != 0 {
		t.Errorf("关系的溯源与块序号不对：%+v", res.Relations[0])
	}
	// 丢掉的东西必须留下原因（不静默）。
	kinds := map[string]string{}
	for _, d := range res.Dropped {
		kinds[d.Name] = d.Reason
	}
	for _, name := range []string{"越界的", "别处的", "茨木童子→不存在的东西"} {
		if kinds[name] == "" {
			t.Errorf("%q 该在 drop 清单里并带原因：%+v", name, res.Dropped)
		}
	}
	if res.Rounds != 1 {
		t.Errorf("默认不补抽时该只有 1 轮，实际 %d", res.Rounds)
	}
}

func TestExtractNoLineFallsBackToChunkRange(t *testing.T) {
	f := &fakeCompleter{replies: []string{`{"entities":[
	  {"name":"茨木童子","type":"式神","description":"没给行号","doc":"raw/式神/茨木.md"}
	],"relations":[]}`}}
	res, err := Extract(context.Background(), f, testChunks(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	e := res.Entities[0]
	if e.Line != 0 || e.FromLine != 10 || e.ToLine != 30 {
		t.Errorf("没给行号时该退回块区间：%+v", e.Provenance)
	}
	if !strings.Contains(e.Note, "没给行号") {
		t.Errorf("退回要有说明（不静默）：%q", e.Note)
	}
}

func TestExtractGleaningMergesAndDedupes(t *testing.T) {
	f := &fakeCompleter{replies: []string{
		// 首轮
		`{"entities":[{"name":"茨木童子","type":"式神","description":"靠鬼手输出","doc":"raw/式神/茨木.md","line":11}],
		   "relations":[]}`,
		// 补抽轮：一条重复、一条新的
		`{"entities":[{"name":"茨木童子","type":"式神","description":"重复的","doc":"raw/式神/茨木.md","line":12},
		              {"name":"鬼手","type":"技能","description":"补抽到的","doc":"raw/式神/茨木.md","line":26}],
		   "relations":[]}`,
	}}
	res, err := Extract(context.Background(), f, testChunks(), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if res.Rounds != 2 {
		t.Errorf("默认该跑 2 轮（首轮 + 补抽），实际 %d", res.Rounds)
	}
	if len(res.Entities) != 2 {
		t.Fatalf("重复的该去掉，留 2 条：%+v", res.Entities)
	}
	if !strings.Contains(f.prompts[1], "上一轮你已经抽出了这些") {
		t.Error("补抽轮的提示词该带上上一轮回包")
	}
	if !strings.Contains(f.prompts[1], "只输出新增的") {
		t.Error("补抽轮该要求只输出新增")
	}
}

func TestExtractErrors(t *testing.T) {
	if _, err := Extract(context.Background(), &fakeCompleter{}, nil, Options{}); err == nil {
		t.Error("没有块该报错")
	}
	if _, err := Extract(context.Background(), nil, testChunks(), Options{}); err == nil {
		t.Error("没有 Completer 该报错")
	}
	f := &fakeCompleter{err: errors.New("模型挂了")}
	if _, err := Extract(context.Background(), f, testChunks(), Options{}); err == nil {
		t.Error("调用失败该往上抛")
	} else if !strings.Contains(err.Error(), "模型挂了") {
		t.Errorf("错误里该保留底层原因：%v", err)
	}
}

func TestLooksLikeNoise(t *testing.T) {
	noise := []string{
		"增益减益.csv", "docs/式神/<名字>.md", "raw/式神/茨木.md", "tables/式神技能.csv",
		"ssot vault", "dsh --profile headless", "mcp__ssot__file_read",
		"2026", "1.2", "无", "待定", "N/A", "https://yys.huijiwiki.com/wiki/x", "",
	}
	for _, n := range noise {
		if ok, why := LooksLikeNoise(n); !ok {
			t.Errorf("%q 该被判成噪声（原因：%s）", n, why)
		}
	}
	// 真正的实体名一个都不能误伤。
	keep := []string{"茨木童子", "罗生门", "伤害计算", "狐影戏法", "平安京", "SR", "263%"}
	for _, n := range keep {
		if ok, why := LooksLikeNoise(n); ok {
			t.Errorf("%q 不该被判成噪声：%s", n, why)
		}
	}
}

// TestExtractDropsNoisyNames 是这一轮的验收：坏例在**代码里**被拦住并记下原因。
func TestExtractDropsNoisyNames(t *testing.T) {
	f := &fakeCompleter{replies: []string{`{"entities":[
	  {"name":"茨木童子","type":"式神","description":"正常实体","doc":"raw/式神/茨木.md","line":11},
	  {"name":"docs/式神/<名字>.md","type":"物品","description":"文件路径","doc":"raw/式神/茨木.md","line":11},
	  {"name":"ssot vault","type":"Other","description":"命令示例","doc":"raw/式神/茨木.md","line":11},
	  {"name":"2026","type":"数值","description":"年份","doc":"raw/式神/茨木.md","line":11},
	  {"name":"无","type":"Other","description":"空占位","doc":"raw/式神/茨木.md","line":11}
	],"relations":[]}`}}
	res, err := Extract(context.Background(), f, testChunks(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entities) != 1 || res.Entities[0].Name != "茨木童子" {
		t.Fatalf("只该留下真实体：%+v", res.Entities)
	}
	if len(res.Dropped) != 4 {
		t.Fatalf("4 条噪声该被丢掉并记原因：%+v", res.Dropped)
	}
	for _, d := range res.Dropped {
		if d.Reason == "" || d.Kind != "entity" {
			t.Errorf("丢掉的原因要写清楚：%+v", d)
		}
	}
}

func TestEntityTypeFallsBackToOther(t *testing.T) {
	cases := map[string]string{"式神": "式神", "技能": "技能", "OTHER": "Other",
		"Skill": "Other", "": "Other", "  机制  ": "机制"}
	for in, want := range cases {
		if got := entityType(in); got != want {
			t.Errorf("entityType(%q) = %q，想要 %q", in, got, want)
		}
	}
}
