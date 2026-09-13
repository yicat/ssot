package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// expProject 造一个带场景的最小项目，并把会话记录准备好。
func expProject(t *testing.T) (string, *compose.Session, *ExperienceService) {
	t.Helper()
	dir := workspaceProject(t)
	sess := compose.NewSession(filepath.Dir(dir), dir)
	t.Cleanup(func() { _ = sess.Close() })
	return dir, sess, NewExperienceService(sess)
}

func openSession(t *testing.T, svc *ExperienceService, title string) SessionView {
	t.Helper()
	v, err := svc.OpenSession("calc", title)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func proposal(s SessionView, over func(*ProposalInput)) ProposalInput {
	in := ProposalInput{
		Scenario: "calc", Topic: "倍率口径", Kind: "judgment",
		Statement: "倍率取主伤害那一段",
		Rationale: "与技能描述的主句一致",
		SessionID: s.ID, Anchor: "1",
		ProposedByKind: "human", ProposedByID: "ngnl5",
	}
	if over != nil {
		over(&in)
	}
	return in
}

// ── 提出 ────────────────────────────────────────────────────────────────────

// 验收：每条经验都记录提出者；无归属的经验拒绝记录。
func TestProposeRequiresScenarioAndProposer(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")

	if _, err := svc.Propose(proposal(s, func(in *ProposalInput) { in.Scenario = "" })); err == nil {
		t.Error("无归属的经验必须被拒绝")
	}
	if _, err := svc.Propose(proposal(s, func(in *ProposalInput) { in.ProposedByID = "" })); err == nil {
		t.Error("无提出者的经验必须被拒绝")
	}
	if _, err := svc.Propose(proposal(s, func(in *ProposalInput) { in.SessionID = "" })); err == nil {
		t.Error("无会话依据的经验必须被拒绝")
	}
	if _, err := svc.Propose(proposal(s, func(in *ProposalInput) { in.Anchor = "" })); err == nil {
		t.Error("无锚点的经验必须被拒绝——依据不可回溯就无法审查")
	}
}

// 验收：每条经验可回溯到会话记录中的具体位置。
func TestProposeKeepsAnchor(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	got, err := svc.Propose(proposal(s, func(in *ProposalInput) { in.Anchor = "2:5" }))
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != s.ID || got.Anchor != "2:5" {
		t.Errorf("依据应被保留，实际 %s#%s", got.SessionID, got.Anchor)
	}
	if got.Status != "candidate" {
		t.Errorf("新提出的是候选，实际 %s", got.Status)
	}
	if got.Usable {
		t.Error("候选不得被引用")
	}
}

// 同一个 (场景, 话题, 判断) 重复提出是幂等的，不会堆成十几条。
func TestProposeIsIdempotent(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")

	first, err := svc.Propose(proposal(s, nil))
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Propose(proposal(s, nil))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("同一句话应得到同一条目，实际 %s / %s", first.ID, second.ID)
	}
	list, err := svc.List("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("重复提出不得堆积，实际 %d 条", len(list))
	}
}

// 验收：总结型经验必须带样本量——那是意见，不是经验。
func TestSummaryRequiresSample(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")

	_, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "summary"
		in.Statement = "先手速度线在 210 以上"
	}))
	if err == nil {
		t.Fatal("总结型缺样本量必须被拒绝")
	}
	if !strings.Contains(err.Error(), "样本量") {
		t.Errorf("报错应指明缺样本量，实际：%v", err)
	}

	got, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "summary"
		in.Statement = "先手速度线在 210 以上"
		in.SampleSize = 12
		in.SampleFrom = "近 12 场斗技"
	}))
	if err != nil {
		t.Fatalf("补上样本量后应通过：%v", err)
	}
	if got.SampleSize != 12 || got.SampleFrom != "近 12 场斗技" {
		t.Errorf("样本量与来源都应保留，实际 %+v", got)
	}
}

// 验收：推导型经验带推导链，且写法必须可机械解析——否则「可重放」无从落实。
func TestDerivedRequiresParsableChain(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")

	_, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "derived"
		in.Chain = []string{"我觉得跟暴击有关"} // 自由文本
	}))
	if err == nil {
		t.Fatal("无法解析的推导链必须被拒绝")
	}
	if !strings.Contains(err.Error(), "assert:") {
		t.Errorf("报错应给出正确写法，实际：%v", err)
	}

	got, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "derived"
		in.Chain = []string{"assert:a-1", "formula:crit_factor@1"}
	}))
	if err != nil {
		t.Fatalf("合法推导链应通过：%v", err)
	}
	if len(got.Chain) != 2 {
		t.Errorf("推导链应被保留，实际 %+v", got.Chain)
	}
}

// ── 责任级别 ────────────────────────────────────────────────────────────────

// 责任级别由参与者与依据推导，**不由调用方填写**。
func TestLevelIsDerived(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")

	cases := []struct {
		name string
		in   ProposalInput
		want int
	}{
		{"只有人", proposal(s, func(in *ProposalInput) {
			in.Statement = "人独立得出的判断"
		}), 2},
		{"agent 有推导链", proposal(s, func(in *ProposalInput) {
			in.ProposedByKind = "agent"
			in.ProposedByID = "dsh"
			in.Statement = "agent 按公式推出的判断"
			in.Chain = []string{"assert:a-1"}
		}), 3},
		{"agent 无推导链", proposal(s, func(in *ProposalInput) {
			in.ProposedByKind = "agent"
			in.ProposedByID = "dsh"
			in.Statement = "agent 凭印象给出的判断"
		}), 4},
		{"人机协作", proposal(s, func(in *ProposalInput) {
			in.ProposedByKind = "agent"
			in.ProposedByID = "dsh"
			in.Statement = "协作得出的判断"
			in.Collaborators = []ActorView{{Kind: "human", ID: "ngnl5"}}
		}), 1},
	}
	for _, c := range cases {
		got, err := svc.Propose(c.in)
		if err != nil {
			t.Fatalf("%s：%v", c.name, err)
		}
		if got.Level != c.want {
			t.Errorf("%s：级别应为 %d，实际 %d", c.name, c.want, got.Level)
		}
		if got.MaxConfidence == "" {
			t.Errorf("%s：应给出可信度上限", c.name)
		}
	}
}

// ── 批准 ────────────────────────────────────────────────────────────────────

// 验收：缺少批准者时拒绝记录；agent 作为批准者时拒绝。
func TestApproveRequiresHuman(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	e, err := svc.Propose(proposal(s, nil))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Approve(e.ID, "", "r"); err == nil {
		t.Error("缺批准者必须被拒绝")
	}
	if _, err := svc.Approve(e.ID, "ngnl5", ""); err == nil {
		t.Error("缺理由的批准必须被拒绝")
	}
	got, err := svc.Approve(e.ID, "ngnl5", "与技能主句一致")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "effective" || got.ApprovedBy == nil || got.ApprovedBy.ID != "ngnl5" {
		t.Errorf("批准后应生效并记录批准者，实际 %+v", got)
	}
	if !got.Usable {
		t.Error("生效的经验应可被引用")
	}
}

// 验收：同意修正——agent 只能提出，不能批准。
func TestAgentCannotApprove(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	e, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.ProposedByKind = "agent"
		in.ProposedByID = "dsh"
	}))
	if err != nil {
		t.Fatal(err)
	}
	// 界面只传人类名字，因此这里无法构造 agent 批准者；
	// 但领域层必须挡住——用 store 直接试一次。
	p, err := svc.session.Project()
	if err != nil {
		t.Fatal(err)
	}
	cur, ok, err := p.Store.Entry(e.ID)
	if err != nil || !ok {
		t.Fatal("取不到刚提出的经验")
	}
	if _, err := cur.Approve(verification.Actor{Kind: verification.Agent, ID: "dsh"}, "r"); err == nil {
		t.Error("agent 批准必须被领域层拒绝")
	}
}

// 验收：驳回是审计轨迹，条目保留且不得再被批准。
func TestRejectKeepsEntry(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	e, err := svc.Propose(proposal(s, nil))
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Reject(e.ID, "ngnl5", "依据不足")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "rejected" {
		t.Errorf("状态应为已驳回，实际 %s", got.Status)
	}
	list, err := svc.List("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Error("驳回不得删除条目")
	}
	if _, err := svc.Approve(e.ID, "ngnl5", "算了还是通过"); err == nil {
		t.Error("已驳回的经验不得再被批准")
	}
}

// ── 冲突 ────────────────────────────────────────────────────────────────────

// 验收：新判断与既有经验冲突时并存并标记，不覆盖、不择一。
func TestConflictsCoexist(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")

	a, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Statement = "倍率取主伤害那一段"
	}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Statement = "倍率取最后一段劈斩"
	}))
	if err != nil {
		t.Fatal(err)
	}
	// 不同话题不构成冲突
	if _, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Topic = "另议"
		in.Statement = "倍率取主伤害那一段"
	})); err != nil {
		t.Fatal(err)
	}

	groups, err := svc.Conflicts("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("应有 1 组冲突，实际 %d：%+v", len(groups), groups)
	}
	if len(groups[0].Entries) != 2 {
		t.Fatalf("冲突组应含 2 条，实际 %d", len(groups[0].Entries))
	}
	ids := map[string]bool{}
	for _, e := range groups[0].Entries {
		ids[e.ID] = true
	}
	if !ids[a.ID] || !ids[b.ID] {
		t.Error("两条说法都必须在组里——系统不替你裁决")
	}
}

// 说法相同的不是冲突。
func TestSameStatementIsNotConflict(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	if _, err := svc.Propose(proposal(s, nil)); err != nil {
		t.Fatal(err)
	}
	groups, err := svc.Conflicts("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("同一句话不得算冲突，实际 %+v", groups)
	}
}

// ── 会话记录（依据）────────────────────────────────────────────────────────

// 验收：会话记录不可删改——只能开、只能追加。
func TestSessionIsAppendOnly(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	if len(s.Turns) != 0 {
		t.Fatalf("新会话不应有发言，实际 %d 段", len(s.Turns))
	}

	got, err := svc.AppendTurn(s.ID, "human", "ngnl5", "暴击系数到底怎么算？")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 1 || got.Turns[0].Seq != 1 {
		t.Fatalf("应有 1 段发言，实际 %+v", got.Turns)
	}
	got, err = svc.AppendTurn(s.ID, "agent", "dsh", "按 1 + cri × crid，与实测一致")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("应有 2 段发言，实际 %d", len(got.Turns))
	}
	if got.Turns[1].Role.Kind != "agent" {
		t.Error("参与者必须逐段记录——否则无法判断哪句是谁说的")
	}

	// 首段不得被改写
	if got.Turns[0].Text != "暴击系数到底怎么算？" {
		t.Error("追加不得改动已有发言——可删改的依据不是依据")
	}

	// 再开一个同名会话不会覆盖已有的
	again := openSession(t, svc, "第一次讨论")
	if again.ID == s.ID {
		t.Error("同一纳秒之外的同名会话应是新会话，不应撞进已有记录")
	}
	sessions, err := svc.Sessions("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("应有 2 段会话，实际 %d", len(sessions))
	}
}

func TestAppendTurnRequiresActorAndText(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	if _, err := svc.AppendTurn(s.ID, "human", "", "说了句话"); err == nil {
		t.Error("无参与者的发言必须被拒绝")
	}
	if _, err := svc.AppendTurn(s.ID, "human", "ngnl5", ""); err == nil {
		t.Error("空发言必须被拒绝")
	}
	if _, err := svc.AppendTurn("nope", "human", "ngnl5", "x"); err == nil {
		t.Error("往不存在的会话里追加必须被拒绝")
	}
}

// ── 失效与待重算 ────────────────────────────────────────────────────────────

// 验收：依赖断言被驳回时经验标记失效。
func TestRefreshStalesOnRejectedAssertion(t *testing.T) {
	dir, sess, svc := expProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})

	p, err := sess.Project()
	if err != nil {
		t.Fatal(err)
	}
	// 找一条断言并驳回它
	as, err := p.Store.BySubject("skill", "262_01")
	if err != nil || len(as) == 0 {
		t.Fatal("应能取到技能断言")
	}
	target := as[0]
	if err := p.Store.Verify(verification.Record{
		AssertionID: target.ID, Decision: verification.Rejected, Method: verification.Editorial,
		ProposedBy: verification.Actor{Kind: verification.Agent, ID: "dsh"},
		ApprovedBy: &verification.Actor{Kind: verification.Human, ID: "ngnl5"},
		Reason:     "与原文不符", At: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	s := openSession(t, svc, "第一次讨论")
	e, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "derived"
		in.Chain = []string{"assert:" + target.ID}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(e.ID, "ngnl5", "依据充分"); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Refresh("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Staled) != 1 || res.Staled[0] != e.ID {
		t.Fatalf("依赖被驳回的经验应标记失效，实际 %+v", res)
	}
	list, err := svc.List("calc")
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Status != "stale" || list[0].Usable {
		t.Errorf("失效的经验不得被引用，实际 %+v", list[0])
	}
	if list[0].Statement != e.Statement {
		t.Error("失效不得改写判断本身——它是记录")
	}
}

// 验收：依赖公式被修改时标记待重算。
func TestRefreshRecomputesOnFormulaVersionChange(t *testing.T) {
	dir, _, svc := expProject(t)
	// 把公式版本从 1 改成 2
	fp := filepath.Join(dir, "formulas", "scale.formula.yml")
	b, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(fp, []byte(strings.Replace(string(b), `version: "1"`, `version: "2"`, 1)), 0o644)

	s := openSession(t, svc, "第一次讨论")
	e, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "derived"
		in.Chain = []string{"formula:scale@1"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(e.ID, "ngnl5", "依据充分"); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Refresh("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recompute) != 1 || res.Recompute[0] != e.ID {
		t.Fatalf("依赖公式版本变化的经验应标记待重算，实际 %+v", res)
	}
	list, _ := svc.List("calc")
	if list[0].Status != "recompute" {
		t.Errorf("状态应为待重算，实际 %s", list[0].Status)
	}
}

// 依赖完好的经验不得被无端标记。
func TestRefreshLeavesHealthyEntriesAlone(t *testing.T) {
	dir, _, svc := expProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})

	s := openSession(t, svc, "第一次讨论")
	e, err := svc.Propose(proposal(s, func(in *ProposalInput) {
		in.Kind = "derived"
		in.Chain = []string{"formula:scale@1"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(e.ID, "ngnl5", "依据充分"); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Refresh("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Staled) != 0 || len(res.Recompute) != 0 {
		t.Errorf("依赖完好的经验不该被动，实际 %+v", res)
	}
	list, _ := svc.List("calc")
	if list[0].Status != "effective" {
		t.Errorf("状态应保持生效，实际 %s", list[0].Status)
	}
}

// ── 取代 ────────────────────────────────────────────────────────────────────

// 验收：经验修订只追加版本，原条目保留。
func TestSupersedeKeepsOldEntry(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	old, err := svc.Propose(proposal(s, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(old.ID, "ngnl5", "依据充分"); err != nil {
		t.Fatal(err)
	}

	next, prev, err := svc.Supersede(old.ID, proposal(s, func(in *ProposalInput) {
		in.Statement = "改判：倍率取最后一段劈斩"
		in.Rationale = "重新核对原文后改了判断"
	}))
	if err != nil {
		t.Fatal(err)
	}
	if next.Supersedes != old.ID {
		t.Errorf("新条目应指向被取代的旧条目，实际 %q", next.Supersedes)
	}
	if prev.Status != "superseded" {
		t.Errorf("旧条目应标为已被取代，实际 %s", prev.Status)
	}

	list, err := svc.List("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("旧条目必须保留，实际 %d 条", len(list))
	}
	var found bool
	for _, e := range list {
		if e.ID == old.ID && e.Statement == old.Statement {
			found = true
		}
	}
	if !found {
		t.Error("旧条目的内容不得被改动")
	}
}

// 内容相同的新判断无需取代。
func TestSupersedeRejectsIdentical(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	old, err := svc.Propose(proposal(s, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Supersede(old.ID, proposal(s, nil)); err == nil {
		t.Error("内容相同的「取代」必须被拒绝")
	}
}

// 生效与否不影响「经验必须归属场景」这条底线：跨场景取不到。
func TestListIsScopedToScenario(t *testing.T) {
	_, _, svc := expProject(t)
	s := openSession(t, svc, "第一次讨论")
	if _, err := svc.Propose(proposal(s, nil)); err != nil {
		t.Fatal(err)
	}
	other, err := svc.List("另一个场景")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("经验挂在场景下，跨场景不得可见，实际 %d 条", len(other))
	}
	all, err := svc.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("空场景名表示全部，实际 %d 条", len(all))
	}
}

// 断言：AssertionStatus 对不存在的断言返回 false，而不是报错。
func TestAssertionStatusMissing(t *testing.T) {
	dir, sess, _ := expProject(t)
	_ = dir
	p, err := sess.Project()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := p.Store.AssertionStatus("nope"); err != nil || ok {
		t.Errorf("不存在的断言应返回 false 而非错误，实际 ok=%v err=%v", ok, err)
	}
}
