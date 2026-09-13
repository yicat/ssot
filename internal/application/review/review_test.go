package review

import (
	"errors"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

type fakeSource struct {
	items  []assertion.Assertion
	refs   map[string]int
	groups [][]assertion.Assertion
	last   assertion.Filter
}

func (f *fakeSource) Select(fl assertion.Filter) ([]assertion.Assertion, error) {
	f.last = fl
	return f.items, nil
}
func (f *fakeSource) ReferenceCounts() (map[string]int, error) { return f.refs, nil }
func (f *fakeSource) Conflicts() ([][]assertion.Assertion, error) {
	return f.groups, nil
}

type fakeWriter struct {
	recs   []verification.Record
	failOn string
}

func (f *fakeWriter) Verify(r verification.Record) error {
	if f.failOn != "" && r.AssertionID == f.failOn {
		return errors.New("模拟失败")
	}
	f.recs = append(f.recs, r)
	return nil
}

func mk(id string, over func(*assertion.Assertion)) assertion.Assertion {
	a := assertion.Assertion{
		ID: id, Entity: "shikigami", Subject: "262", Predicate: "atk",
		Value:      value.OfUnit(float64(1), "point"),
		Source:     assertion.Source{Name: "wiki"},
		Provenance: assertion.Provenance{Artifact: "a.json", Anchor: "x", Revision: "revid:1", CapturedAt: time.Now()},
		Confidence: assertion.L1, Status: assertion.StatusPending,
	}
	if over != nil {
		over(&a)
	}
	return a
}

// 影响面必须体现在排序上：被派生引用的排在前面。
func TestQueueOrdersByImpact(t *testing.T) {
	src := &fakeSource{
		items: []assertion.Assertion{mk("low", nil), mk("high", nil)},
		refs:  map[string]int{"high": 5},
	}
	got, err := Queue(src, assertion.Filter{Status: "pending"}, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Assertion.ID != "high" {
		t.Fatalf("被引用多的应排前面，实际首项 %s", got[0].Assertion.ID)
	}
	if got[0].Priority.Tier != verification.TierImpact {
		t.Errorf("应归入影响面，实际 %s", got[0].Priority.Tier)
	}
}

// 强制抽检项必须进入队列，即使分数低。
func TestQueueKeepsSampledItems(t *testing.T) {
	var items []assertion.Assertion
	for i := 0; i < 20; i++ {
		id := string(rune('a' + i))
		items = append(items, mk(id, func(a *assertion.Assertion) { a.ID = id }))
	}
	src := &fakeSource{items: items, refs: map[string]int{}}
	// 高比例抽检，保证一定能看到
	got, err := Queue(src, assertion.Filter{Status: "pending"}, 5, 1.0, 7)
	if err != nil {
		t.Fatal(err)
	}
	sampled := 0
	for _, it := range got {
		if it.Priority.Sampled {
			sampled++
		}
	}
	if sampled == 0 {
		t.Error("抽检项必须出现在队列里")
	}
}

func TestQueueEmpty(t *testing.T) {
	got, err := Queue(&fakeSource{}, assertion.Filter{}, 10, 0.1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("无候选时应返回空，实际 %d", len(got))
	}
}

// 冲突只按组摆出，系统不裁决。
func TestConflictsGroupsWithoutAdjudicating(t *testing.T) {
	a := mk("a", nil)
	b := mk("b", func(x *assertion.Assertion) { x.Value = value.OfUnit(float64(2), "point") })
	src := &fakeSource{groups: [][]assertion.Assertion{{a, b}}}
	got, err := Conflicts(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Claims) != 2 {
		t.Fatalf("应返回 1 组 2 条说法，实际 %+v", got)
	}
	if got[0].Subject != "262" || got[0].Predicate != "atk" {
		t.Errorf("分组应带上身份，实际 %s.%s", got[0].Subject, got[0].Predicate)
	}
}

// 预览默认只看待核验的，避免重复核验已处理项。
func TestPreviewDefaultsToPending(t *testing.T) {
	src := &fakeSource{}
	if _, err := Preview(src, assertion.Filter{Entity: "shikigami"}); err != nil {
		t.Fatal(err)
	}
	if src.last.Status != string(assertion.StatusPending) {
		t.Errorf("未指定状态时应默认 pending，实际 %q", src.last.Status)
	}
}

// 批量核验：缺批准者或缺理由都必须被拒绝，且一条都不写。
func TestBatchRequiresHumanAndReason(t *testing.T) {
	src := &fakeSource{items: []assertion.Assertion{mk("a", nil)}}

	for _, in := range []BatchInput{
		{Decision: verification.Approved, Reason: "r"}, // 缺批准者
		{Decision: verification.Approved, By: "yicat"}, // 缺理由
		{Decision: "maybe", By: "yicat", Reason: "r"},  // 未知结论
	} {
		w := &fakeWriter{}
		if _, err := Batch(src, w, in, time.Now()); err == nil {
			t.Errorf("非法输入必须被拒绝：%+v", in)
		}
		if len(w.recs) != 0 {
			t.Errorf("被拒的批量不得写入任何记录，实际 %d 条", len(w.recs))
		}
	}
}

// 每条断言各自留痕，审计轨迹不合并——批量不等于免责。
func TestBatchWritesOneRecordPerAssertion(t *testing.T) {
	src := &fakeSource{items: []assertion.Assertion{mk("a", nil), mk("b", nil), mk("c", nil)}}
	w := &fakeWriter{}
	res, err := Batch(src, w, BatchInput{
		Decision: verification.Approved,
		Method:   verification.Editorial,
		By:       "yicat",
		Reason:   "与原件逐字比对一致",
		Proposed: "ingest-pipeline",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 3 || res.Failed != 0 {
		t.Errorf("应成功 3 条，实际 成功 %d 失败 %d", res.Applied, res.Failed)
	}
	if len(w.recs) != 3 {
		t.Fatalf("每条断言应各有一条记录，实际 %d", len(w.recs))
	}
	for _, r := range w.recs {
		if r.ApprovedBy == nil || !r.ApprovedBy.IsHuman() {
			t.Error("批准者必须是人")
		}
		if r.ProposedBy.Kind != verification.Agent {
			t.Error("提出者应为 agent——人批准的是 agent 提出的东西")
		}
		if r.Reason == "" {
			t.Error("每条记录都必须带理由")
		}
	}
}

// 部分失败不中断整批，但要如实报告。
func TestBatchReportsPartialFailure(t *testing.T) {
	src := &fakeSource{items: []assertion.Assertion{mk("a", nil), mk("b", nil), mk("c", nil)}}
	w := &fakeWriter{failOn: "b"}
	res, err := Batch(src, w, BatchInput{
		Decision: verification.Approved, Method: verification.Editorial,
		By: "yicat", Reason: "r",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 2 || res.Failed != 1 {
		t.Errorf("应为 成功 2 失败 1，实际 成功 %d 失败 %d", res.Applied, res.Failed)
	}
	if res.FirstErr == nil {
		t.Error("部分失败必须报告首个错误")
	}
}
