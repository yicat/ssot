// 切块：把一篇文档的正文切成派生层的「块」。
//
// 口径来自 docs/specs/derived.spec.md 与 docs/plans/derived-layer.md §1：
//   - 抽取块与嵌入块**分开**：抽取照 LightRAG 用 2000 token（喂 LLM 的上下文），
//     嵌入用 512 子块（实测：整体召回与 2000+m3 无显著差异，但便宜 18 倍、命中更细）。
//   - 切法照 LightRAG 的段落语义思路：**标题处优先断开**，其余按段落合并到目标大小；
//     单段超长时按句子切开（不许把一句话切碎）。
//   - 每个块带**文件行号区间**（不是正文内行号）——这是我们的块级溯源，比 LightRAG 的 chunk 级更细。
//
// 本包只允许 Go 标准库（分层铁律）。
package vault

import (
	"strings"
)

// ChunkTargets 是两套块大小（token）。0 表示不切这一套。
type ChunkTargets struct {
	// Extract 是抽取用的块（喂 LLM）：LightRAG 的 DEFAULT_CHUNK_P_SIZE。
	Extract int
	// Embed 是嵌入用的子块：受嵌入模型上限约束（bge-small-zh 为 512）。
	Embed int
}

// 默认值：与 spec/plan 里定的数字一致（改这里就等于改口径，先改 spec）。
const (
	DefaultExtractTokens = 2000
	DefaultEmbedTokens   = 512
)

// DefaultChunkTargets 返回已定的两套块大小。
func DefaultChunkTargets() ChunkTargets {
	return ChunkTargets{Extract: DefaultExtractTokens, Embed: DefaultEmbedTokens}
}

// ChunkStat 是派生层块的统计（索引状态要能被人看见，「不静默」）。
type ChunkStat struct {
	Docs          int // 有同步记录的文档数
	Chunks        int // 嵌入块（chunk）
	ExtractChunks int // 抽取块（extract_chunk）
	Stale         int // 标了 stale 的文档数
}

// Chunk 是一个块。行号是**文件行号**（1 起，闭区间），与 vaultfs 的 BodyOffset 同一套。
type Chunk struct {
	// Ord 是块在文档内的序号（0 起，稳定，用于排序与增量比对）。
	Ord int
	// FromLine / ToLine 是这个块在文件里的行号区间。
	FromLine int
	ToLine   int
	// Text 是块的内容（已 trim 首尾空白）。
	Text string
	// Tokens 是估算的 token 数（见 EstimateTokens）。
	Tokens int
}

// EstimateTokens 估算一段文本的 token 数。
//
// 规则（与 scripts/check/chunk-sizing.mjs 一致，改了要两边一起改）：
//   - 汉字：1 字 ≈ 1 token（BERT WordPiece 对常用汉字基本单字成词）
//   - 拉丁字母/数字串：4 字符 ≈ 1 token
//   - 其它可见字符（标点等）：1 个 ≈ 1 token
//
// 这是**估算**，不是真分词。实测参照：`暴击伤害提高20%` 在 bge 词表下是 10 token
// （见 docs/notes/embedding-spike.md §二）。要精确就得上真分词器，那是 spike 的活。
func EstimateTokens(text string) int {
	tokens := 0
	latinRun := 0
	flushLatin := func() {
		if latinRun > 0 {
			tokens += (latinRun + 3) / 4
			latinRun = 0
		}
	}
	for _, r := range text {
		switch {
		case isHan(r):
			flushLatin()
			tokens++
		case isLatinOrDigit(r):
			latinRun++
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flushLatin()
		default:
			flushLatin()
			tokens++
		}
	}
	flushLatin()
	return tokens
}

func isHan(r rune) bool {
	return (r >= 0x3400 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff)
}

func isLatinOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// ChunkBody 把正文切成块。
//
// bodyOffset 是正文第一行在**文件**里的行号（vaultfs 给的 Doc.BodyOffset）；
// 返回的块行号 = bodyOffset + 正文内行号 - 1。bodyOffset<=0 时按 1 处理。
//
// target<=0 时用 DefaultEmbedTokens。返回空正文时返回空切片（不是错误）。
func ChunkBody(body string, bodyOffset, target int) []Chunk {
	if target <= 0 {
		target = DefaultEmbedTokens
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if bodyOffset <= 0 {
		bodyOffset = 1
	}

	// 第一步：把正文切成「段」——标题独占一段，其余按空行分段。
	// 同时记住每段的起始正文内行号（1 起），这样块的行号能一路带下去。
	type para struct {
		text  string
		line  int // 正文内起始行号（1 起）
		head  bool
		endLn int // 正文内结束行号
	}
	var paras []para
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if isHeadingLine(line) {
			paras = append(paras, para{text: strings.TrimSpace(line), line: i + 1, head: true, endLn: i + 1})
			i++
			continue
		}
		start := i
		var buf []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !isHeadingLine(lines[i]) {
			buf = append(buf, lines[i])
			i++
		}
		paras = append(paras, para{text: strings.TrimSpace(strings.Join(buf, "\n")), line: start + 1, endLn: i})
	}

	// 第二步：超长的段**先在段内**按句子打包成 ≤ target 的片段（不许切碎一句话，也不许跨段拼）。
	//
	// 为什么必须段内先打包（2026-09-18 改，P2 实测）：早先的版本把长段的句子摊平成一串句子，
	// 再在第三步与**相邻段落**一起贪心打包——结果一个块里常常横跨两段，
	// 「意思不聚焦」，检索质量明显变差（同 vault 同查询：R@1 48.3% vs 参照 65.9%，
	// 见 docs/notes/embedding-spike.md §六.8）。段落是语义单元，先包段内、再谈跨段合并。
	var pieces []para
	for _, p := range paras {
		if EstimateTokens(p.text) <= target || p.head {
			pieces = append(pieces, p)
			continue
		}
		var buf []string
		bufTok := 0
		flush := func() {
			if len(buf) == 0 {
				return
			}
			pieces = append(pieces, para{
				text: strings.TrimSpace(strings.Join(buf, "")), // 句与句直接相接（标点已在句尾）
				line: p.line, endLn: p.endLn,
			})
			buf = buf[:0]
			bufTok = 0
		}
		for _, s := range splitSentences(p.text) {
			t := EstimateTokens(s)
			if len(buf) > 0 && bufTok+t > target {
				flush()
			}
			buf = append(buf, s)
			bufTok += t
		}
		flush()
	}

	// 第三步：按目标大小合并；遇到标题就断开（块不跨标题）。
	var out []Chunk
	cur := []string{}
	curStart, curEnd := 0, 0
	curTokens := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(cur, "\n\n"))
		out = append(out, Chunk{
			Ord:      len(out),
			FromLine: bodyOffset + curStart - 1,
			ToLine:   bodyOffset + curEnd - 1,
			Text:     text,
			Tokens:   EstimateTokens(text),
		})
		cur = cur[:0]
		curTokens = 0
	}
	for _, p := range pieces {
		if p.head && len(cur) > 0 {
			flush()
		}
		t := EstimateTokens(p.text)
		if len(cur) > 0 && curTokens+t > target {
			flush()
		}
		if len(cur) == 0 {
			curStart = p.line
		}
		cur = append(cur, p.text)
		curEnd = p.endLn
		curTokens += t
	}
	flush()
	return out
}

// isHeadingLine 判断一行是不是标题。
//
// 认 `# 标题` 与 `#标题` 两种写法（Obsidian 两种都当标题），但 `## 不是标题` 之外的东西
// 不认：`#` 最多 6 个，且第 7 个字符不能再是 `#`（否则 `#######` 这种会被误判）。
func isHeadingLine(line string) bool {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, "#") {
		return false
	}
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n > 6 {
		return false
	}
	if len(t) == n {
		return true // 只有 `#`，也算标题行
	}
	return t[n] != '#'
}

// splitSentences 按句末标点切句（保留标点）。切不开时整段返回。
func splitSentences(text string) []string {
	const ends = "。！？；.!?;"
	var out []string
	var buf strings.Builder
	for _, r := range text {
		buf.WriteRune(r)
		if strings.ContainsRune(ends, r) {
			out = append(out, strings.TrimSpace(buf.String()))
			buf.Reset()
		}
	}
	if strings.TrimSpace(buf.String()) != "" {
		out = append(out, strings.TrimSpace(buf.String()))
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(text)}
	}
	return out
}
