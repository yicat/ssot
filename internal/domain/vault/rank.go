package vault

import (
	"strings"
	"unicode"
)

// 混合检索的**纯规则**：字面重合度怎么算、怎么跟向量分数融合。
//
// 为什么在 domain：这两件事换掉存储与模型也照样成立（「像不像」的判据是我们的），
// 而且必须**确定**——vault.spec.md 要求同一次查询结果顺序永远一样，
// 所以权重写死、不引入随机（见 ADR 0010）。
//
// 口径来源：docs/notes/embedding-spike.md §六.3 的实测（β 对实体名式查询单调有效，
// 对正文句式查询无害）。改这里的数字就等于改口径——先改 spec。

// HybridWeights 是字面部分的权重：分数 = 余弦 + TitleTags × 标题重合 + Body × 正文重合。
type HybridWeights struct {
	// TitleTags 是「标题 + 标签」的 2-gram 重合率权重。
	TitleTags float64
	// Body 是块正文的 2-gram 重合率权重。
	Body float64
}

// DefaultHybridWeights 是已定的权重。
//
// ⚠️ 这个数字是**我们自己量出来的**，不是照抄 spike：spike 当时报「β 取 0.1～0.2 单调有效」，
// 但 P2 用生产代码重测时，β=0.05 才是最好的（标题式查询 R@1 +3.0pp），β=0.2 反而掉 6.8pp
// （见 docs/notes/embedding-spike.md §六.7 与 docs/OPEN.md #21）。谁改了这里的数先看那两处。
func DefaultHybridWeights() HybridWeights {
	return HybridWeights{TitleTags: 0.05, Body: 0}
}

// Overlap2Gram 量查询与目标文本的字面重合度：查询里**去重后的 2-gram** 有多少落在目标里。
//
// 为什么用 2-gram：中文没有词边界，装一套分词器不值当；2-gram 对「实体名出现在标题里」
// 这件事足够敏感（这正是纯向量最弱、最需要补的一类查询）。
//
// 清理规则跟实测脚本一致：只留汉字、字母、数字，其余（标点、空格）一律去掉；
// 清理后不足 2 个字符返回 0（没有可比的 gram）。
func Overlap2Gram(query, target string) float64 {
	clean := keepAlnumHan(query)
	runes := []rune(clean)
	if len(runes) < 2 {
		return 0
	}
	seen := map[string]bool{}
	total := 0
	hit := 0
	for i := 0; i+2 <= len(runes); i++ {
		g := string(runes[i : i+2])
		if seen[g] {
			continue
		}
		seen[g] = true
		total++
		if strings.Contains(target, g) {
			hit++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// FuseHybrid 把向量分数与字面分数合成一个分数。
//
// 高的那个说了算：向量管「意思像不像」，字面管「名字对不对上」。
func FuseHybrid(cosine, titleOverlap, bodyOverlap float64, w HybridWeights) float64 {
	return cosine + w.TitleTags*titleOverlap + w.Body*bodyOverlap
}

// keepAlnumHan 只留汉字、字母、数字（与实测脚本的口径一致）。
func keepAlnumHan(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return string(out)
}
