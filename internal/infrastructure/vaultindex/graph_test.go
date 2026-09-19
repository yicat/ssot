package vaultindex

import "testing"

// TestGraphWriteReadMerge 是 P3 入库的验收：来源各占一行、归并只合条目、幂等、纠错优先。
func TestGraphWriteReadMerge(t *testing.T) {
	idx := newVectorVault(t) // 两篇文档：伤害.md（published）、茨木.md（draft）

	ents := []EntityRow{
		{Name: "茨木童子", Type: "式神", Description: "靠鬼手输出", Doc: "docs/式神/茨木.md", FromLine: 10, ToLine: 20, Line: 12},
		{Name: "茨木童子", Type: "式神", Description: "另一种说法", Doc: "docs/机制/伤害.md", FromLine: 1, ToLine: 5, Line: 3},
		{Name: "鬼手", Type: "技能", Description: "鬼手", Doc: "docs/式神/茨木.md", FromLine: 10, ToLine: 20, Line: 15},
	}
	n, err := idx.PutEntities(ents)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("该写进 3 行，实际 %d", n)
	}

	st, err := idx.ExtractStat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entities != 3 || st.EntityNames != 2 || st.Docs != 2 {
		t.Fatalf("家底不对（3 行来源 / 2 个实体 / 2 篇来源）：%+v", st)
	}

	// 幂等：同一来源重复写不会变多（跑两遍抽取不该把图越写越肿）。
	if _, err := idx.PutEntities(ents); err != nil {
		t.Fatal(err)
	}
	if st2, _ := idx.ExtractStat(); st2.Entities != 3 {
		t.Errorf("重复写该幂等，实际 %d 行", st2.Entities)
	}

	// 纠错优先：同一条来源标成 corrected 之后，再写 derived 不该把它改回去。
	if _, err := idx.PutEntities([]EntityRow{{
		Name: "茨木童子", Type: "式神", Description: "人改过的说法", Authority: AuthorityCorrected,
		Doc: "docs/式神/茨木.md", FromLine: 10, ToLine: 20, Line: 12,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.PutEntities([]EntityRow{{
		Name: "茨木童子", Type: "式神", Description: "模型又抽了一遍",
		Doc: "docs/式神/茨木.md", FromLine: 10, ToLine: 20, Line: 12,
	}}); err != nil {
		t.Fatal(err)
	}
	rows, err := idx.EntitiesFor("docs/式神/茨木.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("这篇该有 2 条来源行：%+v", rows)
	}
	for _, r := range rows {
		if r.Authority != AuthorityCorrected && r.Name == "茨木童子" {
			t.Errorf("纠错过的条目不该被 derived 覆盖：%+v", r)
		}
	}

	// 状态是从 docs 现读的（派生表不复制会变的东西）——draft 要带出来。
	found := false
	for _, r := range rows {
		if r.Doc == "docs/式神/茨木.md" {
			found = true
			if r.Status != "draft" {
				t.Errorf("状态该是 draft（未核验可见）：%+v", r)
			}
		}
	}
	if !found {
		t.Fatal("没读到那篇的来源行")
	}

	// 归并读法：同一实体只出一条，来源清单列全。
	merged, err := idx.EntitiesMerged(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 {
		t.Fatalf("归并后该是 2 个实体：%+v", merged)
	}
	var ci EntityRow
	for _, m := range merged {
		if m.Name == "茨木童子" {
			ci = m
		}
	}
	if ci.Authority != AuthorityCorrected {
		t.Errorf("归并后该体现「有人纠错过」：%+v", ci)
	}
	for _, want := range []string{"docs/式神/茨木.md:12", "docs/机制/伤害.md:3"} {
		if !contains(ci.Doc, want) {
			t.Errorf("来源清单里该有 %q，实际 %q", want, ci.Doc)
		}
	}
}

func TestGraphRelationsAndIdempotency(t *testing.T) {
	idx := newVectorVault(t)
	rels := []RelationRow{
		{Src: "茨木童子", Dst: "鬼手", Keywords: "技能/输出", Description: "拥有", Doc: "docs/式神/茨木.md", FromLine: 10, ToLine: 20, Line: 15},
		{Src: "茨木童子", Dst: "鬼手", Keywords: "技能/输出", Description: "换个说法", Doc: "docs/机制/伤害.md", FromLine: 1, ToLine: 5, Line: 2},
	}
	n, err := idx.PutRelations(rels)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("该写进 2 行，实际 %d", n)
	}
	st, err := idx.ExtractStat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Relations != 2 || st.RelationKeys != 1 {
		t.Fatalf("关系家底不对（2 行来源 / 1 条关系）：%+v", st)
	}
	if _, err := idx.PutRelations(rels); err != nil {
		t.Fatal(err)
	}
	if st2, _ := idx.ExtractStat(); st2.Relations != 2 {
		t.Errorf("重复写该幂等，实际 %d 行", st2.Relations)
	}

	got, err := idx.RelationsFor("docs/式神/茨木.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Src != "茨木童子" || got[0].Dst != "鬼手" {
		t.Fatalf("读回来的关系不对：%+v", got)
	}
	if got[0].Status != "draft" {
		t.Errorf("关系也要带来源文档的状态：%+v", got[0])
	}

	docs, err := idx.ExtractedDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Errorf("有产物的文档该是 2 篇：%v", docs)
	}
}

func TestRebuildDropsGraph(t *testing.T) {
	idx := newVectorVault(t)
	if _, err := idx.PutEntities([]EntityRow{{
		Name: "茨木童子", Type: "式神", Doc: "docs/式神/茨木.md", FromLine: 1, ToLine: 2, Line: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	st, err := idx.ExtractStat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entities != 0 {
		t.Errorf("重建索引会把抽取产物一起丢掉（它是派生的，要重抽）：%+v", st)
	}
}

// 归并读法里**描述要纠错优先**：只把 authority 标成 corrected 而描述还是模型那份，等于没生效。
func TestEntitiesMergedPrefersCorrectedDescription(t *testing.T) {
	idx := newVectorVault(t)
	// 两条来源：一条模型抽的、一条人纠正的（来源不同，所以都会留着）。
	if _, err := idx.PutEntities([]EntityRow{
		{Name: "茨木童子", Type: "式神", Description: "模型写的：靠鬼手输出",
			Authority: AuthorityDerived, Doc: "docs/机制/伤害.md", FromLine: 1, ToLine: 5, Line: 3},
		{Name: "茨木童子", Type: "式神", Description: "人改过的：鬼手是右手",
			Authority: AuthorityCorrected, Doc: "docs/式神/茨木.md", FromLine: 8, ToLine: 10, Line: 8},
	}); err != nil {
		t.Fatal(err)
	}
	merged, err := idx.EntitiesMerged(10)
	if err != nil {
		t.Fatal(err)
	}
	var got EntityRow
	for _, m := range merged {
		if m.Name == "茨木童子" {
			got = m
		}
	}
	if got.Authority != AuthorityCorrected {
		t.Errorf("该标 corrected：%+v", got)
	}
	if got.Description != "人改过的：鬼手是右手" {
		t.Errorf("描述该取纠正那份（`MIN(description)` 会挑到模型那份）：%+v", got)
	}

	top, err := idx.TopEntities(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range top {
		if m.Name == "茨木童子" && m.Description != "人改过的：鬼手是右手" {
			t.Errorf("TopEntities 也该纠错优先：%+v", m)
		}
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (hay == needle || indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
