// 向量相关的用例：把块的向量算出来存进派生索引、用向量检索。
//
// 这一层只做编排（先取待嵌入的块、分批、写回、报进度），规则在 domain，
// 模型与 ONNX 运行时在 infrastructure/vembed，索引在 infrastructure/vaultindex。
package vaultapp

import (
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultindex"
	"github.com/ngnl5/ssot/internal/infrastructure/vembed"
)

// EmbedBatch 是一次写入的块数：太小则事务多，太大则失败时要重算的更多。
const EmbedBatch = 64

// EmbedResult 是一次嵌入的结果。
type EmbedResult struct {
	Embedded int     // 这次嵌了多少块
	Pending  int     // 还剩多少块没嵌
	Dim      int     // 向量维度
	Seconds  float64 // 花了多久（嵌进索引的实测，不含模型加载）
}

// Searcher 是「打开一次、查很多次」的检索器：模型与向量都只加载一次。
//
// 界面与 MCP 都是常驻进程，所以**必须**用它——每次现开模型、每次从库里读 25MB 向量
// 是 222～241 ms（实测），用它是 2 ms 量级（见 docs/OPEN.md #19）。
type Searcher struct {
	eng *vembed.Engine
	s   *vaultindex.Searcher
	st  time.Duration // 打开（加载模型 + 读向量）用了多久，给状态显示用
}

// NewSearcher 加载模型与向量，返回检索器。weights 是字面项权重（零值 = 纯向量）。
func (s *Service) NewSearcher(weights vault.HybridWeights) (*Searcher, error) {
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	start := time.Now()
	eng, err := s.openEmbedder()
	if err != nil {
		return nil, err
	}
	is, err := s.index.NewSearcher(vaultindex.KindChunk, weights)
	if err != nil {
		eng.Close()
		return nil, err
	}
	return &Searcher{eng: eng, s: is, st: time.Since(start)}, nil
}

// Close 释放模型会话。
func (sr *Searcher) Close() {
	if sr.eng != nil {
		sr.eng.Close()
	}
}

// Chunks 是参与检索的块数。
func (sr *Searcher) Chunks() int { return sr.s.Len() }

// LoadDuration 是打开检索器的耗时（模型加载 + 读向量）。
func (sr *Searcher) LoadDuration() time.Duration { return sr.st }

// MemoryBytes 是常驻内存的粗略估算。
func (sr *Searcher) MemoryBytes() int { return sr.s.MemoryBytes() }

// Search 做一次混合检索。
func (sr *Searcher) Search(q string, limit int) ([]vault.VectorHit, error) {
	if q == "" {
		return nil, fmt.Errorf("查询词不能为空")
	}
	vec, err := sr.eng.Embed(q)
	if err != nil {
		return nil, err
	}
	return sr.s.Search(q, vec, limit)
}

// SearchQuery 只把查询词编码一次、返回向量（基准脚本要对同一个查询跑多套权重时用）。
func (sr *Searcher) Embed(q string) ([]float32, error) { return sr.eng.Embed(q) }

// SearchWithVector 用给定向量检索（避免同一查询重复编码）。
func (sr *Searcher) SearchWithVector(q string, vec []float32, limit int) ([]vault.VectorHit, error) {
	return sr.s.Search(q, vec, limit)
}

// openEmbedder 打开嵌入引擎，错误里给**人话**（缺模型是常见情况，别只说 file not found）。
func (s *Service) openEmbedder() (*vembed.Engine, error) {
	if s.embedDir == "" {
		return nil, fmt.Errorf("还没配嵌入模型：设 SSOT_EMBED_DIR 指向模型目录" +
			"（目录里要有 model.onnx、tokenizer.json、onnxruntime.dll）——模型怎么分发还没定，见 docs/OPEN.md #15")
	}
	return vembed.Open(vembed.Options{ModelDir: s.embedDir})
}

// EmbedPending 把「还没嵌入的块」嵌入并写进索引；limit<=0 表示不限量。
//
// 分两段：先把待嵌入的块查出来（`chunk` 表），一批一批算、一批一批写。
// 写完的块在 embedding 表里有行，所以**中断了也能接着跑**——不用从头再来。
func (s *Service) EmbedPending(limit int, progress func(done, total int)) (EmbedResult, error) {
	res := EmbedResult{}
	if err := s.ensureIndex(); err != nil {
		return res, err
	}
	total, err := s.index.PendingCount(vaultindex.KindChunk)
	if err != nil {
		return res, err
	}
	if total == 0 {
		st, err := s.index.VectorStat(vaultindex.KindChunk)
		if err != nil {
			return res, err
		}
		res.Pending, res.Dim = 0, st.Dim
		return res, nil
	}
	if limit > 0 && limit < total {
		total = limit
	}

	eng, err := s.openEmbedder()
	if err != nil {
		return res, err
	}
	defer eng.Close()

	start := time.Now()
	done := 0
	for done < total {
		n := EmbedBatch
		if rest := total - done; rest < n {
			n = rest
		}
		batch, err := s.index.PendingChunks(vaultindex.KindChunk, n)
		if err != nil {
			return res, err
		}
		if len(batch) == 0 {
			break // 待嵌的都被别人嵌完了
		}
		keys := make([]string, 0, len(batch))
		vecs := make([][]float32, 0, len(batch))
		for _, c := range batch {
			vec, err := eng.Embed(c.Text)
			if err != nil {
				return res, fmt.Errorf("嵌入 %s 第 %d 块失败：%w", c.Doc, c.Ord, err)
			}
			keys = append(keys, vaultindex.ChunkKey(c.Doc, c.Ord))
			vecs = append(vecs, vec)
		}
		if err := s.index.PutEmbeddings(vaultindex.KindChunk, keys, vecs); err != nil {
			return res, err
		}
		done += len(batch)
		res.Dim = len(vecs[0])
		if progress != nil {
			progress(done, total)
		}
	}

	res.Embedded = done
	res.Seconds = time.Since(start).Seconds()
	left, err := s.index.PendingCount(vaultindex.KindChunk)
	if err != nil {
		return res, err
	}
	res.Pending = left
	return res, nil
}

// VectorSearch 用向量检索：查询词过同一个模型，然后暴力扫块向量。
//
// 返回带**文件行号区间**与文档状态的命中（块级溯源 + 未核验可见）。
func (s *Service) VectorSearch(q string, limit int) ([]vault.VectorHit, error) {
	if q == "" {
		return nil, fmt.Errorf("查询词不能为空")
	}
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	eng, err := s.openEmbedder()
	if err != nil {
		return nil, err
	}
	defer eng.Close()
	vec, err := eng.Embed(q)
	if err != nil {
		return nil, err
	}
	return s.index.SearchVector(vaultindex.KindChunk, vec, limit)
}

// VectorStat 是嵌入进度（给界面与状态显示用）。
type VectorStat struct {
	Chunks   int // 总块数
	Embedded int // 已嵌入
	Dim      int
	ModelDir string
}

// VectorStat 读嵌入进度。
func (s *Service) VectorStat() (VectorStat, error) {
	if err := s.ensureIndex(); err != nil {
		return VectorStat{}, err
	}
	st, err := s.index.VectorStat(vaultindex.KindChunk)
	if err != nil {
		return VectorStat{}, err
	}
	return VectorStat{Chunks: st.Chunks, Embedded: st.Embedded, Dim: st.Dim, ModelDir: s.embedDir}, nil
}

// ScanVectors 把全表扫一遍并返回条数（给性能测量用，调用方自己掐表）。
func (s *Service) ScanVectors(q string) (int, time.Duration, error) {
	if err := s.ensureIndex(); err != nil {
		return 0, 0, err
	}
	eng, err := s.openEmbedder()
	if err != nil {
		return 0, 0, err
	}
	defer eng.Close()
	vec, err := eng.Embed(q)
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	n, err := s.index.ScanVectors(vaultindex.KindChunk, vec)
	return n, time.Since(start), err
}
