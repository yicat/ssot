// 场景运行的接口（见 docs/specs/workspace.spec.md）。
//
// 它复用 application/scenario.Run，与 CLI 走**同一条**顺序：
// 完整性检查 → 取数 → 求值。界面不得自己拼一套，
// 否则 CLI 能跑的场景在界面上会因为规则不同而跑不了。
package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/application/scenario"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/assertion"
)

// RefCandidates 是一个「取自其他实体」的绑定及其候选。
type RefCandidates struct {
	Name        string `json:"name"`
	Entity      string `json:"entity"`
	Via         string `json:"via"`
	Description string `json:"description"`
	// Candidates 是该主实体下可选的来源主体；未选主实体时为空。
	Candidates []string `json:"candidates"`
}

// RunSetup 是运行一个场景所需的全部输入（面向界面）。
type RunSetup struct {
	Scenario string `json:"scenario"`
	// Entity 是主实体，主体从这里选。
	Entity string `json:"entity"`
	// Subjects 是主实体上的可选主体。
	Subjects []string `json:"subjects"`
	// Subject 是当前选中的主体；null 表示未选。
	Subject *string `json:"subject"`
	// Runnable 报告 requires 是否满足。不满足时界面必须挡住运行按钮。
	Runnable bool              `json:"runnable"`
	Requires []RequirementView `json:"requires"`
	Missing  []string          `json:"missing"`
	Inputs   []InputSpecView   `json:"inputs"`
	Refs     []RefCandidates   `json:"refs"`
}

// RunInput 是一个外部输入。
type RunInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Unit 为空时按声明补全；声明也没有则拒绝。
	Unit string `json:"unit"`
}

// RefInput 是一个「取自其他实体」的绑定。
type RefInput struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
}

// OutputValue 是一项参与计算的输入。
type OutputValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

// ClaimRef 是一条被依赖的断言（面向界面）。
type ClaimRef struct {
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Predicate  string `json:"predicate"`
	Value      string `json:"value"`
	Status     string `json:"status"`
	Confidence string `json:"confidence"`
}

// ExternalInput 是本次使用的一个外部输入。**始终标注未核验**。
type ExternalInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Why   string `json:"why"`
}

// RunResult 是一次运行的结果。
type RunResult struct {
	OK         bool   `json:"ok"`
	Scenario   string `json:"scenario"`
	Subject    string `json:"subject"`
	Result     string `json:"result"`
	ResultUnit string `json:"resultUnit"`
	// Bindings 是参与计算的每一项取值，含它来自哪里。
	Bindings []OutputValue `json:"bindings"`
	Notes    []string      `json:"notes"`
	// UnverifiedRatio 是本次产出所依赖的**未核验比例**（0..1）。
	UnverifiedRatio float64           `json:"unverifiedRatio"`
	Unverified      []ClaimRef        `json:"unverified"`
	External        []ExternalInput   `json:"external"`
	Requires        []RequirementView `json:"requires"`
	Missing         []string          `json:"missing"`
	// Message 是人话结果或拒绝原因。
	Message string `json:"message"`
	// At 是运行时刻，便于把界面上看到的数字与某一次运行对上。
	At string `json:"at"`
}

// RunSetup 返回运行所需的输入结构。
//
// subject 为空时只给出可选主体，不列引用候选——候选取决于选了哪个主体。
func (s *ScenarioService) RunSetup(name, subject string) (RunSetup, error) {
	p, err := s.session.Project()
	if err != nil {
		return RunSetup{}, err
	}
	sc, ok := p.Scenario(name)
	if !ok {
		return RunSetup{}, fmt.Errorf("项目 %s 中没有场景 %q", p.Name(), name)
	}
	out := RunSetup{
		Scenario: sc.Name, Entity: sc.Entity,
		Subjects: []string{}, Requires: []RequirementView{},
		Missing: []string{}, Inputs: []InputSpecView{}, Refs: []RefCandidates{},
	}

	subjects, err := p.Store.Subjects(sc.Entity)
	if err != nil {
		return out, err
	}
	if subjects != nil {
		out.Subjects = subjects
	}
	if subject != "" {
		out.Subject = &subject
	}

	rep, err := s.check(p, sc)
	if err != nil {
		return out, err
	}
	out.Runnable = rep.Runnable()
	for _, q := range rep.Requirements {
		out.Requires = append(out.Requires, RequirementView{
			Want: q.Want, Status: string(q.Status), Have: q.Have,
			Total: q.Total, Coverage: q.Coverage, Detail: q.Detail,
		})
	}
	out.Missing = rep.Missing()
	if out.Missing == nil {
		out.Missing = []string{}
	}
	for _, in := range sc.Inputs {
		out.Inputs = append(out.Inputs, InputSpecView{
			Name: in.Name, Description: in.Description, Unit: in.Unit, Min: in.Min, Max: in.Max,
		})
	}
	for _, r := range sc.Refs {
		ref := RefCandidates{
			Name: r.Name, Entity: r.Entity, Via: r.Via,
			Description: r.Description, Candidates: []string{},
		}
		if subject != "" {
			cands, err := p.Store.SubjectsByRef(r.Entity, r.Via,
				subjectAsRefValue(p, sc.Entity, subject))
			if err != nil {
				return out, err
			}
			if cands != nil {
				ref.Candidates = cands
			}
		}
		out.Refs = append(out.Refs, ref)
	}
	return out, nil
}

// subjectAsRefValue 把主体标识转成引用比较用的值。
//
// 引用（如 skill.character_id）存的是**数值**，而主体标识是字符串，
// 因此这里按主实体的身份字段类型转换一次。转不了就原样返回：
// 查询会匹配不到候选，界面显示「没有候选」——而不是静默算错。
func subjectAsRefValue(p *compose.Project, entity, subject string) any {
	e, ok := p.Schema.Lookup(entity)
	if !ok {
		return subject
	}
	id := e.Identity()
	if id == nil {
		return subject
	}
	if string(id.Type) == "number" {
		var f float64
		if _, err := fmt.Sscanf(subject, "%g", &f); err == nil {
			return f
		}
	}
	return subject
}

// Run 执行一次场景运行。
func (s *ScenarioService) Run(name, subject string, inputs []RunInput,
	refs []RefInput) (RunResult, error) {

	p, err := s.session.Project()
	if err != nil {
		return RunResult{}, err
	}
	sc, ok := p.Scenario(name)
	if !ok {
		return RunResult{}, fmt.Errorf("项目 %s 中没有场景 %q", p.Name(), name)
	}
	if subject == "" {
		return RunResult{}, fmt.Errorf("必须选择主体")
	}

	formulas, err := formula.LoadDir(p.FormulasDir(), p.Units)
	if err != nil {
		return RunResult{}, fmt.Errorf("加载公式：%w", err)
	}

	// 外部输入：名字必须在场景中声明，单位必须与声明一致，范围必须合法。
	given := map[string]string{}
	for _, in := range inputs {
		spec, ok := findInput(sc, in.Name)
		if !ok {
			return RunResult{}, fmt.Errorf("场景 %s 没有声明外部输入 %q（可用：%s）",
				sc.Name, in.Name, inputNames(sc))
		}
		unit := in.Unit
		if unit == "" {
			unit = spec.Unit
		}
		if unit != spec.Unit {
			return RunResult{}, fmt.Errorf(
				"外部输入 %q 声明的单位是 %s，收到 %s——不做隐式换算", in.Name, spec.Unit, unit)
		}
		if strings.TrimSpace(in.Value) == "" {
			return RunResult{}, fmt.Errorf("外部输入 %q 没有填值", in.Name)
		}
		if err := checkRange(spec, in.Value); err != nil {
			return RunResult{}, err
		}
		given[in.Name] = strings.TrimSpace(in.Value) + " " + unit
	}

	extra := map[string]scenario.Ref{}
	for _, rf := range refs {
		decl, ok := sc.RefByName(rf.Name)
		if !ok {
			return RunResult{}, fmt.Errorf("场景 %s 没有声明引用 %q", sc.Name, rf.Name)
		}
		if rf.Subject == "" {
			return RunResult{}, fmt.Errorf("引用 %q 没有选来源主体", rf.Name)
		}
		extra[rf.Name] = scenario.Ref{Entity: decl.Entity, Subject: rf.Subject}
	}

	in := scenario.RunInput{Entity: sc.Entity, Subject: subject, Extra: extra, Values: given}
	out, rep, runErr := scenario.Run(sc, formulas, p.Store, p.Store, p.Units, in)

	res := RunResult{
		Scenario: sc.Name, Subject: subject, At: time.Now().UTC().Format(time.RFC3339),
		Bindings: []OutputValue{}, Notes: []string{},
		Unverified: []ClaimRef{}, External: []ExternalInput{}, Requires: []RequirementView{},
		Missing: []string{},
	}
	for _, q := range rep.Requirements {
		res.Requires = append(res.Requires, RequirementView{
			Want: q.Want, Status: string(q.Status), Have: q.Have,
			Total: q.Total, Coverage: q.Coverage, Detail: q.Detail,
		})
	}
	for _, spec := range sc.Inputs {
		if v, ok := given[spec.Name]; ok {
			res.External = append(res.External, ExternalInput{
				Name: spec.Name, Value: v, Why: spec.Description,
			})
		}
	}
	if runErr != nil {
		res.OK = false
		res.Message = runErr.Error()
		res.Missing = rep.Missing()
		if res.Missing == nil {
			res.Missing = []string{}
		}
		return res, nil
	}

	res.OK = true
	res.Notes = out.Notes
	if res.Notes == nil {
		res.Notes = []string{}
	}
	res.Result = out.Result.String()
	res.ResultUnit = out.Result.Unit
	res.Bindings = bindingViews(out)

	total, unverified := countUnverified(p, sc.Entity, subject, extra)
	res.Unverified = unverified
	if total > 0 {
		res.UnverifiedRatio = float64(len(unverified)) / float64(total)
	}
	res.Message = "已算出 " + out.Result.String()
	return res, nil
}

func findInput(sc scenario.Spec, name string) (scenario.Input, bool) {
	for _, in := range sc.Inputs {
		if in.Name == name {
			return in, true
		}
	}
	return scenario.Input{}, false
}

func inputNames(sc scenario.Spec) string {
	if len(sc.Inputs) == 0 {
		return "（该场景没有外部输入）"
	}
	parts := make([]string, 0, len(sc.Inputs))
	for _, in := range sc.Inputs {
		parts = append(parts, in.Name)
	}
	return strings.Join(parts, ", ")
}

func checkRange(in scenario.Input, raw string) error {
	var v float64
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%g", &v); err != nil {
		return fmt.Errorf("外部输入 %q 不是数值：%q", in.Name, raw)
	}
	if in.Min != nil && v < *in.Min {
		return fmt.Errorf("外部输入 %q 的值 %v 小于下限 %v", in.Name, v, *in.Min)
	}
	if in.Max != nil && v > *in.Max {
		return fmt.Errorf("外部输入 %q 的值 %v 大于上限 %v", in.Name, v, *in.Max)
	}
	return nil
}

func bindingViews(out scenario.Output) []OutputValue {
	names := make([]string, 0, len(out.Bindings))
	for n := range out.Bindings {
		names = append(names, n)
	}
	// 顺序必须稳定：每次打开结果不同的顺序，人就无法判断「和上次是不是一样」。
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	res := make([]OutputValue, 0, len(names))
	for _, n := range names {
		v := out.Bindings[n]
		res = append(res, OutputValue{Name: n, Value: v.String(), Unit: v.Unit})
	}
	return res
}

// countUnverified 统计本次产出依赖的断言里有多少条未核验。
//
// 这是输出侧的硬规则：**任何方案都必须给出未核验比例**。
// 只显示结果而不显示它有多可信，正是「看起来已经核验过」的来源。
func countUnverified(p *compose.Project, entity, subject string,
	extra map[string]scenario.Ref) (int, []ClaimRef) {

	var all []assertion.Assertion
	if as, err := p.Store.BySubject(entity, subject); err == nil {
		all = append(all, as...)
	}
	for _, ref := range extra {
		if as, err := p.Store.BySubject(ref.Entity, ref.Subject); err == nil {
			all = append(all, as...)
		}
	}
	bad := []ClaimRef{}
	for _, a := range all {
		if a.Status == assertion.StatusVerified {
			continue
		}
		bad = append(bad, ClaimRef{
			ID: a.ID, Subject: a.Entity + "/" + a.Subject, Predicate: a.Predicate,
			Value: a.Value.String(), Status: string(a.Status), Confidence: string(a.Confidence),
		})
	}
	return len(all), bad
}
