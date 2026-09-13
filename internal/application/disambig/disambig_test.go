package disambig

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/application/admit"
	"github.com/ngnl5/ssot/internal/application/ingest"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/decision"
	"github.com/ngnl5/ssot/internal/domain/metamodel"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

var at = time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func testSchema(t *testing.T) *schema.Set {
	t.Helper()
	e := &metamodel.Entity{
		Name:        "skill",
		MetaVersion: metamodel.Version,
		Fields: []metamodel.Field{
			{Key: "id", Type: metamodel.TypeText, Required: true, Identity: true},
			{Key: "ratio", Type: metamodel.TypeNumber, Unit: "percent"},
			{Key: "ratio_max", Type: metamodel.TypeNumber, Unit: "percent"},
		},
	}
	set, ps := schema.NewSet(e)
	if len(ps) > 0 {
		t.Fatalf("构造 schema 失败：%v", ps)
	}
	return set
}

func opts() admit.Options {
	return admit.Options{
		Entity: "skill", Source: assertion.Source{Name: "huijiwiki", Tier: "semi-official"},
		CapturedAt: at, Units: unit.Default(),
	}
}

// memPort 是内存端口。它按与存储层相同的语义（Refresh 合并 + 原子裁决）实现，
// 使应用层的编排逻辑可被独立测试。
type memPort struct {
	items      map[string]decision.Item
	assertions map[string]assertion.Assertion
	// written 记录**经由裁决**写进去的断言 ID。
	// 不能拿 assertions 的条数当依据：库里本来就有接入阶段写入的身份字段。
	written []string
	verifs  []verification.Record
	impact  map[string]int
}

func newPort() *memPort {
	return &memPort{
		items:      map[string]decision.Item{},
		assertions: map[string]assertion.Assertion{},
		impact:     map[string]int{},
	}
}

func (m *memPort) UpsertDecisions(items []decision.Item) (decision.Upsert, error) {
	var res decision.Upsert
	for _, in := range items {
		if err := in.Validate(); err != nil {
			return res, err
		}
		res.Seen++
		old, ok := m.items[in.ID]
		if !ok {
			m.items[in.ID] = in
			res.Created++
			continue
		}
		merged := old.Refresh(in.Candidates, in.Revision, in.Reason, in.Context)
		if merged.Status != old.Status {
			res.Reopened++
		}
		if sameItem(old, merged) {
			res.Unchanged++
			continue
		}
		m.items[in.ID] = merged
		res.Updated++
	}
	return res, nil
}

// sameItem 与存储层保持同一口径：候选与状态都没变才算「没变」。
func sameItem(a, b decision.Item) bool {
	aj, _ := json.Marshal(a.Candidates)
	bj, _ := json.Marshal(b.Candidates)
	return a.Status == b.Status && a.Revision == b.Revision &&
		a.Reason == b.Reason && string(aj) == string(bj)
}

func (m *memPort) Decisions(status string, limit int) ([]decision.Item, error) {
	var out []decision.Item
	for _, it := range m.items {
		if status == "" || status == string(decision.StatusOpen) {
			if !it.Status.NeedsAttention() {
				continue
			}
		} else if status != "all" && it.Status != decision.Status(status) {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

func (m *memPort) Decision(id string) (decision.Item, bool, error) {
	it, ok := m.items[id]
	return it, ok, nil
}

func (m *memPort) ResolveDecision(it decision.Item, cs assertion.ChangeSet, rec *verification.Record) error {
	if err := it.Validate(); err != nil {
		return err
	}
	// 模拟原子性：断言与核验记录一起成功，否则一起不写
	pending := make([]assertion.Assertion, 0, len(cs.Insert))
	for _, a := range cs.Insert {
		if err := a.Validate(); err != nil {
			return err
		}
		pending = append(pending, a)
	}
	if rec != nil {
		if err := rec.Validate(); err != nil {
			return err
		}
	}
	for _, a := range pending {
		// 库里已有的身份字段会被重新带上，但 INSERT OR IGNORE 不会重复写。
		if _, exists := m.assertions[a.ID]; !exists {
			m.written = append(m.written, a.ID)
		}
		m.assertions[a.ID] = a
	}
	if rec != nil {
		m.verifs = append(m.verifs, *rec)
	}
	m.items[it.ID] = it
	return nil
}

func (m *memPort) DeferDecision(it decision.Item) error {
	if err := it.Validate(); err != nil {
		return err
	}
	m.items[it.ID] = it
	return nil
}

func (m *memPort) SubjectImpact() (map[string]int, error) { return m.impact, nil }

func (m *memPort) KeysFor(string) (map[string][]string, error) { return map[string][]string{}, nil }

// BySubject 返回库里已有的断言。真实场景里必填字段（id、name、cost…）
// 早已入库，裁决时要把它们补回记录，否则准入会因为「记录不完整」整条拒绝。
func (m *memPort) BySubject(entity, subject string) ([]assertion.Assertion, error) {
	var out []assertion.Assertion
	for _, a := range m.assertions {
		if a.Entity == entity && a.Subject == subject {
			out = append(out, a)
		}
	}
	return out, nil
}

func ambig(subject string) ingest.Unresolved {
	mk := func(pct float64, anchor, ctx string) ingest.Candidate {
		return ingest.Candidate{
			Entity: "skill", Subject: subject, Predicate: "ratio",
			Value: value.OfUnit(pct, "percent"), Artifact: "Data:Character/x.json",
			Anchor: anchor, Context: ctx, Revision: "revid:1", Parsing: ingest.ParsingText,
		}
	}
	return ingest.Unresolved{
		Entity: "skill", Artifact: "Data:Character/x.json", Subject: subject,
		Predicate: "ratio", Revision: "revid:1", CapturedAt: at,
		Reason:  "描述中出现 2 个伤害倍率（33%, 88%），无法确定取哪一个",
		Context: "造成攻击33%伤害，若目标…则造成攻击88%伤害",
		Candidates: []ingest.Candidate{
			mk(33, "skills[0].description#m0", "造成攻击33%伤害"),
			mk(88, "skills[0].description#m1", "则造成攻击88%伤害"),
		},
	}
}

func human(id string) verification.Actor { return verification.Actor{Kind: verification.Human, ID: id} }

func register(t *testing.T, p *memPort, u ...ingest.Unresolved) {
	t.Helper()
	for _, x := range u {
		seedIdentity(t, p, x)
	}
	if _, err := Record(p, u, "huijiwiki"); err != nil {
		t.Fatalf("登记失败：%v", err)
	}
}

// seedIdentity 用**接入层自己的准入路径**把身份字段写进库。
//
// 不能手搓一个假 ID：断言 ID 是准入层按 (身份, 取值) 确定性算出来的，
// 手搓的 ID 与它不等，裁决时补进来的同一条字段就会被当成新断言重复写入——
// 那样测的是假象，不是行为。
func seedIdentity(t *testing.T, p *memPort, x ingest.Unresolved) {
	t.Helper()
	cs, _, err := admit.Run([]ingest.Candidate{{
		Entity: x.Entity, Subject: x.Subject, Predicate: "id",
		Value: value.Of(x.Subject), Artifact: x.Artifact, Anchor: "skills[0].id",
		Revision: x.Revision, Parsing: ingest.ParsingDirect,
	}}, testSchema(t), map[string][]string{}, opts())
	if err != nil {
		t.Fatalf("构造既有字段失败：%v", err)
	}
	if len(cs.Insert) != 1 {
		t.Fatalf("应有 1 条身份字段，实际 %d", len(cs.Insert))
	}
	p.assertions[cs.Insert[0].ID] = cs.Insert[0]
}

func onlyItem(t *testing.T, p *memPort, subject string) decision.Item {
	t.Helper()
	for _, it := range p.items {
		if it.Subject == subject {
			return it
		}
	}
	t.Fatalf("找不到主体 %s 的事项", subject)
	return decision.Item{}
}

// 验收：候选不足两个时登记被拒绝，而不是静默丢弃。
func TestRecordRejectsNonAmbiguity(t *testing.T) {
	p := newPort()
	u := ambig("s1")
	u.Candidates = u.Candidates[:1]
	if _, err := Record(p, []ingest.Unresolved{u}, "huijiwiki"); err == nil {
		t.Error("单候选的歧义必须被拒绝：单候选不是歧义")
	}
	if len(p.items) != 0 {
		t.Error("被拒绝的歧义不得留下事项")
	}
}

// 验收：重复接入同一批数据不改变已裁决事项的状态。
func TestRecordIsIdempotent(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")

	res, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 1, By: human("ngnl5"),
		Reason: "主伤害那一句取 88", Method: verification.Editorial,
	}, at)
	if err != nil {
		t.Fatalf("裁决失败：%v", err)
	}
	if res.AssertionID == "" {
		t.Error("裁决应当产出断言")
	}

	up, err := Record(p, []ingest.Unresolved{ambig("s1")}, "huijiwiki")
	if err != nil {
		t.Fatalf("重复接入失败：%v", err)
	}
	if up.Unchanged != 1 || up.Reopened != 0 {
		t.Errorf("重复接入应当完全没变，实际 %+v", up)
	}
	got := onlyItem(t, p, "s1")
	if got.Status != decision.StatusDecided {
		t.Errorf("重复接入不得把已裁决推回待办，实际 %s", got.Status)
	}
	if got.Resolution == nil || got.Resolution.AssertionID != res.AssertionID {
		t.Error("重复接入不得清掉裁决记录")
	}
	if len(p.written) != 1 {
		t.Errorf("重复接入不得重复写断言，实际 %d 条", len(p.written))
	}
}

// 验收：选中候选后经准入写入断言，分级不高于该候选解析方式的上限。
func TestResolveWritesAssertionThroughAdmission(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")

	res, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 0, By: human("ngnl5"),
		Reason: "第一句才是主伤害", Method: verification.Editorial,
	}, at)
	if err != nil {
		t.Fatalf("裁决失败：%v", err)
	}
	a, ok := p.assertions[res.AssertionID]
	if !ok {
		t.Fatalf("断言 %s 未写入", res.AssertionID)
	}
	if a.Confidence == assertion.L1 {
		t.Error("文本抽取的候选不得定为 L1")
	}
	if a.Confidence != assertion.L2 {
		t.Errorf("文本抽取应为 L2，实际 %s", a.Confidence)
	}
	if a.Predicate != "ratio" || a.Subject != "s1" {
		t.Errorf("断言的谓词与主体不对：%s.%s", a.Subject, a.Predicate)
	}
	if a.Provenance.Anchor != "skills[0].description#m0" {
		t.Errorf("断言应指向被选中候选的锚点，实际 %s", a.Provenance.Anchor)
	}
	if a.Provenance.Revision != "revid:1" {
		t.Errorf("断言应带上源修订标识，实际 %s", a.Provenance.Revision)
	}

	// 裁决必须同时留下核验记录——否则「谁、凭什么」就断了
	if len(p.verifs) != 1 {
		t.Fatalf("应恰好留下 1 条核验记录，实际 %d", len(p.verifs))
	}
	rec := p.verifs[0]
	if rec.AssertionID != res.AssertionID {
		t.Error("核验记录必须挂在产出的断言上")
	}
	if !rec.ApprovedBy.IsHuman() {
		t.Error("批准者必须是人")
	}
	if rec.Reason == "" {
		t.Error("核验记录必须有理由")
	}
}

// 验收：核验记录缺少理由时整笔被拒绝，事项保持待判定。
func TestResolveWithoutReasonChangesNothing(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")

	if _, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 0, By: human("ngnl5"), Method: verification.Editorial,
	}, at); err == nil {
		t.Fatal("没有理由的裁决必须被拒绝")
	}
	if len(p.written) != 0 || len(p.verifs) != 0 {
		t.Error("被拒绝的裁决不得留下断言或核验记录")
	}
	if got := onlyItem(t, p, "s1"); got.Status != decision.StatusOpen {
		t.Errorf("被拒绝的裁决不得改变事项状态，实际 %s", got.Status)
	}
}

// 验收：agent 无法裁决。
func TestAgentCannotResolve(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")
	_, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 0, By: verification.Actor{Kind: verification.Agent, ID: "bot"},
		Reason: "r", Method: verification.Editorial,
	}, at)
	if err == nil {
		t.Fatal("agent 裁决必须被拒绝")
	}
	if len(p.written) != 0 {
		t.Error("被拒绝的裁决不得留下断言")
	}
}

// 验收：选「都不对」时不产生断言，该谓词记为缺失且记录理由。
func TestResolveNoneWritesNoAssertion(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")

	res, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: decision.None, By: human("ngnl5"),
		Reason: "两处都是条件分支里的数值，主伤害那一段没写倍率",
		Method: verification.Editorial,
	}, at)
	if err != nil {
		t.Fatalf("判「都不对」应当被接受：%v", err)
	}
	if len(p.written) != 0 || len(p.verifs) != 0 {
		t.Error("判「都不对」时不得产生断言或核验记录")
	}
	if res.AssertionID != "" {
		t.Error("判「都不对」时不应返回断言 ID")
	}
	got := onlyItem(t, p, "s1")
	if got.Resolution == nil || !got.Resolution.IsNone() {
		t.Fatal("事项应记为已裁决且结论为「都不对」")
	}
	if got.Resolution.Reason == "" {
		t.Error("结论必须带理由——只有结论没有理由的不是裁决")
	}

	st, err := Summary(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Missing != 1 || st.Decided != 1 || st.Open != 0 {
		t.Errorf("统计应体现 1 条缺失、1 条已裁决，实际 %+v", st)
	}
}

// 验收：暂缓不产生断言，事项仍在待判定队列中。
func TestDeferKeepsItemInQueue(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")

	if _, err := Defer(p, it.ID, human("ngnl5"), "等下个版本再看", at); err != nil {
		t.Fatalf("暂缓失败：%v", err)
	}
	if len(p.written) != 0 {
		t.Error("暂缓不得产生断言")
	}
	q, err := Queue(p, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 {
		t.Fatalf("暂缓事项必须仍在队列里（那是未处理，不是结论），实际 %d 条", len(q))
	}
	if q[0].Item.Status != decision.StatusDeferred {
		t.Errorf("队列里应标为已暂缓，实际 %s", q[0].Item.Status)
	}
}

// 验收：选中的候选未通过准入时整笔被拒绝，事项保持待判定。
func TestResolveRejectedByAdmission(t *testing.T) {
	p := newPort()
	u := ambig("s1")
	// 把候选的谓词改成 schema 未声明的名字——准入应当拒绝，
	// 于是整笔裁决失败，而不是「先记为已裁决再报错」。
	it, err := FromUnresolved(u, "huijiwiki")
	if err != nil {
		t.Fatal(err)
	}
	it.Predicate = "damage_ratio_undefined"
	if err := it.Validate(); err != nil {
		t.Fatal(err)
	}
	p.items[it.ID] = it

	if _, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 0, By: human("ngnl5"), Reason: "r", Method: verification.Editorial,
	}, at); err == nil {
		t.Fatal("schema 未声明的谓词必须让整笔裁决失败")
	}
	if len(p.written) != 0 {
		t.Error("被准入拒绝的裁决不得留下断言")
	}
	if got := p.items[it.ID]; got.Status != decision.StatusOpen {
		t.Errorf("被拒绝的裁决不得改变事项状态，实际 %s", got.Status)
	}
}

// 验收：修订变化后事项重新打开；不再包含原选项时标记需复核。
func TestRecordReopensAndMarksStale(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")
	if _, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 1, By: human("ngnl5"), Reason: "取 88", Method: verification.Editorial,
	}, at); err != nil {
		t.Fatal(err)
	}

	// 新修订里 88% 不见了 —— 必须标记需复核，不得静默保留旧结论
	u := ambig("s1")
	u.Revision = "revid:2"
	u.Candidates[1].Value = value.OfUnit(120, "percent")
	u.Candidates[1].Revision = "revid:2"
	up, err := Record(p, []ingest.Unresolved{u}, "huijiwiki")
	if err != nil {
		t.Fatal(err)
	}
	if up.Reopened != 1 {
		t.Errorf("应有 1 项需复核，实际 %+v", up)
	}
	got := onlyItem(t, p, "s1")
	if got.Status != decision.StatusStale {
		t.Fatalf("原选项消失应标记需复核，实际 %s", got.Status)
	}
	// 需复核事项重新出现在队列里
	q, _ := Queue(p, "", 0)
	if len(q) != 1 || q[0].Item.Status != decision.StatusStale {
		t.Error("需复核事项必须回到需要人看的队列里")
	}
}

// 验收：未选中的候选在裁决后仍可查询到。
func TestCandidatesRetainedAfterResolve(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")
	if _, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 1, By: human("ngnl5"), Reason: "取 88", Method: verification.Editorial,
	}, at); err != nil {
		t.Fatal(err)
	}
	got := onlyItem(t, p, "s1")
	if len(got.Candidates) != 2 {
		t.Fatalf("裁决后候选不得被裁掉，实际 %d 个", len(got.Candidates))
	}
	if !strings.Contains(got.Candidates[0].Context, "33%") {
		t.Error("未选中的候选连同上下文都必须保留")
	}
}

// 验收：已裁决事项再次裁决被拒绝。
func TestResolveTwiceIsRejected(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"))
	it := onlyItem(t, p, "s1")
	if _, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 1, By: human("ngnl5"), Reason: "取 88", Method: verification.Editorial,
	}, at); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: it.ID, Choice: 0, By: human("ngnl5"), Reason: "改主意", Method: verification.Editorial,
	}, at)
	if !errors.Is(err, decision.ErrAlreadyDecided) {
		t.Errorf("二次裁决应被拒绝并说明不支持改判，实际 %v", err)
	}
	if len(p.written) != 1 {
		t.Errorf("二次裁决不得写入第二条断言，实际 %d", len(p.written))
	}
}

// 验收：队列按影响面排序，排在前面的理由必须可见。
func TestQueueOrdersByImpactWithVisibleReason(t *testing.T) {
	p := newPort()
	register(t, p, ambig("s1"), ambig("s2"), ambig("s3"))
	p.impact["skill|s2"] = 42

	q, err := Queue(p, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 3 {
		t.Fatalf("应有 3 条，实际 %d", len(q))
	}
	if q[0].Item.Subject != "s2" {
		t.Errorf("影响面最大者应排最前，实际 %s", q[0].Item.Subject)
	}
	if !strings.Contains(q[0].Reason, "42") {
		t.Errorf("排序理由必须可见，实际 %q", q[0].Reason)
	}
	if q[0].Impact != 42 {
		t.Errorf("影响面应随条目返回，实际 %d", q[0].Impact)
	}
}

// 验收：事项不存在时报错，而不是静默成功。
func TestResolveUnknownItem(t *testing.T) {
	p := newPort()
	if _, err := Resolve(p, testSchema(t), opts(), ResolveInput{
		ID: "nope", Choice: 0, By: human("h"), Reason: "r", Method: verification.Editorial,
	}, at); err == nil {
		t.Error("不存在的事项必须报错")
	}
	if _, err := Defer(p, "nope", human("h"), "r", at); err == nil {
		t.Error("不存在的事项暂缓必须报错")
	}
}
