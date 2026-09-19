package vaultapp

import (
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/infrastructure/vaultindex"
)

// TestGraphSearchModes 是 P4 的落地验收（离线、不花额度）：
// 实体名式查询能靠图找到「提到它的那块」，四种模式都跑得通，缺模型时降级要说出来。
func TestGraphSearchModes(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/机制/伤害.md",
		"---\ntitle: 伤害\nstatus: published\n---\n\n"+repeat("最终伤害等于攻击乘以系数。", 40))
	write(t, root, "docs/式神/NAME_A.md",
		"---\ntitle: NAME_A\nstatus: draft\n---\n\n"+repeat("NAME_A 靠鬼手打伤害。", 40))

	svc := New(root)
	if err := svc.Reindex(); err != nil {
		t.Fatal(err)
	}
	// 造点图：NAME_A 出现在它自己的文档里，NAME_B 是它的邻居。
	idx := vaultindex.New(root)
	if _, err := idx.PutEntities([]vaultindex.EntityRow{
		{Name: "NAME_A", Type: "TYPE_X", Doc: "docs/式神/NAME_A.md", FromLine: 5, ToLine: 40, Line: 6},
		{Name: "NAME_B", Type: "TYPE_X", Doc: "docs/机制/伤害.md", FromLine: 5, ToLine: 40, Line: 7},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.PutRelations([]vaultindex.RelationRow{
		{Src: "NAME_A", Dst: "NAME_B", Keywords: "K", Doc: "docs/式神/NAME_A.md", FromLine: 5, ToLine: 40, Line: 6},
	}); err != nil {
		t.Fatal(err)
	}

	// local：查询里的实体名 → 它出现的块（并且要带状态）。
	res, err := svc.GraphSearch("NAME_A 是什么", ModeLocal, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entities) == 0 || res.Entities[0] != "NAME_A" {
		t.Fatalf("该对上 NAME_A：%+v", res.Entities)
	}
	if len(res.Hits) == 0 {
		t.Fatal("该找到 NAME_A 出现的块")
	}
	found := false
	for _, h := range res.Hits {
		if h.Doc == "docs/式神/NAME_A.md" {
			found = true
			if h.Status != "draft" {
				t.Errorf("结果要带文档状态（未核验可见）：%+v", h)
			}
			if h.FromLine == 0 || h.ToLine < h.FromLine {
				t.Errorf("结果要带行号区间：%+v", h)
			}
		}
	}
	if !found {
		t.Errorf("该命中 NAME_A 那篇：%+v", res.Hits)
	}

	// global / hybrid / mix 都要能跑（mix 没有模型时应**明说降级**，而不是静默）。
	for _, mode := range []GraphMode{ModeGlobal, ModeHybrid, ModeMix} {
		r, err := svc.GraphSearch("NAME_A", mode, 5)
		if err != nil {
			t.Fatalf("%s 模式失败：%v", mode, err)
		}
		if len(r.Hits) == 0 {
			t.Errorf("%s 模式该有结果：%+v", mode, r)
		}
		if mode == ModeMix && r.Note == "" {
			t.Error("mix 没有模型时该说明降级（不许静默）")
		}
	}

	// 空查询与未知模式要报错。
	if _, err := svc.GraphSearch("", ModeLocal, 5); err == nil {
		t.Error("空查询该报错")
	}
	if _, err := ParseGraphMode("nope"); err == nil {
		t.Error("未知模式该报错")
	}
	if m, err := ParseGraphMode(""); err != nil || m != ModeHybrid {
		t.Errorf("空模式该默认 hybrid：%v %v", m, err)
	}
	_ = strings.TrimSpace
}
