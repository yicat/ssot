// Package vextract 是实体/关系抽取：把若干**块**合成一次调用喂给模型，把回包的 JSON 变成
// 带块级溯源的 entity / relation。
//
// 为什么落在 infrastructure 而不是 domain：它依赖外部模型（见 docs/plans/derived-layer.md §2
// 「换掉模型就不需要了 → 不进 domain」）。这里只做**提示词、解析、校验**这些确定的活；
// 「谁来跑这次调用」由 Completer 端口决定（现在接的是 dsh，将来换后端不影响这里）。
//
// 口径来自 docs/specs/derived.spec.md：
//   - §二：字段与提示词结构照 LightRAG 借（entity_name / entity_type / entity_description；
//     source / target / keywords / description；类型词表不合用就 Other），**每条加** doc + 行号 + status；
//   - §三.1：**不许一块一次调用**——固定开销按调用收（实测单次 ~21.5k 输入），必须多块合一次；
//   - §三.2：抽取这类机械任务默认关推理（overlay 在 `.dsh/extraction.patch.yml`）；
//   - §六.4：**提示词与实体类型词表还没定稿**——下面这版是起点，改它要有跑批的证据。
//
// ⚠️ 本包**不写数据库、不碰 status**：抽取产物只是派生的候选，发布与核验仍然是人（ADR 0009）。
package vextract

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// Chunk 是喂给模型的一块（只要抽取要用到的东西：身份、行号、正文）。
type Chunk struct {
	Doc      string // 相对路径（块的来源）
	Ord      int    // 块在文档内的序号（回查用）
	FromLine int    // 在**文件**里的起始行
	ToLine   int    // 在文件里的结束行
	Text     string
}

// Provenance 是「这条从哪儿来」——块级溯源。
//
// 行号有两种：**块的行号区间**一定来自我们（可信）；**模型给的行号**要校验，
// 落在区间内才采信，否则记 0 并留一句说明（不静默地编一个行号）。
type Provenance struct {
	Doc      string
	FromLine int
	ToLine   int
	Line     int    // 模型给的行号（已校验）；0 = 没给或越界
	Note     string // 行号上的说明（空 = 一切正常）
}

// Entity 是一个实体。
type Entity struct {
	Name        string
	Type        string // 词表见 EntityTypes；不合用用 Other
	Description string
	Provenance
}

// Relation 是一条关系。Source / Target 必须是同一批里抽到的实体名。
type Relation struct {
	Source      string
	Target      string
	Keywords    string
	Description string
	Provenance
	// ChunkOrd 是它来自哪一块（同一文档内序号），便于回查原文。
	ChunkOrd int
}

// Drop 是被丢掉的条目与原因。「不静默」：丢什么、为什么丢，都要能查。
type Drop struct {
	Kind   string // entity / relation
	Name   string // 名字（能取到的话）
	Reason string
}

// Result 是一次抽取的结果。
type Result struct {
	Entities  []Entity
	Relations []Relation
	Dropped   []Drop
	// Rounds 是实际跑了几轮（首轮 + 补抽轮）。
	Rounds int
}

// Completer 是「把提示词发给模型、拿回文本」这件事的端口。
//
// 实现方负责：用受限工具集 + 关推理（`ssot-agent` 那套）、把模型输出**原样**拿回来
// （可能夹着推理痕迹，所以解析要能从中挑出 JSON）。
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Options 是一次抽取的参数。
type Options struct {
	// Gleaning 是补抽轮数（LightRAG 的 DEFAULT_MAX_GLEANING = 1）。0 = 只抽一轮。
	Gleaning int
	// Config 是**项目声明**来的抽取配置（类型词表 / 忽略的名字形状 / 空占位词 / 反例）。
	// 零值 = 只有兜底 Other、不忽略任何形状——即「代码里没有默认数据知识」。
	Config vault.ExtractConfig
	// Known 是**图里已经有的实体名**（不在本批、但已经抽出来过的那些）。
	// 关系的端点校验要连它一起看：只按本批校验会把「实体在上一批抽出、这一批引用它」的关系丢掉
	// （实测：20 篇丢了 3 条，全库会放大 —— docs/OPEN.md #26）。
	Known map[string]bool
}

// DefaultOptions 是已定的默认值：照 LightRAG 做一轮补抽。
//
// ⚠️ 补抽会让调用数翻倍（成本×2）——所以它必须能被显式关掉，跑了批再定。
func DefaultOptions() Options { return Options{Gleaning: 1} }

// Extract 对一批块做抽取（多块一次调用，见包注释 §三.1）。
func Extract(ctx context.Context, c Completer, chunks []Chunk, opt Options) (Result, error) {
	if len(chunks) == 0 {
		return Result{}, fmt.Errorf("没有块可抽")
	}
	if c == nil {
		return Result{}, fmt.Errorf("没有给 Completer（谁来跑这次模型调用）")
	}

	var out Result
	seenEnt := map[string]bool{}
	seenRel := map[string]bool{}

	rounds := 1 + max(opt.Gleaning, 0)
	prev := ""
	for round := 0; round < rounds; round++ {
		prompt := BuildPrompt(chunks, opt.Config)
		if round > 0 {
			prompt = BuildGleaningPrompt(chunks, prev, opt.Config)
		}
		text, err := c.Complete(ctx, prompt)
		if err != nil {
			return out, fmt.Errorf("第 %d 轮调用失败：%w", round+1, err)
		}
		prev = text
		raw, err := ParseResponse(text)
		if err != nil {
			return out, fmt.Errorf("第 %d 轮回包解析失败：%w", round+1, err)
		}
		mergeInto(&out, raw, chunks, opt, seenEnt, seenRel)
		out.Rounds = round + 1
	}
	sortResult(&out)
	return out, nil
}

// mergeInto 把一轮的原始产物校验后并进结果（去重、丢坏条目并记原因）。
func mergeInto(out *Result, raw rawExtraction, chunks []Chunk, opt Options, seenEnt, seenRel map[string]bool) {
	cfg := opt.Config
	byDoc := map[string][]Chunk{}
	for _, c := range chunks {
		byDoc[c.Doc] = append(byDoc[c.Doc], c)
	}

	known := map[string]bool{}
	for _, e := range raw.Entities {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			out.Dropped = append(out.Dropped, Drop{Kind: "entity", Reason: "没给 name"})
			continue
		}
		ch, ok := locate(byDoc, e.Doc, e.Line)
		if !ok {
			out.Dropped = append(out.Dropped, Drop{Kind: "entity", Name: name, Reason: fmt.Sprintf("doc=%q 不在这批块里", e.Doc)})
			continue
		}
		// 过滤分两层：**通用形状**（纯数字，见 filter.go）与**项目声明的形状**（name_patterns）。
		if isPlainNumber(name) {
			out.Dropped = append(out.Dropped, Drop{Kind: "entity", Name: name, Reason: "只有数字，不是实体名"})
			continue
		}
		if noise, why := cfg.ShouldIgnoreName(name); noise {
			out.Dropped = append(out.Dropped, Drop{Kind: "entity", Name: name, Reason: why})
			continue
		}
		key := name + "\x00" + strings.TrimSpace(e.Type)
		known[name] = true
		if seenEnt[key] {
			continue // 补抽轮里重复的：不重复入账，也不记 drop（那是正常的）
		}
		seenEnt[key] = true
		ent := Entity{
			Name: name, Type: cfg.NormalizeType(e.Type), Description: strings.TrimSpace(e.Description),
			Provenance: prov(ch, e.Line),
		}
		out.Entities = append(out.Entities, ent)
	}
	for _, r := range raw.Relations {
		src, dst := strings.TrimSpace(r.Source), strings.TrimSpace(r.Target)
		if src == "" || dst == "" {
			out.Dropped = append(out.Dropped, Drop{Kind: "relation", Reason: "source/target 有一个是空的"})
			continue
		}
		if !known[src] && !opt.Known[src] || !known[dst] && !opt.Known[dst] {
			// 端点既不在本批、也不在图里已有的实体里：丢掉并说明（宁可不建图，也不凭空造实体）。
			out.Dropped = append(out.Dropped, Drop{Kind: "relation", Name: src + "→" + dst,
				Reason: "端点既不在本批抽到、也不在图里已有（可能是模型编的）"})
			continue
		}
		ch, ok := locate(byDoc, r.Doc, r.Line)
		if !ok {
			out.Dropped = append(out.Dropped, Drop{Kind: "relation", Name: src + "→" + dst,
				Reason: fmt.Sprintf("doc=%q 不在这批块里", r.Doc)})
			continue
		}
		key := src + "\x00" + dst + "\x00" + strings.TrimSpace(r.Keywords)
		if seenRel[key] {
			continue
		}
		seenRel[key] = true
		out.Relations = append(out.Relations, Relation{
			Source: src, Target: dst, Keywords: strings.TrimSpace(r.Keywords),
			Description: strings.TrimSpace(r.Description),
			Provenance:  prov(ch, r.Line), ChunkOrd: ch.Ord,
		})
	}
}

// locate 找出这条条目属于哪一块：doc 要对得上；行号给了就要求落在某块区间内。
func locate(byDoc map[string][]Chunk, doc string, line int) (Chunk, bool) {
	list, ok := byDoc[strings.TrimSpace(doc)]
	if !ok || len(list) == 0 {
		return Chunk{}, false
	}
	if line > 0 {
		for _, c := range list {
			if line >= c.FromLine && line <= c.ToLine {
				return c, true
			}
		}
		return Chunk{}, false // 行号越界：这一条不可信，丢掉
	}
	return list[0], true
}

// prov 组装溯源：块区间来自我们；模型给的行号校验过才采信。
func prov(c Chunk, line int) Provenance {
	p := Provenance{Doc: c.Doc, FromLine: c.FromLine, ToLine: c.ToLine}
	switch {
	case line > 0 && line >= c.FromLine && line <= c.ToLine:
		p.Line = line
	case line > 0:
		p.Note = fmt.Sprintf("模型给的行号 %d 不在块区间 %d-%d 内，已退回区间", line, c.FromLine, c.ToLine)
	default:
		p.Note = "模型没给行号，只有块区间"
	}
	return p
}

func sortResult(out *Result) {
	sort.SliceStable(out.Entities, func(i, j int) bool {
		if out.Entities[i].Doc != out.Entities[j].Doc {
			return out.Entities[i].Doc < out.Entities[j].Doc
		}
		return out.Entities[i].Name < out.Entities[j].Name
	})
	sort.SliceStable(out.Relations, func(i, j int) bool {
		if out.Relations[i].Doc != out.Relations[j].Doc {
			return out.Relations[i].Doc < out.Relations[j].Doc
		}
		if out.Relations[i].Source != out.Relations[j].Source {
			return out.Relations[i].Source < out.Relations[j].Source
		}
		return out.Relations[i].Target < out.Relations[j].Target
	})
}

// rawExtraction 是模型回包的形状（JSON）。
type rawExtraction struct {
	Entities  []rawEntity   `json:"entities"`
	Relations []rawRelation `json:"relations"`
}

type rawEntity struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Doc         string `json:"doc"`
	Line        int    `json:"line"`
}

type rawRelation struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Keywords    string `json:"keywords"`
	Description string `json:"description"`
	Doc         string `json:"doc"`
	Line        int    `json:"line"`
}

// ParseResponse 从模型回包里挑出 JSON 并解析。
//
// 为什么不能直接 Unmarshal：回包可能夹着推理痕迹、```json 围栏、解释性文字（实测见过
// `dsh: reasoning:` 那种），**而且模型可能把答案拆成两个 JSON 对象连着吐出来**——
// 20 篇跑批第 34 批就是这么挂的：`invalid character '{' after top-level value`。
// 所以这里扫出**所有括号配平的 JSON 对象**，逐个解析后合并。
// 解析失败的报错要带**回包开头**，否则排查时只能干瞪眼。
func ParseResponse(text string) (rawExtraction, error) {
	var out rawExtraction
	s := strings.TrimSpace(text)
	if s == "" {
		return out, fmt.Errorf("回包是空的")
	}
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimPrefix(strings.TrimSpace(s), "json")
	}
	objs := balancedObjects(s)
	if len(objs) == 0 {
		return out, fmt.Errorf("回包里找不到 JSON 对象（开头 %q）", head(text, 120))
	}
	var firstErr error
	parsed := 0
	for _, o := range objs {
		var one rawExtraction
		if err := json.Unmarshal([]byte(o), &one); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out.Entities = append(out.Entities, one.Entities...)
		out.Relations = append(out.Relations, one.Relations...)
		parsed++
	}
	if parsed == 0 {
		return rawExtraction{}, fmt.Errorf("JSON 解析失败：%w（开头 %q）", firstErr, head(text, 120))
	}
	return out, nil
}

// balancedObjects 扫出所有「大括号配平」的 JSON 对象（照顾字符串里的括号与转义）。
func balancedObjects(s string) []string {
	var out []string
	depth, start := 0, -1
	inStr, esc := false, false
	for i, r := range s {
		switch {
		case esc:
			esc = false
		case r == '\\' && inStr:
			esc = true
		case r == '"':
			inStr = !inStr
		case inStr:
			// 字符串里的括号不算结构
		case r == '{':
			if depth == 0 {
				start = i
			}
			depth++
		case r == '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					out = append(out, s[start:i+1])
					start = -1
				}
			}
		}
	}
	return out
}
func head(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// fileLine 是给提示词用的行号区间说明。
func fileLine(c Chunk) string {
	if c.FromLine == c.ToLine {
		return fmt.Sprintf("%d", c.FromLine)
	}
	return fmt.Sprintf("%d-%d", c.FromLine, c.ToLine)
}
