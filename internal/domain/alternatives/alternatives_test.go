package alternatives

import (
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
)

var at = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func base(id string, over func(*Plan)) Plan {
	p := Plan{
		ID: id, Scenario: "daily", Title: "方案 " + id,
		Objective: "收益最大",
		Metrics: []Metric{
			{Name: "资源", Value: 100, Unit: "point", LowerIsBetter: true},
			{Name: "时间", Value: 30, Unit: "min", LowerIsBetter: true},
		},
		Opportunity:   "放弃另一条线的进度",
		Depends:       []string{"assert:a1"},
		MaxConfidence: assertion.L2,
		Sources:       []string{"huijiwiki"},
		Status:        StatusCandidate,
		At:            at,
		Preference:    "省时间",
	}
	if over != nil {
		over(&p)
	}
	return p
}

// ── 自证 ────────────────────────────────────────────────────────────────────

// 验收：每个方案都声明目标与约束。
func TestPlanRequiresObjective(t *testing.T) {
	p := base("p1", func(p *Plan) { p.Objective = "" })
	if err := p.Validate(); err == nil {
		t.Fatal("不声明目标的方案必须被拒绝")
	} else if !strings.Contains(err.Error(), "目标") {
		t.Errorf("报错应指明缺目标，实际：%v", err)
	}
}

// 验收：代价必须含机会成本——它是取舍里最容易被忽略的一项。
func TestPlanRequiresOpportunityCost(t *testing.T) {
	p := base("p1", func(p *Plan) { p.Opportunity = "" })
	if err := p.Validate(); err == nil {
		t.Fatal("缺机会成本的方案必须被拒绝")
	} else if !strings.Contains(err.Error(), "机会成本") {
		t.Errorf("报错应指明缺机会成本，实际：%v", err)
	}
}

func TestPlanRequiresSourceAndScenario(t *testing.T) {
	if err := base("p1", func(p *Plan) { p.Scenario = "" }).Validate(); err == nil {
		t.Error("无归属场景的方案必须被拒绝")
	}
	if err := base("p1", func(p *Plan) { p.Sources = nil }).Validate(); err == nil {
		t.Error("无来源的方案必须被拒绝")
	}
}

func TestUnverifiedRatioMustBeInRange(t *testing.T) {
	if err := base("p1", func(p *Plan) { p.UnverifiedRatio = 1.5 }).Validate(); err == nil {
		t.Error("未核验比例越界必须被拒绝")
	}
}

// ── 支配与剪枝 ──────────────────────────────────────────────────────────────

// 支配是**所有维度都不劣且至少一个严格更优**。
func TestDominatesRequiresAllDimensions(t *testing.T) {
	a := base("a", func(p *Plan) {
		p.Metrics = []Metric{
			{Name: "资源", Value: 80, LowerIsBetter: true},
			{Name: "时间", Value: 30, LowerIsBetter: true},
		}
	})
	b := base("b", nil)
	if !Dominates(a, b) {
		t.Error("资源更省、时间相同，a 应支配 b")
	}
	if Dominates(b, a) {
		t.Error("支配关系是单向的")
	}

	// 一优一劣：互不支配
	c := base("c", func(p *Plan) {
		p.Metrics = []Metric{
			{Name: "资源", Value: 80, LowerIsBetter: true},
			{Name: "时间", Value: 40, LowerIsBetter: true},
		}
	})
	if Dominates(c, b) || Dominates(b, c) {
		t.Error("一优一劣时应互不支配——不得只看一个维度就下结论")
	}
}

// 全都是「越大越好」的维度时，比较方向必须跟着翻。
func TestDominatesRespectsDirection(t *testing.T) {
	high := base("high", func(p *Plan) {
		p.Metrics = []Metric{{Name: "收益", Value: 200, LowerIsBetter: false}}
	})
	low := base("low", func(p *Plan) {
		p.Metrics = []Metric{{Name: "收益", Value: 100, LowerIsBetter: false}}
	})
	if !Dominates(high, low) {
		t.Error("收益更高应支配收益更低")
	}
}

// 比较前提不同就不是「同一组维度上的两个选择」。
func TestDominatesNeedsSameConditions(t *testing.T) {
	a := base("a", func(p *Plan) {
		p.Metrics = []Metric{{Name: "资源", Value: 80, LowerIsBetter: true}}
	})
	b := base("b", func(p *Plan) {
		p.Metrics = []Metric{{Name: "资源", Value: 100, LowerIsBetter: true}}
	})
	if !Dominates(a, b) {
		t.Error("同目标、同假设、同偏好时才谈得上支配")
	}
	// 偏好不同 → 不可比
	b2 := base("b2", func(p *Plan) {
		p.Preference = "省资源"
		p.Metrics = []Metric{{Name: "资源", Value: 100, LowerIsBetter: true}}
	})
	if Dominates(a, b2) {
		t.Error("偏好不同就不是同一组维度上的两个选择，不得下支配结论")
	}
}

// 维度对不上时不可比——不作支配结论。
func TestDominatesRequiresComparableMetrics(t *testing.T) {
	a := base("a", func(p *Plan) {
		p.Metrics = []Metric{{Name: "资源", Value: 1, LowerIsBetter: true}}
	})
	b := base("b", func(p *Plan) {
		p.Metrics = []Metric{{Name: "风险", Value: 1, LowerIsBetter: true}}
	})
	if Dominates(a, b) || Dominates(b, a) {
		t.Error("维度对不上时不可比")
	}
}

// 验收：被支配的方案被剪枝，且说得出是被谁支配的。
func TestPruneKeepsParetoSetAndSaysWhy(t *testing.T) {
	good := base("good", func(p *Plan) {
		p.Metrics = []Metric{
			{Name: "资源", Value: 80, LowerIsBetter: true},
			{Name: "时间", Value: 30, LowerIsBetter: true},
		}
	})
	bad := base("bad", func(p *Plan) {
		p.Metrics = []Metric{
			{Name: "资源", Value: 120, LowerIsBetter: true},
			{Name: "时间", Value: 30, LowerIsBetter: true},
		}
	})
	res := Prune([]Plan{good, bad})
	if len(res.Kept) != 1 || res.Kept[0].ID != "good" {
		t.Fatalf("应只留下 good，实际 %+v", res.Kept)
	}
	if len(res.Pruned) != 1 {
		t.Fatalf("应剪掉 1 个，实际 %d", len(res.Pruned))
	}
	if res.Pruned[0].By != "good" || res.Pruned[0].Dominance == "" {
		t.Error("剪枝必须说得出是被谁支配的——凭空消失会让人以为系统没算出来")
	}
}

// 验收：含随机因素的结果以区间或期望呈现——这里体现为不可比的方案不被强行排序。
func TestParetoSetKeepsIncomparable(t *testing.T) {
	a := base("a", func(p *Plan) {
		p.Metrics = []Metric{
			{Name: "资源", Value: 80, LowerIsBetter: true},
			{Name: "时间", Value: 40, LowerIsBetter: true},
		}
	})
	b := base("b", func(p *Plan) {
		p.Metrics = []Metric{
			{Name: "资源", Value: 120, LowerIsBetter: true},
			{Name: "时间", Value: 20, LowerIsBetter: true},
		}
	})
	res := Prune([]Plan{a, b})
	if len(res.Kept) != 2 {
		t.Errorf("互不支配的两个方案都该留下，实际 %d", len(res.Kept))
	}
}

// ── 合并 ────────────────────────────────────────────────────────────────────

// 验收：差异低于阈值的方案被合并，避免伪多样性。
func TestMergeEquivalent(t *testing.T) {
	a := base("a", func(p *Plan) {
		p.Metrics = []Metric{{Name: "资源", Value: 100, LowerIsBetter: true}}
	})
	b := base("b", func(p *Plan) {
		p.Metrics = []Metric{{Name: "资源", Value: 101, LowerIsBetter: true}}
	})
	kept, groups := MergeEquivalent([]Plan{a, b}, 5)
	if len(kept) != 1 {
		t.Fatalf("差异低于阈值应合并，实际留下 %d 个", len(kept))
	}
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Errorf("合并组应记录两个方案，实际 %+v", groups)
	}

	kept2, _ := MergeEquivalent([]Plan{a, b}, 0.5)
	if len(kept2) != 2 {
		t.Errorf("阈值收紧后不应再合并，实际 %d 个", len(kept2))
	}
}

// ── 呈现 ────────────────────────────────────────────────────────────────────

// 验收：未声明偏好时不得给出单一方案。
func TestEvaluateRefusesSinglePlanWithoutPreference(t *testing.T) {
	// 两个方案假设了同一种偏好 —— 那等于「针对该玩家的单一方案」
	p1 := base("p1", nil)
	p2 := base("p2", func(p *Plan) { p.Title = "方案 p2 的另一版" })

	_, err := Evaluate([]Plan{p1, p2}, Request{}, 0)
	if err == nil {
		t.Fatal("未声明偏好且只有一个偏好假设时，必须拒绝")
	}
	if !strings.Contains(err.Error(), "偏好") {
		t.Errorf("报错应指明是偏好问题，实际：%v", err)
	}
}

// 验收：未声明偏好时可以给备选集合，但每个备选必须标注它假设了哪种偏好。
func TestEvaluateAllowsMultiSolutionWithPreferences(t *testing.T) {
	p1 := base("p1", nil)
	p2 := base("p2", func(p *Plan) {
		p.Title = "省资源那条线"
		p.Preference = "省资源"
		p.Metrics = []Metric{
			{Name: "资源", Value: 60, LowerIsBetter: true},
			{Name: "时间", Value: 60, LowerIsBetter: true},
		}
	})
	ev, err := Evaluate([]Plan{p1, p2}, Request{}, 0)
	if err != nil {
		t.Fatalf("覆盖两种偏好时应被接受：%v", err)
	}
	if len(ev.Plans) != 2 {
		t.Fatalf("两个偏好不同、互不支配的方案都该呈现，实际 %d", len(ev.Plans))
	}
	for _, p := range ev.Plans {
		if p.Preference == "" {
			t.Error("每个备选都必须标注它假设的偏好")
		}
	}
}

// 验收：全部方案被剪枝时，输出的是冲突的约束清单而非空结果。
// 验收：约束互斥导致无可行方案时，输出的是冲突的约束清单而非空结果。
//
// 这是「全部方案都被剪枝」的真正来源：不是系统算不出来，
// 而是要求本身自相矛盾——后者是信息，前者只是故障。
func TestEvaluateReportsConstraintsWhenNoneFeasible(t *testing.T) {
	p1 := base("p1", func(p *Plan) {
		p.Constraints = []string{"不花勾玉"}
	})
	p2 := base("p2", func(p *Plan) {
		p.Title = "省资源那条线"
		p.Preference = "省资源"
		p.Constraints = []string{"必须用勾玉加速"}
	})

	// 两条要求不可能同时满足
	ev, err := Evaluate([]Plan{p1, p2}, Request{
		Preference:  "省时间",
		Constraints: []string{"不花勾玉", "必须用勾玉加速"},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Plans) != 0 {
		t.Fatalf("没有任何方案满足全部约束，实际留下 %d 个", len(ev.Plans))
	}
	if len(ev.Constraints) == 0 {
		t.Fatal("必须报出冲突的约束清单，而不是返回空结果")
	}
	joined := strings.Join(ev.Constraints, " / ")
	if !strings.Contains(joined, "不花勾玉") || !strings.Contains(joined, "必须用勾玉加速") {
		t.Errorf("清单应把互相打架的约束都摆出来，实际 %v", ev.Constraints)
	}
	if !strings.Contains(ev.Note, "互相打架") {
		t.Errorf("说明应指出这是要求自相矛盾，实际 %q", ev.Note)
	}
}

// 验收：仅有一个方案时明确说明未发现实质不同的备选，不假装有多样性。
func TestEvaluateSaysWhenThereIsNothingToCompare(t *testing.T) {
	p := base("p1", func(p *Plan) {
		p.Preference = "省时间"
	})
	ev, err := Evaluate([]Plan{p}, Request{Preference: "省时间"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Plans) != 1 {
		t.Fatalf("应留下 1 个，实际 %d", len(ev.Plans))
	}
	if !strings.Contains(ev.Note, "未发现实质不同的备选") {
		t.Errorf("必须说清「没得比」，实际：%q", ev.Note)
	}
}

// 验收：已驳回依据的方案不参与呈现。
func TestEvaluateSkipsInvalidPlans(t *testing.T) {
	ok := base("ok", nil)
	bad := base("bad", func(p *Plan) {
		p.Preference = "省资源"
		p.Status = StatusInvalid
	})
	ev, err := Evaluate([]Plan{ok, bad}, Request{Preference: "省时间"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ev.Plans {
		if p.ID == "bad" {
			t.Error("不可执行的方案不得参与呈现")
		}
	}
}

// 验收：偏好推断只是建议，必须经显式确认才生效。
func TestPreferenceInferenceIsOnlySuggested(t *testing.T) {
	chosen := base("chosen", func(p *Plan) {
		p.Status = StatusChosen
		p.Preference = "省时间"
	})
	other := base("other", func(p *Plan) {
		p.Title = "省资源那条线"
		p.Preference = "省资源"
		p.Metrics = []Metric{
			{Name: "资源", Value: 30, LowerIsBetter: true},
			{Name: "时间", Value: 90, LowerIsBetter: true},
		}
	})
	ev, err := Evaluate([]Plan{chosen, other}, Request{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ev.PreferenceInferred != "省时间" {
		t.Errorf("应给出偏好建议，实际 %q", ev.PreferenceInferred)
	}
	// 但呈现里不得删掉其他备选
	if len(ev.Plans) != 2 {
		t.Errorf("推断偏好不得删除其他备选，实际 %d 个", len(ev.Plans))
	}
}

// ── 排序 ────────────────────────────────────────────────────────────────────

// 验收：玩家选定方案后，其他备选仍可访问；长期偏好只影响顺序，不删除。
func TestOrderKeepsAllAlternatives(t *testing.T) {
	chosen := base("chosen", func(p *Plan) { p.Status = StatusChosen })
	a := base("a", func(p *Plan) { p.Title = "A"; p.Preference = "省资源" })
	b := base("b", func(p *Plan) { p.Title = "B"; p.Preference = "省时间" })

	got := Order([]Plan{a, b, chosen}, "省资源")
	if len(got) != 3 {
		t.Fatalf("不得删除任何备选，实际 %d", len(got))
	}
	if got[0].ID != "chosen" {
		t.Errorf("已选中的应排最前，实际 %s", got[0].ID)
	}
	// 顺序必须确定
	again := Order([]Plan{a, b, chosen}, "省资源")
	for i := range got {
		if got[i].ID != again[i].ID {
			t.Fatal("两次排序结果必须相同")
		}
	}
	// 偏好只影响顺序
	if got[1].ID != "a" {
		t.Errorf("偏好「省资源」的方案应排在前面，实际 %s", got[1].ID)
	}
}
