// Package derive 从已有断言派生新断言。
//
// 见 docs/specs/derivation.spec.md。固化判据只有一条：**是否上下文中立**。
// 中立的值（满级面板、期望暴击系数）固化为项目级 L3 断言，全项目共享；
// 依赖场景上下文的值不固化，只作场景中间结果。
//
// L3 断言必须带推导链（依赖了哪些断言、用了哪个公式）。
package derive

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/expr"
	"github.com/ngnl5/ssot/internal/domain/unit"
)

// Reader 是派生所需的读取端口（由消费方定义，避免依赖具体存储）。
type Reader interface {
	Subjects(entity string) ([]string, error)
	BySubject(entity, subject string) ([]assertion.Assertion, error)
}

// Options 是一次派生的上下文。
type Options struct {
	Entity    string // 源实体，例如 shikigami
	Predicate string // 派生出的事实名，例如 crit_factor
	Source    assertion.Source
}

// Result 是派生结果。
type Result struct {
	ChangeSet  assertion.ChangeSet
	Subjects   int
	Derived    int
	Skipped    int
	SkipReason []string
}

// Run 对某实体的每个主体执行一次派生。
//
// 输入缺失或为「未知」时**跳过**而不是猜——未知参与运算仍是未知（见 expr）。
func Run(r Reader, f *formula.Formula, units *unit.Table, opts Options) (Result, error) {
	var res Result
	subjects, err := r.Subjects(opts.Entity)
	if err != nil {
		return res, err
	}
	res.Subjects = len(subjects)

	// 绑定名 → 来源路径；来源路径即谓词名
	sources := make([]string, 0, len(f.Bindings))
	for name := range f.Bindings {
		sources = append(sources, name)
	}
	sort.Strings(sources)

	now := time.Now().UTC()
	for _, subject := range subjects {
		as, err := r.BySubject(opts.Entity, subject)
		if err != nil {
			return res, err
		}
		byPred := map[string]assertion.Assertion{}
		for _, a := range as {
			byPred[a.Predicate] = a
		}

		in := map[string]expr.Val{}
		var inputs []string
		missing := ""
		for _, name := range sources {
			path := f.Bindings[name]
			a, ok := byPred[path]
			if !ok {
				missing = "缺少输入 " + path
				break
			}
			v, err := formula.FromValue(a.Value)
			if err != nil {
				missing = fmt.Sprintf("输入 %s 无法参与计算：%v", path, err)
				break
			}
			if v.Kind == expr.KUnknown {
				missing = "输入 " + path + " 为未知"
				break
			}
			in[name] = v
			inputs = append(inputs, a.ID)
		}
		if missing != "" {
			res.Skipped++
			if len(res.SkipReason) < 20 {
				res.SkipReason = append(res.SkipReason, subject+": "+missing)
			}
			continue
		}

		got, err := f.Eval(in, units)
		if err != nil {
			res.Skipped++
			if len(res.SkipReason) < 20 {
				res.SkipReason = append(res.SkipReason, fmt.Sprintf("%s: 求值失败 %v", subject, err))
			}
			continue
		}
		if got.Kind == expr.KUnknown {
			res.Skipped++
			continue
		}

		sort.Strings(inputs)
		a := assertion.Assertion{
			ID:        derivedID(opts.Entity, subject, opts.Predicate, f.Name),
			Entity:    opts.Entity,
			Subject:   subject,
			Predicate: opts.Predicate,
			// 派生结果统一以 fraction 口径表示比值类结果
			Value:      formula.ToValue(got),
			Source:     opts.Source,
			Provenance: assertion.Provenance{Artifact: "formula:" + f.Name, Anchor: "derived", Revision: f.Version, CapturedAt: now},
			Confidence: assertion.L3,
			Status:     assertion.StatusPending,
			Derived:    &assertion.Derivation{Formula: f.Name, FormulaRev: f.Version, Inputs: inputs},
		}
		if err := a.Validate(); err != nil {
			return res, fmt.Errorf("派生结果不合法：%w", err)
		}
		res.ChangeSet.Insert = append(res.ChangeSet.Insert, a)
		res.Derived++
	}
	return res, nil
}

func derivedID(entity, subject, predicate, formula string) string {
	h := sha1.Sum([]byte("L3|" + entity + "|" + subject + "|" + predicate + "|" + formula))
	return "d" + hex.EncodeToString(h[:8])
}
