package vault

import "testing"

func TestMatchEntityNames(t *testing.T) {
	names := []string{"NAME_A", "NAME_B", "NAME_C_LONG", "短"}
	// 名字整串出现在查询里 → 1.0，并且排最前。
	got := MatchEntityNames("NAME_A 怎么用 NAME_B", names, 0.5)
	if len(got) == 0 || got[0].Name != "NAME_A" || got[0].Score != 1 {
		t.Fatalf("该先命中 NAME_A：%+v", got)
	}
	if got[1].Name != "NAME_B" {
		t.Errorf("NAME_B 也该命中：%+v", got)
	}
	// 名字里包含查询（短查询问长名字）。
	got2 := MatchEntityNames("NAME_C", names, 0.5)
	if len(got2) == 0 || got2[0].Name != "NAME_C_LONG" {
		t.Errorf("短查询该对上长名字：%+v", got2)
	}
	// 太短的名字（<2 字）不参与，免得误伤。
	for _, m := range got2 {
		if m.Name == "短" {
			t.Errorf("单字名不该参与匹配：%+v", got2)
		}
	}
	// 完全对不上 → 空。
	if m := MatchEntityNames("完全不相干的东西", names, 0.9); len(m) != 0 {
		t.Errorf("高阈值下不该命中：%+v", m)
	}
	// 空查询 → 空。
	if m := MatchEntityNames("   ", names, 0.5); m != nil {
		t.Errorf("空查询该返回空：%+v", m)
	}
}

func TestFuseCandidates(t *testing.T) {
	// 同一个块被两路命中 → 分数相加、理由拼接；排序确定。
	cands := []GraphCandidate{
		{Doc: "d1", FromLine: 10, ToLine: 20, Score: 1.0, Why: "本地"},
		{Doc: "d1", FromLine: 10, ToLine: 25, Score: 0.5, Why: "向量"},
		{Doc: "d2", FromLine: 5, ToLine: 8, Score: 1.2, Why: "本地"},
	}
	got := FuseCandidates(cands, 10)
	if len(got) != 2 {
		t.Fatalf("该合并成 2 条：%+v", got)
	}
	if got[0].Doc != "d1" || got[0].Score != 1.5 {
		t.Errorf("分数该相加（1.0+0.5=1.5）且排最前：%+v", got[0])
	}
	if got[0].ToLine != 25 {
		t.Errorf("区间该取并集的右端：%+v", got[0])
	}
	if got[0].Why != "本地；向量" {
		t.Errorf("理由该拼接：%q", got[0].Why)
	}
	// 同分时按 (doc, 起始行) 升序——确定。
	tie := FuseCandidates([]GraphCandidate{
		{Doc: "b", FromLine: 1, Score: 1},
		{Doc: "a", FromLine: 9, Score: 1},
		{Doc: "a", FromLine: 2, Score: 1},
	}, 10)
	if tie[0].Doc != "a" || tie[0].FromLine != 2 || tie[1].FromLine != 9 || tie[2].Doc != "b" {
		t.Errorf("同分排序不确定：%+v", tie)
	}
	// limit 生效。
	if n := len(FuseCandidates(cands, 1)); n != 1 {
		t.Errorf("limit 该生效：%d", n)
	}
}
