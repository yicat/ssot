package vaultindex

import "testing"

// TestGraphQueries 是 P4「local 模式」的地基验收：实体能连到它的关系与它出现的块。
func TestGraphQueries(t *testing.T) {
	idx := newVectorVault(t) // docs/机制/伤害.md（published）与 docs/式神/茨木.md（draft）

	ents := []EntityRow{
		{Name: "NAME_A", Type: "TYPE_X", Description: "A", Doc: "docs/式神/茨木.md", FromLine: 1, ToLine: 5, Line: 3},
		{Name: "NAME_B", Type: "TYPE_X", Description: "B", Doc: "docs/机制/伤害.md", FromLine: 10, ToLine: 20, Line: 12},
	}
	if _, err := idx.PutEntities(ents); err != nil {
		t.Fatal(err)
	}
	rels := []RelationRow{
		{Src: "NAME_A", Dst: "NAME_B", Keywords: "K1", Description: "A→B", Doc: "docs/式神/茨木.md", FromLine: 1, ToLine: 5, Line: 3},
		{Src: "NAME_C", Dst: "NAME_A", Keywords: "K2", Description: "C→A", Doc: "docs/机制/伤害.md", FromLine: 10, ToLine: 20, Line: 15},
	}
	if _, err := idx.PutRelations(rels); err != nil {
		t.Fatal(err)
	}

	// 一跳邻居：两个方向都要（out 是 A→B，in 是 C→A），按方向与名字可预期。
	nb, err := idx.Neighbors("NAME_A", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(nb) != 2 {
		t.Fatalf("该有 2 条一跳：%+v", nb)
	}
	got := map[string]string{}
	for _, n := range nb {
		got[n.Dir] = n.Other
	}
	if got["out"] != "NAME_B" || got["in"] != "NAME_C" {
		t.Errorf("方向或另一端不对：%+v", got)
	}
	for _, n := range nb {
		if n.Rel.Doc == "" || n.Rel.FromLine == 0 {
			t.Errorf("关系要带来源（doc + 行号），否则回不了原文：%+v", n.Rel)
		}
	}

	// 实体 → 块：用行号区间求交，且只取提到的那个文档的块。
	chunks, err := idx.ChunksForEntity("NAME_B", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("该找到提到 NAME_B 的块")
	}
	for _, c := range chunks {
		if c.Doc != "docs/机制/伤害.md" {
			t.Errorf("只该取实体来源文档里的块：%+v", c)
		}
		if c.ToLine < 10 || c.FromLine > 20 {
			t.Errorf("该与实体的来源行号区间相交：%+v", c)
		}
	}

	// 按度数取实体：NAME_A 有 2 条关系来源行，该排在只有 1 条的前面。
	top, err := idx.TopEntities(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) == 0 || top[0].Name != "NAME_A" {
		t.Errorf("度数（来源行数）最高的该排第一：%+v", top)
	}
}
