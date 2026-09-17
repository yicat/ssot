// Package vembed 是嵌入（向量）的运行时适配：分词 → ONNX 推理 → CLS 池化 + L2 归一。
//
// 为什么落在 infrastructure：它依赖外部模型与 ONNX 运行时（见 docs/plans/derived-layer.md §2），
// 换掉模型这套东西就不需要了——所以不进 domain。
//
// 免 cgo 是硬约束（本机没有 gcc）：走 `github.com/getcharzp/onnxruntime_purego` 加载
// `onnxruntime.dll`。这条路的可行性由 `scripts/check/onnx-probe` 实测过
// （见 docs/notes/embedding-spike.md §六.5）。
//
// 对拍的基准由**另一套实现**产生：`scripts/check/embed-ref.mjs`（transformers.js + onnxruntime-node）
// 把 token id 与向量写进 `testdata/parity.json`，这里的测试拿它当考卷。
package vembed

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Tokenizer 是 BERT WordPiece 分词器，照 `tokenizer.json` 实现。
//
// 只实现了我们**实际需要**的那几种配置（BertNormalizer + BertPreTokenizer + WordPiece），
// 遇到别的配置**直接报错**，不装作通用。
type Tokenizer struct {
	vocab   map[string]int64
	prefix  string // 续接子词前缀，通常是 "##"
	unkID   int64
	clsID   int64
	sepID   int64
	padID   int64
	maxLen  int // 截断上限（含 [CLS]/[SEP]）
	maxChar int // 单个词的字符上限，超了整个词变 [UNK]

	cleanText    bool
	handleCJK    bool
	lowercase    bool
	stripAccents bool
}

// tokenizerFile 是 `tokenizer.json` 里我们要读的那部分。
type tokenizerFile struct {
	Truncation *struct {
		MaxLength int `json:"max_length"`
	} `json:"truncation"`
	AddedTokens []struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
		Special bool   `json:"special"`
	} `json:"added_tokens"`
	Normalizer struct {
		Type               string `json:"type"`
		CleanText          bool   `json:"clean_text"`
		HandleChineseChars bool   `json:"handle_chinese_chars"`
		StripAccents       *bool  `json:"strip_accents"`
		Lowercase          bool   `json:"lowercase"`
	} `json:"normalizer"`
	PreTokenizer struct {
		Type string `json:"type"`
	} `json:"pre_tokenizer"`
	Model struct {
		Type                    string           `json:"type"`
		UnkToken                string           `json:"unk_token"`
		ContinuingSubwordPrefix string           `json:"continuing_subword_prefix"`
		MaxInputCharsPerWord    int              `json:"max_input_chars_per_word"`
		Vocab                   map[string]int64 `json:"vocab"`
	} `json:"model"`
}

// LoadTokenizer 读 `tokenizer.json`。
func LoadTokenizer(path string) (*Tokenizer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tf tokenizerFile
	if err := json.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	if tf.Normalizer.Type != "BertNormalizer" {
		return nil, fmt.Errorf("分词器归一化类型是 %q，本实现只认 BertNormalizer", tf.Normalizer.Type)
	}
	if tf.PreTokenizer.Type != "BertPreTokenizer" {
		return nil, fmt.Errorf("预分词类型是 %q，本实现只认 BertPreTokenizer", tf.PreTokenizer.Type)
	}
	if tf.Model.Type != "WordPiece" {
		return nil, fmt.Errorf("模型类型是 %q，本实现只认 WordPiece", tf.Model.Type)
	}
	if len(tf.Model.Vocab) == 0 {
		return nil, fmt.Errorf("%s 里没有词表", path)
	}

	t := &Tokenizer{
		vocab:        tf.Model.Vocab,
		prefix:       tf.Model.ContinuingSubwordPrefix,
		maxChar:      tf.Model.MaxInputCharsPerWord,
		maxLen:       512,
		cleanText:    tf.Normalizer.CleanText,
		handleCJK:    tf.Normalizer.HandleChineseChars,
		lowercase:    tf.Normalizer.Lowercase,
		stripAccents: tf.Normalizer.Lowercase, // strip_accents 缺省时跟随 lowercase（HF 的口径）
	}
	if tf.Normalizer.StripAccents != nil {
		t.stripAccents = *tf.Normalizer.StripAccents
	}
	if tf.Truncation != nil && tf.Truncation.MaxLength > 0 {
		t.maxLen = tf.Truncation.MaxLength
	}
	if t.maxChar <= 0 {
		t.maxChar = 100
	}

	// 特殊记号：优先信 added_tokens（tokenizer.json 里就是这么标的）。
	idOf := map[string]int64{}
	for _, a := range tf.AddedTokens {
		idOf[a.Content] = a.ID
	}
	need := func(tok string) (int64, error) {
		if id, ok := idOf[tok]; ok {
			return id, nil
		}
		if id, ok := t.vocab[tok]; ok {
			return id, nil
		}
		return 0, fmt.Errorf("%s 里找不到特殊记号 %s", path, tok)
	}
	if t.unkID, err = need(tf.Model.UnkToken); err != nil {
		return nil, err
	}
	if t.clsID, err = need("[CLS]"); err != nil {
		return nil, err
	}
	if t.sepID, err = need("[SEP]"); err != nil {
		return nil, err
	}
	if t.padID, err = need("[PAD]"); err != nil {
		return nil, err
	}
	return t, nil
}

// MaxLen 是截断上限（含 [CLS] 与 [SEP]）。
func (t *Tokenizer) MaxLen() int { return t.maxLen }

// UnkID / CLSID / SEPID / PadID 是几个特殊记号的 id（测试与调试要看得见）。
func (t *Tokenizer) UnkID() int64 { return t.unkID }
func (t *Tokenizer) CLSID() int64 { return t.clsID }
func (t *Tokenizer) SEPID() int64 { return t.sepID }
func (t *Tokenizer) PadID() int64 { return t.padID }

// VocabSize 是词表大小。
func (t *Tokenizer) VocabSize() int { return len(t.vocab) }

// Encode 把一条文本切成 token id，并给出 attention mask。
//
// 顺序照 HF 的管线：normalize → pre-tokenize → WordPiece → 加 [CLS]/[SEP] → 截断。
// **截断是「砍尾巴」**：实测 transformers.js 对超长文本就是砍掉尾部，
// 最后一个 token 不是 [SEP]（见 testdata/parity.json 的长文本用例）。
func (t *Tokenizer) Encode(text string) (ids []int64, mask []int64) {
	ids = make([]int64, 0, 64)
	ids = append(ids, t.clsID)
	for _, word := range t.preTokenize(t.normalize(text)) {
		ids = append(ids, t.wordPiece(word)...)
	}
	ids = append(ids, t.sepID)
	if len(ids) > t.maxLen {
		ids = ids[:t.maxLen]
	}
	mask = make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	return ids, mask
}

// normalize 是 BertNormalizer：清理控制字符、把空白归一成空格、中文字符两侧补空格、可选小写。
func (t *Tokenizer) normalize(text string) string {
	var sb strings.Builder
	sb.Grow(len(text) + len(text)/4)
	write := func(r rune) {
		if t.handleCJK && isChineseChar(r) {
			sb.WriteByte(' ')
			sb.WriteRune(r)
			sb.WriteByte(' ')
			return
		}
		if t.lowercase {
			r = unicode.ToLower(r)
		}
		sb.WriteRune(r)
	}
	for _, r := range text {
		if t.cleanText {
			// 控制字符（含 NUL 与替换符）直接丢掉；空白一律变成普通空格。
			if r == 0 || r == 0xFFFD || unicode.IsControl(r) {
				continue
			}
			if unicode.IsSpace(r) {
				sb.WriteByte(' ')
				continue
			}
		}
		write(r)
	}
	return sb.String()
}

// preTokenize 是 BertPreTokenizer：先按空白切，再把标点单独切出来（标点自己成一个词）。
func (t *Tokenizer) preTokenize(text string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			flush()
		case isBertPunc(r):
			flush()
			out = append(out, string(r))
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

// wordPiece 是贪心最长匹配（HF 的 WordPiece::tokenize）。
//
// 整词太长（超过 max_input_chars_per_word）或有一段匹配不上，整个词变 [UNK]。
func (t *Tokenizer) wordPiece(word string) []int64 {
	chars := []rune(word)
	if len(chars) > t.maxChar {
		return []int64{t.unkID}
	}
	var out []int64
	start := 0
	for start < len(chars) {
		end := len(chars)
		var hit int64
		found := false
		for start < end {
			sub := string(chars[start:end])
			if start > 0 {
				sub = t.prefix + sub
			}
			if id, ok := t.vocab[sub]; ok {
				hit, found = id, true
				break
			}
			end--
		}
		if !found {
			return []int64{t.unkID}
		}
		out = append(out, hit)
		start = end
	}
	return out
}

// isChineseChar 是 BertNormalizer 认的中文字符范围（与 HF 的实现一致）。
func isChineseChar(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x20000 && r <= 0x2A6DF,
		r >= 0x2A700 && r <= 0x2B73F,
		r >= 0x2B740 && r <= 0x2B81F,
		r >= 0x2B820 && r <= 0x2CEAF,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0x2F800 && r <= 0x2FA1F:
		return true
	}
	return false
}

// isBertPunc 是 BertPreTokenizer 认的标点：ASCII 标点 ∪ Unicode 标点类。
//
// ⚠️ 不能只用 unicode.IsPunct：`=` `×` `|` `~` 这些是**符号类**（Sm/Sk/Sc），
// 不在 P 类里，但 ASCII 标点那一支会把 `=` 抓住（`×` 不是 ASCII 标点，按符号处理——
// 与 HF 一致：`最终伤害 = 攻击 × 系数` 的实测 id 就是这么来的）。
func isBertPunc(r rune) bool {
	if r < 128 {
		return strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", r)
	}
	return unicode.IsPunct(r)
}
