package vaultapp

import (
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/infrastructure/scopefile"
)

// TestExtractFollowsDeclaration 是收录范围的落地验收：
// 声明说「这层不收」→ 抽取就不该碰它，而且要说清跳过了几篇。
func TestExtractFollowsDeclaration(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/机制/伤害.md",
		"---\ntitle: 伤害\nstatus: published\ntags: [整理层]\n---\n\n"+
			repeat("最终伤害等于攻击乘以系数。", 40))
	write(t, root, "raw/DIR_X/流水.md",
		"---\ntitle: 流水\nstatus: draft\ntags: [原始层, TAG_X]\n---\n\n"+
			repeat("一句话流水。", 40))

	svc := New(root)
	// 没声明：全收（默认不静默少收）。
	_, skipped, err := svc.ExtractChunksForDocs(0, 8)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("没声明时不该跳过任何文档，实际 %d", skipped)
	}

	// 声明排除 raw/DIR_X/** → 抽取跳过它。
	if err := scopefile.Save(root,
		vault.DerivedScope{Exclude: vault.ScopeRule{Paths: []string{"raw/DIR_X/**"}}},
		vault.ExtractConfig{EntityTypes: []string{"TYPE_X"}}); err != nil {
		t.Fatal(err)
	}
	batches, skipped, err := svc.ExtractChunksForDocs(0, 8)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("该跳过 1 篇（raw/DIR_X/流水.md），实际 %d", skipped)
	}
	for _, b := range batches {
		for _, c := range b {
			if c.Doc == "raw/DIR_X/流水.md" {
				t.Errorf("按声明被排除的文档不该进抽取：%s", c.Doc)
			}
		}
	}

	// 一致性检查：被排除的那篇已经在派生层里（索引建过），该报成「越界」。
	res, err := svc.ScopeCheck()
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || res.InScope != 1 {
		t.Errorf("该收 1 篇 / 共 2 篇：%+v", res)
	}
	if len(res.OutOfScope) != 1 || res.OutOfScope[0] != "raw/DIR_X/流水.md" {
		t.Errorf("越界该报出来：%+v", res.OutOfScope)
	}

	// 门：agent 不能改收录范围。
	agent, err := vault.ParseActor("agent:整理")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScope(vault.DerivedScope{}, vault.ExtractConfig{}, agent); err == nil {
		t.Error("agent 不该能设收录范围（只有人）")
	}
	human, err := vault.ParseActor("human:我")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScope(vault.DerivedScope{}, vault.ExtractConfig{EntityTypes: []string{"TYPE_X"}}, human); err != nil {
		t.Errorf("人该能设：%v", err)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
