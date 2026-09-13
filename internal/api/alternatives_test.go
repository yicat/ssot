package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

func altProject(t *testing.T) (string, *AlternativesService) {
	t.Helper()
	dir := workspaceProject(t)
	sess := compose.NewSession(filepath.Dir(dir), dir)
	t.Cleanup(func() { _ = sess.Close() })
	return dir, NewAlternativesService(sess)
}

func planInput(over func(*PlanProposalInput)) PlanProposalInput {
	in := PlanProposalInput{
		Scenario: "calc", Title: "打对折那条线",
		Objective:   "收益最大",
		Constraints: []string{"不花勾玉"},
		Preference:  "省时间",
		Metrics: []MetricView{
			{Name: "资源", Value: 100, Unit: "point", LowerIsBetter: true},
			{Name: "时间", Value: 30, Unit: "min", LowerIsBetter: true},
		},
		Opportunity: "放弃另一条线的进度",
		Sources:     []string{"huijiwiki"},
	}
	if over != nil {
		over(&in)
	}
	return in
}

// ── 自证 ────────────────────────────────────────────────────────────────────

// 验收：每个方案都声明目标、约束、代价（含机会成本）。
func TestPlanRequiresObjectiveAndOpportunity(t *testing.T) {
	_, svc := altProject(t)

	if _, err := svc.Propose(planInput(func(in *PlanProposalInput) { in.Objective = "" })); err == nil {
		t.Error("不声明目标的方案必须被拒绝")
	}
	if _, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Title = "另一条线"
		in.Opportunity = ""
	})); err == nil {
		t.Error("缺机会成本的方案必须被拒绝")
	}
}

// 验收：方案可信度不得高于其依赖断言的最低可信度。
func TestPlanConfidenceIsCappedByDependencies(t *testing.T) {
	dir, svc := altProject(t)
	seed(t, dir,
		mk("skill", "262_01", "id", value.Of("262_01")),
		mk("skill", "262_01", "character_id", value.OfUnit(262.0, "point")),
	)
	p, err := svc.port()
	if err != nil {
		t.Fatal(err)
	}
	as, err := p.Store.BySubject("skill", "262_01")
	if err != nil || len(as) == 0 {
		t.Fatal("应能取到技能断言")
	}
	id := as[0].ID

	got, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Depends = []string{"assert:" + id}
	}))
	if err != nil {
		t.Fatal(err)
	}
	// 库里写入的是 L2，方案因此不得高于 L2
	if got.MaxConfidence != "L2" {
		t.Errorf("方案可信度应被依赖压到 L2，实际 %s", got.MaxConfidence)
	}
	// 依赖的断言尚未核验，未核验比例应为 100%
	if got.UnverifiedRatio != 1 {
		t.Errorf("未核验比例应为 1，实际 %v", got.UnverifiedRatio)
	}
}

// 验收：已驳回断言不参与任何成案。
func TestPlanWithRejectedDependencyIsNotExecutable(t *testing.T) {
	dir, svc := altProject(t)
	seed(t, dir, mk("skill", "262_01", "id", value.Of("262_01")))
	p, err := svc.port()
	if err != nil {
		t.Fatal(err)
	}
	as, err := p.Store.BySubject("skill", "262_01")
	if err != nil || len(as) == 0 {
		t.Fatal("应能取到断言")
	}
	if err := p.Store.Verify(verification.Record{
		AssertionID: as[0].ID, Decision: verification.Rejected, Method: verification.Editorial,
		ProposedBy: verification.Actor{Kind: verification.Agent, ID: "dsh"},
		ApprovedBy: &verification.Actor{Kind: verification.Human, ID: "ngnl5"},
		Reason:     "与原文不符", At: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Depends = []string{"assert:" + as[0].ID}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "invalid" || got.Executable {
		t.Errorf("依赖被驳回的方案应不可执行，实际 %s", got.Status)
	}
	if _, err := svc.Choose(got.ID, "ngnl5", "就它了"); err == nil {
		t.Error("不可执行的方案不得被选中")
	}
}

// 依赖写法必须可机械解析，否则「依据可追溯」无从落实。
func TestPlanDependencyFormatIsChecked(t *testing.T) {
	_, svc := altProject(t)
	_, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Depends = []string{"我觉得跟暴击有关"}
	}))
	if err == nil {
		t.Fatal("无法解析的依赖必须被拒绝")
	}
	if !strings.Contains(err.Error(), "assert:") {
		t.Errorf("报错应给出正确写法，实际：%v", err)
	}
}

// ── 呈现 ────────────────────────────────────────────────────────────────────

// 验收：未声明偏好时不产生单一方案，而是产生标注了各自偏好假设的备选集合。
func TestEvaluateRefusesSingleSolutionWithoutPreference(t *testing.T) {
	_, svc := altProject(t)
	if _, err := svc.Propose(planInput(nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Title = "同一偏好的另一版"
	})); err != nil {
		t.Fatal(err)
	}

	// 未声明偏好 + 只有一种偏好假设 -> 拒绝
	if _, err := svc.Evaluate("calc", "", nil, 0); err == nil {
		t.Fatal("未声明偏好且只有一个偏好假设时必须拒绝")
	}

	// 补一个不同偏好的备选之后可以呈现
	if _, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Title = "省资源那条线"
		in.Preference = "省资源"
		in.Metrics = []MetricView{
			{Name: "资源", Value: 30, Unit: "point", LowerIsBetter: true},
			{Name: "时间", Value: 90, Unit: "min", LowerIsBetter: true},
		}
	})); err != nil {
		t.Fatal(err)
	}
	ev, err := svc.Evaluate("calc", "", nil, 0)
	if err != nil {
		t.Fatalf("覆盖两种偏好时应可呈现：%v", err)
	}
	for _, plan := range ev.Plans {
		if plan.Preference == "" {
			t.Error("每个备选都必须标注它假设的偏好")
		}
	}
}

// 验收：被支配的方案被剪枝，并说得出是被谁支配的。
func TestEvaluatePrunesDominated(t *testing.T) {
	_, svc := altProject(t)
	good, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Title = "更省的那条"
		in.Metrics = []MetricView{
			{Name: "资源", Value: 50, Unit: "point", LowerIsBetter: true},
			{Name: "时间", Value: 30, Unit: "min", LowerIsBetter: true},
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	bad, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Title = "更贵的那条"
		in.Metrics = []MetricView{
			{Name: "资源", Value: 200, Unit: "point", LowerIsBetter: true},
			{Name: "时间", Value: 30, Unit: "min", LowerIsBetter: true},
		}
	}))
	if err != nil {
		t.Fatal(err)
	}

	ev, err := svc.Evaluate("calc", "省时间", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Plans) != 1 || ev.Plans[0].ID != good.ID {
		t.Fatalf("应只留下更省的那个，实际 %+v", ev.Plans)
	}
	if len(ev.Pruned) != 1 || ev.Pruned[0].By != good.ID {
		t.Fatalf("剪枝必须说得出被谁支配，实际 %+v", ev.Pruned)
	}
	_ = bad
}

// 验收：全部方案都不可行时，输出的是冲突的约束清单而非空结果。
func TestEvaluateReportsConflictingConstraints(t *testing.T) {
	_, svc := altProject(t)
	if _, err := svc.Propose(planInput(nil)); err != nil {
		t.Fatal(err)
	}
	ev, err := svc.Evaluate("calc", "省时间", []string{"不花勾玉", "必须用勾玉"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Plans) != 0 {
		t.Fatalf("没有方案满足全部约束，实际留下 %d 个", len(ev.Plans))
	}
	if len(ev.Constraints) == 0 {
		t.Fatal("必须报出约束清单，而不是返回空结果")
	}
	joined := strings.Join(ev.Constraints, " / ")
	if !strings.Contains(joined, "不花勾玉") || !strings.Contains(joined, "必须用勾玉") {
		t.Errorf("清单应把打架的约束都摆出来，实际 %v", ev.Constraints)
	}
	if !strings.Contains(ev.Note, "互相打架") {
		t.Errorf("说明应指出是要求自相矛盾，实际 %q", ev.Note)
	}
}

// 验收：仅有一个方案时明确说明未发现实质不同的备选。
func TestEvaluateSaysNothingToCompare(t *testing.T) {
	_, svc := altProject(t)
	if _, err := svc.Propose(planInput(nil)); err != nil {
		t.Fatal(err)
	}
	ev, err := svc.Evaluate("calc", "省时间", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ev.Note, "未发现实质不同的备选") {
		t.Errorf("必须说清「没得比」，实际 %q", ev.Note)
	}
}

// ── 选择与反馈 ──────────────────────────────────────────────────────────────

// 验收：玩家选择与理由被持久化；其他备选仍可访问。
func TestChoosePersistsAndKeepsOthers(t *testing.T) {
	_, svc := altProject(t)
	a, err := svc.Propose(planInput(nil))
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Title = "省资源那条线"
		in.Preference = "省资源"
		in.Metrics = []MetricView{
			{Name: "资源", Value: 30, Unit: "point", LowerIsBetter: true},
			{Name: "时间", Value: 90, Unit: "min", LowerIsBetter: true},
		}
	}))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Choose(a.ID, "", "就它了"); err == nil {
		t.Error("缺选定人必须被拒绝")
	}
	if _, err := svc.Choose(a.ID, "ngnl5", ""); err == nil {
		t.Error("缺理由必须被拒绝")
	}
	got, err := svc.Choose(a.ID, "ngnl5", "时间更短")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "chosen" || !got.Chosen || got.ChosenBy != "human:ngnl5" {
		t.Errorf("选定结果应被记录，实际 %+v", got)
	}
	if got.ChooseReason != "时间更短" {
		t.Error("选择理由必须保留——它是推断偏好的原料")
	}

	list, err := svc.List("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("其他备选必须仍可访问，实际 %d 个", len(list))
	}
	if list[0].ID != a.ID {
		t.Errorf("已选中的应排最前，实际 %s", list[0].ID)
	}
	if list[1].ID != b.ID {
		t.Error("未被选中的备选不得被删掉")
	}
}

// 验收：依据变更后方案被标记为「依据已变」，且不静默沿用。
func TestRefreshMarksFormulaChange(t *testing.T) {
	dir, svc := altProject(t)
	fp := filepath.Join(dir, "formulas", "scale.formula.yml")
	b, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fp,
		[]byte(strings.Replace(string(b), `version: "1"`, `version: "2"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Depends = []string{"formula:scale@1"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	// 依赖断言为空时方案可信度只能是 L4
	if plan.MaxConfidence != "L4" {
		t.Errorf("没有断言依据时不得声称高可信度，实际 %s", plan.MaxConfidence)
	}

	res, err := svc.Refresh("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stale) != 1 || res.Stale[0] != plan.ID {
		t.Fatalf("公式版本变化应标记「依据已变」，实际 %+v", res)
	}
	list, _ := svc.List("calc")
	if list[0].Status != "stale" {
		t.Errorf("状态应为依据已变，实际 %s", list[0].Status)
	}
	// 「依据已变」仍可执行，但要提示
	if !list[0].Executable {
		t.Error("依据已变不是不可执行——它是提示，不是硬拦截")
	}
}

// 验收：依据被驳回后，方案不可执行且指出失效依据。
func TestRefreshInvalidatesOnRejectedAssertion(t *testing.T) {
	dir, svc := altProject(t)
	seed(t, dir, mk("skill", "262_01", "id", value.Of("262_01")))
	p, err := svc.port()
	if err != nil {
		t.Fatal(err)
	}
	as, err := p.Store.BySubject("skill", "262_01")
	if err != nil || len(as) == 0 {
		t.Fatal("应能取到断言")
	}
	// 先建方案（此时断言还是 pending）
	plan, err := svc.Propose(planInput(func(in *PlanProposalInput) {
		in.Depends = []string{"assert:" + as[0].ID}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status == "invalid" {
		t.Fatal("断言尚未驳回时方案应可执行")
	}

	// 再驳回
	if err := p.Store.Verify(verification.Record{
		AssertionID: as[0].ID, Decision: verification.Rejected, Method: verification.Editorial,
		ProposedBy: verification.Actor{Kind: verification.Agent, ID: "dsh"},
		ApprovedBy: &verification.Actor{Kind: verification.Human, ID: "ngnl5"},
		Reason:     "与原文不符", At: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Refresh("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Invalid) != 1 || res.Invalid[0] != plan.ID {
		t.Fatalf("依据被驳回后方案应标记不可执行，实际 %+v", res)
	}
	list, _ := svc.List("calc")
	if list[0].Executable {
		t.Error("不可执行的方案不得再被执行")
	}
	if _, err := svc.Choose(plan.ID, "ngnl5", "就它了"); err == nil {
		t.Error("不可执行的方案不得被选中")
	}
}

// 方案只归属于场景：跨场景不可见。
func TestPlansAreScopedToScenario(t *testing.T) {
	_, svc := altProject(t)
	if _, err := svc.Propose(planInput(nil)); err != nil {
		t.Fatal(err)
	}
	other, err := svc.List("另一个场景")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("方案挂在场景下，跨场景不得可见，实际 %d 个", len(other))
	}
	all, err := svc.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("空场景名表示全部，实际 %d 个", len(all))
	}
}
