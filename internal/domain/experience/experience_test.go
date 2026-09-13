package experience

import (
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

var at = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func human(id string) verification.Actor { return verification.Actor{Kind: verification.Human, ID: id} }
func agent(id string) verification.Actor { return verification.Actor{Kind: verification.Agent, ID: id} }

func base(over func(*Entry)) Entry {
	e := Entry{
		ID: "e1", Scenario: "damage-calc", Topic: "crit_factor 口径",
		Kind: KindJudgment, Statement: "暴击系数按 1 + cri × crid 算",
		Rationale:  "与游戏内实测一致",
		ProposedBy: human("ngnl5"),
		SessionID:  "s1", Anchor: "3:12",
		Status: StatusCandidate, At: at,
	}
	if over != nil {
		over(&e)
	}
	return e
}

// 责任级别是**算出来的**，不是填出来的。
func TestLevelIsDerivedFromParticipants(t *testing.T) {
	cases := []struct {
		name string
		e    Entry
		want Responsibility
	}{
		{"只有人", base(nil), LevelHuman},
		{"只有 agent 且有推导链", base(func(e *Entry) {
			e.ProposedBy = agent("dsh")
			e.Chain = []string{"a1", "formula:crit_factor"}
		}), LevelReproducible},
		{"只有 agent 且无推导链", base(func(e *Entry) {
			e.ProposedBy = agent("dsh")
		}), LevelInferred},
		{"人机协作", base(func(e *Entry) {
			e.ProposedBy = agent("dsh")
			e.Collaborators = []verification.Actor{human("ngnl5")}
			e.Chain = []string{"a1"}
		}), LevelCoauthored},
	}
	for _, c := range cases {
		if got := c.e.Level(); got != c.want {
			t.Errorf("%s：级别应为 %d，实际 %d", c.name, c.want, got)
		}
	}
}

// 「协作」的判据是参与者里同时出现人与 agent，不是「记不清是谁做的」。
func TestCollaborationNeedsBothKinds(t *testing.T) {
	twoAgents := base(func(e *Entry) {
		e.ProposedBy = agent("a")
		e.Collaborators = []verification.Actor{agent("b")}
		e.Chain = []string{"x"}
	})
	if twoAgents.Level() != LevelReproducible {
		t.Errorf("两个 agent 不算协作，实际级别 %d", twoAgents.Level())
	}
}

// 可信度上限：级别 1/2 同为 L2——差别在来源，不在分级本身。
func TestMaxConfidence(t *testing.T) {
	if LevelCoauthored.MaxConfidence() != assertion.L2 {
		t.Error("协作的上限应为 L2：经验始终是判断，不是直引")
	}
	if LevelHuman.MaxConfidence() != assertion.L2 {
		t.Error("人工确认的上限应为 L2")
	}
	if LevelReproducible.MaxConfidence() != assertion.L3 {
		t.Error("可复现应到 L3")
	}
	if LevelInferred.MaxConfidence() != assertion.L4 {
		t.Error("推断只能是 L4")
	}
}

// 验收：无归属的经验拒绝记录。
func TestEntryRequiresScenario(t *testing.T) {
	e := base(func(e *Entry) { e.Scenario = "" })
	if err := e.Validate(); err == nil {
		t.Fatal("无归属的经验必须被拒绝")
	} else if !strings.Contains(err.Error(), "归属场景") {
		t.Errorf("报错应说明缺归属，实际：%v", err)
	}
}

// 验收：没有比较键就无法发现冲突。
func TestEntryRequiresTopic(t *testing.T) {
	e := base(func(e *Entry) { e.Topic = "" })
	if err := e.Validate(); err == nil {
		t.Error("缺话题必须被拒绝")
	}
}

// 验收：推导型经验必须带推导链。
func TestDerivedRequiresChain(t *testing.T) {
	e := base(func(e *Entry) { e.Kind = KindDerived })
	if err := e.Validate(); err == nil {
		t.Fatal("推导型缺推导链必须被拒绝")
	}
	e.Chain = []string{"a1"}
	if err := e.Validate(); err != nil {
		t.Errorf("补上推导链后应通过：%v", err)
	}
}

// 验收：总结型经验必须带样本量——没有样本量的「经验」只是意见。
func TestSummaryRequiresSample(t *testing.T) {
	e := base(func(e *Entry) { e.Kind = KindSummary })
	if err := e.Validate(); err == nil {
		t.Fatal("总结型缺样本量必须被拒绝")
	} else if !strings.Contains(err.Error(), "样本量") {
		t.Errorf("报错应指明缺样本量，实际：%v", err)
	}
	e.Sample = &Sample{Size: 0}
	if err := e.Validate(); err == nil {
		t.Error("样本量为 0 同样必须被拒绝")
	}
	e.Sample = &Sample{Size: 12, From: "近 12 场斗技"}
	if err := e.Validate(); err != nil {
		t.Errorf("有样本量后应通过：%v", err)
	}
}

// 验收：必须能回溯到会话记录中的具体位置。
func TestEntryRequiresAnchor(t *testing.T) {
	for _, mutate := range []func(*Entry){
		func(e *Entry) { e.SessionID = "" },
		func(e *Entry) { e.Anchor = "" },
	} {
		e := base(mutate)
		if err := e.Validate(); err == nil {
			t.Error("缺少会话或锚点必须被拒绝——依据不可回溯的经验无法被审查")
		}
	}
}

// 验收：每条经验都记录提出者。
func TestEntryRequiresProposer(t *testing.T) {
	e := base(func(e *Entry) { e.ProposedBy = verification.Actor{} })
	if err := e.Validate(); err == nil {
		t.Error("缺提出者必须被拒绝")
	}
}

// 验收：agent 作为批准者时拒绝。
func TestAgentCannotApprove(t *testing.T) {
	e := base(nil)
	if _, err := e.Approve(agent("dsh"), "我看着没问题"); err == nil {
		t.Fatal("agent 批准必须被拒绝")
	} else if !strings.Contains(err.Error(), "人") {
		t.Errorf("报错应说明批准者必须是 人，实际：%v", err)
	}
}

// 验收：批准者缺失时拒绝记录。
func TestApproveRequiresActor(t *testing.T) {
	e := base(nil)
	if _, err := e.Approve(verification.Actor{}, "r"); err == nil {
		t.Error("未标识批准者必须被拒绝")
	}
}

// 验收：只有状态没有理由的不是确认。
func TestApproveRequiresReason(t *testing.T) {
	e := base(nil)
	if _, err := e.Approve(human("ngnl5"), ""); err == nil {
		t.Error("缺理由的批准必须被拒绝")
	}
}

// 验收：批准后状态为生效。
func TestApproveMakesEffective(t *testing.T) {
	e := base(nil)
	got, err := e.Approve(human("ngnl5"), "与实测一致")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusEffective {
		t.Errorf("状态应为生效，实际 %s", got.Status)
	}
	if got.ApprovedBy == nil || got.ApprovedBy.ID != "ngnl5" {
		t.Error("批准者应被记录")
	}
	if got.ProposedBy.ID != "ngnl5" {
		t.Error("自提自批是允许的，但提出者不得被抹掉——记录里必须体现")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("生效后的不变量应自洽：%v", err)
	}
}

// 验收：驳回是审计轨迹，不能靠再批准抹掉。
func TestRejectedCannotBeApproved(t *testing.T) {
	e := base(nil)
	rej, err := e.Reject(human("ngnl5"), "依据不足")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rej.Approve(human("ngnl5"), "算了还是通过"); err == nil {
		t.Error("已驳回的经验不得再被批准")
	}
}

// 验收：生效的经验不得被重复批准。
func TestCannotApproveTwice(t *testing.T) {
	e := base(nil)
	ok, err := e.Approve(human("ngnl5"), "r")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ok.Approve(human("other"), "r2"); err == nil {
		t.Error("重复批准必须被拒绝")
	}
}

// 验收：冲突经验并存且被标记，不自动择一。
func TestConflictsAreMarkedNotResolved(t *testing.T) {
	a := base(nil)
	b := base(func(e *Entry) {
		e.ID = "e2"
		e.Statement = "暴击系数按 cure 口径算"
	})
	c := base(func(e *Entry) {
		e.ID = "e3"
		e.Topic = "别的話題"
	})
	d := base(func(e *Entry) { e.ID = "e4" }) // 说法完全相同

	got := a.Conflicts([]Entry{a, b, c, d})
	if len(got) != 1 || got[0] != "e2" {
		t.Errorf("只有同一场景同一话题的不同说法才算冲突，实际 %v", got)
	}
}

// 验收：修订只追加版本，原条目保留。
func TestSupersedeKeepsOldEntry(t *testing.T) {
	old := base(nil)
	next := base(func(e *Entry) {
		e.ID = "e2"
		e.Statement = "改成 1 + cri × cure"
	})
	got, err := old.Supersede(next)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSuperseded {
		t.Errorf("旧条目应为「已被取代」，实际 %s", got.Status)
	}
	if got.Statement != old.Statement {
		t.Error("旧条目的内容不得被改动——只能追加")
	}
}

// 取代只能发生在同一场景的同一话题上。
func TestSupersedeRejectsDifferentTopic(t *testing.T) {
	old := base(nil)
	next := base(func(e *Entry) { e.ID = "e2"; e.Topic = "别的话题" })
	if _, err := old.Supersede(next); err == nil {
		t.Error("跨话题取代必须被拒绝")
	}
}

// 依赖断言被驳回 → 标记失效，但条目本身保留。
func TestStaleKeepsEntry(t *testing.T) {
	e := base(nil)
	got := e.Stale("依赖的断言 a1 已被驳回")
	if got.Status != StatusStale {
		t.Errorf("状态应为失效，实际 %s", got.Status)
	}
	if got.Statement != e.Statement {
		t.Error("失效不得改写判断本身——它是记录")
	}
	if got.Status.Usable() {
		t.Error("失效的经验不得被引用")
	}
}

// 排序必须确定：级别高的在前，同级按时间倒序。
func TestOrderIsDeterministic(t *testing.T) {
	mk := func(id string, level int, at time.Time) Entry {
		e := base(func(e *Entry) { e.ID = id; e.At = at })
		switch level {
		case 1:
			e.ProposedBy = agent("dsh")
			e.Collaborators = []verification.Actor{human("h")}
		case 3:
			e.ProposedBy = agent("dsh")
			e.Chain = []string{"x"}
		case 4:
			e.ProposedBy = agent("dsh")
		}
		return e
	}
	items := []Entry{
		mk("inferred", 4, at),
		mk("coauthored", 1, at),
		mk("reproducible", 3, at),
	}
	first := Order(items)
	second := Order(items)
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatal("两次排序结果必须相同")
		}
	}
	if first[0].ID != "coauthored" || first[2].ID != "inferred" {
		t.Errorf("级别高的应在前，实际 %s, %s, %s", first[0].ID, first[1].ID, first[2].ID)
	}
}
