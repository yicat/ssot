// 场景的接口（见 docs/specs/workspace.spec.md）。
//
// 场景是「缺什么」的载体：它声明 requires，界面据此把缺口摆出来。
// 判定规则在 application/scenario 里，本文件只做转换。
package api

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/application/scenario"
	"github.com/ngnl5/ssot/internal/compose"
)

// RequirementView 是一项 requires 的检查结果（面向界面）。
type RequirementView struct {
	Want     string  `json:"want"`
	Status   string  `json:"status"`
	Have     int     `json:"have"`
	Total    int     `json:"total"`
	Coverage float64 `json:"coverage"`
	Detail   string  `json:"detail"`
}

// InputSpecView 是一个外部输入的声明（面向界面）。
//
// Unit 与 Min/Max 必须暴露给界面：没有量纲的输入框就是一个歧义制造机，
// 而声明了范围却不校验比不声明更糟——那会让人以为自己被保护着。
type InputSpecView struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Unit        string   `json:"unit"`
	Min         *float64 `json:"min"`
	Max         *float64 `json:"max"`
}

// FormulaStatusView 是一个依赖公式的状态（面向界面）。
type FormulaStatusView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// Cases 是算例的通过情况。
	Cases []CaseResultView `json:"cases"`
}

// CaseResultView 是一条算例的结果。
type CaseResultView struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// ScenarioOverview 是场景概览（面向界面）。
type ScenarioOverview struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Runnable 报告 requires 是否全部满足。
	Runnable bool `json:"runnable"`
	// Requires 逐项列出「需要什么 / 现有多少 / 覆盖率」——
	// 只报一个总数等于没报。
	Requires []RequirementView   `json:"requires"`
	Inputs   []InputSpecView     `json:"inputs"`
	Formulas []FormulaStatusView `json:"formulas"`
	Outputs  []string            `json:"outputs"`
	// Missing 是缺失项的人话清单，供界面直接显示。
	Missing []string `json:"missing"`
}

// ScenarioService 暴露场景。
type ScenarioService struct {
	session *compose.Session
}

// NewScenarioService 构造服务。
func NewScenarioService(s *compose.Session) *ScenarioService { return &ScenarioService{session: s} }

// List 返回当前项目的场景，按名称排序。
func (s *ScenarioService) List() ([]ScenarioRef, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	out := make([]ScenarioRef, 0, len(p.Scenarios))
	for _, sc := range p.Scenarios {
		ref := ScenarioRef{
			Name: sc.Name, Description: sc.Description,
			Inputs: len(sc.Inputs), RequiresTotal: len(sc.Requires),
		}
		if rep, err := s.check(p, sc); err == nil {
			ref.RequiresMet = rep.Runnable()
		} else {
			ref.Skipped = err.Error()
		}
		out = append(out, ref)
	}
	return out, nil
}

// Select 选中一个场景。名称必须在该项目的场景列表中。
func (s *ScenarioService) Select(name string) (SessionState, error) {
	if err := s.session.SelectScenario(name); err != nil {
		return SessionState{}, err
	}
	return NewProjectService(s.session).Current()
}

// Overview 返回场景概览：requires 逐项、外部输入声明、依赖公式与输出。
func (s *ScenarioService) Overview(name string) (ScenarioOverview, error) {
	p, err := s.session.Project()
	if err != nil {
		return ScenarioOverview{}, err
	}
	sc, ok := p.Scenario(name)
	if !ok {
		return ScenarioOverview{}, fmt.Errorf("项目 %s 中没有场景 %q", p.Name(), name)
	}

	ov := ScenarioOverview{
		Name: sc.Name, Description: sc.Description,
		Requires: []RequirementView{}, Inputs: []InputSpecView{},
		Formulas: []FormulaStatusView{}, Outputs: sc.Outputs,
		Missing: []string{},
	}
	if ov.Outputs == nil {
		ov.Outputs = []string{}
	}

	rep, err := s.check(p, sc)
	if err != nil {
		return ov, err
	}
	for _, q := range rep.Requirements {
		ov.Requires = append(ov.Requires, RequirementView{
			Want: q.Want, Status: string(q.Status), Have: q.Have,
			Total: q.Total, Coverage: q.Coverage, Detail: q.Detail,
		})
	}
	for _, f := range rep.Formulas {
		fv := FormulaStatusView{Name: f.Name, Status: string(f.Status), Cases: []CaseResultView{}}
		for _, c := range f.Cases {
			detail := "通过"
			if !c.Passed {
				detail = fmt.Sprintf("期望 %s，实际 %s", c.Want, c.Got)
				if c.Err != nil {
					detail = c.Err.Error()
				}
			}
			fv.Cases = append(fv.Cases, CaseResultView{
				Name: c.Name, Passed: c.Passed, Detail: detail,
			})
		}
		ov.Formulas = append(ov.Formulas, fv)
	}
	for _, in := range sc.Inputs {
		ov.Inputs = append(ov.Inputs, InputSpecView{
			Name: in.Name, Description: in.Description, Unit: in.Unit,
			Min: in.Min, Max: in.Max,
		})
	}
	ov.Runnable = rep.Runnable()
	ov.Missing = rep.Missing()
	if ov.Missing == nil {
		ov.Missing = []string{}
	}
	return ov, nil
}

// check 执行场景的完整性检查。
func (s *ScenarioService) check(p *compose.Project, sc scenario.Spec) (scenario.CheckReport, error) {
	formulas, err := formula.LoadDir(p.FormulasDir(), p.Units)
	if err != nil {
		return scenario.CheckReport{}, fmt.Errorf("加载公式：%w", err)
	}
	return scenario.Check(sc, p.Store, formulas, p.Units)
}
