// Package admit 是准入层：候选到断言的唯一入口。
//
// 见 docs/specs/admission.spec.md。它**不直接写库**，而是产出变更集，
// 由存储层原子应用。若准入层能直接写事实源，「准入」就不存在了——
// 所有校验都会降级成建议。
//
// 六项判定（MVP 实现前五项）：
//  1. schema 校验   2. 引用完整性   3. 分级判定
//  4. 来源登记（MVP 简化）            5. 冲突检测   6. 初始核验状态
package admit

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/ngnl5/ssot/internal/application/ingest"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/validate"
	"github.com/ngnl5/ssot/internal/domain/value"
)

// Options 是一次准入的上下文。
type Options struct {
	Entity     string
	Source     assertion.Source
	CapturedAt time.Time
	Units      *unit.Table
}

// Problem 是一条被拒绝的候选原因。
type Problem struct {
	Subject   string
	Predicate string
	Reason    string
}

func (p Problem) String() string {
	return fmt.Sprintf("%s.%s: %s", p.Subject, p.Predicate, p.Reason)
}

// Report 是准入结果的可读报告。
type Report struct {
	Candidates int
	Records    int
	Accepted   int
	Rejected   int
	Conflicts  int
	Markers    []validate.Marker
	Problems   []Problem
	// Undeclared 是数据中出现的、schema 未声明的谓词（漂移，不阻断）
	Undeclared []string
}

// Summary 返回一行摘要。
func (r Report) Summary() string {
	return fmt.Sprintf("候选 %d，记录 %d，接受 %d，拒绝 %d，冲突 %d，标记 %d",
		r.Candidates, r.Records, r.Accepted, r.Rejected, r.Conflicts, len(r.Markers))
}

// Run 执行准入。
//
// existing 是库中已有的 (key -> 已存在的取值 JSON) 映射，用于冲突检测。
// 冲突的处理是**并存并标记**，绝不覆盖——同一件事的不同说法都要保留，
// 由人裁决，系统不替用户选。
func Run(cands []ingest.Candidate, set *schema.Set, existing map[string][]string, opts Options) (assertion.ChangeSet, Report, error) {
	rep := Report{Candidates: len(cands)}

	e, ok := set.Lookup(opts.Entity)
	if !ok {
		return assertion.ChangeSet{}, rep, fmt.Errorf("schema 中没有实体 %q", opts.Entity)
	}

	// 按主体分组，同时过滤 schema 未声明的谓词。
	//
	// 过滤结果必须**带进后面建断言的循环**——否则未声明谓词会被写入，
	// 漂移就从「报告」变成了「污染」。（实测踩过这个 bug。）
	grouped := map[string][]ingest.Candidate{}
	undeclared := map[string]bool{}
	var order []string
	for _, c := range cands {
		if c.Entity != opts.Entity {
			continue
		}
		if _, ok := e.FieldByKey(c.Predicate); !ok {
			undeclared[c.Predicate] = true
			continue
		}
		if _, seen := grouped[c.Subject]; !seen {
			order = append(order, c.Subject)
		}
		grouped[c.Subject] = append(grouped[c.Subject], c)
	}
	for k := range undeclared {
		rep.Undeclared = append(rep.Undeclared, k)
	}
	sort.Strings(rep.Undeclared)
	sort.Strings(order)

	ck := validate.Checkers{Units: opts.Units}
	var cs assertion.ChangeSet

	for _, subject := range order {
		rep.Records++
		group := grouped[subject]
		vals := make(map[string]value.Value, len(group))
		for _, c := range group {
			vals[c.Predicate] = c.Value
		}
		rec := validate.Record{Entity: opts.Entity, Values: vals}
		vr := validate.Check(e, rec, ck)
		rep.Markers = append(rep.Markers, vr.Markers...)

		if !vr.OK() {
			rep.Rejected++
			for _, v := range vr.Violations {
				rep.Problems = append(rep.Problems, Problem{Subject: subject, Predicate: v.Field, Reason: v.Rule + ": " + v.Detail})
			}
			continue
		}

		// 为该主体的每个**已通过过滤的**候选建断言
		for _, c := range group {
			a := toAssertion(c, opts)
			key := a.Key()
			vj := assertion.ValueJSON(a.Value)

			if vals, ok := existing[key]; ok && !containsStr(vals, vj) {
				// 身份相同而取值不同 —— 冲突
				cs.Conflict = append(cs.Conflict, a.ID)
				rep.Conflicts++
			}
			cs.Insert = append(cs.Insert, a)
			rep.Accepted++
		}
	}

	return cs, rep, nil
}

// toAssertion 把候选转成断言。
//
// 分级由解析方式决定：**回退解析不得定为 L1**。
// 初始状态一律为「待核验」——**自动通过不等于已核验**。
func toAssertion(c ingest.Candidate, opts Options) assertion.Assertion {
	conf := c.Parsing.MaxConfidence()
	return assertion.Assertion{
		ID:         assertionID(c),
		Entity:     c.Entity,
		Subject:    c.Subject,
		Predicate:  c.Predicate,
		Value:      c.Value,
		Qualifiers: c.Qualifiers,
		Source:     opts.Source,
		Provenance: assertion.Provenance{
			Artifact:   c.Artifact,
			Anchor:     c.Anchor,
			Revision:   c.Revision,
			CapturedAt: opts.CapturedAt,
		},
		Confidence: conf,
		Status:     assertion.StatusPending,
	}
}

// assertionID 是确定性的：同一 (身份, 取值) 永远得到同一 ID，
// 因此重复准入是幂等的。
func assertionID(c ingest.Candidate) string {
	h := sha1.Sum([]byte(c.Entity + "|" + c.Subject + "|" + c.Predicate + "|" +
		c.Qualifiers.String() + "|" + assertion.ValueJSON(c.Value)))
	return "a" + hex.EncodeToString(h[:8])
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
