package verification

import (
	"math/rand"
	"testing"
)

// 类别之间的顺序不该被数量翻转：争议永远压过影响面。
func TestDisputedOutranksImpact(t *testing.T) {
	disputed := Rank(PriorityInput{Disputed: true})
	highImpact := Rank(PriorityInput{References: 50})
	if disputed.Score <= highImpact.Score {
		t.Errorf("争议(%d) 应压过 影响面(%d)", disputed.Score, highImpact.Score)
	}
	if disputed.Tier != TierDisputed {
		t.Errorf("有争议时应归入 %s，实际 %s", TierDisputed, disputed.Tier)
	}
}

// 影响面永远压过分级。
func TestImpactOutranksConfidence(t *testing.T) {
	impact := Rank(PriorityInput{References: 1})
	l4 := Rank(PriorityInput{Confidence: "L4"})
	if impact.Score <= l4.Score {
		t.Errorf("影响面(%d) 应压过 分级(%d)", impact.Score, l4.Score)
	}
}

// 方向：L4 推断最需要核验，L1 直引虽然同样未核验但风险最低。
func TestL4OutranksL1(t *testing.T) {
	l4 := Rank(PriorityInput{Confidence: "L4"})
	l1 := Rank(PriorityInput{Confidence: "L1"})
	if l4.Score <= l1.Score {
		t.Errorf("L4(%d) 的核验优先级应高于 L1(%d)", l4.Score, l1.Score)
	}
}

// 主导理由只保留一条 —— 界面上它要和断言并列显示，四条理由没人看。
func TestReasonIsSingleAndDominant(t *testing.T) {
	p := Rank(PriorityInput{Disputed: true, References: 9, Confidence: "L4", Sampled: true})
	if p.Reason == "" {
		t.Fatal("必须给出理由")
	}
	if p.Tier != TierDisputed {
		t.Errorf("多种因素并存时应归入最高类别 %s，实际 %s", TierDisputed, p.Tier)
	}
	if len([]rune(p.Reason)) > 60 {
		t.Errorf("理由应简洁，实际 %d 字：%s", len([]rune(p.Reason)), p.Reason)
	}
}

func TestRoutineWhenNothingApplies(t *testing.T) {
	p := Rank(PriorityInput{})
	if p.Tier != TierRoutine {
		t.Errorf("无特殊因素时应为 %s，实际 %s", TierRoutine, p.Tier)
	}
	if p.Score != 0 {
		t.Errorf("无特殊因素时分数应为 0，实际 %d", p.Score)
	}
}

// 抽检项**必须**在截断时保住 —— 否则它会被高分项永远挤出队列，
// 抽检就等于没做，系统性问题仍然发现不了。
func TestSampledSurvivesTruncation(t *testing.T) {
	var items []Ranked
	// 50 条高分（影响面）
	for i := 0; i < 50; i++ {
		id := string(rune('A'+i%26)) + "-high"
		items = append(items, Ranked{ID: id, Priority: Rank(PriorityInput{References: 10})})
	}
	// 1 条低分但被抽中
	items = append(items, Ranked{ID: "SAMPLED", Priority: Rank(PriorityInput{Sampled: true})})

	got := Order(items, 10)
	found := false
	for _, it := range got {
		if it.ID == "SAMPLED" {
			found = true
		}
	}
	if !found {
		t.Error("抽检项不得被高分项挤出队列")
	}
	if len(got) != 10 {
		t.Errorf("应恰好截断到 10 条，实际 %d", len(got))
	}
}

func TestOrderIsByScoreDescending(t *testing.T) {
	items := []Ranked{
		{ID: "low", Priority: Rank(PriorityInput{Confidence: "L1"})},
		{ID: "high", Priority: Rank(PriorityInput{References: 5})},
		{ID: "mid", Priority: Rank(PriorityInput{Confidence: "L4"})},
	}
	got := Order(items, 0)
	if len(got) != 3 || got[0].ID != "high" {
		t.Errorf("应按分数降序，实际首项 %s", got[0].ID)
	}
}

// 「随机」不等于「不可复现」——出问题必须能重放同一批。
func TestSampleIsReproducibleWithSeed(t *testing.T) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = string(rune('a' + i%26))
		ids[i] += "-" + string(rune('0'+i/26))
	}
	a := Sample(ids, 0.1, rand.New(rand.NewSource(42)))
	b := Sample(ids, 0.1, rand.New(rand.NewSource(42)))
	if len(a) != len(b) {
		t.Fatalf("同种子应得到同样多的抽检项：%d vs %d", len(a), len(b))
	}
	for k := range a {
		if !b[k] {
			t.Errorf("同种子应得到同一批抽检项，%q 只出现在一侧", k)
		}
	}
	c := Sample(ids, 0.1, rand.New(rand.NewSource(43)))
	diff := 0
	for k := range a {
		if !c[k] {
			diff++
		}
	}
	if diff == 0 {
		t.Error("不同种子应得到不同的抽检批次")
	}
}

func TestSampleEdgeCases(t *testing.T) {
	if got := Sample(nil, 0.5, rand.New(rand.NewSource(1))); len(got) != 0 {
		t.Error("空候选应返回空抽检")
	}
	ids := []string{"a", "b", "c"}
	if got := Sample(ids, 0, rand.New(rand.NewSource(1))); len(got) != 0 {
		t.Error("比例为 0 时不应抽检")
	}
	// 比例极小时至少抽 1 个，否则抽检形同虚设
	if got := Sample(ids, 0.001, rand.New(rand.NewSource(1))); len(got) != 1 {
		t.Errorf("比例极小时应至少抽 1 个，实际 %d", len(got))
	}
	if got := Sample(ids, 2, rand.New(rand.NewSource(1))); len(got) != 3 {
		t.Errorf("比例大于 1 时应被截到候选总数，实际 %d", len(got))
	}
}
