// 混合检索：**向量 + 字面（标题/标签/正文）**，分数写死、排序稳定。
//
// 为什么必须混合：实测纯向量在**实体名式短查询**上 R@1 只有 21.6%（see docs/notes/embedding-spike.md §六.3），
// 而加一个「标题+标签」的 2-gram 字面项就能把它抬到 26.5%（β=0.1）／R@5 54.9% → 62.7%（β=0.2），
// 对正文句式查询没有任何损失。这是 P2 的分水岭。
//
// 为什么要有 Searcher（而不是每次现查）：实测「每查一次从 SQLite 读 25MB 向量」要 222～241 ms，
// 而内存里点积只要 2 ms（OPEN.md #19）。界面与 MCP 都是常驻进程，把块元数据、正文、
// 向量一次性读进内存（demo 约 32 MB）是**硬要求**，不是优化项。
package vaultindex

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// VectorCache 是内存里的向量表：键按 (doc, ord) 排序，向量平铺成一段 float32。
type VectorCache struct {
	kind string
	dim  int
	keys []string
	vecs []float32
}

// LoadVectorCache 把某一层的向量读进内存。
func (idx *Index) LoadVectorCache(kind string) (*VectorCache, error) {
	if _, err := chunkTable(kind); err != nil {
		return nil, err
	}
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT owner_id, dim, vec FROM embedding WHERE owner_kind = ? ORDER BY owner_id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := &VectorCache{kind: kind}
	for rows.Next() {
		var key string
		var dim int
		var blob []byte
		if err := rows.Scan(&key, &dim, &blob); err != nil {
			return nil, err
		}
		if c.dim == 0 {
			c.dim = dim
		}
		if dim != c.dim {
			continue // 换过模型留下的旧向量：跳过（维度混着算没意义）
		}
		if len(blob) != dim*4 {
			continue
		}
		c.keys = append(c.keys, key)
		c.vecs = append(c.vecs, decodeVector(blob, dim)...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// Len 是内存里的向量条数。
func (c *VectorCache) Len() int { return len(c.keys) }

// Dim 是向量维度（空表时为 0）。
func (c *VectorCache) Dim() int { return c.dim }

// Bytes 是向量占的内存字节数。
func (c *VectorCache) Bytes() int { return len(c.vecs) * 4 }

// dot 是第 i 条向量与查询的点积（向量已归一化 → 就是余弦）。
func (c *VectorCache) dot(i int, q []float32) float64 {
	base := i * c.dim
	var s float64
	for j := 0; j < c.dim; j++ {
		s += float64(c.vecs[base+j]) * float64(q[j])
	}
	return s
}

// Searcher 是「读一次、查很多次」的检索器。
//
// 内存里放三样：块的元数据与正文（结果与字面项要用）、每个文档的「标题+标签」（字面项要用）、
// 每个块的向量（打分要用）。
type Searcher struct {
	idx     *Index
	kind    string
	weights vault.HybridWeights

	keys []string // 与向量同序（只有嵌过的块）
	// vecIdx[i] 是 keys[i] 的向量在 cache 里的下标。**必须显式存**：一旦有块因为
	// 缺元数据被过滤掉，拿 i 直接当 cache 下标就会串位——那是静默的错答案。
	vecIdx []int
	vecs   *VectorCache
	meta   map[string]chunkMeta // doc#ord → 元数据
	titles map[string]string    // doc → 「标题 + 标签」
}

type chunkMeta struct {
	doc      string
	ord      int
	fromLine int
	toLine   int
	text     string
	status   vault.Status
}

// NewSearcher 把索引读进内存并返回检索器。
//
// 顺序按 `doc, ord` 固定，所以同样的查询、同样的数据，结果顺序**永远一样**。
func (idx *Index) NewSearcher(kind string, w vault.HybridWeights) (*Searcher, error) {
	table, err := chunkTable(kind)
	if err != nil {
		return nil, err
	}
	cache, err := idx.LoadVectorCache(kind)
	if err != nil {
		return nil, err
	}
	s := &Searcher{
		idx:     idx,
		kind:    kind,
		weights: w,
		vecs:    cache,
		meta:    make(map[string]chunkMeta, len(cache.keys)),
		titles:  map[string]string{},
	}
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// 块元数据 + 正文（只要嵌过的块：没向量的块在融合里拿不到余弦分，参与不了排序）。
	rows, err := db.Query(fmt.Sprintf(
		`SELECT c.doc, c.ord, c.from_line, c.to_line, c.text, COALESCE(d.status,'')
		   FROM %s c JOIN embedding e ON e.owner_kind = ? AND e.owner_id = c.doc || '#' || c.ord
		   LEFT JOIN docs d ON d.path = c.doc`, table), kind)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m chunkMeta
		var status string
		if err := rows.Scan(&m.doc, &m.ord, &m.fromLine, &m.toLine, &m.text, &status); err != nil {
			rows.Close()
			return nil, err
		}
		m.status = vault.Status(status)
		s.meta[ChunkKey(m.doc, m.ord)] = m
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 每篇文档的「标题 + 标签」（就是索引里那两样，不再去读文件）。
	trows, err := db.Query(`SELECT path, COALESCE(title,''), COALESCE(tags,'') FROM docs`)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var path, title, tags string
		if err := trows.Scan(&path, &title, &tags); err != nil {
			return nil, err
		}
		s.titles[path] = title + " " + strings.ReplaceAll(tags, ",", " ")
	}
	if err := trows.Err(); err != nil {
		return nil, err
	}

	for i, k := range cache.keys {
		if _, ok := s.meta[k]; ok {
			s.keys = append(s.keys, k)
			s.vecIdx = append(s.vecIdx, i)
		}
	}
	return s, nil
}

// Len 是参与检索的块数。
func (s *Searcher) Len() int { return len(s.keys) }

// MemoryBytes 是常驻内存的粗略估算（向量 + 正文），给「要不要缓存」这个问题一个数字。
func (s *Searcher) MemoryBytes() int {
	n := s.vecs.Bytes()
	for _, m := range s.meta {
		n += len(m.text) + len(m.doc) + 64
	}
	return n
}

// Search 做一次混合检索：向量余弦 + 字面重合度，取前 limit 条。
func (s *Searcher) Search(query string, qvec []float32, limit int) ([]vault.VectorHit, error) {
	if len(qvec) == 0 {
		return nil, fmt.Errorf("查询向量是空的")
	}
	if s.vecs.Dim() != 0 && len(qvec) != s.vecs.Dim() {
		return nil, fmt.Errorf("查询向量是 %d 维，索引里的向量是 %d 维（换过模型？重建索引）",
			len(qvec), s.vecs.Dim())
	}
	if limit <= 0 {
		limit = 10
	}

	// 字面项：每篇文档算一次（同一篇的块共用），正文项按需算。
	titleOverlap := make(map[string]float64, len(s.titles))
	overlapOf := func(doc string) float64 {
		if v, ok := titleOverlap[doc]; ok {
			return v
		}
		v := vault.Overlap2Gram(query, s.titles[doc])
		titleOverlap[doc] = v
		return v
	}

	type scored struct {
		key   string
		score float64
	}
	top := make([]scored, 0, len(s.keys))
	for i, key := range s.keys {
		m := s.meta[key]
		cos := s.vecs.dot(s.vecIdx[i], qvec)
		body := 0.0
		if s.weights.Body != 0 {
			body = vault.Overlap2Gram(query, m.text)
		}
		top = append(top, scored{key: key, score: vault.FuseHybrid(cos, overlapOf(m.doc), body, s.weights)})
	}
	// 确定性排序：分数降序，同分按 key 升序（同查询同顺序，ADR 0010）。
	sort.Slice(top, func(i, j int) bool {
		if top[i].score != top[j].score {
			return top[i].score > top[j].score
		}
		return top[i].key < top[j].key
	})
	if len(top) > limit {
		top = top[:limit]
	}

	out := make([]vault.VectorHit, 0, len(top))
	for _, t := range top {
		m := s.meta[t.key]
		out = append(out, vault.VectorHit{
			Doc:      m.doc,
			FromLine: m.fromLine,
			ToLine:   m.toLine,
			Ord:      m.ord,
			Text:     m.text,
			Status:   m.status,
			Score:    float32(t.score),
		})
	}
	return out, nil
}

// AllChunks 返回某一层的全部块（按 doc, ord）——给对拍/基准脚本重建「检索池」用。
//
// 产品代码走 Searcher；这个口子存在的理由是：评测必须能复现「池子里只有这 N 篇」这种设定。
func (idx *Index) AllChunks(kind string) ([]ChunkRef, error) {
	table, err := chunkTable(kind)
	if err != nil {
		return nil, err
	}
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(fmt.Sprintf(
		`SELECT doc, ord, text, from_line, to_line FROM %s ORDER BY doc, ord`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChunkRef{}
	for rows.Next() {
		var c ChunkRef
		if err := rows.Scan(&c.Doc, &c.Ord, &c.Text, &c.FromLine, &c.ToLine); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
