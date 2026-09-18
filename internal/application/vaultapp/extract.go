// 抽取的用例编排：取块 → 分批 → 调模型 → 拿回带块级溯源的产物。
//
// 现在这一版**只取块与分批**，把「调用」留给调用方（CLI 先验，入库下一步做）：
//   - 入库要等 entity/relation 表与合并规则落地（plan §3、spec §九.2）；
//   - 触发方式还没拍板（OPEN.md #2），所以这里不排队、不后台，只按显式参数取一批。
package vaultapp

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultindex"
	"github.com/ngnl5/ssot/internal/infrastructure/vextract"
)

// StoreExtraction 把一次抽取的产物写进派生索引（**幂等**：同一来源重复写不会变多）。
//
// 来源行原样写：entity 按 (name,type,doc,from_line,to_line,line)、relation 按
// (src,dst,keywords,doc,from_line,to_line,line)——**归并只合条目，不丢出处**（plan §3）。
// 状态不复制：读的时候 join `docs` 现拿，所以「未核验可见」永远准。
func (s *Service) StoreExtraction(res vextract.Result) (vault.ExtractStat, error) {
	if err := s.ensureIndex(); err != nil {
		return vault.ExtractStat{}, err
	}
	ents := make([]vaultindex.EntityRow, 0, len(res.Entities))
	for _, e := range res.Entities {
		ents = append(ents, vaultindex.EntityRow{
			Name: e.Name, Type: e.Type, Description: e.Description, Authority: vaultindex.AuthorityDerived,
			Doc: e.Doc, FromLine: e.FromLine, ToLine: e.ToLine, Line: e.Line,
		})
	}
	rels := make([]vaultindex.RelationRow, 0, len(res.Relations))
	for _, r := range res.Relations {
		rels = append(rels, vaultindex.RelationRow{
			Src: r.Source, Dst: r.Target, Keywords: r.Keywords, Description: r.Description,
			Authority: vaultindex.AuthorityDerived,
			Doc:       r.Doc, FromLine: r.FromLine, ToLine: r.ToLine, Line: r.Line,
		})
	}
	if _, err := s.index.PutEntities(ents); err != nil {
		return vault.ExtractStat{}, err
	}
	if _, err := s.index.PutRelations(rels); err != nil {
		return vault.ExtractStat{}, err
	}
	return s.index.ExtractStat()
}

// ExtractStat 读派生图的家底（给状态显示与验证）。
func (s *Service) ExtractStat() (vault.ExtractStat, error) {
	if err := s.ensureIndex(); err != nil {
		return vault.ExtractStat{}, err
	}
	return s.index.ExtractStat()
}

// EntitiesMerged 按 (name,type) 归并读实体（来源清单在 Doc 字段里）。
func (s *Service) EntitiesMerged(limit int) ([]vaultindex.EntityRow, error) {
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	return s.index.EntitiesMerged(limit)
}

// ExtractBatch 是默认每批多少块（**多块合一次调用**，spec §三.1）。
//
// 为什么是 8：ACP 这条路上没有命令行长度限制，所以上限由**一次调用的输出**决定
// （一批太大，模型漏项与 JSON 截断的风险都上升）。8 块 ≈ 16k token 上下文，
// 先拿这个数跑批，用实测的漏项/坏行号再调——别把它当定稿。
const ExtractBatch = 8

// ExtractChunksForDocs 取前 docs 篇（按路径排序）的**抽取块**（2000 口径），并按 batch 分批。
//
// batch<=0 用 ExtractBatch；docs<=0 表示全部。
func (s *Service) ExtractChunksForDocs(docs, batch int) ([][]vextract.Chunk, error) {
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	all, err := s.index.AllChunks(vaultindex.KindExtract)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("索引里没有抽取块：先跑 `ssot vault index`")
	}

	// 按顺序取前 docs 篇（AllChunks 已按 doc, ord 排序）。
	limitDocs := docs
	if limitDocs <= 0 {
		limitDocs = 1 << 30
	}
	seen := map[string]bool{}
	var picked []vextract.Chunk
	for _, c := range all {
		if !seen[c.Doc] {
			if len(seen) >= limitDocs {
				continue
			}
			seen[c.Doc] = true
		}
		picked = append(picked, vextract.Chunk{
			Doc: c.Doc, Ord: c.Ord, FromLine: c.FromLine, ToLine: c.ToLine, Text: c.Text,
		})
	}
	if len(picked) == 0 {
		return nil, fmt.Errorf("没取到块")
	}

	if batch <= 0 {
		batch = ExtractBatch
	}
	var out [][]vextract.Chunk
	for i := 0; i < len(picked); i += batch {
		end := i + batch
		if end > len(picked) {
			end = len(picked)
		}
		out = append(out, picked[i:end])
	}
	return out, nil
}
