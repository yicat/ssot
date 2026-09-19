package vault

import (
	"sort"
	"strings"
)

// 图检索的**纯规则**：查询怎么对上实体、多路候选怎么合并。
//
// 为什么在 domain：换掉存储与模型，这两条判断照样成立；而且必须**确定**
// （同查询同顺序，ADR 0010）。权重写死在这里——改它就是改口径，先改 spec。

// GraphWeights 是各路的权重（写死的默认值）。
type GraphWeights struct {
	// Local：命中的实体 → 它出现的块。
	Local float64
	// Global：度数高的实体 → 它出现的块。
	Global float64
	// Vector：与 P2 的向量分数相加（只有 mix 模式用）。
	Vector float64
}

// DefaultGraphWeights 是已定的权重。
//
// 取值理由：**图是补字面/向量够不着的东西**（实体名式查询），所以 local 给得最重；
// global 是「从主题往下罩」，比 local 虚，给一半；向量只在 mix 里作为一路加进来。
func DefaultGraphWeights() GraphWeights {
	return GraphWeights{Local: 1.0, Global: 0.5, Vector: 1.0}
}

// EntityMatch 是一个「查询对上了某个实体名」的结果。
type EntityMatch struct {
	Name  string
	Score float64 // 0..1
	Why   string  // 人话理由（界面上要能说清「为什么想到它」）
}

// MatchEntityNames 用**字面**把查询对上实体名，不调模型。
//
// 规则（确定、可解释）：
//  1. 实体名整串出现在查询里 → 1.0（「茨木童子的鬼手技能」对上「茨木童子」）；
//  2. 否则看 2-gram 重合率（`Overlap2Gram`），取 ≥ minScore 的；
//  3. 太短的名字（<2 字）不参与，免得误伤。
//
// 返回按分数降序、同分按名字升序（稳定）。
func MatchEntityNames(query string, names []string, minScore float64) []EntityMatch {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	if minScore <= 0 {
		minScore = 0.5
	}
	var out []EntityMatch
	for _, n := range names {
		name := strings.TrimSpace(n)
		if len([]rune(name)) < 2 {
			continue
		}
		if strings.Contains(q, name) {
			out = append(out, EntityMatch{Name: name, Score: 1, Why: "查询里直接出现了这个名字"})
			continue
		}
		// 反方向：实体名里包含查询（短查询问一个长名字）。
		if len([]rune(q)) >= 2 && strings.Contains(name, q) {
			out = append(out, EntityMatch{Name: name, Score: 1, Why: "这个名字里包含查询"})
			continue
		}
		if s := Overlap2Gram(q, name); s >= minScore {
			out = append(out, EntityMatch{Name: name, Score: s, Why: "字面重合"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// GraphCandidate 是一路候选：某篇文档里的某个块，以及它为什么被选上。
type GraphCandidate struct {
	Doc      string
	FromLine int
	ToLine   int
	Score    float64
	Why      string
}

// FuseCandidates 把多路候选合并成一份：同一个块（doc + 起始行）的分数**相加**，
// 理由拼接；最后按分数降序、同分按 (doc, 起始行) 升序——顺序永远一样。
func FuseCandidates(cands []GraphCandidate, limit int) []GraphCandidate {
	merged := map[string]*GraphCandidate{}
	order := []string{}
	for _, c := range cands {
		key := c.Doc + "\x00" + itoa(c.FromLine)
		if have, ok := merged[key]; ok {
			have.Score += c.Score
			if c.Why != "" && !strings.Contains(have.Why, c.Why) {
				have.Why += "；" + c.Why
			}
			if c.ToLine > have.ToLine {
				have.ToLine = c.ToLine
			}
			continue
		}
		cc := c
		merged[key] = &cc
		order = append(order, key)
	}
	out := make([]GraphCandidate, 0, len(order))
	for _, k := range order {
		out = append(out, *merged[k])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Doc != out[j].Doc {
			return out[i].Doc < out[j].Doc
		}
		return out[i].FromLine < out[j].FromLine
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// itoa 是个小工具（避免为一个拼接引入 strconv 到调用方）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
