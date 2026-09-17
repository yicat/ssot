package vembed

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// newTestTokenizer 造一个极小的分词器：只够验算法（贪心最长匹配、标点切开、截断）。
func newTestTokenizer() *Tokenizer {
	return &Tokenizer{
		vocab: map[string]int64{
			"un": 1, "##aff": 2, "##able": 3,
			"玩": 4, "游": 5, "戏": 6,
			"a": 7, "b": 8, ",": 9,
		},
		prefix:       "##",
		unkID:        100,
		clsID:        101,
		sepID:        102,
		padID:        0,
		maxLen:       512,
		maxChar:      100,
		cleanText:    true,
		handleCJK:    true,
		lowercase:    false,
		stripAccents: false,
	}
}

func idsEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTokenizerAlgorithm(t *testing.T) {
	tok := newTestTokenizer()
	cases := []struct {
		name string
		text string
		want []int64
	}{
		{"续接子词贪心匹配", "unaffable", []int64{101, 1, 2, 3, 102}},
		{"有一段匹配不上就整词 [UNK]", "unknownxyz", []int64{101, 100, 102}},
		{"中文逐字（归一化时两侧补空格）", "玩游戏", []int64{101, 4, 5, 6, 102}},
		{"ASCII 标点单独成 token", "a,b", []int64{101, 7, 9, 8, 102}},
		{"空格按空白切", "a b", []int64{101, 7, 8, 102}},
		// 控制字符是被**丢掉**、不是换成空格：于是 a、b 连成一个词 "ab"，
		// 这个小词表里没有 → [UNK]。（真词表里 "ab" 是 9386，见参照实现的实测。）
		{"NUL 丢掉后两边会连成一个词", "a\x00b", []int64{101, 100, 102}},
		{"控制字符丢了、空格还在，词照样按空白切开", "a\x00 b", []int64{101, 7, 8, 102}},
		{"U+3000 也算空白", "a\u3000b", []int64{101, 7, 8, 102}},
		{"空文本也要有 [CLS]/[SEP]", "", []int64{101, 102}},
	}
	for _, c := range cases {
		got, mask := tok.Encode(c.text)
		if !idsEqual(got, c.want) {
			t.Errorf("%s：Encode(%q) = %v，想要 %v", c.name, c.text, got, c.want)
		}
		for i, m := range mask {
			if m != 1 {
				t.Errorf("%s：attention mask 该全是 1，第 %d 位是 %d", c.name, i, m)
			}
		}
	}

	// 截断：超上限直接砍尾巴（实测 transformers.js 就是这么做的，末尾不是 [SEP]）。
	tok.maxLen = 4
	got, _ := tok.Encode("玩游戏")
	if !idsEqual(got, []int64{101, 4, 5, 6}) {
		t.Errorf("截断不对：%v", got)
	}
}

// parityCase 是参照数据里的一条（由 scripts/check/embed-ref.mjs 生成）。
type parityCase struct {
	Text   string    `json:"text"`
	Why    string    `json:"why"`
	IDs    []int64   `json:"ids"`
	Mask   []int64   `json:"mask"`
	Vector []float32 `json:"vector"`
	Dim    int       `json:"dim"`
}

type parityFile struct {
	Model       string       `json:"model"`
	GeneratedBy string       `json:"generatedBy"`
	Inputs      []string     `json:"inputs"`
	Outputs     []string     `json:"outputs"`
	Cases       []parityCase `json:"cases"`
}

func loadParity(t *testing.T) parityFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "parity.json"))
	if err != nil {
		t.Fatalf("读不到参照数据（它该随仓库提交）：%v", err)
	}
	var pf parityFile
	if err := json.Unmarshal(b, &pf); err != nil {
		t.Fatalf("参照数据解析失败：%v", err)
	}
	if len(pf.Cases) == 0 {
		t.Fatal("参照数据里没有用例")
	}
	return pf
}

// modelDir 找模型目录；没配就跳过（模型不进仓库，见 docs/OPEN.md #15）。
func modelDir(t *testing.T) string {
	t.Helper()
	dir := DefaultModelDir("")
	if dir == "" {
		t.Skip("没设 SSOT_EMBED_DIR：跳过需要模型的对拍（模型与运行时怎么分发还没定，见 docs/OPEN.md #15）")
	}
	if _, err := os.Stat(filepath.Join(dir, "tokenizer.json")); err != nil {
		t.Skipf("模型目录 %s 里没有 tokenizer.json：%v", dir, err)
	}
	return dir
}

// TestTokenizerParity 是 P1 的硬验收之一：分词结果与 transformers.js **逐条一致**。
func TestTokenizerParity(t *testing.T) {
	dir := modelDir(t)
	pf := loadParity(t)

	tok, err := LoadTokenizer(filepath.Join(dir, "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tok.VocabSize(), 21128; got != want {
		t.Errorf("词表大小 %d，想要 %d——词表不对说明读错了文件", got, want)
	}
	for _, c := range pf.Cases {
		ids, mask := tok.Encode(c.Text)
		if !idsEqual(ids, c.IDs) {
			t.Errorf("分词与参照不一致：%q（%s）\n  我们 %v\n  参照 %v", c.Text, c.Why, ids, c.IDs)
			continue
		}
		if !idsEqual(mask, c.Mask) {
			t.Errorf("attention mask 与参照不一致：%q\n  我们 %v\n  参照 %v", c.Text, mask, c.Mask)
		}
	}
	if t.Failed() {
		t.Logf("参照来源：%s", pf.GeneratedBy)
	}
}

// TestVectorParity 是另一条硬验收：向量与 transformers.js 对得上（同一模型、同样池化）。
//
// 阈值是实测出来的（见 docs/notes/embedding-spike.md §六）：int8 量化模型两边都确定，
// 差异只来自浮点求和顺序，量级 1e-7。
func TestVectorParity(t *testing.T) {
	dir := modelDir(t)
	pf := loadParity(t)

	eng, err := Open(Options{ModelDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	if eng.Dim() != 512 {
		t.Errorf("模型维度该是 512，实际 %d", eng.Dim())
	}

	worstMax, worstCos := 0.0, 1.0
	for _, c := range pf.Cases {
		vec, err := eng.Embed(c.Text)
		if err != nil {
			t.Fatalf("Embed(%q) 失败：%v", c.Text, err)
		}
		if len(vec) != len(c.Vector) {
			t.Fatalf("维度不一致：%q 我们 %d，参照 %d", c.Text, len(vec), len(c.Vector))
		}
		var dot, na, nb, maxAbs float64
		for i := range vec {
			a, b := float64(vec[i]), float64(c.Vector[i])
			dot += a * b
			na += a * a
			nb += b * b
			if d := math.Abs(a - b); d > maxAbs {
				maxAbs = d
			}
		}
		cos := dot / (math.Sqrt(na) * math.Sqrt(nb))
		if cos < worstCos {
			worstCos = cos
		}
		if maxAbs > worstMax {
			worstMax = maxAbs
		}
		if n := math.Abs(math.Sqrt(na) - 1); n > 1e-6 {
			t.Errorf("%q：向量不是单位向量（模 %g）", c.Text, math.Sqrt(na))
		}
		if cos < 0.99999 {
			t.Errorf("%q：与参照的余弦只有 %.6f（%s）", c.Text, cos, c.Why)
		}
		if maxAbs > 1e-4 {
			t.Errorf("%q：与参照最大分量差 %.3e（%s）", c.Text, maxAbs, c.Why)
		}
	}
	t.Logf("对拍 %d 条：最小余弦 %.7f，最大分量差 %.2e", len(pf.Cases), worstCos, worstMax)
}
