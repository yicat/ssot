// 备选方案的接口（见 docs/specs/alternatives.spec.md）。
//
// 核心立场只有一句：**系统不替用户做取舍。** 因此这里没有任何
// 「推荐」「最佳」「默认选中」的字段——只有取舍、代价、依据与不确定性。
package api

import (
	"fmt"
	"time"

	appalt "github.com/ngnl5/ssot/internal/application/alternatives"
	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/alternatives"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// MetricView 是一个可比的维度。
type MetricView struct {
	Name          string  `json:"name"`
	Value         float64 `json:"value"`
	Unit          string  `json:"unit"`
	LowerIsBetter bool    `json:"lowerIsBetter"`
}

// PlanView 是一个方案（面向界面）。
type PlanView struct {
	ID       string `json:"id"`
	Scenario string `json:"scenario"`
	Title    string `json:"title"`
	// Objective 是它优化的东西。**必填**——不声明目标的方案无法被比较。
	Objective   string   `json:"objective"`
	Constraints []string `json:"constraints"`
	Assumptions []string `json:"assumptions"`
	// Preference 是它假设的偏好。未声明偏好时每个备选都必须标注。
	Preference string   `json:"preference"`
	Actions    []string `json:"actions"`

	Metrics []MetricView `json:"metrics"`
	// Opportunity 是机会成本。**必填**——最容易在事后才发现的一项。
	Opportunity string `json:"opportunity"`

	Depends         []string `json:"depends"`
	UnverifiedRatio float64  `json:"unverifiedRatio"`
	// MaxConfidence 不得高于依赖断言中的最低。
	MaxConfidence string `json:"maxConfidence"`

	Sources      []string `json:"sources"`
	FromConflict bool     `json:"fromConflict"`

	Status     string `json:"status"`
	StatusText string `json:"statusText"`
	// Executable 报告该方案能否被执行。
	Executable bool `json:"executable"`
	// Chosen 报告它是不是被选中的那个。**其他备选仍然可访问。**
	Chosen       bool   `json:"chosen"`
	ChosenBy     string `json:"chosenBy"`
	ChooseReason string `json:"chooseReason"`
	At           string `json:"at"`
}

// PlanProposalInput 是一次提出。
type PlanProposalInput struct {
	Scenario     string       `json:"scenario"`
	Title        string       `json:"title"`
	Objective    string       `json:"objective"`
	Constraints  []string     `json:"constraints"`
	Assumptions  []string     `json:"assumptions"`
	Preference   string       `json:"preference"`
	Actions      []string     `json:"actions"`
	Metrics      []MetricView `json:"metrics"`
	Opportunity  string       `json:"opportunity"`
	Depends      []string     `json:"depends"`
	Sources      []string     `json:"sources"`
	FromConflict bool         `json:"fromConflict"`
}

// PrunedView 记录一个被剪掉的方案及原因。
type PrunedView struct {
	Plan PlanView `json:"plan"`
	// By 是支配它的那个方案的 ID。
	By        string `json:"by"`
	Dominance string `json:"dominance"`
}

// EvaluationView 是一次对比呈现。
type EvaluationView struct {
	// Plans 是剪枝与合并之后留下的方案。**已选中的排在最前，其余仍在。**
	Plans  []PlanView   `json:"plans"`
	Pruned []PrunedView `json:"pruned"`
	// Merged 是被合并的组（每组第一个是保留者）。
	Merged [][]string `json:"merged"`
	// Constraints 在没有任何方案可呈现时给出冲突的约束清单。
	Constraints []string `json:"constraints"`
	// Note 是人话说明：只有一个方案时要说清「未发现实质不同的备选」。
	Note string `json:"note"`
	// PreferenceInferred 是一个**建议**，必须经人确认才生效。
	PreferenceInferred string `json:"preferenceInferred"`
}

// PlanRefreshView 是一次依据检查的结果。
type PlanRefreshView struct {
	Checked int      `json:"checked"`
	Invalid []string `json:"invalid"`
	Stale   []string `json:"stale"`
}

// AlternativesService 暴露备选方案。
type AlternativesService struct {
	session *compose.Session
}

// NewAlternativesService 构造服务。
func NewAlternativesService(s *compose.Session) *AlternativesService {
	return &AlternativesService{session: s}
}

func (s *AlternativesService) port() (*compose.Project, error) { return s.session.Project() }

// List 返回某场景的方案，已选中的在最前。
func (s *AlternativesService) List(scenario string) ([]PlanView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	plans, err := p.Store.Plans(scenario)
	if err != nil {
		return nil, err
	}
	ordered := alternatives.Order(plans, "")
	out := make([]PlanView, 0, len(ordered))
	for _, x := range ordered {
		out = append(out, toPlanView(x))
	}
	return out, nil
}

// Propose 提出一个方案。重复提出同一份方案返回已有条目。
func (s *AlternativesService) Propose(in PlanProposalInput) (PlanView, error) {
	p, err := s.port()
	if err != nil {
		return PlanView{}, err
	}
	proposal, err := toPlanProposal(in)
	if err != nil {
		return PlanView{}, err
	}
	plan, err := appalt.Propose(p.Store, proposal, time.Now().UTC())
	if err != nil {
		return PlanView{}, err
	}
	return toPlanView(plan), nil
}

// Choose 选定一个方案。选定者必须是人，且要留下理由。
func (s *AlternativesService) Choose(id, by, reason string) (PlanView, error) {
	p, err := s.port()
	if err != nil {
		return PlanView{}, err
	}
	if by == "" {
		return PlanView{}, fmt.Errorf("必须指明选定人（必须是人）——系统不替使用者做取舍")
	}
	plan, err := appalt.Choose(p.Store, id, verification.Actor{Kind: verification.Human, ID: by}, reason)
	if err != nil {
		return PlanView{}, err
	}
	return toPlanView(plan), nil
}

// Evaluate 把某场景的方案整理成可并排对比的结果。
//
// 偏好留空表示**未声明偏好**：此时若备选覆盖不到两种偏好，它会拒绝呈现，
// 而不是替你选一个。
func (s *AlternativesService) Evaluate(scenario, preference string,
	constraints []string, threshold float64) (EvaluationView, error) {

	p, err := s.port()
	if err != nil {
		return EvaluationView{}, err
	}
	ev, err := appalt.Evaluate(p.Store, scenario, alternatives.Request{
		Preference: preference, Constraints: constraints,
	}, threshold)
	if err != nil {
		return EvaluationView{}, err
	}
	return toEvaluationView(ev), nil
}

// Refresh 检查全部方案的依据：依赖被驳回 -> 不可执行；公式变更 -> 依据已变。
func (s *AlternativesService) Refresh(scenario string) (PlanRefreshView, error) {
	p, err := s.port()
	if err != nil {
		return PlanRefreshView{}, err
	}
	formulas, err := formula.LoadDir(p.FormulasDir(), p.Units)
	if err != nil {
		return PlanRefreshView{}, fmt.Errorf("加载公式：%w", err)
	}
	res, err := appalt.Refresh(p.Store, formulas, scenario)
	if err != nil {
		return PlanRefreshView{}, err
	}
	out := PlanRefreshView{Checked: res.Checked, Invalid: res.Invalid, Stale: res.Stale}
	if out.Invalid == nil {
		out.Invalid = []string{}
	}
	if out.Stale == nil {
		out.Stale = []string{}
	}
	return out, nil
}

// ── 转换 ────────────────────────────────────────────────────────────────────

func toPlanView(p alternatives.Plan) PlanView {
	v := PlanView{
		ID: p.ID, Scenario: p.Scenario, Title: p.Title, Objective: p.Objective,
		Constraints: []string{}, Assumptions: []string{}, Preference: p.Preference,
		Actions: []string{}, Metrics: []MetricView{}, Opportunity: p.Opportunity,
		Depends: []string{}, UnverifiedRatio: p.UnverifiedRatio,
		MaxConfidence: string(p.MaxConfidence),
		Sources:       []string{}, FromConflict: p.FromConflict,
		Status: string(p.Status), StatusText: p.Status.Label(),
		Executable:   p.Status.Executable(),
		Chosen:       p.Status == alternatives.StatusChosen,
		ChosenBy:     p.ChosenBy,
		ChooseReason: p.ChooseReason,
		At:           p.At.Format(time.RFC3339),
	}
	if p.Constraints != nil {
		v.Constraints = p.Constraints
	}
	if p.Assumptions != nil {
		v.Assumptions = p.Assumptions
	}
	if p.Actions != nil {
		v.Actions = p.Actions
	}
	if p.Depends != nil {
		v.Depends = p.Depends
	}
	if p.Sources != nil {
		v.Sources = p.Sources
	}
	for _, m := range p.Metrics {
		v.Metrics = append(v.Metrics, MetricView{
			Name: m.Name, Value: m.Value, Unit: m.Unit, LowerIsBetter: m.LowerIsBetter,
		})
	}
	return v
}

func toEvaluationView(ev alternatives.Evaluation) EvaluationView {
	out := EvaluationView{
		Plans: []PlanView{}, Pruned: []PrunedView{}, Merged: [][]string{},
		Constraints: []string{}, Note: ev.Note,
		PreferenceInferred: ev.PreferenceInferred,
	}
	ordered := alternatives.Order(ev.Plans, ev.PreferenceInferred)
	for _, p := range ordered {
		out.Plans = append(out.Plans, toPlanView(p))
	}
	for _, pr := range ev.Pruned {
		out.Pruned = append(out.Pruned, PrunedView{
			Plan: toPlanView(pr.Plan), By: pr.By, Dominance: pr.Dominance,
		})
	}
	if ev.Merged != nil {
		out.Merged = ev.Merged
	}
	if ev.Constraints != nil {
		out.Constraints = ev.Constraints
	}
	return out
}

func toPlanProposal(in PlanProposalInput) (appalt.Proposal, error) {
	metrics := make([]alternatives.Metric, 0, len(in.Metrics))
	for _, m := range in.Metrics {
		if m.Name == "" {
			return appalt.Proposal{}, fmt.Errorf("比较维度必须有名字")
		}
		metrics = append(metrics, alternatives.Metric{
			Name: m.Name, Value: m.Value, Unit: m.Unit, LowerIsBetter: m.LowerIsBetter,
		})
	}
	return appalt.Proposal{
		Scenario: in.Scenario, Title: in.Title, Objective: in.Objective,
		Constraints: in.Constraints, Assumptions: in.Assumptions,
		Preference: in.Preference, Actions: in.Actions,
		Metrics: metrics, Opportunity: in.Opportunity,
		Depends: in.Depends, Sources: in.Sources,
		FromConflict: in.FromConflict,
	}, nil
}
