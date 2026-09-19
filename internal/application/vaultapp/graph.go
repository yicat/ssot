// 图检索（P4）：把「查询 → 实体 → 关系/块」这条链编排起来。
//
// 机制见 docs/specs/derived.spec.md「图检索（P4）」：
//
//	local  = 查询对上实体名（**字面匹配，不调模型**）→ 它出现的块；
//	global = 度数高的实体 → 它们出现的块；
//	hybrid = 两路合并；
//	mix    = 再叠一路向量。
//
// 「找实体不调模型」是有意的：字面匹配确定、免费、可解释，而且实体名式查询要的正是这个；
// LLM 抽关键词留给将来需要「高层主题」的场合（那时要批量，不然每次调用的固定开销太贵）。
package vaultapp

import (
	"fmt"
	"sort"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// GraphMode 是图检索的模式。
type GraphMode string

const (
	ModeLocal  GraphMode = "local"
	ModeGlobal GraphMode = "global"
	ModeHybrid GraphMode = "hybrid"
	ModeMix    GraphMode = "mix"
)

// GraphResult 是一次图检索的结果。
type GraphResult struct {
	Mode     GraphMode
	Hits     []vault.VectorHit
	Entities []string // 这次查询对上了哪些实体（可解释：为什么是这些结果）
	Note     string   // 降级说明（比如 mix 模式没配模型）
}

// ParseGraphMode 解析模式名（空 = hybrid）。
func ParseGraphMode(s string) (GraphMode, error) {
	switch GraphMode(s) {
	case "":
		return ModeHybrid, nil
	case ModeLocal, ModeGlobal, ModeHybrid, ModeMix:
		return GraphMode(s), nil
	default:
		return "", fmt.Errorf("未知的检索模式 %q（只有 local / global / hybrid / mix）", s)
	}
}

// GraphSearch 按模式做一次图检索。
func (s *Service) GraphSearch(query string, mode GraphMode, limit int) (GraphResult, error) {
	if query == "" {
		return GraphResult{}, fmt.Errorf("查询词不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	if err := s.ensureIndex(); err != nil {
		return GraphResult{}, err
	}
	w := vault.DefaultGraphWeights()
	res := GraphResult{Mode: mode}

	// 状态：结果要带文档状态（未核验可见）。
	items, err := s.List()
	if err != nil {
		return res, err
	}
	status := map[string]vault.Status{}
	for _, it := range items {
		status[it.Path] = it.Status
	}

	var cands []vault.GraphCandidate
	addChunks := func(name string, weight, perEntity float64, why string) error {
		chunks, err := s.index.ChunksForEntity(name, 12)
		if err != nil {
			return err
		}
		for _, c := range chunks {
			cands = append(cands, vault.GraphCandidate{
				Doc: c.Doc, FromLine: c.FromLine, ToLine: c.ToLine,
				Score: weight * perEntity, Why: why + "：" + name,
			})
		}
		return nil
	}

	if mode == ModeLocal || mode == ModeHybrid || mode == ModeMix {
		names, err := s.index.EntityNames()
		if err != nil {
			return res, err
		}
		list := make([]string, 0, len(names))
		for n := range names {
			list = append(list, n)
		}
		sort.Strings(list) // 确定：同样的库，同样的候选顺序
		matches := vault.MatchEntityNames(query, list, 0.5)
		if len(matches) > 8 {
			matches = matches[:8]
		}
		for _, m := range matches {
			res.Entities = append(res.Entities, m.Name)
			if err := addChunks(m.Name, w.Local, m.Score, "查询对上了实体"); err != nil {
				return res, err
			}
		}
	}

	if mode == ModeGlobal || mode == ModeHybrid || mode == ModeMix {
		top, err := s.index.TopEntities(20)
		if err != nil {
			return res, err
		}
		for i, e := range top {
			// 度数越高排越前：用名次的倒数当权重（确定、可解释）。
			per := 1.0 / float64(1+i)
			if err := addChunks(e.Name, w.Global, per, "度数高的实体"); err != nil {
				return res, err
			}
		}
	}

	if mode == ModeMix {
		sr, err := s.NewSearcher(vault.DefaultHybridWeights())
		if err != nil {
			// 降级要说出来（不许静默给一半结果）。
			res.Note = "mix 的向量那一路没跑起来（" + err.Error() + "），这次只用了图"
		} else {
			defer sr.Close()
			hits, err := sr.Search(query, 60)
			if err != nil {
				res.Note = "mix 的向量那一路失败（" + err.Error() + "），这次只用了图"
			}
			for _, h := range hits {
				cands = append(cands, vault.GraphCandidate{
					Doc: h.Doc, FromLine: h.FromLine, ToLine: h.ToLine,
					Score: w.Vector * float64(h.Score), Why: "向量",
				})
			}
		}
	}

	// 同一篇文档里、行号区间重叠的块合并（图那一路同一个实体会带来若干块）。
	fused := vault.FuseCandidates(cands, limit)
	for _, c := range fused {
		res.Hits = append(res.Hits, vault.VectorHit{
			Doc: c.Doc, FromLine: c.FromLine, ToLine: c.ToLine,
			Status: status[c.Doc], Score: float32(c.Score),
		})
	}
	return res, nil
}
