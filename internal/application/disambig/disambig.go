// Package disambig 是「待判定」的用例编排。
//
// 见 docs/specs/decision.spec.md。它把三件事从界面里抽出来，使 CLI 与 GUI
// 走同一套逻辑：
//
//	登记 —— 接入发现的歧义落库成为待办
//	排队 —— 按影响面排序，排在前面的理由必须可见
//	裁决 —— 人选中一个候选，经**与普通候选完全相同**的准入路径写入断言
//
// 规则本身在 domain/decision 里（纯函数、可测）；本包只负责取数与编排，
// 数据来源由调用方以端口注入。
package disambig

import (
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/application/admit"
	"github.com/ngnl5/ssot/internal/application/ingest"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/decision"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Port 是待判定所需的读写端口。
type Port interface {
	UpsertDecisions([]decision.Item) (decision.Upsert, error)
	Decisions(status string, limit int) ([]decision.Item, error)
	Decision(id string) (decision.Item, bool, error)
	ResolveDecision(decision.Item, assertion.ChangeSet, *verification.Record) error
	DeferDecision(decision.Item) error
	SubjectImpact() (map[string]int, error)
	KeysFor(entity string) (map[string][]string, error)
	BySubject(entity, subject string) ([]assertion.Assertion, error)
}

// Record 把接入层发现的歧义登记为待判定事项。
//
// 重复接入必须幂等：修订与候选都没变时状态一律不动，
// 否则每次同步都会把人已经裁决过的结论推回待办。
func Record(p Port, unresolved []ingest.Unresolved, source string) (decision.Upsert, error) {
	if len(unresolved) == 0 {
		return decision.Upsert{}, nil
	}
	items := make([]decision.Item, 0, len(unresolved))
	for _, u := range unresolved {
		it, err := FromUnresolved(u, source)
		if err != nil {
			return decision.Upsert{}, err
		}
		items = append(items, it)
	}
	return p.UpsertDecisions(items)
}

// FromUnresolved 把一条接入歧义转成待判定事项。
//
// 候选不足两个时返回错误而不是静默丢弃——那说明抽取器与本层的约定被破坏了，
// 必须报出来，不能让人以为「没有待判定事项」等于「没有歧义」。
func FromUnresolved(u ingest.Unresolved, source string) (decision.Item, error) {
	cands := make([]decision.Candidate, 0, len(u.Candidates))
	for _, c := range u.Candidates {
		cands = append(cands, decision.Candidate{
			Value:   c.Value,
			Anchor:  c.Anchor,
			Context: c.Context,
			Parsing: c.Parsing,
		})
	}
	it, err := decision.New(u.Entity, u.Subject, u.Predicate, u.Reason, u.Context,
		u.Artifact, u.Revision, source, u.CapturedAt, cands)
	if err != nil {
		return decision.Item{}, fmt.Errorf("歧义 %s.%s：%w", u.Subject, u.Predicate, err)
	}
	return it, nil
}

// Entry 是队列里的一条待判定事项。
type Entry struct {
	Item   decision.Item
	Impact int
	Reason string
}

// Queue 返回按影响面排序的待判定队列。
//
// status 为空表示「还需要人看的」：待判定 + 已暂缓 + 需复核。
// 暂缓不是结论，因此**必须留在队列里**，只是换个标记。
func Queue(p Port, status string, limit int) ([]Entry, error) {
	items, err := p.Decisions(status, 0)
	if err != nil {
		return nil, err
	}
	impact, err := p.SubjectImpact()
	if err != nil {
		return nil, err
	}
	ordered := decision.Order(items, impact, limit)
	out := make([]Entry, 0, len(ordered))
	for _, it := range ordered {
		out = append(out, Entry{
			Item:   it,
			Impact: decision.ImpactOf(it, impact),
			Reason: decision.ImpactReason(it, impact),
		})
	}
	return out, nil
}

// Stats 是待判定的整体状态。
type Stats struct {
	Total    int
	Open     int
	Deferred int
	Decided  int
	Missing  int // 判为「都不对」——谓词据此记为缺失
	Stale    int
}

// Summary 统计各状态。
func Summary(p Port) (Stats, error) {
	items, err := p.Decisions("all", 0)
	if err != nil {
		return Stats{}, err
	}
	var st Stats
	for _, it := range items {
		st.Total++
		switch it.Status {
		case decision.StatusOpen:
			st.Open++
		case decision.StatusDeferred:
			st.Deferred++
		case decision.StatusStale:
			st.Stale++
		case decision.StatusDecided:
			st.Decided++
		}
		if it.Resolution != nil && it.Resolution.IsNone() {
			st.Missing++
		}
	}
	return st, nil
}

// ResolveInput 是一次裁决的输入。
type ResolveInput struct {
	ID     string
	Choice int // 候选序号；decision.None(-1) 表示「都不对」
	By     verification.Actor
	Method verification.Method
	Reason string
	// Evidence 是依据。以「实测」裁决时必填。
	Evidence string
	// Proposed 是提出候选的一方（agent），用于核验记录的追责。
	Proposed verification.Actor
}

// ResolveResult 是一次裁决的结果。
type ResolveResult struct {
	Item        decision.Item
	AssertionID string
	Message     string
}

// ErrConflict 表示选中的取值与库中已有断言冲突。
//
// 它被单独区分出来，是因为处理方式不同：冲突要人先去冲突页裁决，
// 而不是在这条流程里被顺手覆盖掉。
type ErrConflict struct{ Detail string }

func (e ErrConflict) Error() string {
	return "选中的取值与已有断言冲突：" + e.Detail +
		"——请先在冲突页裁决，系统不替你在两处之间做选择"
}

// Resolve 执行一次裁决。
//
// 选中候选时走的是**与普通接入完全相同的准入路径**：同样的 schema 校验、
// 量纲校验、唯一性与引用检查、冲突检测。差别只在最后的核验状态——
// 人已经看过原文并做出判断，因此可以直接记为已核验。
//
// 任何一步失败则整笔不做，事项保持原状态：不得「先记为已裁决再报错」。
func Resolve(p Port, set *schema.Set, opts admit.Options, in ResolveInput, now time.Time) (ResolveResult, error) {
	it, ok, err := p.Decision(in.ID)
	if err != nil {
		return ResolveResult{}, err
	}
	if !ok {
		return ResolveResult{}, fmt.Errorf("待判定事项 %s 不存在", in.ID)
	}
	if in.Proposed.ID == "" {
		in.Proposed = verification.Actor{Kind: verification.Agent, ID: "ingest-pipeline"}
	}
	if in.Method == "" {
		in.Method = verification.Editorial
	}
	// 准入上下文以**事项自身**为准：裁决不是一次新采集，
	// 实体、来源、采集时间都必须沿用原件，否则溯源会指向错误的时间点。
	opts.Entity = it.Entity
	opts.CapturedAt = it.CapturedAt
	if it.Source != "" {
		opts.Source.Name = it.Source
	}

	r := decision.Resolution{
		Choice:   in.Choice,
		By:       in.By,
		Reason:   in.Reason,
		Method:   in.Method,
		Evidence: in.Evidence,
		At:       now,
	}
	if in.Choice != decision.None && in.Choice >= 0 && in.Choice < len(it.Candidates) {
		r.ChosenValue = it.Candidates[in.Choice].Value
	}

	var (
		next decision.Item
		cs   assertion.ChangeSet
		rec  *verification.Record
	)
	if it.Status == decision.StatusStale {
		next, err = it.ResolveStale(r)
	} else {
		next, err = it.Resolve(r)
	}
	if err != nil {
		return ResolveResult{}, err
	}

	if in.Choice >= 0 {
		c := it.Candidates[in.Choice]
		existing, err := p.KeysFor(it.Entity)
		if err != nil {
			return ResolveResult{}, err
		}
		cands, err := withRequiredFields(p, set, it, c)
		if err != nil {
			return ResolveResult{}, err
		}
		var rep admit.Report
		// 注意用 = 而不是 := ——这里必须写进上面那个 cs，
		// 否则裁决记录指向的断言不会随事务一起落库。
		cs, rep, err = admit.Run(cands, set, existing, opts)
		if err != nil {
			return ResolveResult{}, err
		}
		chosen := pick(cs.Insert, it.Predicate)
		if chosen == nil {
			return ResolveResult{}, fmt.Errorf("选中的候选未通过准入：%s", problemText(rep))
		}
		if len(cs.Conflict) > 0 {
			return ResolveResult{}, ErrConflict{Detail: fmt.Sprintf("断言 %s", cs.Conflict[0])}
		}
		if err := next.BindAssertion(chosen.ID); err != nil {
			return ResolveResult{}, err
		}
		rec = &verification.Record{
			AssertionID: chosen.ID,
			Decision:    verification.Approved,
			Method:      r.Method,
			ProposedBy:  in.Proposed,
			ApprovedBy:  &r.By,
			Reason:      r.Reason,
			Evidence:    r.Evidence,
			At:          now,
		}
	}

	if err := p.ResolveDecision(next, cs, rec); err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{Item: next, AssertionID: assertionIDOf(rec), Message: messageFor(next)}, nil
}

// Defer 暂缓一条事项。不产生断言。
func Defer(p Port, id string, by verification.Actor, reason string, now time.Time) (decision.Item, error) {
	it, ok, err := p.Decision(id)
	if err != nil {
		return decision.Item{}, err
	}
	if !ok {
		return decision.Item{}, fmt.Errorf("待判定事项 %s 不存在", id)
	}
	next, err := it.Defer(decision.Deferral{By: by, Reason: reason, At: now})
	if err != nil {
		return decision.Item{}, err
	}
	if err := p.DeferDecision(next); err != nil {
		return decision.Item{}, err
	}
	return next, nil
}

func toCandidate(it decision.Item, c decision.Candidate) ingest.Candidate {
	return ingest.Candidate{
		Entity:    it.Entity,
		Subject:   it.Subject,
		Predicate: it.Predicate,
		Value:     c.Value,
		Artifact:  it.Artifact,
		Anchor:    c.Anchor,
		Context:   c.Context,
		Revision:  it.Revision,
		Parsing:   c.Parsing,
	}
}

// withRequiredFields 把被裁决的候选补成一条**完整的记录**再送进准入。
//
// 准入校验的是记录而不是单个字段（见 metamodel.spec.md）：一条 ratio 候选
// 会因为「同一记录里的 id 不在这次候选里」被整体拒绝。而 id、name、cost
// 这些必填字段本来就已经在库里了，把它们的现有取值一并带上才是如实申报。
//
// 只补**必填**字段：非必填的一并带上会让准入重新校验一堆与本次裁决无关的东西，
// 那些字段将来若失效，会把一条本来成立的裁决一起拖垮。
func withRequiredFields(p Port, set *schema.Set, it decision.Item, c decision.Candidate) ([]ingest.Candidate, error) {
	out := []ingest.Candidate{toCandidate(it, c)}
	e, ok := set.Lookup(it.Entity)
	if !ok {
		return nil, fmt.Errorf("schema 中没有实体 %q", it.Entity)
	}
	have, err := p.BySubject(it.Entity, it.Subject)
	if err != nil {
		return nil, err
	}
	for _, a := range have {
		if a.Predicate == it.Predicate {
			continue // 本次要裁决的字段以候选为准
		}
		f, ok := e.FieldByKey(a.Predicate)
		if !ok || !f.Required {
			continue
		}
		out = append(out, ingest.Candidate{
			Entity:    a.Entity,
			Subject:   a.Subject,
			Predicate: a.Predicate,
			Value:     a.Value,
			Artifact:  a.Provenance.Artifact,
			Anchor:    a.Provenance.Anchor,
			Revision:  a.Provenance.Revision,
			Parsing:   assertion.ParsingDirect, // 库里已有的直取字段，保持其原分级口径
		})
	}
	return out, nil
}

// pick 从准入产出的断言里取出本次裁决的那一条。
func pick(as []assertion.Assertion, predicate string) *assertion.Assertion {
	for i := range as {
		if as[i].Predicate == predicate {
			return &as[i]
		}
	}
	return nil
}

func problemText(rep admit.Report) string {
	if len(rep.Problems) == 0 {
		return "准入层未接受该候选"
	}
	s := ""
	for i, p := range rep.Problems {
		if i > 0 {
			s += "；"
		}
		s += p.String()
	}
	return s
}

func assertionIDOf(rec *verification.Record) string {
	if rec == nil {
		return ""
	}
	return rec.AssertionID
}

func messageFor(it decision.Item) string {
	if it.Resolution != nil && it.Resolution.IsNone() {
		return "已判定原文中的候选均不成立，该谓词记为缺失"
	}
	return fmt.Sprintf("已裁决，断言 %s 记为已核验", it.Resolution.AssertionID)
}
