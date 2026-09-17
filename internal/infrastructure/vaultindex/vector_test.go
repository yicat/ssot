package vaultindex

import (
	"testing"
)

// newVectorVault 造一个能切出多个块的小 vault（两块文档，状态一 draft 一 published）。
func newVectorVault(t *testing.T) *Index {
	t.Helper()
	root := t.TempDir()
	write(t, root, "docs/机制/伤害.md",
		"---\ntitle: 伤害\nstatus: published\n---\n\n"+bigDoc("伤害", 6))
	write(t, root, "docs/式神/茨木.md",
		"---\ntitle: 茨木\nstatus: draft\n---\n\n茨木童子靠鬼手打伤害。\n")
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestEmbeddingWriteSearchAndPending(t *testing.T) {
	idx := newVectorVault(t)

	st, err := idx.VectorStat(KindChunk)
	if err != nil {
		t.Fatal(err)
	}
	if st.Chunks == 0 || st.Embedded != 0 {
		t.Fatalf("刚重建时该有块但没向量：%+v", st)
	}

	pending, err := idx.PendingChunks(KindChunk, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != st.Chunks {
		t.Fatalf("待嵌入该是全部块：%d vs %d", len(pending), st.Chunks)
	}
	// 顺序必须是确定的（同输入同顺序）：按文档、序号。
	for i := 1; i < len(pending); i++ {
		a, b := pending[i-1], pending[i]
		if a.Doc > b.Doc || (a.Doc == b.Doc && a.Ord >= b.Ord) {
			t.Fatalf("待嵌入顺序不按 (doc, ord)：%+v 在 %+v 之前", a, b)
		}
	}

	// 只给第一块写一个 4 维向量（维度不必是 512：这一层不关心维度，只要一致）。
	first := pending[0]
	if err := idx.PutEmbeddings(KindChunk,
		[]string{ChunkKey(first.Doc, first.Ord)}, [][]float32{{1, 0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	left, err := idx.PendingCount(KindChunk)
	if err != nil {
		t.Fatal(err)
	}
	if left != st.Chunks-1 {
		t.Fatalf("写过一块之后待嵌入该少一个：%d，想要 %d", left, st.Chunks-1)
	}

	// 再写第二块，方向与第一块正交：查询该把第一块排在前面。
	second := pending[1]
	if err := idx.PutEmbeddings(KindChunk,
		[]string{ChunkKey(second.Doc, second.Ord)}, [][]float32{{0, 1, 0, 0}}); err != nil {
		t.Fatal(err)
	}

	hits, err := idx.SearchVector(KindChunk, []float32{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("该只命中有向量的两块：%+v", hits)
	}
	if hits[0].Doc != first.Doc || hits[0].Ord != first.Ord {
		t.Errorf("最像的那块该排第一：%+v", hits[0])
	}
	if hits[0].FromLine < 1 || hits[0].ToLine < hits[0].FromLine || hits[0].Text == "" {
		t.Errorf("命中要带文件行号区间与原文：%+v", hits[0])
	}
	if hits[0].Score <= hits[1].Score {
		t.Errorf("分数该降序：%+v", hits)
	}
	// 状态要带出来（未核验可见）。
	wantStatus := map[string]string{"docs/机制/伤害.md": "published", "docs/式神/茨木.md": "draft"}
	for _, h := range hits {
		if string(h.Status) != wantStatus[h.Doc] {
			t.Errorf("%s 的状态该是 %q，实际 %q", h.Doc, wantStatus[h.Doc], h.Status)
		}
	}

	// 维度对不上的向量要跳过，而不是算出没意义的分数。
	if err := idx.PutEmbeddings(KindChunk,
		[]string{ChunkKey("docs/式神/茨木.md", 0)}, [][]float32{{1, 0}}); err != nil {
		t.Fatal(err)
	}
	hits2, err := idx.SearchVector(KindChunk, []float32{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits2 {
		if h.Doc == "docs/式神/茨木.md" && h.Ord == 0 {
			t.Errorf("维度 2 的向量不该出现在 4 维查询的结果里：%+v", h)
		}
	}

	// 键与向量数量不一致必须报错（不许写半截）。
	if err := idx.PutEmbeddings(KindChunk, []string{"a", "b"}, [][]float32{{1, 0, 0, 0}}); err == nil {
		t.Error("键与向量数量不一致该报错")
	}
	if err := idx.PutEmbeddings("文档", []string{"a"}, [][]float32{{1}}); err == nil {
		t.Error("未知的层该报错")
	}
}

// TestRebuildDropsEmbeddings：向量是派生的，重建索引就是重算——
// 旧的向量必须一起没掉，否则会拿旧向量去配新块（那是静默的错）。
func TestRebuildDropsEmbeddings(t *testing.T) {
	idx := newVectorVault(t)
	pending, err := idx.PendingChunks(KindChunk, 1)
	if err != nil || len(pending) == 0 {
		t.Fatalf("该有块：%v %v", pending, err)
	}
	if err := idx.PutEmbeddings(KindChunk,
		[]string{ChunkKey(pending[0].Doc, pending[0].Ord)}, [][]float32{{1, 0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	st, err := idx.VectorStat(KindChunk)
	if err != nil {
		t.Fatal(err)
	}
	if st.Embedded != 0 {
		t.Errorf("重建后不该留着旧向量：%+v", st)
	}
}

// TestScanVectors 是给性能测量用的那条路：扫全表不报错、条数对得上。
func TestScanVectors(t *testing.T) {
	idx := newVectorVault(t)
	pending, err := idx.PendingChunks(KindChunk, 3)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(pending))
	vecs := make([][]float32, 0, len(pending))
	for _, c := range pending {
		keys = append(keys, ChunkKey(c.Doc, c.Ord))
		vecs = append(vecs, []float32{0.6, 0.8, 0, 0})
	}
	if err := idx.PutEmbeddings(KindChunk, keys, vecs); err != nil {
		t.Fatal(err)
	}
	n, err := idx.ScanVectors(KindChunk, []float32{0.6, 0.8, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(pending) {
		t.Errorf("扫过 %d 条，想要 %d 条", n, len(pending))
	}
}
