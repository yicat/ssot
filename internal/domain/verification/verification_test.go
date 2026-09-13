package verification

import (
	"strings"
	"testing"
	"time"
)

func human(id string) *Actor { return &Actor{Kind: Human, ID: id} }

func base() Record {
	return Record{
		AssertionID: "a1",
		Decision:    Approved,
		Method:      Editorial,
		ProposedBy:  Actor{Kind: Agent, ID: "ingest-pipeline"},
		ApprovedBy:  human("yicat"),
		Reason:      "与原文逐字比对一致",
		At:          time.Now(),
	}
}

func TestValidRecordPasses(t *testing.T) {
	if err := base().Validate(); err != nil {
		t.Fatalf("完整核验记录不应报错：%v", err)
	}
}

// 无追责的核验等于没有核验：提出者与批准者必须分别标识。
func TestRequiresBothProposerAndApprover(t *testing.T) {
	r := base()
	r.ApprovedBy = nil
	if err := r.Validate(); err == nil {
		t.Error("缺批准者必须被拒绝")
	}

	r = base()
	r.ProposedBy = Actor{}
	if err := r.Validate(); err == nil {
		t.Error("缺提出者必须被拒绝")
	}

	r = base()
	r.ProposedBy = Actor{Kind: Agent}
	if err := r.Validate(); err == nil {
		t.Error("提出者缺身份标识必须被拒绝")
	}
}

// agent 可以产出大量候选，但不能自己决定什么算数。
func TestAgentCannotApprove(t *testing.T) {
	r := base()
	r.ApprovedBy = &Actor{Kind: Agent, ID: "some-agent"}
	err := r.Validate()
	if err == nil {
		t.Fatal("agent 不得作为批准者")
	}
	if !strings.Contains(err.Error(), "必须是**人**") {
		t.Errorf("错误消息应说明批准者必须是人，实际：%v", err)
	}
}

// 只有状态没有理由的不是核验。
func TestRequiresReason(t *testing.T) {
	r := base()
	r.Reason = ""
	if err := r.Validate(); err == nil {
		t.Error("缺理由必须被拒绝")
	}
}

// 以实测核验时必须记录依据（版本、配置、样本数）——否则与猜测无异。
func TestMeasurementRequiresEvidence(t *testing.T) {
	r := base()
	r.Method = Measurement
	if err := r.Validate(); err == nil {
		t.Error("实测缺依据必须被拒绝")
	}
	r.Evidence = "版本 2026-09；双方均无增益；样本 3 次"
	if err := r.Validate(); err != nil {
		t.Errorf("带依据的实测应通过：%v", err)
	}
}

// 四级方法强度必须可比较 —— 「重算不得高于其依赖」这类规则依赖它。
func TestMethodRankOrdering(t *testing.T) {
	if !(Measurement.Rank() > Recompute.Rank() &&
		Recompute.Rank() > CrossSource.Rank() &&
		CrossSource.Rank() > Editorial.Rank()) {
		t.Error("方法强度顺序应为 实测 > 重算 > 多源 > 编审")
	}
}

// 编审是判断，不是验证 —— 它必须能被区分出来。
func TestEditorialIsLowest(t *testing.T) {
	if Editorial.Rank() != 1 {
		t.Errorf("编审应为最低强度，实际 %d", Editorial.Rank())
	}
	if Editorial.Label() != "编审" {
		t.Errorf("编审的中文名应为「编审」，实际 %q", Editorial.Label())
	}
}

func TestUnknownDecisionAndMethodRejected(t *testing.T) {
	r := base()
	r.Decision = "maybe"
	if err := r.Validate(); err == nil {
		t.Error("未知结论必须被拒绝")
	}

	r = base()
	r.Method = "vibes"
	err := r.Validate()
	if err == nil {
		t.Fatal("未知方法必须被拒绝")
	}
	if !strings.Contains(err.Error(), "editorial") {
		t.Errorf("错误消息应列出合法方法，实际：%v", err)
	}
}

func TestMissingTimeRejected(t *testing.T) {
	r := base()
	r.At = time.Time{}
	if err := r.Validate(); err == nil {
		t.Error("缺时间必须被拒绝")
	}
}

func TestDecisionMapsToStatus(t *testing.T) {
	cases := map[Decision]string{
		Approved: "verified",
		Rejected: "rejected",
		Disputed: "disputed",
	}
	for d, want := range cases {
		if got := d.StatusFor(); got != want {
			t.Errorf("%s 应映射到 %s，实际 %s", d, want, got)
		}
	}
}

func TestSummarizeAnswersWhoWhenWhy(t *testing.T) {
	s := base().Summarize()
	for _, want := range []string{"编审", "agent:ingest-pipeline", "human:yicat", "逐字比对"} {
		if !strings.Contains(s, want) {
			t.Errorf("摘要应包含 %q，实际 %q", want, s)
		}
	}
}
