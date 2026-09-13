package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"
	"github.com/ngnl5/ssot/internal/infrastructure/store"
)

// 造一个最小项目：schema 合法、库里有几条待核验断言。
// 不依赖真实的 projects/onmyoji，避免测试与真实数据耦合。
func tempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("units.yml", "units: []\n")
	write("schema/x.schema.yml", `
entity: shikigami
metamodelVersion: 1
fields:
  - key: id
    type: number
    unit: point
    identity: true
    required: true
  - key: atk
    type: number
    unit: point
    required: true
`)

	st, err := store.Open(filepath.Join(dir, ".data", "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	mk := func(subject, pred string, v value.Value) assertion.Assertion {
		return assertion.Assertion{
			ID: "id-" + subject + "-" + pred, Entity: "shikigami", Subject: subject, Predicate: pred,
			Value:  v,
			Source: assertion.Source{Name: "test", Tier: "semi-official"},
			Provenance: assertion.Provenance{
				Artifact: "a.json", Anchor: "x", Revision: "revid:1", CapturedAt: time.Now(),
			},
			Confidence: assertion.L1, Status: assertion.StatusPending,
		}
	}
	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{
		mk("262", "id", value.OfUnit(float64(262), "point")),
		mk("262", "atk", value.OfUnit(float64(3082), "point")),
	}}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	return dir
}

// newService 构造服务并在测试结束时释放。
//
// 服务惰性持有打开的库，不释放会导致 t.TempDir 清理失败
// （Windows 上文件被占用）——这个失败本身就是个提醒：
// 资源必须有明确的释放点。
func newService(t *testing.T) *ReviewService {
	t.Helper()
	svc := NewReviewService(tempProject(t))
	t.Cleanup(func() { _ = svc.close() })
	return svc
}

func TestServicePendingAndStats(t *testing.T) {
	svc := newService(t)

	items, err := svc.Pending("", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("应有 2 条待核验，实际 %d", len(items))
	}
	if items[0].Artifact == "" || items[0].Anchor == "" || items[0].Revision == "" {
		t.Errorf("队列项必须带溯源，实际 %+v", items[0])
	}

	st, err := svc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 2 || st.ByStatus["pending"] != 2 {
		t.Errorf("统计不正确：%+v", st)
	}
}

// 批准必须是人，且必须给理由 —— 这条规则在领域层，服务层只转发。
func TestServiceApproveRequiresHumanAndReason(t *testing.T) {
	svc := newService(t)

	if err := svc.Approve("id-262-atk", "", "editorial", "理由", ""); err == nil {
		t.Error("缺批准者必须被拒绝")
	}
	if err := svc.Approve("id-262-atk", "yicat", "editorial", "", ""); err == nil {
		t.Error("缺理由必须被拒绝")
	}
	if err := svc.Approve("id-262-atk", "yicat", "measurement", "我打过", ""); err == nil {
		t.Error("实测无依据必须被拒绝")
	}
	if err := svc.Approve("不存在", "yicat", "editorial", "理由", ""); err == nil {
		t.Error("核验不存在的断言必须被拒绝")
	}
}

func TestServiceApproveAndHistory(t *testing.T) {
	svc := newService(t)

	if err := svc.Approve("id-262-atk", "yicat", "editorial", "与原文逐字比对一致", ""); err != nil {
		t.Fatal(err)
	}

	hist, err := svc.History("id-262-atk")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("应有 1 条核验记录，实际 %d", len(hist))
	}
	h := hist[0]
	if h.ApprovedBy != "human:yicat" {
		t.Errorf("必须记录批准者，实际 %q", h.ApprovedBy)
	}
	if h.ProposedBy != "agent:ingest-pipeline" {
		t.Errorf("必须记录提出者（与批准者分别标识），实际 %q", h.ProposedBy)
	}
	if h.Reason == "" || h.At == "" {
		t.Errorf("必须记录理由与时间，实际 %+v", h)
	}

	// 批准后应离开待核验队列
	pending, _ := svc.Pending("", "", 0)
	if len(pending) != 1 {
		t.Errorf("队列应只剩 1 条，实际 %d", len(pending))
	}
	verified, _ := svc.Pending("", "verified", 0)
	if len(verified) != 1 {
		t.Errorf("已核验应有 1 条，实际 %d", len(verified))
	}
}

func TestServiceReject(t *testing.T) {
	svc := newService(t)
	if err := svc.Reject("id-262-id", "yicat", "取值可疑"); err != nil {
		t.Fatal(err)
	}
	rejected, err := svc.Pending("", "rejected", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 1 {
		t.Fatalf("应有 1 条已驳回，实际 %d", len(rejected))
	}
	if rejected[0].Status != string(assertion.StatusRejected) {
		t.Errorf("状态应为 rejected，实际 %s", rejected[0].Status)
	}
}

// 项目加载失败必须如实报错，而不是返回空队列让人误以为「没有待核验」。
func TestServiceReportsBrokenProject(t *testing.T) {
	svc := NewReviewService(filepath.Join(t.TempDir(), "not-a-project"))
	if _, err := svc.Pending("", "", 0); err == nil {
		t.Error("项目不可加载时必须报错，不得静默返回空队列")
	}
}

func TestServiceProjectDir(t *testing.T) {
	dir := tempProject(t)
	if got := NewReviewService(dir).ProjectDir(); got != dir {
		t.Errorf("ProjectDir 应返回构造时的目录，实际 %q", got)
	}
}

// 批准者必须是 agent 之外的实体 —— 由领域层拒绝，服务层不得绕过。
func TestServiceCannotApproveAsAgent(t *testing.T) {
	dir := tempProject(t)
	st, err := store.Open(filepath.Join(dir, ".data", "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	err = st.Verify(verification.Record{
		AssertionID: "id-262-atk",
		Decision:    verification.Approved,
		Method:      verification.Editorial,
		ProposedBy:  verification.Actor{Kind: verification.Agent, ID: "a"},
		ApprovedBy:  &verification.Actor{Kind: verification.Agent, ID: "b"},
		Reason:      "r",
		At:          time.Now(),
	})
	if err == nil {
		t.Error("agent 作为批准者必须被拒绝")
	}
}
