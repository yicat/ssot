// Package alternatives 是备选方案的用例编排。
//
// 见 docs/specs/alternatives.spec.md。规则本身在 domain/alternatives 里
// （纯函数、可测）；本包负责三件必须碰数据的事：
//
//	算可信度 —— 方案级可信度**不得高于其依赖断言中的最低**
//	查依据   —— 依赖被驳回则不可执行；依赖公式变更则标记「依据已变」
//	记录选择 —— 选了哪个、为什么选，与其他备选一起保留
package alternatives

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/domain/alternatives"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Port 是备选方案所需的读写端口。
type Port interface {
	PutPlan(alternatives.Plan) error
	UpdatePlanStatus(id, status, chosenBy, reason string) error
	Plan(id string) (alternatives.Plan, bool, error)
	Plans(scenario string) ([]alternatives.Plan, error)
	// Assertion 返回某断言的核验状态与分级。
	Assertion(id string) (status, confidence string, ok bool, err error)
}

// PlanID 是方案的身份：由 (场景, 标题, 目标) 决定。
//
// 同一份方案重复提出是幂等的；改写的方案得到不同的标题或目标，
// 因此是不同的版本——「结论相同但依据不同」正该如此保留两者。
func PlanID(scenario, title, objective string) string {
	h := sha1.Sum([]byte(scenario + "|" + title + "|" + objective))
	return "p" + hex.EncodeToString(h[:8])
}

// Proposal 是一次提出。
type Proposal struct {
	Scenario    string
	Title       string
	Objective   string
	Constraints []string
	Assumptions []string
	Preference  string
	Actions     []string
	Metrics     []alternatives.Metric
	Opportunity string
	// Depends 是依赖：`assert:<ID>` 或 `formula:<名>@<版本>`。
	Depends []string
	Sources []string
	// FromConflict 报告它是不是因为来源冲突而单独成案的。
	FromConflict bool
}

// Derivation 是依赖检查的结果。
type Derivation struct {
	// MaxConfidence 是依赖断言中的**最低**分级。
	MaxConfidence assertion.Confidence
	// UnverifiedRatio 是依赖断言中未核验的比例。
	UnverifiedRatio float64
	// Blocked 非空表示有依赖已被驳回，方案不可执行。
	Blocked string
	// Warnings 是可在界面上提示的问题（如公式无算例）。
	Warnings []string
}

// Derive 从依赖算出方案的可信度与阻塞情况。
//
// 「方案可信度不得高于其所依赖断言中的最低可信度」这条规则在这里落地：
// 允许人工上调就等于把核验分级绕过去了。
func Derive(p Port, depends []string) (Derivation, error) {
	d := Derivation{MaxConfidence: assertion.L4}
	var (
		total, unverified int
		worst             = assertion.L1
		seenAny           bool
	)
	for _, dep := range depends {
		switch {
		case strings.HasPrefix(dep, "assert:"):
			id := strings.TrimPrefix(dep, "assert:")
			status, conf, ok, err := p.Assertion(id)
			if err != nil {
				return d, err
			}
			if !ok {
				d.Blocked = fmt.Sprintf("依赖的断言 %s 已不存在", id)
				return d, nil
			}
			if status == string(assertion.StatusRejected) {
				d.Blocked = fmt.Sprintf("依赖的断言 %s 已被驳回", id)
				return d, nil
			}
			total++
			if status != string(assertion.StatusVerified) {
				unverified++
			}
			c := assertion.Confidence(conf)
			if !seenAny || rank(c) < rank(worst) {
				worst = c
				seenAny = true
			}
		case strings.HasPrefix(dep, "formula:"):
			// 公式不参与分级，但算例未通过会让方案不可执行。
		default:
			return d, fmt.Errorf("依赖项 %q 写法不合法：应为 assert:<ID> 或 formula:<名>@<版本>", dep)
		}
	}
	if seenAny {
		d.MaxConfidence = worst
	}
	if total > 0 {
		d.UnverifiedRatio = float64(unverified) / float64(total)
	}
	return d, nil
}

func rank(c assertion.Confidence) int {
	switch c {
	case assertion.L1:
		return 4
	case assertion.L2:
		return 3
	case assertion.L3:
		return 2
	case assertion.L4:
		return 1
	}
	return 0
}

// Propose 提出一个方案。重复提出同一份方案返回已有条目。
func Propose(p Port, in Proposal, now time.Time) (alternatives.Plan, error) {
	if in.Scenario == "" {
		return alternatives.Plan{}, fmt.Errorf("方案必须归属场景")
	}
	id := PlanID(in.Scenario, in.Title, in.Objective)
	if existing, ok, err := p.Plan(id); err != nil {
		return alternatives.Plan{}, err
	} else if ok {
		return existing, nil
	}
	der, err := Derive(p, in.Depends)
	if err != nil {
		return alternatives.Plan{}, err
	}
	plan := alternatives.Plan{
		ID: id, Scenario: in.Scenario, Title: in.Title, Objective: in.Objective,
		Constraints: in.Constraints, Assumptions: in.Assumptions,
		Preference: in.Preference, Actions: in.Actions,
		Metrics: in.Metrics, Opportunity: in.Opportunity,
		Depends: in.Depends, UnverifiedRatio: der.UnverifiedRatio,
		MaxConfidence: der.MaxConfidence,
		Sources:       in.Sources, FromConflict: in.FromConflict,
		Status: alternatives.StatusCandidate, At: now,
	}
	if der.Blocked != "" {
		plan.Status = alternatives.StatusInvalid
	}
	if err := p.PutPlan(plan); err != nil {
		return alternatives.Plan{}, err
	}
	return plan, nil
}

// Choose 选定一个方案。
//
// 选定者必须是**人**：让 agent 替使用者决定选哪个，等于把取舍权交出去了，
// 而这正是本层要守住的边界。选定后其他备选**仍然可访问**。
func Choose(p Port, id string, by verification.Actor, reason string) (alternatives.Plan, error) {
	plan, ok, err := p.Plan(id)
	if err != nil {
		return alternatives.Plan{}, err
	}
	if !ok {
		return alternatives.Plan{}, fmt.Errorf("方案 %s 不存在", id)
	}
	if !by.Valid() || !by.IsHuman() {
		return alternatives.Plan{}, fmt.Errorf("选定方案必须由人记录——系统不替使用者做取舍")
	}
	if reason == "" {
		return alternatives.Plan{}, fmt.Errorf("选定缺少理由——保留选择理由，才能从历史里看出偏好")
	}
	if !plan.Status.Executable() {
		return alternatives.Plan{}, fmt.Errorf(
			"方案 %s 的状态为「%s」，依据已失效，不得执行", id, plan.Status.Label())
	}
	if err := p.UpdatePlanStatus(id, string(alternatives.StatusChosen), by.String(), reason); err != nil {
		return alternatives.Plan{}, err
	}
	plan.Status = alternatives.StatusChosen
	plan.ChosenBy = by.String()
	plan.ChooseReason = reason
	return plan, nil
}

// Evaluate 取某场景的方案并整理成可呈现的结果。
func Evaluate(p Port, scenario string, req alternatives.Request, threshold float64) (alternatives.Evaluation, error) {
	plans, err := p.Plans(scenario)
	if err != nil {
		return alternatives.Evaluation{}, err
	}
	return alternatives.Evaluate(plans, req, threshold)
}

// RefreshResult 报告一次依据检查的结果。
type RefreshResult struct {
	Checked int
	Invalid []string
	Stale   []string
}

// Refresh 检查全部方案的依据。
//
// 方案是**持久化**的：它记录当时基于什么做了什么取舍。因此依据变化时
// 不重算、不静默替换，只标记——重算会抹掉当初的依据，而复盘正是靠它。
func Refresh(p Port, formulas map[string]*formula.Formula, scenario string) (RefreshResult, error) {
	plans, err := p.Plans(scenario)
	if err != nil {
		return RefreshResult{}, err
	}
	var res RefreshResult
	for _, plan := range plans {
		if plan.Status == alternatives.StatusSuperseded {
			continue
		}
		res.Checked++
		der, err := Derive(p, plan.Depends)
		if err != nil {
			return res, err
		}
		if der.Blocked != "" {
			if err := p.UpdatePlanStatus(plan.ID, string(alternatives.StatusInvalid),
				plan.ChosenBy, der.Blocked); err != nil {
				return res, err
			}
			res.Invalid = append(res.Invalid, plan.ID)
			continue
		}
		if reason, changed := formulaChanged(formulas, plan.Depends); changed {
			if err := p.UpdatePlanStatus(plan.ID, string(alternatives.StatusStale),
				plan.ChosenBy, reason); err != nil {
				return res, err
			}
			res.Stale = append(res.Stale, plan.ID)
		}
	}
	sort.Strings(res.Invalid)
	sort.Strings(res.Stale)
	return res, nil
}

func formulaChanged(formulas map[string]*formula.Formula, depends []string) (string, bool) {
	for _, dep := range depends {
		if !strings.HasPrefix(dep, "formula:") {
			continue
		}
		body := strings.TrimPrefix(dep, "formula:")
		name, version := body, ""
		if i := strings.LastIndex(body, "@"); i >= 0 {
			name, version = body[:i], body[i+1:]
		}
		f, ok := formulas[name]
		if !ok {
			return fmt.Sprintf("依赖的公式 %s 已不存在", name), true
		}
		if version != "" && f.Version != version {
			return fmt.Sprintf("依赖的公式 %s 已从 %s 改为 %s", name, version, f.Version), true
		}
	}
	return "", false
}
