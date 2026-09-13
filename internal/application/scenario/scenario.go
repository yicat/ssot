// Package scenario 加载并运行场景。
//
// 见 docs/specs/scenario.spec.md。场景是**用途边界**（不是隔离边界）：
// 同一项目下所有场景读同一批断言，场景只组织「读什么、算什么、产出什么」。
//
// 场景是「缺什么」的载体：它声明 requires，系统据此报告缺什么——
// 把数据缺口从事后发现变成事前可见。
//
// 已知偏差（MVP 简化）：与本包的兄弟包 formula 一样直接读 YAML，
// 文件 I/O 本应属于基础设施。依赖方向未被违反，但留待 MVP 之后重构。
package scenario

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/expr"
	"github.com/ngnl5/ssot/internal/domain/unit"

	"gopkg.in/yaml.v3"
)

// Input 是场景声明的**外部输入**。
//
// 它与 requires 的区别是根本性的：
//
//	requires —— 库里**应当有**的数据；缺失即不可运行
//	inputs   —— 库**本来就不该有**的值，由调用方在运行时给出
//
// 混为一谈会导致两种错误：把外部输入当成缺失数据而拒绝运行，
// 或者把缺失数据当成外部输入而悄悄绕过。
type Input struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Unit 是该外部输入的量纲，**必填**。
	//
	// 没有量纲的输入无法参与计算：`def_reduction=0.5` 到底是一半还是 0.5%，
	// 光看数字无从判断。实测踩过 crit=50 与 cri=0.5 的歧义，
	// 外部输入更不该重蹈——它是人手敲进去的，没人替它把关。
	Unit string `yaml:"unit"`
	// Min/Max 是取值范围的**可选**约束。声明了就必须校验：
	// 声明了不校验比不声明更糟，那会让人以为自己被保护着。
	Min *float64 `yaml:"min"`
	Max *float64 `yaml:"max"`
}

// RefDecl 声明「这个绑定值来自另一个实体上的主体」。
//
// 场景的输入不必都来自同一个实体：伤害计算需要式神的攻击与暴击系数，
// 以及技能的倍率——后者在另一个实体上。
//
// 声明它而不是让调用方随手 `--ref ratio=skill:262_01`，是因为
// **随手写的引用无法校验也无法列举**：界面既不知道要找哪个实体，
// 也不知道有哪些候选，只能让人凭记忆敲一个 ID。
type RefDecl struct {
	// Name 是公式的绑定路径，例如 ratio。
	Name string `yaml:"name"`
	// Entity 是目标实体，例如 skill。
	Entity string `yaml:"entity"`
	// Via 是目标实体上指向本项目主体的谓词，例如 character_id。
	// 有了它，界面才能列出候选，而不是让人猜。
	Via         string `yaml:"via"`
	Description string `yaml:"description"`
}

// Spec 是场景声明。
type Spec struct {
	Name        string `yaml:"scenario"`
	Description string `yaml:"description"`
	// Entity 是这个场景的**主实体**：运行时的主体（例如式神）来自它。
	//
	// 显式声明而不是在代码里写死 "shikigami"：场景换一个主实体
	// （例如按御魂算），不该需要改程序。
	Entity   string    `yaml:"entity"`
	Requires []string  `yaml:"requires"` // "entity.predicate"
	Inputs   []Input   `yaml:"inputs"`   // 运行时外部输入
	Refs     []RefDecl `yaml:"refs"`     // 取自其他实体的绑定
	Formulas []string  `yaml:"formulas"`
	Outputs  []string  `yaml:"outputs"`
}

// RefByName 按绑定路径取引用声明。
func (s Spec) RefByName(name string) (RefDecl, bool) {
	for _, r := range s.Refs {
		if r.Name == name {
			return r, true
		}
	}
	return RefDecl{}, false
}

// IsInput 报告某个路径是否是本场景声明的外部输入。
func (s Spec) IsInput(name string) bool {
	for _, in := range s.Inputs {
		if in.Name == name {
			return true
		}
	}
	return false
}

// Load 加载场景声明。
func Load(path string) (Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	s, err := parseSpec(string(b))
	if err != nil {
		return Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// parseSpec 从字符串解析场景声明。
func parseSpec(src string) (Spec, error) {
	var s Spec
	if err := yaml.Unmarshal([]byte(src), &s); err != nil {
		return Spec{}, err
	}
	if s.Name == "" {
		return Spec{}, fmt.Errorf("场景缺少 scenario 名")
	}
	if s.Entity == "" {
		return Spec{}, fmt.Errorf("场景 %s 缺少 entity——运行时的主体来自哪个实体必须写清楚，"+
			"在代码里写死会让换主实体变成改程序", s.Name)
	}
	for _, r := range s.Requires {
		if !strings.Contains(r, ".") {
			return Spec{}, fmt.Errorf("requires 项 %q 必须形如 entity.predicate——无法判定的需求不得声明", r)
		}
	}
	for _, in := range s.Inputs {
		if in.Name == "" {
			return Spec{}, fmt.Errorf("外部输入缺少 name")
		}
		if in.Unit == "" {
			return Spec{}, fmt.Errorf(
				"外部输入 %q 缺少 unit——没有量纲的输入无法参与计算，`0.5` 与 `50%%` 的歧义正是这么来的", in.Name)
		}
		if in.Min != nil && in.Max != nil && *in.Min > *in.Max {
			return Spec{}, fmt.Errorf("外部输入 %q 的 min(%v) 大于 max(%v)", in.Name, *in.Min, *in.Max)
		}
	}
	seenRef := map[string]bool{}
	for _, r := range s.Refs {
		if r.Name == "" {
			return Spec{}, fmt.Errorf("引用声明缺少 name")
		}
		if r.Entity == "" {
			return Spec{}, fmt.Errorf("引用 %q 缺少 entity——不知道去哪找候选", r.Name)
		}
		if r.Via == "" {
			return Spec{}, fmt.Errorf(
				"引用 %q 缺少 via——没有它就无法列出候选，界面只能让人凭记忆敲 ID", r.Name)
		}
		if seenRef[r.Name] {
			return Spec{}, fmt.Errorf("引用 %q 重复声明", r.Name)
		}
		seenRef[r.Name] = true
		if s.IsInput(r.Name) {
			return Spec{}, fmt.Errorf("引用 %q 与外部输入同名：同一个绑定路径只能有一个来源", r.Name)
		}
	}
	return s, nil
}

// LoadDir 加载目录下第一个场景声明。
func LoadDir(dir string) (Spec, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.scenario.yml"))
	if err != nil {
		return Spec{}, err
	}
	if len(paths) == 0 {
		return Spec{}, fmt.Errorf("%s 下没有找到 *.scenario.yml", dir)
	}
	sort.Strings(paths)
	return Load(paths[0])
}

// Skipped 是一个被跳过的场景目录及其原因。
//
// 跳过必须**带原因**：一场静默的跳过会让人以为「项目只有两个场景」，
// 而事实是第三个场景的声明文件写坏了。
type Skipped struct {
	Dir    string
	Reason string
}

// ProjectScenarios 列出项目下的全部场景，按名称排序。
//
// 约定 `scenarios/<目录>/<目录>.scenario.yml`；也接受目录下任意单个
// `*.scenario.yml`（实测中文件名与目录名并不总是相同）。
//
// 返回的 skipped 不能丢：调用方有义务把它显示出来。
func ProjectScenarios(projectDir string) (specs []Spec, skipped []Skipped, err error) {
	root := filepath.Join(projectDir, "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil // 没有 scenarios 目录 = 该项目还没有场景
		}
		return nil, nil, err
	}

	seen := map[string]string{} // 场景名 -> 首次出现的目录
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		paths, err := filepath.Glob(filepath.Join(dir, "*.scenario.yml"))
		if err != nil {
			return nil, nil, err
		}
		switch len(paths) {
		case 0:
			skipped = append(skipped, Skipped{Dir: dir, Reason: "目录下没有 *.scenario.yml"})
			continue
		case 1:
		default:
			sort.Strings(paths)
			skipped = append(skipped, Skipped{
				Dir:    dir,
				Reason: fmt.Sprintf("目录下有 %d 个场景声明，无法确定用哪个", len(paths)),
			})
			continue
		}

		s, err := Load(paths[0])
		if err != nil {
			skipped = append(skipped, Skipped{Dir: dir, Reason: err.Error()})
			continue
		}
		if first, dup := seen[s.Name]; dup {
			// 同名场景是定义冲突：它会让「选中某场景」变成一件含糊的事。
			return nil, skipped, fmt.Errorf("场景名 %q 重复：%s 与 %s", s.Name, first, dir)
		}
		seen[s.Name] = dir
		specs = append(specs, s)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs, skipped, nil
}

// Find 按名称取场景声明。
func Find(specs []Spec, name string) (Spec, bool) {
	for _, s := range specs {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// Store 是完整性检查所需的读取端口。
type Store interface {
	PredicateCount(entity, predicate string) (int, error)
	SubjectCount(entity string) (int, error)
}

// Status 是单项 requires 的状态。
type Status string

const (
	Satisfied Status = "satisfied" // 已满足
	Missing   Status = "missing"   // 数据不存在
	Unusable  Status = "unusable"  // 存在但不可用（例如依赖的公式算例未通过）
)

// Requirement 是一项 requires 的检查结果。
type Requirement struct {
	Want     string
	Status   Status
	Have     int
	Total    int
	Coverage float64
	Detail   string
}

// FormulaStatus 是一个依赖公式的状态。
type FormulaStatus struct {
	Name   string
	Status formula.Status
	Cases  []formula.CaseResult
}

// CheckReport 是场景的完整性检查结果。
type CheckReport struct {
	Scenario     string
	Requirements []Requirement
	Formulas     []FormulaStatus
}

// Runnable 报告场景是否可以运行。
//
// requires 缺失时**拒绝运行**，不得静默降级，也不得用不完整数据产出方案。
func (r CheckReport) Runnable() bool {
	for _, q := range r.Requirements {
		if q.Status != Satisfied {
			return false
		}
	}
	for _, f := range r.Formulas {
		if f.Status == formula.StatusFailed {
			return false
		}
	}
	return true
}

// Missing 返回缺失项清单。
func (r CheckReport) Missing() []string {
	var out []string
	for _, q := range r.Requirements {
		if q.Status != Satisfied {
			out = append(out, q.Want+"（"+string(q.Status)+"）")
		}
	}
	for _, f := range r.Formulas {
		if f.Status == formula.StatusFailed {
			out = append(out, "公式 "+f.Name+" 算例未通过")
		}
	}
	return out
}

// Run 执行一次完整运行：完整性检查 → 取数 → 求值。
//
// 顺序不能颠倒，也不能跳过检查：**requires 缺失时拒绝运行**，
// 不得用不完整数据产出方案。检查报告一并返回，好让调用方
// 在成功时也能显示「覆盖了 51%，存在缺口」这类信息。
func Run(spec Spec, formulas map[string]*formula.Formula, st Store, r ValueReader,
	units *unit.Table, in RunInput) (Output, CheckReport, error) {

	rep, err := Check(spec, st, formulas, units)
	if err != nil {
		return Output{}, rep, err
	}
	if !rep.Runnable() {
		return Output{}, rep, fmt.Errorf(
			"场景 %s 的 requires 未满足，拒绝运行（不以不完整数据产出方案）：%v",
			spec.Name, rep.Missing())
	}

	if len(spec.Formulas) == 0 {
		return Output{}, rep, fmt.Errorf("场景 %s 没有声明公式", spec.Name)
	}
	fname := spec.Formulas[0]
	f, ok := formulas[fname]
	if !ok {
		return Output{}, rep, fmt.Errorf("场景依赖的公式 %q 不存在", fname)
	}
	_, fst := f.Verify(units)

	out, err := Execute(spec, f, r, units, in, fst)
	return out, rep, err
}

// Check 执行完整性检查。
func Check(spec Spec, st Store, formulas map[string]*formula.Formula, units *unit.Table) (CheckReport, error) {
	rep := CheckReport{Scenario: spec.Name}

	totalCache := map[string]int{}
	for _, want := range spec.Requires {
		parts := strings.SplitN(want, ".", 2)
		entity, predicate := parts[0], parts[1]

		total, ok := totalCache[entity]
		if !ok {
			n, err := st.SubjectCount(entity)
			if err != nil {
				return rep, err
			}
			total = n
			totalCache[entity] = n
		}
		have, err := st.PredicateCount(entity, predicate)
		if err != nil {
			return rep, err
		}

		q := Requirement{Want: want, Have: have, Total: total}
		if total > 0 {
			q.Coverage = float64(have) / float64(total)
		}
		switch {
		case total == 0:
			q.Status = Missing
			q.Detail = "库中没有该实体的任何数据"
		case have == 0:
			q.Status = Missing
			q.Detail = "该谓词完全没有数据"
		default:
			q.Status = Satisfied
			if q.Coverage < 1 {
				q.Detail = fmt.Sprintf("覆盖 %d/%d（%.0f%%），存在缺口", have, total, q.Coverage*100)
			}
		}
		rep.Requirements = append(rep.Requirements, q)
	}

	for _, name := range spec.Formulas {
		f, ok := formulas[name]
		if !ok {
			rep.Formulas = append(rep.Formulas, FormulaStatus{Name: name, Status: formula.Status("absent")})
			continue
		}
		cases, st := f.Verify(units)
		rep.Formulas = append(rep.Formulas, FormulaStatus{Name: name, Status: st, Cases: cases})
	}
	return rep, nil
}

// ValueReader 提供主体上的断言取值。
type ValueReader interface {
	BySubject(entity, subject string) ([]assertion.Assertion, error)
}

// Ref 指向另一个实体上的主体。
//
// 场景的输入不必都来自同一个实体：伤害计算需要式神的攻击与暴击系数、
// 以及技能的倍率——后者在另一个实体上。
type Ref struct {
	Entity  string
	Subject string
}

// RunInput 是运行场景的输入。
//
// 三类来源，必须分清：
//
//	Entity/Subject —— 主实体（例如式神）
//	Extra          —— 其他实体上的引用（例如技能）
//	Values         —— 库中本不该有的外部输入（例如防御减免）
//
// 注意 def_reduction：**我们没有它的权威来源**，因此它是外部输入而非内置常量。
// 缺的部分必须显式标注，不能猜。
type RunInput struct {
	Entity  string
	Subject string
	Extra   map[string]Ref    // 绑定路径 -> 其他实体引用
	Values  map[string]string // 绑定路径 -> 字面量
}

// Output 是场景产出。
type Output struct {
	Scenario   string
	Subject    string
	Bindings   map[string]expr.Val
	Result     expr.Val
	Notes      []string
	Unverified []string // 未核验的依赖
}

// Execute 运行场景：取数 → 求值 → 产出（含标注）。
func Execute(spec Spec, f *formula.Formula, r ValueReader, units *unit.Table, in RunInput, formulaState formula.Status) (Output, error) {
	out := Output{Scenario: spec.Name, Subject: in.Subject, Bindings: map[string]expr.Val{}}

	as, err := r.BySubject(in.Entity, in.Subject)
	if err != nil {
		return out, err
	}
	byPred := map[string]assertion.Assertion{}
	for _, a := range as {
		byPred[a.Predicate] = a
	}

	for name, path := range f.Bindings {
		// 1. 外部输入优先（调用方显式提供）
		if lit, ok := in.Values[path]; ok {
			v, err := formula.ParseLiteral(lit)
			if err != nil {
				return out, fmt.Errorf("输入 %s 无法解析：%w", path, err)
			}
			out.Bindings[name] = v
			out.Notes = append(out.Notes, fmt.Sprintf("输入 %s=%s 由调用方提供，**未核验**", path, lit))
			out.Unverified = append(out.Unverified, path)
			continue
		}

		// 2. 主实体上的断言
		a, ok := byPred[path]

		// 3. 其他实体上的断言（例如技能的倍率）
		if !ok {
			if ref, hasRef := in.Extra[path]; hasRef {
				other, err := r.BySubject(ref.Entity, ref.Subject)
				if err != nil {
					return out, err
				}
				for _, x := range other {
					if x.Predicate == path {
						a, ok = x, true
						break
					}
				}
				if !ok {
					return out, fmt.Errorf("%s=%s 上没有 %s", ref.Entity, ref.Subject, path)
				}
			}
		}

		if !ok {
			if spec.IsInput(path) {
				return out, fmt.Errorf("缺少外部输入 %s，请用 --bind %s=<字面量> 提供", path, path)
			}
			return out, fmt.Errorf("缺少输入 %s——它既不在库中，也未在场景的 inputs 中声明", path)
		}
		v, err := formula.FromValue(a.Value)
		if err != nil {
			return out, fmt.Errorf("输入 %s 无法参与计算：%w", path, err)
		}
		if v.Kind == expr.KUnknown {
			return out, fmt.Errorf("输入 %s 为未知，拒绝以不完整数据产出方案", path)
		}
		out.Bindings[name] = v
		if a.Status != assertion.StatusVerified {
			out.Notes = append(out.Notes, fmt.Sprintf("输入 %s 的断言状态为 %s（未核验）", path, a.Status))
			out.Unverified = append(out.Unverified, path)
		}
	}

	if formulaState == formula.StatusUnverified {
		out.Notes = append(out.Notes, "公式 "+f.Name+" 无算例，**未验证**")
	}
	if len(out.Unverified) > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("本产出依赖 %d 项未核验输入，结论仅供参考", len(out.Unverified)))
	}

	res, err := f.Eval(out.Bindings, units)
	if err != nil {
		return out, err
	}
	out.Result = res
	return out, nil
}
