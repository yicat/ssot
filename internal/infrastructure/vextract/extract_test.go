package vextract

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
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

// testConfig 是「一个项目会怎么声明」的样子——测试里**不写死产品数据**，
// 只用中性占位符（TYPE_X / NAME_X / EMPTY_X / DIR_X）。
func testConfig() vault.ExtractConfig {
	return vault.ExtractConfig{
		EntityTypes:        []string{"TYPE_X", "TYPE_Y"},
		IgnoreNamePatterns: []string{"*.*", "*/*", "*<*", "--*", "NAME_X.*"},
		EmptyWords:         []string{"EMPTY_X"},
		Examples:           []string{"DIR_X 是路径，不是实体"},
	}
}

func TestBuildPromptCarriesDocAndLineRange(t *testing.T) {
	p := BuildPrompt(testChunks(), testConfig())
	for _, want := range []string{"raw/式神/茨木.md", "行号: 10-30", "docs/机制/伤害.md", "行号: 40-60",
		"共 2 块", "Other", "source 与 target"} {
		if !strings.Contains(p, want) {
			t.Errorf("提示词里该有 %q", want)
		}
	}
	// 词表与忽略形状必须**来自声明**（代码里不许写死）。
	for _, want := range []string{"TYPE_X", "NAME_X.*", "EMPTY_X", "DIR_X 是路径"} {
		if !strings.Contains(p, want) {
			t.Errorf("提示词里该有声明提供的东西：%q", want)
		}
	}
	// 代码里不许再出现具体数据词（这条是 spec 的验收条款）。
	// ⚠️ 只查**指令部分**：块正文是测试夹具（里面本来就有中文词），不算代码写死。
	instr := p
	if i := strings.Index(p, "--- 块 "); i > 0 {
		instr = p[:i]
	}
	for _, bad := range []string{"式神", "技能", "增益减益", "mcp__", "ssot vault"} {
		if strings.Contains(instr, bad) {
			t.Errorf("提示词指令部分不该出现写死的数据词 %q", bad)
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

	res, err := Extract(context.Background(), f, testChunks(), Options{Config: testConfig()})
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
	// 模型给的类型不在声明词表里 → 兜底 Other（声明说了算，代码不认数据词）。
	if byName["罗生门"].Type != "Other" {
		t.Errorf("不在声明词表里的类型该收敛成 Other：%+v", byName["罗生门"])
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
	res, err := Extract(context.Background(), f, testChunks(), Options{Config: testConfig()})
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
	res, err := Extract(context.Background(), f, testChunks(), Options{Gleaning: DefaultOptions().Gleaning, Config: testConfig()})
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
	if _, err := Extract(context.Background(), &fakeCompleter{}, nil, Options{Config: testConfig()}); err == nil {
		t.Error("没有块该报错")
	}
	if _, err := Extract(context.Background(), nil, testChunks(), Options{Config: testConfig()}); err == nil {
		t.Error("没有 Completer 该报错")
	}
	f := &fakeCompleter{err: errors.New("模型挂了")}
	if _, err := Extract(context.Background(), f, testChunks(), Options{Config: testConfig()}); err == nil {
		t.Error("调用失败该往上抛")
	} else if !strings.Contains(err.Error(), "模型挂了") {
		t.Errorf("错误里该保留底层原因：%v", err)
	}
}

// TestShouldIgnoreNameIsConfigDriven：忽略是**按声明的形状**，不是代码里的清单。
func TestShouldIgnoreNameIsConfigDriven(t *testing.T) {
	cfg := testConfig()
	for _, n := range []string{"a.md", "dir/x", "占<位>符", "--flag", "EMPTY_X", "  "} {
		if ok, why := cfg.ShouldIgnoreName(n); !ok {
			t.Errorf("%q 该按声明被忽略（%s）", n, why)
		}
	}
	for _, n := range []string{"NAME_X", "NAME_X技能"} {
		if ok, why := cfg.ShouldIgnoreName(n); ok {
			t.Errorf("%q 不该被忽略：%s", n, why)
		}
	}
}

// TestIsPlainNumber：只拦纯数字；带单位的量是合法实体。
func TestIsPlainNumber(t *testing.T) {
	for _, n := range []string{"2026", "1.2", "-3"} {
		if !isPlainNumber(n) {
			t.Errorf("%q 该判成纯数字", n)
		}
	}
	for _, n := range []string{"263%", "+30", "TYPE_A", "版本2"} {
		if isPlainNumber(n) {
			t.Errorf("%q 不该判成纯数字", n)
		}
	}
}

// TestExtractDropsNoisyNames 是这一轮的验收：**按声明的形状**在代码里拦住坏名字并记原因。
//
// 夹具用中性占位符（DIR_X / NAME_X / EMPTY_X），不拿产品数据当例子——
// 测试验的是管线，不是这批数据。
func TestExtractDropsNoisyNames(t *testing.T) {
	f := &fakeCompleter{replies: []string{`{"entities":[
	  {"name":"NAME_X","type":"TYPE_X","description":"正常实体","doc":"raw/DIR_X/夹具.md","line":11},
	  {"name":"DIR_X/a.md","type":"TYPE_X","description":"像路径","doc":"raw/DIR_X/夹具.md","line":11},
	  {"name":"NAME_X.csv","type":"TYPE_X","description":"像文件名","doc":"raw/DIR_X/夹具.md","line":11},
	  {"name":"占<位>符","type":"TYPE_X","description":"像占位符","doc":"raw/DIR_X/夹具.md","line":11},
	  {"name":"--flag","type":"TYPE_X","description":"像命令行旗标","doc":"raw/DIR_X/夹具.md","line":11},
	  {"name":"EMPTY_X","type":"TYPE_X","description":"空占位词","doc":"raw/DIR_X/夹具.md","line":11},
	  {"name":"2026","type":"TYPE_X","description":"纯数字","doc":"raw/DIR_X/夹具.md","line":11}
	],"relations":[]}`}}
	chunks := []Chunk{{Doc: "raw/DIR_X/夹具.md", Ord: 0, FromLine: 10, ToLine: 30,
		Text: "正文里讲了一个东西。"}}
	res, err := Extract(context.Background(), f, chunks, Options{Config: testConfig()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entities) != 1 || res.Entities[0].Name != "NAME_X" {
		t.Fatalf("只该留下 1 个真实体：%+v", res.Entities)
	}
	if len(res.Dropped) != 6 {
		t.Fatalf("6 条该被丢掉并记原因：%+v", res.Dropped)
	}
	for _, d := range res.Dropped {
		if d.Reason == "" || d.Kind != "entity" {
			t.Errorf("丢掉的原因要写清楚：%+v", d)
		}
	}
}
func TestNormalizeTypeFollowsDeclaration(t *testing.T) {
	cfg := testConfig()
	cases := map[string]string{"TYPE_X": "TYPE_X", "type_x": "TYPE_X", "OTHER": "Other",
		"Skill": "Other", "": "Other"}
	for in, want := range cases {
		if got := cfg.NormalizeType(in); got != want {
			t.Errorf("NormalizeType(%q) = %q，想要 %q", in, got, want)
		}
	}
}

// TestRelationEndpointsCheckWholeGraph：端点**在图里已知**（上一批抽出来的）不该被丢，
// 只有「谁也不认识」的端点才丢（docs/OPEN.md #26）。
func TestRelationEndpointsCheckWholeGraph(t *testing.T) {
	f := &fakeCompleter{replies: []string{`{"entities":[
	  {"name":"NAME_X","type":"TYPE_X","description":"本批抽到的","doc":"raw/DIR_X/夹具.md","line":11}
	],"relations":[
	  {"source":"NAME_X","target":"NAME_OLD","keywords":"K","description":"引用上一批抽到的实体","doc":"raw/DIR_X/夹具.md","line":11},
	  {"source":"NAME_X","target":"NAME_GHOST","keywords":"K","description":"谁也不认识","doc":"raw/DIR_X/夹具.md","line":11}
	]}`}}
	chunks := []Chunk{{Doc: "raw/DIR_X/夹具.md", Ord: 0, FromLine: 10, ToLine: 30, Text: "正文。"}}
	// 不带上 Known：两条都该被丢（端点不在本批）。
	res, err := Extract(context.Background(), f, chunks, Options{Config: testConfig()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Relations) != 0 {
		t.Fatalf("不带 Known 时两条都该丢：%+v", res.Relations)
	}
	// 带上 Known（图里已有 NAME_OLD）：只丢「谁也不认识」的那条。
	f2 := &fakeCompleter{replies: f.replies}
	f2.replies = []string{`{"entities":[
	  {"name":"NAME_X","type":"TYPE_X","description":"本批抽到的","doc":"raw/DIR_X/夹具.md","line":11}
	],"relations":[
	  {"source":"NAME_X","target":"NAME_OLD","keywords":"K","description":"引用上一批抽到的实体","doc":"raw/DIR_X/夹具.md","line":11},
	  {"source":"NAME_X","target":"NAME_GHOST","keywords":"K","description":"谁也不认识","doc":"raw/DIR_X/夹具.md","line":11}
	]}`}
	res2, err := Extract(context.Background(), f2, chunks, Options{Config: testConfig(), Known: map[string]bool{"NAME_OLD": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Relations) != 1 || res2.Relations[0].Target != "NAME_OLD" {
		t.Fatalf("图里已知的端点不该被丢：%+v", res2.Relations)
	}
	if len(res2.Dropped) != 1 {
		t.Errorf("只该丢 1 条（端点谁也不认识）：%+v", res2.Dropped)
	}
}
