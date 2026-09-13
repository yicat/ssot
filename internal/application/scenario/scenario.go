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
}

// Spec 是场景声明。
type Spec struct {
	Name        string   `yaml:"scenario"`
	Description string   `yaml:"description"`
	Requires    []string `yaml:"requires"` // "entity.predicate"
	Inputs      []Input  `yaml:"inputs"`   // 运行时外部输入
	Formulas    []string `yaml:"formulas"`
	Outputs     []string `yaml:"outputs"`
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
	for _, r := range s.Requires {
		if !strings.Contains(r, ".") {
			return Spec{}, fmt.Errorf("requires 项 %q 必须形如 entity.predicate——无法判定的需求不得声明", r)
		}
	}
	for _, in := range s.Inputs {
		if in.Name == "" {
			return Spec{}, fmt.Errorf("外部输入缺少 name")
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
