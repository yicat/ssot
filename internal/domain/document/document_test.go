package document

import (
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/verification"
)

var (
	captured = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	now      = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	human    = verification.Actor{Kind: verification.Human, ID: "ngnl5"}
)

func mirror(rev, hash string, at time.Time) Doc {
	d, err := New(Input{
		Source: "huijiwiki", Title: "Data:Character/262.json",
		Kind: KindEvidence, Revision: rev, ContentHash: hash,
		ArtifactPath: ".huiji/raw/Data_Character_262.json", CapturedAt: captured,
	}, at)
	if err != nil {
		panic(err)
	}
	return d
}

func written(body string, at time.Time) Doc {
	d, err := New(Input{
		Source: "内部", Title: "防御减免为何无权威来源",
		Kind: KindExplanation, Revision: "v1", Body: body, CapturedAt: captured,
	}, at)
	if err != nil {
		panic(err)
	}
	return d
}

// ── 登记 ────────────────────────────────────────────────────────────────────

// 验收：自撰文档必须有正文；镜像文档可以只给归档路径。
func TestEmptyDocumentIsRejected(t *testing.T) {
	_, err := New(Input{
		Source: "内部", Title: "空的", Kind: KindExplanation,
		Revision: "v1", CapturedAt: captured,
	}, now)
	if err == nil {
		t.Fatal("既没正文也没归档路径的文档必须被拒绝——空文档不是依据")
	}
	if !strings.Contains(err.Error(), "依据") {
		t.Errorf("报错应说清原因，实际：%v", err)
	}
}

func TestMirrorDocumentNeedsNoBody(t *testing.T) {
	d := mirror("revid:8053", "sha256:abc", now)
	if d.Body != "" {
		t.Error("镜像文档不必存正文")
	}
	if d.ArtifactPath == "" {
		t.Error("镜像文档必须有归档路径")
	}
}

// 验收：摘要由内容算出，不采信来源声明。
func TestHashComesFromContent(t *testing.T) {
	a := written("防御减免没有权威来源", now)
	b := written("防御减免有权威来源", now)
	if a.Hash == b.Hash {
		t.Fatal("内容不同，摘要必须不同")
	}
	if a.Hash == "" || algoOf(a.Hash) != "sha256" {
		t.Errorf("摘要应带算法前缀，实际 %q", a.Hash)
	}
	// 同一内容两次登记得到同一摘要
	if a.Hash != written("防御减免没有权威来源", now).Hash {
		t.Error("同一内容必须得到同一摘要")
	}
}

func TestDocumentRequiresSourceRevisionAndTime(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"缺来源", Input{Title: "t", Kind: KindRule, Revision: "v1", Body: "x", CapturedAt: captured}, "来源"},
		{"缺标题", Input{Source: "s", Kind: KindRule, Revision: "v1", Body: "x", CapturedAt: captured}, "标题"},
		{"缺修订", Input{Source: "s", Title: "t", Kind: KindRule, Body: "x", CapturedAt: captured}, "修订"},
		{"缺采集时间", Input{Source: "s", Title: "t", Kind: KindRule, Revision: "v1", Body: "x"}, "采集时间"},
	}
	for _, c := range cases {
		if _, err := New(c.in, now); err == nil {
			t.Errorf("%s 必须被拒绝", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s 的报错应提到 %q，实际：%v", c.name, c.want, err)
		}
	}
}

// 验收：文档身份跨修订稳定。
func TestIdentityIsStableAcrossRevisions(t *testing.T) {
	a := mirror("revid:1", "sha256:a", now)
	b := mirror("revid:2", "sha256:b", now)
	if a.ID != b.ID {
		t.Error("同一来源同一标题换修订时，文档身份必须不变")
	}
	if a.RevID == b.RevID {
		t.Error("不同修订必须有不同的记录标识")
	}
}

// ── 只追加 ──────────────────────────────────────────────────────────────────

// 验收：换修订时旧修订保留，且标为已被取代。
func TestApplyKeepsOldRevision(t *testing.T) {
	old := mirror("revid:8053", "sha256:old", now)
	next := mirror("revid:8112", "sha256:new", now.Add(time.Hour))

	history, ch, err := Apply([]Doc{old}, next)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Changed {
		t.Fatal("修订变了，Changed 应为 true")
	}
	if ch.Previous == nil || ch.Previous.RevID != old.RevID {
		t.Error("变更必须带上被取代的那份修订")
	}
	if len(history) != 2 {
		t.Fatalf("旧修订必须保留，实际 %d 份", len(history))
	}
	var kept Doc
	for _, h := range history {
		if h.RevID == old.RevID {
			kept = h
		}
	}
	if kept.Status != StatusSuperseded {
		t.Errorf("旧修订应标为已被取代，实际 %s", kept.Status)
	}
	if kept.SupersededBy != next.RevID {
		t.Error("旧修订应指向取代它的那份")
	}
	if cur := Current(history); cur == nil || cur.RevID != next.RevID {
		t.Error("当前修订应是最新那份")
	}
}

// 验收：同一份修订重复登记是幂等的。
func TestApplyIsIdempotent(t *testing.T) {
	old := mirror("revid:8053", "sha256:same", now)
	history, ch, err := Apply([]Doc{old}, mirror("revid:8053", "sha256:same", now.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if ch.Changed {
		t.Error("同一修订重复登记不该产生变更")
	}
	if len(history) != 1 {
		t.Errorf("历史不该增长，实际 %d 份", len(history))
	}
}

// 验收：修订标识没变但内容变了，同样算变更——**不能只信修订标识**。
func TestContentChangeWithSameRevisionCounts(t *testing.T) {
	old := mirror("revid:8053", "sha256:old", now)
	_, ch, err := Apply([]Doc{old}, mirror("revid:8053", "sha256:different", now.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Changed {
		t.Error("修订标识没变但内容变了，必须算变更——来源可以忘记更新修订号")
	}
}

// 验收：修订回退被识别出来。
func TestRevisionRevertIsDetected(t *testing.T) {
	first := mirror("revid:1", "sha256:a", now)
	second := mirror("revid:2", "sha256:b", now.Add(time.Hour))
	history, _, err := Apply([]Doc{first}, second)
	if err != nil {
		t.Fatal(err)
	}
	// 回退到 revid:1，但内容与当初不同（来源出过问题）
	back := mirror("revid:1", "sha256:c", now.Add(2*time.Hour))
	_, ch, err := Apply(history, back)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Reverted {
		t.Error("修订标识回退到旧值必须被标注——这通常意味着来源出过问题")
	}
}

// ── 核验 ────────────────────────────────────────────────────────────────────

// 验收：文档核验人必须是人，理由必填。
func TestVerifyRequiresHumanAndReason(t *testing.T) {
	d := written("防御减免没有权威来源", now)
	if _, err := d.Verify(verification.Actor{Kind: verification.Agent, ID: "dsh"},
		verification.Editorial, "r"); err == nil {
		t.Error("agent 核验文档必须被拒绝")
	}
	if _, err := d.Verify(human, verification.Editorial, ""); err == nil {
		t.Error("缺理由必须被拒绝")
	}
	if _, err := d.Verify(verification.Actor{}, verification.Editorial, "r"); err == nil {
		t.Error("缺核验人必须被拒绝")
	}
}

// 验收：文档核验方法只能是编审——一个人读过不构成多源。
func TestDocumentCannotClaimCrossSource(t *testing.T) {
	d := written("防御减免没有权威来源", now)
	_, err := d.Verify(human, verification.CrossSource, "三份攻略都这么说")
	if err == nil {
		t.Fatal("一份文档不得记成多源")
	}
	if !strings.Contains(err.Error(), "多源") {
		t.Errorf("报错应说清原因，实际：%v", err)
	}
	if _, err := d.Verify(human, verification.Measurement, "我量过"); err == nil {
		t.Error("文档核验不得记为实测")
	}
}

func TestVerifyMakesVerified(t *testing.T) {
	d := written("防御减免没有权威来源", now)
	got, err := d.Verify(human, verification.Editorial, "通读一遍，与游戏内一致")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusVerified {
		t.Errorf("状态应为已核验，实际 %s", got.Status)
	}
	if got.VerifiedBy == nil || got.VerifiedBy.ID != "ngnl5" {
		t.Error("核验人应被记录")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("核验后的不变量应自洽：%v", err)
	}
}

// 已被取代的修订不该被核验：核验应针对当前修订。
func TestCannotVerifySuperseded(t *testing.T) {
	old := mirror("revid:1", "sha256:a", now)
	history, _, _ := Apply([]Doc{old}, mirror("revid:2", "sha256:b", now.Add(time.Hour)))
	var superseded Doc
	for _, h := range history {
		if h.Status == StatusSuperseded {
			superseded = h
		}
	}
	if _, err := superseded.Verify(human, verification.Editorial, "r"); err == nil {
		t.Error("已被取代的修订不得被核验")
	}
}

func TestRejectKeepsDocument(t *testing.T) {
	d := written("防御减免没有权威来源", now)
	got, err := d.Reject(human, "来源不可靠")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRejected {
		t.Errorf("状态应为已驳回，实际 %s", got.Status)
	}
	if got.Title != d.Title || got.Body != d.Body {
		t.Error("驳回不得改动内容")
	}
	if _, err := got.Verify(human, verification.Editorial, "算了还是收下"); err == nil {
		t.Error("已驳回的文档不得再被核验")
	}
}

// ── 适用范围 ────────────────────────────────────────────────────────────────

// 验收：未声明适用范围 = 全项目适用，而不是哪儿都不适用。
func TestEmptyAppliesMeansProjectWide(t *testing.T) {
	a := AppliesTo{}
	if !a.AppliesToScenario("任意场景") {
		t.Error("未声明适用范围应表示全项目适用")
	}
	b := AppliesTo{Scenarios: []string{"damage-calc"}}
	if !b.AppliesToScenario("damage-calc") {
		t.Error("声明了的场景应适用")
	}
	if b.AppliesToScenario("别的场景") {
		t.Error("没声明的场景不适用")
	}
}

// ── 变更的判据 ──────────────────────────────────────────────────────────────

// 摘要算法不同时不下「内容一样」的结论：宁可多核验一次。
func TestDifferentAlgoIsNotSameContent(t *testing.T) {
	a := mirror("revid:1", "sha256:abc", now)
	b := mirror("revid:1", "md5:abc", now)
	if a.SameContent(b) {
		t.Error("摘要算法不同不得判为内容相同")
	}
}

// Current 跳过已被取代的，Latest 不跳。
func TestCurrentVsLatest(t *testing.T) {
	old := mirror("revid:1", "sha256:a", now)
	history, _, _ := Apply([]Doc{old}, mirror("revid:2", "sha256:b", now.Add(time.Hour)))
	if c := Current(history); c == nil || c.Revision != "revid:2" {
		t.Error("Current 应是生效的那份")
	}
	if l := Latest(history); l == nil || l.Revision != "revid:2" {
		t.Error("Latest 应是最晚登记的那份")
	}
}
